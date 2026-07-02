package pitr

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"pig/cli/pgbackrest"
	"pig/internal/output"
	"pig/internal/utils"
)

func TestBuildPlanBasic(t *testing.T) {
	state := &SystemState{
		PatroniActive: true,
		PGRunning:     true,
		DataDir:       "/pg/data",
		DbSU:          "postgres",
	}
	opts := &Options{
		Time: "2026-01-31 01:00:00",
	}

	plan := BuildPlan(state, opts)
	if plan == nil {
		t.Fatal("BuildPlan returned nil")
	}
	if plan.Command == "" {
		t.Error("Plan.Command should not be empty")
	}
	if !strings.Contains(plan.Command, "-t") {
		t.Errorf("Plan.Command should include -t, got %q", plan.Command)
	}

	if !containsAction(plan.Actions, "Stop Patroni service") {
		t.Error("Plan.Actions should include stopping Patroni")
	}
	if !containsAction(plan.Actions, "Ensure PostgreSQL is stopped") {
		t.Error("Plan.Actions should include stopping PostgreSQL")
	}
	if !containsAction(plan.Actions, "Execute pgBackRest restore") {
		t.Error("Plan.Actions should include pgBackRest restore")
	}

	if !containsResource(plan.Affects, "backup") {
		t.Error("Plan.Affects should include backup info")
	}
	if !containsResource(plan.Affects, "target") {
		t.Error("Plan.Affects should include recovery target")
	}
	if !strings.Contains(plan.Expected, "/pg/data") {
		t.Errorf("Plan.Expected should mention data dir, got %q", plan.Expected)
	}
	if len(plan.Risks) == 0 {
		t.Error("Plan.Risks should not be empty")
	}
}

// TestBuildPlanManagedDirAlwaysStopsPatroni (B10): with --skip-patroni removed,
// an active Patroni on the managed data dir is ALWAYS stopped before restore.
func TestBuildPlanManagedDirAlwaysStopsPatroni(t *testing.T) {
	state := &SystemState{
		PatroniActive: true,
		PGRunning:     false,
		DataDir:       "/pg/data",
		DbSU:          "postgres",
	}
	opts := &Options{
		Default:   true,
		NoRestart: true,
	}

	plan := BuildPlan(state, opts)
	if !containsAction(plan.Actions, "Stop Patroni service") {
		t.Error("Plan.Actions should always include Patroni stop for managed data dir")
	}
	if containsAction(plan.Actions, "Start PostgreSQL") {
		t.Error("Plan.Actions should not include PostgreSQL start when no-restart is set")
	}
	if !strings.Contains(plan.Expected, "remains stopped") {
		t.Errorf("Plan.Expected should mention stopped state, got %q", plan.Expected)
	}
}

func TestBuildPlanCustomDataDirSideRestoreDoesNotManagePatroni(t *testing.T) {
	state := &SystemState{
		PatroniActive: true,
		SideRestore:   true,
		PGRunning:     false,
		DataDir:       "/tmp/pitr-restore",
		DbSU:          "postgres",
	}
	opts := &Options{
		Default:   true,
		DataDir:   "/tmp/pitr-restore",
		NoRestart: true,
	}

	plan := BuildPlan(state, opts)
	if containsAction(plan.Actions, "Stop Patroni service") {
		t.Fatal("side restore plan should not stop Patroni")
	}
	if containsAction(plan.Actions, "Ensure PostgreSQL is stopped") {
		t.Fatal("side restore plan should not stop PostgreSQL only because Patroni is active")
	}
	for _, res := range plan.Affects {
		if res.Type == "service" && (res.Name == "patroni" || res.Name == "postgresql") {
			t.Fatalf("side restore plan should not affect live services, got %+v", res)
		}
	}
}

func TestPrintExecutionPlanSideRestoreShowsPatroniLeftRunning(t *testing.T) {
	state := &SystemState{
		PatroniActive: true,
		SideRestore:   true,
		DataDir:       "/tmp/pitr-restore",
		DbSU:          "postgres",
	}
	opts := &Options{Default: true, DataDir: "/tmp/pitr-restore"}

	output := capturePITRStderr(t, func() {
		printExecutionPlan(state, opts)
	})

	if !strings.Contains(output, "Patroni Service: ") || !strings.Contains(output, "left running for custom data dir") {
		t.Fatalf("side restore plan should show active Patroni left running, got:\n%s", output)
	}
	if strings.Contains(output, "Stop Patroni service") {
		t.Fatalf("side restore plan should not stop Patroni, got:\n%s", output)
	}
}

func TestBuildPlanIncludesRecoveryWaitForAutoPromoteTargets(t *testing.T) {
	state := &SystemState{
		DataDir: "/pg/data",
		DbSU:    "postgres",
	}

	plan := BuildPlan(state, &Options{Default: true})
	if !containsAction(plan.Actions, "Wait for PostgreSQL recovery to complete") {
		t.Fatalf("default PITR plan should include recovery completion wait, got %+v", plan.Actions)
	}

	plan = BuildPlan(state, &Options{Time: "2026-01-31 01:00:00"})
	if containsAction(plan.Actions, "Wait for PostgreSQL recovery to complete") {
		t.Fatalf("manual PITR target should not wait for promotion, got %+v", plan.Actions)
	}

	plan = BuildPlan(state, &Options{Time: "2026-01-31 01:00:00", TargetAction: "promote"})
	if !containsAction(plan.Actions, "Wait for PostgreSQL recovery to complete") {
		t.Fatalf("target-action=promote PITR plan should include recovery completion wait, got %+v", plan.Actions)
	}
}

