package postgres

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pig/cli/ext"
	"pig/internal/output"
	"pig/internal/utils"
)

type DatabaseCloneMode string

const (
	DatabaseCloneModeUnknown DatabaseCloneMode = "unknown"
	DatabaseCloneModeCOW     DatabaseCloneMode = "cow"
	DatabaseCloneModeCopy    DatabaseCloneMode = "copy"
)

const (
	maxPostgresIdentifierBytes = 63
	minPostgresCloneVersion    = 140000
	fileCopyStrategyVersion    = 150000
	fileCopyMethodCloneVersion = 180000
	cloneStrategyDefault       = "DEFAULT"
	cloneStrategyFileCopy      = "FILE_COPY"
)

type CloneOptions struct {
	DbSU         string
	Plan         bool
	Yes          bool
	SourceDB     string
	DestDB       string
	Owner        string
	ConnDB       string
	Port         int
	ConnLimit    int
	ConnLimitSet bool
	Preflight    ClonePreflight
	Warnings     []string
	OwnerChanged bool
	OwnerWarning string
}

type ClonePreflight struct {
	ServerVersion       int               `json:"server_version" yaml:"server_version"`
	FileCopyMethod      string            `json:"file_copy_method,omitempty" yaml:"file_copy_method,omitempty"`
	FileCopyMethodError string            `json:"file_copy_method_error,omitempty" yaml:"file_copy_method_error,omitempty"`
	DataDirectory       string            `json:"data_directory,omitempty" yaml:"data_directory,omitempty"`
	FileSystem          string            `json:"file_system,omitempty" yaml:"file_system,omitempty"`
	CloneMode           DatabaseCloneMode `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Strategy            string            `json:"strategy" yaml:"strategy"`
	FileSystemError     string            `json:"file_system_error,omitempty" yaml:"file_system_error,omitempty"`
}

func (p ClonePreflight) Warnings() []string {
	warnings := []string{}
	if p.ServerVersion == 0 {
		warnings = append(warnings, "PostgreSQL version could not be verified")
	}
	if p.ServerVersion > 0 && p.ServerVersion < fileCopyStrategyVersion {
		warnings = append(warnings, fmt.Sprintf("PostgreSQL 15+ supports STRATEGY FILE_COPY; current server_version_num=%d will use default template copy", p.ServerVersion))
	}
	if p.ServerVersion > 0 && p.ServerVersion < fileCopyMethodCloneVersion {
		warnings = append(warnings, fmt.Sprintf("PostgreSQL 18+ is required for file_copy_method=clone / CoW database clone; current server_version_num=%d will use regular database copy", p.ServerVersion))
		return warnings
	}
	if !strings.EqualFold(p.FileCopyMethod, "clone") {
		if p.FileCopyMethod == "" {
			if p.FileCopyMethodError != "" {
				warnings = append(warnings, fmt.Sprintf("file_copy_method=clone could not be verified: %s", p.FileCopyMethodError))
			} else {
				warnings = append(warnings, "file_copy_method=clone could not be verified")
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("file_copy_method=clone is recommended, current value is %s", p.FileCopyMethod))
		}
	}
	if p.CloneMode != DatabaseCloneModeCOW {
		if p.FileSystemError != "" {
			warnings = append(warnings, fmt.Sprintf("CoW clone support could not be verified for data_directory %s: %s", p.DataDirectory, p.FileSystemError))
		} else {
			warnings = append(warnings, fmt.Sprintf("CoW clone is not confirmed for data_directory %s on filesystem %s", p.DataDirectory, p.FileSystem))
		}
	}
	return warnings
}

type CloneResult struct {
	Kind           string          `json:"kind" yaml:"kind"`
	Source         string          `json:"source" yaml:"source"`
	Destination    string          `json:"destination" yaml:"destination"`
	SourcePort     int             `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	BackupMode     string          `json:"backup_mode,omitempty" yaml:"backup_mode,omitempty"`
	CloneMode      string          `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Started        bool            `json:"started" yaml:"started"`
	ConnectCommand string          `json:"connect_command,omitempty" yaml:"connect_command,omitempty"`
	CleanupCommand string          `json:"cleanup_command,omitempty" yaml:"cleanup_command,omitempty"`
	Duration       float64         `json:"duration_seconds" yaml:"duration_seconds"`
	Preflight      *ClonePreflight `json:"preflight,omitempty" yaml:"preflight,omitempty"`
	Warnings       []string        `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	OwnerRequested string          `json:"owner_requested,omitempty" yaml:"owner_requested,omitempty"`
	OwnerChanged   bool            `json:"owner_changed,omitempty" yaml:"owner_changed,omitempty"`
	OwnerWarning   string          `json:"owner_warning,omitempty" yaml:"owner_warning,omitempty"`
}

