package fork

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pig/cli/ext"
	"pig/cli/pgbackrest"
	"pig/cli/postgres"
	"pig/internal/output"
	"pig/internal/utils"
)

type ForkError struct {
	Code int
	Err  error
}

func (e *ForkError) Error() string {
	if e == nil || e.Err == nil {
		return "fork error"
	}
	return e.Err.Error()
}

func (e *ForkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Plan(opts *Options) (*output.Plan, error) {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	if n.Kind == KindInstance {
		sourceData, destData, err := validateForkDataPaths(n.Instance.SourceData, n.Instance.DestData)
		if err != nil {
			return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
		}
		n.Instance.SourceData = sourceData
		n.Instance.DestData = destData
	} else if n.Kind == KindDatabase {
		if err := prepareDatabaseClone(n); err != nil {
			return nil, err
		}
	}
	return BuildPlan(n, inferPlanState(n)), nil
}

func Execute(opts *Options) error {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return exitForkError(output.CodeForkInvalidArgs, err)
	}
	if n.Kind == KindDatabase {
		if err := prepareDatabaseClone(n); err != nil {
			var fe *ForkError
			if errors.As(err, &fe) {
				return &utils.ExitCodeError{Code: output.ExitCode(fe.Code), Err: fe}
			}
			return err
		}
		printDatabasePreflight(n.Database.Preflight, n.Database.Warnings)
	}
	if !n.Yes {
		if n.Kind == KindDatabase {
			if err := confirmDatabaseWarnings(n.Database.Warnings, "CLONE", 10); err != nil {
				return exitForkError(output.CodeForkInvalidArgs, err)
			}
		} else if err := pgbackrest.ConfirmWithCountdown("This will create a PostgreSQL instance fork and may replace the destination!", "FORK"); err != nil {
			return exitForkError(output.CodeForkInvalidArgs, err)
		}
	}
	data, err := executeNormalized(n)
	if err != nil {
		if fe, ok := err.(*ForkError); ok {
			return &utils.ExitCodeError{Code: output.ExitCode(fe.Code), Err: fe}
		}
		return err
	}
	if n.Kind == KindDatabase {
		printDatabaseResult(data)
	}
	return nil
}

func ExecuteResult(opts *Options) *output.Result {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return output.Fail(output.CodeForkInvalidArgs, err.Error())
	}
	if n.Kind == KindDatabase {
		if err := prepareDatabaseClone(n); err != nil {
			if fe, ok := err.(*ForkError); ok {
				return output.Fail(fe.Code, fe.Error())
			}
			return output.Fail(output.CodeForkPrecheckFailed, err.Error())
		}
	}
	data, err := executeNormalized(n)
	if err != nil {
		if fe, ok := err.(*ForkError); ok {
			return output.Fail(fe.Code, fe.Error())
		}
		return output.Fail(output.CodeForkPrecheckFailed, err.Error())
	}
	if n.Kind == KindDatabase {
		return output.OK("database clone completed", data)
	}
	return output.OK("instance fork completed", data)
}

func executeNormalized(opts *Options) (ResultData, error) {
	start := time.Now()
	switch opts.Kind {
	case KindInstance:
		state, err := precheckInstance(opts)
		if err != nil {
			return ResultData{}, err
		}
		if err := executeInstance(opts, state); err != nil {
			return ResultData{}, err
		}
		return instanceResult(opts, state, time.Since(start)), nil
	case KindDatabase:
		if err := executeDatabase(opts); err != nil {
			return ResultData{}, err
		}
		return databaseResult(opts, time.Since(start)), nil
	default:
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: fmt.Errorf("invalid fork kind %q", opts.Kind)}
	}
}

func exitForkError(code int, err error) error {
	fe := &ForkError{Code: code, Err: err}
	return &utils.ExitCodeError{Code: output.ExitCode(code), Err: fe}
}

func inferPlanState(opts *Options) *State {
	state := &State{CloneMode: CloneModeUnknown}
	if opts == nil {
		return state
	}
	switch opts.Mode {
	case ModeCold:
		state.BackupMode = BackupModeCold
	default:
		state.BackupMode = BackupModeHot
	}
	return state
}

