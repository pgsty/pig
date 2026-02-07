package cmd

import (
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

// patroniStatusCmd: pig pt status - comprehensive status check
var patroniStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "stat"},
	Short:   "Show comprehensive patroni status",
	Long: `Show comprehensive Patroni status including:
  1. Patroni service status (systemctl status patroni)
  2. Patroni processes (ps aux | grep patroni)
  3. Patroni cluster status (patronictl list)`,
	Example: `
  pig pt status          # Show comprehensive status
  pig pt status -o json  # Structured JSON output
  pig pt st              # Same as above (shortcut)`,
	Annotations: ancsAnn("pig patroni status", "query", "stable", "safe", true, "safe", "none", "dbsu", 3000),
	RunE: func(cmd *cobra.Command, args []string) error {
		dbsu := utils.GetDBSU(patroniDBSU)

		// Structured output
		if config.IsStructuredOutput() {
			result := patroni.StatusResult(dbsu)
			return handleAuxResult(result)
		}

		// Default passthrough
		return patroni.Status(dbsu)
	},
}

// ============================================================================
// Service Shortcuts (via systemctl) - pig pt start/stop
// ============================================================================

// patroniStartCmd: pig pt start - shortcut for pig pt svc start
var patroniStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"boot", "up"},
	Short:       "Start patroni service (shortcut for 'svc start')",
	Annotations: ancsAnn("pig patroni start", "action", "volatile", "unsafe", true, "medium", "none", "root", 10000),
	Long:        `Start the Patroni daemon service using systemctl. This is a shortcut for 'pig pt svc start'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni start", args, nil, func() error {
			return patroni.Systemctl("start")
		})
	},
}

// patroniStopCmd: pig pt stop - shortcut for pig pt svc stop
var patroniStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"halt", "dn", "down"},
	Short:       "Stop patroni service (shortcut for 'svc stop')",
	Annotations: ancsAnn("pig patroni stop", "action", "volatile", "unsafe", true, "high", "recommended", "root", 10000),
	Long:        `Stop the Patroni daemon service using systemctl. This is a shortcut for 'pig pt svc stop'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni stop", args, nil, func() error {
			return patroni.Systemctl("stop")
		})
	},
}

// ============================================================================
// Service Management (via systemctl) - pig pt svc
// ============================================================================

var patroniSvcCmd = &cobra.Command{
	Use:         "service",
	Aliases:     []string{"svc", "s"},
	Short:       "Manage patroni daemon service",
	Annotations: ancsAnn("pig patroni service", "query", "stable", "safe", true, "safe", "none", "root", 100),
	Long: `Manage the Patroni daemon service using systemctl.

These commands control the Patroni process itself, not the PostgreSQL
instances it manages. For PostgreSQL operations, use:
  - pig pt restart   (restart PostgreSQL via patronictl)
  - pig pt reload    (reload PostgreSQL config)`,
}

var patroniSvcStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"boot", "up"},
	Short:       "Start patroni service",
	Annotations: ancsAnn("pig patroni service start", "action", "volatile", "unsafe", true, "medium", "none", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service start", args, nil, func() error {
			return patroni.Systemctl("start")
		})
	},
}

var patroniSvcStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"halt", "dn", "down"},
	Short:       "Stop patroni service",
	Annotations: ancsAnn("pig patroni service stop", "action", "volatile", "unsafe", true, "high", "recommended", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service stop", args, nil, func() error {
			return patroni.Systemctl("stop")
		})
	},
}

var patroniSvcRestartCmd = &cobra.Command{
	Use:         "restart",
	Aliases:     []string{"reboot", "rt"},
	Short:       "Restart patroni service",
	Annotations: ancsAnn("pig patroni service restart", "action", "volatile", "unsafe", false, "high", "recommended", "root", 30000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service restart", args, nil, func() error {
			return patroni.Systemctl("restart")
		})
	},
}

var patroniSvcReloadCmd = &cobra.Command{
	Use:         "reload",
	Aliases:     []string{"rl", "hup"},
	Short:       "Reload patroni service",
	Annotations: ancsAnn("pig patroni service reload", "action", "volatile", "restricted", true, "low", "none", "root", 1000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service reload", args, nil, func() error {
			return patroni.Systemctl("reload")
		})
	},
}

var patroniSvcStatusCmd = &cobra.Command{
	Use:         "status",
	Aliases:     []string{"st", "stat"},
	Short:       "Show patroni service status",
	Annotations: ancsAnn("pig patroni service status", "query", "volatile", "safe", true, "safe", "none", "root", 500),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service status", args, nil, func() error {
			return patroni.Systemctl("status")
		})
	},
}