func TestBuildPlanNilInputs(t *testing.T) {
	// Test nil state
	plan := BuildPlan(nil, &Options{Default: true})
	if plan == nil {
		t.Fatal("BuildPlan(nil, opts) should not return nil")
	}
	if len(plan.Actions) != 0 {
		t.Errorf("BuildPlan with nil state should have empty actions, got %d", len(plan.Actions))
	}

	// Test nil opts
	plan = BuildPlan(&SystemState{DataDir: "/pg/data"}, nil)
	if plan == nil {
		t.Fatal("BuildPlan(state, nil) should not return nil")
	}
	if len(plan.Actions) != 0 {
		t.Errorf("BuildPlan with nil opts should have empty actions, got %d", len(plan.Actions))
	}

	// Test both nil
	plan = BuildPlan(nil, nil)
	if plan == nil {
		t.Fatal("BuildPlan(nil, nil) should not return nil")
	}
}

func TestBuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		opts     *Options
		contains []string
		excludes []string
	}{
		{
			name:     "nil opts",
			opts:     nil,
			contains: []string{"pig", "pitr"},
			excludes: []string{"-t", "-d", "--plan"},
		},
		{
			name:     "default target",
			opts:     &Options{Default: true},
			contains: []string{"pig", "pitr", "-d"},
			excludes: []string{"-t", "-I"},
		},
		{
			name:     "time target",
			opts:     &Options{Time: "2026-01-31 01:00:00"},
			contains: []string{"-t"},
			excludes: []string{"-d", "-I"},
		},
		{
			name:     "immediate target",
			opts:     &Options{Immediate: true},
			contains: []string{"-I"},
			excludes: []string{"-d", "-t"},
		},
		{
			name:     "with backup set",
			opts:     &Options{Default: true, Set: "20240101-010101F"},
			contains: []string{"-b", "20240101-010101F"},
			excludes: []string{},
		},
		{
			name:     "with flags",
			opts:     &Options{Default: true, NoRestart: true, Exclusive: true, ForceStop: true},
			contains: []string{"--no-restart", "-X", "--force-stop"},
			excludes: []string{"--skip-patroni", "-P"},
		},
		{
			name:     "with target action",
			opts:     &Options{Time: "2026-01-31 01:00:00", TargetAction: "promote"},
			contains: []string{"--target-action", "promote"},
			excludes: []string{"-P"},
		},
		{
			name: "with operational context",
			opts: &Options{
				Time:       "2026-01-31 01:00:00",
				DataDir:    "/data/pg",
				Stanza:     "pg-prod",
				ConfigPath: "/etc/pgbackrest/custom.conf",
				Repo:       "2",
				DbSU:       "postgres",
			},
			contains: []string{"-t", `"2026-01-31 01:00:00"`, "-D", "/data/pg", "-s", "pg-prod", "-c", "/etc/pgbackrest/custom.conf", "-r", "2", "-U", "postgres"},
			excludes: []string{},
		},
		{
			name:     "plan mode",
			opts:     &Options{Default: true, Plan: true},
			contains: []string{"--plan"},
			excludes: []string{},
		},
		{
			name:     "lsn target",
			opts:     &Options{LSN: "0/1234567"},
			contains: []string{"--lsn", "0/1234567"},
			excludes: []string{"-d", "-t"},
		},
		{
			name:     "xid target",
			opts:     &Options{XID: "12345"},
			contains: []string{"--xid", "12345"},
			excludes: []string{"-d", "-t"},
		},
		{
			name:     "name target",
			opts:     &Options{Name: "my_restore_point"},
			contains: []string{"--name", "my_restore_point"},
			excludes: []string{"-d", "-t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildCommand(tt.opts)
			for _, c := range tt.contains {
				if !strings.Contains(cmd, c) {
					t.Errorf("command should contain %q: %q", c, cmd)
				}
			}
			for _, e := range tt.excludes {
				if strings.Contains(cmd, e) {
					t.Errorf("command should not contain %q: %q", e, cmd)
				}
			}
		})
	}
}