func precheckInstance(opts *Options) (*State, error) {
	inst := opts.Instance
	sourceData, destData, err := validateForkDataPaths(inst.SourceData, inst.DestData)
	if err != nil {
		return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	opts.Instance.SourceData = sourceData
	opts.Instance.DestData = destData
	inst = opts.Instance

	exists, initialized := postgres.CheckDataDirAsDBSU(opts.DbSU, inst.SourceData)
	if !exists || !initialized {
		return nil, &ForkError{Code: output.CodeForkSourceNotFound, Err: fmt.Errorf("source data directory is not initialized: %s", inst.SourceData)}
	}

	if destExists, _ := postgres.CheckDataDirAsDBSU(opts.DbSU, inst.DestData); destExists && !opts.Replace {
		return nil, &ForkError{Code: output.CodeForkDestExists, Err: fmt.Errorf("destination data directory exists: %s (use --replace)", inst.DestData)}
	}

	if opts.Start && !isPortFree(inst.DestPort) {
		return nil, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is in use: %d", inst.DestPort)}
	}

	running := canConnect(opts.DbSU, inst.SourcePort)
	if opts.Mode == ModeHot && !running {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source instance is not reachable on port %d; use --mode cold if it is stopped", inst.SourcePort)}
	}
	if (opts.Mode == ModeCold || (opts.Mode == ModeAuto && !running)) && hasPostmasterPID(opts.DbSU, inst.SourceData) {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source data directory has postmaster.pid; refusing cold copy of a possibly running instance")}
	}

	mode := BackupModeHot
	if opts.Mode == ModeCold || (opts.Mode == ModeAuto && !running) {
		mode = BackupModeCold
	}
	cloneMode, fs := detectCloneMode(inst.SourceData, inst.DestData)
	state := &State{BackupMode: mode, CloneMode: cloneMode, FS: fs}
	if err := requireCOW(state, opts.Replace); err != nil {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	return state, nil
}

func executeInstance(opts *Options, state *State) error {
	inst := opts.Instance
	if state.BackupMode == BackupModeCold {
		if err := coldCopy(opts.DbSU, inst.SourceData, inst.DestData); err != nil {
			return &ForkError{Code: output.CodeForkCopyFailed, Err: err}
		}
	} else {
		if err := hotCopy(opts.DbSU, inst); err != nil {
			return err
		}
	}

	if err := configureInstance(opts.DbSU, inst.DestData, inst.DestPort); err != nil {
		return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}

	if opts.Start {
		cfg := &postgres.Config{PgData: inst.DestData, DbSU: opts.DbSU}
		if err := postgres.Start(cfg, &postgres.StartOptions{Timeout: inst.Timeout}); err != nil {
			return &ForkError{Code: output.CodeForkStartFailed, Err: err}
		}
		if err := verifyInstance(opts.DbSU, inst.DestPort); err != nil {
			return &ForkError{Code: output.CodeForkVerifyFailed, Err: err}
		}
		state.Started = true
	}
	info := BuildForkInfo(opts, state)
	if err := WriteForkInfoAs(opts.DbSU, inst.DestData, info); err != nil {
		return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}
	return nil
}

func coldCopy(dbsu, src, dst string) error {
	return copyDataDir(dbsu, src, dst)
}

func hotCopy(dbsu string, inst InstanceOptions) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return &ForkError{Code: output.CodeForkDependencyMissing, Err: fmt.Errorf("postgresql not found: %w", err)}
	}
	session, err := newPsqlBackupSession(dbsu, pg.Psql(), inst.SourcePort)
	if err != nil {
		return &ForkError{Code: output.CodeForkBackupFailed, Err: err}
	}
	defer session.Close()

	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	label := fmt.Sprintf("pig_fork_%s_%s", inst.Name, time.Now().Format("20060102_150405"))
	copyFn := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyDataDir(dbsu, inst.SourceData, inst.DestData); err != nil {
			return err
		}
		return ctx.Err()
	}
	if err := runHotBackupCopy(session, label, copyFn); err != nil {
		var copyErr copyPhaseError
		if errors.As(err, &copyErr) {
			return &ForkError{Code: output.CodeForkCopyFailed, Err: copyErr.Err}
		}
		return &ForkError{Code: output.CodeForkBackupFailed, Err: err}
	}
	if err := validateCopiedDataDir(dbsu, inst.DestData); err != nil {
		return &ForkError{Code: output.CodeForkCopyFailed, Err: err}
	}
	return nil
}

