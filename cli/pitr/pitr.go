/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Package pitr provides orchestrated Point-In-Time Recovery functionality.
It coordinates Patroni, PostgreSQL, and pgBackRest to perform PITR safely.
*/
package pitr

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pig/cli/patroni"
	"pig/cli/pgbackrest"
	"pig/cli/postgres"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ============================================================================
// Options
// ============================================================================

// Options holds all options for PITR command
type Options struct {
	// Recovery targets (exactly one required)
	Default   bool   // Recover to end of WAL stream (latest)
	Immediate bool   // Recover to backup consistency point
	Time      string // Recover to specific timestamp
	Name      string // Recover to named restore point
	LSN       string // Recover to specific LSN
	XID       string // Recover to specific transaction ID

	// Backup selection
	Set string // Recover from specific backup set

	// PITR control
	SkipPatroni bool // Skip Patroni operations
	NoRestart   bool // Don't restart PostgreSQL after restore
	Plan        bool // Show plan only, don't execute
	DryRun      bool // Show plan only, don't execute
	Yes         bool // Skip confirmations

	// Common (inherited from pgbackrest)
	Stanza     string // pgBackRest stanza name
	ConfigPath string // pgBackRest config file path
	Repo       string // Repository number
	DbSU       string // Database superuser
	DataDir    string // Target data directory
	Exclusive  bool   // Stop before target (exclusive)
	Promote    bool   // Auto-promote after recovery
}

// ============================================================================
// System State
// ============================================================================

// SystemState holds the current system state before PITR
type SystemState struct {
	PatroniActive bool // Patroni service is active
	PGRunning     bool // PostgreSQL is running
	PGPID         int  // PostgreSQL PID (if running)
	DataDir       string
	DbSU          string
}

// ============================================================================
// Constants
// ============================================================================

const (
	// Stop retry configuration
	maxStopRetries    = 3
	initialRetryDelay = 2 * time.Second

	// Wait for PG to stop after Patroni stops
	pgStopWaitTime   = 5 * time.Second
	pgStopCheckCount = 6 // Check 6 times (total 30 seconds)
)

// PITRError represents a typed error with a semantic PITR error code.
type PITRError struct {
	Code int
	Err  error
}

func (e *PITRError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "pitr error"
}

func (e *PITRError) Unwrap() error {
	return e.Err
}

// ============================================================================
// Main Entry Point
// ============================================================================

// Execute performs the PITR workflow
func Execute(opts *Options) error {
	// Phase 1: Pre-check and validation
	state, err := preCheck(opts)
	if err != nil {
		if pe, ok := err.(*PITRError); ok {
			return &utils.ExitCodeError{Code: output.ExitCode(pe.Code), Err: pe}
		}
		return err
	}

	// Build and show execution plan (text only)
	printExecutionPlan(state, opts)

	// Confirm with countdown (unless --yes)
	if !opts.Yes {
		if err := pgbackrest.ConfirmWithCountdown("This will overwrite the current database!", "PITR"); err != nil {
			return &utils.ExitCodeError{Code: output.ExitCode(output.CodePITRInvalidArgs), Err: &PITRError{Code: output.CodePITRInvalidArgs, Err: err}}
		}
	}

	// Phase 2: Stop Patroni (if active)
	patroniWasStopped := false
	if state.PatroniActive && !opts.SkipPatroni {
		if pitrErr := stopPatroni(); pitrErr != nil {
			return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
		}
		patroniWasStopped = true
	}

	// Phase 3: Ensure PostgreSQL is stopped
	if pitrErr := ensurePostgresStopped(state, patroniWasStopped); pitrErr != nil {
		return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
	}

	// Phase 4: Execute pgBackRest restore
	if pitrErr := executeRestore(state, opts); pitrErr != nil {
		return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
	}

	// Phase 5: Start PostgreSQL (unless --no-restart)
	if !opts.NoRestart {
		if pitrErr := startPostgres(state, opts); pitrErr != nil {
			return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
		}
	}

	// Phase 6: Post-restore guidance
	if pitrErr := postRestore(opts, patroniWasStopped); pitrErr != nil {
		return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
	}

	return nil
}

