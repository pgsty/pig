package postgres

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
	if n.Start {
		t.Error("Start should default to false")
	}
	if !n.Instance.Managed {
		t.Error("default fork should be managed")
	}
}

func TestNormalizeAcceptsCanonicalStartAndDeprecatedRun(t *testing.T) {
	for _, opts := range []*Options{
		{Kind: KindInstance, Start: true, Instance: InstanceOptions{Name: "dev"}},
		{Kind: KindInstance, Run: true, Instance: InstanceOptions{Name: "dev"}},
	} {
		n, err := NormalizeOptions(opts)
		if err != nil {
			t.Fatalf("NormalizeOptions returned error: %v", err)
		}
		if !n.Start {
			t.Fatalf("NormalizeOptions should enable Start for %#v", opts)
		}
	}

	n, err := NormalizeOptions(&Options{
		Kind:    KindInstance,
		Start:   true,
		NoStart: true,
		Instance: InstanceOptions{
			Name: "dev",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}
	if n.Start {
		t.Fatal("NoStart should still disable a requested start")
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

func TestBuildInstancePlanWithStartStartsFork(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind:  KindInstance,
		Start: true,
		Instance: InstanceOptions{
			Name: "dev",
		},
		Plan: true,
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}

	plan := BuildPlan(opts, &State{CloneMode: CloneModeCOW, BackupMode: BackupModeHot})
	if plan.Command != "pig pg fork init dev --start --plan" {
		t.Errorf("Command = %q, want pig pg fork init dev --start --plan", plan.Command)
	}
	if !containsForkAction(plan.Actions, "Start forked PostgreSQL instance") {
		t.Errorf("plan actions missing start step: %#v", plan.Actions)
	}
}

func TestPlanRunsReadOnlyPrecheck(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "data-dev")
	_, err := Plan(&Options{
		Kind: KindInstance,
		Plan: true,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: filepath.Join(root, "missing-source"),
			DestData:   dest,
			DestPort:   15432,
		},
	})
	if err == nil {
		t.Fatal("expected plan to reject missing source data directory")
	}
	if !strings.Contains(err.Error(), "source data directory") {
		t.Fatalf("error should mention source data directory, got %v", err)
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

	want := "pig pg fork init dev -D /pg/data2 --src-port 15431 --dst-data /tmp/dev-fork --dst-port 15433 --plan"
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

	got, err := firstFreePortAvoiding(15432, map[int]bool{
		15432: true,
		15433: true,
	})
	if err != nil {
		t.Fatalf("firstFreePortAvoiding returned error: %v", err)
	}
	if got != 15434 {
		t.Fatalf("firstFreePortAvoiding() = %d, want 15434", got)
	}
}

func TestFirstFreePortAvoidingReportsExhaustion(t *testing.T) {
	original := forkPortFree
	t.Cleanup(func() {
		forkPortFree = original
	})
	forkPortFree = func(port int) bool { return false }

	if got, err := firstFreePortAvoiding(65535, nil); err == nil || got != 0 {
		t.Fatalf("firstFreePortAvoiding exhaustion = (%d, %v), want (0, error)", got, err)
	}
}

func TestReservedForkPortsExcludesCurrentDataDir(t *testing.T) {
	forks := []ForkInfo{
		{Name: "dev", Target: ForkEndpoint{Data: "/pg/data-dev", Port: 15432}},
		{Name: "test", Target: ForkEndpoint{Data: "/pg/data-test", Port: 15433}},
		{Name: "old", Target: ForkEndpoint{Data: "/pg/data-old"}},
	}

	reserved := reservedForkPortsAs("", forks, "/pg/data-dev")
	if reserved[15432] {
		t.Fatal("reservedForkPortsAs should ignore the current fork data directory")
	}
	if !reserved[15433] {
		t.Fatal("reservedForkPortsAs should include other managed fork ports")
	}
	if reserved[0] {
		t.Fatal("reservedForkPortsAs should ignore empty ports")
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

func TestRegularCopyFallbackIsAllowedButWarns(t *testing.T) {
	// A regular (non-CoW) copy is never blocked; it only triggers a countdown
	// warning so the operator can cancel before a full-size copy proceeds.
	if reason := forkCountdownReason(&State{CloneMode: CloneModeCopy, FS: "ext4"}); reason == "" {
		t.Fatal("regular copy fallback should warn via a countdown reason")
	}
	if reason := forkCountdownReason(&State{CloneMode: CloneModeCOW, FS: "xfs"}); reason != "" {
		t.Fatalf("CoW clone should not warn, got %q", reason)
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
		Kind:  KindInstance,
		Start: true,
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
	if info.Commands.Stop != "pig pg fork stop dev" {
		t.Fatalf("Stop command = %q, want pig pg fork stop dev", info.Commands.Stop)
	}
	if info.Commands.Remove != "pig pg fork rm dev --stop" {
		t.Fatalf("Remove command = %q, want pig pg fork rm dev --stop", info.Commands.Remove)
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
	if info.Commands.Stop != "pig pg fork stop --dst-data /tmp/dev-fork" {
		t.Fatalf("Stop command = %q, want unmanaged pig command", info.Commands.Stop)
	}
	if info.Commands.Remove != "pig pg fork rm --dst-data /tmp/dev-fork --stop" {
		t.Fatalf("Remove command = %q, want unmanaged pig command", info.Commands.Remove)
	}
}

func TestInstanceResultUsesPigCleanupCommand(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind: KindInstance,
		Instance: InstanceOptions{
			Name: "dev",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}
	result := instanceResult(opts, &State{BackupMode: BackupModeHot, CloneMode: CloneModeCOW}, 0)
	if result.CleanupCommand != "pig pg fork rm dev --stop" {
		t.Fatalf("CleanupCommand = %q, want pig fork removal command", result.CleanupCommand)
	}
}

func TestForkExecutionSummaryIncludesPrecheckedTarget(t *testing.T) {
	opts, err := NormalizeOptions(&Options{
		Kind:  KindInstance,
		Start: true,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: "/pg/data",
			SourcePort: 5432,
			DestData:   "/tmp/dev-fork",
			DestPort:   15440,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeOptions returned error: %v", err)
	}
	summary := forkExecutionSummary(opts, &State{BackupMode: BackupModeHot, CloneMode: CloneModeCopy, FS: "ext4"})

	for _, want := range []string{
		"Precheck: OK",
		"Source: /pg/data @ 5432 (verified)",
		"Target: /tmp/dev-fork @ 15440 (unmanaged)",
		"After copy: start fork",
		"Backup: hot backup",
		"Copy: regular copy (ext4); may use full data directory space",
		"Pig: ",
		"Command: pig pg fork init dev --dst-data /tmp/dev-fork --dst-port 15440 --start",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestForkCreateHintShowsNextStepsForStoppedFork(t *testing.T) {
	hint := ForkCreateHint(ResultData{
		Name:            "dev",
		Destination:     "/pg/data-dev",
		DestinationPort: 15432,
		StartCommand:    "pig pg fork start dev",
		CleanupCommand:  "pig pg fork rm dev --stop",
	})
	for _, want := range []string{
		"Created: dev (/pg/data-dev)",
		"Port: 15432",
		"State: stopped",
		"Start: pig pg fork start dev",
		"Remove: pig pg fork rm dev --stop",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("create hint missing %q:\n%s", want, hint)
		}
	}
}

func TestForkCreateHintShowsConnectForStartedFork(t *testing.T) {
	hint := ForkCreateHint(ResultData{
		Name:            "dev",
		Destination:     "/pg/data-dev",
		DestinationPort: 15432,
		Started:         true,
		ConnectCommand:  "psql -p 15432 -d postgres",
		StopCommand:     "pig pg fork stop dev",
		CleanupCommand:  "pig pg fork rm dev --stop",
	})
	for _, want := range []string{
		"Created: dev (/pg/data-dev)",
		"State: running",
		"Connect: psql -p 15432 -d postgres",
		"Stop: pig pg fork stop dev",
		"Remove: pig pg fork rm dev --stop",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("create hint missing %q:\n%s", want, hint)
		}
	}
}

func TestForkActionHintShowsStopAndRemoveResults(t *testing.T) {
	stopHint := ForkActionHint("fork stop", ResultData{Name: "dev", Destination: "/pg/data-dev"})
	if !strings.Contains(stopHint, "Stopped: dev (/pg/data-dev)") {
		t.Fatalf("stop hint = %q", stopHint)
	}
	alreadyHint := ForkActionHint("fork stop", ResultData{Name: "dev", Destination: "/pg/data-dev", Already: true})
	if !strings.Contains(alreadyHint, "Already stopped: dev (/pg/data-dev)") {
		t.Fatalf("already stopped hint = %q", alreadyHint)
	}
	removeHint := ForkActionHint("fork remove", ResultData{Name: "dev", Destination: "/pg/data-dev"})
	if !strings.Contains(removeHint, "Removed: dev (/pg/data-dev)") {
		t.Fatalf("remove hint = %q", removeHint)
	}
}

func TestForkConnectionHintShowsPortAndPsqlCommand(t *testing.T) {
	hint := ForkConnectionHint(ResultData{
		DestinationPort: 15432,
		Started:         true,
		ConnectCommand:  "psql -p 15432 -d postgres",
	})
	for _, want := range []string{
		"Fork is running on port 15432",
		"Connect: psql -p 15432 -d postgres",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q:\n%s", want, hint)
		}
	}
}

func TestForkConnectionHintEmptyForStoppedFork(t *testing.T) {
	if hint := ForkConnectionHint(ResultData{DestinationPort: 15432}); hint != "" {
		t.Fatalf("stopped fork hint = %q, want empty", hint)
	}
}

func TestCountdownTickMessageUsesProceedingWording(t *testing.T) {
	if got := countdownTickMessage(5); got != "\rProceeding in 5 seconds... " {
		t.Fatalf("countdown tick = %q", got)
	}
}

func TestForkProgressWritesOnlyForInteractiveExecution(t *testing.T) {
	disabled := captureForkStderr(t, func() {
		forkProgress(&Options{}, "copying data directory")
	})
	if disabled != "" {
		t.Fatalf("progress should be silent when disabled, got %q", disabled)
	}

	enabled := captureForkStderr(t, func() {
		forkProgress(&Options{Progress: true}, "copying data directory")
	})
	if enabled != "Step: copying data directory\n" {
		t.Fatalf("progress output = %q", enabled)
	}
}

func TestStartForkRejectsPortOverrideForRunningFork(t *testing.T) {
	originalRead := forkReadFileAsDBSU
	originalCheckDataDir := forkCheckDataDir
	originalRunning := forkCheckPostgresRunning
	t.Cleanup(func() {
		forkReadFileAsDBSU = originalRead
		forkCheckDataDir = originalCheckDataDir
		forkCheckPostgresRunning = originalRunning
	})
	forkCheckDataDir = func(dbsu, dataDir string) (bool, bool) {
		return true, true
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"dev","managed":true,"target":{"data":"/pg/data-dev","port":15432}}`, nil
	}
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		return true, 123
	}

	_, err := StartFork(ForkTargetOptions{Name: "dev", DestPort: 15440})
	if err == nil {
		t.Fatal("expected running fork port override to be rejected")
	}
	if !strings.Contains(err.Error(), "already running") || !strings.Contains(err.Error(), "15432") {
		t.Fatalf("error should mention running fork port, got %v", err)
	}
}

func TestReservedManagedForkPortsScansVisibleSymlinkRootAsDBSU(t *testing.T) {
	originalLstat := forkLstat
	originalOutput := forkDBSUCommandOutput
	originalRead := forkReadFileAsDBSU
	t.Cleanup(func() {
		forkLstat = originalLstat
		forkDBSUCommandOutput = originalOutput
		forkReadFileAsDBSU = originalRead
	})
	forkLstat = func(path string) (os.FileInfo, error) {
		if path != "/pg" {
			t.Fatalf("unexpected lstat path: %s", path)
		}
		return fakeForkFileInfo{name: "pg"}, nil
	}
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if strings.Join(args, " ") != "find -H /pg -mindepth 1 -maxdepth 1 -type d -name data-* -print" {
			t.Fatalf("unexpected scan command: %#v", args)
		}
		return "/pg/data-dev\n", nil
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		if path != "/pg/data-dev/fork.json" {
			t.Fatalf("unexpected metadata path: %s", path)
		}
		return `{"kind":"pg_fork","version":1,"name":"dev","target":{"data":"/pg/data-dev","port":15432}}`, nil
	}

	reserved := reservedManagedForkPorts("postgres", "")
	if !reserved[15432] {
		t.Fatalf("reservedManagedForkPorts should include metadata port 15432: %#v", reserved)
	}
}

func TestForkPortReservationIgnoresCurrentForkPathAlias(t *testing.T) {
	originalLstat := forkLstat
	originalOutput := forkDBSUCommandOutput
	originalRead := forkReadFileAsDBSU
	originalResolve := postgresDBSUCommandOutput
	t.Cleanup(func() {
		forkLstat = originalLstat
		forkDBSUCommandOutput = originalOutput
		forkReadFileAsDBSU = originalRead
		postgresDBSUCommandOutput = originalResolve
	})
	forkLstat = func(path string) (os.FileInfo, error) {
		return fakeForkFileInfo{name: filepath.Base(path)}, nil
	}
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if strings.Join(args, " ") != "find -H /pg -mindepth 1 -maxdepth 1 -type d -name data-* -print" {
			t.Fatalf("unexpected scan command: %#v", args)
		}
		return "/pg/data-dev\n", nil
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"dev","managed":true,"target":{"data":"/data/postgres/pg-meta-18/data-dev","port":15432}}`, nil
	}
	postgresDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		switch strings.Join(args, " ") {
		case "readlink -f /data/postgres/pg-meta-18/data-dev", "readlink -f /pg/data-dev":
			return "/data/postgres/pg-meta-18/data-dev\n", nil
		default:
			t.Fatalf("unexpected resolve command: %#v", args)
		}
		return "", nil
	}

	if forkPortReservedByManagedFork("postgres", 15432, "/pg/data-dev") {
		t.Fatal("current fork path alias should not reserve its own port")
	}
}

func TestScanForksAsReadsForkInfoAndOrphans(t *testing.T) {
	originalOutput := forkDBSUCommandOutput
	originalRead := forkReadFileAsDBSU
	t.Cleanup(func() {
		forkDBSUCommandOutput = originalOutput
		forkReadFileAsDBSU = originalRead
	})
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if len(args) < 2 || args[0] != "find" {
			t.Fatalf("ScanForksAs should enumerate via find, got %#v", args)
		}
		return "/pg/data-dev\n/pg/data-old\n", nil
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		if path == "/pg/data-dev/fork.json" {
			return `{"kind":"pg_fork","version":1,"name":"dev","target":{"data":"/pg/data-dev","port":15432,"started":true}}`, nil
		}
		return "", os.ErrNotExist
	}

	forks, err := ScanForksAs("postgres", "/pg")
	if err != nil {
		t.Fatalf("ScanForksAs returned error: %v", err)
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

func TestRemoveForkRefusesRunningForkWithoutStop(t *testing.T) {
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
	if !strings.Contains(err.Error(), "--stop") || strings.Contains(err.Error(), "--stop -f") {
		t.Fatalf("error should mention --stop without requiring -f, got %v", err)
	}
}

func TestRemoveForkStopDoesNotRequireForce(t *testing.T) {
	originalCommand := forkDBSUCommand
	originalRead := forkReadFileAsDBSU
	originalCheckDataDir := forkCheckDataDir
	originalRunning := forkCheckPostgresRunning
	originalStop := forkStopPostgres
	t.Cleanup(func() {
		forkDBSUCommand = originalCommand
		forkReadFileAsDBSU = originalRead
		forkCheckDataDir = originalCheckDataDir
		forkCheckPostgresRunning = originalRunning
		forkStopPostgres = originalStop
	})
	forkCheckDataDir = func(dbsu, dataDir string) (bool, bool) {
		return true, true
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"dev","managed":true,"target":{"data":"/pg/data-dev","port":15432}}`, nil
	}
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		return true, 123
	}
	stopped := false
	forkStopPostgres = func(cfg *Config, opts *StopOptions) error {
		stopped = true
		return nil
	}
	commands := []string{}
	forkDBSUCommand = func(dbsu string, args []string) error {
		commands = append(commands, strings.Join(args, " "))
		return nil
	}

	_, err := RemoveFork(ForkTargetOptions{Name: "dev", StopBefore: true, Yes: true})
	if err != nil {
		t.Fatalf("RemoveFork returned error: %v", err)
	}
	if !stopped {
		t.Fatal("--stop should stop the running fork without requiring -f")
	}
	if !containsForkArg(commands, "rm -rf -- /pg/data-dev") {
		t.Fatalf("remove command not executed: %#v", commands)
	}
}

func TestRemoveForkSuppressesCommandEchoByDefault(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	dataDir := filepath.Join(root, "dev-fork")
	if err := os.Mkdir(dataDir, 0755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "fork.json"), []byte(`{"kind":"pg_fork","version":1,"name":"dev","managed":false,"target":{"data":"`+dataDir+`","port":15432}}`), 0644); err != nil {
		t.Fatalf("write fork.json: %v", err)
	}

	errOutput := captureForkStderr(t, func() {
		if _, err := RemoveFork(ForkTargetOptions{DbSU: dbsu, DestData: dataDir, Force: true, Yes: true}); err != nil {
			t.Fatalf("RemoveFork returned error: %v", err)
		}
	})
	if errOutput != "" {
		t.Fatalf("RemoveFork should be silent by default, got stderr %q", errOutput)
	}
}

func TestRemoveForkConfirmsBeforeStoppingRunningFork(t *testing.T) {
	originalCommand := forkDBSUCommand
	originalRead := forkReadFileAsDBSU
	originalCheckDataDir := forkCheckDataDir
	originalRunning := forkCheckPostgresRunning
	originalStop := forkStopPostgres
	originalConfirm := forkConfirmCountdown
	t.Cleanup(func() {
		forkDBSUCommand = originalCommand
		forkReadFileAsDBSU = originalRead
		forkCheckDataDir = originalCheckDataDir
		forkCheckPostgresRunning = originalRunning
		forkStopPostgres = originalStop
		forkConfirmCountdown = originalConfirm
	})
	forkCheckDataDir = func(dbsu, dataDir string) (bool, bool) {
		return true, true
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"dev","managed":true,"target":{"data":"/pg/data-dev","port":15432}}`, nil
	}
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		return true, 123
	}
	events := []string{}
	forkConfirmCountdown = func(warning, action string) error {
		events = append(events, "confirm")
		return nil
	}
	forkStopPostgres = func(cfg *Config, opts *StopOptions) error {
		events = append(events, "stop")
		return nil
	}
	forkDBSUCommand = func(dbsu string, args []string) error {
		if strings.Join(args, " ") == "rm -rf -- /pg/data-dev" {
			events = append(events, "remove")
		}
		return nil
	}

	_, err := RemoveFork(ForkTargetOptions{Name: "dev", StopBefore: true})
	if err != nil {
		t.Fatalf("RemoveFork returned error: %v", err)
	}
	if strings.Join(events, ",") != "confirm,stop,remove" {
		t.Fatalf("events = %#v, want confirm before stop/remove", events)
	}
}

func TestRemoveForkRejectsManagedForkThroughDstData(t *testing.T) {
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
		return true, true
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"dev","managed":true,"target":{"data":"/pg/data-dev","port":15432}}`, nil
	}
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		return false, 0
	}
	forkDBSUCommand = func(dbsu string, args []string) error {
		t.Fatalf("remove command should not run for managed fork through --dst-data: %#v", args)
		return nil
	}

	_, err := RemoveFork(ForkTargetOptions{DestData: "/pg/data-dev", Force: true, Yes: true})
	if err == nil {
		t.Fatal("expected managed fork through --dst-data to be rejected")
	}
	if !strings.Contains(err.Error(), "managed fork") {
		t.Fatalf("error should mention managed fork, got %v", err)
	}
}

func TestRemoveForkRejectsMismatchedForkMetadataTarget(t *testing.T) {
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
		return true, true
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"dev","managed":false,"target":{"data":"/tmp/other-fork","port":15432}}`, nil
	}
	forkCheckPostgresRunning = func(dbsu, dataDir string) (bool, int) {
		return false, 0
	}
	forkDBSUCommand = func(dbsu string, args []string) error {
		t.Fatalf("remove command should not run with mismatched metadata target: %#v", args)
		return nil
	}

	_, err := RemoveFork(ForkTargetOptions{DestData: "/tmp/dev-fork", Force: true, Yes: true})
	if err == nil {
		t.Fatal("expected mismatched metadata target to be rejected")
	}
	if !strings.Contains(err.Error(), "metadata target") {
		t.Fatalf("error should mention metadata target, got %v", err)
	}
}

func TestRemoveForkRejectsUnmanagedSymlink(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "link")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatalf("mkdir real dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "fork.json"), []byte(`{"kind":"pg_fork","version":1,"name":"dev","managed":false,"target":{"data":"`+linkDir+`","port":15432}}`), 0644); err != nil {
		t.Fatalf("write fork.json: %v", err)
	}
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink fork dir: %v", err)
	}

	_, err := RemoveFork(ForkTargetOptions{DbSU: dbsu, DestData: linkDir, Force: true, Yes: true})
	if err == nil {
		t.Fatal("expected symlink fork path to be rejected")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error should mention symlink, got %v", err)
	}
	if _, statErr := os.Stat(realDir); statErr != nil {
		t.Fatalf("real fork dir should remain after rejected symlink removal: %v", statErr)
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
	if !strings.Contains(err.Error(), "Hint:") || !strings.Contains(err.Error(), "pig pg fork stop") {
		t.Fatalf("error should include stop hint, got %v", err)
	}
}

