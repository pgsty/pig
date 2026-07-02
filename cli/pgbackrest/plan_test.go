/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pb plan builders, replayable command rendering, and safety guards.
*/
package pgbackrest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pig/internal/output"
)

// writeTestConfig writes a pgbackrest.conf into a temp dir and returns its path.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pgbackrest.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

const singleStanzaConf = `[global]
repo1-path=/pg/backup
repo1-type=posix

[pg-meta]
pg1-path=/data/custom
pg1-port=5433
`

const multiStanzaConf = `[global]
repo1-path=/pg/backup

[pg-meta]
pg1-path=/data/a

[pg-test]
pg1-path=/data/b
`

// TestBuildRestorePlanResolvesEffectiveConfig verifies the plan shows the
// stanza and pg1-path-derived data directory that execution would actually
// use, not the /pg/data fallback.
func TestBuildRestorePlanResolvesEffectiveConfig(t *testing.T) {
	cfg := &Config{ConfigPath: writeTestConfig(t, singleStanzaConf)}
	plan := BuildRestorePlan(cfg, &RestoreOptions{Default: true})

	if len(plan.Affects) == 0 || plan.Affects[0].Type != "data_dir" {
		t.Fatalf("expected data_dir resource first, got %+v", plan.Affects)
	}
	if plan.Affects[0].Name != "/data/custom" {
		t.Errorf("plan data_dir = %q, want /data/custom (stanza pg1-path)", plan.Affects[0].Name)
	}
	foundStanza := false
	for _, check := range plan.Preconditions {
		if check.Name == "pgbackrest stanza" {
			foundStanza = true
			if check.Detail != "pg-meta" {
				t.Errorf("plan stanza = %q, want pg-meta", check.Detail)
			}
		}
		if check.Name == "config resolution" {
			t.Errorf("resolved plan must not carry the unresolved-config check: %+v", check)
		}
	}
	if !foundStanza {
		t.Fatalf("missing stanza precondition: %+v", plan.Preconditions)
	}
	if !strings.Contains(plan.Command, "--stanza pg-meta") {
		t.Errorf("plan command should pin the resolved stanza, got %q", plan.Command)
	}
}

// TestBuildRestorePlanUnresolvedConfigDegrades verifies plans degrade with an
// explicit unresolved marker instead of failing when the config is unreadable.
func TestBuildRestorePlanUnresolvedConfigDegrades(t *testing.T) {
	cfg := &Config{ConfigPath: filepath.Join(t.TempDir(), "missing.conf")}
	plan := BuildRestorePlan(cfg, &RestoreOptions{Default: true})

	found := false
	for _, check := range plan.Preconditions {
		if check.Name == "config resolution" && check.Status == "unresolved" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unresolved-config precondition, got %+v", plan.Preconditions)
	}
}

// TestRestorePlanExecuteNextActionIsReplayable verifies the plan's execute
// next action is a concrete command (no "..." placeholders) preserving flags.
func TestRestorePlanExecuteNextActionIsReplayable(t *testing.T) {
	cfg := &Config{ConfigPath: writeTestConfig(t, singleStanzaConf), Repo: "2"}
	opts := &RestoreOptions{Time: "2025-01-01 00:00:00+08"}
	plan := BuildRestorePlan(cfg, opts)

	if len(plan.NextActions) == 0 || !plan.NextActions[0].Required {
		t.Fatalf("expected required execute next action first, got %+v", plan.NextActions)
	}
	cmd := plan.NextActions[0].Command
	for _, want := range []string{"pig pb restore", "--stanza pg-meta", "--repo 2", "--time", "--yes"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("execute command %q missing %q", cmd, want)
		}
	}
	if strings.Contains(cmd, "...") {
		t.Errorf("execute command must not contain placeholder ellipsis: %q", cmd)
	}
}

