/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Package pitr provides orchestrated Point-In-Time Recovery functionality.
It coordinates Patroni, PostgreSQL, and pgBackRest to perform PITR safely.
*/
package pitr

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pig/cli/ext"
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
	NoRestart bool // Don't restart PostgreSQL after restore
	Plan      bool // Show plan only, don't execute
	Yes       bool // Skip confirmations
	Quiet     bool // Suppress human progress logs
	Timeout   int  // PostgreSQL start/recovery timeout in seconds

	// Common (inherited from pgbackrest)
	Stanza         string // pgBackRest stanza name
	ConfigPath     string // pgBackRest config file path
	Repo           string // Repository number
	DbSU           string // Database superuser
	DataDir        string // Target data directory
	Exclusive      bool   // Stop before target (exclusive)
	TargetAction   string // Action at target: pause, promote, shutdown
	TargetTimeline string // Timeline to recover along: latest, current, N, or 0xN
	ExtraArgs      []string
	ForceStop      bool // Allow immediate stop and kill fallback if fast stop fails
}

// ============================================================================
// System State
// ============================================================================

// SystemState holds the current system state before PITR
type SystemState struct {
	PatroniActive  bool // Patroni service is active
	SideRestore    bool // Restore targets an explicit custom data directory
	PGRunning      bool // PostgreSQL is running
	PGPID          int  // PostgreSQL PID (if running)
	DataDir        string
	ManagedDataDir string
	DbSU           string
	PBConfig       *pgbackrest.Config
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

	// Wait for crash/archive recovery to finish after pg_ctl reports startup.
	pgRecoveryWaitTime    = 2 * time.Second
	pgRecoveryWaitTimeout = 120 * time.Second
)

var (
	pitrDataDirOwnerAsDBSU         = dataDirOwnerAsDBSU
	pitrDataDirEmptyAsDBSU         = dataDirEmptyAsDBSU
	pitrCheckDataDirAsDBSU         = postgres.CheckDataDirAsDBSU
	pitrCheckPostgresRunningAsDBSU = postgres.CheckPostgresRunningAsDBSU
	pitrDBSUCommandOutput          = utils.DBSUCommandOutput
	pitrLoadPgBackRestInfo         = pgbackrest.LoadInfo
	pitrQueryRecoveryState         = queryRecoveryState
	pitrQueryPostRestoreState      = queryPostRestoreState
	pitrResolvePathAsDBSU          = resolvePathAsDBSU
	pitrRestore                    = pgbackrest.Restore
	pitrStartPostgres              = postgres.Start
	pitrPostgresCompactStatus      = postgres.PrintCompactStatusSummary
	pitrStopPostgres               = postgres.Stop
	pitrSleep                      = time.Sleep
)

var pitrGetPgInstall func(*postgres.Config) (*ext.PostgresInstall, error) = postgres.GetPgInstall

const (
	pitrFailurePhaseRestore = "restore"
	pitrFailurePhaseStart   = "start"
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

	// Confirm before destructive restore (unless --yes)
	if !opts.Yes {
		if err := utils.Confirm("This will overwrite the current database!", "PITR"); err != nil {
			return &utils.ExitCodeError{Code: output.ExitCode(output.CodePITRInvalidArgs), Err: &PITRError{Code: output.CodePITRInvalidArgs, Err: err}}
		}
	}

	// Phase 2: Stop Patroni (if active)
	patroniWasStopped := false
	if shouldManagePatroni(state.PatroniActive, state.SideRestore) {
		if pitrErr := stopPatroni(); pitrErr != nil {
			return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
		}
		patroniWasStopped = true
	}

	// Phase 3: Ensure PostgreSQL is stopped
	if pitrErr := ensurePostgresStopped(state, opts, patroniWasStopped); pitrErr != nil {
		return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
	}

	// Phase 4: Execute pgBackRest restore
	if pitrErr := executeRestore(state, opts); pitrErr != nil {
		printPITRFailureGuidance(state, opts, patroniWasStopped, pitrFailurePhaseRestore)
		return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
	}

	// Phase 5: Start PostgreSQL (unless --no-restart)
	postgresStarted := false
	if !opts.NoRestart {
		if pitrErr := startPostgres(state, opts); pitrErr != nil {
			printPITRFailureGuidance(state, opts, patroniWasStopped, pitrFailurePhaseStart)
			return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
		}
		postgresStarted = true
	}

	// Phase 6: Post-restore guidance
	if pitrErr := postRestore(state, opts, patroniWasStopped, postgresStarted); pitrErr != nil {
		return &utils.ExitCodeError{Code: output.ExitCode(pitrErr.Code), Err: pitrErr}
	}

	return nil
}

// ExecuteResult performs the PITR workflow and returns a structured Result.
func ExecuteResult(opts *Options) *output.Result {
	if opts != nil && opts.Quiet {
		return withQuietStderr(func() *output.Result {
			return executeResult(opts)
		})
	}
	return executeResult(opts)
}

func executeResult(opts *Options) *output.Result {
	startTime := time.Now()

	state, err := preCheck(opts)
	if err != nil {
		if pe, ok := err.(*PITRError); ok {
			return output.Fail(pe.Code, pe.Error())
		}
		return output.Fail(output.CodePITRPrecheckFailed, err.Error())
	}

	// Confirm before destructive restore (unless --yes)
	if !opts.Yes {
		if err := utils.Confirm("This will overwrite the current database!", "PITR"); err != nil {
			return output.Fail(output.CodePITRInvalidArgs, "pitr confirmation cancelled").WithDetail(err.Error())
		}
	}

	patroniWasStopped := false
	if shouldManagePatroni(state.PatroniActive, state.SideRestore) {
		if pitrErr := stopPatroni(); pitrErr != nil {
			return output.Fail(pitrErr.Code, pitrErr.Error())
		}
		patroniWasStopped = true
	}

	if pitrErr := ensurePostgresStopped(state, opts, patroniWasStopped); pitrErr != nil {
		return output.Fail(pitrErr.Code, pitrErr.Error())
	}

	if pitrErr := executeRestore(state, opts); pitrErr != nil {
		return pitrFailureResult(pitrErr, state, opts, patroniWasStopped, pitrFailurePhaseRestore)
	}

	postgresStarted := false
	if !opts.NoRestart {
		if pitrErr := startPostgres(state, opts); pitrErr != nil {
			return pitrFailureResult(pitrErr, state, opts, patroniWasStopped, pitrFailurePhaseStart)
		}
		postgresStarted = true
	}

	// Post-restore steps
	if pitrErr := postRestore(state, opts, patroniWasStopped, postgresStarted); pitrErr != nil {
		return output.Fail(pitrErr.Code, pitrErr.Error())
	}

	endTime := time.Now()
	postState := collectPostRestoreState(state, postgresStarted)
	return newPITRSuccessResult(state, opts, patroniWasStopped, postgresStarted, startTime, endTime, postState)
}

