package cmd

import (
	"strings"

	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

var (
	patroniDBSU       string
	patroniSetYes     bool
	patroniSetPlan    bool
	patroniLogFollow  bool
	patroniLogLines   int
	patroniLogDir     string
	patroniConfigFile string
	patroniDCSURL     string
	patroniInsecure   bool
)

var (
	patroniPassthroughExec = patroni.Passthrough
	patroniSetExec         = patroni.ApplySet
	patroniSystemctlExec   = patroni.Systemctl
)

// patroniCmd is deliberately a runnable parent. Once pflag encounters the
// first non-option token, every remaining token belongs to patronictl.
var patroniCmd = &cobra.Command{
	Use:         "patroni [options] <command> [arguments...]",
	Aliases:     []string{"pt"},
	Short:       "Run patronictl with Pigsty service and config helpers",
	GroupID:     "pigsty",
	Annotations: ancsAnn("pig patroni", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 300000),
	Long: `pig pt transparently launches the installed patronictl binary.

Pig only owns config-file selection and these local helpers:
  pig pt set KEY=VALUE...       update PostgreSQL or scalar Patroni settings
  pig pt start|stop             manage the local patroni service
  pig pt service <action>       start, stop, restart, reload, or show service
  pig pt status                 show local and cluster status
  pig pt log ...                view local Patroni logs

Every other command and all arguments after it are passed unchanged to
patronictl. Use "pig pt -- COMMAND ..." to bypass a local-name collision.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runtimeCommandError(cmd, patroniPassthroughExec(utils.GetDBSU(patroniDBSU), args))
	},
}

func registerPatroniCommand() *cobra.Command {
	patroniCmd.PersistentPreRunE = patroniCommandPreRun
	patroniCmd.Flags().SetInterspersed(false)

	patroniCmd.PersistentFlags().StringVarP(&patroniConfigFile, "config-file", "c", "", "Patroni/patronictl configuration file")
	patroniCmd.PersistentFlags().StringVar(&patroniDBSU, "dbsu", "", "OS user used to execute patronictl (default: postgres)")
	patroniCmd.PersistentFlags().StringVarP(&patroniDCSURL, "dcs-url", "d", "", "override the Patroni DCS URL")
	patroniCmd.PersistentFlags().StringVar(&patroniDCSURL, "dcs", "", "alias for --dcs-url")
	patroniCmd.PersistentFlags().BoolVarP(&patroniInsecure, "insecure", "k", false, "allow TLS without certificate verification")

	patroniSetCmd.Flags().BoolVarP(&patroniSetYes, "yes", "y", false, "skip patronictl confirmation")
	patroniSetCmd.Flags().BoolVar(&patroniSetPlan, "plan", false, "show the translated command without changing configuration")

	patroniLogCmd.PersistentFlags().StringVar(&patroniLogDir, "log-dir", "", "log directory (default: selected config log.dir, then /pg/log/patroni)")
	patroniLogCmd.Flags().BoolVarP(&patroniLogFollow, "follow", "f", false, "follow log output")
	patroniLogCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogTailCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogTailCmd.Flags().BoolP("follow", "f", false, "(no-op: tail always follows)")
	patroniLogCatCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogGrepCmd.Flags().IntP("lines", "n", 0, "search only the last N lines")

	patroniLogCmd.AddCommand(patroniLogCatCmd, patroniLogTailCmd, patroniLogGrepCmd)
	patroniSvcCmd.AddCommand(
		patroniSvcStartCmd,
		patroniSvcStopCmd,
		patroniSvcRestartCmd,
		patroniSvcReloadCmd,
		patroniSvcStatusCmd,
	)
	patroniCmd.AddCommand(
		patroniSetCmd,
		patroniLogCmd,
		patroniStatusCmd,
		patroniStartCmd,
		patroniStopCmd,
		patroniSvcCmd,
	)
	return patroniCmd
}

func patroniCommandPreRun(cmd *cobra.Command, args []string) error {
	if err := commandModulePreRun(cmd, args); err != nil {
		return err
	}
	patroni.SetCommandOptions(patroni.CommandOptions{
		ConfigFile: patroniConfigFile,
		DCSURL:     patroniDCSURL,
		Insecure:   patroniInsecure,
	})

	// A native passthrough has no Pig structured representation. Reject it
	// before config probing or process execution.
	if cmd == patroniCmd && len(args) > 0 && config.IsStructuredOutput() {
		return structuredParamError(
			output.MODULE_PT,
			"pig patroni",
			"native patronictl forwarding supports text output only",
			"use Patronictl's native output option after its command token",
			nil,
			nil,
		)
	}

	if !patroniCommandNeedsConfig(cmd, args) {
		return nil
	}
	return patroni.ResolveCommandOptions(utils.GetDBSU(patroniDBSU))
}

func patroniCommandNeedsConfig(cmd *cobra.Command, args []string) bool {
	if cmd == patroniCmd && len(args) == 0 {
		return false
	}
	// Native forwarding and mutating set execution resolve inside Passthrough,
	// after patronictl has been found in the caller's PATH.
	if cmd == patroniCmd || (cmd == patroniSetCmd && !patroniSetPlan) {
		return false
	}
	if (cmd == patroniLogCmd || cmd == patroniLogCatCmd || cmd == patroniLogTailCmd || cmd == patroniLogGrepCmd) && patroniLogDir != "" {
		return false
	}
	if cmd == patroniStartCmd || cmd == patroniStopCmd || cmd == patroniSvcCmd {
		return false
	}
	for _, serviceCmd := range []*cobra.Command{
		patroniSvcStartCmd,
		patroniSvcStopCmd,
		patroniSvcRestartCmd,
		patroniSvcReloadCmd,
		patroniSvcStatusCmd,
	} {
		if cmd == serviceCmd {
			return false
		}
	}
	return true
}

var patroniSetCmd = &cobra.Command{
	Use:         "set <key=value>...",
	Short:       "Set PostgreSQL parameters or scalar Patroni controls",
	Args:        cobra.ArbitraryArgs,
	Annotations: ancsAnn("pig patroni set", "action", "volatile", "restricted", false, "medium", "recommended", "dbsu", 3000),
	Example: `
  pig pt set max_connections=200
  pig pt set ttl=60 synchronous_mode=on --yes
  pig pt set shared_buffers=4GB --plan`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pairs, err := patroni.ParseSetPairs(args)
		if err != nil {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni set",
				"invalid configuration arguments",
				err.Error(),
				args,
				nil,
			)
		}

		if patroniSetPlan {
			return handlePlanOutput(patroni.BuildSetPlan(pairs))
		}
		if config.IsStructuredOutput() && !patroniSetYes {
			return requireStructuredConfirmation(
				"pt",
				output.CodePtConfirmationRequired,
				"pig patroni set requires --yes (-y)",
				"pig patroni set",
				"pt:dcs-config",
				"medium",
				"",
				"",
			)
		}

		dbsu := utils.GetDBSU(patroniDBSU)
		pgPairs := patroni.PostgreSQLSetTokens(pairs)
		structured := config.IsStructuredOutput()
		nextActions := patroni.SetNextActions(pgPairs)
		return runtimeCommandError(cmd, runLegacyStructuredWithNextActions(
			legacyModulePt,
			"pig patroni set",
			args,
			map[string]interface{}{"count": len(pairs)},
			nextActions,
			func() error {
				if err := patroniSetExec(dbsu, pairs, patroniSetYes); err != nil {
					return err
				}
				if !structured {
					printPatroniSetHints(pgPairs)
				}
				return nil
			},
		))
	},
}

func printPatroniSetHints(pgPairs []string) {
	if len(pgPairs) == 0 {
		return
	}
	analysis := patroni.AnalyzePGConfigPairs(pgPairs)
	if analysis.RequiresRestart {
		utils.PrintWarn("If changed, these PostgreSQL parameters require a restart: %s", strings.Join(analysis.RestartParams, ", "))
		utils.PrintInfo("If the change was applied, verify with:")
		utils.PrintHint([]string{"pig", "pt", "list"})
		utils.PrintInfo("If members report pending restart, run: pig pt restart CLUSTER --pending")
		return
	}
	utils.PrintInfo("If the change was applied, verify with:")
	utils.PrintHint([]string{"pig", "pt", "list"})
}

var patroniLogCmd = &cobra.Command{
	Use:         "log",
	Aliases:     []string{"l"},
	Short:       "View patroni logs",
	Annotations: ancsAnn("pig patroni log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if patroniLogFollow {
			return runLogStream(output.MODULE_PT, "pig patroni log", "pig pt log show", patroniLogLines, args,
				map[string]interface{}{"follow": true, "lines": patroniLogLines}, func() error {
					return patroni.LogTail(patroniLogDir, utils.GetDBSU(patroniDBSU), patroniLogLines)
				})
		}
		return runLogSnapshot("pig pt log", patroniLogLines,
			func() error { return patroni.LogShowJSONL(patroniLogDir, utils.GetDBSU(patroniDBSU), patroniLogLines) },
			func() error {
				logDir := patroni.LogDir(patroniLogDir, utils.GetDBSU(patroniDBSU))
				return runLegacyStructured(legacyModulePt, "pig patroni log", args, map[string]interface{}{
					"log_dir": logDir,
					"follow":  false,
					"lines":   patroniLogLines,
				}, func() error { return patroni.LogCat(logDir, utils.GetDBSU(patroniDBSU), patroniLogLines) })
			})
	},
}

var patroniLogCatCmd = &cobra.Command{
	Use:         "show",
	Aliases:     []string{"cat", "c", "s"},
	Short:       "Output recent patroni logs",
	Annotations: ancsAnn("pig patroni log show", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogSnapshot("pig pt log show", patroniLogLines,
			func() error { return patroni.LogShowJSONL(patroniLogDir, utils.GetDBSU(patroniDBSU), patroniLogLines) },
			func() error {
				logDir := patroni.LogDir(patroniLogDir, utils.GetDBSU(patroniDBSU))
				return runLegacyStructured(legacyModulePt, "pig patroni log show", args, map[string]interface{}{
					"log_dir": logDir,
					"lines":   patroniLogLines,
				}, func() error { return patroni.LogCat(logDir, utils.GetDBSU(patroniDBSU), patroniLogLines) })
			})
	},
}

var patroniLogTailCmd = &cobra.Command{
	Use:         "tail",
	Aliases:     []string{"t", "f", "follow"},
	Short:       "Tail patroni logs",
	Annotations: ancsAnn("pig patroni log tail", "query", "volatile", "safe", true, "safe", "none", "dbsu", 0),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogStream(output.MODULE_PT, "pig patroni log tail", "pig pt log show", patroniLogLines, args,
			map[string]interface{}{"lines": patroniLogLines}, func() error {
				return patroni.LogTail(patroniLogDir, utils.GetDBSU(patroniDBSU), patroniLogLines)
			})
	},
}

var patroniLogGrepCmd = &cobra.Command{
	Use:         "grep <pattern>",
	Aliases:     []string{"g", "search"},
	Short:       "Search patroni log files",
	Annotations: ancsAnn("pig patroni log grep", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
	Args:        cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lines := 0
		if cmd.Flags().Changed("lines") {
			lines, _ = cmd.Flags().GetInt("lines")
			if err := validateLogLines(lines); err != nil {
				return err
			}
		}
		logDir := patroni.LogDir(patroniLogDir, utils.GetDBSU(patroniDBSU))
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni log grep",
				"log grep is not supported in structured output",
				"use VictoriaLogs for structured log filtering",
				args,
				map[string]interface{}{"log_dir": logDir, "lines": lines},
			)
		}
		return runLegacyStructured(legacyModulePt, "pig patroni log grep", args, map[string]interface{}{
			"log_dir": logDir,
			"lines":   lines,
		}, func() error {
			err := patroni.LogGrep(logDir, utils.GetDBSU(patroniDBSU), args[0], lines)
			if utils.IsSilentExit(err) {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
			}
			return err
		})
	},
}

var patroniStatusCmd = &cobra.Command{
	Use:         "status",
	Aliases:     []string{"st"},
	Short:       "Show local Patroni and cluster status",
	Annotations: ancsAnn("pig patroni status", "query", "stable", "safe", true, "safe", "none", "dbsu", 3000),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbsu := utils.GetDBSU(patroniDBSU)
		if config.IsStructuredOutput() {
			return handleAuxResult(patroni.StatusResult(dbsu))
		}
		return patroni.Status(dbsu)
	},
}

var patroniStartCmd = &cobra.Command{
	Use:          "start",
	Aliases:      []string{"up"},
	Hidden:       true,
	SilenceUsage: true,
	Short:        "Start patroni service",
	Args:         cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("start", args)
	},
}

var patroniStopCmd = &cobra.Command{
	Use:          "stop",
	Aliases:      []string{"dn"},
	Hidden:       true,
	SilenceUsage: true,
	Short:        "Stop patroni service",
	Args:         cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("stop", args)
	},
}

var patroniSvcCmd = &cobra.Command{
	Use:         "service",
	Aliases:     []string{"svc"},
	Short:       "Manage the local patroni systemd service",
	Annotations: ancsAnn("pig patroni service", "query", "stable", "safe", true, "safe", "none", "root", 100),
}

var (
	patroniSvcStartCmd   = newServiceActionCommand(legacyModulePt, "pig patroni service", "patroni service", "start", []string{"up"}, "", runPatroniSystemctl)
	patroniSvcStopCmd    = newServiceActionCommand(legacyModulePt, "pig patroni service", "patroni service", "stop", []string{"dn"}, "", runPatroniSystemctl)
	patroniSvcRestartCmd = newServiceActionCommand(legacyModulePt, "pig patroni service", "patroni service", "restart", []string{"rs"}, "", runPatroniSystemctl)
	patroniSvcReloadCmd  = newServiceActionCommand(legacyModulePt, "pig patroni service", "patroni service", "reload", []string{"rl"}, "", runPatroniSystemctl)
	patroniSvcStatusCmd  = newServiceActionCommand(legacyModulePt, "pig patroni service", "patroni service", "status", []string{"st"}, "", runPatroniSystemctl)
)

func runPatroniSystemctl(action string) error { return patroniSystemctlExec(action) }

func runPatroniSvcAction(action string, args []string) error {
	return runLegacyStructured(legacyModulePt, "pig patroni service "+action, args, nil, func() error {
		return patroniSystemctlExec(action)
	})
}