// TestCommandBuildersPreserveGlobalFlags verifies -s/-c/-r/-U survive into
// rebuilt commands so agent replays target what the user addressed.
func TestCommandBuildersPreserveGlobalFlags(t *testing.T) {
	confPath := writeTestConfig(t, singleStanzaConf)
	cfg := &Config{ConfigPath: confPath, Stanza: "pg-test", Repo: "3", DbSU: "dbadmin"}

	restore := RestoreCommand(cfg, &RestoreOptions{Default: true}, false, true)
	for _, want := range []string{"--stanza pg-test", "--config " + confPath, "--repo 3", "--dbsu dbadmin", "--default", "--yes"} {
		if !strings.Contains(restore, want) {
			t.Errorf("RestoreCommand %q missing %q", restore, want)
		}
	}

	expire := ExpireCommand(cfg, &ExpireOptions{Set: "20250101-120000F"}, true, false)
	for _, want := range []string{"pig pb expire", "--stanza pg-test", "--set 20250101-120000F", "--plan"} {
		if !strings.Contains(expire, want) {
			t.Errorf("ExpireCommand %q missing %q", expire, want)
		}
	}

	del := DeleteCommand(cfg, false, true)
	for _, want := range []string{"pig pb delete", "--stanza pg-test", "--yes"} {
		if !strings.Contains(del, want) {
			t.Errorf("DeleteCommand %q missing %q", del, want)
		}
	}
}

// TestBuildExpirePlanEmbedsDryRunOutput verifies the structured expire plan
// carries the native dry-run output when the config resolves.
func TestBuildExpirePlanEmbedsDryRunOutput(t *testing.T) {
	orig := expireDryRun
	defer func() { expireDryRun = orig }()

	var gotStanza string
	expireDryRun = func(cfg *Config, opts *ExpireOptions) (string, error) {
		gotStanza = cfg.Stanza
		return "INFO: expire full backup set 20240101-120000F\n", nil
	}

	cfg := &Config{ConfigPath: writeTestConfig(t, singleStanzaConf)}
	plan := BuildExpirePlan(cfg, &ExpireOptions{})

	if gotStanza != "pg-meta" {
		t.Errorf("dry-run should run against the resolved stanza, got %q", gotStanza)
	}
	if !strings.Contains(plan.DryRunOutput, "20240101-120000F") {
		t.Errorf("plan.DryRunOutput missing dry-run detail: %q", plan.DryRunOutput)
	}
	found := false
	for _, check := range plan.Verifications {
		if check.Name == "dry run" && check.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ok dry-run verification, got %+v", plan.Verifications)
	}
}

// TestBuildExpirePlanDryRunUnavailable verifies the plan degrades with an
// unavailable marker when pgbackrest cannot run (e.g. binary missing).
func TestBuildExpirePlanDryRunUnavailable(t *testing.T) {
	orig := expireDryRun
	defer func() { expireDryRun = orig }()
	expireDryRun = func(cfg *Config, opts *ExpireOptions) (string, error) {
		return "", errors.New("pgbackrest not found")
	}

	cfg := &Config{ConfigPath: writeTestConfig(t, singleStanzaConf)}
	plan := BuildExpirePlan(cfg, &ExpireOptions{Set: "20250101-120000F"})

	if plan.DryRunOutput != "" {
		t.Errorf("DryRunOutput should be empty on dry-run failure, got %q", plan.DryRunOutput)
	}
	found := false
	for _, check := range plan.Verifications {
		if check.Name == "dry run" && check.Status == "unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unavailable dry-run verification, got %+v", plan.Verifications)
	}
}

// TestRequireExplicitStanza covers single/multi/explicit stanza selection.
func TestRequireExplicitStanza(t *testing.T) {
	multiPath := writeTestConfig(t, multiStanzaConf)

	stanzas, err := RequireExplicitStanza(&Config{ConfigPath: multiPath})
	if err == nil {
		t.Fatal("multi-stanza config without --stanza must be ambiguous")
	}
	if len(stanzas) != 2 || stanzas[0] != "pg-meta" || stanzas[1] != "pg-test" {
		t.Fatalf("unexpected stanza candidates: %v", stanzas)
	}

	if _, err := RequireExplicitStanza(&Config{ConfigPath: multiPath, Stanza: "pg-test"}); err != nil {
		t.Fatalf("explicit stanza must pass: %v", err)
	}

	singlePath := writeTestConfig(t, singleStanzaConf)
	if _, err := RequireExplicitStanza(&Config{ConfigPath: singlePath}); err != nil {
		t.Fatalf("single stanza must pass: %v", err)
	}

	// Config read failures defer to the normal resolution flow.
	if _, err := RequireExplicitStanza(&Config{ConfigPath: filepath.Join(t.TempDir(), "nope.conf")}); err != nil {
		t.Fatalf("unreadable config must not classify as ambiguous: %v", err)
	}
}