func TestPrecheckInstanceExistingDestinationIncludesReplacementHint(t *testing.T) {
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

	_, err := precheckInstance(&Options{
		Kind: KindInstance,
		DbSU: dbsu,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: source,
			DestData:   dest,
			SourcePort: 5432,
			DestPort:   15432,
		},
	})
	if err == nil {
		t.Fatal("expected existing destination error")
	}
	for _, want := range []string{"destination data directory exists", "Hint:", "-f/--force", "pig pg fork rm --dst-data", dest} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("existing destination error missing %q: %v", want, err)
		}
	}
}

func TestPrecheckInstanceReportsUnreachableSourcePortBeforeColdCopyFallback(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	source := filepath.Join(root, "data")
	dest := filepath.Join(root, "data-dev")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "postmaster.pid"), []byte(strconv.Itoa(os.Getpid())+"\n"+source+"\n123\n5432\n"), 0644); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}

	_, err := precheckInstance(&Options{
		Kind: KindInstance,
		DbSU: dbsu,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: source,
			DestData:   dest,
			SourcePort: 15999,
			DestPort:   15432,
		},
	})
	if err == nil {
		t.Fatal("expected unreachable source port error")
	}
	if !strings.Contains(err.Error(), "15999") || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("error should mention unreachable source port, got %v", err)
	}
}