// ExecuteResult performs the PITR workflow and returns a structured Result.
func ExecuteResult(opts *Options) *output.Result {
	startTime := time.Now()

	state, err := preCheck(opts)
	if err != nil {
		if pe, ok := err.(*PITRError); ok {
			return output.Fail(pe.Code, pe.Error())
		}
		return output.Fail(output.CodePITRPrecheckFailed, err.Error())
	}

	// Confirm with countdown (unless --yes)
	if !opts.Yes {
		if err := pgbackrest.ConfirmWithCountdown("This will overwrite the current database!", "PITR"); err != nil {
			return output.Fail(output.CodePITRInvalidArgs, "pitr confirmation cancelled").WithDetail(err.Error())
		}
	}

	patroniWasStopped := false
	if state.PatroniActive && !opts.SkipPatroni {
		if pitrErr := stopPatroni(); pitrErr != nil {
			return output.Fail(pitrErr.Code, pitrErr.Error())
		}
		patroniWasStopped = true
	}

	if pitrErr := ensurePostgresStopped(state, patroniWasStopped); pitrErr != nil {
		return output.Fail(pitrErr.Code, pitrErr.Error())
	}

	if pitrErr := executeRestore(state, opts); pitrErr != nil {
		return output.Fail(pitrErr.Code, pitrErr.Error())
	}

	postgresStarted := false
	if !opts.NoRestart {
		if pitrErr := startPostgres(state, opts); pitrErr != nil {
			return output.Fail(pitrErr.Code, pitrErr.Error())
		}
		postgresStarted = true
	}

	// Post-restore steps
	if pitrErr := postRestore(opts, patroniWasStopped); pitrErr != nil {
		return output.Fail(pitrErr.Code, pitrErr.Error())
	}

	endTime := time.Now()
	data := newPITRResultData(state, opts, patroniWasStopped, postgresStarted, startTime, endTime)
	return output.OK("pitr completed", data)
}

// ============================================================================
// Phase 1: Pre-Check
// ============================================================================

func preCheck(opts *Options) (*SystemState, error) {
	// Validate recovery target
	if err := validateRecoveryTarget(opts); err != nil {
		return nil, &PITRError{Code: output.CodePITRInvalidArgs, Err: err}
	}

	// Determine DBSU and data directory
	dbsu := utils.GetDBSU(opts.DbSU)
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = postgres.DefaultPgData
	}

	// Check data directory exists and is initialized
	exists, initialized := postgres.CheckDataDirAsDBSU(dbsu, dataDir)
	if !exists {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: fmt.Errorf("data directory %s does not exist", dataDir)}
	}
	if !initialized {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: fmt.Errorf("data directory %s is not initialized (no PG_VERSION)", dataDir)}
	}

	// Check current state
	patroniActive := utils.IsServiceActive("patroni")
	pgRunning, pgPID := postgres.CheckPostgresRunningAsDBSU(dbsu, dataDir)

	state := &SystemState{
		PatroniActive: patroniActive,
		PGRunning:     pgRunning,
		PGPID:         pgPID,
		DataDir:       dataDir,
		DbSU:          dbsu,
	}

	return state, nil
}

func validateRecoveryTarget(opts *Options) error {
	targets := 0
	if opts.Default {
		targets++
	}
	if opts.Immediate {
		targets++
	}
	if opts.Time != "" {
		targets++
	}
	if opts.Name != "" {
		targets++
	}
	if opts.LSN != "" {
		targets++
	}
	if opts.XID != "" {
		targets++
	}

	if targets == 0 {
		return fmt.Errorf("no recovery target specified, use one of: --default, --immediate, --time, --name, --lsn, --xid")
	}
	if targets > 1 {
		return fmt.Errorf("multiple recovery targets specified, choose only one")
	}
	return nil
}

// ============================================================================
// Execution Plan Display
// ============================================================================

