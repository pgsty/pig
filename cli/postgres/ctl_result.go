/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pg start/stop/restart/reload structured output DTOs and result constructors.
*/
package postgres

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"pig/cli/ext"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// Polling and timeout constants for structured result functions
const (
	// pidPollInterval is the interval between PID checks when waiting for PostgreSQL to start
	pidPollInterval = 100 * time.Millisecond

	// rolePollInterval is the interval between role checks when waiting for role change after promotion
	rolePollInterval = 500 * time.Millisecond

	// defaultPidWaitTimeout is the default timeout for waiting for a PID after start/restart
	defaultPidWaitTimeout = 5 * time.Second

	// defaultRoleWaitTimeout is the default timeout for waiting for role change after promotion
	defaultRoleWaitTimeout = 10 * time.Second
)

// ============================================================================
// Init Result DTO (Story 2.4)
// ============================================================================

// PgInitResultData contains the result data for pg init operation.
// This struct is used as the Data field in output.Result for structured output.
type PgInitResultData struct {
	DataDir  string `json:"data_dir" yaml:"data_dir"`
	Version  int    `json:"version" yaml:"version"`
	Locale   string `json:"locale" yaml:"locale"`
	Encoding string `json:"encoding" yaml:"encoding"`
	Checksum bool   `json:"checksum,omitempty" yaml:"checksum,omitempty"`
	Force    bool   `json:"force,omitempty" yaml:"force,omitempty"`
}

// InitOK creates a successful result for pg init operation.
func InitOK(dataDir string, version int, locale, encoding string, checksum bool) *output.Result {
	return output.OK("PostgreSQL cluster initialized successfully", &PgInitResultData{
		DataDir:  dataDir,
		Version:  version,
		Locale:   locale,
		Encoding: encoding,
		Checksum: checksum,
	})
}

// InitOKForce creates a successful result for pg init operation with --force flag.
// When --force is used, an existing data directory was removed before initialization.
func InitOKForce(dataDir string, version int, locale, encoding string, checksum bool) *output.Result {
	return output.OK("PostgreSQL cluster initialized (force mode)", &PgInitResultData{
		DataDir:  dataDir,
		Version:  version,
		Locale:   locale,
		Encoding: encoding,
		Checksum: checksum,
		Force:    true,
	}).WithDetail("Previous data directory was removed")
}

// ============================================================================
// Start Result DTO
// ============================================================================

// PgStartResultData contains the result data for pg start operation.
// This struct is used as the Data field in output.Result for structured output.
type PgStartResultData struct {
	PID     int    `json:"pid" yaml:"pid"`
	DataDir string `json:"data_dir" yaml:"data_dir"`
	NoWait  bool   `json:"no_wait,omitempty" yaml:"no_wait,omitempty"`
}

// StartOK creates a successful result for pg start operation.
func StartOK(pid int, dataDir string) *output.Result {
	return output.OK("PostgreSQL started successfully", &PgStartResultData{
		PID:     pid,
		DataDir: dataDir,
	})
}

// StartOKNoWait creates a successful result for pg start operation with --no-wait flag.
// When --no-wait is used, the PID may be 0 as we don't wait for the process to fully start.
func StartOKNoWait(dataDir string) *output.Result {
	return output.OK("PostgreSQL start initiated (no-wait mode)", &PgStartResultData{
		PID:     0,
		DataDir: dataDir,
		NoWait:  true,
	}).WithDetail("PID not available in no-wait mode; check status with 'pig pg status'")
}

// ============================================================================
// Stop Result DTO
// ============================================================================

// PgStopResultData contains the result data for pg stop operation.
// This struct is used as the Data field in output.Result for structured output.
type PgStopResultData struct {
	StoppedPID int    `json:"stopped_pid" yaml:"stopped_pid"`
	DataDir    string `json:"data_dir" yaml:"data_dir"`
	Mode       string `json:"mode,omitempty" yaml:"mode,omitempty"`
	NoWait     bool   `json:"no_wait,omitempty" yaml:"no_wait,omitempty"`
}

// StopOK creates a successful result for pg stop operation.
func StopOK(stoppedPID int, dataDir, mode string) *output.Result {
	return output.OK("PostgreSQL stopped successfully", &PgStopResultData{
		StoppedPID: stoppedPID,
		DataDir:    dataDir,
		Mode:       mode,
	})
}