type CloneError struct {
	Code int
	Err  error
}

func (e *CloneError) Error() string {
	if e == nil || e.Err == nil {
		return "database clone error"
	}
	return e.Err.Error()
}

func (e *CloneError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NormalizeCloneOptions(opts *CloneOptions) (*CloneOptions, error) {
	if opts == nil {
		return nil, fmt.Errorf("clone options are required")
	}
	n := *opts
	n.DbSU = utils.GetDBSU(n.DbSU)
	n.SourceDB = strings.TrimSpace(n.SourceDB)
	n.DestDB = strings.TrimSpace(n.DestDB)
	n.Owner = strings.TrimSpace(n.Owner)
	n.ConnDB = strings.TrimSpace(n.ConnDB)
	if n.SourceDB == "" {
		return nil, fmt.Errorf("source database is required")
	}
	if strings.EqualFold(n.SourceDB, "template0") || strings.EqualFold(n.SourceDB, "template1") {
		return nil, fmt.Errorf("source database %q is a system template; clone an existing user database instead", n.SourceDB)
	}
	if err := validatePostgresIdentifier("source database", n.SourceDB); err != nil {
		return nil, err
	}
	if n.DestDB != "" {
		if err := validatePostgresIdentifier("destination database", n.DestDB); err != nil {
			return nil, err
		}
	}
	if n.Owner != "" {
		if err := validatePostgresIdentifier("owner", n.Owner); err != nil {
			return nil, err
		}
	}
	if n.ConnDB == "" {
		if strings.EqualFold(n.SourceDB, "postgres") {
			n.ConnDB = "template1"
		} else {
			n.ConnDB = "postgres"
		}
	}
	if err := validatePostgresIdentifier("connection database", n.ConnDB); err != nil {
		return nil, err
	}
	if strings.EqualFold(n.ConnDB, n.SourceDB) {
		return nil, fmt.Errorf("connection database must differ from source database %q", n.SourceDB)
	}
	if n.Port == 0 {
		if port := os.Getenv("PG_PORT"); port != "" {
			if p, err := strconv.Atoi(port); err == nil && p > 0 {
				n.Port = p
			}
		}
	}
	if n.Port == 0 {
		n.Port = 5432
	}
	if !validClonePort(n.Port) {
		return nil, fmt.Errorf("invalid PostgreSQL port %d (must be 1-65535)", n.Port)
	}
	if n.ConnLimitSet && n.ConnLimit < -1 {
		return nil, fmt.Errorf("invalid connection limit %d (must be -1 or greater)", n.ConnLimit)
	}
	return &n, nil
}

func PlanClone(opts *CloneOptions) (*output.Plan, error) {
	n, err := NormalizeCloneOptions(opts)
	if err != nil {
		return nil, &CloneError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	if err := prepareClone(n); err != nil {
		return nil, err
	}
	return BuildClonePlan(n), nil
}

func ExecuteClone(opts *CloneOptions) error {
	n, err := NormalizeCloneOptions(opts)
	if err != nil {
		return exitCloneError(output.CodeForkInvalidArgs, err)
	}
	if err := prepareClone(n); err != nil {
		return cloneExitError(err)
	}
	printClonePreflight(n.Preflight, n.Warnings)
	if !n.Yes {
		if err := confirmCloneWarnings(n.Warnings, "CLONE", 10); err != nil {
			return exitCloneError(output.CodeForkInvalidArgs, err)
		}
	}
	data, err := executePreparedClone(n)
	if err != nil {
		return cloneExitError(err)
	}
	printCloneResult(data)
	return nil
}

func ExecuteCloneResult(opts *CloneOptions) *output.Result {
	n, err := NormalizeCloneOptions(opts)
	if err != nil {
		return output.Fail(output.CodeForkInvalidArgs, err.Error())
	}
	if err := prepareClone(n); err != nil {
		if cloneErr, ok := err.(*CloneError); ok {
			return output.Fail(cloneErr.Code, cloneErr.Error())
		}
		return output.Fail(output.CodeForkPrecheckFailed, err.Error())
	}
	data, err := executePreparedClone(n)
	if err != nil {
		if cloneErr, ok := err.(*CloneError); ok {
			return output.Fail(cloneErr.Code, cloneErr.Error())
		}
		return output.Fail(output.CodeForkPrecheckFailed, err.Error())
	}
	return output.OK("database clone completed", data)
}

func BuildClonePlan(opts *CloneOptions) *output.Plan {
	if opts == nil {
		return &output.Plan{Command: "pig pg clone"}
	}
	strategy := cloneStrategyForVersion(opts.Preflight.ServerVersion)
	actions := []output.Action{
		{Step: 1, Description: fmt.Sprintf("Terminate existing connections to %s", opts.SourceDB)},
		{Step: 2, Description: fmt.Sprintf("Create database %s from template %s", opts.DestDB, opts.SourceDB)},
	}
	if opts.Owner != "" {
		actions = append(actions, output.Action{
			Step:        3,
			Description: fmt.Sprintf("Best-effort alter database %s owner to %s", opts.DestDB, opts.Owner),
		})
	}

	risks := []string{
		"CREATE DATABASE from template requires no active connections on the source database",
		"Applications with persistent reconnect may cause clone to fail; consider a maintenance window",
		"Active source database sessions will be terminated",
	}
	if opts.Preflight.CloneMode == DatabaseCloneModeCopy {
		risks = append(risks, "Database copy may fall back to regular file copy if clone support is unavailable")
	}
	risks = append(risks, opts.Warnings...)

	return &output.Plan{
		Command: BuildCloneCommand(opts),
		Actions: actions,
		Affects: []output.Resource{
			{Type: "database", Name: opts.SourceDB, Impact: "read", Detail: fmt.Sprintf("port %d", opts.Port)},
			{Type: "database", Name: opts.DestDB, Impact: "create", Detail: strategy},
		},
		Expected: fmt.Sprintf("Database %s cloned from %s using %s", opts.DestDB, opts.SourceDB, strategy),
		Risks:    risks,
	}
}

func BuildCloneCommand(opts *CloneOptions) string {
	if opts == nil {
		return "pig pg clone"
	}
	args := []string{"pig", "pg", "clone", opts.SourceDB}
	if opts.DestDB != "" {
		args = append(args, opts.DestDB)
	}
	if opts.Port != 0 && opts.Port != 5432 {
		args = append(args, "-p", fmt.Sprintf("%d", opts.Port))
	}
	if opts.ConnDB != "" && opts.ConnDB != "postgres" {
		args = append(args, "--conn-db", opts.ConnDB)
	}
	if opts.Owner != "" {
		args = append(args, "--owner", opts.Owner)
	}
	if opts.ConnLimitSet {
		args = append(args, "--conn-limit", fmt.Sprintf("%d", opts.ConnLimit))
	}
	if opts.Yes {
		args = append(args, "-y")
	}
	if opts.Plan {
		args = append(args, "--plan")
	}
	return utils.ShellQuoteArgs(args)
}

func BuildDatabaseCloneSQL(opts *CloneOptions) string {
	if opts == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\\set ON_ERROR_STOP on\n")
	sb.WriteString("SELECT pg_terminate_backend(pid)\n")
	sb.WriteString("  FROM pg_stat_activity\n")
	sb.WriteString(" WHERE datname = '")
	sb.WriteString(EscapeDatabaseString(opts.SourceDB))
	sb.WriteString("'\n")
	sb.WriteString("   AND pid <> pg_backend_pid();\n")
	sb.WriteString("CREATE DATABASE ")
	sb.WriteString(QuoteIdentifier(opts.DestDB))
	sb.WriteString(" WITH TEMPLATE ")
	sb.WriteString(QuoteIdentifier(opts.SourceDB))
	if cloneSQLUsesStrategy(opts.Preflight.ServerVersion) {
		sb.WriteString(" STRATEGY FILE_COPY")
	}
	if opts.ConnLimitSet {
		sb.WriteString(" CONNECTION LIMIT ")
		sb.WriteString(fmt.Sprintf("%d", opts.ConnLimit))
	}
	sb.WriteString(";\n")
	return sb.String()
}

func BuildDatabaseAlterOwnerSQL(destDB, owner string) string {
	if strings.TrimSpace(owner) == "" {
		return ""
	}
	return fmt.Sprintf("\\set ON_ERROR_STOP on\nALTER DATABASE %s OWNER TO %s;\n", QuoteIdentifier(destDB), QuoteIdentifier(owner))
}

func NextDatabaseCloneName(source string, existing map[string]bool) string {
	for i := 1; ; i++ {
		suffix := fmt.Sprintf("_%d", i)
		candidate := trimIdentifierToBytes(source, maxPostgresIdentifierBytes-len(suffix)) + suffix
		if !existing[candidate] {
			return candidate
		}
	}
}

func validatePostgresIdentifier(kind, value string) error {
	if len(value) > maxPostgresIdentifierBytes {
		return fmt.Errorf("%s %q is %d bytes; PostgreSQL identifiers are limited to %d bytes", kind, value, len(value), maxPostgresIdentifierBytes)
	}
	return nil
}

func trimIdentifierToBytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := 0
	for i := range value {
		if i > maxBytes {
			break
		}
		end = i
	}
	return value[:end]
}

func cloneSQLUsesStrategy(serverVersion int) bool {
	return serverVersion == 0 || serverVersion >= fileCopyStrategyVersion
}

func cloneStrategyForVersion(serverVersion int) string {
	if serverVersion > 0 && serverVersion < fileCopyStrategyVersion {
		return cloneStrategyDefault
	}
	return cloneStrategyFileCopy
}

func QuoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func EscapeDatabaseString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func executePreparedClone(opts *CloneOptions) (CloneResult, error) {
	start := time.Now()
	if err := executeCloneSQL(opts); err != nil {
		return CloneResult{}, err
	}
	return cloneResult(opts, time.Since(start)), nil
}

func executeCloneSQL(opts *CloneOptions) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return &CloneError{Code: output.CodeForkDependencyMissing, Err: fmt.Errorf("postgresql not found: %w", err)}
	}
	sql := BuildDatabaseCloneSQL(opts)
	args := clonePsqlFileArgs(pg.Psql(), opts.Port, opts.ConnDB)
	printClonePsqlFileHint(args, "<clone-sql>")
	if err := runCloneSQLFile(opts.DbSU, args, sql); err != nil {
		return &CloneError{Code: output.CodeForkDatabaseFailed, Err: err}
	}
	if ownerSQL := BuildDatabaseAlterOwnerSQL(opts.DestDB, opts.Owner); ownerSQL != "" {
		ownerArgs := clonePsqlFileArgs(pg.Psql(), opts.Port, opts.ConnDB)
		printClonePsqlFileHint(ownerArgs, "<owner-sql>")
		if err := runCloneSQLFile(opts.DbSU, ownerArgs, ownerSQL); err != nil {
			opts.OwnerChanged = false
			opts.OwnerWarning = err.Error()
		} else {
			opts.OwnerChanged = true
		}
	}
	return nil
}