func copyDataDir(dbsu, src, dst string) error {
	script := fmt.Sprintf(`set -e
rm -rf %s
cp -a --reflink=auto %s %s
test -f %s
`, quoteArg(dst), quoteArg(src), quoteArg(dst), quoteArg(filepath.Join(dst, "PG_VERSION")))
	return utils.DBSUCommand(dbsu, []string{"sh", "-c", script})
}

func validateCopiedDataDir(dbsu, dst string) error {
	return utils.DBSUCommand(dbsu, []string{"test", "-f", filepath.Join(dst, "PG_VERSION")})
}

type backupSession interface {
	Exec(sql string) (string, error)
	Close() error
}

type backupFunctions struct {
	start  string
	stop   string
	legacy bool
}

type copyPhaseError struct {
	Err error
}

func (e copyPhaseError) Error() string {
	return e.Err.Error()
}

func (e copyPhaseError) Unwrap() error {
	return e.Err
}

func runHotBackupCopy(session backupSession, label string, copyFn func() error) error {
	version, err := backupServerVersion(session)
	if err != nil {
		return err
	}
	names := backupFunctionNames(version)
	if _, err := session.Exec(buildBackupStartSQL(label, names)); err != nil {
		return err
	}
	copyErr := copyFn()
	_, stopErr := session.Exec(buildBackupStopSQL(names))
	if copyErr != nil {
		if stopErr != nil {
			return copyPhaseError{Err: fmt.Errorf("copy failed: %w; backup stop also failed: %v", copyErr, stopErr)}
		}
		return copyPhaseError{Err: copyErr}
	}
	return stopErr
}

func backupServerVersion(session backupSession) (int, error) {
	out, err := session.Exec("SELECT current_setting('server_version_num')")
	if err != nil {
		return 0, err
	}
	version, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("invalid server_version_num %q: %w", strings.TrimSpace(out), err)
	}
	return version, nil
}

func backupFunctionNames(version int) backupFunctions {
	if version < 150000 {
		return backupFunctions{start: "pg_start_backup", stop: "pg_stop_backup", legacy: true}
	}
	return backupFunctions{start: "pg_backup_start", stop: "pg_backup_stop"}
}

func buildBackupStartSQL(label string, names backupFunctions) string {
	if names.legacy {
		return fmt.Sprintf("CHECKPOINT;\nSELECT %s('%s', true, false);\n", names.start, EscapeSQLString(label))
	}
	return fmt.Sprintf("CHECKPOINT;\nSELECT %s('%s', fast => true);\n", names.start, EscapeSQLString(label))
}

func buildBackupStopSQL(names backupFunctions) string {
	if names.legacy {
		return fmt.Sprintf("SELECT * FROM %s(false, false);\n", names.stop)
	}
	return fmt.Sprintf("SELECT * FROM %s(wait_for_archive => false);\n", names.stop)
}

type psqlBackupSession struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	out     *bufio.Scanner
	errbuf  *bytes.Buffer
	seq     int
	waited  bool
	waitErr error
}

func newPsqlBackupSession(dbsu, psql string, port int) (*psqlBackupSession, error) {
	args := []string{psql, "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-p", fmt.Sprintf("%d", port), "-d", "postgres"}
	cmd, err := utils.BuildDBSUCommand(dbsu, args)
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errbuf := &bytes.Buffer{}
	cmd.Stderr = errbuf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	return &psqlBackupSession{cmd: cmd, stdin: stdin, out: scanner, errbuf: errbuf}, nil
}