func TestGetTargetDescription(t *testing.T) {
	tests := []struct {
		name     string
		opts     *Options
		expected string
	}{
		{"default", &Options{Default: true}, "Latest (end of WAL stream)"},
		{"immediate", &Options{Immediate: true}, "Backup consistency point"},
		{"time", &Options{Time: "2026-01-31"}, "Time: 2026-01-31"},
		{"name", &Options{Name: "my_point"}, "Restore point: my_point"},
		{"lsn", &Options{LSN: "0/1234"}, "LSN: 0/1234"},
		{"xid", &Options{XID: "999"}, "XID: 999"},
		{"none", &Options{}, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTargetDescription(tt.opts)
			if got != tt.expected {
				t.Errorf("getTargetDescription() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildActions(t *testing.T) {
	// Test with nil inputs
	actions := buildActions(nil, nil)
	if actions != nil {
		t.Errorf("buildActions(nil, nil) should return nil, got %v", actions)
	}

	actions = buildActions(&SystemState{}, nil)
	if actions != nil {
		t.Errorf("buildActions(state, nil) should return nil, got %v", actions)
	}

	actions = buildActions(nil, &Options{})
	if actions != nil {
		t.Errorf("buildActions(nil, opts) should return nil, got %v", actions)
	}

	// Test normal case
	state := &SystemState{PatroniActive: true, PGRunning: true, DataDir: "/pg/data"}
	opts := &Options{Default: true}
	actions = buildActions(state, opts)
	if len(actions) < 3 {
		t.Errorf("buildActions should return at least 3 actions, got %d", len(actions))
	}
}

func TestBuildAffects(t *testing.T) {
	// Test with nil inputs
	affects := buildAffects(nil, nil)
	if affects != nil {
		t.Errorf("buildAffects(nil, nil) should return nil, got %v", affects)
	}

	// Test normal case
	state := &SystemState{PatroniActive: true, DataDir: "/pg/data"}
	opts := &Options{Default: true}
	affects = buildAffects(state, opts)
	if len(affects) < 2 {
		t.Errorf("buildAffects should return at least 2 resources, got %d", len(affects))
	}

	// Test with specific backup set
	opts = &Options{Default: true, Set: "20240101-010101F"}
	affects = buildAffects(state, opts)
	hasBackup := false
	for _, a := range affects {
		if a.Type == "backup" && a.Name == "20240101-010101F" {
			hasBackup = true
			break
		}
	}
	if !hasBackup {
		t.Error("buildAffects should include specified backup set")
	}
}

func TestBuildExpected(t *testing.T) {
	// Test with nil inputs
	expected := buildExpected(nil, nil)
	if expected != "" {
		t.Errorf("buildExpected(nil, nil) should return empty, got %q", expected)
	}

	// Test normal case
	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Default: true}
	expected = buildExpected(state, opts)
	if !strings.Contains(expected, "/pg/data") {
		t.Errorf("buildExpected should contain data dir, got %q", expected)
	}

	// Test with NoRestart
	opts = &Options{Default: true, NoRestart: true}
	expected = buildExpected(state, opts)
	if !strings.Contains(expected, "stopped") {
		t.Errorf("buildExpected with NoRestart should mention stopped, got %q", expected)
	}

	// Test with target action
	opts = &Options{Time: "2026-01-31 01:00:00", TargetAction: "promote"}
	expected = buildExpected(state, opts)
	if !strings.Contains(expected, "target action promote") {
		t.Errorf("buildExpected with TargetAction should mention target action, got %q", expected)
	}
}

func TestBuildRisks(t *testing.T) {
	// Test with nil inputs
	risks := buildRisks(nil, nil)
	if risks != nil {
		t.Errorf("buildRisks(nil, nil) should return nil, got %v", risks)
	}

	// Test base risks
	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Default: true}
	risks = buildRisks(state, opts)
	if len(risks) == 0 {
		t.Error("buildRisks should return at least one risk")
	}

	// Test with Patroni active
	state = &SystemState{PatroniActive: true, DataDir: "/pg/data"}
	opts = &Options{Default: true}
	risks = buildRisks(state, opts)
	hasPatroniRisk := false
	for _, r := range risks {
		if strings.Contains(r, "Patroni") {
			hasPatroniRisk = true
			break
		}
	}
	if !hasPatroniRisk {
		t.Error("buildRisks with Patroni active should mention Patroni")
	}

	// Test with Exclusive
	opts = &Options{Default: true, Exclusive: true}
	risks = buildRisks(state, opts)
	hasExclusiveRisk := false
	for _, r := range risks {
		if strings.Contains(r, "Exclusive") || strings.Contains(r, "before target") {
			hasExclusiveRisk = true
			break
		}
	}
	if !hasExclusiveRisk {
		t.Error("buildRisks with Exclusive should mention exclusive mode")
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", `"with space"`},
		{"with\ttab", `"with\ttab"`},
		{"no-special", "no-special"},
	}

	for _, tt := range tests {
		got := quoteIfNeeded(tt.input)
		if got != tt.expected {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func containsAction(actions []output.Action, needle string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, needle) {
			return true
		}
	}
	return false
}

func containsResource(resources []output.Resource, resType string) bool {
	for _, res := range resources {
		if res.Type == resType {
			return true
		}
	}
	return false
}

// ============================================================================
// PITRError and Error Code Tests
// ============================================================================

// TestPITRError_InvalidArgs verifies missing recovery target returns 160101
func TestPITRError_InvalidArgs(t *testing.T) {
	opts := &Options{} // No recovery target specified
	err := validateRecoveryTarget(opts)
	if err == nil {
		t.Fatal("validateRecoveryTarget should fail with no target")
	}

	pitrErr := &PITRError{Code: output.CodePITRInvalidArgs, Err: err}
	if pitrErr.Code != 160101 {
		t.Errorf("expected code 160101, got %d", pitrErr.Code)
	}
	if pitrErr.Error() == "" {
		t.Error("PITRError.Error() should not be empty")
	}

	// Also test multiple targets
	opts = &Options{Default: true, Immediate: true}
	err = validateRecoveryTarget(opts)
	if err == nil {
		t.Fatal("validateRecoveryTarget should fail with multiple targets")
	}
}

func TestValidateOptionsRejectsInvalidTimeAndRestoreExtraArgs(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
	}{
		{name: "invalid time", opts: &Options{Time: "2025-01-01 12:00:00junk"}},
		{name: "target extra arg", opts: &Options{Default: true, ExtraArgs: []string{"--target-action=promote"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOptions(tt.opts)
			if err == nil {
				t.Fatal("ValidateOptions should reject invalid PITR input")
			}
			if !strings.Contains(err.Error(), "invalid time format") && !strings.Contains(err.Error(), "conflicts with pig restore flags") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestPITRError_PrecheckFailed verifies precheck failures return 160601
func TestPITRError_PrecheckFailed(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRPrecheckFailed, Err: fmt.Errorf("data directory /pg/data does not exist")}
	if pitrErr.Code != 160601 {
		t.Errorf("expected code 160601, got %d", pitrErr.Code)
	}
	if !strings.Contains(pitrErr.Error(), "data directory") {
		t.Errorf("error message should mention data directory, got %q", pitrErr.Error())
	}
}

// TestPITRError_StopFailed verifies stop service failures return 160801
func TestPITRError_StopFailed(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRStopFailed, Err: fmt.Errorf("failed to stop patroni service")}
	if pitrErr.Code != 160801 {
		t.Errorf("expected code 160801, got %d", pitrErr.Code)
	}
	if !strings.Contains(pitrErr.Error(), "patroni") {
		t.Errorf("error message should mention patroni, got %q", pitrErr.Error())
	}
}

// TestPITRError_RestoreFailed verifies restore failures return 160802
func TestPITRError_RestoreFailed(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRRestoreFailed, Err: fmt.Errorf("pgbackrest restore failed: exit code 28")}
	if pitrErr.Code != 160802 {
		t.Errorf("expected code 160802, got %d", pitrErr.Code)
	}
	if !strings.Contains(pitrErr.Error(), "pgbackrest") {
		t.Errorf("error message should mention pgbackrest, got %q", pitrErr.Error())
	}
}

// TestPITRError_StartFailed verifies start failures return 160803
func TestPITRError_StartFailed(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRStartFailed, Err: fmt.Errorf("failed to start postgresql")}
	if pitrErr.Code != 160803 {
		t.Errorf("expected code 160803, got %d", pitrErr.Code)
	}
}

// TestPITRError_PostFailed verifies post-restore failures return 160804
func TestPITRError_PostFailed(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRPostFailed, Err: fmt.Errorf("post-restore guidance failed")}
	if pitrErr.Code != 160804 {
		t.Errorf("expected code 160804, got %d", pitrErr.Code)
	}
}

// TestPITRError_NoBackup verifies missing backup returns 160301
func TestPITRError_NoBackup(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRNoBackup, Err: fmt.Errorf("backup set 'nonexistent' not found")}
	if pitrErr.Code != 160301 {
		t.Errorf("expected code 160301, got %d", pitrErr.Code)
	}
	if !strings.Contains(pitrErr.Error(), "backup") {
		t.Errorf("error message should mention backup, got %q", pitrErr.Error())
	}
}

func TestClassifyRestoreError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "missing backup set",
			err:  fmt.Errorf("unable to find backup set 20250101-010101F"),
			want: output.CodePITRNoBackup,
		},
		{
			name: "backup set not found",
			err:  fmt.Errorf("backup set 'foo' does not exist"),
			want: output.CodePITRNoBackup,
		},
		{
			name: "backup set invalid",
			err:  fmt.Errorf("backup set 19000101-000000F is not valid"),
			want: output.CodePITRNoBackup,
		},
		{
			name: "generic restore failure",
			err:  fmt.Errorf("restore command failed with exit code 28"),
			want: output.CodePITRRestoreFailed,
		},
		{
			name: "nil error",
			err:  nil,
			want: output.CodePITRRestoreFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRestoreError(tt.err)
			if got != tt.want {
				t.Fatalf("classifyRestoreError() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsNoBackupError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "no prior backup",
			message: "ERROR: [037]: no prior backup exists",
			want:    true,
		},
		{
			name:    "unable to find backup",
			message: "unable to find backup set for stanza",
			want:    true,
		},
		{
			name:    "backup set not found",
			message: "backup set 'foo' not found",
			want:    true,
		},
		{
			name:    "backup set invalid",
			message: "backup set 19000101-000000F is not valid",
			want:    true,
		},
		{
			name:    "non-backup not found",
			message: "config file not found",
			want:    false,
		},
		{
			name:    "generic restore error",
			message: "restore process failed with timeout",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNoBackupError(tt.message)
			if got != tt.want {
				t.Fatalf("isNoBackupError(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

// TestPITRError_PgRunning verifies PG cannot be stopped returns 160602
func TestPITRError_PgRunning(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRPgRunning, Err: fmt.Errorf("postgresql still running after kill -9, manual intervention required")}
	if pitrErr.Code != 160602 {
		t.Errorf("expected code 160602, got %d", pitrErr.Code)
	}
	if !strings.Contains(pitrErr.Error(), "still running") {
		t.Errorf("error message should mention still running, got %q", pitrErr.Error())
	}
}

// TestPITRError_ExitCodeMapping verifies error code → exit code mapping consistency
func TestPITRError_ExitCodeMapping(t *testing.T) {
	tests := []struct {
		name         string
		code         int
		expectedExit int
	}{
		{"InvalidArgs", output.CodePITRInvalidArgs, 2},       // CAT_PARAM → Exit 2
		{"NoBackup", output.CodePITRNoBackup, 4},             // CAT_DEPEND → Exit 4
		{"PrecheckFailed", output.CodePITRPrecheckFailed, 9}, // CAT_STATE → Exit 9
		{"PgRunning", output.CodePITRPgRunning, 9},           // CAT_STATE → Exit 9
		{"StopFailed", output.CodePITRStopFailed, 1},         // CAT_OPERATION → Exit 1
		{"RestoreFailed", output.CodePITRRestoreFailed, 1},   // CAT_OPERATION → Exit 1
		{"StartFailed", output.CodePITRStartFailed, 1},       // CAT_OPERATION → Exit 1
		{"PostFailed", output.CodePITRPostFailed, 1},         // CAT_OPERATION → Exit 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := output.ExitCode(tt.code)
			if exitCode != tt.expectedExit {
				t.Errorf("ExitCode(%d) = %d, want %d", tt.code, exitCode, tt.expectedExit)
			}
		})
	}
}

// TestPITRError_ConfirmCancel verifies confirmation cancel returns 160101 (not 160801)
func TestPITRError_ConfirmCancel(t *testing.T) {
	// Simulate what ExecuteResult does when confirmation is cancelled
	cancelErr := fmt.Errorf("user cancelled")
	result := output.Fail(output.CodePITRInvalidArgs, "pitr confirmation cancelled").WithDetail(cancelErr.Error())

	if result.Code != output.CodePITRInvalidArgs {
		t.Errorf("confirmation cancel should use CodePITRInvalidArgs (160101), got %d", result.Code)
	}
	if result.Code == output.CodePITRStopFailed {
		t.Error("confirmation cancel must NOT use CodePITRStopFailed (was the old buggy behavior)")
	}
}

// TestPITRError_ErrorCodeValues verifies all error code numeric values
func TestPITRError_ErrorCodeValues(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"CodePITRInvalidArgs", output.CodePITRInvalidArgs, 160101},
		{"CodePITRNoBackup", output.CodePITRNoBackup, 160301},
		{"CodePITRPrecheckFailed", output.CodePITRPrecheckFailed, 160601},
		{"CodePITRPgRunning", output.CodePITRPgRunning, 160602},
		{"CodePITRStopFailed", output.CodePITRStopFailed, 160801},
		{"CodePITRRestoreFailed", output.CodePITRRestoreFailed, 160802},
		{"CodePITRStartFailed", output.CodePITRStartFailed, 160803},
		{"CodePITRPostFailed", output.CodePITRPostFailed, 160804},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.code, tt.expected)
			}
		})
	}
}