func clonePsqlFileArgs(psql string, port int, connDB string) []string {
	return []string{psql, "-X", "-p", fmt.Sprintf("%d", port), "-d", connDB, "-f"}
}

func clonePsqlFileHint(args []string, placeholder string) string {
	if placeholder == "" {
		return utils.ShellQuoteArgs(args)
	}
	return utils.ShellQuoteArgs(args) + " " + placeholder
}

func printClonePsqlFileHint(args []string, placeholder string) {
	fmt.Fprintf(os.Stderr, "%s$ %s%s\n", utils.ColorBlue, clonePsqlFileHint(args, placeholder), utils.ColorReset)
}

type databaseInfo struct {
	Name string
}

func prepareClone(opts *CloneOptions) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return &CloneError{Code: output.CodeForkDependencyMissing, Err: fmt.Errorf("postgresql not found: %w", err)}
	}
	preflight := ClonePreflight{Strategy: cloneStrategyFileCopy, CloneMode: DatabaseCloneModeCopy}
	versionText, err := runPsqlQuery(opts.DbSU, pg.Psql(), opts.Port, opts.ConnDB, "SELECT current_setting('server_version_num')")
	if err != nil {
		return &CloneError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	version, err := strconv.Atoi(strings.TrimSpace(versionText))
	if err != nil {
		return &CloneError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("invalid server_version_num %q: %w", strings.TrimSpace(versionText), err)}
	}
	if version < minPostgresCloneVersion {
		return &CloneError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("PostgreSQL 14+ is required for database clone, current server_version_num=%d", version)}
	}
	preflight.ServerVersion = version
	preflight.Strategy = cloneStrategyForVersion(version)
	if dataDirectory, err := runPsqlQuery(opts.DbSU, pg.Psql(), opts.Port, opts.ConnDB, "SHOW data_directory"); err == nil {
		preflight.DataDirectory = strings.TrimSpace(dataDirectory)
	} else {
		preflight.FileSystemError = err.Error()
	}
	if version >= fileCopyMethodCloneVersion {
		if fileCopyMethod, err := runPsqlQuery(opts.DbSU, pg.Psql(), opts.Port, opts.ConnDB, "SHOW file_copy_method"); err == nil {
			preflight.FileCopyMethod = strings.TrimSpace(fileCopyMethod)
		} else {
			preflight.FileCopyMethodError = err.Error()
		}
		if preflight.DataDirectory != "" {
			cloneMode, fs, fsErr := detectDatabaseCloneMode(preflight.DataDirectory)
			preflight.CloneMode = cloneMode
			preflight.FileSystem = fs
			if fsErr != nil {
				preflight.FileSystemError = fsErr.Error()
			}
		}
	}
	opts.Preflight = preflight

	databases, err := listDatabases(opts.DbSU, pg.Psql(), opts.Port, opts.ConnDB)
	if err != nil {
		return &CloneError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	existing := make(map[string]bool, len(databases))
	sourceFound := false
	for _, db := range databases {
		existing[db.Name] = true
		if db.Name == opts.SourceDB {
			sourceFound = true
		}
	}
	if !sourceFound {
		return &CloneError{Code: output.CodeForkSourceNotFound, Err: fmt.Errorf("source database does not exist: %s", opts.SourceDB)}
	}
	if opts.DestDB == "" {
		opts.DestDB = NextDatabaseCloneName(opts.SourceDB, existing)
		if err := validatePostgresIdentifier("destination database", opts.DestDB); err != nil {
			return &CloneError{Code: output.CodeForkInvalidArgs, Err: err}
		}
	} else if existing[opts.DestDB] {
		return &CloneError{Code: output.CodeForkDestExists, Err: fmt.Errorf("destination database already exists: %s", opts.DestDB)}
	}
	opts.Warnings = opts.Preflight.Warnings()
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
		databases = append(databases, databaseInfo{Name: name})
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
	out, err := utils.DBSUCommandOutput(dbsu, args)
	if err != nil {
		if msg := strings.TrimSpace(out); msg != "" {
			return out, fmt.Errorf("%w: %s", err, msg)
		}
		return out, err
	}
	return out, nil
}

