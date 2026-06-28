/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	postgrescli "pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

type forkCLIOptions struct {
	plan  bool
	yes   bool
	list  bool
	force bool
	run   bool

	sourceData string
	destData   string
	sourcePort int
	destPort   int
	timeout    int
	stopMode   string
	stopBefore bool
}

func newPgForkCommand() *cobra.Command {
	opts := &forkCLIOptions{}
	cmd := &cobra.Command{
		Use:               "fork <name>|<command>",
		Short:             "Manage local disposable PostgreSQL physical forks",
		Args:              forkArgs(opts),
		Annotations:       ancsAnn("pig postgres fork", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
		PersistentPreRunE: postgresPreRun,
		Long: `Manage local disposable PostgreSQL physical forks.

Use "pig pg fork init <name>" to create a managed fork under /pg/data-<name>.
The shorthand "pig pg fork <name>" is kept as an alias for init. An explicit
-d/--dst-data creates an unmanaged fork outside the enumerated /pg/data-* set.`,
		Example: `  pig pg fork init dev                  # /pg/data -> /pg/data-dev
  pig pg fork dev                       # shorthand for "pig pg fork init dev"
  pig pg fork init dev -D /pg/data2 -P 15431
  pig pg fork init dev -r -p 15432      # start fork on a specific destination port
  pig pg fork init dev -d /tmp/dev      # unmanaged destination escape hatch
  pig pg fork list                      # list managed /pg/data-* forks
  pig pg fork stop dev                  # stop a managed fork
  pig pg fork rm dev --stop -f          # stop and remove a running fork`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.list {
				return runForkList(cmd)
			}
			return runFork(cmd, buildInstanceOptions(opts, args[0]))
		},
	}
	addForkCommonFlags(cmd, opts)
	addPgForkFlags(cmd, opts)
	cmd.AddCommand(newPgForkInitCommand(opts))
	cmd.AddCommand(newPgForkListCommand())
	cmd.AddCommand(newPgForkStartCommand(opts))
	cmd.AddCommand(newPgForkStopCommand(opts))
	cmd.AddCommand(newPgForkRemoveCommand(opts))
	return cmd
}

func addForkCommonFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	addPlanFlags(cmd, &opts.plan, &opts.yes)
}

func addPgForkFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.Flags().BoolVar(&opts.list, "list", false, "list local forks under /pg/data-*")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "replace existing fork data directory and skip confirmation")
	cmd.Flags().BoolVarP(&opts.run, "run", "r", false, "start forked instance after copy")
	cmd.Flags().StringVar(&opts.sourceData, "src-data", "", "source PostgreSQL data directory (also set by pg -D/--data; default: $PG_DATA or /pg/data)")
	cmd.Flags().IntVarP(&opts.sourcePort, "src-port", "P", 0, "source PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVarP(&opts.destData, "dst-data", "d", "", "unmanaged destination data directory escape hatch")
	cmd.Flags().IntVarP(&opts.destPort, "dst-port", "p", 0, "destination PostgreSQL port (default: first free port from 15432)")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
}

func newPgForkInitCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "init <name>",
		Aliases:     []string{"create"},
		Short:       "Create a PostgreSQL physical fork",
		Args:        cobra.ExactArgs(1),
		Annotations: ancsAnn("pig postgres fork init", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
		Example: `  pig pg fork init dev
  pig pg fork init dev -r
  pig pg fork init dev -D /pg/data2 -P 15431
  pig pg fork init dev -d /tmp/dev-fork -p 15432`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFork(cmd, buildInstanceOptions(opts, args[0]))
		},
	}
	addPgForkCreateFlags(cmd, opts)
	return cmd
}

func addPgForkCreateFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "replace existing stopped fork data directory and skip confirmation")
	cmd.Flags().BoolVarP(&opts.run, "run", "r", false, "start forked instance after copy")
	cmd.Flags().StringVar(&opts.sourceData, "src-data", "", "source PostgreSQL data directory (also set by pg -D/--data; default: $PG_DATA or /pg/data)")
	cmd.Flags().IntVarP(&opts.sourcePort, "src-port", "P", 0, "source PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVarP(&opts.destData, "dst-data", "d", "", "unmanaged destination data directory escape hatch")
	cmd.Flags().IntVarP(&opts.destPort, "dst-port", "p", 0, "destination PostgreSQL port (default: first free port from 15432)")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
}

func newPgForkListCommand() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List managed PostgreSQL forks",
		Args:        cobra.NoArgs,
		Annotations: ancsAnn("pig postgres fork list", "query", "volatile", "safe", true, "safe", "none", "dbsu", 1000),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runForkList(cmd)
		},
	}
}

func newPgForkStartCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "start <name>",
		Short:       "Start a PostgreSQL fork",
		Args:        forkTargetArgs(opts),
		Annotations: ancsAnn("pig postgres fork start", "action", "volatile", "unsafe", true, "medium", "none", "dbsu", 10000),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.destData = ""
			}
			return runForkAction("fork start", func() (postgrescli.ResultData, error) {
				return postgrescli.StartFork(buildForkTargetOptions(opts, firstArg(args)))
			})
		},
	}
	cmd.Flags().StringVarP(&opts.destData, "dst-data", "d", "", "unmanaged fork data directory")
	cmd.Flags().IntVarP(&opts.destPort, "dst-port", "p", 0, "destination PostgreSQL port override")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
	return cmd
}

func newPgForkStopCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "stop <name>",
		Short:       "Stop a PostgreSQL fork",
		Args:        forkTargetArgs(opts),
		Annotations: ancsAnn("pig postgres fork stop", "action", "volatile", "unsafe", true, "high", "recommended", "dbsu", 10000),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.destData = ""
			}
			return runForkAction("fork stop", func() (postgrescli.ResultData, error) {
				return postgrescli.StopFork(buildForkTargetOptions(opts, firstArg(args)))
			})
		},
	}
	cmd.Flags().StringVarP(&opts.destData, "dst-data", "d", "", "unmanaged fork data directory")
	cmd.Flags().StringVarP(&opts.stopMode, "mode", "m", "", "shutdown mode: smart, fast, or immediate")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "shutdown timeout in seconds")
	return cmd
}

func newPgForkRemoveCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "rm <name>",
		Aliases:     []string{"remove", "delete"},
		Short:       "Remove a PostgreSQL fork",
		Args:        forkTargetArgs(opts),
		Annotations: ancsAnn("pig postgres fork rm", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 30000),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.destData = ""
			}
			return runForkAction("fork remove", func() (postgrescli.ResultData, error) {
				return postgrescli.RemoveFork(buildForkTargetOptions(opts, firstArg(args)))
			})
		},
	}
	cmd.Flags().StringVarP(&opts.destData, "dst-data", "d", "", "unmanaged fork data directory")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "skip confirmation for stopped forks")
	cmd.Flags().BoolVar(&opts.stopBefore, "stop", false, "stop a running fork before removing it; requires -f")
	cmd.Flags().StringVarP(&opts.stopMode, "mode", "m", "", "shutdown mode when --stop is used: smart, fast, or immediate")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "shutdown timeout in seconds")
	return cmd
}

func buildInstanceOptions(cli *forkCLIOptions, name string) *postgrescli.Options {
	sourceData := cli.sourceData
	if sourceData == "" {
		sourceData = pgConfig.PgData
	}
	return &postgrescli.Options{
		Kind:    postgrescli.KindInstance,
		DbSU:    pgConfig.DbSU,
		Plan:    cli.plan,
		Yes:     cli.yes || cli.force,
		Run:     cli.run,
		Replace: cli.force,
		Instance: postgrescli.InstanceOptions{
			Name:       name,
			SourceData: sourceData,
			SourcePort: cli.sourcePort,
			DestData:   cli.destData,
			DestPort:   cli.destPort,
			Timeout:    cli.timeout,
		},
	}
}

func buildForkTargetOptions(cli *forkCLIOptions, name string) postgrescli.ForkTargetOptions {
	return postgrescli.ForkTargetOptions{
		DbSU:       pgConfig.DbSU,
		Name:       name,
		DestData:   cli.destData,
		DestPort:   cli.destPort,
		Timeout:    cli.timeout,
		StopMode:   cli.stopMode,
		Force:      cli.force,
		StopBefore: cli.stopBefore,
		Yes:        cli.yes || cli.force || config.IsStructuredOutput(),
	}
}

func forkArgs(opts *forkCLIOptions) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if opts.list {
			if len(args) != 0 {
				return fmt.Errorf("--list does not accept fork name")
			}
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	}
}

func forkTargetArgs(opts *forkCLIOptions) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if opts.destData != "" {
			if len(args) != 0 {
				return fmt.Errorf("fork name and --dst-data are mutually exclusive")
			}
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	}
}

func runForkList(cmd *cobra.Command) error {
	forks, err := postgrescli.ScanForksAs(pgConfig.DbSU, "/pg")
	if err != nil {
		return handleForkError(&postgrescli.ForkError{Code: output.CodeForkPrecheckFailed, Err: err})
	}
	if config.IsStructuredOutput() {
		return handleAuxResult(output.OK("fork list", forks))
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPORT\tSTATE\tDATA")
	for _, fork := range forks {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", fork.Name, fork.Target.Port, forkListStatus(fork), fork.Target.Data)
	}
	return tw.Flush()
}

func forkListStatus(fork postgrescli.ForkInfo) string {
	if fork.Orphan {
		return "orphan"
	}
	return "forked"
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func runFork(cmd *cobra.Command, opts *postgrescli.Options) error {
	if opts.Instance.DestData != "" && !config.IsStructuredOutput() {
		utils.PrintWarn("--dst-data creates an unmanaged fork; it will not appear in `pig pg fork list`")
	}
	if opts.Plan {
		plan, err := postgrescli.Plan(opts)
		if err != nil {
			if config.IsStructuredOutput() {
				return handleAuxResult(forkErrorResult(err))
			}
			return handleForkError(err)
		}
		return handlePlanOutput(plan)
	}

	if config.IsStructuredOutput() {
		opts.Yes = true
		return handleAuxResult(postgrescli.ExecuteResult(opts))
	}

	if err := postgrescli.Execute(opts); err != nil {
		return handleForkError(err)
	}
	return nil
}

func runForkAction(message string, action func() (postgrescli.ResultData, error)) error {
	result, err := action()
	if config.IsStructuredOutput() {
		if err != nil {
			return handleAuxResult(forkErrorResult(err))
		}
		return handleAuxResult(output.OK(message, result))
	}
	if err != nil {
		return handleForkError(err)
	}
	return nil
}

func forkErrorResult(err error) *output.Result {
	if err == nil {
		return output.OK("fork completed", nil)
	}
	var forkErr *postgrescli.ForkError
	if errors.As(err, &forkErr) {
		return output.Fail(forkErr.Code, forkErr.Error())
	}
	return output.Fail(output.CodeForkPrecheckFailed, err.Error())
}

func handleForkError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *utils.ExitCodeError
	if errors.As(err, &exitErr) {
		return err
	}
	var forkErr *postgrescli.ForkError
	if errors.As(err, &forkErr) {
		return &utils.ExitCodeError{Code: output.ExitCode(forkErr.Code), Err: forkErr}
	}
	return fmt.Errorf("fork failed: %w", err)
}