func TestPrecheckInstanceRejectsSourcePortDataDirMismatch(t *testing.T) {
	originalProbe := forkProbeSourceDataDir
	originalPortFree := forkPortFree
	t.Cleanup(func() {
		forkProbeSourceDataDir = originalProbe
		forkPortFree = originalPortFree
	})
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	source := filepath.Join(root, "data")
	other := filepath.Join(root, "other")
	dest := filepath.Join(root, "data-dev")
	for _, dir := range []string{source, other} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
			t.Fatalf("write PG_VERSION: %v", err)
		}
	}
	forkProbeSourceDataDir = func(dbsu string, port int) (string, error) {
		if port != 5432 {
			t.Fatalf("probe port = %d, want 5432", port)
		}
		return other, nil
	}
	forkPortFree = func(port int) bool { return true }

	_, err := precheckInstance(&Options{
		Kind: KindInstance,
		DbSU: dbsu,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: source,
			DestData:   dest,
			SourcePort: 5432,
			DestPort:   15432,
		},
	})
	if err == nil {
		t.Fatal("expected source port/data directory mismatch to be rejected")
	}
	if !strings.Contains(err.Error(), "does not match source data directory") {
		t.Fatalf("error should mention source data directory mismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), "Hint:") {
		t.Fatalf("error should include hint, got %v", err)
	}
}

