package output

// Status code structure follows the 222 pattern: MMCCNN
// MM: Module code (00-99)
// CC: Category code (00-99)
// NN: Specific error code (00-99)

// Module codes (MM) - identifies which subsystem generated the result
// Module codes start from 10 to avoid octal literal issues (no leading zeros)
const (
	MODULE_EXT    = 100000 // Extension management (MM=10)
	MODULE_REPO   = 110000 // Repository management (MM=11)
	MODULE_BUILD  = 120000 // Build system (MM=12)
	MODULE_PG     = 130000 // PostgreSQL control (MM=13)
	MODULE_PB     = 140000 // pgBackRest (MM=14)
	MODULE_PT     = 150000 // Patroni (MM=15)
	MODULE_PITR   = 160000 // PITR recovery (MM=16)
	MODULE_PE     = 170000 // pg_exporter (MM=17)
	MODULE_CTX    = 180000 // Context (MM=18)
	MODULE_STY    = 200000 // Pigsty management (MM=20)
	MODULE_DO     = 210000 // Task orchestration (MM=21)
	MODULE_CONFIG = 900000 // Configuration system (MM=90)
	MODULE_SYSTEM = 990000 // System-level errors (MM=99)
)

// Category codes (CC) - classifies the type of result/error
const (
	CAT_SUCCESS   = 0   // Success/informational
	CAT_PARAM     = 100 // Parameter/usage errors
	CAT_PERM      = 200 // Permission errors
	CAT_DEPEND    = 300 // Dependency errors
	CAT_NETWORK   = 400 // Network errors
	CAT_RESOURCE  = 500 // Resource errors
	CAT_STATE     = 600 // State errors
	CAT_CONFIG    = 700 // Configuration errors
	CAT_OPERATION = 800 // Operation errors
	CAT_INTERNAL  = 900 // Internal errors
)

// Extension module specific codes (MODULE_EXT = 100000)
const (
	CodeExtensionNotFound      = MODULE_EXT + CAT_RESOURCE + 1  // Extension not found in catalog
	CodeExtensionNoPackage     = MODULE_EXT + CAT_RESOURCE + 2  // Extension has no package for current OS/PG
	CodeExtensionCatalogError  = MODULE_EXT + CAT_CONFIG + 1    // Catalog loading/parsing error
	CodeExtensionNoPG          = MODULE_EXT + CAT_STATE + 1     // No PostgreSQL installation found
	CodeExtensionUnsupportedOS = MODULE_EXT + CAT_STATE + 2     // Operating system not supported
	CodeExtensionPgConfigError = MODULE_EXT + CAT_STATE + 3     // pg_config detection/validation error
	CodeExtensionInvalidArgs   = MODULE_EXT + CAT_PARAM + 1     // Invalid or missing arguments
	CodeExtensionInstallFailed = MODULE_EXT + CAT_OPERATION + 1 // Package manager installation failed
	CodeExtensionRemoveFailed  = MODULE_EXT + CAT_OPERATION + 2 // Package manager removal failed
	CodeExtensionUpdateFailed  = MODULE_EXT + CAT_OPERATION + 3 // Package manager update failed
	CodeExtensionImportFailed  = MODULE_EXT + CAT_OPERATION + 4 // Package download/import failed
	CodeExtensionLinkFailed    = MODULE_EXT + CAT_OPERATION + 5 // PostgreSQL link/unlink failed
	CodeExtensionReloadFailed  = MODULE_EXT + CAT_OPERATION + 6 // Catalog reload failed
)