// TestPITRError_NilError verifies PITRError handles nil Err gracefully
func TestPITRError_NilError(t *testing.T) {
	pitrErr := &PITRError{Code: output.CodePITRStopFailed, Err: nil}
	if pitrErr.Error() != "pitr error" {
		t.Errorf("PITRError with nil Err should return 'pitr error', got %q", pitrErr.Error())
	}
	if pitrErr.Unwrap() != nil {
		t.Error("PITRError with nil Err should Unwrap to nil")
	}
}

// TestPITRError_Unwrap verifies PITRError properly wraps underlying errors
func TestPITRError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	pitrErr := &PITRError{Code: output.CodePITRStopFailed, Err: inner}

	if !errors.Is(pitrErr, inner) {
		t.Error("PITRError should wrap the inner error (errors.Is)")
	}
	if pitrErr.Unwrap() != inner {
		t.Error("PITRError.Unwrap() should return the inner error")
	}
}

// TestPITRError_TextModeExitCode verifies Execute() wraps PITRError in ExitCodeError
func TestPITRError_TextModeExitCode(t *testing.T) {
	// Simulate what Execute() does for each error type
	tests := []struct {
		name         string
		pitrErr      *PITRError
		expectedExit int
	}{
		{
			"StopFailed",
			&PITRError{Code: output.CodePITRStopFailed, Err: fmt.Errorf("stop failed")},
			1,
		},
		{
			"InvalidArgs",
			&PITRError{Code: output.CodePITRInvalidArgs, Err: fmt.Errorf("invalid args")},
			2,
		},
		{
			"PgRunning",
			&PITRError{Code: output.CodePITRPgRunning, Err: fmt.Errorf("pg running")},
			9,
		},
		{
			"NoBackup",
			&PITRError{Code: output.CodePITRNoBackup, Err: fmt.Errorf("no backup")},
			4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate Execute() wrapping
			exitErr := &utils.ExitCodeError{Code: output.ExitCode(tt.pitrErr.Code), Err: tt.pitrErr}
			if exitErr.Code != tt.expectedExit {
				t.Errorf("ExitCodeError.Code = %d, want %d", exitErr.Code, tt.expectedExit)
			}

			// Verify utils.ExitCode extracts properly
			exitCode := utils.ExitCode(exitErr)
			if exitCode != tt.expectedExit {
				t.Errorf("utils.ExitCode() = %d, want %d", exitCode, tt.expectedExit)
			}
		})
	}
}