// StopOKNoWait creates a successful result for pg stop operation with --no-wait flag.
func StopOKNoWait(stoppedPID int, dataDir, mode string) *output.Result {
	return output.OK("PostgreSQL stop initiated (no-wait mode)", &PgStopResultData{
		StoppedPID: stoppedPID,
		DataDir:    dataDir,
		Mode:       mode,
		NoWait:     true,
	}).WithDetail("Stop command sent; check status with 'pig pg status'")
}

// ============================================================================
// Restart Result DTO
// ============================================================================

// PgRestartResultData contains the result data for pg restart operation.
// This struct is used as the Data field in output.Result for structured output.
type PgRestartResultData struct {
	OldPID  int    `json:"old_pid" yaml:"old_pid"`
	NewPID  int    `json:"new_pid" yaml:"new_pid"`
	DataDir string `json:"data_dir" yaml:"data_dir"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`
	NoWait  bool   `json:"no_wait,omitempty" yaml:"no_wait,omitempty"`
}

// RestartOK creates a successful result for pg restart operation.
func RestartOK(oldPID, newPID int, dataDir, mode string) *output.Result {
	return output.OK("PostgreSQL restarted successfully", &PgRestartResultData{
		OldPID:  oldPID,
		NewPID:  newPID,
		DataDir: dataDir,
		Mode:    mode,
	})
}

// RestartOKNoWait creates a successful result for pg restart operation with --no-wait flag.
func RestartOKNoWait(oldPID int, dataDir, mode string) *output.Result {
	return output.OK("PostgreSQL restart initiated (no-wait mode)", &PgRestartResultData{
		OldPID:  oldPID,
		NewPID:  0,
		DataDir: dataDir,
		Mode:    mode,
		NoWait:  true,
	}).WithDetail("New PID not available in no-wait mode; check status with 'pig pg status'")
}

// ============================================================================
// Reload Result DTO
// ============================================================================

// PgReloadResultData contains the result data for pg reload operation.
// This struct is used as the Data field in output.Result for structured output.
type PgReloadResultData struct {
	Reloaded bool   `json:"reloaded" yaml:"reloaded"`
	PID      int    `json:"pid,omitempty" yaml:"pid,omitempty"`
	DataDir  string `json:"data_dir" yaml:"data_dir"`
}

// ReloadOK creates a successful result for pg reload operation.
func ReloadOK(pid int, dataDir string) *output.Result {
	return output.OK("PostgreSQL configuration reloaded", &PgReloadResultData{
		Reloaded: true,
		PID:      pid,
		DataDir:  dataDir,
	})
}

// ============================================================================
// Structured Result Functions (wrap existing control functions)
// ============================================================================

// StartResult executes pg start and returns a structured result.
// It captures the PID before/after and handles errors with appropriate codes.
func StartResult(cfg *Config, opts *StartOptions) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Pre-check: data directory initialization (permission-aware)
	exists, initialized, err := checkDataDirStateAsDBSU(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL data directory").
			WithData(&PgStartResultData{DataDir: dataDir}).
			WithDetail(err.Error())
	}
	if !exists {
		return output.Fail(output.CodePgStatusDataDirNotFound,
			"PostgreSQL data directory not found").
			WithData(&PgStartResultData{DataDir: dataDir}).
			WithDetail(fmt.Sprintf("data_dir=%s", dataDir))
	}
	if !initialized {
		return output.Fail(output.CodePgStatusNotInitialized,
			"Data directory not initialized").
			WithData(&PgStartResultData{DataDir: dataDir}).
			WithDetail(fmt.Sprintf("data_dir=%s (run 'pig pg init' first)", dataDir))
	}

	// Pre-check: already running? (permission-aware)
	running, pid, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL status").
			WithData(&PgStartResultData{DataDir: dataDir}).
			WithDetail(err.Error())
	}
	if running {
		force := opts != nil && opts.Force
		if !force {
			return output.Fail(output.CodePgAlreadyRunning,
				"PostgreSQL is already running").
				WithData(&PgStartResultData{PID: pid, DataDir: dataDir}).
				WithDetail(fmt.Sprintf("PID=%d; use -y to force start", pid))
		}
		logrus.Debugf("forcing start even though PostgreSQL is running (PID=%d)", pid)
	}

	// Execute the start operation
	err = Start(cfg, opts)
	if err != nil {
		code := classifyCtlError(err, output.CodePgStartFailed)
		return output.Fail(code, "Failed to start PostgreSQL").
			WithData(&PgStartResultData{DataDir: dataDir}).
			WithDetail(err.Error())
	}

	// Handle no-wait mode
	noWait := opts != nil && opts.NoWait
	if noWait {
		return StartOKNoWait(dataDir)
	}

	// Post-check: get new PID (with retry for race conditions)
	newPID := waitForPID(dbsu, dataDir, defaultPidWaitTimeout)
	if newPID == 0 {
		return output.Fail(output.CodePgTimeout, "Timed out waiting for PostgreSQL PID").
			WithData(&PgStartResultData{PID: 0, DataDir: dataDir}).
			WithDetail(fmt.Sprintf("No PID detected within %s; check 'pig pg status'", defaultPidWaitTimeout))
	}
	return StartOK(newPID, dataDir)
}