// TestDeleteResultRefusesAmbiguousStanza verifies stanza deletion refuses to
// auto-pick a target even with --yes, and suggests per-stanza previews that
// preserve the caller's --config/--repo/--dbsu.
func TestDeleteResultRefusesAmbiguousStanza(t *testing.T) {
	confPath := writeTestConfig(t, multiStanzaConf)
	cfg := &Config{ConfigPath: confPath, Repo: "2", DbSU: "dbadmin"}
	result := DeleteResult(cfg, &DeleteOptions{Yes: true})

	if result.Success {
		t.Fatalf("ambiguous delete must fail, got %+v", result)
	}
	if result.Code != output.CodePbAmbiguousStanza {
		t.Fatalf("code = %d, want CodePbAmbiguousStanza(%d)", result.Code, output.CodePbAmbiguousStanza)
	}
	if len(result.NextActions) != 2 {
		t.Fatalf("expected per-stanza preview next actions, got %+v", result.NextActions)
	}
	for _, action := range result.NextActions {
		if !strings.Contains(action.Command, "--stanza pg-") || !strings.Contains(action.Command, "--plan") {
			t.Errorf("next action should preview one explicit stanza: %q", action.Command)
		}
		if strings.Contains(action.Command, "--yes") {
			t.Errorf("ambiguity refusal must not suggest a --yes command: %q", action.Command)
		}
		for _, want := range []string{"--config " + confPath, "--repo 2", "--dbsu dbadmin"} {
			if !strings.Contains(action.Command, want) {
				t.Errorf("preview command %q must preserve %q", action.Command, want)
			}
		}
	}
}

// TestBuildDeletePlanAmbiguousStanzaBlocks verifies the delete plan never pins
// an auto-detected stanza when several are configured.
func TestBuildDeletePlanAmbiguousStanzaBlocks(t *testing.T) {
	cfg := &Config{ConfigPath: writeTestConfig(t, multiStanzaConf)}
	plan := BuildDeletePlan(cfg, &DeleteOptions{})

	if strings.Contains(plan.Command, "--stanza") {
		t.Errorf("ambiguous delete plan must not pin a stanza: %q", plan.Command)
	}
	for _, res := range plan.Affects {
		if res.Type == "stanza" && res.Name != "ambiguous" {
			t.Errorf("ambiguous delete plan must not name a stanza as affected: %+v", res)
		}
	}
	blocked := false
	for _, check := range plan.Preconditions {
		if check.Name == "stanza selection" && check.Status == "blocked" {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("expected blocked stanza-selection precondition, got %+v", plan.Preconditions)
	}
	for _, action := range plan.NextActions {
		if strings.Contains(action.Command, "--yes") {
			t.Errorf("ambiguous delete plan must not suggest --yes execution: %q", action.Command)
		}
	}
}

// TestBuildDeletePlanSingleStanzaPins verifies the single-stanza delete plan
// pins the resolved stanza into the replayable execute command.
func TestBuildDeletePlanSingleStanzaPins(t *testing.T) {
	cfg := &Config{ConfigPath: writeTestConfig(t, singleStanzaConf)}
	plan := BuildDeletePlan(cfg, &DeleteOptions{})

	if !strings.Contains(plan.Command, "--stanza pg-meta") {
		t.Errorf("delete plan should pin the single stanza: %q", plan.Command)
	}
	if len(plan.NextActions) == 0 {
		t.Fatalf("expected execute next action: %+v", plan.NextActions)
	}
	execCmd := plan.NextActions[0].Command
	if !strings.Contains(execCmd, "--stanza pg-meta") || !strings.Contains(execCmd, "--yes") {
		t.Fatalf("execute next action should pin stanza and carry --yes: %q", execCmd)
	}
}

// TestRestoreResultRequiresYes verifies the cli-layer confirmation re-check:
// RestoreResult must refuse without --yes even if a caller skips the cmd gate.
func TestRestoreResultRequiresYes(t *testing.T) {
	result := RestoreResult(&Config{}, &RestoreOptions{Default: true})
	if result.Success {
		t.Fatal("RestoreResult without Yes must fail")
	}
	if result.Code != output.CodePbConfirmationRequired {
		t.Fatalf("code = %d, want CodePbConfirmationRequired(%d)", result.Code, output.CodePbConfirmationRequired)
	}

	if nilResult := RestoreResult(&Config{}, nil); nilResult.Code != output.CodePbConfirmationRequired {
		t.Fatalf("nil opts must hit the confirmation gate, got %+v", nilResult)
	}
}

// pinNormalizeZone pins normalizeTimeLocation to a named zone for the test.
func pinNormalizeZone(t *testing.T, name string) {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("timezone database missing %s: %v", name, err)
	}
	orig := normalizeTimeLocation
	normalizeTimeLocation = loc
	t.Cleanup(func() { normalizeTimeLocation = orig })
}

