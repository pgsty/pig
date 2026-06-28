package postgres

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

func TestNormalizeInstanceDefaults(t *testing.T) {
	opts := &Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name: "dev",
		},
	}

	n, err := NormalizeOptions(opts)
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	if n.Instance.SourceData != "/pg/data" {
		t.Errorf("SourceData = %q, want /pg/data", n.Instance.SourceData)
	}
	if n.Instance.SourcePort != 5432 {
		t.Errorf("SourcePort = %d, want 5432", n.Instance.SourcePort)
	}
	if n.Instance.DestData != "/pg/data-dev" {
		t.Errorf("DestData = %q, want /pg/data-dev", n.Instance.DestData)
	}
	if n.Instance.DestPort != 15432 {
		t.Errorf("DestPort = %d, want 15432", n.Instance.DestPort)
	}
	if n.DbSU != "postgres" {
		t.Errorf("DbSU = %q, want postgres", n.DbSU)
	}
	if n.Mode != ModeAuto {
		t.Errorf("Mode = %q, want %q", n.Mode, ModeAuto)
	}
	if n.Start {
		t.Error("Start should default to false")
	}
	if !n.Instance.Managed {
		t.Error("default fork should be managed")
	}
}

func TestNormalizeRejectsInvalidInstanceName(t *testing.T) {
	_, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name: "bad/name",
		},
	})
	if err == nil {
		t.Fatal("expected invalid fork name error")
	}
	if !strings.Contains(err.Error(), "fork name") {
		t.Fatalf("error should mention fork name, got %v", err)
	}
}

func TestNormalizeNumericInstanceNameUsesDataDashPath(t *testing.T) {
	n, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name: "1",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}
	if n.Instance.DestData != "/pg/data-1" {
		t.Errorf("DestData = %q, want /pg/data-1", n.Instance.DestData)
	}
}

func TestNormalizeInstanceWithExplicitDestinationIsUnmanaged(t *testing.T) {
	n, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name:     "dev",
			DestData: "/tmp/dev-fork",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}
	if n.Instance.Managed {
		t.Fatal("explicit destination should create an unmanaged fork")
	}
	if n.Instance.DestData != "/tmp/dev-fork" {
		t.Fatalf("DestData = %q, want /tmp/dev-fork", n.Instance.DestData)
	}
}

func TestNormalizeInstanceRejectsInvalidPorts(t *testing.T) {
	tests := []struct {
		name string
		inst InstanceOptions
	}{
		{"negative source port", InstanceOptions{Name: "dev", SourcePort: -1}},
		{"overflow source port", InstanceOptions{Name: "dev", SourcePort: 70000}},
		{"negative destination port", InstanceOptions{Name: "dev", DestPort: -1}},
		{"overflow destination port", InstanceOptions{Name: "dev", DestPort: 70000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeOptions(&Options{Kind: KindInstance, Instance: tt.inst})
			if err == nil {
				t.Fatal("expected invalid port error")
			}
			if !strings.Contains(err.Error(), "port") {
				t.Fatalf("error should mention port, got %v", err)
			}
		})
	}
}

func TestValidateForkDataPathsRejectsUnsafeDestinations(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "data")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	link := filepath.Join(root, "data-link")
	if err := os.Symlink(src, link); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	tests := []struct {
		name string
		dst  string
	}{
		{"same directory", src},
		{"source parent", root},
		{"source child", filepath.Join(src, "child")},
		{"symlink to source", link},
		{"root directory", "/"},
		{"pg root directory", "/pg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := validateForkDataPaths(src, tt.dst); err == nil {
				t.Fatalf("validateForkDataPaths(%q, %q) returned nil error", src, tt.dst)
			}
		})
	}
}

func TestValidateForkDataPathsAllowsSiblingForkDirectory(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "data")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	dst := filepath.Join(root, "data-dev")

	normalizedSrc, normalizedDst, err := validateForkDataPaths(src, dst)
	if err != nil {
		t.Fatalf("validateForkDataPaths returned error: %v", err)
	}
	rootExpected, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root symlink: %v", err)
	}
	wantSrc := filepath.Join(rootExpected, "data")
	wantDst := filepath.Join(rootExpected, "data-dev")
	if normalizedSrc != wantSrc {
		t.Fatalf("normalized source = %q, want %q", normalizedSrc, wantSrc)
	}
	if normalizedDst != wantDst {
		t.Fatalf("normalized destination = %q, want %q", normalizedDst, wantDst)
	}
}