// StopResult executes pg stop and returns a structured result.
// It captures the PID before stop and handles errors with appropriate codes.
func StopResult(cfg *Config, opts *StopOptions) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get stop mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}

	// Pre-check: get current PID (permission-aware)
	running, stoppedPID, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL status").
			WithData(&PgStopResultData{StoppedPID: 0, DataDir: dataDir, Mode: mode}).
			WithDetail(err.Error())
	}
	if !running {
		return output.Fail(output.CodePgAlreadyStopped,
			"PostgreSQL is not running").
			WithData(&PgStopResultData{StoppedPID: 0, DataDir: dataDir, Mode: mode}).
			WithDetail("No PostgreSQL process found")
	}

	// Execute the stop operation
	err = Stop(cfg, opts)
	if err != nil {
		code := classifyCtlError(err, output.CodePgStopFailed)
		return output.Fail(code, "Failed to stop PostgreSQL").
			WithData(&PgStopResultData{StoppedPID: stoppedPID, DataDir: dataDir, Mode: mode}).
			WithDetail(err.Error())
	}

	// Handle no-wait mode
	noWait := opts != nil && opts.NoWait
	if noWait {
		return StopOKNoWait(stoppedPID, dataDir, mode)
	}

	return StopOK(stoppedPID, dataDir, mode)
}

// RestartResult executes pg restart and returns a structured result.
// It captures old_pid before and new_pid after restart.
func RestartResult(cfg *Config, opts *RestartOptions) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get restart mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}

	// Pre-check: get current PID (permission-aware)
	running, oldPID, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL status").
			WithData(&PgRestartResultData{OldPID: 0, DataDir: dataDir, Mode: mode}).
			WithDetail(err.Error())
	}
	if !running {
		return output.Fail(output.CodePgNotRunning,
			"PostgreSQL is not running").
			WithData(&PgRestartResultData{OldPID: oldPID, DataDir: dataDir, Mode: mode}).
			WithDetail("Cannot restart a stopped server")
	}

	// Execute the restart operation
	err = Restart(cfg, opts)
	if err != nil {
		code := classifyCtlError(err, output.CodePgRestartFailed)
		return output.Fail(code, "Failed to restart PostgreSQL").
			WithData(&PgRestartResultData{OldPID: oldPID, DataDir: dataDir, Mode: mode}).
			WithDetail(err.Error())
	}

	// Handle no-wait mode
	noWait := opts != nil && opts.NoWait
	if noWait {
		return RestartOKNoWait(oldPID, dataDir, mode)
	}

	// Post-check: get new PID (with retry for race conditions)
	newPID := waitForPID(dbsu, dataDir, defaultPidWaitTimeout)
	if newPID == 0 {
		return output.Fail(output.CodePgTimeout, "Timed out waiting for PostgreSQL PID after restart").
			WithData(&PgRestartResultData{OldPID: oldPID, NewPID: 0, DataDir: dataDir, Mode: mode}).
			WithDetail(fmt.Sprintf("No new PID detected within %s; check 'pig pg status'", defaultPidWaitTimeout))
	}
	return RestartOK(oldPID, newPID, dataDir, mode)
}