func pitrFailureResult(pitrErr *PITRError, state *SystemState, opts *Options, patroniWasStopped bool, phase string) *output.Result {
	code := output.CodePITRRestoreFailed
	message := "pitr failed"
	if pitrErr != nil {
		code = pitrErr.Code
		message = pitrErr.Error()
	}

	result := output.Fail(code, message)
	if detail := pitrFailureDetail(state, patroniWasStopped, phase); detail != "" {
		result.WithDetail(detail)
	}
	return result.WithNextActions(pitrFailureNextActions(state, opts, patroniWasStopped, phase)...)
}

func pitrFailureDetail(state *SystemState, patroniWasStopped bool, phase string) string {
	dataDir := postgres.DefaultPgData
	if state != nil && state.DataDir != "" {
		dataDir = state.DataDir
	}

	switch phase {
	case pitrFailurePhaseRestore:
		if patroniWasStopped {
			return fmt.Sprintf("PITR restore failed after Patroni was stopped. Patroni remains stopped; data directory %s may be partially restored. Do NOT start Patroni until PITR is rerun or the recovered state is validated.", dataDir)
		}
		return fmt.Sprintf("PITR restore failed; data directory %s may be partially restored. Validate or rerun PITR before starting PostgreSQL.", dataDir)
	case pitrFailurePhaseStart:
		if patroniWasStopped {
			return fmt.Sprintf("PostgreSQL failed to start after PITR restore. Patroni remains stopped; validate data directory %s before starting Patroni.", dataDir)
		}
		return fmt.Sprintf("PostgreSQL failed to start after PITR restore. Validate data directory %s before declaring recovery complete.", dataDir)
	default:
		if patroniWasStopped {
			return "PITR failed after Patroni was stopped. Patroni remains stopped until the recovered state is validated."
		}
		return ""
	}
}

func pitrFailureNextActions(state *SystemState, opts *Options, patroniWasStopped bool, phase string) []output.NextAction {
	dataDir := postgres.DefaultPgData
	sideRestore := false
	if state != nil {
		if state.DataDir != "" {
			dataDir = state.DataDir
		}
		sideRestore = state.SideRestore
	}

	actions := []output.NextAction{}
	if patroniWasStopped {
		actions = append(actions, output.NextAction{
			Command:  "systemctl status patroni",
			Reason:   "Confirm Patroni remains stopped before deciding how to recover",
			Required: false,
		})
	}
	switch phase {
	case pitrFailurePhaseRestore:
		actions = append(actions, output.NextAction{
			Command:  pigPBLogShowCommand(statePBConfig(state)),
			Reason:   "Inspect pgBackRest restore failure logs",
			Required: false,
		})
	case pitrFailurePhaseStart:
		actions = append(actions, postgresLogInspectAction(dataDir, sideRestore))
	}
	actions = append(actions, output.NextAction{
		Command:  buildCommandWith(opts, false, true),
		Reason:   "After fixing the failure, rerun PITR to restore a clean target state before starting Patroni",
		Required: false,
	})
	return actions
}

func printPITRFailureGuidance(state *SystemState, opts *Options, patroniWasStopped bool, phase string) {
	detail := pitrFailureDetail(state, patroniWasStopped, phase)
	actions := pitrFailureNextActions(state, opts, patroniWasStopped, phase)
	if detail == "" && len(actions) == 0 {
		return
	}
	if detail != "" {
		fmt.Fprintf(os.Stderr, "\n%sWARNING: %s%s\n", utils.ColorYellow, detail, utils.ColorReset)
	}
	if len(actions) > 0 {
		fmt.Fprintf(os.Stderr, "\nSuggested next actions:\n")
		for _, action := range actions {
			if action.Command != "" {
				fmt.Fprintf(os.Stderr, "  %s\n", action.Command)
			}
		}
	}
}

func withQuietStderr(fn func() *output.Result) *output.Result {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fn()
	}
	oldStderr := os.Stderr
	os.Stderr = devNull
	defer func() {
		os.Stderr = oldStderr
		_ = devNull.Close()
	}()
	return fn()
}

// ============================================================================
// Phase 1: Pre-Check
// ============================================================================

func preCheck(opts *Options) (*SystemState, error) {
	if err := ValidateOptions(opts); err != nil {
		return nil, &PITRError{Code: output.CodePITRInvalidArgs, Err: err}
	}

	// Determine DBSU, pgBackRest config, and data directory.
	dbsu := utils.GetDBSU(opts.DbSU)
	pbConfig, err := effectivePgBackRestConfigFromPITR(opts, dbsu)
	if err != nil {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
	}
	managedDataDir := pgbackrest.ResolveDataDir(pbConfig, "")
	dataDir := pgbackrest.ResolveDataDir(pbConfig, opts.DataDir)
	sideRestore, err := classifyPITRSideRestore(dataDir, managedDataDir, func(path string) (string, error) {
		return pitrResolvePathAsDBSU(dbsu, path)
	})
	if err != nil {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
	}
	if !sideRestore {
		dataDir = managedDataDir
	}
	if err := validateSideRestorePolicy(sideRestore, opts.NoRestart); err != nil {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
	}

	// Check data directory exists and is usable as a restore target.
	exists, initialized := pitrCheckDataDirAsDBSU(dbsu, dataDir)
	if err := validatePITRRestoreTarget(dbsu, dataDir, sideRestore, exists, initialized); err != nil {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
	}
	if sideRestore {
		owner, err := pitrDataDirOwnerAsDBSU(dbsu, dataDir)
		if err != nil {
			return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
		}
		if err := validatePITRDataDirOwner(dataDir, dbsu, owner); err != nil {
			return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
		}
	}

	infos, err := pitrLoadPgBackRestInfo(pbConfig, opts.Set)
	if err != nil {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
	}
	if err := validatePgBackRestPreflight(infos, opts); err != nil {
		return nil, &PITRError{Code: output.CodePITRPrecheckFailed, Err: err}
	}

	// Check current state
	patroniActive := utils.IsServiceActive("patroni")
	pgRunning, pgPID := postgres.CheckPostgresRunningAsDBSU(dbsu, dataDir)

	state := &SystemState{
		PatroniActive:  patroniActive,
		SideRestore:    sideRestore,
		PGRunning:      pgRunning,
		PGPID:          pgPID,
		DataDir:        dataDir,
		ManagedDataDir: managedDataDir,
		DbSU:           dbsu,
		PBConfig:       pbConfig,
	}

	return state, nil
}

func effectivePgBackRestConfigFromPITR(opts *Options, dbsu string) (*pgbackrest.Config, error) {
	pbConfig := pgbackrestConfigFromPITR(opts)
	pbConfig.DbSU = dbsu
	return pgbackrest.GetEffectiveConfig(pbConfig)
}

func pgbackrestConfigFromPITR(opts *Options) *pgbackrest.Config {
	pbConfig := pgbackrest.DefaultConfig()
	if opts == nil {
		return pbConfig
	}
	if opts.Stanza != "" {
		pbConfig.Stanza = opts.Stanza
	}
	if opts.ConfigPath != "" {
		pbConfig.ConfigPath = opts.ConfigPath
	}
	if opts.Repo != "" {
		pbConfig.Repo = opts.Repo
	}
	if opts.DbSU != "" {
		pbConfig.DbSU = opts.DbSU
	}
	return pbConfig
}

