/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	forkpkg "pig/cli/fork"
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

	dbPort   int
	connDB   string
	noKill   bool
	strategy string
}

func newPgForkCommand() *cobra.Command {
	opts := &forkCLIOptions{}
	cmd := &cobra.Command{
		Use:               "fork <fork-name>",
		Short:             "Create a local disposable PostgreSQL physical fork",
		Args:              forkArgs(opts),
		Annotations:       ancsAnn("pig postgres fork", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
		PersistentPreRunE: forkPreRun,
		Long: `Create a local disposable PostgreSQL physical fork.

The fork is not registered into Pigsty. By default it creates /pg/data-<name>,
writes fork.json, and does not start the forked instance unless -r/--run is set.`,
		Example: `  pig pg fork dev                       # /pg/data -> /pg/data-dev
  pig pg fork dev -D /tmp/dat2         # fork from another source data directory
  pig pg fork dev -r                   # start fork after copy on first free high port
  pig pg fork dev -r -p 12345          # start fork on a specific port
  pig pg fork --list                   # list local /pg/data-* forks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.list {
				return runForkList(cmd)
			}
			return runFork(cmd, buildInstanceOptions(opts, args[0]))
		},
	}
	addForkCommonFlags(cmd, opts)
	addPgForkFlags(cmd, opts)
	return cmd
}

func newPgCloneCommand() *cobra.Command {
	opts := &forkCLIOptions{}
	cmd := &cobra.Command{
		Use:               "clone <source-db> <dest-db>",
		Short:             "Clone a PostgreSQL database with FILE_COPY",
		Args:              cobra.ExactArgs(2),
		Annotations:       ancsAnn("pig postgres clone", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 60000),
		PersistentPreRunE: forkPreRun,
		Long: `Clone a database inside the current PostgreSQL instance.

This wraps CREATE DATABASE ... TEMPLATE ... STRATEGY FILE_COPY. For normal
template databases it terminates existing source-database sessions first,
matching Pigsty's pgsql-db clone workflow. Use --no-kill to skip that step.`,
		Example: `  pig pg clone app app_fork             # clone app to app_fork
  pig pg clone app app_fork --no-kill   # require source DB to be idle
  pig pg clone app app_fork -p 5433     # clone on another local port
  pig pg clone app app_fork --plan      # preview clone plan`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFork(cmd, buildDatabaseOptions(opts, args[0], args[1]))
		},
	}
	addForkCommonFlags(cmd, opts)
	addPgCloneFlags(cmd, opts)
	return cmd
}

func forkPreRun(cmd *cobra.Command, args []string) error {
	if err := initAll(); err != nil {
		return err
	}
	applyStructuredOutputSilence(cmd)
	return nil
}

func addForkCommonFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.PersistentFlags().BoolVar(&opts.plan, "plan", false, "show execution plan without running")
	cmd.PersistentFlags().BoolVar(&opts.plan, "dry-run", false, "alias for --plan")
	cmd.PersistentFlags().BoolVarP(&opts.yes, "yes", "y", false, "skip confirmation prompt")
}

func addPgForkFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.Flags().BoolVar(&opts.list, "list", false, "list local forks under /pg/data-*")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "replace existing fork data directory and skip confirmation")
	cmd.Flags().BoolVarP(&opts.run, "run", "r", false, "start forked instance after copy")
	cmd.Flags().StringVarP(&opts.destData, "data", "d", "", "destination data directory (default: /pg/data-<name>)")
	cmd.Flags().IntVarP(&opts.destPort, "port", "p", 0, "destination PostgreSQL port (default: first free port from 15432)")
	cmd.Flags().IntVarP(&opts.sourcePort, "src-port", "P", 0, "source PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVarP(&opts.sourceData, "src-data", "D", "", "source data directory (default: /pg/data or $PG_DATA)")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
}

func addPgCloneFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.Flags().IntVarP(&opts.dbPort, "port", "p", 0, "PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVar(&opts.connDB, "conn-db", "", "database used to run CREATE DATABASE (default: postgres)")
	cmd.Flags().BoolVar(&opts.noKill, "no-kill", false, "do not terminate active source database connections")
	cmd.Flags().StringVar(&opts.strategy, "strategy", "", "CREATE DATABASE strategy: FILE_COPY or WAL_LOG")
}

func buildInstanceOptions(cli *forkCLIOptions, name string) *forkpkg.Options {
	return &forkpkg.Options{
		Kind:    forkpkg.KindInstance,
		DbSU:    pgConfig.DbSU,
		Plan:    cli.plan,
		Yes:     cli.yes || cli.force,
		Run:     cli.run,
		Replace: cli.force,
		Instance: forkpkg.InstanceOptions{
			Name:       name,
			SourceData: firstNonEmpty(cli.sourceData, pgConfig.PgData),
			SourcePort: cli.sourcePort,
			DestData:   cli.destData,
			DestPort:   cli.destPort,
			Timeout:    cli.timeout,
		},
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

func runForkList(cmd *cobra.Command) error {
	forks, err := forkpkg.ScanForks("/pg")
	if err != nil {
		return handleForkError(&forkpkg.ForkError{Code: output.CodeForkPrecheckFailed, Err: err})
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

func forkListStatus(fork forkpkg.ForkInfo) string {
	if fork.Orphan {
		return "orphan"
	}
	return "forked"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func buildDatabaseOptions(cli *forkCLIOptions, sourceDB, destDB string) *forkpkg.Options {
	return &forkpkg.Options{
		Kind: forkpkg.KindDatabase,
		DbSU: pgConfig.DbSU,
		Plan: cli.plan,
		Yes:  cli.yes,
		Database: forkpkg.DatabaseOptions{
			SourceDB: sourceDB,
			DestDB:   destDB,
			Port:     cli.dbPort,
			ConnDB:   cli.connDB,
			NoKill:   cli.noKill,
			Strategy: cli.strategy,
		},
	}
}

func runFork(cmd *cobra.Command, opts *forkpkg.Options) error {
	if opts.Plan {
		plan, err := forkpkg.Plan(opts)
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
		return handleAuxResult(forkpkg.ExecuteResult(opts))
	}

	if err := forkpkg.Execute(opts); err != nil {
		return handleForkError(err)
	}
	return nil
}

func forkErrorResult(err error) *output.Result {
	if err == nil {
		return output.OK("fork completed", nil)
	}
	var forkErr *forkpkg.ForkError
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
	var forkErr *forkpkg.ForkError
	if errors.As(err, &forkErr) {
		return &utils.ExitCodeError{Code: output.ExitCode(forkErr.Code), Err: forkErr}
	}
	return fmt.Errorf("fork failed: %w", err)
}