func (s *psqlBackupSession) Exec(sql string) (string, error) {
	s.seq++
	marker := fmt.Sprintf("__PIG_FORK_SQL_DONE_%d__", s.seq)
	if _, err := fmt.Fprintf(s.stdin, "%s\n\\echo %s\n", strings.TrimSpace(sql), marker); err != nil {
		return "", err
	}
	lines := []string{}
	for s.out.Scan() {
		line := strings.TrimSpace(s.out.Text())
		if line == marker {
			return strings.TrimSpace(strings.Join(lines, "\n")), nil
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := s.out.Err(); err != nil {
		return strings.TrimSpace(strings.Join(lines, "\n")), err
	}
	waitErr := s.wait()
	errText := strings.TrimSpace(s.errbuf.String())
	if waitErr != nil {
		if errText != "" {
			return strings.TrimSpace(strings.Join(lines, "\n")), fmt.Errorf("%w: %s", waitErr, errText)
		}
		return strings.TrimSpace(strings.Join(lines, "\n")), waitErr
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), fmt.Errorf("psql session ended before marker %s", marker)
}

func (s *psqlBackupSession) Close() error {
	if s == nil {
		return nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	return s.wait()
}

func (s *psqlBackupSession) wait() error {
	if s.waited {
		return s.waitErr
	}
	s.waitErr = s.cmd.Wait()
	s.waited = true
	return s.waitErr
}

func configureInstance(dbsu, dataDir string, port int) error {
	script := fmt.Sprintf(`set -e
dir=%s
autoconf="$dir/postgresql.auto.conf"
rm -f "$dir/postmaster.pid" "$dir/postmaster.opts" "$dir/standby.signal" "$dir/recovery.signal"
rm -rf "$dir/pg_replslot/"*
touch "$autoconf"
set_param() {
  key="$1"
  value="$2"
  if grep -q "^${key} = " "$autoconf" 2>/dev/null; then
    sed -i "s|^${key} = .*|${key} = ${value}|" "$autoconf"
  else
    printf '%%s = %%s\n' "$key" "$value" >> "$autoconf"
  fi
}
set_param port %d
set_param archive_mode off
set_param log_directory "'log'"
sed -i '/^primary_conninfo/d;/^primary_slot_name/d;/^recovery_target/d' "$autoconf" 2>/dev/null || true
`, quoteArg(dataDir), port)
	return utils.DBSUCommand(dbsu, []string{"sh", "-c", script})
}

func verifyInstance(dbsu string, port int) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return err
	}
	return utils.DBSUCommand(dbsu, []string{pg.Psql(), "-p", fmt.Sprintf("%d", port), "-d", "postgres", "-Atc", "SELECT 1"})
}

func executeDatabase(opts *Options) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return &ForkError{Code: output.CodeForkDependencyMissing, Err: fmt.Errorf("postgresql not found: %w", err)}
	}
	sql := BuildDatabaseCloneSQL(&opts.Database)
	args := []string{pg.Psql(), "-p", fmt.Sprintf("%d", opts.Database.Port), "-d", opts.Database.ConnDB, "-f"}
	utils.PrintHint(append(args, "<clone-sql>"))
	if err := runSQLFile(opts.DbSU, args, sql); err != nil {
		return &ForkError{Code: output.CodeForkDatabaseFailed, Err: err}
	}
	if ownerSQL := BuildDatabaseAlterOwnerSQL(opts.Database.DestDB, opts.Database.Owner); ownerSQL != "" {
		ownerArgs := []string{pg.Psql(), "-p", fmt.Sprintf("%d", opts.Database.Port), "-d", opts.Database.ConnDB, "-f"}
		utils.PrintHint(append(ownerArgs, "<owner-sql>"))
		if err := runSQLFile(opts.DbSU, ownerArgs, ownerSQL); err != nil {
			opts.Database.OwnerChanged = false
			opts.Database.OwnerWarning = err.Error()
		} else {
			opts.Database.OwnerChanged = true
		}
	}
	return nil
}

type databaseInfo struct {
	Name string
}

