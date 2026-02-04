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

// PITR module specific codes (MODULE_PITR = 160000)
const (
	CodePITRInvalidArgs    = MODULE_PITR + CAT_PARAM + 1     // Invalid or missing arguments
	CodePITRPrecheckFailed = MODULE_PITR + CAT_STATE + 1     // Pre-check/validation failed
	CodePITRStopFailed     = MODULE_PITR + CAT_OPERATION + 1 // Stop services failed
	CodePITRRestoreFailed  = MODULE_PITR + CAT_OPERATION + 2 // Restore failed
	CodePITRStartFailed    = MODULE_PITR + CAT_OPERATION + 3 // Start PostgreSQL failed
	CodePITRPostFailed     = MODULE_PITR + CAT_OPERATION + 4 // Post-restore steps failed
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