// Repository module specific codes (MODULE_REPO = 110000)
const (
	CodeRepoInvalidArgs       = MODULE_REPO + CAT_PARAM + 1     // Invalid or missing arguments
	CodeRepoNotFound          = MODULE_REPO + CAT_RESOURCE + 1  // Repository not found
	CodeRepoModuleNotFound    = MODULE_REPO + CAT_RESOURCE + 2  // Module not found
	CodeRepoPackageNotFound   = MODULE_REPO + CAT_RESOURCE + 3  // Offline package not found
	CodeRepoDirNotFound       = MODULE_REPO + CAT_RESOURCE + 4  // Directory not found
	CodeRepoManagerError      = MODULE_REPO + CAT_CONFIG + 1    // Repository manager initialization error
	CodeRepoUnsupportedOS     = MODULE_REPO + CAT_STATE + 1     // Operating system not supported for repo operations
	CodeRepoAddFailed         = MODULE_REPO + CAT_OPERATION + 1 // Add repository failed
	CodeRepoBackupFailed      = MODULE_REPO + CAT_OPERATION + 2 // Backup repository failed
	CodeRepoUpdateFailed      = MODULE_REPO + CAT_OPERATION + 3 // Update cache failed
	CodeRepoRemoveFailed      = MODULE_REPO + CAT_OPERATION + 4 // Remove repository failed
	CodeRepoCacheUpdateFailed = MODULE_REPO + CAT_OPERATION + 5 // Cache update failed
	CodeRepoCreateFailed      = MODULE_REPO + CAT_OPERATION + 6 // Create local repository failed
	CodeRepoBootFailed        = MODULE_REPO + CAT_OPERATION + 7 // Boot from offline package failed
	CodeRepoCacheFailed       = MODULE_REPO + CAT_OPERATION + 8 // Cache/pack operation failed
	CodeRepoReloadFailed      = MODULE_REPO + CAT_OPERATION + 9 // Reload catalog failed
)

// PostgreSQL module specific codes (MODULE_PG = 130000)
const (
	// Status command codes (13_06_xx - State category)
	CodePgStatusNotRunning       = MODULE_PG + CAT_STATE + 1    // PostgreSQL is not running
	CodePgStatusNotInitialized   = MODULE_PG + CAT_STATE + 2    // Data directory not initialized
	CodePgStatusDataDirNotFound  = MODULE_PG + CAT_RESOURCE + 1 // Data directory not found
	CodePgInitDirExists          = MODULE_PG + CAT_RESOURCE + 2 // Data directory already initialized
	CodePgStatusPermissionDenied = MODULE_PG + CAT_PERM + 1     // Permission denied reading status

	// Control operation state errors (13_06_xx - State category)
	CodePgAlreadyRunning      = MODULE_PG + CAT_STATE + 3 // PostgreSQL is already running (start failed)
	CodePgAlreadyStopped      = MODULE_PG + CAT_STATE + 4 // PostgreSQL is already stopped (stop failed)
	CodePgNotRunning          = MODULE_PG + CAT_STATE + 5 // PostgreSQL not running (reload/restart failed)
	CodePgInitRunningConflict = MODULE_PG + CAT_STATE + 6 // PostgreSQL running, cannot init with --force

	// Control operation errors (13_08_xx - Operation category)
	CodePgStartFailed   = MODULE_PG + CAT_OPERATION + 1 // pg_ctl start failed
	CodePgStopFailed    = MODULE_PG + CAT_OPERATION + 2 // pg_ctl stop failed
	CodePgRestartFailed = MODULE_PG + CAT_OPERATION + 3 // pg_ctl restart failed
	CodePgReloadFailed  = MODULE_PG + CAT_OPERATION + 4 // pg_ctl reload failed
	CodePgTimeout       = MODULE_PG + CAT_OPERATION + 5 // Operation timed out
	CodePgInitFailed    = MODULE_PG + CAT_OPERATION + 6 // initdb failed
	CodePgPromoteFailed = MODULE_PG + CAT_OPERATION + 7 // pg_ctl promote failed

	// Permission errors (13_02_xx - Permission category)
	CodePgPermissionDenied = MODULE_PG + CAT_PERM + 2 // Permission denied executing pg_ctl

	// Dependency errors (13_03_xx - Dependency category)
	CodePgNotFound = MODULE_PG + CAT_DEPEND + 1 // PostgreSQL installation not found

	// Promote-specific state errors (13_06_xx - State category)
	CodePgAlreadyPrimary           = MODULE_PG + CAT_STATE + 7  // Instance is already primary (promote unnecessary)
	CodePgReplicationNotConfigured = MODULE_PG + CAT_CONFIG + 1 // Replication not configured (cannot determine role)
)

