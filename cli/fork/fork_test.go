package fork

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if plan.Command != "pig pg fork dev --plan" {
		t.Errorf("Command = %q, want pig pg fork dev --plan", plan.Command)
	}
	for _, want := range []string{
		"Start PostgreSQL backup mode",
		"Clone data directory with CoW",
		"Prepare forked instance configuration",
	} {
		if !containsAction(plan.Actions, want) {
			t.Errorf("plan actions missing %q: %#v", want, plan.Actions)
		}
	}
	if containsAction(plan.Actions, "Start forked PostgreSQL instance") {
		t.Errorf("plan should not start by default: %#v", plan.Actions)
	}
	if !containsResource(plan.Affects, "instance", "/pg/data-dev") {
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
	if plan.Command != "pig pg fork dev -r --plan" {
		t.Errorf("Command = %q, want pig pg fork dev -r --plan", plan.Command)
	}
	if !containsAction(plan.Actions, "Start forked PostgreSQL instance") {
		t.Errorf("plan actions missing start step: %#v", plan.Actions)
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
}

func (s *fakeBackupSession) Exec(sql string) (string, error) {
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

func TestBuildDatabaseCloneSQL(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&DatabaseOptions{
		SourceDB: "app",
		DestDB:   "app_fork",
	})

	for _, want := range []string{
		"\\set ON_ERROR_STOP on",
		"SELECT pg_terminate_backend(pid)",
		`datname = 'app'`,
		`CREATE DATABASE "app_fork" WITH TEMPLATE "app" STRATEGY FILE_COPY;`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestBuildDatabaseCloneSQLCanSkipConnectionKill(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&DatabaseOptions{
		SourceDB: "app",
		DestDB:   "app_fork",
		NoKill:   true,
	})
	if strings.Contains(sql, "pg_terminate_backend") {
		t.Fatalf("SQL should not terminate connections when NoKill is set:\n%s", sql)
	}
}

func TestQuoteIdentifierEscapesDoubleQuotes(t *testing.T) {
	got := QuoteIdentifier(`a"b`)
	want := `"a""b"`
	if got != want {
		t.Fatalf("QuoteIdentifier() = %q, want %q", got, want)
	}
}

func TestBuildDatabasePlan(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindDatabase,
		Database: DatabaseOptions{
			SourceDB: "app",
			DestDB:   "app_fork",
		},
		Plan: true,
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	plan := BuildPlan(opts, &State{CloneMode: CloneModeCOW})
	if plan.Command != "pig pg clone app app_fork --plan" {
		t.Errorf("Command = %q, want database clone command", plan.Command)
	}
	for _, want := range []string{
		"Terminate existing connections to app",
		"Create database app_fork from template app",
	} {
		if !containsAction(plan.Actions, want) {
			t.Errorf("plan actions missing %q: %#v", want, plan.Actions)
		}
	}
	if !containsResource(plan.Affects, "database", "app_fork") {
		t.Errorf("plan affects should include destination database app_fork: %#v", plan.Affects)
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

func containsAction(actions []output.Action, text string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, text) {
			return true
		}
	}
	return false
}

func containsResource(resources []output.Resource, typ, name string) bool {
	for _, resource := range resources {
		if resource.Type == typ && resource.Name == name {
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