// ReloadResult executes pg reload and returns a structured result.
func ReloadResult(cfg *Config) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Pre-check: PostgreSQL must be running (permission-aware)
	running, pid, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL status").
			WithData(&PgReloadResultData{Reloaded: false, DataDir: dataDir}).
			WithDetail(err.Error())
	}
	if !running {
		return output.Fail(output.CodePgNotRunning,
			"PostgreSQL is not running").
			WithData(&PgReloadResultData{Reloaded: false, DataDir: dataDir}).
			WithDetail("Cannot reload configuration on stopped server")
	}

	// Execute the reload operation
	err = Reload(cfg)
	if err != nil {
		code := classifyCtlError(err, output.CodePgReloadFailed)
		return output.Fail(code, "Failed to reload PostgreSQL configuration").
			WithData(&PgReloadResultData{Reloaded: false, PID: pid, DataDir: dataDir}).
			WithDetail(err.Error())
	}

	return ReloadOK(pid, dataDir)
}

// ============================================================================
// Init Structured Result Function (Story 2.4)
// ============================================================================

// InitResult executes pg init and returns a structured result.
// It validates the data directory state and handles errors with appropriate codes.
func InitResult(cfg *Config, opts *InitOptions) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get encoding and locale (with defaults)
	encoding := DefaultEncoding
	locale := DefaultLocale
	checksum := false
	force := false

	if opts != nil {
		if opts.Encoding != "" {
			encoding = opts.Encoding
		}
		if opts.Locale != "" {
			locale = opts.Locale
		}
		checksum = opts.Checksum
		force = opts.Force
	}

	// Pre-check: data directory state (permission-aware)
	exists, initialized, err := checkDataDirStateAsDBSU(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL data directory").
			WithData(&PgInitResultData{DataDir: dataDir}).
			WithDetail(err.Error())
	}
	if initialized {
		if !force {
			return output.Fail(output.CodePgInitDirExists,
				"Data directory already initialized").
				WithData(&PgInitResultData{DataDir: dataDir}).
				WithDetail(fmt.Sprintf("data_dir=%s exists with PG_VERSION; use --force to overwrite (DANGEROUS)", dataDir))
		}

		// Force mode: check if PostgreSQL is running (NEVER allow overwrite if running)
		running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)
		if running {
			return output.Fail(output.CodePgInitRunningConflict,
				"PostgreSQL is running, cannot overwrite").
				WithData(&PgInitResultData{DataDir: dataDir}).
				WithDetail(fmt.Sprintf("PID=%d; stop PostgreSQL before using --force", pid))
		}
	}

	// Find PostgreSQL to get version
	pgVer := 0
	if cfg != nil {
		pgVer = cfg.PgVersion
	}
	pg, err := ext.FindPostgres(pgVer)
	if err != nil {
		return output.Fail(output.CodePgNotFound,
			"PostgreSQL not found").
			WithData(&PgInitResultData{DataDir: dataDir}).
			WithDetail(err.Error())
	}
	version := pg.MajorVersion

	// Execute the init operation
	err = InitDB(cfg, opts)
	if err != nil {
		code := classifyCtlError(err, output.CodePgInitFailed)
		return output.Fail(code, "Failed to initialize PostgreSQL cluster").
			WithData(&PgInitResultData{
				DataDir:  dataDir,
				Version:  version,
				Locale:   locale,
				Encoding: encoding,
				Checksum: checksum,
			}).
			WithDetail(err.Error())
	}

	// Success
	if force && exists {
		return InitOKForce(dataDir, version, locale, encoding, checksum)
	}
	return InitOK(dataDir, version, locale, encoding, checksum)
}

// ============================================================================
// Promote Result DTO (Story 2.5)
// ============================================================================

