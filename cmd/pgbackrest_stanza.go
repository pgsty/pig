package cmd

import (
	"pig/cli/pgbackrest"
	"pig/internal/config"

	"github.com/spf13/cobra"
)

// ============================================================================
// Stanza Management Commands
// ============================================================================

var pbCreateNoOnline bool
var pbCreateForce bool

var pbCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"cr"},
	Short:   "Create stanza (stanza-create)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest create",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Initialize a new stanza. Must be run before the first backup.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.CreateOptions{
			NoOnline: pbCreateNoOnline,
			Force:    pbCreateForce,
		}

		if config.IsStructuredOutput() {
			result := pgbackrest.CreateResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Create(pbConfig, opts)
	},
}

var pbUpgradeNoOnline bool

var pbUpgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Aliases: []string{"up"},
	Short:   "Upgrade stanza (stanza-upgrade)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest upgrade",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Update stanza after PostgreSQL major version upgrade.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.UpgradeOptions{
			NoOnline: pbUpgradeNoOnline,
		}

		if config.IsStructuredOutput() {
			result := pgbackrest.UpgradeResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Upgrade(pbConfig, opts)
	},
}

var pbDeleteForce bool
var pbDeleteYes bool

var pbDeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"del", "rm"},
	Short:   "Delete stanza (stanza-delete)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest delete",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "critical",
		"confirm":    "required",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Delete a stanza and all its backups.

WARNING: This is a DESTRUCTIVE and IRREVERSIBLE operation!
All backups for the stanza will be permanently deleted.

	Requires --force flag to confirm the operation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.DeleteOptions{
			Force: pbDeleteForce,
			Yes:   pbDeleteYes,
		}

		if config.IsStructuredOutput() {
			// Structured mode implicitly skips confirmation (equivalent to --yes)
			opts.Yes = true
			result := pgbackrest.DeleteResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Delete(pbConfig, opts)
	},
}