func TestPrecheckInstancePortReservedErrorNamesManagedFork(t *testing.T) {
	originalLstat := forkLstat
	originalOutput := forkDBSUCommandOutput
	originalRead := forkReadFileAsDBSU
	t.Cleanup(func() {
		forkLstat = originalLstat
		forkDBSUCommandOutput = originalOutput
		forkReadFileAsDBSU = originalRead
	})
	dbsu := withCurrentUserAsDBSU(t)
	root := t.TempDir()
	source := filepath.Join(root, "data")
	dest := filepath.Join(root, "data-dev")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "PG_VERSION"), []byte("18\n"), 0644); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	forkLstat = func(path string) (os.FileInfo, error) {
		if path != "/pg" {
			t.Fatalf("unexpected lstat path: %s", path)
		}
		return fakeForkFileInfo{name: "pg"}, nil
	}
	forkDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if args[0] != "find" {
			t.Fatalf("unexpected command: %#v", args)
		}
		return "/pg/data-other\n", nil
	}
	forkReadFileAsDBSU = func(path, dbsu string) (string, error) {
		return `{"kind":"pg_fork","version":1,"name":"other","managed":true,"target":{"data":"/pg/data-other","port":15432}}`, nil
	}

	_, err := precheckInstance(&Options{
		Kind: KindInstance,
		DbSU: dbsu,
		Instance: InstanceOptions{
			Name:       "dev",
			SourceData: source,
			DestData:   dest,
			SourcePort: 5432,
			DestPort:   15432,
		},
	})
	if err == nil {
		t.Fatal("expected reserved port error")
	}
	for _, want := range []string{"15432", "other", "/pg/data-other", "Hint:", "pig pg fork list", "-p"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("reserved port error missing %q: %v", want, err)
		}
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