// pgBackRest module specific codes (MODULE_PB = 140000)
const (
	// Parameter errors (14_01_xx - Param category)
	CodePbInvalidBackupType         = MODULE_PB + CAT_PARAM + 1 // Invalid backup type specified
	CodePbInvalidRestoreParams      = MODULE_PB + CAT_PARAM + 2 // Invalid restore parameters
	CodePbStanzaDeleteRequiresForce = MODULE_PB + CAT_PARAM + 3 // Stanza delete requires --force
	CodePbInvalidInfoParams         = MODULE_PB + CAT_PARAM + 4 // Invalid info command parameters

	// Permission errors (14_02_xx - Permission category)
	CodePbPermissionDenied = MODULE_PB + CAT_PERM + 1 // Permission denied accessing pgBackRest

	// Dependency errors (14_03_xx - Depend category)
	CodePbNoBaseBackup = MODULE_PB + CAT_DEPEND + 1 // No base backup exists for incremental backup

	// Resource errors (14_05_xx - Resource category)
	CodePbStanzaNotFound = MODULE_PB + CAT_RESOURCE + 1 // Stanza not found or not configured
	CodePbBackupNotFound = MODULE_PB + CAT_RESOURCE + 2 // Specified backup not found

	// State errors (14_06_xx - State category)
	CodePbNotPrimary   = MODULE_PB + CAT_STATE + 1 // Instance is not primary, backup requires primary
	CodePbPgNotRunning = MODULE_PB + CAT_STATE + 2 // PostgreSQL is not running
	CodePbPgRunning    = MODULE_PB + CAT_STATE + 3 // PostgreSQL is running (cannot restore)
	CodePbStanzaExists = MODULE_PB + CAT_STATE + 4 // Stanza already exists (use --force)

	// Configuration errors (14_07_xx - Config category)
	CodePbConfigNotFound = MODULE_PB + CAT_CONFIG + 1 // pgBackRest configuration file not found

	// Operation errors (14_08_xx - Operation category)
	CodePbInfoFailed          = MODULE_PB + CAT_OPERATION + 1 // Info command execution failed
	CodePbBackupFailed        = MODULE_PB + CAT_OPERATION + 2 // Backup command execution failed
	CodePbRestoreFailed       = MODULE_PB + CAT_OPERATION + 3 // Restore command execution failed
	CodePbStanzaCreateFailed  = MODULE_PB + CAT_OPERATION + 4 // Stanza create failed
	CodePbStanzaUpgradeFailed = MODULE_PB + CAT_OPERATION + 5 // Stanza upgrade failed
	CodePbStanzaDeleteFailed  = MODULE_PB + CAT_OPERATION + 6 // Stanza delete failed
)

// Patroni module specific codes (MODULE_PT = 150000)
const (
	// Dependency errors (15_03_xx - Depend category)
	CodePtNotFound = MODULE_PT + CAT_DEPEND + 1 // patronictl not found in PATH

	// Permission errors (15_02_xx - Permission category)
	CodePtPermDenied = MODULE_PT + CAT_PERM + 1 // Permission denied accessing patronictl

	// State errors (15_06_xx - State category)
	CodePtNotRunning = MODULE_PT + CAT_STATE + 1 // Patroni is not running

	// Configuration errors (15_07_xx - Config category)
	CodePtConfigNotFound = MODULE_PT + CAT_CONFIG + 1 // Patroni config file not found

	// Parameter errors (15_01_xx - Param category)
	CodePtSwitchoverNeedForce = MODULE_PT + CAT_PARAM + 1 // switchover requires --force in structured output mode

	// Operation errors (15_08_xx - Operation category)
	CodePtListFailed       = MODULE_PT + CAT_OPERATION + 1 // patronictl list execution failed
	CodePtParseFailed      = MODULE_PT + CAT_OPERATION + 2 // patronictl output parse failed
	CodePtStatusFailed     = MODULE_PT + CAT_OPERATION + 3 // patronictl status command failed
	CodePtConfigShowFailed = MODULE_PT + CAT_OPERATION + 4 // patronictl show-config execution failed
	CodePtSwitchoverFailed = MODULE_PT + CAT_OPERATION + 5 // patronictl switchover execution failed
	CodePtFailoverFailed   = MODULE_PT + CAT_OPERATION + 6 // patronictl failover execution failed

	// Parameter errors (15_01_xx - Param category)
	CodePtFailoverNeedForce   = MODULE_PT + CAT_PARAM + 2 // failover requires --force in structured output mode
	CodePtInvalidConfigAction = MODULE_PT + CAT_PARAM + 3 // invalid pt config action
)