func TestPlanRejectsUnsafeInstanceDestination(t *testing.T) {
	_, err := Plan(&Options{
		Kind: KindInstance,
		Plan: true,
		Instance: InstanceOptions{
			Name:     "dev",
			DestData: "/",
		},
	})
	if err == nil {
		t.Fatal("expected unsafe destination error")
	}
	if !strings.Contains(err.Error(), "unsafe destination") {
		t.Fatalf("error should mention unsafe destination, got %v", err)
	}
}

func TestBuildInstancePlan(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name: "dev",
		},
		Plan: true,
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	plan := BuildPlan(opts, &State{
		CloneMode:  CloneModeCOW,
		BackupMode: BackupModeHot,
	})
	if plan == nil {
		t.Fatal("BuildPlan returned nil")
	}
	if plan.Command != "pig pg fork init dev --plan" {
		t.Errorf("Command = %q, want pig pg fork init dev --plan", plan.Command)
	}
	for _, want := range []string{
		"Start PostgreSQL backup mode",
		"Clone data directory with CoW",
		"Prepare forked instance configuration",
	} {
		if !containsForkAction(plan.Actions, want) {
			t.Errorf("plan actions missing %q: %#v", want, plan.Actions)
		}
	}
	if containsForkAction(plan.Actions, "Start forked PostgreSQL instance") {
		t.Errorf("plan should not start by default: %#v", plan.Actions)
	}
	if !containsForkResource(plan.Affects, "instance", "/pg/data-dev") {
		t.Errorf("plan affects should include destination instance /pg/data-dev: %#v", plan.Affects)
	}
	if len(plan.Risks) == 0 {
		t.Error("plan should include risks")
	}
}

func TestBuildInstancePlanWithRunStartsFork(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Run:  true,
		Instance: InstanceOptions{
			Name: "dev",
		},
		Plan: true,
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	plan := BuildPlan(opts, &State{CloneMode: CloneModeCOW, BackupMode: BackupModeHot})
	if plan.Command != "pig pg fork init dev -r --plan" {
		t.Errorf("Command = %q, want pig pg fork init dev -r --plan", plan.Command)
	}
	if !containsForkAction(plan.Actions, "Start forked PostgreSQL instance") {
		t.Errorf("plan actions missing start step: %#v", plan.Actions)
	}
}

func TestBuildCommandUsesUppercaseSourceAndLowercaseDestinationFlags(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Plan: true,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: "/pg/data2",
			SourcePort: 15431,
			DestData:   "/tmp/dev-fork",
			DestPort:   15433,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	want := "pig pg fork init dev -D /pg/data2 -P 15431 -d /tmp/dev-fork -p 15433 --plan"
	if got := BuildCommand(opts); got != want {
		t.Fatalf("BuildCommand() = %q, want %q", got, want)
	}
}

func TestFirstFreePortAvoidsReservedForkPorts(t *testing.T) {
	original := forkPortFree
	t.Cleanup(func() {
		forkPortFree = original
	})
	forkPortFree = func(port int) bool { return true }

	got := firstFreePortAvoiding(15432, map[int]bool{
		15432: true,
		15433: true,
	})
	if got != 15434 {
		t.Fatalf("firstFreePortAvoiding() = %d, want 15434", got)
	}
}

func TestReservedForkPortsExcludesCurrentDataDir(t *testing.T) {
	forks := []ForkInfo{
		{Name: "dev", Target: ForkEndpoint{Data: "/pg/data-dev", Port: 15432}},
		{Name: "test", Target: ForkEndpoint{Data: "/pg/data-test", Port: 15433}},
		{Name: "old", Target: ForkEndpoint{Data: "/pg/data-old"}},
	}

	reserved := reservedForkPorts(forks, "/pg/data-dev")
	if reserved[15432] {
		t.Fatal("reservedForkPorts should ignore the current fork data directory")
	}
	if !reserved[15433] {
		t.Fatal("reservedForkPorts should include other managed fork ports")
	}
	if reserved[0] {
		t.Fatal("reservedForkPorts should ignore empty ports")
	}
}