func TestPostmasterPIDMatchesDataDirAsDBSUResolvesSymlinkAliases(t *testing.T) {
	original := postgresDBSUCommandOutput
	t.Cleanup(func() {
		postgresDBSUCommandOutput = original
	})
	postgresDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		switch strings.Join(args, " ") {
		case "readlink -f /data/postgres/pg-meta-18/data-dev":
			return "/data/postgres/pg-meta-18/data-dev\n", nil
		case "readlink -f /pg/data-dev":
			return "/data/postgres/pg-meta-18/data-dev\n", nil
		default:
			t.Fatalf("unexpected command: %#v", args)
		}
		return "", nil
	}

	pidContent := strconv.Itoa(os.Getpid()) + "\n/data/postgres/pg-meta-18/data-dev\n1782617444\n15432\n"
	if !postmasterPIDMatchesDataDirAsDBSU("postgres", pidContent, "/pg/data-dev") {
		t.Fatal("dbsu path resolution should treat symlink and real data paths as the same fork")
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

type fakeForkFileInfo struct {
	name string
}

func (f fakeForkFileInfo) Name() string       { return f.name }
func (f fakeForkFileInfo) Size() int64        { return 0 }
func (f fakeForkFileInfo) Mode() os.FileMode  { return os.ModeSymlink }
func (f fakeForkFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeForkFileInfo) IsDir() bool        { return false }
func (f fakeForkFileInfo) Sys() any           { return nil }

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

func captureForkStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe failed: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr failed: %v", err)
	}
	return string(out)
}