// TestPITRError_PreCheckReturnsTypedError verifies preCheck returns PITRError
func TestPITRError_PreCheckReturnsTypedError(t *testing.T) {
	// Test with no recovery target
	opts := &Options{} // Missing target
	_, err := preCheck(opts)
	if err == nil {
		t.Fatal("preCheck should fail with no target")
	}

	pitrErr, ok := err.(*PITRError)
	if !ok {
		t.Fatalf("preCheck error should be *PITRError, got %T", err)
	}
	if pitrErr.Code != output.CodePITRInvalidArgs {
		t.Errorf("preCheck with no target should return CodePITRInvalidArgs (160101), got %d", pitrErr.Code)
	}
}

func TestPITRError_PreCheckValidatesRestoreOptionsBeforeDataDir(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
		want string
	}{
		{name: "invalid lsn", opts: &Options{LSN: "BAD"}, want: "invalid LSN"},
		{name: "invalid xid", opts: &Options{XID: "abc"}, want: "invalid XID"},
		{name: "default target-action", opts: &Options{Default: true, TargetAction: "promote"}, want: "--target-action"},
		{name: "default exclusive", opts: &Options{Default: true, Exclusive: true}, want: "--exclusive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := preCheck(tt.opts)
			if err == nil {
				t.Fatal("preCheck should fail")
			}
			pitrErr, ok := err.(*PITRError)
			if !ok {
				t.Fatalf("preCheck error should be *PITRError, got %T", err)
			}
			if pitrErr.Code != output.CodePITRInvalidArgs {
				t.Fatalf("preCheck code = %d, want %d", pitrErr.Code, output.CodePITRInvalidArgs)
			}
			if !strings.Contains(pitrErr.Error(), tt.want) {
				t.Fatalf("preCheck error = %q, want it to mention %q", pitrErr.Error(), tt.want)
			}
		})
	}
}

func TestValidatePITRDataDirAllowsExplicitUninitializedDirectory(t *testing.T) {
	if err := validatePITRDataDir("/tmp/pitr-restore", true, true, false); err != nil {
		t.Fatalf("explicit custom data dir should not require PG_VERSION: %v", err)
	}
	if err := validatePITRDataDir("/tmp/pitr-restore", true, false, false); err == nil {
		t.Fatal("explicit custom data dir should still require the directory to exist")
	}
	if err := validatePITRDataDir("/pg/data", false, true, false); err == nil {
		t.Fatal("default data dir should require PG_VERSION")
	}
}