func TestForkStartOptionsUsesForkLogFile(t *testing.T) {
	opts := forkStartOptions(InstanceOptions{
		DestData: "/pg/data-dev",
		Timeout:  42,
	})
	if opts.Timeout != 42 {
		t.Fatalf("Timeout = %d, want 42", opts.Timeout)
	}
	if opts.LogFile != "/pg/data-dev/log/fork.log" {
		t.Fatalf("LogFile = %q, want fork log under destination log directory", opts.LogFile)
	}
}

func TestForkPsqlProbeArgsDisablePsqlrc(t *testing.T) {
	args := forkPsqlProbeArgs("/usr/bin/psql", 15432)
	if !containsForkArg(args, "-X") {
		t.Fatalf("forkPsqlProbeArgs() should include -X to disable .psqlrc: %#v", args)
	}
	if got := strings.Join(args, " "); !strings.Contains(got, "-p 15432") || !strings.Contains(got, "SELECT 1") {
		t.Fatalf("forkPsqlProbeArgs() = %#v", args)
	}
}

func TestBackupFunctionNamesFollowServerVersion(t *testing.T) {
	tests := []struct {
		version int
		start   string
		stop    string
	}{
		{140000, "pg_start_backup", "pg_stop_backup"},
		{150000, "pg_backup_start", "pg_backup_stop"},
		{180000, "pg_backup_start", "pg_backup_stop"},
	}

	for _, tt := range tests {
		t.Run(tt.start, func(t *testing.T) {
			names := backupFunctionNames(tt.version)
			if names.start != tt.start || names.stop != tt.stop {
				t.Fatalf("backupFunctionNames(%d) = %#v, want start=%s stop=%s", tt.version, names, tt.start, tt.stop)
			}
		})
	}
}

func TestBackupServerVersionTerminatesInteractiveSQL(t *testing.T) {
	session := &fakeBackupSession{
		outputs: []string{"180000"},
	}

	if _, err := backupServerVersion(session); err != nil {
		t.Fatalf("backupServerVersion returned error: %v", err)
	}
	if len(session.queries) != 1 {
		t.Fatalf("backupServerVersion queries = %#v", session.queries)
	}
	if !strings.HasSuffix(strings.TrimSpace(session.queries[0]), ";") {
		t.Fatalf("backupServerVersion query must end with semicolon for interactive psql, got %q", session.queries[0])
	}
}

func TestBuildBackupSQLDoesNotEmbedShellCopy(t *testing.T) {
	names := backupFunctionNames(180000)
	for _, sql := range []string{buildBackupStartSQL("pig_fork_dev", names), buildBackupStopSQL(names)} {
		if strings.Contains(sql, `\!`) || strings.Contains(sql, "cp -a") || strings.Contains(sql, "rm -rf") {
			t.Fatalf("backup SQL should not embed shell copy commands:\n%s", sql)
		}
	}
}

func TestHotBackupRunsCopyBetweenStartAndStopOnSameSession(t *testing.T) {
	var events []string
	session := &fakeBackupSession{
		outputs: []string{"180000", ""},
		onExec: func(sql string) {
			switch {
			case strings.Contains(sql, "server_version_num"):
				events = append(events, "version")
			case strings.Contains(sql, "pg_backup_start"):
				events = append(events, "start")
			case strings.Contains(sql, "pg_backup_stop"):
				events = append(events, "stop")
			}
		},
	}
	copyFn := func() error {
		events = append(events, "copy")
		return nil
	}

	if err := runHotBackupCopy(session, "label", copyFn); err != nil {
		t.Fatalf("runHotBackupCopy returned error: %v", err)
	}
	want := strings.Join([]string{"version", "start", "copy", "stop"}, ",")
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if session.closed {
		t.Fatal("runHotBackupCopy should not close caller-owned session")
	}
}