func runCloneSQLFile(dbsu string, args []string, sql string) error {
	file, err := os.CreateTemp("", "pig-clone-*.sql")
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

func confirmCloneWarnings(warnings []string, action string, seconds int) error {
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

func detectDatabaseCloneMode(path string) (DatabaseCloneMode, string, error) {
	mount, fs := dfMountAndFS(path)
	if mount == "" || fs == "" {
		return DatabaseCloneModeCopy, "", fmt.Errorf("unable to detect filesystem for %s", path)
	}
	switch strings.ToLower(fs) {
	case "xfs":
		if xfsReflinkEnabled(mount) {
			return DatabaseCloneModeCOW, fs, nil
		}
		return DatabaseCloneModeCopy, fs, nil
	case "btrfs", "bcachefs", "ocfs2", "apfs":
		return DatabaseCloneModeCOW, fs, nil
	default:
		return DatabaseCloneModeCopy, fs, nil
	}
}

func cloneResult(opts *CloneOptions, elapsed time.Duration) CloneResult {
	warnings := append([]string{}, opts.Warnings...)
	preflight := opts.Preflight
	data := CloneResult{
		Kind:           "database",
		Source:         opts.SourceDB,
		Destination:    opts.DestDB,
		SourcePort:     opts.Port,
		BackupMode:     "template",
		CloneMode:      cloneStrategyForVersion(preflight.ServerVersion),
		Started:        true,
		ConnectCommand: utils.ShellQuoteArgs([]string{"psql", "-p", fmt.Sprintf("%d", opts.Port), "-d", opts.DestDB}),
		CleanupCommand: utils.ShellQuoteArgs([]string{"dropdb", "-p", fmt.Sprintf("%d", opts.Port), opts.DestDB}),
		Duration:       elapsed.Seconds(),
		Preflight:      &preflight,
		Warnings:       warnings,
	}
	if opts.Owner != "" {
		data.OwnerRequested = opts.Owner
		data.OwnerChanged = opts.OwnerChanged
		data.OwnerWarning = opts.OwnerWarning
	}
	return data
}

func printClonePreflight(preflight ClonePreflight, warnings []string) {
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

func printCloneResult(data CloneResult) {
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

func validClonePort(port int) bool {
	return port >= 1 && port <= 65535
}

func exitCloneError(code int, err error) error {
	cloneErr := &CloneError{Code: code, Err: err}
	return &utils.ExitCodeError{Code: output.ExitCode(code), Err: cloneErr}
}

func cloneExitError(err error) error {
	if err == nil {
		return nil
	}
	if cloneErr, ok := err.(*CloneError); ok {
		return &utils.ExitCodeError{Code: output.ExitCode(cloneErr.Code), Err: cloneErr}
	}
	return err
}