// PITR module specific codes (MODULE_PITR = 160000)
const (
	CodePITRInvalidArgs    = MODULE_PITR + CAT_PARAM + 1     // Invalid or missing arguments (160101)
	CodePITRNoBackup       = MODULE_PITR + CAT_DEPEND + 1    // Backup not found or unavailable (160301)
	CodePITRPrecheckFailed = MODULE_PITR + CAT_STATE + 1     // Pre-check/validation failed (160601)
	CodePITRPgRunning      = MODULE_PITR + CAT_STATE + 2     // PostgreSQL cannot be stopped (160602)
	CodePITRStopFailed     = MODULE_PITR + CAT_OPERATION + 1 // Stop services failed (160801)
	CodePITRRestoreFailed  = MODULE_PITR + CAT_OPERATION + 2 // Restore failed (160802)
	CodePITRStartFailed    = MODULE_PITR + CAT_OPERATION + 3 // Start PostgreSQL failed (160803)
	CodePITRPostFailed     = MODULE_PITR + CAT_OPERATION + 4 // Post-restore steps failed (160804)
)

// Context module specific codes (MODULE_CTX = 180000)
const (
	CodeCtxInvalidModule    = MODULE_CTX + CAT_PARAM + 1     // Invalid module name
	CodeCtxPermissionDenied = MODULE_CTX + CAT_PERM + 1      // Permission denied accessing resources
	CodeCtxCollectionFailed = MODULE_CTX + CAT_OPERATION + 1 // Information collection failed
)

// Pigsty module specific codes (MODULE_STY = 200000)
const (
	CodeStyConfigureInvalidArgs      = MODULE_STY + CAT_PARAM + 1     // Invalid configure arguments
	CodeStyConfigureTemplateNotFound = MODULE_STY + CAT_RESOURCE + 1  // Configure template file not found
	CodeStyConfigureFailed           = MODULE_STY + CAT_OPERATION + 1 // Configure generation failed
	CodeStyConfigureWriteFailed      = MODULE_STY + CAT_OPERATION + 2 // Configure output write failed
)

// System module specific codes (MODULE_SYSTEM = 990000)
const (
	CodeSystemInvalidArgs   = MODULE_SYSTEM + CAT_PARAM + 1     // Invalid command/flag/arguments
	CodeSystemCommandFailed = MODULE_SYSTEM + CAT_OPERATION + 1 // Unclassified command execution failure
)

// ExitCode converts a status code to a shell exit code.
// It extracts the category (CC) from the 222 structure (MMCCNN) and maps it to exit codes.
//
// Exit code mapping:
//   - CC=00 (success/info) → Exit 0
//   - CC=01 (param/usage) → Exit 2
//   - CC=02 (permission) → Exit 3
//   - CC=03 (dependency) → Exit 4
//   - CC=04 (network) → Exit 5
//   - CC=05 (resource) → Exit 6
//   - CC=06 (state) → Exit 9
//   - CC=07 (config) → Exit 8
//   - CC=08 (operation) → Exit 1
//   - CC=09 (internal) → Exit 1
func ExitCode(code int) int {
	if code == 0 {
		return 0
	}

	// Handle invalid negative codes
	if code < 0 {
		return 1
	}

	// Extract category (CC) from MMCCNN structure
	// CC is the hundreds digit of the last 4 digits
	category := (code % 10000) / 100

	switch category {
	case 0: // Success/informational
		return 0
	case 1: // Parameter/usage errors
		return 2
	case 2: // Permission errors
		return 3
	case 3: // Dependency errors
		return 4
	case 4: // Network errors
		return 5
	case 5: // Resource errors
		return 6
	case 6: // State errors
		return 9
	case 7: // Configuration errors
		return 8
	case 8, 9: // Operation/Internal errors
		return 1
	default:
		return 1
	}
}