func TestHotBackupStopsBackupWhenCopyFails(t *testing.T) {
	var events []string
	session := &fakeBackupSession{
		outputs: []string{"180000", ""},
		onExec: func(sql string) {
			if strings.Contains(sql, "pg_backup_start") {
				events = append(events, "start")
			}
			if strings.Contains(sql, "pg_backup_stop") {
				events = append(events, "stop")
			}
		},
	}
	copyFn := func() error {
		events = append(events, "copy")
		return os.ErrPermission
	}

	err := runHotBackupCopy(session, "label", copyFn)
	if err == nil {
		t.Fatal("expected copy error")
	}
	want := strings.Join([]string{"start", "copy", "stop"}, ",")
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestRequireCOWRejectsRegularCopyUnlessForced(t *testing.T) {
	state := &State{CloneMode: CloneModeCopy, FS: "ext4"}
	if err := requireCOW(state, false); err == nil {
		t.Fatal("expected non-CoW error without force")
	}
	if err := requireCOW(state, true); err != nil {
		t.Fatalf("force should allow regular copy fallback: %v", err)
	}
}

type fakeBackupSession struct {
	outputs []string
	onExec  func(string)
	closed  bool
	queries []string
}

func (s *fakeBackupSession) Exec(sql string) (string, error) {
	s.queries = append(s.queries, sql)
	if s.onExec != nil {
		s.onExec(sql)
	}
	if len(s.outputs) == 0 {
		return "", nil
	}
	output := s.outputs[0]
	s.outputs = s.outputs[1:]
	return output, nil
}

func (s *fakeBackupSession) Close() error {
	s.closed = true
	return nil
}

func TestBuildForkInfoIncludesKeyFields(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Run:  true,
		Instance: InstanceOptions{
			Name: "dev",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	info := BuildForkInfo(opts, &State{BackupMode: BackupModeHot, CloneMode: CloneModeCOW, FS: "xfs", Started: true})
	if info.Name != "dev" {
		t.Errorf("Name = %q, want dev", info.Name)
	}
	if info.Target.Data != "/pg/data-dev" {
		t.Errorf("Target.Data = %q, want /pg/data-dev", info.Target.Data)
	}
	if info.Target.Port != 15432 {
		t.Errorf("Target.Port = %d, want 15432", info.Target.Port)
	}
	if info.Copy.Actual != "cow" {
		t.Errorf("Copy.Actual = %q, want cow", info.Copy.Actual)
	}
	if !info.Managed {
		t.Error("BuildForkInfo should mark default forks as managed")
	}
}

func TestBuildForkInfoMarksExplicitDestinationAsUnmanaged(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name:     "dev",
			DestData: "/tmp/dev-fork",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}
	info := BuildForkInfo(opts, &State{BackupMode: BackupModeCold, CloneMode: CloneModeCOW})
	if info.Managed {
		t.Fatal("explicit destination fork should be recorded as unmanaged")
	}
}

func TestScanForksReadsForkInfoAndOrphans(t *testing.T) {
	root := t.TempDir()
	writeForkInfoForTest(t, root, "data-dev", `{"kind":"pg_fork","version":1,"name":"dev","target":{"data":"/pg/data-dev","port":15432,"started":true}}`)
	if err := os.Mkdir(filepath.Join(root, "data-old"), 0755); err != nil {
		t.Fatalf("mkdir orphan: %v", err)
	}

	forks, err := ScanForks(root)
	if err != nil {
		t.Fatalf("ScanForks returned error: %v", err)
	}
	if len(forks) != 2 {
		t.Fatalf("len(forks) = %d, want 2: %#v", len(forks), forks)
	}
	if forks[0].Name != "dev" || forks[0].Target.Port != 15432 {
		t.Fatalf("first fork = %#v, want dev with port 15432", forks[0])
	}
	if forks[1].Name != "old" || !forks[1].Orphan {
		t.Fatalf("second fork = %#v, want orphan old", forks[1])
	}
}

func TestScanForksAsFollowsSymlinkRoot(t *testing.T) {
	original := forkDBSUCommandOutput
	t.Cleanup(func() {
		forkDBSUCommandOutput = original
	})
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if len(args) < 3 || args[0] != "find" || args[1] != "-H" || args[2] != "/pg" {
			t.Fatalf("ScanForksAs should use find -H for symlink roots, got %#v", args)
		}
		return "", nil
	}
	if _, err := ScanForksAs("postgres", "/pg"); err != nil {
		t.Fatalf("ScanForksAs returned error: %v", err)
	}
}

