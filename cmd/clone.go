/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"errors"
	"fmt"

	postgrescli "pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

type cloneCLIOptions struct {
	plan      bool
	yes       bool
	port      int
	connDB    string
	owner     string
	connLimit int
}

func newPgCloneCommand() *cobra.Command {
	opts := &cloneCLIOptions{}
	cmd := &cobra.Command{
		Use:               "clone <source-db> [dest-db]",
		Short:             "Clone a PostgreSQL database with FILE_COPY",
		Args:              cobra.RangeArgs(1, 2),
		Annotations:       ancsAnn("pig postgres clone", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 60000),
		PersistentPreRunE: postgresPreRun,
		Long: `Clone a database inside the current PostgreSQL instance.

This wraps CREATE DATABASE ... TEMPLATE ... STRATEGY FILE_COPY. It terminates
existing source-database sessions immediately before cloning, matching Pigsty's
pgsql-db clone workflow.`,
		Example: `  pig pg clone meta                       # clone meta to meta_1/meta_2/...
  pig pg clone meta meta_fork            # clone meta to meta_fork
  pig pg clone meta meta_fork --owner dba # set owner on cloned database
  pig pg clone meta meta_fork -p 5433     # clone on another local port
  pig pg clone meta meta_fork --plan      # preview clone plan`,
		RunE: func(cmd *cobra.Command, args []string) error {
			destDB := ""
			if len(args) > 1 {
				destDB = args[1]
			}
			return runClone(cmd, buildCloneOptions(opts, args[0], destDB, cmd.Flags().Changed("conn-limit")))
		},
	}
	addPlanFlags(cmd, &opts.plan, &opts.yes)
	addPgCloneFlags(cmd, opts)
	return cmd
}

func addPgCloneFlags(cmd *cobra.Command, opts *cloneCLIOptions) {
	cmd.Flags().IntVarP(&opts.port, "port", "p", 0, "PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVar(&opts.connDB, "conn-db", "", "database used to run CREATE DATABASE (default: postgres, or template1 when cloning postgres)")
	cmd.Flags().StringVar(&opts.owner, "owner", "", "best-effort owner change after clone")
	cmd.Flags().IntVar(&opts.connLimit, "conn-limit", 0, "connection limit for cloned database (-1 = no limit, 0 = disallow)")
}

func buildCloneOptions(cli *cloneCLIOptions, sourceDB, destDB string, connLimitSet bool) *postgrescli.CloneOptions {
	return &postgrescli.CloneOptions{
		DbSU:         pgConfig.DbSU,
		Plan:         cli.plan,
		Yes:          cli.yes,
		SourceDB:     sourceDB,
		DestDB:       destDB,
		Owner:        cli.owner,
		Port:         cli.port,
		ConnDB:       cli.connDB,
		ConnLimit:    cli.connLimit,
		ConnLimitSet: connLimitSet,
	}
}

func runClone(cmd *cobra.Command, opts *postgrescli.CloneOptions) error {
	if opts.Plan {
		plan, err := postgrescli.PlanClone(opts)
		if err != nil {
			if config.IsStructuredOutput() {
				return handleAuxResult(cloneErrorResult(err))
			}
			return handleCloneError(err)
		}
		return handlePlanOutput(plan)
	}

	if config.IsStructuredOutput() {
		opts.Yes = true
		return handleAuxResult(postgrescli.ExecuteCloneResult(opts))
	}

	if err := postgrescli.ExecuteClone(opts); err != nil {
		return handleCloneError(err)
	}
	return nil
}

func cloneErrorResult(err error) *output.Result {
	if err == nil {
		return output.OK("database clone completed", nil)
	}
	var cloneErr *postgrescli.CloneError
	if errors.As(err, &cloneErr) {
		return output.Fail(cloneErr.Code, cloneErr.Error())
	}
	return output.Fail(output.CodeForkPrecheckFailed, err.Error())
}

func handleCloneError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *utils.ExitCodeError
	if errors.As(err, &exitErr) {
		return err
	}
	var cloneErr *postgrescli.CloneError
	if errors.As(err, &cloneErr) {
		return &utils.ExitCodeError{Code: output.ExitCode(cloneErr.Code), Err: cloneErr}
	}
	return fmt.Errorf("database clone failed: %w", err)
}
