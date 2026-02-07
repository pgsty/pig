package cmd

import (
	"pig/cli/postgres"

	"github.com/spf13/cobra"
)

// ============================================================================
// Service Management Commands (via systemctl) - pig pg svc
// ============================================================================

var pgSvcCmd = &cobra.Command{
	Use:         "service",
	Aliases:     []string{"svc", "s"},
	Short:       "Manage postgres systemd service",
	Annotations: ancsAnn("pig postgres service", "query", "stable", "safe", true, "safe", "none", "root", 100),
	Long: `Manage the PostgreSQL systemd service.

These commands control the postgres service via systemctl. Unlike the pg_ctl
commands (pig pg start/stop/restart/reload), these operate through systemd.

Use these commands when PostgreSQL is managed as a systemd service.
For direct pg_ctl operations, use the parent commands instead.`,
}

var pgSvcStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"boot", "up"},
	Short:       "Start postgres systemd service",
	Annotations: ancsAnn("pig postgres service start", "action", "volatile", "unsafe", true, "medium", "none", "root", 10000),
	Example:     `  pig pg svc start                 # systemctl start postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePg, "pig postgres service start", args, nil, func() error {
			return postgres.RunSystemctl("start", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"halt", "dn", "down"},
	Short:       "Stop postgres systemd service",
	Annotations: ancsAnn("pig postgres service stop", "action", "volatile", "unsafe", true, "high", "recommended", "root", 10000),
	Example:     `  pig pg svc stop                  # systemctl stop postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePg, "pig postgres service stop", args, nil, func() error {
			return postgres.RunSystemctl("stop", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcRestartCmd = &cobra.Command{
	Use:         "restart",
	Aliases:     []string{"reboot", "rt"},
	Short:       "Restart postgres systemd service",
	Annotations: ancsAnn("pig postgres service restart", "action", "volatile", "unsafe", false, "high", "recommended", "root", 30000),
	Example:     `  pig pg svc restart               # systemctl restart postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePg, "pig postgres service restart", args, nil, func() error {
			return postgres.RunSystemctl("restart", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcReloadCmd = &cobra.Command{
	Use:         "reload",
	Aliases:     []string{"rl", "hup"},
	Short:       "Reload postgres systemd service",
	Annotations: ancsAnn("pig postgres service reload", "action", "volatile", "restricted", true, "low", "none", "root", 1000),
	Example:     `  pig pg svc reload                # systemctl reload postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePg, "pig postgres service reload", args, nil, func() error {
			return postgres.RunSystemctl("reload", postgres.DefaultSystemdService)
		})
	},
}

var pgSvcStatusCmd = &cobra.Command{
	Use:         "status",
	Aliases:     []string{"st", "stat"},
	Short:       "Show postgres systemd service status",
	Annotations: ancsAnn("pig postgres service status", "query", "volatile", "safe", true, "safe", "none", "root", 500),
	Example:     `  pig pg svc status                # systemctl status postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePg, "pig postgres service status", args, nil, func() error {
			return postgres.RunSystemctl("status", postgres.DefaultSystemdService)
		})
	},
}