func TestManagedForkDataDirUsesNameRule(t *testing.T) {
	dataDir, err := ManagedForkDataDir("dev")
	if err != nil {
		t.Fatalf("ManagedForkDataDir returned error: %v", err)
	}
	if dataDir != "/pg/data-dev" {
		t.Fatalf("dataDir = %q, want /pg/data-dev", dataDir)
	}
}

func TestResolveForkTargetRequiresNameOrDestination(t *testing.T) {
	if _, err := ResolveForkTarget(ForkTargetOptions{}); err == nil {
		t.Fatal("expected missing target error")
	}
	if _, err := ResolveForkTarget(ForkTargetOptions{Name: "dev", DestData: "/tmp/dev-fork"}); err == nil {
		t.Fatal("expected ambiguous target error")
	}
}

func TestResolveForkTargetAllowsUnmanagedDestination(t *testing.T) {
	root := t.TempDir()
	target, err := ResolveForkTarget(ForkTargetOptions{DestData: filepath.Join(root, "dev fork")})
	if err != nil {
		t.Fatalf("ResolveForkTarget returned error: %v", err)
	}
	if target != filepath.Join(root, "dev fork") {
		t.Fatalf("target = %q, want explicit destination", target)
	}
}

func TestRemoveForkRefusesRunningForkWithoutStopForce(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	dataDir := filepath.Join(root, "data-dev")
	if err := os.Mkdir(dataDir, 0755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "fork.json"), []byte(`{"kind":"pg_fork","version":1,"name":"dev","target":{"data":"`+dataDir+`","port":15432}}`), 0644); err != nil {
		t.Fatalf("write fork.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "postmaster.pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}

	_, err := RemoveFork(ForkTargetOptions{DbSU: dbsu, DestData: dataDir, Force: true, Yes: true})
	if err == nil {
		t.Fatal("expected running fork removal to be refused")
	}
	if !strings.Contains(err.Error(), "--stop -f") {
		t.Fatalf("error should mention explicit --stop -f, got %v", err)
	}
}

func TestRemoveForkAllowsForcedManagedOrphan(t *testing.T) {
	originalCommand := forkDBSUCommand
	originalRead := forkReadFileAsDBSU
	originalCheckDataDir := forkCheckDataDir
	originalRunning := forkCheckPostgresRunning
	t.Cleanup(func() {
		forkDBSUCommand = originalCommand
		forkReadFileAsDBSU = originalRead
		forkCheckDataDir = originalCheckDataDir
		forkCheckPostgresRunning = originalRunning
	})
	forkCheckDataDir = func(dbsu, dataDir string) (bool, bool) {
		if dataDir != "/pg/data-dev" {
			t.Fatalf("data dir check = %q", dataDir)
		}
		return true, false
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return "", os.ErrNotExist
	}
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		return false, 0
	}
	commands := []string{}
	forkDBSUCommand = func(dbsu string, args []string) error {
		commands = append(commands, strings.Join(args, " "))
		return nil
	}

	if _, err := RemoveFork(ForkTargetOptions{Name: "dev", Force: true, Yes: true}); err != nil {
		t.Fatalf("RemoveFork returned error: %v", err)
	}
	if !containsForkArg(commands, "rm -rf -- /pg/data-dev") {
		t.Fatalf("remove command not executed: %#v", commands)
	}
}