// TestNormalizeTimeAppendsLocalTimezone verifies every timezone-less input
// form gets the local offset appended, resolved AT THE TARGET DATE (so DST
// zones do not inherit the current season's offset).
func TestNormalizeTimeAppendsLocalTimezone(t *testing.T) {
	t.Run("fixed offset zone", func(t *testing.T) {
		pinNormalizeZone(t, "Asia/Shanghai")
		tests := []struct {
			in   string
			want string
		}{
			{"2025-01-01", "2025-01-01 00:00:00+08"},
			{"2025-01-01 12:00:00", "2025-01-01 12:00:00+08"},
			{"2025-01-01T12:00:00", "2025-01-01 12:00:00+08"},
			{"2025-01-01 12:00:00+08", "2025-01-01 12:00:00+08"},
			{"2025-01-01 12:00:00+05:30", "2025-01-01 12:00:00+05:30"},
			{"", ""},
			// Timezone-bearing inputs are canonicalized to the documented
			// space-separated ±HH[:MM] spelling; the INPUT offset is
			// preserved, never shifted to the pinned local zone.
			{"2025-01-01T12:00:00+08", "2025-01-01 12:00:00+08"},
			{"2025-01-01T12:00:00+05:30", "2025-01-01 12:00:00+05:30"},
			{"2025-01-01T12:00:00-04", "2025-01-01 12:00:00-04"},
			{"2025-01-01 12:00:00Z", "2025-01-01 12:00:00+00"},
			{"2025-01-01T12:00:00Z", "2025-01-01 12:00:00+00"},
			{"2025-01-01 12:00:00+0530", "2025-01-01 12:00:00+05:30"},
		}
		for _, tt := range tests {
			got := normalizeTime(tt.in)
			if got != tt.want {
				t.Errorf("normalizeTime(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// Canonicalization must be idempotent: replayed commands feed
			// normalized values back through this path.
			if again := normalizeTime(got); again != got {
				t.Errorf("normalizeTime not idempotent: %q -> %q -> %q", tt.in, got, again)
			}
		}

		// Time-only anchors to today's date in the zone with the modern
		// offset, never a year-0 LMT artifact like +08:05.
		got := normalizeTime("12:00:00")
		if !strings.HasSuffix(got, "+08") || !strings.Contains(got, " 12:00:00") {
			t.Errorf("normalizeTime(12:00:00) = %q, want today 12:00:00+08", got)
		}
	})

	t.Run("DST zone uses target date offset", func(t *testing.T) {
		pinNormalizeZone(t, "America/New_York")
		tests := []struct {
			in   string
			want string
		}{
			{"2025-01-15 12:00:00", "2025-01-15 12:00:00-05"}, // EST in winter
			{"2025-07-15 12:00:00", "2025-07-15 12:00:00-04"}, // EDT in summer
			{"2025-01-15", "2025-01-15 00:00:00-05"},
		}
		for _, tt := range tests {
			if got := normalizeTime(tt.in); got != tt.want {
				t.Errorf("normalizeTime(%q) = %q, want %q", tt.in, got, tt.want)
			}
		}
	})

	t.Run("half hour zone", func(t *testing.T) {
		pinNormalizeZone(t, "Asia/Kolkata")
		if got := normalizeTime("2025-01-01 12:00:00"); got != "2025-01-01 12:00:00+05:30" {
			t.Errorf("normalizeTime = %q, want +05:30 suffix", got)
		}
	})
}

// TestQuoteArgShellSafety verifies replayable-command quoting survives shell
// metacharacters (globs, $-expansion, backticks) via POSIX single quoting.
func TestQuoteArgShellSafety(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"20250101-120000F", "20250101-120000F"},
		{"/etc/pgbackrest/pgbackrest.conf", "/etc/pgbackrest/pgbackrest.conf"},
		{"20250101-*", "'20250101-*'"},
		{"$HOME", "'$HOME'"},
		{"`cmd`", "'`cmd`'"},
		{"a b", "'a b'"},
		{"a;b", "'a;b'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
	}
	for _, tt := range tests {
		if got := quoteArg(tt.in); got != tt.want {
			t.Errorf("quoteArg(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRestorePlanCommandUsesNormalizedTime verifies replay determinism: the
// plan command and execute next action carry the normalized (timezone- and
// date-completed) time, not the raw relative input.
func TestRestorePlanCommandUsesNormalizedTime(t *testing.T) {
	pinNormalizeZone(t, "Asia/Shanghai")
	cfg := &Config{ConfigPath: writeTestConfig(t, singleStanzaConf)}
	plan := BuildRestorePlan(cfg, &RestoreOptions{Time: "2025-01-01"})

	want := "--time '2025-01-01 00:00:00+08'"
	if !strings.Contains(plan.Command, want) {
		t.Errorf("plan command %q missing normalized time %q", plan.Command, want)
	}
	if len(plan.NextActions) == 0 || !strings.Contains(plan.NextActions[0].Command, want) {
		t.Errorf("execute next action missing normalized time %q: %+v", want, plan.NextActions)
	}
}

// TestValidateRestoreExtraArgsBlocksConfigOverrides verifies the passthrough
// blacklist covers pig-owned selection flags, every spelling of the data
// directory option, and recovery_target overrides via --recovery-option.
func TestValidateRestoreExtraArgsBlocksConfigOverrides(t *testing.T) {
	blockedArgs := []string{
		"--stanza=other", "--config=/etc/other.conf", "--repo=2", "--stanza",
		"--config-path=/etc/other", "--config-include-path=/etc/other.d",
		"--pg1-path=/unsafe", "--pg-path=/unsafe", "--pg2-path=/unsafe",
		"--db-path=/unsafe", "--db1-path=/unsafe",
		// the entire --repo[N]-* family redefines the backup source
		"--repo-path=/other/repo", "--repo1-path=/other/repo",
		"--repo-host=evil", "--repo1-host-user=evil",
		"--repo1-type=s3", "--repo1-s3-bucket=evil", "--repo1-s3-endpoint=evil",
		"--repo1-sftp-host=evil", "--repo1-azure-container=evil", "--repo1-gcs-bucket=evil",
		"--repo1-cipher-pass=leak",
		"--recovery-option=recovery_target_time=2020-01-01",
	}
	for _, blocked := range blockedArgs {
		if err := ValidateRestoreExtraArgs([]string{blocked}); err == nil {
			t.Errorf("extra arg %q must be rejected", blocked)
		}
	}
	// Relocation escape hatches stay allowed: they neither move the declared
	// PGDATA nor change the backup source.
	if err := ValidateRestoreExtraArgs([]string{"--delta", "--process-max=4", "--tablespace-map=ts1=/mnt/x", "--link-all"}); err != nil {
		t.Errorf("benign extra args should pass: %v", err)
	}
}

// TestParseRestoredBackupSet verifies label extraction from restore output.
func TestParseRestoredBackupSet(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{"INFO: restore backup set 20250101-120000F", "20250101-120000F"},
		{"P00   INFO: restore backup set 20250101-120000F_20250102-130000I, recovery will start", "20250101-120000F_20250102-130000I"},
		{"no such line here", ""},
	}
	for _, tt := range tests {
		if got := parseRestoredBackupSet(tt.output); got != tt.want {
			t.Errorf("parseRestoredBackupSet(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

// TestEnsureConsoleInfoLog verifies the INFO console level is appended only
// when the caller has not already set one (exact option match).
func TestEnsureConsoleInfoLog(t *testing.T) {
	got := ensureConsoleInfoLog([]string{"--pg1-path=/pg/data"})
	if got[len(got)-1] != "--log-level-console=info" {
		t.Errorf("expected appended console info level, got %v", got)
	}
	pre := []string{"--pg1-path=/pg/data", "--log-level-console=warn"}
	if got := ensureConsoleInfoLog(pre); len(got) != len(pre) {
		t.Errorf("caller-provided console level must not be duplicated: %v", got)
	}
	// A same-prefix but different option must not suppress the injection.
	odd := []string{"--log-level-console-custom=x"}
	if got := ensureConsoleInfoLog(odd); got[len(got)-1] != "--log-level-console=info" {
		t.Errorf("prefix lookalike must not suppress injection: %v", got)
	}
}

// TestRestoreNextActionsMirrorTextHints verifies the structured next actions
// follow the same branching as printPostRestoreHints.
func TestRestoreNextActionsMirrorTextHints(t *testing.T) {
	// Default target on managed dir: start + verify + recreate, no promote.
	actions := restoreNextActions(&Config{}, &RestoreOptions{Default: true})
	joined := joinActionCommands(actions)
	if !strings.Contains(joined, "pig pg start") || !strings.Contains(joined, "pig pb create") {
		t.Errorf("default-dir actions missing start/create: %q", joined)
	}
	if strings.Contains(joined, "promote") {
		t.Errorf("--default restore must not suggest promote: %q", joined)
	}
	if !actions[0].Required {
		t.Errorf("starting PostgreSQL should be the required next action: %+v", actions[0])
	}

	// Time target without target-action: manual promote suggested.
	actions = restoreNextActions(&Config{}, &RestoreOptions{Time: "2025-01-01"})
	if !strings.Contains(joinActionCommands(actions), "pig pg promote") {
		t.Errorf("time-target restore should suggest manual promote: %+v", actions)
	}

	// Promote target-action: no manual promote needed.
	actions = restoreNextActions(&Config{}, &RestoreOptions{Time: "2025-01-01", TargetAction: "promote"})
	if strings.Contains(joinActionCommands(actions), "promote to primary") {
		t.Errorf("target-action=promote must not suggest manual promote: %+v", actions)
	}

	// Custom data dir: pg_ctl commands instead of pig pg.
	actions = restoreNextActions(&Config{}, &RestoreOptions{Time: "2025-01-01", DataDir: "/data/side"})
	joined = joinActionCommands(actions)
	if !strings.Contains(joined, "pg_ctl -D /data/side start") || !strings.Contains(joined, "pg_ctl -D /data/side promote") {
		t.Errorf("custom-dir actions should use pg_ctl with the side directory: %q", joined)
	}
}

func joinActionCommands(actions []output.NextAction) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, action.Command)
	}
	return strings.Join(parts, " | ")
}

// TestBackupRolePostgresConfigDerivesFromStanza verifies the role probe
// targets the stanza's pg1-path and the pb-level DBSU.
func TestBackupRolePostgresConfigDerivesFromStanza(t *testing.T) {
	cfg := &Config{
		ConfigPath: writeTestConfig(t, singleStanzaConf),
		Stanza:     "pg-meta",
		DbSU:       "dbadmin",
	}
	pgConfig := backupRolePostgresConfig(cfg)
	if pgConfig.PgData != "/data/custom" {
		t.Errorf("PgData = %q, want /data/custom from stanza pg1-path", pgConfig.PgData)
	}
	if pgConfig.DbSU != "dbadmin" {
		t.Errorf("DbSU = %q, want dbadmin", pgConfig.DbSU)
	}

	if fallback := backupRolePostgresConfig(nil); fallback == nil || fallback.PgData == "" {
		t.Errorf("nil config must fall back to postgres defaults, got %+v", fallback)
	}
}