func classifyPITRSideRestore(dataDir string, managedDataDir string, resolver func(string) (string, error)) (bool, error) {
	if dataDir == "" {
		return false, nil
	}
	if managedDataDir == "" {
		managedDataDir = postgres.DefaultPgData
	}

	if filepath.Clean(dataDir) == filepath.Clean(managedDataDir) {
		return false, nil
	}
	if resolver == nil {
		return true, nil
	}

	resolvedDataDir, dataErr := resolver(dataDir)
	resolvedManagedDataDir, managedErr := resolver(managedDataDir)
	if dataErr != nil || resolvedDataDir == "" {
		return true, nil
	}
	if filepath.Clean(resolvedDataDir) == filepath.Clean(managedDataDir) {
		return false, nil
	}
	if managedErr != nil || resolvedManagedDataDir == "" {
		return true, nil
	}
	if filepath.Clean(resolvedDataDir) == filepath.Clean(resolvedManagedDataDir) {
		return false, nil
	}
	return true, nil
}

func resolvePathAsDBSU(dbsu string, path string) (string, error) {
	out, err := utils.DBSUCommandOutput(dbsu, []string{"readlink", "-f", path})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func validatePITRDataDir(dataDir string, explicitCustom bool, exists bool, initialized bool) error {
	if !exists {
		return fmt.Errorf("data directory %s does not exist", dataDir)
	}
	if !explicitCustom && !initialized {
		return fmt.Errorf("data directory %s is not initialized (no PG_VERSION)", dataDir)
	}
	return nil
}

func validatePITRRestoreTarget(dbsu string, dataDir string, sideRestore bool, exists bool, initialized bool) error {
	if err := validatePITRDataDir(dataDir, sideRestore, exists, initialized); err == nil {
		return nil
	} else if sideRestore {
		if !exists {
			return fmt.Errorf("%w\nHint: Pig does not create side-restore directories automatically; run: %s", err, sideRestorePrepareCommand(dbsu, dataDir))
		}
		return err
	} else if !exists {
		return err
	}

	empty, err := pitrDataDirEmptyAsDBSU(dbsu, dataDir)
	if err != nil {
		return fmt.Errorf("check whether managed data directory %s is empty: %w", dataDir, err)
	}
	if !empty {
		return fmt.Errorf("data directory %s is not initialized (no PG_VERSION) and is not empty", dataDir)
	}
	owner, err := pitrDataDirOwnerAsDBSU(dbsu, dataDir)
	if err != nil {
		return err
	}
	if err := validatePITRDataDirOwnerFor(dataDir, dbsu, owner, "managed"); err != nil {
		return err
	}
	return nil
}

func dataDirOwnerAsDBSU(dbsu string, dataDir string) (string, error) {
	out, err := utils.DBSUCommandOutput(dbsu, []string{"stat", "-L", "-c", "%U", dataDir})
	if err != nil {
		return "", fmt.Errorf("check data directory owner: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func dataDirEmptyAsDBSU(dbsu string, dataDir string) (bool, error) {
	out, err := utils.DBSUCommandOutput(dbsu, []string{"find", dataDir, "-mindepth", "1", "-maxdepth", "1", "-print", "-quit"})
	if err != nil {
		return false, fmt.Errorf("check data directory empty: %w", err)
	}
	return strings.TrimSpace(out) == "", nil
}

func validatePITRDataDirOwner(dataDir string, dbsu string, owner string) error {
	return validatePITRDataDirOwnerFor(dataDir, dbsu, owner, "custom")
}

func validatePITRDataDirOwnerFor(dataDir string, dbsu string, owner string, kind string) error {
	if owner != dbsu {
		quotedDataDir := pgbackrest.QuoteShellArg(dataDir)
		return fmt.Errorf("%s data directory %s is owned by %s; run: chown %s %s && chmod 700 %s", kind, dataDir, owner, dbsu, quotedDataDir, quotedDataDir)
	}
	return nil
}

func sideRestorePrepareCommand(dbsu string, dataDir string) string {
	quotedDataDir := pgbackrest.QuoteShellArg(dataDir)
	return fmt.Sprintf("install -d -m 700 -o %s -g %s %s", dbsu, dbsu, quotedDataDir)
}

func validateSideRestorePolicy(sideRestore bool, noRestart bool) error {
	if sideRestore && !noRestart {
		return fmt.Errorf("custom data directory PITR requires --no-restart because restored config keeps the original port; start it manually with pg_ctl -D <dir> -o \"-p <free-port>\" start")
	}
	return nil
}

func validatePITRTargetActionPolicy(opts *Options) error {
	if opts == nil {
		return nil
	}
	if opts.TargetAction == "shutdown" && !opts.NoRestart {
		return fmt.Errorf("pitr --target-action=shutdown requires --no-restart because PostgreSQL exits after reaching the recovery target")
	}
	return nil
}

func validatePgBackRestPreflight(infos []pgbackrest.PgBackRestInfo, opts *Options) error {
	if len(infos) == 0 {
		return fmt.Errorf("pgbackrest info returned no stanza")
	}

	info := infos[0]
	if info.Status.Code != 0 {
		return fmt.Errorf("pgbackrest stanza %s status is not ok: %s", info.Name, info.Status.Message)
	}
	if len(info.Backup) == 0 {
		return fmt.Errorf("pgbackrest stanza %s has no backups", info.Name)
	}
	if opts != nil && opts.Set != "" {
		for _, backup := range info.Backup {
			if backup.Label == opts.Set {
				return nil
			}
		}
		return fmt.Errorf("pgbackrest backup set %s not found", opts.Set)
	}
	return nil
}

func shouldManagePatroni(patroniActive bool, sideRestore bool) bool {
	return patroniActive && !sideRestore
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

func ValidateOptions(opts *Options) error {
	if err := validateRecoveryTarget(opts); err != nil {
		return err
	}
	if err := pgbackrest.ValidateRestoreOptions(restoreOptionsFromPITR(opts)); err != nil {
		return err
	}
	if err := validatePITRTargetActionPolicy(opts); err != nil {
		return err
	}
	return nil
}

// ============================================================================
// Execution Plan Display
// ============================================================================

func printExecutionPlan(state *SystemState, opts *Options) {
	managePatroni := shouldManagePatroni(state.PatroniActive, state.SideRestore)

	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s PITR Execution Plan%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)

	// Current state
	fmt.Fprintf(os.Stderr, "\n%sCurrent State:%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "  Data Directory:  %s\n", state.DataDir)
	fmt.Fprintf(os.Stderr, "  Database User:   %s\n", state.DbSU)

	if state.PatroniActive && state.SideRestore {
		fmt.Fprintf(os.Stderr, "  Patroni Service: %sactive%s (left running for custom data dir)\n", utils.ColorGreen, utils.ColorReset)
	} else if state.PatroniActive {
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
	if opts.TargetAction != "" {
		fmt.Fprintf(os.Stderr, "  Target action: %s\n", opts.TargetAction)
	}
	if opts.TargetTimeline != "" {
		fmt.Fprintf(os.Stderr, "  Target timeline: %s\n", opts.TargetTimeline)
	}
	if opts.ForceStop {
		fmt.Fprintf(os.Stderr, "  Force stop: yes (allow immediate stop / kill fallback)\n")
	}

	// Execution steps
	fmt.Fprintf(os.Stderr, "\n%sExecution Steps:%s\n", utils.ColorBold, utils.ColorReset)
	step := 1

	if managePatroni {
		fmt.Fprintf(os.Stderr, "  [%d] Stop Patroni service\n", step)
		step++
	} else if state.PatroniActive && state.SideRestore {
		fmt.Fprintf(os.Stderr, "  [-] Leave Patroni running (custom data directory)\n")
	}

	if state.PGRunning || managePatroni {
		fmt.Fprintf(os.Stderr, "  [%d] Ensure PostgreSQL is stopped\n", step)
		step++
	}

	fmt.Fprintf(os.Stderr, "  [%d] Execute pgBackRest restore\n", step)
	step++

	if !opts.NoRestart {
		fmt.Fprintf(os.Stderr, "  [%d] Start PostgreSQL\n", step)
		step++
		if shouldWaitForRecoveryComplete(opts) {
			fmt.Fprintf(os.Stderr, "  [%d] Wait for PostgreSQL recovery to complete\n", step)
			step++
		}
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
		Command:       buildCommand(opts),
		Boundary:      buildPITRBoundary(state),
		Confirmation:  "required",
		Actions:       actions,
		Affects:       affects,
		Expected:      expected,
		Risks:         risks,
		Preconditions: buildPreconditions(state, opts),
		Verifications: buildVerifications(state, opts),
		NextActions:   buildPlanNextActions(state, opts),
	}
}

func buildPITRBoundary(state *SystemState) string {
	if state != nil && state.SideRestore {
		return "pitr:side-restore"
	}
	return "pitr:managed-recovery"
}

func buildActions(state *SystemState, opts *Options) []output.Action {
	if state == nil || opts == nil {
		return nil
	}
	managePatroni := shouldManagePatroni(state.PatroniActive, state.SideRestore)
	actions := []output.Action{}
	step := 1

	if managePatroni {
		actions = append(actions, output.Action{Step: step, Description: "Stop Patroni service"})
		step++
	}

	if state.PGRunning || managePatroni {
		actions = append(actions, output.Action{Step: step, Description: "Ensure PostgreSQL is stopped"})
		step++
	}

	actions = append(actions, output.Action{Step: step, Description: "Execute pgBackRest restore"})
	step++

	if !opts.NoRestart {
		actions = append(actions, output.Action{Step: step, Description: "Start PostgreSQL"})
		step++
		if shouldWaitForRecoveryComplete(opts) {
			actions = append(actions, output.Action{Step: step, Description: "Wait for PostgreSQL recovery to complete"})
			step++
		}
	}

	actions = append(actions, output.Action{Step: step, Description: "Print post-restore guidance"})

	return actions
}

func buildAffects(state *SystemState, opts *Options) []output.Resource {
	if state == nil || opts == nil {
		return nil
	}
	managePatroni := shouldManagePatroni(state.PatroniActive, state.SideRestore)
	affects := []output.Resource{}

	if managePatroni {
		affects = append(affects, output.Resource{
			Type:   "service",
			Name:   "patroni",
			Impact: "stop",
			Detail: "cluster management paused",
		})
	}

	if state.PGRunning || managePatroni {
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
		Type:   "data_dir",
		Name:   state.DataDir,
		Impact: "overwrite",
		Detail: "data directory restored",
	})

	return affects
}

func buildPreconditions(state *SystemState, opts *Options) []output.Check {
	if state == nil || opts == nil {
		return nil
	}
	target := getTargetDescription(opts)
	preconditions := []output.Check{
		{Name: "recovery target", Status: "required", Detail: target},
		{Name: "data directory", Status: "required", Detail: state.DataDir + " must be accessible and in the expected initialization state"},
		{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaForPlan(state)},
	}
	if state.SideRestore {
		preconditions = append(preconditions,
			output.Check{Name: "restart policy", Status: "required", Detail: "side restore requires --no-restart"},
			output.Check{Name: "service lifecycle", Status: "not-managed", Detail: "Patroni and managed PostgreSQL stay outside side restore lifecycle"},
		)
	} else {
		preconditions = append(preconditions,
			output.Check{Name: "managed data directory", Status: "required", Detail: state.ManagedDataDir},
			output.Check{Name: "service lifecycle", Status: "managed", Detail: "PITR may stop Patroni/PostgreSQL to restore the managed data directory"},
		)
	}
	return preconditions
}

func buildVerifications(state *SystemState, opts *Options) []output.Check {
	if state == nil || opts == nil {
		return nil
	}
	verifications := []output.Check{
		{Name: "restore command", Status: "pending", Detail: "pgBackRest restore exit status"},
		{Name: "post restore state", Status: "manual", Detail: "verify recovered data, LSN/timeline, and service state before resuming HA"},
	}
	if !opts.NoRestart && shouldWaitForRecoveryComplete(opts) {
		verifications = append(verifications, output.Check{
			Name: "recovery complete", Status: "pending", Detail: "pg_is_in_recovery() must become false on the restored instance",
		})
	}
	if shouldManagePatroni(state.PatroniActive, state.SideRestore) {
		verifications = append(verifications, output.Check{
			Name: "patroni state", Status: "manual", Detail: "Patroni remains stopped until the recovered state is validated",
		})
	}
	return verifications
}

func buildPlanNextActions(state *SystemState, opts *Options) []output.NextAction {
	if opts == nil {
		return nil
	}
	return []output.NextAction{
		{Command: buildCommandWith(opts, false, true), Reason: "execute PITR after explicit confirmation", Required: true},
		{Command: pigPBInfoCommand(statePBConfig(state)), Reason: "inspect available pgBackRest backup sets", Required: false},
	}
}

func ConfirmationOperation() output.OperationMeta {
	return output.OperationMeta{
		Module:       "pitr",
		Command:      "pitr",
		Boundary:     "pitr:restore",
		Risk:         "critical",
		Confirmation: "required",
		Executed:     false,
		DryRun:       false,
	}
}

func ConfirmationNextActions(opts *Options) []output.NextAction {
	return []output.NextAction{
		{Command: buildCommandWith(opts, false, true), Reason: "execute PITR after explicit confirmation", Required: true},
		{Command: buildCommandWith(opts, true, false), Reason: "preview PITR prechecks and lifecycle steps", Required: false},
		{Command: pitrPBRestorePlanCommand(opts), Reason: "preview the low-level pgBackRest restore primitive", Required: false},
	}
}

func restoreStanzaForPlan(state *SystemState) string {
	if state != nil && state.PBConfig != nil && state.PBConfig.Stanza != "" {
		return state.PBConfig.Stanza
	}
	return "resolved pgBackRest stanza"
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
	if opts.TargetAction != "" {
		expected = expected + "; target action " + opts.TargetAction
	}
	if opts.TargetTimeline != "" {
		expected = expected + "; target timeline " + opts.TargetTimeline
	}
	if !opts.NoRestart && shouldWaitForRecoveryComplete(opts) {
		expected = expected + "; waits for recovery completion"
	}
	return expected
}

func buildRisks(state *SystemState, opts *Options) []string {
	if state == nil || opts == nil {
		return nil
	}
	managePatroni := shouldManagePatroni(state.PatroniActive, state.SideRestore)
	risks := []string{
		"Current data directory will be overwritten",
	}

	if managePatroni {
		risks = append(risks, "Patroni will be stopped; HA management suspended and Patroni is not restarted or rejoined by this command")
	}
	if opts.NoRestart {
		risks = append(risks, "PostgreSQL will remain stopped after restore")
	}
	if opts.Exclusive {
		risks = append(risks, "Exclusive recovery stops before target; data beyond target not applied")
	}
	if opts.ForceStop {
		risks = append(risks, "Force stop may use immediate shutdown or SIGKILL if fast stop fails")
	}
	return risks
}

func buildCommand(opts *Options) string {
	return buildCommandWith(opts, opts != nil && opts.Plan, false)
}

func buildCommandWith(opts *Options, includePlan bool, includeYes bool) string {
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
		args = append(args, "-t", pgbackrest.QuoteShellArg(pgbackrest.NormalizeRestoreTime(opts.Time)))
	case opts.Name != "":
		args = append(args, "--name", pgbackrest.QuoteShellArg(opts.Name))
	case opts.LSN != "":
		args = append(args, "--lsn", pgbackrest.QuoteShellArg(opts.LSN))
	case opts.XID != "":
		args = append(args, "--xid", pgbackrest.QuoteShellArg(opts.XID))
	}

	if opts.Set != "" {
		args = append(args, "-b", pgbackrest.QuoteShellArg(opts.Set))
	}
	if opts.NoRestart {
		args = append(args, "--no-restart")
	}
	if opts.Exclusive {
		args = append(args, "-X")
	}
	if opts.TargetAction != "" {
		args = append(args, "--target-action", pgbackrest.QuoteShellArg(opts.TargetAction))
	}
	if opts.TargetTimeline != "" {
		args = append(args, "-T", pgbackrest.QuoteShellArg(opts.TargetTimeline))
	}
	if opts.ForceStop {
		args = append(args, "--force-stop")
	}
	if opts.Timeout > 0 && recoveryWaitTimeout(opts) != pgRecoveryWaitTimeout {
		args = append(args, "--timeout", strconv.Itoa(opts.Timeout))
	}
	if opts.Stanza != "" {
		args = append(args, "-s", pgbackrest.QuoteShellArg(opts.Stanza))
	}
	if opts.ConfigPath != "" {
		args = append(args, "-c", pgbackrest.QuoteShellArg(opts.ConfigPath))
	}
	if opts.Repo != "" {
		args = append(args, "-r", pgbackrest.QuoteShellArg(opts.Repo))
	}
	if opts.DbSU != "" {
		args = append(args, "-U", pgbackrest.QuoteShellArg(opts.DbSU))
	}
	if opts.DataDir != "" {
		args = append(args, "-D", pgbackrest.QuoteShellArg(opts.DataDir))
	}
	if includeYes {
		args = append(args, "--yes")
	} else if opts.Yes {
		args = append(args, "-y")
	}
	if includePlan {
		args = append(args, "--plan")
	}
	if len(opts.ExtraArgs) > 0 {
		args = append(args, "--")
		for _, arg := range opts.ExtraArgs {
			args = append(args, pgbackrest.QuoteShellArg(arg))
		}
	}

	return strings.Join(args, " ")
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

func ensurePostgresStopped(state *SystemState, opts *Options, patroniWasStopped bool) *PITRError {
	fmt.Fprintf(os.Stderr, "\n%s=== Ensuring PostgreSQL is Stopped ===%s\n", utils.ColorBold, utils.ColorReset)

	// If Patroni was actually stopped, wait a bit for PG to stop automatically
	if patroniWasStopped {
		fmt.Fprintf(os.Stderr, "Waiting for PostgreSQL to stop (Patroni shutdown)...\n")
		for i := 0; i < pgStopCheckCount; i++ {
			pitrSleep(pgStopWaitTime)
			running, _ := pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
			if !running {
				fmt.Fprintf(os.Stderr, "%sPostgreSQL stopped automatically.%s\n", utils.ColorGreen, utils.ColorReset)
				return nil
			}
			fmt.Fprintf(os.Stderr, "  Still running, waiting... (%d/%d)\n", i+1, pgStopCheckCount)
		}
		fmt.Fprintf(os.Stderr, "PostgreSQL did not stop automatically, proceeding to stop manually.\n")
	}

	// Check if already stopped
	running, pid := pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
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
		err := pitrStopPostgres(pgConfig, stopOpts)
		if err == nil {
			// Verify stopped
			running, _ := pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
			if !running {
				fmt.Fprintf(os.Stderr, "%sPostgreSQL stopped successfully.%s\n", utils.ColorGreen, utils.ColorReset)
				return nil
			}
		}

		if attempt < maxStopRetries {
			fmt.Fprintf(os.Stderr, "  Stop attempt %d failed, retrying in %v...\n", attempt, retryDelay)
			pitrSleep(retryDelay)
			retryDelay *= 2 // Exponential backoff
		}
	}

	if !shouldEscalateStop(opts) {
		return &PITRError{Code: output.CodePITRPgRunning, Err: fmt.Errorf("postgresql did not stop with fast mode; rerun with --force-stop to allow immediate shutdown and kill fallback")}
	}

	// All retries failed, try immediate mode
	fmt.Fprintf(os.Stderr, "%sGraceful stop failed, trying immediate mode...%s\n", utils.ColorYellow, utils.ColorReset)
	stopOpts := &postgres.StopOptions{
		Mode:    "immediate",
		Timeout: 30,
	}
	if err := pitrStopPostgres(pgConfig, stopOpts); err == nil {
		running, _ := pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
		if !running {
			fmt.Fprintf(os.Stderr, "%sPostgreSQL stopped (immediate mode).%s\n", utils.ColorGreen, utils.ColorReset)
			return nil
		}
	}

	// Last resort: kill -9
	fmt.Fprintf(os.Stderr, "%sImmediate mode failed, using kill -9...%s\n", utils.ColorRed, utils.ColorReset)
	running, pid = pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
	if running && pid > 0 {
		if err := killProcess(state.DbSU, pid); err != nil {
			return &PITRError{Code: output.CodePITRStopFailed, Err: fmt.Errorf("failed to kill PostgreSQL process (PID: %d): %w", pid, err)}
		}

		// Wait a moment and verify
		pitrSleep(2 * time.Second)
		running, _ = pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
		if running {
			return &PITRError{Code: output.CodePITRPgRunning, Err: fmt.Errorf("postgresql still running after kill -9, manual intervention required")}
		}
		fmt.Fprintf(os.Stderr, "%sPostgreSQL killed (SIGKILL).%s\n", utils.ColorYellow, utils.ColorReset)
	} else if running {
		return &PITRError{Code: output.CodePITRStopFailed, Err: fmt.Errorf("postgresql is still running but cannot determine PID; manual intervention required")}
	}

	return nil
}

func shouldEscalateStop(opts *Options) bool {
	return opts != nil && opts.ForceStop
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

	pbConfig := state.PBConfig
	if pbConfig == nil {
		pbConfig = pgbackrestConfigFromPITR(opts)
		pbConfig.DbSU = state.DbSU
	}

	restoreOpts := restoreOptionsFromPITR(opts)
	restoreOpts.DataDir = state.DataDir
	restoreOpts.Yes = true // Skip confirmation (already confirmed in PITR)

	// Execute restore
	if err := pitrRestore(pbConfig, restoreOpts); err != nil {
		fmt.Fprintf(os.Stderr, "\n%sERROR: pgBackRest restore failed: %v%s\n", utils.ColorRed, err, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "\nCheck pgBackRest logs:\n")
		fmt.Fprintf(os.Stderr, "  pig pb log show\n")
		fmt.Fprintf(os.Stderr, "  tail -100 /pg/log/pgbackrest/*.log\n")
		code := classifyRestoreError(err)
		if code == output.CodePITRNoBackup {
			return &PITRError{Code: code, Err: fmt.Errorf("backup not found: %w", err)}
		}
		return &PITRError{Code: code, Err: fmt.Errorf("pgbackrest restore failed: %w", err)}
	}

	fmt.Fprintf(os.Stderr, "%spgBackRest restore completed successfully.%s\n", utils.ColorGreen, utils.ColorReset)
	return nil
}

func restoreOptionsFromPITR(opts *Options) *pgbackrest.RestoreOptions {
	return &pgbackrest.RestoreOptions{
		Default:        opts.Default,
		Immediate:      opts.Immediate,
		Time:           opts.Time,
		Name:           opts.Name,
		LSN:            opts.LSN,
		XID:            opts.XID,
		Set:            opts.Set,
		DataDir:        opts.DataDir,
		Exclusive:      opts.Exclusive,
		TargetAction:   opts.TargetAction,
		TargetTimeline: opts.TargetTimeline,
		ExtraArgs:      append([]string(nil), opts.ExtraArgs...),
		SuppressHints:  true,
	}
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

// isNoBackupError delegates to the shared pgbackrest compound matcher so both
// layers classify backup-not-found identically. The former loose clause
// ("backup" + "not found") is dropped: it could misfire on unrelated errors.
func isNoBackupError(message string) bool {
	return pgbackrest.IsBackupNotFoundError(message)
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
	timeout := recoveryWaitTimeout(opts)

	startOpts := &postgres.StartOptions{
		Timeout: int(timeout.Seconds()), // Recovery may take time
	}

	if err := pitrStartPostgres(pgConfig, startOpts); err != nil {
		fmt.Fprintf(os.Stderr, "\n%sERROR: Failed to start PostgreSQL: %v%s\n", utils.ColorRed, err, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "\nCheck PostgreSQL logs:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", postgresLogInspectAction(state.DataDir, state.SideRestore).Command)
		if !state.SideRestore && isDefaultManagedDataDir(state.DataDir) {
			fmt.Fprintf(os.Stderr, "  tail -100 /pg/log/postgres/*.csv\n")
		}
		return &PITRError{Code: output.CodePITRStartFailed, Err: fmt.Errorf("failed to start postgresql: %w", err)}
	}

	// Verify running
	running, pid := postgres.CheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
	if !running {
		return &PITRError{Code: output.CodePITRStartFailed, Err: fmt.Errorf("postgresql failed to start after restore")}
	}

	if shouldWaitForRecoveryComplete(opts) {
		fmt.Fprintf(os.Stderr, "Waiting for PostgreSQL recovery to complete...\n")
		if err := waitForRecoveryComplete(state, timeout); err != nil {
			return &PITRError{Code: output.CodePITRStartFailed, Err: err}
		}
		fmt.Fprintf(os.Stderr, "%sPostgreSQL recovery completed; instance is primary.%s\n", utils.ColorGreen, utils.ColorReset)
	}

	fmt.Fprintf(os.Stderr, "%sPostgreSQL started successfully (PID: %d).%s\n", utils.ColorGreen, pid, utils.ColorReset)
	return nil
}

func shouldWaitForRecoveryComplete(opts *Options) bool {
	return opts != nil && (opts.Default || targetActionFromOptions(opts) == "promote")
}

func recoveryWaitTimeout(opts *Options) time.Duration {
	if opts != nil && opts.Timeout > 0 {
		return time.Duration(opts.Timeout) * time.Second
	}
	return pgRecoveryWaitTimeout
}

func waitForRecoveryComplete(state *SystemState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		inRecovery, err := pitrQueryRecoveryState(state)
		if err == nil {
			if !inRecovery {
				return nil
			}
			lastErr = nil
		} else {
			lastErr = err
		}

		if !time.Now().Before(deadline) {
			if lastErr != nil {
				return fmt.Errorf("postgresql still in recovery after %s (last check failed: %w)", timeout, lastErr)
			}
			return fmt.Errorf("postgresql still in recovery after %s", timeout)
		}

		pitrSleep(pgRecoveryWaitTime)
	}
}

func queryRecoveryState(state *SystemState) (bool, error) {
	out, err := runBoundPsqlQuery(state, "SELECT pg_is_in_recovery()")
	if err != nil {
		return false, fmt.Errorf("query pg_is_in_recovery(): %w", err)
	}

	switch strings.TrimSpace(out) {
	case "t":
		return true, nil
	case "f":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected pg_is_in_recovery() result: %q", strings.TrimSpace(out))
	}
}

func collectPostRestoreState(state *SystemState, postgresStarted bool) *PostRestoreState {
	post := &PostRestoreState{
		Queried:       true,
		PatroniActive: utils.IsServiceActive("patroni"),
	}
	if state == nil {
		post.Error = "system state is nil"
		post.QuerySkippedReason = "system state is nil"
		return post
	}

	running, _ := pitrCheckPostgresRunningAsDBSU(state.DbSU, state.DataDir)
	post.Running = running
	if !running {
		post.QuerySkippedReason = "postgresql is not running"
		return post
	}
	if !postgresStarted {
		post.QuerySkippedReason = "postgresql was not started by PITR"
		return post
	}

	queried, err := pitrQueryPostRestoreState(state)
	if err != nil {
		post.Error = err.Error()
		return post
	}
	if queried == nil {
		post.Error = "post-restore query returned nil state"
		return post
	}
	queried.PatroniActive = post.PatroniActive
	return queried
}

func queryPostRestoreState(state *SystemState) (*PostRestoreState, error) {
	inRecovery, err := queryRecoveryState(state)
	if err != nil {
		return nil, err
	}

	pgConfig := &postgres.Config{
		PgData: state.DataDir,
		DbSU:   state.DbSU,
	}
	pg, err := pitrGetPgInstall(pgConfig)
	if err != nil {
		return nil, fmt.Errorf("find postgresql install: %w", err)
	}

	out, err := runBoundPsqlQuery(state, "SELECT CASE WHEN pg_is_in_recovery() THEN pg_last_wal_replay_lsn()::text ELSE pg_current_wal_lsn()::text END")
	if err != nil {
		return nil, fmt.Errorf("query current/replay lsn: %w", err)
	}

	post := &PostRestoreState{
		Queried:    true,
		SQLQueried: true,
		Running:    true,
		InRecovery: &inRecovery,
		CurrentLSN: strings.TrimSpace(out),
	}
	if timeline, err := queryTimelineID(state, pg.PgControldata()); err == nil {
		post.TimelineID = timeline
	}
	return post, nil
}

func runBoundPsqlQuery(state *SystemState, sql string) (string, error) {
	if state == nil {
		return "", fmt.Errorf("system state is nil")
	}
	info, err := postgres.ReadPostmasterPidInfoAsDBSU(state.DbSU, state.DataDir)
	if err != nil {
		return "", fmt.Errorf("read postmaster connection info: %w", err)
	}

	pgConfig := &postgres.Config{
		PgData: state.DataDir,
		DbSU:   state.DbSU,
	}
	pg, err := pitrGetPgInstall(pgConfig)
	if err != nil {
		return "", fmt.Errorf("find postgresql install: %w", err)
	}

	args := []string{
		pg.Psql(),
		"-X",
		"-qAt",
		"-w",
		"-v", "ON_ERROR_STOP=1",
		"-p", strconv.Itoa(info.Port),
	}
	if info.SocketDir != "" {
		args = append(args, "-h", info.SocketDir)
	}
	args = append(args,
		"-d", "postgres",
		"-c", sql,
	)
	return pitrDBSUCommandOutput(state.DbSU, args)
}

func queryTimelineID(state *SystemState, pgControlData string) (string, error) {
	if state == nil {
		return "", fmt.Errorf("system state is nil")
	}
	out, err := pitrDBSUCommandOutput(state.DbSU, []string{pgControlData, "-D", state.DataDir})
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Latest checkpoint's TimeLineID") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("timeline id not found in pg_controldata")
}

// ============================================================================
// Phase 6: Post-Restore Guidance
// ============================================================================

// postRestore performs post-restore steps and returns *PITRError on failure.
// Currently the post-restore phase only prints guidance, but this wrapper
// ensures CodePITRPostFailed is properly used if any step fails.
func postRestore(state *SystemState, opts *Options, patroniWasStopped bool, postgresStarted bool) *PITRError {
	if shouldPrintPostRestoreStatus(opts, postgresStarted) {
		if err := printPostRestoreStatus(state); err != nil {
			return &PITRError{Code: output.CodePITRPostFailed, Err: fmt.Errorf("failed to print post-restore PostgreSQL status: %w", err)}
		}
	}
	if err := printPostRestoreGuidance(state, opts, patroniWasStopped); err != nil {
		return &PITRError{Code: output.CodePITRPostFailed, Err: fmt.Errorf("failed to write post-restore guidance: %w", err)}
	}
	return nil
}

func shouldPrintPostRestoreStatus(opts *Options, postgresStarted bool) bool {
	return postgresStarted && (opts == nil || !opts.Quiet)
}

func printPostRestoreStatus(state *SystemState) error {
	cfg := &postgres.Config{}
	if state != nil {
		cfg.PgData = state.DataDir
		cfg.DbSU = state.DbSU
	}
	pitrPostgresCompactStatus(cfg)
	return nil
}

func printPostRestoreGuidance(state *SystemState, opts *Options, patroniWasStopped bool) error {
	// Fail fast if stderr is unavailable so post-restore errors can be surfaced.
	if _, err := fmt.Fprint(os.Stderr, ""); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s PITR Complete%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Fprintf(os.Stderr, "%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)

	actions := buildPostRestoreNextActions(state, opts, patroniWasStopped)
	for i, action := range actions {
		reason := action.Reason
		if reason == "" {
			reason = "Next action"
		}
		fmt.Fprintf(os.Stderr, "\n%s[%d] %s:%s\n", utils.ColorBold, i+1, reason, utils.ColorReset)
		if action.Command != "" {
			fmt.Fprintf(os.Stderr, "   %s\n", action.Command)
		}
	}

	fmt.Fprintf(os.Stderr, "\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	return nil
}

func buildPostRestoreNextActions(state *SystemState, opts *Options, patroniWasStopped bool) []output.NextAction {
	if opts == nil {
		opts = &Options{}
	}
	dataDir := postgres.DefaultPgData
	managedDir := postgres.DefaultPgData
	sideRestore := false
	if state != nil {
		if state.DataDir != "" {
			dataDir = state.DataDir
		}
		if state.ManagedDataDir != "" {
			managedDir = state.ManagedDataDir
		}
		sideRestore = state.SideRestore
	} else if opts.DataDir != "" {
		dataDir = opts.DataDir
		sideRestore, _ = classifyPITRSideRestore(dataDir, managedDir, nil)
	}
	if managedDir == "" {
		managedDir = postgres.DefaultPgData
	}

	targetAction := targetActionFromOptions(opts)
	shutdownTarget := targetAction == "shutdown"
	needsManualPromote := targetAction != "promote" && !opts.Default && !shutdownTarget

	if sideRestore {
		return buildSideRestoreNextActions(dataDir, statePBConfig(state), opts, shutdownTarget, needsManualPromote)
	}
	return buildManagedRestoreNextActions(dataDir, statePBConfig(state), opts, patroniWasStopped, shutdownTarget, needsManualPromote)
}

func buildSideRestoreNextActions(dataDir string, pbConfig *pgbackrest.Config, opts *Options, shutdownTarget bool, needsManualPromote bool) []output.NextAction {
	actions := []output.NextAction{}
	quotedDataDir := pgbackrest.QuoteShellArg(dataDir)
	if shutdownTarget {
		if opts.NoRestart {
			actions = append(actions, output.NextAction{
				Command:  fmt.Sprintf("pg_ctl -D %s -o \"-p 5433\" start", quotedDataDir),
				Reason:   "Start PostgreSQL to complete recovery shutdown; port 5433 is an example, replace it if occupied",
				Required: true,
			})
		}
		actions = append(actions,
			output.NextAction{Command: fmt.Sprintf("pg_ctl -D %s status", quotedDataDir), Reason: "Verify recovery shutdown", Required: false},
			postgresLogInspectAction(dataDir, true),
			output.NextAction{Command: sideRestoreStanzaCreateCommand(pbConfig, dataDir), Reason: "Only if converting this side directory into a managed cluster, re-create pgBackRest stanza", Required: false},
		)
		return actions
	}

	if opts.NoRestart {
		actions = append(actions, output.NextAction{
			Command:  fmt.Sprintf("pg_ctl -D %s -o \"-p 5433\" start", quotedDataDir),
			Reason:   "Start PostgreSQL on an alternate port because restored config keeps the original port; 5433 is an example, replace it if occupied",
			Required: true,
		})
	}
	actions = append(actions, output.NextAction{
		Command: fmt.Sprintf("pg_ctl -D %s status", quotedDataDir), Reason: "Verify recovered data", Required: false,
	})
	if needsManualPromote {
		actions = append(actions, output.NextAction{
			Command: fmt.Sprintf("pg_ctl -D %s promote", quotedDataDir), Reason: "If satisfied, promote to primary", Required: false,
		})
	}
	actions = append(actions, output.NextAction{
		Command: sideRestoreStanzaCreateCommand(pbConfig, dataDir), Reason: "Only if converting this side directory into a managed cluster, re-create pgBackRest stanza", Required: false,
	})
	return actions
}

func buildManagedRestoreNextActions(dataDir string, _ *pgbackrest.Config, opts *Options, patroniWasStopped bool, shutdownTarget bool, needsManualPromote bool) []output.NextAction {
	actions := []output.NextAction{}
	if shutdownTarget {
		if opts.NoRestart {
			actions = append(actions, output.NextAction{
				Command:  pigPgCommand("start", dataDir),
				Reason:   "Start PostgreSQL to complete recovery shutdown; PostgreSQL reaches the recovery target and exits because target-action=shutdown",
				Required: true,
			})
		}
		actions = append(actions, postgresLogInspectAction(dataDir, false))
		if patroniWasStopped {
			actions = append(actions, output.NextAction{Command: "systemctl status patroni", Reason: "Keep Patroni stopped until the recovered state is validated", Required: false})
		}
		return actions
	}

	if opts.NoRestart {
		actions = append(actions, output.NextAction{Command: pigPgCommand("start", dataDir), Reason: "Start PostgreSQL", Required: true})
	}
	actions = append(actions, output.NextAction{Command: psqlCommand(dataDir), Reason: "Verify recovered data", Required: false})
	if needsManualPromote {
		actions = append(actions, output.NextAction{Command: pigPgCommand("promote", dataDir), Reason: "If satisfied, promote to primary", Required: false})
	}
	if patroniWasStopped {
		actions = append(actions, output.NextAction{
			Command: "pig pt start",
			Reason:  "Launch Patroni to resume HA; ensure data is correct before starting Patroni",
		})
	}
	actions = append(actions, output.NextAction{Command: pigPBBackupFullCommand(), Reason: "Recreate a full backup if necessary", Required: false})
	return actions
}

func statePBConfig(state *SystemState) *pgbackrest.Config {
	if state == nil {
		return nil
	}
	return state.PBConfig
}

func pigPgCommand(subcommand string, dataDir string) string {
	parts := []string{"pig", "pg", subcommand}
	if !isDefaultManagedDataDir(dataDir) {
		parts = append(parts, "-D", pgbackrest.QuoteShellArg(dataDir))
	}
	return strings.Join(parts, " ")
}

func psqlCommand(dataDir string) string {
	if isDefaultManagedDataDir(dataDir) {
		return "psql"
	}
	return pigPgCommand("psql", dataDir)
}

func pigPBBackupFullCommand() string {
	return "pig pb backup full"
}

func pigPBLogShowCommand(cfg *pgbackrest.Config) string {
	return pigPBCommand("log show", cfg)
}

func pigPBInfoCommand(cfg *pgbackrest.Config) string {
	return pigPBCommand("info", cfg)
}

func pigPBCommand(subcommand string, cfg *pgbackrest.Config) string {
	parts := []string{"pig", "pb", subcommand}
	if cfg != nil {
		if cfg.Stanza != "" {
			parts = append(parts, "-s", pgbackrest.QuoteShellArg(cfg.Stanza))
		}
		if cfg.ConfigPath != "" && cfg.ConfigPath != pgbackrest.DefaultConfigPath {
			parts = append(parts, "-c", pgbackrest.QuoteShellArg(cfg.ConfigPath))
		}
		if cfg.Repo != "" {
			parts = append(parts, "-r", pgbackrest.QuoteShellArg(cfg.Repo))
		}
		if cfg.DbSU != "" {
			parts = append(parts, "-U", pgbackrest.QuoteShellArg(cfg.DbSU))
		}
	}
	return strings.Join(parts, " ")
}

func postgresLogInspectAction(dataDir string, sideRestore bool) output.NextAction {
	if dataDir == "" {
		dataDir = postgres.DefaultPgData
	}
	if sideRestore {
		logDir := filepath.Join(dataDir, "log")
		quotedLogDir := pgbackrest.QuoteShellArg(logDir)
		return output.NextAction{
			Command:  fmt.Sprintf("tail -100 %s/*.csv %s/*.log", quotedLogDir, quotedLogDir),
			Reason:   "Inspect PostgreSQL logs in the side restore data directory",
			Required: false,
		}
	}
	if isDefaultManagedDataDir(dataDir) {
		return output.NextAction{Command: "pig pg log show", Reason: "Inspect PostgreSQL logs before next start", Required: false}
	}
	return output.NextAction{
		Command:  "pig pg log show --log-dir " + pgbackrest.QuoteShellArg(filepath.Join(dataDir, "log")),
		Reason:   "Inspect PostgreSQL logs before next start; adjust --log-dir if restored config uses an absolute log_directory",
		Required: false,
	}
}

func pitrPBRestorePlanCommand(opts *Options) string {
	if opts == nil {
		opts = &Options{}
	}
	return pgbackrest.RestoreCommand(pgbackrestConfigFromPITR(opts), restoreOptionsFromPITR(opts), true, false)
}

func sideRestoreStanzaCreateCommand(cfg *pgbackrest.Config, dataDir string) string {
	parts := []string{"pgbackrest"}
	if cfg != nil {
		if cfg.Stanza != "" {
			parts = append(parts, "--stanza="+pgbackrest.QuoteShellArg(cfg.Stanza))
		}
		if cfg.ConfigPath != "" && cfg.ConfigPath != pgbackrest.DefaultConfigPath {
			parts = append(parts, "--config="+pgbackrest.QuoteShellArg(cfg.ConfigPath))
		}
	}
	parts = append(parts, "--pg1-path="+pgbackrest.QuoteShellArg(dataDir), "stanza-create")
	return strings.Join(parts, " ")
}

func isDefaultManagedDataDir(dataDir string) bool {
	return dataDir == "" || filepath.Clean(dataDir) == filepath.Clean(postgres.DefaultPgData)
}