func TestPrecheckInstanceRefusesReplacingRunningDestination(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	source := filepath.Join(root, "data")
	dest := filepath.Join(root, "data-dev")
	for _, dir := range []string{source, dest} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
			t.Fatalf("write PG_VERSION: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dest, "postmaster.pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		t.Fatalf("write destination postmaster.pid: %v", err)
	}

	_, err := precheckInstance(&Options{
		Kind:    KindInstance,
		Mode:    ModeCold,
		DbSU:    dbsu,
		Replace: true,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: source,
			DestData:   dest,
			SourcePort: 5432,
			DestPort:   15432,
		},
	})
	if err == nil {
		t.Fatal("expected replacing a running destination to be refused")
	}
	if !strings.Contains(err.Error(), "running") {
		t.Fatalf("error should mention running destination, got %v", err)
	}
}

func TestPostmasterPIDMatchesDataDirRejectsCopiedSourcePID(t *testing.T) {
	pidContent := strconv.Itoa(os.Getpid()) + "\n/pg/data\n1782617444\n5432\n"
	if postmasterPIDMatchesDataDir(pidContent, "/pg/data-dev") {
		t.Fatal("postmaster.pid copied from source data directory should not match fork data directory")
	}
	if !postmasterPIDMatchesDataDir(pidContent, "/pg/data") {
		t.Fatal("postmaster.pid should match its own data directory")
	}
}

func TestDetectCloneModeAsUsesDBSUForFilesystemProbes(t *testing.T) {
	original := forkDBSUCommandOutput
	t.Cleanup(func() {
		forkDBSUCommandOutput = original
	})
	calls := []string{}
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		calls = append(calls, dbsu+" "+strings.Join(args, " "))
		switch args[0] {
		case "test":
			return "", nil
		case "df":
			return "Filesystem Type 1K-blocks Used Available Use% Mounted on\n/dev/sdb btrfs 100 1 99 1% /data\n", nil
		default:
			t.Fatalf("unexpected command: %#v", args)
		}
		return "", nil
	}

	mode, fs := detectCloneModeAs("postgres", "/pg/data", "/pg/data-dev")
	if mode != CloneModeCOW || fs != "btrfs" {
		t.Fatalf("detectCloneModeAs() = (%s, %s), want (cow, btrfs)", mode, fs)
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "postgres ") {
			t.Fatalf("filesystem probe did not use dbsu: %#v", calls)
		}
	}
}

func TestXFSReflinkEnabledFallsBackToUsrSbin(t *testing.T) {
	original := forkXFSInfoOutput
	t.Cleanup(func() {
		forkXFSInfoOutput = original
	})
	calls := []string{}
	forkXFSInfoOutput = func(bin, mount string) ([]byte, error) {
		calls = append(calls, bin+" "+mount)
		if bin == "xfs_info" {
			return nil, exec.ErrNotFound
		}
		if bin == "/usr/sbin/xfs_info" {
			return []byte("meta-data=/data reflink=1\n"), nil
		}
		return nil, exec.ErrNotFound
	}

	if !xfsReflinkEnabled("/data") {
		t.Fatal("xfsReflinkEnabled should use /usr/sbin/xfs_info fallback")
	}
	if strings.Join(calls, ",") != "xfs_info /data,/usr/sbin/xfs_info /data" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestExistingParentAsUsesDBSUTestForCandidates(t *testing.T) {
	original := forkDBSUCommandOutput
	t.Cleanup(func() {
		forkDBSUCommandOutput = original
	})
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if strings.Join(args, " ") != "test -d /pg" {
			t.Fatalf("existingParentAs should probe candidates with test -d, got %#v", args)
		}
		return "", nil
	}

	if got := existingParentAs("postgres", "/pg"); got != "/pg" {
		t.Fatalf("existingParentAs() = %q, want /pg", got)
	}
}