func TestValidatePITRDataDirOwnerRequiresDBSUForSideRestore(t *testing.T) {
	err := validatePITRDataDirOwner("/tmp/pitr-restore", "postgres", "vagrant")
	if err == nil {
		t.Fatal("custom data dir owned by another user should fail")
	}
	if !strings.Contains(err.Error(), "owned by vagrant") {
		t.Fatalf("error should mention current owner, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "chown postgres") {
		t.Fatalf("error should suggest chown to dbsu, got %q", err.Error())
	}

	if err := validatePITRDataDirOwner("/tmp/pitr-restore", "postgres", "postgres"); err != nil {
		t.Fatalf("custom data dir owned by dbsu should pass: %v", err)
	}
}

// TestShouldManagePatroni (B10): Patroni lifecycle policy is now state-derived
// only — active Patroni + managed dir is always stopped, side restores never
// touch Patroni, inactive Patroni is never managed.
func TestShouldManagePatroni(t *testing.T) {
	if shouldManagePatroni(true, true) {
		t.Fatal("explicit custom data dir side restore should not manage Patroni")
	}
	if !shouldManagePatroni(true, false) {
		t.Fatal("default data dir PITR should manage active Patroni")
	}
	if shouldManagePatroni(false, false) {
		t.Fatal("inactive Patroni should not be managed")
	}
	if shouldManagePatroni(false, true) {
		t.Fatal("inactive Patroni side restore should not be managed")
	}
}

func TestClassifyPITRDataDirTreatsEquivalentManagedPathAsManaged(t *testing.T) {
	resolver := func(path string) (string, error) {
		switch path {
		case "/pg/data", "/pg/data/", "/pg/../pg/data":
			return "/data/postgres/pg-meta-1/data", nil
		default:
			return path, nil
		}
	}

	sideRestore := classifyPITRSideRestore("/pg/data/", "/pg/data", resolver)
	if sideRestore {
		t.Fatal("trailing-slash managed PGDATA should not be treated as side restore")
	}

	sideRestore = classifyPITRSideRestore("/pg/../pg/data", "/pg/data", resolver)
	if sideRestore {
		t.Fatal("canonically equivalent managed PGDATA should not be treated as side restore")
	}

	sideRestore = classifyPITRSideRestore("/tmp/pitr-restore", "/pg/data", resolver)
	if !sideRestore {
		t.Fatal("distinct custom data dir should be treated as side restore")
	}
}

func TestValidateSideRestorePolicyRequiresNoRestart(t *testing.T) {
	err := validateSideRestorePolicy(true, false)
	if err == nil {
		t.Fatal("custom data dir restore should require --no-restart")
	}
	if !strings.Contains(err.Error(), "--no-restart") {
		t.Fatalf("error should tell the operator to use --no-restart, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "original port") {
		t.Fatalf("error should explain restored config keeps the original port, got %q", err.Error())
	}

	if err := validateSideRestorePolicy(true, true); err != nil {
		t.Fatalf("custom data dir with --no-restart should be allowed: %v", err)
	}
	if err := validateSideRestorePolicy(false, false); err != nil {
		t.Fatalf("managed data dir should not require --no-restart: %v", err)
	}
}

func TestValidatePITRTargetActionPolicyRequiresNoRestartForShutdown(t *testing.T) {
	err := validatePITRTargetActionPolicy(&Options{Time: "2026-01-31 01:00:00", TargetAction: "shutdown"})
	if err == nil {
		t.Fatal("pitr target-action=shutdown should require --no-restart")
	}
	if !strings.Contains(err.Error(), "--no-restart") {
		t.Fatalf("error should mention --no-restart, got %q", err.Error())
	}

	if err := validatePITRTargetActionPolicy(&Options{Time: "2026-01-31 01:00:00", TargetAction: "shutdown", NoRestart: true}); err != nil {
		t.Fatalf("shutdown target action should be allowed with --no-restart: %v", err)
	}
	if err := validatePITRTargetActionPolicy(&Options{Time: "2026-01-31 01:00:00", TargetAction: "promote"}); err != nil {
		t.Fatalf("promote target action should not require --no-restart: %v", err)
	}
}

func TestValidatePgBackRestPreflightRequiresBackup(t *testing.T) {
	info := []pgbackrest.PgBackRestInfo{{
		Name:   "pg-meta",
		Status: pgbackrest.StatusInfo{Code: 0, Message: "ok"},
	}}
	err := validatePgBackRestPreflight(info, &Options{Default: true})
	if err == nil {
		t.Fatal("preflight should reject stanza with no backups")
	}
	if !strings.Contains(err.Error(), "no backups") {
		t.Fatalf("error should mention no backups, got %q", err.Error())
	}
}

func TestValidatePgBackRestPreflightChecksBackupSet(t *testing.T) {
	info := []pgbackrest.PgBackRestInfo{{
		Name:   "pg-meta",
		Status: pgbackrest.StatusInfo{Code: 0, Message: "ok"},
		Backup: []pgbackrest.BackupInfo{
			{Label: "20260629-115724F"},
		},
	}}
	err := validatePgBackRestPreflight(info, &Options{Default: true, Set: "missing"})
	if err == nil {
		t.Fatal("preflight should reject missing backup set")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error should mention missing set, got %q", err.Error())
	}
}

func TestShouldEscalateStopRequiresForceStop(t *testing.T) {
	if shouldEscalateStop(&Options{}) {
		t.Fatal("stop escalation should require explicit --force-stop")
	}
	if !shouldEscalateStop(&Options{ForceStop: true}) {
		t.Fatal("stop escalation should be allowed with --force-stop")
	}
}

func TestBuildRisksManagedPatroniSaysNotRejoined(t *testing.T) {
	risks := buildRisks(&SystemState{PatroniActive: true, DataDir: "/pg/data"}, &Options{Default: true})
	joined := strings.Join(risks, "\n")
	if !strings.Contains(joined, "not restarted or rejoined by this command") {
		t.Fatalf("managed Patroni risk should explain post-restore boundary, got %v", risks)
	}
}

func TestRestoreOptionsFromPITRSuppressesNestedHints(t *testing.T) {
	restoreOpts := restoreOptionsFromPITR(&Options{Default: true})
	if !restoreOpts.SuppressHints {
		t.Fatal("PITR should suppress nested pgBackRest post-restore hints")
	}
}

func TestRestoreOptionsFromPITRPassesTimelineAndTargetAction(t *testing.T) {
	opts := &Options{Time: "2026-01-01 00:00:00+00"}
	setPITRStringField(t, opts, "TargetTimeline", "current")
	setPITRStringField(t, opts, "TargetAction", "shutdown")

	restoreOpts := restoreOptionsFromPITR(opts)

	if got := getStringField(t, restoreOpts, "TargetTimeline"); got != "current" {
		t.Fatalf("restore target timeline = %q, want current", got)
	}
	if got := getStringField(t, restoreOpts, "TargetAction"); got != "shutdown" {
		t.Fatalf("restore target action = %q, want shutdown", got)
	}
}

func TestRestoreOptionsFromPITRPassesExtraArgs(t *testing.T) {
	opts := &Options{Default: true}
	setPITRStringSliceField(t, opts, "ExtraArgs", []string{"--delta", "--process-max=4"})

	restoreOpts := restoreOptionsFromPITR(opts)

	if got := getStringSliceField(t, restoreOpts, "ExtraArgs"); !reflect.DeepEqual(got, []string{"--delta", "--process-max=4"}) {
		t.Fatalf("restore extra args = %v, want [--delta --process-max=4]", got)
	}
}

func setPITRStringField(t *testing.T, target interface{}, fieldName, value string) {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T should expose %s", target, fieldName)
	}
	if !field.CanSet() {
		t.Fatalf("%T.%s is not settable", target, fieldName)
	}
	field.SetString(value)
}

func setPITRStringSliceField(t *testing.T, target interface{}, fieldName string, value []string) {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T should expose %s", target, fieldName)
	}
	if !field.CanSet() {
		t.Fatalf("%T.%s is not settable", target, fieldName)
	}
	field.Set(reflect.ValueOf(value))
}