func prepareDatabaseClone(opts *Options) error {
	if opts == nil || opts.Kind != KindDatabase {
		return nil
	}
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return &ForkError{Code: output.CodeForkDependencyMissing, Err: fmt.Errorf("postgresql not found: %w", err)}
	}
	preflight := DatabasePreflight{Strategy: "FILE_COPY"}
	versionText, err := runPsqlQuery(opts.DbSU, pg.Psql(), opts.Database.Port, opts.Database.ConnDB, "SELECT current_setting('server_version_num')")
	if err != nil {
		return &ForkError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	version, err := strconv.Atoi(strings.TrimSpace(versionText))
	if err != nil {
		return &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("invalid server_version_num %q: %w", strings.TrimSpace(versionText), err)}
	}
	preflight.ServerVersion = version
	if fileCopyMethod, err := runPsqlQuery(opts.DbSU, pg.Psql(), opts.Database.Port, opts.Database.ConnDB, "SHOW file_copy_method"); err == nil {
		preflight.FileCopyMethod = strings.TrimSpace(fileCopyMethod)
	} else {
		preflight.FileCopyMethodError = err.Error()
	}
	if dataDirectory, err := runPsqlQuery(opts.DbSU, pg.Psql(), opts.Database.Port, opts.Database.ConnDB, "SHOW data_directory"); err == nil {
		preflight.DataDirectory = strings.TrimSpace(dataDirectory)
		cloneMode, fs, fsErr := detectCloneModeForPath(preflight.DataDirectory)
		preflight.CloneMode = cloneMode
		preflight.FileSystem = fs
		if fsErr != nil {
			preflight.FileSystemError = fsErr.Error()
		}
	} else {
		preflight.FileSystemError = err.Error()
	}
	opts.Database.Preflight = preflight

	databases, err := listDatabases(opts.DbSU, pg.Psql(), opts.Database.Port, opts.Database.ConnDB)
	if err != nil {
		return &ForkError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	existing := make(map[string]bool, len(databases))
	sourceFound := false
	for _, db := range databases {
		existing[db.Name] = true
		if db.Name == opts.Database.SourceDB {
			sourceFound = true
		}
	}
	if !sourceFound {
		return &ForkError{Code: output.CodeForkSourceNotFound, Err: fmt.Errorf("source database does not exist: %s", opts.Database.SourceDB)}
	}
	if opts.Database.DestDB == "" {
		opts.Database.DestDB = NextDatabaseCloneName(opts.Database.SourceDB, existing)
	} else if existing[opts.Database.DestDB] {
		return &ForkError{Code: output.CodeForkDestExists, Err: fmt.Errorf("destination database already exists: %s", opts.Database.DestDB)}
	}
	opts.Database.Warnings = opts.Database.Preflight.Warnings()
	return nil
}

func listDatabases(dbsu, psql string, port int, connDB string) ([]databaseInfo, error) {
	sql := "SELECT datname FROM pg_database ORDER BY datname"
	out, err := runPsqlQuery(dbsu, psql, port, connDB, sql)
	if err != nil {
		return nil, err
	}
	names := parseDatabaseNames(out)
	databases := make([]databaseInfo, 0, len(names))
	for _, name := range names {
		databases = append(databases, databaseInfo{
			Name: name,
		})
	}
	return databases, nil
}