// PgPromoteResultData contains the result data for pg promote operation.
// This struct is used as the Data field in output.Result for structured output.
type PgPromoteResultData struct {
	Promoted     bool   `json:"promoted" yaml:"promoted"`
	Timeline     int    `json:"timeline,omitempty" yaml:"timeline,omitempty"`
	PreviousRole string `json:"previous_role" yaml:"previous_role"`
	CurrentRole  string `json:"current_role" yaml:"current_role"`
	DataDir      string `json:"data_dir" yaml:"data_dir"`
	PID          int    `json:"pid,omitempty" yaml:"pid,omitempty"`
	NoWait       bool   `json:"no_wait,omitempty" yaml:"no_wait,omitempty"`
}

// PromoteOK creates a successful result for pg promote operation.
func PromoteOK(timeline int, previousRole, currentRole, dataDir string, pid int) *output.Result {
	return output.OK("PostgreSQL standby promoted to primary successfully", &PgPromoteResultData{
		Promoted:     true,
		Timeline:     timeline,
		PreviousRole: previousRole,
		CurrentRole:  currentRole,
		DataDir:      dataDir,
		PID:          pid,
	})
}

// PromoteResult executes pg promote and returns a structured result.
// It validates the instance role before/after promotion and handles errors with appropriate codes.
func PromoteResult(cfg *Config, opts *PromoteOptions) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Pre-check: data directory initialization (permission-aware)
	exists, initialized, err := checkDataDirStateAsDBSU(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL data directory").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: "unknown",
				CurrentRole:  "unknown",
				DataDir:      dataDir,
			}).
			WithDetail(err.Error())
	}
	if !exists {
		return output.Fail(output.CodePgStatusDataDirNotFound,
			"PostgreSQL data directory not found").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: "unknown",
				CurrentRole:  "unknown",
				DataDir:      dataDir,
			}).
			WithDetail(fmt.Sprintf("data_dir=%s", dataDir))
	}
	if !initialized {
		return output.Fail(output.CodePgStatusNotInitialized,
			"Data directory not initialized").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: "unknown",
				CurrentRole:  "unknown",
				DataDir:      dataDir,
			}).
			WithDetail(fmt.Sprintf("data_dir=%s (run 'pig pg init' first)", dataDir))
	}

	// Pre-check: PostgreSQL must be running (permission-aware)
	running, pid, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgPermissionDenied,
			"Permission denied checking PostgreSQL status").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: "unknown",
				CurrentRole:  "unknown",
				DataDir:      dataDir,
			}).
			WithDetail(err.Error())
	}
	if !running {
		return output.Fail(output.CodePgNotRunning,
			"PostgreSQL is not running").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: "unknown",
				CurrentRole:  "unknown",
				DataDir:      dataDir,
			}).
			WithDetail("Cannot promote a stopped instance")
	}

	// Get previous role before promotion
	previousRole := detectRoleString(cfg)

	// Pre-check: if already primary, return state error
	if previousRole == "primary" {
		return output.Fail(output.CodePgAlreadyPrimary,
			"PostgreSQL is already primary").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: previousRole,
				CurrentRole:  previousRole,
				DataDir:      dataDir,
				PID:          pid,
			}).
			WithDetail("Instance is already primary, no promotion needed")
	}

	// Pre-check: if role is unknown, might be misconfigured for replication
	if previousRole == "unknown" {
		return output.Fail(output.CodePgReplicationNotConfigured,
			"Cannot determine instance role").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: previousRole,
				CurrentRole:  "unknown",
				DataDir:      dataDir,
				PID:          pid,
			}).
			WithDetail("Unable to detect if instance is primary or standby; replication may not be configured")
	}

	// Execute the promote operation
	err = Promote(cfg, opts)
	if err != nil {
		code := classifyCtlError(err, output.CodePgPromoteFailed)
		return output.Fail(code, "Failed to promote PostgreSQL standby").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				PreviousRole: previousRole,
				CurrentRole:  previousRole,
				DataDir:      dataDir,
				PID:          pid,
			}).
			WithDetail(err.Error())
	}

	// No-wait mode: do not wait for role change
	if opts != nil && opts.NoWait {
		return output.OK("Promotion initiated (no-wait mode)", &PgPromoteResultData{
			Promoted:     false,
			PreviousRole: previousRole,
			CurrentRole:  "unknown",
			DataDir:      dataDir,
			PID:          pid,
			NoWait:       true,
		}).WithDetail("Role change not verified; check 'pig pg role' or 'pig pg status'")
	}

	// Post-check: get current role after promotion (with retry for role change propagation)
	currentRole := waitForRoleChange(cfg, "primary", defaultRoleWaitTimeout)
	if currentRole == "" {
		currentRole = detectRoleString(cfg)
	}

	// Get timeline after promotion
	timeline := getTimeline(cfg, dbsu)

	// Verify promotion succeeded
	if currentRole != "primary" {
		return output.Fail(output.CodePgPromoteFailed,
			"Promotion completed but role did not change to primary").
			WithData(&PgPromoteResultData{
				Promoted:     false,
				Timeline:     timeline,
				PreviousRole: previousRole,
				CurrentRole:  currentRole,
				DataDir:      dataDir,
				PID:          pid,
			}).
			WithDetail(fmt.Sprintf("Expected role 'primary', got '%s'", currentRole))
	}

	return PromoteOK(timeline, previousRole, currentRole, dataDir, pid)
}