func getStringField(t *testing.T, target interface{}, fieldName string) string {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T should expose %s", target, fieldName)
	}
	return field.String()
}

func getStringSliceField(t *testing.T, target interface{}, fieldName string) []string {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T should expose %s", target, fieldName)
	}
	return field.Interface().([]string)
}

func TestShouldWaitForRecoveryComplete(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
		want bool
	}{
		{name: "nil", opts: nil, want: false},
		{name: "default", opts: &Options{Default: true}, want: true},
		{name: "target-action promote", opts: &Options{Time: "2026-01-31 01:00:00", TargetAction: "promote"}, want: true},
		{name: "manual target", opts: &Options{Time: "2026-01-31 01:00:00"}, want: false},
		{name: "immediate manual", opts: &Options{Immediate: true}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWaitForRecoveryComplete(tt.opts); got != tt.want {
				t.Fatalf("shouldWaitForRecoveryComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecoveryWaitTimeoutUsesOption(t *testing.T) {
	if recoveryWaitTimeout(&Options{}) != pgRecoveryWaitTimeout {
		t.Fatalf("empty options should use default timeout")
	}
	if recoveryWaitTimeout(&Options{Timeout: 300}) != 300*time.Second {
		t.Fatalf("timeout option should be converted to seconds")
	}
	if recoveryWaitTimeout(&Options{Timeout: -1}) != pgRecoveryWaitTimeout {
		t.Fatalf("negative timeout should fall back to default")
	}
}

func TestWaitForRecoveryCompleteStopsWhenPrimary(t *testing.T) {
	oldQuery := pitrQueryRecoveryState
	oldSleep := pitrSleep
	defer func() {
		pitrQueryRecoveryState = oldQuery
		pitrSleep = oldSleep
	}()

	calls := 0
	pitrQueryRecoveryState = func(*SystemState) (bool, error) {
		calls++
		return calls < 3, nil
	}
	pitrSleep = func(time.Duration) {}

	err := waitForRecoveryComplete(&SystemState{DataDir: "/pg/data", DbSU: "postgres"}, time.Second)
	if err != nil {
		t.Fatalf("waitForRecoveryComplete returned error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("waitForRecoveryComplete made %d checks, want 3", calls)
	}
}

// TestPITRError_PostRestoreNilOnSuccess verifies postRestore returns nil on success
func TestPITRError_PostRestoreNilOnSuccess(t *testing.T) {
	opts := &Options{Default: true}
	pitrErr := postRestore(opts, false)
	if pitrErr != nil {
		t.Errorf("postRestore should return nil on success, got %v", pitrErr)
	}
}

func TestPITRError_PostRestoreWriteFailure(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close write pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	opts := &Options{Default: true}
	pitrErr := postRestore(opts, false)
	if pitrErr == nil {
		t.Fatal("postRestore should fail when stderr is unavailable")
	}
	if pitrErr.Code != output.CodePITRPostFailed {
		t.Fatalf("postRestore should return CodePITRPostFailed, got %d", pitrErr.Code)
	}
}

func TestPrintPostRestoreGuidanceUsesCustomDataDir(t *testing.T) {
	output := capturePITRStderr(t, func() {
		err := printPostRestoreGuidance(&Options{
			Default:   true,
			DataDir:   "/tmp/pig-pitr-restore",
			NoRestart: true,
		}, false)
		if err != nil {
			t.Fatalf("printPostRestoreGuidance failed: %v", err)
		}
	})

	if !strings.Contains(output, "pg_ctl -D /tmp/pig-pitr-restore -o \"-p ") {
		t.Fatalf("custom guidance should override port with pg_ctl -o, got:\n%s", output)
	}
	if !strings.Contains(output, "restored config keeps the original port") {
		t.Fatalf("custom guidance should warn that restored config keeps the original port, got:\n%s", output)
	}
	if strings.Contains(output, "pig pg start") {
		t.Fatalf("custom guidance should not suggest default pig pg start, got:\n%s", output)
	}
	if strings.Contains(output, "pg_ctl -D /tmp/pig-pitr-restore promote") {
		t.Fatalf("default custom guidance should not suggest manual promote, got:\n%s", output)
	}
}

func TestWithQuietStderrSuppressesHumanOutput(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	result := withQuietStderr(func() *output.Result {
		fmt.Fprint(os.Stderr, "human progress")
		return output.OK("ok", nil)
	})

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stderr pipe: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stderr pipe: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("quiet stderr should suppress human output, got %q", string(data))
	}
	if result == nil || !result.Success {
		t.Fatalf("quiet stderr should return wrapped result, got %+v", result)
	}
}

func TestCollectPostRestoreStateSkipsQueryWhenPostgresNotStarted(t *testing.T) {
	oldQuery := pitrQueryPostRestoreState
	defer func() {
		pitrQueryPostRestoreState = oldQuery
	}()

	pitrQueryPostRestoreState = func(*SystemState) (*PostRestoreState, error) {
		t.Fatal("post-restore SQL query should not run when PostgreSQL was not started")
		return nil, nil
	}

	post := collectPostRestoreState(&SystemState{DataDir: "/pg/data", DbSU: "postgres"}, false)
	if post == nil || !post.Queried {
		t.Fatalf("post state should be present, got %+v", post)
	}
	if post.Running {
		t.Fatalf("post state should not claim running when PostgreSQL was not started: %+v", post)
	}
	if post.InRecovery != nil {
		t.Fatalf("post state should not include recovery state without SQL evidence: %+v", post)
	}
}

func TestCollectPostRestoreStateUsesQueryWhenPostgresStarted(t *testing.T) {
	oldCheck := pitrCheckPostgresRunningAsDBSU
	oldQuery := pitrQueryPostRestoreState
	defer func() {
		pitrCheckPostgresRunningAsDBSU = oldCheck
		pitrQueryPostRestoreState = oldQuery
	}()

	pitrCheckPostgresRunningAsDBSU = func(string, string) (bool, int) {
		return true, 12345
	}
	inRecovery := false
	pitrQueryPostRestoreState = func(*SystemState) (*PostRestoreState, error) {
		return &PostRestoreState{
			Queried:    true,
			Running:    true,
			InRecovery: &inRecovery,
			CurrentLSN: "0/50001A0",
			TimelineID: "4",
		}, nil
	}

	post := collectPostRestoreState(&SystemState{DataDir: "/pg/data", DbSU: "postgres"}, true)
	if post == nil {
		t.Fatal("post state should be present")
	}
	if post.InRecovery == nil || *post.InRecovery {
		t.Fatalf("post state should report primary recovery state, got %+v", post)
	}
	if post.CurrentLSN != "0/50001A0" || post.TimelineID != "4" {
		t.Fatalf("post state should include LSN/timeline evidence, got %+v", post)
	}
}

func TestPrintPostRestoreGuidanceDefaultDoesNotSuggestManualPromote(t *testing.T) {
	output := capturePITRStderr(t, func() {
		err := printPostRestoreGuidance(&Options{Default: true}, false)
		if err != nil {
			t.Fatalf("printPostRestoreGuidance failed: %v", err)
		}
	})

	if strings.Contains(output, "pig pg promote") {
		t.Fatalf("default PITR guidance should not suggest manual promote, got:\n%s", output)
	}
}

func TestPrintPostRestoreGuidanceManagedDataDirWithTrailingSlashUsesManagedGuidance(t *testing.T) {
	output := capturePITRStderr(t, func() {
		err := printPostRestoreGuidance(&Options{Default: true, DataDir: "/pg/data/"}, false)
		if err != nil {
			t.Fatalf("printPostRestoreGuidance failed: %v", err)
		}
	})

	if !strings.Contains(output, "pig pg psql") {
		t.Fatalf("managed PGDATA guidance should use pig pg psql, got:\n%s", output)
	}
	if strings.Contains(output, "pg_ctl -D /pg/data/") {
		t.Fatalf("managed PGDATA guidance should not use custom pg_ctl path, got:\n%s", output)
	}
}

func TestPrintPostRestoreGuidanceTargetActionPromoteDoesNotSuggestManualPromote(t *testing.T) {
	output := capturePITRStderr(t, func() {
		err := printPostRestoreGuidance(&Options{
			Time:         "2026-01-31 01:00:00",
			TargetAction: "promote",
		}, false)
		if err != nil {
			t.Fatalf("printPostRestoreGuidance failed: %v", err)
		}
	})

	if strings.Contains(output, "pig pg promote") {
		t.Fatalf("target-action=promote guidance should not suggest manual promote, got:\n%s", output)
	}
}

func TestPrintPostRestoreGuidanceTargetActionShutdownUsesRecoveryShutdownGuidance(t *testing.T) {
	output := capturePITRStderr(t, func() {
		err := printPostRestoreGuidance(&Options{
			Time:         "2026-01-31 01:00:00",
			TargetAction: "shutdown",
			NoRestart:    true,
		}, false)
		if err != nil {
			t.Fatalf("printPostRestoreGuidance failed: %v", err)
		}
	})

	if strings.Contains(output, "pig pg psql") {
		t.Fatalf("shutdown target guidance should not suggest psql against a server expected to exit, got:\n%s", output)
	}
	if strings.Contains(output, "pig pg promote") {
		t.Fatalf("shutdown target guidance should not suggest manual promote, got:\n%s", output)
	}
	for _, want := range []string{"reaches the recovery target and exits", "pig pg start", "pig pg log show"} {
		if !strings.Contains(output, want) {
			t.Fatalf("shutdown target guidance should mention %q, got:\n%s", want, output)
		}
	}
}

func capturePITRStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stderr pipe: %v", err)
	}
	os.Stderr = oldStderr
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stderr pipe: %v", err)
	}
	return string(data)
}

// TestPITRError_AllCodesInRange verifies all PITR codes are in 160000-169999
func TestPITRError_AllCodesInRange(t *testing.T) {
	codes := []struct {
		name string
		code int
	}{
		{"CodePITRInvalidArgs", output.CodePITRInvalidArgs},
		{"CodePITRNoBackup", output.CodePITRNoBackup},
		{"CodePITRPrecheckFailed", output.CodePITRPrecheckFailed},
		{"CodePITRPgRunning", output.CodePITRPgRunning},
		{"CodePITRStopFailed", output.CodePITRStopFailed},
		{"CodePITRRestoreFailed", output.CodePITRRestoreFailed},
		{"CodePITRStartFailed", output.CodePITRStartFailed},
		{"CodePITRPostFailed", output.CodePITRPostFailed},
	}

	for _, c := range codes {
		t.Run(c.name, func(t *testing.T) {
			if c.code < 160000 || c.code > 169999 {
				t.Errorf("%s = %d, not in range 160000-169999", c.name, c.code)
			}
		})
	}
}