func TestCopyDataDirUsesDirectCommands(t *testing.T) {
	originalCommand := forkDBSUCommand
	originalRunning := forkCheckPostgresRunning
	t.Cleanup(func() {
		forkDBSUCommand = originalCommand
		forkCheckPostgresRunning = originalRunning
	})
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		if dataDir != "/pg/data-dev" {
			t.Fatalf("running check dataDir = %q", dataDir)
		}
		return false, 0
	}
	commands := []string{}
	forkDBSUCommand = func(dbsu string, args []string) error {
		commands = append(commands, strings.Join(args, " "))
		return nil
	}

	if err := copyDataDir("postgres", "/pg/data", "/pg/data-dev"); err != nil {
		t.Fatalf("copyDataDir returned error: %v", err)
	}
	want := []string{
		"rm -rf -- /pg/data-dev",
		"cp -a --reflink=auto /pg/data /pg/data-dev",
		"test -f /pg/data-dev/PG_VERSION",
	}
	if strings.Join(commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestConfigureInstanceUsesDirectCommandsAndRewritesAutoConf(t *testing.T) {
	originalCommand := forkDBSUCommand
	originalRead := forkReadFileAsDBSU
	originalWrite := forkWriteFileAsDBSU
	t.Cleanup(func() {
		forkDBSUCommand = originalCommand
		forkReadFileAsDBSU = originalRead
		forkWriteFileAsDBSU = originalWrite
	})
	commands := []string{}
	forkDBSUCommand = func(dbsu string, args []string) error {
		if args[0] == "sh" {
			t.Fatalf("configureInstance should not use sh -c: %#v", args)
		}
		commands = append(commands, strings.Join(args, " "))
		return nil
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return "shared_buffers = '1GB'\nprimary_conninfo = 'old'\nport = 1111\n", nil
	}
	written := ""
	forkWriteFileAsDBSU = func(path, content, dbsu string) error {
		written = content
		return nil
	}

	if err := configureInstance("postgres", "/pg/data-dev", 15432); err != nil {
		t.Fatalf("configureInstance returned error: %v", err)
	}
	for _, want := range []string{
		"rm -f /pg/data-dev/postmaster.pid /pg/data-dev/postmaster.opts /pg/data-dev/standby.signal /pg/data-dev/recovery.signal",
		"rm -rf /pg/data-dev/pg_replslot",
		"mkdir -p /pg/data-dev/pg_replslot",
		"touch /pg/data-dev/postgresql.auto.conf",
	} {
		if !containsForkArg(commands, want) {
			t.Fatalf("commands missing %q: %#v", want, commands)
		}
	}
	for _, want := range []string{"shared_buffers = '1GB'", "port = 15432", "archive_mode = off", "log_directory = 'log'"} {
		if !strings.Contains(written, want) {
			t.Fatalf("written auto.conf missing %q:\n%s", want, written)
		}
	}
	for _, removed := range []string{"primary_conninfo", "port = 1111"} {
		if strings.Contains(written, removed) {
			t.Fatalf("written auto.conf should remove %q:\n%s", removed, written)
		}
	}
}

func TestForkErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
		exit int
	}{
		{"invalid args", output.CodeForkInvalidArgs, 2},
		{"dependency missing", output.CodeForkDependencyMissing, 4},
		{"destination exists", output.CodeForkDestExists, 6},
		{"port in use", output.CodeForkPortInUse, 9},
		{"copy failed", output.CodeForkCopyFailed, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := output.ExitCode(tt.code); got != tt.exit {
				t.Fatalf("ExitCode(%d) = %d, want %d", tt.code, got, tt.exit)
			}
		})
	}
}

func containsForkAction(actions []output.Action, text string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, text) {
			return true
		}
	}
	return false
}

func containsForkResource(resources []output.Resource, typ, name string) bool {
	for _, resource := range resources {
		if resource.Type == typ && resource.Name == name {
			return true
		}
	}
	return false
}

func containsForkArg(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeForkInfoForTest(t *testing.T, root, dir, content string) {
	t.Helper()
	path := filepath.Join(root, dir)
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(filepath.Join(path, "fork.json"), []byte(content), 0644); err != nil {
		t.Fatalf("write fork.json: %v", err)
	}
}

func withCurrentUserAsDBSU(t *testing.T) string {
	t.Helper()
	original := config.CurrentUser
	current, err := user.Current()
	if err != nil {
		t.Fatalf("detect current user: %v", err)
	}
	config.CurrentUser = current.Username
	t.Cleanup(func() {
		config.CurrentUser = original
	})
	return current.Username
}