// detectRoleString returns the role as a string ("primary", "replica", or "unknown").
func detectRoleString(cfg *Config) string {
	result, err := DetectRole(cfg, nil)
	if err != nil {
		return "unknown"
	}
	if result == nil {
		return "unknown"
	}
	return string(result.Role)
}

// waitForRoleChange waits for the role to change to the expected value.
// Returns the detected role, or empty string if timeout.
func waitForRoleChange(cfg *Config, expectedRole string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		role := detectRoleString(cfg)
		if role == expectedRole {
			return role
		}
		time.Sleep(rolePollInterval)
	}
	return ""
}

// getTimeline retrieves the current timeline ID from PostgreSQL.
// It first tries SQL query, then falls back to pg_controldata.
func getTimeline(cfg *Config, dbsu string) int {
	// Try SQL query first (works when PostgreSQL is running and accepting connections)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		logrus.Debugf("cannot get PG install for timeline: %v", err)
		return 0
	}

	// Try pg_control_checkpoint() SQL function
	cmdArgs := []string{pg.Psql(), "-AXtqw", "-d", "postgres", "-c",
		"SELECT timeline_id FROM pg_control_checkpoint()"}
	output, err := utils.DBSUCommandOutput(dbsu, cmdArgs)
	if err == nil {
		output = strings.TrimSpace(output)
		if timeline, parseErr := strconv.Atoi(output); parseErr == nil {
			return timeline
		}
	}
	logrus.Debugf("SQL timeline query failed: %v", err)

	// Fall back to pg_controldata
	dataDir := GetPgData(cfg)
	cmdArgs = []string{pg.PgControldata(), "-D", dataDir}
	output, err = utils.DBSUCommandOutput(dbsu, cmdArgs)
	if err != nil {
		logrus.Debugf("pg_controldata failed: %v", err)
		return 0
	}

	// Parse "Latest checkpoint's TimeLineID: N"
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "TimeLineID") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				if timeline, parseErr := strconv.Atoi(strings.TrimSpace(parts[1])); parseErr == nil {
					return timeline
				}
			}
		}
	}

	return 0
}

// ============================================================================
// Helper Functions
// ============================================================================

// waitForPID waits for PostgreSQL to start and returns the PID.
// Returns 0 if the process doesn't start within the timeout.
func waitForPID(dbsu, dataDir string, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)
		if running && pid > 0 {
			return pid
		}
		time.Sleep(pidPollInterval)
	}
	return 0
}

// classifyCtlError examines an error from pg_ctl and returns an appropriate error code.
func classifyCtlError(err error, defaultCode int) int {
	if err == nil {
		return 0
	}

	errMsg := strings.ToLower(err.Error())

	// Permission errors
	if strings.Contains(errMsg, "permission denied") ||
		strings.Contains(errMsg, "operation not permitted") {
		return output.CodePgPermissionDenied
	}

	// Timeout errors
	if strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "timed out") ||
		strings.Contains(errMsg, "did not start in time") ||
		strings.Contains(errMsg, "did not stop in time") ||
		strings.Contains(errMsg, "did not promote in time") {
		return output.CodePgTimeout
	}

	// PostgreSQL not found
	if strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "no such file") {
		return output.CodePgNotFound
	}

	return defaultCode
}
