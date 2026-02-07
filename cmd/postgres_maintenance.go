package cmd

import (
	"pig/cli/postgres"

	"github.com/spf13/cobra"
)

// ============================================================================
// Maintenance Commands
// ============================================================================

var pgVacuumCmd = &cobra.Command{
	Use:         "vacuum [dbname]",
	Short:       "Vacuum database tables",
	Aliases:     []string{"vac", "vc"},
	Annotations: ancsAnn("pig postgres vacuum", "action", "volatile", "restricted", true, "low", "none", "dbsu", 60000),
	Example: `  pig pg vacuum                  # vacuum current database
  pig pg vacuum mydb             # vacuum specific database
  pig pg vacuum -a               # vacuum all databases
  pig pg vacuum mydb -t mytable  # vacuum specific table
  pig pg vacuum mydb -n myschema # vacuum tables in schema
  pig pg vacuum mydb --full      # VACUUM FULL (exclusive lock)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbname := ""
		if len(args) > 0 {
			dbname = args[0]
		}
		opts := &postgres.VacuumOptions{
			MaintOptions: postgres.MaintOptions{
				All:     pgMaintAll,
				Schema:  pgMaintSchema,
				Table:   pgMaintTable,
				Verbose: pgMaintVerbose,
			},
			Full: pgMaintFull,
		}
		return runLegacyStructured(legacyModulePg, "pig postgres vacuum", args, map[string]interface{}{
			"database": dbname,
			"all":      pgMaintAll,
			"schema":   pgMaintSchema,
			"table":    pgMaintTable,
			"verbose":  pgMaintVerbose,
			"full":     pgMaintFull,
		}, func() error {
			return postgres.Vacuum(pgConfig, dbname, opts)
		})
	},
}

var pgAnalyzeCmd = &cobra.Command{
	Use:         "analyze [dbname]",
	Short:       "Analyze database tables",
	Aliases:     []string{"ana", "az"},
	Annotations: ancsAnn("pig postgres analyze", "action", "volatile", "restricted", true, "safe", "none", "dbsu", 60000),
	Example: `  pig pg analyze                 # analyze current database
  pig pg analyze mydb            # analyze specific database
  pig pg analyze -a              # analyze all databases
  pig pg analyze mydb -t mytable # analyze specific table`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbname := ""
		if len(args) > 0 {
			dbname = args[0]
		}
		opts := &postgres.MaintOptions{
			All:     pgMaintAll,
			Schema:  pgMaintSchema,
			Table:   pgMaintTable,
			Verbose: pgMaintVerbose,
		}
		return runLegacyStructured(legacyModulePg, "pig postgres analyze", args, map[string]interface{}{
			"database": dbname,
			"all":      pgMaintAll,
			"schema":   pgMaintSchema,
			"table":    pgMaintTable,
			"verbose":  pgMaintVerbose,
		}, func() error {
			return postgres.Analyze(pgConfig, dbname, opts)
		})
	},
}

var pgFreezeCmd = &cobra.Command{
	Use:         "freeze [dbname]",
	Short:       "Vacuum freeze database",
	Annotations: ancsAnn("pig postgres freeze", "action", "volatile", "restricted", true, "low", "none", "dbsu", 60000),
	Example: `  pig pg freeze                  # vacuum freeze current database
  pig pg freeze mydb             # vacuum freeze specific database
  pig pg freeze -a               # vacuum freeze all databases`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbname := ""
		if len(args) > 0 {
			dbname = args[0]
		}
		opts := &postgres.FreezeOptions{
			All:     pgMaintAll,
			Schema:  pgMaintSchema,
			Table:   pgMaintTable,
			Verbose: pgMaintVerbose,
		}
		return runLegacyStructured(legacyModulePg, "pig postgres freeze", args, map[string]interface{}{
			"database": dbname,
			"all":      pgMaintAll,
			"schema":   pgMaintSchema,
			"table":    pgMaintTable,
			"verbose":  pgMaintVerbose,
		}, func() error {
			return postgres.Freeze(pgConfig, dbname, opts)
		})
	},
}

var pgRepackCmd = &cobra.Command{
	Use:         "repack [dbname]",
	Short:       "Repack database tables (requires pg_repack)",
	Aliases:     []string{"rp"},
	Annotations: ancsAnn("pig postgres repack", "action", "volatile", "unsafe", true, "medium", "recommended", "dbsu", 300000),
	Example: `  pig pg repack mydb             # repack all tables in database
  pig pg repack -a               # repack all databases
  pig pg repack mydb -t mytable  # repack specific table
  pig pg repack mydb -n myschema # repack tables in schema
  pig pg repack mydb -j 4        # parallel repack
  pig pg repack mydb --dry-run   # show what would be repacked`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbname := ""
		if len(args) > 0 {
			dbname = args[0]
		}
		opts := &postgres.RepackOptions{
			MaintOptions: postgres.MaintOptions{
				All:     pgMaintAll,
				Schema:  pgMaintSchema,
				Table:   pgMaintTable,
				Verbose: pgMaintVerbose,
			},
			Jobs:   pgMaintJobs,
			DryRun: pgMaintDryRun,
		}
		return runLegacyStructured(legacyModulePg, "pig postgres repack", args, map[string]interface{}{
			"database": dbname,
			"all":      pgMaintAll,
			"schema":   pgMaintSchema,
			"table":    pgMaintTable,
			"verbose":  pgMaintVerbose,
			"jobs":     pgMaintJobs,
			"dry_run":  pgMaintDryRun,
		}, func() error {
			return postgres.Repack(pgConfig, dbname, opts)
		})
	},
}