func printExecutionPlan(state *SystemState, opts *Options) {
	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s PITR Execution Plan%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)

	// Current state
	fmt.Fprintf(os.Stderr, "\n%sCurrent State:%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "  Data Directory:  %s\n", state.DataDir)
	fmt.Fprintf(os.Stderr, "  Database User:   %s\n", state.DbSU)

	if state.PatroniActive {
		fmt.Fprintf(os.Stderr, "  Patroni Service: %sactive%s\n", utils.ColorGreen, utils.ColorReset)
	} else {
		fmt.Fprintf(os.Stderr, "  Patroni Service: %sinactive%s\n", utils.ColorYellow, utils.ColorReset)
	}

	if state.PGRunning {
		fmt.Fprintf(os.Stderr, "  PostgreSQL:      %srunning%s (PID: %d)\n", utils.ColorGreen, utils.ColorReset, state.PGPID)
	} else {
		fmt.Fprintf(os.Stderr, "  PostgreSQL:      %sstopped%s\n", utils.ColorYellow, utils.ColorReset)
	}

	// Recovery target
	fmt.Fprintf(os.Stderr, "\n%sRecovery Target:%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "  %s\n", getTargetDescription(opts))
	if opts.Set != "" {
		fmt.Fprintf(os.Stderr, "  Backup Set: %s\n", opts.Set)
	}
	if opts.Exclusive {
		fmt.Fprintf(os.Stderr, "  Mode: exclusive (stop before target)\n")
	}
	if opts.Promote {
		fmt.Fprintf(os.Stderr, "  Auto-promote: yes\n")
	}

	// Execution steps
	fmt.Fprintf(os.Stderr, "\n%sExecution Steps:%s\n", utils.ColorBold, utils.ColorReset)
	step := 1

	if state.PatroniActive && !opts.SkipPatroni {
		fmt.Fprintf(os.Stderr, "  [%d] Stop Patroni service\n", step)
		step++
	} else if opts.SkipPatroni {
		fmt.Fprintf(os.Stderr, "  [-] Skip Patroni (--skip-patroni)\n")
	}

	if state.PGRunning || state.PatroniActive {
		fmt.Fprintf(os.Stderr, "  [%d] Ensure PostgreSQL is stopped\n", step)
		step++
	}

	fmt.Fprintf(os.Stderr, "  [%d] Execute pgBackRest restore\n", step)
	step++

	if !opts.NoRestart {
		fmt.Fprintf(os.Stderr, "  [%d] Start PostgreSQL\n", step)
		step++
	} else {
		fmt.Fprintf(os.Stderr, "  [-] Skip PostgreSQL start (--no-restart)\n")
	}

	fmt.Fprintf(os.Stderr, "  [%d] Print post-restore guidance\n", step)

	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
}

func getTargetDescription(opts *Options) string {
	if opts.Default {
		return "Latest (end of WAL stream)"
	}
	if opts.Immediate {
		return "Backup consistency point"
	}
	if opts.Time != "" {
		return fmt.Sprintf("Time: %s", opts.Time)
	}
	if opts.Name != "" {
		return fmt.Sprintf("Restore point: %s", opts.Name)
	}
	if opts.LSN != "" {
		return fmt.Sprintf("LSN: %s", opts.LSN)
	}
	if opts.XID != "" {
		return fmt.Sprintf("XID: %s", opts.XID)
	}
	return "Unknown"
}

func isPlanMode(opts *Options) bool {
	if opts == nil {
		return false
	}
	return opts.Plan || opts.DryRun
}

// Plan builds a plan with pre-check validation for CLI plan mode.
func Plan(opts *Options) (*output.Plan, error) {
	state, err := preCheck(opts)
	if err != nil {
		return nil, err
	}
	return BuildPlan(state, opts), nil
}

// BuildPlan constructs a structured execution plan for PITR.
// The plan content must remain consistent with actual execution steps (NFR9).
func BuildPlan(state *SystemState, opts *Options) *output.Plan {
	actions := buildActions(state, opts)
	affects := buildAffects(state, opts)
	expected := buildExpected(state, opts)
	risks := buildRisks(state, opts)

	return &output.Plan{
		Command:  buildCommand(opts),
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildActions(state *SystemState, opts *Options) []output.Action {
	if state == nil || opts == nil {
		return nil
	}
	actions := []output.Action{}
	step := 1

	if state.PatroniActive && !opts.SkipPatroni {
		actions = append(actions, output.Action{Step: step, Description: "Stop Patroni service"})
		step++
	}

	if state.PGRunning || state.PatroniActive {
		actions = append(actions, output.Action{Step: step, Description: "Ensure PostgreSQL is stopped"})
		step++
	}

	actions = append(actions, output.Action{Step: step, Description: "Execute pgBackRest restore"})
	step++

	if !opts.NoRestart {
		actions = append(actions, output.Action{Step: step, Description: "Start PostgreSQL"})
		step++
	}

	actions = append(actions, output.Action{Step: step, Description: "Print post-restore guidance"})

	return actions
}

func buildAffects(state *SystemState, opts *Options) []output.Resource {
	if state == nil || opts == nil {
		return nil
	}
	affects := []output.Resource{}

	if state.PatroniActive && !opts.SkipPatroni {
		affects = append(affects, output.Resource{
			Type:   "service",
			Name:   "patroni",
			Impact: "stop",
			Detail: "cluster management paused",
		})
	}

	if state.PGRunning || state.PatroniActive {
		affects = append(affects, output.Resource{
			Type:   "service",
			Name:   "postgresql",
			Impact: "stop",
		})
	}

	backupSet := "latest"
	if opts.Set != "" {
		backupSet = opts.Set
	}
	affects = append(affects, output.Resource{
		Type:   "backup",
		Name:   backupSet,
		Impact: "restore",
		Detail: "pgBackRest",
	})

	target := getTargetDescription(opts)
	if target != "" {
		affects = append(affects, output.Resource{
			Type:   "target",
			Name:   target,
			Impact: "recovery",
		})
	}

	affects = append(affects, output.Resource{
		Type:   "data",
		Name:   state.DataDir,
		Impact: "overwrite",
		Detail: "data directory restored",
	})

	return affects
}

func buildExpected(state *SystemState, opts *Options) string {
	if state == nil || opts == nil {
		return ""
	}
	target := getTargetDescription(opts)
	expected := fmt.Sprintf("PostgreSQL restored to %s (data dir: %s)", target, state.DataDir)
	if opts.NoRestart {
		expected = expected + "; PostgreSQL remains stopped"
	}
	if opts.Promote {
		expected = expected + "; auto-promote enabled"
	}
	return expected
}

func buildRisks(state *SystemState, opts *Options) []string {
	if state == nil || opts == nil {
		return nil
	}
	risks := []string{
		"Current data directory will be overwritten",
	}

	if state.PatroniActive && !opts.SkipPatroni {
		risks = append(risks, "Patroni will be stopped; HA management suspended")
	}
	if opts.SkipPatroni {
		risks = append(risks, "Patroni is not stopped; ensure cluster safety before restoring")
	}
	if opts.NoRestart {
		risks = append(risks, "PostgreSQL will remain stopped after restore")
	}
	if opts.Exclusive {
		risks = append(risks, "Exclusive recovery stops before target; data beyond target not applied")
	}
	return risks
}

func buildCommand(opts *Options) string {
	if opts == nil {
		return "pig pitr"
	}
	args := []string{"pig", "pitr"}

	switch {
	case opts.Default:
		args = append(args, "-d")
	case opts.Immediate:
		args = append(args, "-I")
	case opts.Time != "":
		args = append(args, "-t", quoteIfNeeded(opts.Time))
	case opts.Name != "":
		args = append(args, "-n", opts.Name)
	case opts.LSN != "":
		args = append(args, "-l", opts.LSN)
	case opts.XID != "":
		args = append(args, "-x", opts.XID)
	}

	if opts.Set != "" {
		args = append(args, "-b", opts.Set)
	}
	if opts.SkipPatroni {
		args = append(args, "--skip-patroni")
	}
	if opts.NoRestart {
		args = append(args, "--no-restart")
	}
	if opts.Exclusive {
		args = append(args, "-X")
	}
	if opts.Promote {
		args = append(args, "-P")
	}
	if opts.Plan || opts.DryRun {
		args = append(args, "--plan")
	}

	return strings.Join(args, " ")
}

func quoteIfNeeded(value string) string {
	if strings.ContainsAny(value, " \t") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

// ============================================================================
// Phase 2: Stop Patroni
// ============================================================================

func stopPatroni() *PITRError {
	fmt.Fprintf(os.Stderr, "\n%s=== Stopping Patroni Service ===%s\n", utils.ColorBold, utils.ColorReset)

	if err := patroni.Systemctl("stop"); err != nil {
		return &PITRError{Code: output.CodePITRStopFailed, Err: fmt.Errorf("failed to stop patroni service: %w", err)}
	}

	fmt.Fprintf(os.Stderr, "%sPatroni service stopped.%s\n", utils.ColorGreen, utils.ColorReset)
	return nil
}

// ============================================================================
// Phase 3: Ensure PostgreSQL Stopped
// ============================================================================

func ensurePostgresStopped(state *SystemState, patroniWasStopped bool) *PITRError {
	fmt.Fprintf(os.Stderr, "\n%s=== Ensuring PostgreSQL is Stopped ===%s\n", utils.ColorBold, utils.ColorReset)

	// If Patroni was actually stopped, wait a bit for PG to stop automatically
	if patroniWasStopped {
		fmt.Fprintf(os.Stderr, "Waiting for PostgreSQL to stop (Patroni shutdown)...\n")
		for i := 0; i < pgStopCheckCount; i++ {
			time.Sleep(pgStopWaitTime)
			running, _ := postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
			if !running {
				fmt.Fprintf(os.Stderr, "%sPostgreSQL stopped automatically.%s\n", utils.ColorGreen, utils.ColorReset)
				return nil
			}
			fmt.Fprintf(os.Stderr, "  Still running, waiting... (%d/%d)\n", i+1, pgStopCheckCount)
		}
		fmt.Fprintf(os.Stderr, "PostgreSQL did not stop automatically, proceeding to stop manually.\n")
	}

	// Check if already stopped
	running, pid := postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
	if !running {
		fmt.Fprintf(os.Stderr, "%sPostgreSQL is not running.%s\n", utils.ColorGreen, utils.ColorReset)
		return nil
	}

	// Try pg_ctl stop with exponential backoff
	fmt.Fprintf(os.Stderr, "Stopping PostgreSQL (PID: %d)...\n", pid)

	pgConfig := &postgres.Config{
		PgData: state.DataDir,
		DbSU:   state.DbSU,
	}

	retryDelay := initialRetryDelay
	for attempt := 1; attempt <= maxStopRetries; attempt++ {
		stopOpts := &postgres.StopOptions{
			Mode:    "fast",
			Timeout: 30,
		}

		logrus.Debugf("Stop attempt %d/%d with mode=fast", attempt, maxStopRetries)
		err := postgres.Stop(pgConfig, stopOpts)
		if err == nil {
			// Verify stopped
			running, _ := postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
			if !running {
				fmt.Fprintf(os.Stderr, "%sPostgreSQL stopped successfully.%s\n", utils.ColorGreen, utils.ColorReset)
				return nil
			}
		}

		if attempt < maxStopRetries {
			fmt.Fprintf(os.Stderr, "  Stop attempt %d failed, retrying in %v...\n", attempt, retryDelay)
			time.Sleep(retryDelay)
			retryDelay *= 2 // Exponential backoff
		}
	}

	// All retries failed, try immediate mode
	fmt.Fprintf(os.Stderr, "%sGraceful stop failed, trying immediate mode...%s\n", utils.ColorYellow, utils.ColorReset)
	stopOpts := &postgres.StopOptions{
		Mode:    "immediate",
		Timeout: 30,
	}
	if err := postgres.Stop(pgConfig, stopOpts); err == nil {
		running, _ := postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
		if !running {
			fmt.Fprintf(os.Stderr, "%sPostgreSQL stopped (immediate mode).%s\n", utils.ColorGreen, utils.ColorReset)
			return nil
		}
	}

	// Last resort: kill -9
	fmt.Fprintf(os.Stderr, "%sImmediate mode failed, using kill -9...%s\n", utils.ColorRed, utils.ColorReset)
	running, pid = postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
	if running && pid > 0 {
		if err := killProcess(state.DbSU, pid); err != nil {
			return &PITRError{Code: output.CodePITRStopFailed, Err: fmt.Errorf("failed to kill PostgreSQL process (PID: %d): %w", pid, err)}
		}

		// Wait a moment and verify
		time.Sleep(2 * time.Second)
		running, _ = postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
		if running {
			return &PITRError{Code: output.CodePITRPgRunning, Err: fmt.Errorf("postgresql still running after kill -9, manual intervention required")}
		}
		fmt.Fprintf(os.Stderr, "%sPostgreSQL killed (SIGKILL).%s\n", utils.ColorYellow, utils.ColorReset)
	}

	return nil
}

// killProcess sends SIGKILL to a process as DBSU
func killProcess(dbsu string, pid int) error {
	args := []string{"kill", "-9", strconv.Itoa(pid)}
	utils.PrintHint(args)
	return utils.DBSUCommand(dbsu, args)
}

// ============================================================================
// Phase 4: Execute Restore
// ============================================================================

func executeRestore(state *SystemState, opts *Options) *PITRError {
	fmt.Fprintf(os.Stderr, "\n%s=== Executing pgBackRest Restore ===%s\n", utils.ColorBold, utils.ColorReset)

	// Build pgbackrest config
	pbConfig := pgbackrest.DefaultConfig()
	if opts.Stanza != "" {
		pbConfig.Stanza = opts.Stanza
	}
	if opts.ConfigPath != "" {
		pbConfig.ConfigPath = opts.ConfigPath
	}
	if opts.Repo != "" {
		pbConfig.Repo = opts.Repo
	}
	pbConfig.DbSU = state.DbSU

	// Build restore options
	restoreOpts := &pgbackrest.RestoreOptions{
		Default:   opts.Default,
		Immediate: opts.Immediate,
		Time:      opts.Time,
		Name:      opts.Name,
		LSN:       opts.LSN,
		XID:       opts.XID,
		Set:       opts.Set,
		DataDir:   opts.DataDir,
		Exclusive: opts.Exclusive,
		Promote:   opts.Promote,
		Yes:       true, // Skip confirmation (already confirmed in PITR)
	}

	// Execute restore
	if err := pgbackrest.Restore(pbConfig, restoreOpts); err != nil {
		fmt.Fprintf(os.Stderr, "\n%sERROR: pgBackRest restore failed: %v%s\n", utils.ColorRed, err, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "\nCheck pgBackRest logs:\n")
		fmt.Fprintf(os.Stderr, "  pig pb log cat\n")
		fmt.Fprintf(os.Stderr, "  tail -100 /var/log/pgbackrest/*.log\n")
		code := classifyRestoreError(err)
		if code == output.CodePITRNoBackup {
			return &PITRError{Code: code, Err: fmt.Errorf("backup not found: %w", err)}
		}
		return &PITRError{Code: code, Err: fmt.Errorf("pgbackrest restore failed: %w", err)}
	}

	fmt.Fprintf(os.Stderr, "%spgBackRest restore completed successfully.%s\n", utils.ColorGreen, utils.ColorReset)
	return nil
}

func classifyRestoreError(err error) int {
	if err == nil {
		return output.CodePITRRestoreFailed
	}
	if isNoBackupError(err.Error()) {
		return output.CodePITRNoBackup
	}
	return output.CodePITRRestoreFailed
}

func isNoBackupError(message string) bool {
	msg := strings.ToLower(message)

	if strings.Contains(msg, "no prior backup exists") {
		return true
	}
	if strings.Contains(msg, "unable to find backup") {
		return true
	}
	if strings.Contains(msg, "no backup set") {
		return true
	}
	if strings.Contains(msg, "backup set") &&
		(strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")) {
		return true
	}
	if strings.Contains(msg, "backup") &&
		(strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")) {
		return true
	}

	return false
}

// ============================================================================
// Phase 5: Start PostgreSQL
// ============================================================================

func startPostgres(state *SystemState, opts *Options) *PITRError {
	fmt.Fprintf(os.Stderr, "\n%s=== Starting PostgreSQL ===%s\n", utils.ColorBold, utils.ColorReset)

	pgConfig := &postgres.Config{
		PgData: state.DataDir,
		DbSU:   state.DbSU,
	}

	startOpts := &postgres.StartOptions{
		Timeout: 120, // Recovery may take time
	}

	if err := postgres.Start(pgConfig, startOpts); err != nil {
		fmt.Fprintf(os.Stderr, "\n%sERROR: Failed to start PostgreSQL: %v%s\n", utils.ColorRed, err, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "\nCheck PostgreSQL logs:\n")
		fmt.Fprintf(os.Stderr, "  pig pg log cat\n")
		fmt.Fprintf(os.Stderr, "  tail -100 /pg/log/postgres/*.csv\n")
		return &PITRError{Code: output.CodePITRStartFailed, Err: fmt.Errorf("failed to start postgresql: %w", err)}
	}

	// Verify running
	running, pid := postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
	if !running {
		return &PITRError{Code: output.CodePITRStartFailed, Err: fmt.Errorf("postgresql failed to start after restore")}
	}

	fmt.Fprintf(os.Stderr, "%sPostgreSQL started successfully (PID: %d).%s\n", utils.ColorGreen, pid, utils.ColorReset)
	return nil
}

// ============================================================================
// Phase 6: Post-Restore Guidance
// ============================================================================

// postRestore performs post-restore steps and returns *PITRError on failure.
// Currently the post-restore phase only prints guidance, but this wrapper
// ensures CodePITRPostFailed is properly used if any step fails.
func postRestore(opts *Options, patroniWasStopped bool) *PITRError {
	if err := printPostRestoreGuidance(opts, patroniWasStopped); err != nil {
		return &PITRError{Code: output.CodePITRPostFailed, Err: fmt.Errorf("failed to write post-restore guidance: %w", err)}
	}
	return nil
}

func printPostRestoreGuidance(opts *Options, patroniWasStopped bool) error {
	// Fail fast if stderr is unavailable so post-restore errors can be surfaced.
	if _, err := fmt.Fprint(os.Stderr, ""); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s PITR Complete%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)

	step := 1

	if opts.NoRestart {
		fmt.Fprintf(os.Stderr, "\n%s[%d] Start PostgreSQL:%s\n", utils.ColorBold, step, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "   pig pg start\n")
		step++
	}

	fmt.Fprintf(os.Stderr, "\n%s[%d] Verify recovered data:%s\n", utils.ColorBold, step, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "   pig pg psql\n")
	step++

	if !opts.Promote {
		fmt.Fprintf(os.Stderr, "\n%s[%d] If satisfied, promote to primary:%s\n", utils.ColorBold, step, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "   pig pg promote\n")
		step++
	}

	if patroniWasStopped {
		fmt.Fprintf(os.Stderr, "\n%s[%d] To resume Patroni cluster management:%s\n", utils.ColorBold, step, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "   %sWARNING: Ensure data is correct before starting Patroni!%s\n", utils.ColorYellow, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "   systemctl start patroni\n")
		fmt.Fprintf(os.Stderr, "\n   Or if you want this node to be the leader:\n")
		fmt.Fprintf(os.Stderr, "   1. Promote PostgreSQL first: pig pg promote\n")
		fmt.Fprintf(os.Stderr, "   2. Then start Patroni: systemctl start patroni\n")
		step++
	}

	fmt.Fprintf(os.Stderr, "\n%s[%d] Re-create pgBackRest stanza if needed:%s\n", utils.ColorBold, step, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "   pig pb create\n")

	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	return nil
}
