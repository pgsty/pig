package cmd

import (
	"pig/cli/postgres"

	"github.com/spf13/cobra"
)

// ============================================================================
// Service Management Commands (via systemctl) - pig pg svc
// ============================================================================

var pgSvcCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"svc", "s"},
	Short:   "Manage postgres systemd service",
	Annotations: map[string]string{
		"name":       "pig postgres service",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "100",
	},
	Long: `Manage the PostgreSQL systemd service.

These commands control the postgres service via systemctl. Unlike the pg_ctl
commands (pig pg start/stop/restart/reload), these operate through systemd.

Use these commands when PostgreSQL is managed as a systemd service.
For direct pg_ctl operations, use the parent commands instead.`,
}

var pgSvcStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"boot", "up"},
	Short:   "Start postgres systemd service",
	Annotations: map[string]string{
		"name":       "pig postgres service start",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "10000",
	},
	Example: `  pig pg svc start                 # systemctl start postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgLegacy("pig postgres service start", args, nil, func() error {
			return postgres.RunSystemctl("start", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"halt", "dn", "down"},
	Short:   "Stop postgres systemd service",
	Annotations: map[string]string{
		"name":       "pig postgres service stop",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "10000",
	},
	Example: `  pig pg svc stop                  # systemctl stop postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgLegacy("pig postgres service stop", args, nil, func() error {
			return postgres.RunSystemctl("stop", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcRestartCmd = &cobra.Command{
	Use:     "restart",
	Aliases: []string{"reboot", "rt"},
	Short:   "Restart postgres systemd service",
	Annotations: map[string]string{
		"name":       "pig postgres service restart",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "30000",
	},
	Example: `  pig pg svc restart               # systemctl restart postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgLegacy("pig postgres service restart", args, nil, func() error {
			return postgres.RunSystemctl("restart", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcReloadCmd = &cobra.Command{
	Use:     "reload",
	Aliases: []string{"rl", "hup"},
	Short:   "Reload postgres systemd service",
	Annotations: map[string]string{
		"name":       "pig postgres service reload",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "1000",
	},
	Example: `  pig pg svc reload                # systemctl reload postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgLegacy("pig postgres service reload", args, nil, func() error {
			return postgres.RunSystemctl("reload", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "stat"},
	Short:   "Show postgres systemd service status",
	Annotations: map[string]string{
		"name":       "pig postgres service status",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "500",
	},
	Example: `  pig pg svc status                # systemctl status postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgLegacy("pig postgres service status", args, nil, func() error {
			return postgres.RunSystemctl("status", postgres.DefaultSystemdService)
		})
	},
}