func parseDatabaseNames(out string) []string {
	if strings.TrimSpace(out) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

func runPsqlQuery(dbsu, psql string, port int, connDB string, sql string) (string, error) {
	args := []string{
		psql,
		"-X",
		"-qAt",
		"-F", "\t",
		"-v", "ON_ERROR_STOP=1",
		"-p", fmt.Sprintf("%d", port),
		"-d", connDB,
		"-c", sql,
	}
	return utils.DBSUCommandOutput(dbsu, args)
}

func runSQLFile(dbsu string, args []string, sql string) error {
	file, err := os.CreateTemp("", "pig-fork-*.sql")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString(sql); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(path, 0644); err != nil {
		return err
	}
	args = append(args, path)
	return utils.DBSUCommand(dbsu, args)
}

func confirmDatabaseWarnings(warnings []string, action string, seconds int) error {
	if len(warnings) == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\n%sWARNING: preflight warnings above mean this may become a heavy production file copy.%s\n", utils.ColorYellow, utils.ColorReset)
	fmt.Fprintln(os.Stderr, "Press Ctrl+C to cancel, or wait for countdown...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigChan)
		close(sigChan)
	}()

	for i := seconds; i > 0; i-- {
		select {
		case <-sigChan:
			fmt.Fprintf(os.Stderr, "\n%s cancelled.\n", action)
			return fmt.Errorf("%s cancelled by user", action)
		case <-time.After(time.Second):
			fmt.Fprintf(os.Stderr, "\rStarting %s in %d seconds... ", action, i)
		}
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func canConnect(dbsu string, port int) bool {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return false
	}
	_, err = utils.DBSUCommandOutput(dbsu, []string{pg.Psql(), "-p", fmt.Sprintf("%d", port), "-d", "postgres", "-Atc", "SELECT 1"})
	return err == nil
}

func isPortFree(port int) bool {
	if !validPort(port) {
		return false
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func hasPostmasterPID(dbsu, dataDir string) bool {
	_, err := utils.DBSUCommandOutput(dbsu, []string{"cat", filepath.Join(dataDir, "postmaster.pid")})
	return err == nil
}

func validateForkDataPaths(src, dst string) (string, string, error) {
	srcPath, err := normalizeDataPath(src)
	if err != nil {
		return "", "", err
	}
	dstPath, err := normalizeDataPath(dst)
	if err != nil {
		return "", "", err
	}
	dstLiteral, err := cleanAbsPath(dst)
	if err != nil {
		return "", "", err
	}
	if dstLiteral == "/" || dstLiteral == "/pg" || dstPath == "/" || dstPath == "/pg" {
		return "", "", fmt.Errorf("unsafe destination data directory: %s", dst)
	}
	if srcPath == dstPath {
		return "", "", fmt.Errorf("source and destination data directories must differ: %s", srcPath)
	}
	if pathContains(dstPath, srcPath) {
		return "", "", fmt.Errorf("destination data directory must not be a parent of source data directory: %s", dstPath)
	}
	if pathContains(srcPath, dstPath) {
		return "", "", fmt.Errorf("destination data directory must not be inside source data directory: %s", dstPath)
	}
	return srcPath, dstPath, nil
}

func normalizeDataPath(path string) (string, error) {
	cleaned, err := cleanAbsPath(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return filepath.Clean(resolved), nil
	}

	parent := cleaned
	suffix := []string{}
	for {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		next := filepath.Dir(parent)
		if next == parent {
			return cleaned, nil
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = next
	}
}

func cleanAbsPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("data directory path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func requireCOW(state *State, force bool) error {
	if state != nil && state.CloneMode == CloneModeCOW {
		return nil
	}
	fs := "unknown"
	if state != nil && state.FS != "" {
		fs = state.FS
	}
	if force {
		return nil
	}
	return fmt.Errorf("copy-on-write is not available on source filesystem %q; use --force to allow regular copy fallback", fs)
}

func detectCloneModeForPath(path string) (CloneMode, string, error) {
	mount, fs := dfMountAndFS(path)
	if mount == "" || fs == "" {
		return CloneModeCopy, "", fmt.Errorf("unable to detect filesystem for %s", path)
	}
	switch strings.ToLower(fs) {
	case "xfs":
		if xfsReflinkEnabled(mount) {
			return CloneModeCOW, fs, nil
		}
		return CloneModeCopy, fs, nil
	case "btrfs", "bcachefs", "ocfs2", "apfs":
		return CloneModeCOW, fs, nil
	default:
		return CloneModeCopy, fs, nil
	}
}

func detectCloneMode(src, dst string) (CloneMode, string) {
	dstParent := existingParent(filepath.Dir(dst))
	srcMount, srcFS := dfMountAndFS(src)
	dstMount, _ := dfMountAndFS(dstParent)
	if srcMount == "" || dstMount == "" || srcMount != dstMount {
		return CloneModeCopy, srcFS
	}
	switch strings.ToLower(srcFS) {
	case "xfs":
		if xfsReflinkEnabled(srcMount) {
			return CloneModeCOW, srcFS
		}
		return CloneModeCopy, srcFS
	case "btrfs", "bcachefs", "ocfs2":
		return CloneModeCOW, srcFS
	default:
		return CloneModeCopy, srcFS
	}
}

func xfsReflinkEnabled(mount string) bool {
	out, err := exec.Command("xfs_info", mount).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "reflink=1")
}

func existingParent(path string) string {
	for path != "" && path != "." && path != "/" {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			return path
		}
		path = filepath.Dir(path)
	}
	return "/"
}

func dfMountAndFS(path string) (string, string) {
	out, err := exec.Command("df", "-T", path).Output()
	if err != nil {
		return "", ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "", ""
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 7 {
		return "", ""
	}
	return fields[6], fields[1]
}

func instanceResult(opts *Options, state *State, elapsed time.Duration) ResultData {
	inst := opts.Instance
	return ResultData{
		Kind:            KindInstance,
		Source:          inst.SourceData,
		Destination:     inst.DestData,
		SourcePort:      inst.SourcePort,
		DestinationPort: inst.DestPort,
		BackupMode:      string(state.BackupMode),
		CloneMode:       string(state.CloneMode),
		Started:         state.Started,
		ConnectCommand:  fmt.Sprintf("psql -p %d", inst.DestPort),
		CleanupCommand:  fmt.Sprintf("pg_ctl -D %s stop; rm -rf %s", inst.DestData, inst.DestData),
		Duration:        elapsed.Seconds(),
	}
}

func databaseResult(opts *Options, elapsed time.Duration) ResultData {
	db := opts.Database
	warnings := append([]string{}, db.Warnings...)
	preflight := db.Preflight
	data := ResultData{
		Kind:           KindDatabase,
		Source:         db.SourceDB,
		Destination:    db.DestDB,
		SourcePort:     db.Port,
		BackupMode:     "template",
		CloneMode:      "FILE_COPY",
		Started:        true,
		ConnectCommand: fmt.Sprintf("psql -p %d -d %s", db.Port, db.DestDB),
		CleanupCommand: fmt.Sprintf("dropdb -p %d %s", db.Port, db.DestDB),
		Duration:       elapsed.Seconds(),
		Preflight:      &preflight,
		Warnings:       warnings,
	}
	if db.Owner != "" {
		data.OwnerRequested = db.Owner
		data.OwnerChanged = db.OwnerChanged
		data.OwnerWarning = db.OwnerWarning
	}
	return data
}

func printDatabasePreflight(preflight DatabasePreflight, warnings []string) {
	utils.PrintSection("Database Clone Preflight")
	fmt.Fprintf(os.Stderr, "PG Version:        %d\n", preflight.ServerVersion)
	fmt.Fprintf(os.Stderr, "file_copy_method:  %s\n", valueOrUnknown(preflight.FileCopyMethod))
	if preflight.FileCopyMethodError != "" {
		fmt.Fprintf(os.Stderr, "file_copy_error:   %s\n", preflight.FileCopyMethodError)
	}
	fmt.Fprintf(os.Stderr, "strategy:          %s\n", preflight.Strategy)
	fmt.Fprintf(os.Stderr, "data_directory:    %s\n", valueOrUnknown(preflight.DataDirectory))
	fmt.Fprintf(os.Stderr, "filesystem:        %s\n", valueOrUnknown(preflight.FileSystem))
	fmt.Fprintf(os.Stderr, "copy:              %s\n", valueOrUnknown(string(preflight.CloneMode)))
	for _, warning := range warnings {
		utils.PrintWarn("%s", warning)
	}
}

func printDatabaseResult(data ResultData) {
	fmt.Fprintf(os.Stderr, "%sDatabase cloned:%s %s\n", utils.ColorGreen, utils.ColorReset, data.Destination)
	fmt.Fprintf(os.Stderr, "%sConnect:%s %s\n", utils.ColorCyan, utils.ColorReset, data.ConnectCommand)
	if data.OwnerRequested != "" {
		if data.OwnerChanged {
			fmt.Fprintf(os.Stderr, "%sOwner:%s changed to %s\n", utils.ColorCyan, utils.ColorReset, data.OwnerRequested)
		} else {
			utils.PrintWarn("owner was not changed to %s: %s", data.OwnerRequested, data.OwnerWarning)
		}
	}
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
