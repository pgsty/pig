package cmd

import (
	"errors"
	"fmt"
	"os"
	"pig/cli/ext"
	"pig/cli/postgres"
	postgrescli "pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// ============================================================================
// Global Flags (shared by all pg subcommands)
// ============================================================================

var pgConfig = postgres.DefaultConfig()

// Additional flags for specific commands
var (
	// init flags
	pgInitEncoding string
	pgInitLocale   string
	pgInitChecksum bool
	pgInitForce    bool
	pgInitYes      bool

	// start flags
	pgStartLog     string
	pgStartTimeout int
	pgStartNoWait  bool
	pgStartOptions string

	// stop flags
	pgStopMode    string
	pgStopTimeout int
	pgStopNoWait  bool
	pgStopPlan    bool

	// restart flags
	pgRestartMode    string
	pgRestartTimeout int
	pgRestartNoWait  bool
	pgRestartOptions string
	pgRestartPlan    bool

	// promote flags
	pgPromoteTimeout int
	pgPromoteNoWait  bool
	pgPromoteYes     bool
	pgPromotePlan    bool

	// log flags
	pgLogNum            int
	pgLogFollow         bool
	pgLogGrepIgnoreCase bool
	pgLogGrepContext    int

	// ps flags
	pgPsAll      bool
	pgPsUser     string
	pgPsDatabase string

	// psql flags
	pgPsqlCommand string
	pgPsqlFile    string

	// kill flags
	pgKillExecute bool
	pgKillPid     int
	pgKillUser    string
	pgKillDb      string
	pgKillState   string
	pgKillQuery   string
	pgKillAll     bool
	pgKillCancel  bool
	pgKillWatch   int
	pgKillPlan    bool

	// maintenance flags
	pgMaintAll     bool
	pgMaintSchema  string
	pgMaintTable   string
	pgMaintVerbose bool
	pgMaintFull    bool
	pgMaintYes     bool
	pgMaintJobs    int
	pgMaintPlan    bool

	// role flags
	pgRoleVerbose bool
)

var (
	pgInitCommandExec    = postgres.InitDB
	pgPromoteCommandExec = postgres.Promote
	pgVacuumCommandExec  = postgres.Vacuum
)

// ============================================================================
// Main Command: pig pg
// ============================================================================

var pgCmd = &cobra.Command{
	Use:         "postgres",
	Short:       "Manage local PostgreSQL server & databases",
	Aliases:     []string{"pg"},
	GroupID:     "pigsty",
	Annotations: ancsAnn("pig postgres", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Long: `Local PostgreSQL primitives (pg_ctl / psql / local files). Cluster-level operations live in "pig pt"; orchestrated point-in-time recovery in "pig pitr".

Manage local PostgreSQL server and databases.

Server Control (via pg_ctl):
  pig pg init     [-v ver] [-D datadir]     initialize data directory
  pig pg start    [-D datadir]              start PostgreSQL server
  pig pg stop     [-D datadir] [-m fast]    stop PostgreSQL server
  pig pg restart  [-D datadir] [-m fast]    restart PostgreSQL server
  pig pg reload   [-D datadir]              reload configuration
  pig pg status   [-D datadir]              show server status
  pig pg promote  [-D datadir]              promote standby to primary
  pig pg role     [-D datadir] [-V]         detect instance role (primary/replica)

Service Management (via systemctl):
  pig pg svc start                          start postgres systemd service
  pig pg svc stop                           stop postgres systemd service
  pig pg svc restart                        restart postgres systemd service
  pig pg svc reload                         reload postgres systemd service
  pig pg svc status                         show postgres service status

Connection & Query:
  pig pg psql     [db] [-c cmd]             connect to database via psql
  pig pg ps       [-a] [-u user]            show current connections
  pig pg kill     [-x] [-u user]            terminate connections (dry-run by default)

Database Maintenance:
  pig pg vacuum   [db] [-a] [-t table]      vacuum tables
  pig pg analyze  [db] [-a] [-t table]      analyze tables
  pig pg freeze   [db] [-a] [-t table]      vacuum freeze tables
  pig pg repack   [db] [-a] [-t table]      repack tables (online rebuild)

Tuning:
  pig pg tune     [-p profile] [-n]        generate optimized parameters

Utilities:
  pig pg log <list|tail|cat|less|grep>      view PostgreSQL logs
`,
}

// ============================================================================
// Subcommand: pig pg init
// ============================================================================

var pgInitCmd = &cobra.Command{
	Use:         "init [-- initdb-options...]",
	Short:       "Initialize PostgreSQL data directory",
	Aliases:     []string{"initdb", "i"},
	Annotations: ancsAnn("pig postgres init", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 30000),
	Example: `  pig pg init                      # use default settings
  pig pg init -v 18                # use PostgreSQL 18
  pig pg init -D /data/pg18 -k     # specify datadir with checksums
  pig pg init -o json              # structured output (JSON)
  pig pg init -- --waldir=/wal     # pass extra options to initdb`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.InitOptions{
			Encoding:  pgInitEncoding,
			Locale:    pgInitLocale,
			Checksum:  pgInitChecksum,
			Force:     pgInitForce,
			ExtraArgs: args,
		}

		// The T2 gate fires only when --force would actually destroy something:
		// an initialized data directory (B21). Wiping nothing needs no consent.
		destructive := pgInitForce && pgInitTargetInitialized(pgConfig)

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			if destructive && !pgInitYes {
				return requireStructuredConfirmation("pg",
					output.CodePgConfirmationRequired,
					"pg init --force requires explicit confirmation",
					"init", "pg:local-instance", "high",
					"pig pg init --force --yes",
					"", // pg init has no --plan
				)
			}
			result := postgres.InitResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		if destructive {
			if err := requireTextHighRiskConfirmation(pgInitYes,
				fmt.Sprintf("This will overwrite the initialized PostgreSQL data directory %s", postgres.GetPgData(pgConfig)),
				"pg init --force",
			); err != nil {
				return err
			}
		}
		return pgInitCommandExec(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg start
// ============================================================================

var pgStartCmd = &cobra.Command{
	Use:         "start",
	Short:       "Start PostgreSQL server",
	Aliases:     []string{"boot", "up"},
	Annotations: ancsAnn("pig postgres start", "action", "volatile", "unsafe", true, "medium", "none", "dbsu", 10000),
	Example: `  pig pg start                     # start with defaults (no-op if already running)
  pig pg start -D /data/pg18       # specify data directory
  pig pg start -l /pg/log/pg.log   # redirect output to log file
  pig pg start -O "-p 5433"        # pass options to postgres
  pig pg start -o json             # structured output (JSON)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.StartOptions{
			LogFile: pgStartLog,
			Timeout: pgStartTimeout,
			NoWait:  pgStartNoWait,
			Options: pgStartOptions,
		}

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.StartResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.Start(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg stop
// ============================================================================

var pgStopCmd = &cobra.Command{
	Use:     "stop",
	Short:   "Stop PostgreSQL server",
	Aliases: []string{"halt", "down"},
	Annotations: mergeAnn(
		ancsAnn("pig postgres stop", "action", "volatile", "unsafe", true, "high", "recommended", "dbsu", 10000),
		map[string]string{
			"flags.mode.choices": "smart,fast,immediate",
		},
	),
	Example: `  pig pg stop                      # fast stop (default)
  pig pg stop -m smart             # wait for clients to disconnect
  pig pg stop -m immediate         # immediate shutdown
  pig pg stop --plan               # preview stop plan without executing
  pig pg stop -o json              # structured output (JSON)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.StopOptions{
			Mode:    pgStopMode,
			Timeout: pgStopTimeout,
			NoWait:  pgStopNoWait,
		}

		// Plan mode: show plan without executing
		if pgStopPlan {
			plan := postgres.BuildStopPlan(pgConfig, opts)
			return handlePlanOutput(plan)
		}

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.StopResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.Stop(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg restart
// ============================================================================

var pgRestartCmd = &cobra.Command{
	Use:     "restart",
	Short:   "Restart PostgreSQL server",
	Aliases: []string{"reboot"},
	Annotations: mergeAnn(
		ancsAnn("pig postgres restart", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 30000),
		map[string]string{
			"flags.mode.choices": "smart,fast,immediate",
		},
	),
	Example: `  pig pg restart                   # fast restart
  pig pg restart -m immediate      # immediate restart
  pig pg restart -O "-p 5433"      # restart with new options
  pig pg restart --plan            # preview restart plan without executing
  pig pg restart -o json           # structured output (JSON)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.RestartOptions{
			Mode:    pgRestartMode,
			Timeout: pgRestartTimeout,
			NoWait:  pgRestartNoWait,
			Options: pgRestartOptions,
		}

		// Plan mode: show plan without executing
		if pgRestartPlan {
			plan := postgres.BuildRestartPlan(pgConfig, opts)
			return handlePlanOutput(plan)
		}

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.RestartResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.Restart(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg reload
// ============================================================================

var pgReloadCmd = &cobra.Command{
	Use:         "reload",
	Short:       "Reload PostgreSQL configuration",
	Aliases:     []string{"hup"},
	Annotations: ancsAnn("pig postgres reload", "action", "volatile", "restricted", true, "low", "none", "dbsu", 1000),
	Example: `  pig pg reload                    # reload config (SIGHUP)
  pig pg reload -D /data/pg18      # specify data directory
  pig pg reload -o json            # structured output (JSON)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.ReloadResult(pgConfig)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.Reload(pgConfig)
	},
}

// ============================================================================
// Subcommand: pig pg status
// ============================================================================

var pgStatusCmd = &cobra.Command{
	Use:         "status",
	Short:       "Show PostgreSQL server status",
	Aliases:     []string{"st", "stat"},
	Annotations: ancsAnn("pig postgres status", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Example: `  pig pg status                    # check server status
  pig pg status -D /data/pg18      # specify data directory
  pig pg status -o json            # structured output (JSON)
  pig pg status -o yaml            # structured output (YAML)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.StatusResult(pgConfig)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.Status(pgConfig)
	},
}

// ============================================================================
// Subcommand: pig pg promote
// ============================================================================

var pgPromoteCmd = &cobra.Command{
	Use:         "promote",
	Short:       "Promote standby to primary",
	Aliases:     []string{"pro"},
	Annotations: ancsAnn("pig postgres promote", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 10000),
	Example: `  pig pg promote                   # promote standby
  pig pg promote -D /data/pg18     # specify data directory
  pig pg promote -o json           # structured output (JSON)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.PromoteOptions{
			Timeout: pgPromoteTimeout,
			NoWait:  pgPromoteNoWait,
		}

		if pgPromotePlan {
			return handlePlanOutput(postgres.BuildPromotePlan(pgConfig, opts))
		}

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			if !pgPromoteYes {
				return structuredConfirmationError(
					output.CodePgConfirmationRequired,
					"pg promote requires explicit confirmation",
					"structured output mode does not prompt interactively; rerun with --yes to execute or --plan to preview",
					output.OperationMeta{
						Module:       "pg",
						Command:      "promote",
						Boundary:     "pg:local-instance",
						Risk:         "critical",
						Confirmation: "required",
						Executed:     false,
						DryRun:       false,
					},
					[]output.NextAction{
						{Command: "pig pg promote --yes", Reason: "execute local-only promotion after explicit confirmation", Required: true},
						{Command: "pig pg promote --plan", Reason: "preview local-only promotion", Required: false},
						{Command: "pig pt switchover --plan", Reason: "use Patroni-managed planned leadership transfer", Required: false},
					},
				)
			}
			result := postgres.PromoteResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		if err := requireTextHighRiskConfirmation(pgPromoteYes,
			"This will promote the local PostgreSQL standby outside cluster orchestration",
			"pg promote",
		); err != nil {
			return err
		}
		return pgPromoteCommandExec(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg role
// ============================================================================

var pgRoleCmd = &cobra.Command{
	Use:         "role",
	Short:       "Detect PostgreSQL instance role (primary or replica)",
	Aliases:     []string{"r"},
	Annotations: ancsAnn("pig postgres role", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Example: `  pig pg role                     # output: primary, replica, or unknown
  pig pg role -V                  # verbose output with detection details
  pig pg role -D /data/pg18       # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.RoleOptions{
			Verbose: pgRoleVerbose,
		}
		return runLegacyStructured(legacyModulePg, "pig postgres role", args, map[string]interface{}{
			"verbose": pgRoleVerbose,
		}, func() error {
			return postgres.PrintRole(pgConfig, opts)
		})
	},
}

// ============================================================================
// Command Registration
// ============================================================================

func registerPostgresCommand() *cobra.Command {
	pgCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := commandModulePreRun(cmd, args); err != nil {
			return err
		}
		if err := ext.DetectPostgres(); err != nil {
			logrus.Debugf("DetectPostgres: %v", err)
		}
		return nil
	}

	// Global flags for all pg subcommands
	pgCmd.PersistentFlags().IntVarP(&pgConfig.PgVersion, "version", "v", 0, "PostgreSQL major version")
	pgCmd.PersistentFlags().StringVarP(&pgConfig.PgData, "data", "D", "", "data directory (default: /pg/data)")
	pgCmd.PersistentFlags().StringVarP(&pgConfig.DbSU, "dbsu", "U", "", "database superuser (default: $PIG_DBSU or postgres)")

	registerPgControlCommands()
	registerPgLogCommands()
	registerPgConnectionCommands()
	registerPgMaintenanceCommands()
	registerPgServiceCommands()
	registerPgTuneCommands()
	registerPgForkCommands()
	return pgCmd
}

func pgPreRun(cmd *cobra.Command, args []string) error {
	if pgCmd.PersistentPreRunE == nil {
		return nil
	}
	return pgCmd.PersistentPreRunE(cmd, args)
}

func registerPgControlCommands() {
	// init subcommand flags
	pgInitCmd.Flags().StringVarP(&pgInitEncoding, "encoding", "E", "", "database encoding (default: UTF8)")
	pgInitCmd.Flags().StringVar(&pgInitLocale, "locale", "", "locale setting (default: C)")
	pgInitCmd.Flags().BoolVarP(&pgInitChecksum, "data-checksum", "k", false, "enable data checksums")
	pgInitCmd.Flags().BoolVarP(&pgInitForce, "force", "f", false, "force init, remove existing data directory (DANGEROUS)")
	pgInitCmd.Flags().BoolVarP(&pgInitYes, "yes", "y", false, "skip confirmation when --force overwrites a data directory")

	// start subcommand flags
	pgStartCmd.Flags().StringVarP(&pgStartLog, "log", "l", "", "redirect stdout/stderr to log file")
	pgStartCmd.Flags().IntVarP(&pgStartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStartCmd.Flags().BoolVar(&pgStartNoWait, "no-wait", false, "do not wait for startup")
	pgStartCmd.Flags().StringVarP(&pgStartOptions, "options", "O", "", "options passed to postgres")

	// stop subcommand flags
	pgStopCmd.Flags().StringVarP(&pgStopMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgStopCmd.Flags().IntVarP(&pgStopTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStopCmd.Flags().BoolVar(&pgStopNoWait, "no-wait", false, "do not wait for shutdown")
	pgStopCmd.Flags().BoolVar(&pgStopPlan, "plan", false, "preview stop plan without executing")

	// restart subcommand flags
	pgRestartCmd.Flags().StringVarP(&pgRestartMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgRestartCmd.Flags().IntVarP(&pgRestartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgRestartCmd.Flags().BoolVar(&pgRestartNoWait, "no-wait", false, "do not wait for restart")
	pgRestartCmd.Flags().StringVarP(&pgRestartOptions, "options", "O", "", "options passed to postgres")
	pgRestartCmd.Flags().BoolVar(&pgRestartPlan, "plan", false, "preview restart plan without executing")

	// promote subcommand flags
	pgPromoteCmd.Flags().IntVarP(&pgPromoteTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgPromoteCmd.Flags().BoolVar(&pgPromoteNoWait, "no-wait", false, "do not wait for promotion")
	pgPromoteCmd.Flags().BoolVarP(&pgPromoteYes, "yes", "y", false, "skip confirmation prompt")
	pgPromoteCmd.Flags().BoolVar(&pgPromotePlan, "plan", false, "preview promote plan without executing")

	// role subcommand flags
	pgRoleCmd.Flags().BoolVarP(&pgRoleVerbose, "verbose", "V", false, "show detailed detection process")

	// Register subcommands - Phase 1
	pgCmd.AddCommand(
		pgInitCmd,
		pgStartCmd,
		pgStopCmd,
		pgRestartCmd,
		pgReloadCmd,
		pgStatusCmd,
		pgPromoteCmd,
		pgRoleCmd,
	)
}

func registerPgLogCommands() {
	// Log command flags
	pgLogCmd.PersistentFlags().StringVar(&pgConfig.LogDir, "log-dir", "", "log directory (default: /pg/log/postgres)")
	pgLogCmd.Flags().BoolVarP(&pgLogFollow, "follow", "f", false, "follow log output")
	pgLogCmd.PersistentFlags().IntVarP(&pgLogNum, "lines", "n", 50, "number of lines")
	pgLogTailCmd.Flags().BoolP("follow", "f", false, "(no-op: tail always follows)")
	pgLogGrepCmd.Flags().BoolVar(&pgLogGrepIgnoreCase, "ignore-case", false, "ignore case")
	pgLogGrepCmd.Flags().IntVarP(&pgLogGrepContext, "context", "C", 0, "show N lines of context")

	// Log subcommands
	pgLogCmd.AddCommand(pgLogListCmd, pgLogTailCmd, pgLogCatCmd, pgLogLessCmd, pgLogGrepCmd)
	pgCmd.AddCommand(pgLogCmd)
}

func registerPgConnectionCommands() {
	// psql command flags
	pgPsqlCmd.Flags().StringVarP(&pgPsqlCommand, "command", "c", "", "run single SQL command")
	pgPsqlCmd.Flags().StringVarP(&pgPsqlFile, "file", "f", "", "run commands from file")
	pgCmd.AddCommand(pgPsqlCmd)

	// ps command flags
	pgPsCmd.Flags().BoolVarP(&pgPsAll, "all", "a", false, "show all connections (including system)")
	pgPsCmd.Flags().StringVarP(&pgPsUser, "user", "u", "", "filter by user")
	pgPsCmd.Flags().StringVarP(&pgPsDatabase, "database", "d", "", "filter by database")
	pgCmd.AddCommand(pgPsCmd)

	// kill command flags
	pgKillCmd.Flags().BoolVarP(&pgKillExecute, "execute", "x", false, "actually kill (default is dry-run)")
	pgKillCmd.Flags().IntVar(&pgKillPid, "pid", 0, "kill specific PID")
	pgKillCmd.Flags().StringVarP(&pgKillUser, "user", "u", "", "filter by user")
	pgKillCmd.Flags().StringVarP(&pgKillDb, "database", "d", "", "filter by database")
	pgKillCmd.Flags().StringVarP(&pgKillState, "state", "s", "", "filter by state (idle/active/idle in transaction)")
	pgKillCmd.Flags().StringVarP(&pgKillQuery, "query", "q", "", "filter by query pattern")
	pgKillCmd.Flags().BoolVarP(&pgKillAll, "all", "a", false, "include replication connections")
	pgKillCmd.Flags().BoolVarP(&pgKillCancel, "cancel", "c", false, "cancel query instead of terminate")
	pgKillCmd.Flags().IntVar(&pgKillWatch, "watch", 0, "repeat every N seconds")
	pgKillCmd.Flags().BoolVar(&pgKillPlan, "plan", false, "preview kill plan without executing")
	pgCmd.AddCommand(pgKillCmd)
}

func addPgMaintFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&pgMaintAll, "all", "a", false, "process all databases")
	cmd.Flags().StringVar(&pgMaintSchema, "schema", "", "schema name")
	cmd.Flags().StringVarP(&pgMaintTable, "table", "t", "", "table name")
	cmd.Flags().BoolVarP(&pgMaintVerbose, "verbose", "V", false, "verbose output")
}

func registerPgMaintenanceCommands() {
	// vacuum command
	addPgMaintFlags(pgVacuumCmd)
	pgVacuumCmd.Flags().BoolVarP(&pgMaintFull, "full", "F", false, "VACUUM FULL (requires exclusive lock)")
	pgVacuumCmd.Flags().BoolVarP(&pgMaintYes, "yes", "y", false, "skip VACUUM FULL confirmation prompt")
	pgVacuumCmd.Flags().BoolVar(&pgMaintPlan, "plan", false, "preview vacuum plan without executing")
	pgCmd.AddCommand(pgVacuumCmd)

	// analyze command
	addPgMaintFlags(pgAnalyzeCmd)
	pgCmd.AddCommand(pgAnalyzeCmd)

	// freeze command
	addPgMaintFlags(pgFreezeCmd)
	pgCmd.AddCommand(pgFreezeCmd)

	// repack command
	addPgMaintFlags(pgRepackCmd)
	pgRepackCmd.Flags().IntVarP(&pgMaintJobs, "jobs", "j", 1, "number of parallel jobs")
	pgRepackCmd.Flags().BoolVar(&pgMaintPlan, "plan", false, "show repack plan without executing")
	pgCmd.AddCommand(pgRepackCmd)
}

func registerPgServiceCommands() {
	pgSvcCmd.AddCommand(
		pgSvcStartCmd,
		pgSvcStopCmd,
		pgSvcRestartCmd,
		pgSvcReloadCmd,
		pgSvcStatusCmd,
	)
	pgCmd.AddCommand(pgSvcCmd)
}

func registerPgForkCommands() {
	pgCmd.AddCommand(
		newPgForkCommand(),
		newPgCloneCommand(),
	)
}

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
		PersistentPreRunE: pgPreRun,
		Long: `Clone a database inside the current PostgreSQL instance.

This wraps CREATE DATABASE ... TEMPLATE ... STRATEGY FILE_COPY. It terminates
existing source-database sessions immediately before cloning, matching Pigsty's
pgsql-db clone workflow.`,
		Example: `  pig pg clone meta                       # clone meta to meta_1/meta_2/...
  pig pg clone meta meta_fork             # clone meta to meta_fork
  pig pg clone meta meta_dev --owner dba  # set owner on cloned database
  pig pg clone meta fork3    --port 5433  # source instance on another local port
  pig pg clone meta metadb4  --plan       # preview clone plan`,
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
	cmd.Flags().IntVar(&opts.port, "port", 0, "source instance port (default: 5432 or $PG_PORT)")
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
		if opts == nil || !opts.Yes {
			return structuredPgConfirmationRequired(
				output.CodeForkConfirmationRequired,
				"pg clone requires explicit confirmation",
				"clone",
				"pg:local-instance",
				"high",
				buildCloneConfirmationCommand(opts, true, false),
				buildCloneConfirmationCommand(opts, false, true),
			)
		}
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

func buildCloneConfirmationCommand(opts *postgrescli.CloneOptions, includeYes bool, includePlan bool) string {
	if opts == nil {
		args := []string{"pig", "pg", "clone"}
		if includeYes {
			args = append(args, "--yes")
		}
		if includePlan {
			args = append(args, "--plan")
		}
		return strings.Join(args, " ")
	}
	n := *opts
	n.Yes = false
	n.Plan = includePlan
	command := postgrescli.BuildCloneCommand(&n)
	if includeYes {
		command += " --yes"
	}
	return command
}

// pgInitTargetInitialized reports whether pg init's target data directory is
// already initialized (PG_VERSION present); overridable for tests.
var pgInitTargetInitialized = func(cfg *postgres.Config) bool {
	dbsu := utils.GetDBSU(cfg.DbSU)
	_, initialized := postgres.CheckDataDirAsDBSU(dbsu, postgres.GetPgData(cfg))
	return initialized
}

func structuredPgConfirmationRequired(code int, message, command, boundary, risk, executeCommand, planCommand string) error {
	return requireStructuredConfirmation("pg", code, message, command, boundary, risk, executeCommand, planCommand)
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

// ============================================================================
// Connection Commands
// ============================================================================

var pgPsqlCmd = &cobra.Command{
	Use:         "psql [dbname]",
	Short:       "Connect to PostgreSQL database via psql",
	Aliases:     []string{"sql", "connect"},
	Annotations: ancsAnn("pig postgres psql", "action", "volatile", "safe", false, "medium", "none", "dbsu", 0),
	Example: `  pig pg psql                    # connect to postgres database
  pig pg psql mydb               # connect to specific database
  pig pg psql mydb -c "SELECT 1" # run single command
  pig pg psql -f script.sql      # run SQL script file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbname := ""
		if len(args) > 0 {
			dbname = args[0]
		}
		opts := &postgres.PsqlOptions{
			Command: pgPsqlCommand,
			File:    pgPsqlFile,
		}
		if config.IsStructuredOutput() && pgPsqlCommand == "" && pgPsqlFile == "" {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres psql",
				"interactive psql session is not supported in structured output",
				"use -c/--command or -f/--file when using -o json/-o yaml",
				args,
				map[string]interface{}{"database": dbname},
			)
		}
		return runLegacyStructured(legacyModulePg, "pig postgres psql", args, map[string]interface{}{
			"database": dbname,
			"command":  pgPsqlCommand,
			"file":     pgPsqlFile,
		}, func() error {
			return postgres.Psql(pgConfig, dbname, opts)
		})
	},
}

var pgPsCmd = &cobra.Command{
	Use:         "ps",
	Short:       "Show PostgreSQL connections",
	Aliases:     []string{"activity", "act"},
	Annotations: ancsAnn("pig postgres ps", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Example: `  pig pg ps                      # show client connections
  pig pg ps -a                   # show all connections
  pig pg ps -u admin             # filter by user
  pig pg ps -d mydb              # filter by database`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.PsOptions{
			All:      pgPsAll,
			User:     pgPsUser,
			Database: pgPsDatabase,
		}
		return runLegacyStructured(legacyModulePg, "pig postgres ps", args, map[string]interface{}{
			"all":      pgPsAll,
			"user":     pgPsUser,
			"database": pgPsDatabase,
		}, func() error {
			return postgres.Ps(pgConfig, opts)
		})
	},
}

var pgKillCmd = &cobra.Command{
	Use:         "kill",
	Short:       "Kill PostgreSQL connections (dry-run by default)",
	Aliases:     []string{"k"},
	Annotations: ancsAnn("pig postgres kill", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 1000),
	Example: `  pig pg kill                    # show what would be killed (dry-run)
  pig pg kill -x                 # actually kill connections
  pig pg kill --pid 12345 -x     # kill specific PID
  pig pg kill -u admin -x        # kill connections by user
  pig pg kill -d mydb -x         # kill connections to database
  pig pg kill -s idle -x         # kill idle connections
  pig pg kill --cancel -x        # cancel queries instead of terminate
  pig pg kill --watch 5 -x       # repeat every 5 seconds`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.KillOptions{
			Execute: pgKillExecute,
			Pid:     pgKillPid,
			User:    pgKillUser,
			Db:      pgKillDb,
			State:   pgKillState,
			Query:   pgKillQuery,
			All:     pgKillAll,
			Cancel:  pgKillCancel,
			Watch:   pgKillWatch,
		}
		if pgKillPlan {
			if err := postgres.ValidateKillOptions(opts); err != nil {
				return structuredParamError(
					output.MODULE_PG,
					"pig postgres kill",
					"invalid kill parameters",
					err.Error(),
					args,
					map[string]interface{}{
						"execute":  pgKillExecute,
						"pid":      pgKillPid,
						"user":     pgKillUser,
						"database": pgKillDb,
						"state":    pgKillState,
						"query":    pgKillQuery,
						"all":      pgKillAll,
						"cancel":   pgKillCancel,
						"watch":    pgKillWatch,
						"plan":     pgKillPlan,
					},
				)
			}
			return handlePlanOutput(postgres.BuildKillPlan(pgConfig, opts))
		}
		if config.IsStructuredOutput() && pgKillWatch > 0 {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres kill",
				"watch mode is not supported in structured output",
				"remove --watch/-w when using -o json/-o yaml",
				args,
				map[string]interface{}{"watch": pgKillWatch},
			)
		}
		return runLegacyStructured(legacyModulePg, "pig postgres kill", args, map[string]interface{}{
			"execute":  pgKillExecute,
			"pid":      pgKillPid,
			"user":     pgKillUser,
			"database": pgKillDb,
			"state":    pgKillState,
			"query":    pgKillQuery,
			"all":      pgKillAll,
			"cancel":   pgKillCancel,
			"watch":    pgKillWatch,
		}, func() error {
			return postgres.Kill(pgConfig, opts)
		})
	},
}

type forkCLIOptions struct {
	plan  bool
	yes   bool
	list  bool
	force bool
	start bool

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
		SilenceUsage:      true,
		Annotations:       ancsAnn("pig postgres fork", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
		PersistentPreRunE: pgPreRun,
		Long: `Manage local disposable PostgreSQL physical forks.

Use "pig pg fork init <name>" to create a managed fork under /pg/data-<name>.
The shorthand "pig pg fork <name>" is kept as an alias for init. An explicit
--dst-data creates an unmanaged fork outside the enumerated /pg/data-* set.`,
		Example: `  pig pg fork init dev                  # /pg/data -> /pg/data-dev
  pig pg fork dev                       # shorthand for "pig pg fork init dev"
  pig pg fork init dev -D /pg/data2 --src-port 15431
  pig pg fork init dev --start --dst-port 15440 # start fork on a specific destination port
  pig pg fork init dev --dst-data /tmp/dev      # unmanaged destination escape hatch
  pig pg fork list                      # list managed /pg/data-* forks
  pig pg fork stop dev                  # stop a managed fork
  pig pg fork rm dev --stop             # stop and remove a running fork`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.list {
				return runForkList(cmd)
			}
			return runFork(cmd, buildInstanceOptions(opts, args[0]))
		},
	}
	setPgForkUseLine(cmd, "pig pg fork <name>|<command>")
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
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "replace existing stopped fork data directory and skip confirmation")
	addForkStartFlag(cmd, opts)
	cmd.Flags().StringVar(&opts.sourceData, "src-data", "", "source PostgreSQL data directory (also set by pg -D/--data; default: $PG_DATA or /pg/data)")
	cmd.Flags().IntVar(&opts.sourcePort, "src-port", 0, "source PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVar(&opts.destData, "dst-data", "", "unmanaged destination data directory escape hatch")
	cmd.Flags().IntVar(&opts.destPort, "dst-port", 0, "destination PostgreSQL port (default: first free port from 15432)")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
}

func newPgForkInitCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "init <name>",
		Aliases:      []string{"create"},
		Short:        "Create a PostgreSQL physical fork",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		Annotations:  ancsAnn("pig postgres fork init", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
		Example: `  pig pg fork init dev
  pig pg fork init dev --start
  pig pg fork init dev -D /pg/data2 --src-port 15431
  pig pg fork init dev --dst-data /tmp/dev-fork --dst-port 15440`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFork(cmd, buildInstanceOptions(opts, args[0]))
		},
	}
	setPgForkUseLine(cmd, "pig pg fork init <name>")
	addPgForkCreateFlags(cmd, opts)
	return cmd
}

func addPgForkCreateFlags(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "replace existing stopped fork data directory and skip confirmation")
	addForkStartFlag(cmd, opts)
	cmd.Flags().StringVar(&opts.sourceData, "src-data", "", "source PostgreSQL data directory (also set by pg -D/--data; default: $PG_DATA or /pg/data)")
	cmd.Flags().IntVar(&opts.sourcePort, "src-port", 0, "source PostgreSQL port (default: 5432 or $PG_PORT)")
	cmd.Flags().StringVar(&opts.destData, "dst-data", "", "unmanaged destination data directory escape hatch")
	cmd.Flags().IntVar(&opts.destPort, "dst-port", 0, "destination PostgreSQL port (default: first free port from 15432)")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
}

func newPgForkListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List managed PostgreSQL forks",
		Args:        cobra.NoArgs,
		Annotations: ancsAnn("pig postgres fork list", "query", "volatile", "safe", true, "safe", "none", "dbsu", 1000),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runForkList(cmd)
		},
	}
	setPgForkUseLine(cmd, "pig pg fork list")
	return cmd
}

func newPgForkStartCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "start <name>",
		Short:        "Start a PostgreSQL fork",
		Args:         forkTargetArgs(opts),
		SilenceUsage: true,
		Annotations:  ancsAnn("pig postgres fork start", "action", "volatile", "unsafe", true, "medium", "none", "dbsu", 10000),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.destData = ""
			}
			if opts.plan {
				return runForkTargetPlan("start", opts, firstArg(args), "Start PostgreSQL fork")
			}
			return runForkAction("fork start", func() (postgrescli.ResultData, error) {
				return postgrescli.StartFork(buildForkTargetOptions(opts, firstArg(args)))
			})
		},
	}
	setPgForkUseLine(cmd, "pig pg fork start <name>|--dst-data <dir>")
	cmd.Flags().StringVar(&opts.destData, "dst-data", "", "unmanaged fork data directory")
	cmd.Flags().IntVar(&opts.destPort, "dst-port", 0, "destination PostgreSQL port override")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "startup timeout in seconds")
	return cmd
}

func newPgForkStopCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "stop <name>",
		Short:        "Stop a PostgreSQL fork",
		Args:         forkTargetArgs(opts),
		SilenceUsage: true,
		Annotations:  ancsAnn("pig postgres fork stop", "action", "volatile", "unsafe", true, "high", "recommended", "dbsu", 10000),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.destData = ""
			}
			if opts.plan {
				return runForkTargetPlan("stop", opts, firstArg(args), "Stop PostgreSQL fork")
			}
			return runForkAction("fork stop", func() (postgrescli.ResultData, error) {
				return postgrescli.StopFork(buildForkTargetOptions(opts, firstArg(args)))
			})
		},
	}
	setPgForkUseLine(cmd, "pig pg fork stop <name>|--dst-data <dir>")
	cmd.Flags().StringVar(&opts.destData, "dst-data", "", "unmanaged fork data directory")
	cmd.Flags().StringVarP(&opts.stopMode, "mode", "m", "", "shutdown mode: smart, fast, or immediate")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "shutdown timeout in seconds")
	return cmd
}

func newPgForkRemoveCommand(opts *forkCLIOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "rm <name>",
		Aliases:      []string{"remove", "delete"},
		Short:        "Remove a PostgreSQL fork",
		Args:         forkTargetArgs(opts),
		SilenceUsage: true,
		Annotations:  ancsAnn("pig postgres fork rm", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 30000),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.destData = ""
			}
			if opts.plan {
				return runForkTargetPlan("rm", opts, firstArg(args), "Remove PostgreSQL fork")
			}
			if err := requireForkRemoveStructuredConfirmation(opts, firstArg(args)); err != nil {
				return err
			}
			return runForkAction("fork remove", func() (postgrescli.ResultData, error) {
				return postgrescli.RemoveFork(buildForkTargetOptions(opts, firstArg(args)))
			})
		},
	}
	setPgForkUseLine(cmd, "pig pg fork rm <name>|--dst-data <dir>")
	cmd.Flags().StringVar(&opts.destData, "dst-data", "", "unmanaged fork data directory")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "skip confirmation for stopped forks")
	cmd.Flags().BoolVar(&opts.stopBefore, "stop", false, "stop a running fork before removing it")
	cmd.Flags().StringVarP(&opts.stopMode, "mode", "m", "", "shutdown mode when --stop is used: smart, fast, or immediate")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "shutdown timeout in seconds")
	return cmd
}

func addForkStartFlag(cmd *cobra.Command, opts *forkCLIOptions) {
	cmd.Flags().BoolVarP(&opts.start, "start", "s", false, "start forked instance after copy")
	cmd.Flags().BoolVarP(&opts.start, "run", "r", false, "deprecated alias for --start")
	_ = cmd.Flags().MarkHidden("run")
}

func setPgForkUseLine(cmd *cobra.Command, useLine string) {
	tmpl := strings.Replace(cmd.UsageTemplate(), "{{.UseLine}}", useLine, 1)
	tmpl = strings.ReplaceAll(tmpl, "{{.CommandPath}}", "pig pg fork")
	cmd.SetUsageTemplate(tmpl)
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
		Start:   cli.start,
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
		Yes:        cli.yes || cli.force,
		Progress:   !config.IsStructuredOutput(),
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
	refreshForkRuntimeStates(forks)
	if config.IsStructuredOutput() {
		return handleAuxResult(output.OK("fork list", forks))
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPORT\tSTATE\tPID\tAGE\tSOURCE\tCOPY\tDATA")
	now := time.Now()
	for _, fork := range forks {
		row := formatForkListRow(fork, now)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.name,
			row.port,
			row.state,
			row.pid,
			row.age,
			row.source,
			row.copy,
			row.data,
		)
	}
	return tw.Flush()
}

func refreshForkRuntimeStates(forks []postgrescli.ForkInfo) {
	dbsu := utils.GetDBSU(pgConfig.DbSU)
	for i := range forks {
		if forks[i].Orphan || forks[i].Target.Data == "" {
			continue
		}
		running, pid := postgrescli.CheckPostgresRunningAsDBSU(dbsu, forks[i].Target.Data)
		forks[i].Target.Started = running
		if running {
			forks[i].Target.PID = pid
		} else {
			forks[i].Target.PID = 0
		}
	}
}

func forkListStatus(fork postgrescli.ForkInfo) string {
	if fork.Orphan {
		return "orphan"
	}
	if fork.Target.Started {
		return "running"
	}
	return "stopped"
}

type forkListRow struct {
	name   string
	port   string
	state  string
	pid    string
	age    string
	source string
	copy   string
	data   string
}

func formatForkListAge(createdAt string, now time.Time) string {
	if createdAt == "" {
		return "-"
	}
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "-"
	}
	age := now.Sub(created)
	if age < 0 {
		age = 0
	}
	switch {
	case age < time.Minute:
		return "<1m"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	}
}

func formatForkListRow(fork postgrescli.ForkInfo, now time.Time) forkListRow {
	row := forkListRow{
		name:   fork.Name,
		port:   "-",
		state:  forkListStatus(fork),
		pid:    "-",
		age:    formatForkListAge(fork.CreatedAt, now),
		source: "-",
		copy:   "-",
		data:   fork.Target.Data,
	}
	if fork.Target.Port > 0 {
		row.port = fmt.Sprintf("%d", fork.Target.Port)
	}
	if fork.Target.Started && fork.Target.PID > 0 {
		row.pid = fmt.Sprintf("%d", fork.Target.PID)
	}
	switch {
	case fork.Source.Data != "" && fork.Source.Port > 0:
		row.source = fmt.Sprintf("%s:%d", fork.Source.Data, fork.Source.Port)
	case fork.Source.Data != "":
		row.source = fork.Source.Data
	case fork.Source.Port > 0:
		row.source = fmt.Sprintf(":%d", fork.Source.Port)
	}
	if fork.Copy.Actual != "" && fork.Copy.Actual != string(postgrescli.CloneModeUnknown) {
		row.copy = fork.Copy.Actual
	}
	return row
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
		if opts == nil || (!opts.Yes && !opts.Replace) {
			return structuredPgConfirmationRequired(
				output.CodeForkConfirmationRequired,
				"pg fork init requires explicit confirmation",
				"fork init",
				"pg:local-instance",
				"critical",
				buildForkConfirmationCommand(opts, true, false),
				buildForkConfirmationCommand(opts, false, true),
			)
		}
		return handleAuxResult(postgrescli.ExecuteResult(opts))
	}

	if err := postgrescli.Execute(opts); err != nil {
		return handleForkError(err)
	}
	return nil
}

func buildForkConfirmationCommand(opts *postgrescli.Options, includeYes bool, includePlan bool) string {
	if opts == nil {
		args := []string{"pig", "pg", "fork", "init"}
		if includeYes {
			args = append(args, "--yes")
		}
		if includePlan {
			args = append(args, "--plan")
		}
		return strings.Join(args, " ")
	}
	n := *opts
	n.Yes = false
	n.Plan = includePlan
	command := postgrescli.BuildCommand(&n)
	if includeYes {
		command += " --yes"
	}
	return command
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
	fmt.Fprint(os.Stderr, postgrescli.ForkActionHint(message, result))
	return nil
}

func requireForkRemoveStructuredConfirmation(cli *forkCLIOptions, name string) error {
	if !config.IsStructuredOutput() || cli == nil || cli.yes || cli.force {
		return nil
	}
	executeCmd := buildForkTargetPlanCommand("rm", cli, name)
	if executeCmd == "" {
		executeCmd = "pig pg fork rm"
	}
	return structuredPgConfirmationRequired(
		output.CodeForkConfirmationRequired,
		"pg fork rm requires explicit confirmation",
		"fork rm",
		"pg:local-instance",
		"critical",
		executeCmd+" --yes",
		executeCmd+" --plan",
	)
}

func runForkTargetPlan(verb string, cli *forkCLIOptions, name string, description string) error {
	command := buildForkTargetPlanCommand(verb, cli, name)
	target := name
	if cli.destData != "" {
		target = cli.destData
	}
	impact := verb
	if verb == "rm" {
		impact = "remove"
	}
	plan := &output.Plan{
		Command: command,
		Actions: []output.Action{
			{Step: 1, Description: fmt.Sprintf("Run %s", command)},
		},
		Affects: []output.Resource{
			{Type: "fork", Name: target, Impact: impact},
		},
		Expected: description,
	}
	return handlePlanOutput(plan)
}

func buildForkTargetPlanCommand(verb string, cli *forkCLIOptions, name string) string {
	args := []string{"pig", "pg", "fork", verb}
	if cli.destData != "" {
		args = append(args, "--dst-data", cli.destData)
	} else {
		args = append(args, name)
	}
	switch verb {
	case "start":
		if cli.destPort != 0 {
			args = append(args, "--dst-port", fmt.Sprintf("%d", cli.destPort))
		}
	case "stop":
		appendForkStopFlags(&args, cli)
	case "rm":
		if cli.stopBefore {
			args = append(args, "--stop")
		}
		if cli.force {
			args = append(args, "-f")
		}
		appendForkStopFlags(&args, cli)
	}
	return utils.ShellQuoteArgs(args)
}

func appendForkStopFlags(args *[]string, cli *forkCLIOptions) {
	if cli.stopMode != "" {
		*args = append(*args, "-m", cli.stopMode)
	}
	if cli.timeout != 0 {
		*args = append(*args, "-t", fmt.Sprintf("%d", cli.timeout))
	}
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

// ============================================================================
// Log Commands
// ============================================================================

var pgLogCmd = &cobra.Command{
	Use:         "log",
	Short:       "View PostgreSQL log files",
	Aliases:     []string{"l"},
	Annotations: ancsAnn("pig postgres log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Long: `View and search PostgreSQL log files in /pg/log/postgres directory.

	  pig pg log                   # show latest log lines
	  pig pg log -f                # tail -f latest log
	  pig pg log list              # list log files
	  pig pg log tail              # tail -f latest log
	  pig pg log show [-n 50]      # show last N lines
	  pig pg log less              # open in less
	  pig pg log grep <pattern>    # search logs`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(pgLogNum); err != nil {
			return err
		}
		if pgLogFollow {
			if config.IsStructuredOutput() {
				return structuredParamError(
					output.MODULE_PG,
					"pig postgres log",
					"log follow mode is not supported in structured output",
					"use 'pig pg log show -n N -o json' for structured snapshot",
					args,
					map[string]interface{}{"follow": pgLogFollow, "lines": pgLogNum},
				)
			}
			return postgres.LogTail(postgres.GetLogDir(pgConfig), "", pgLogNum)
		}
		if err := rejectUnsupportedLogOutputFormat("pig pg log"); err != nil {
			return err
		}
		if isJSONLogOutput() {
			return postgres.LogShowJSONL(postgres.GetLogDir(pgConfig), "", pgLogNum)
		}
		return runLegacyStructured(legacyModulePg, "pig postgres log", args, map[string]interface{}{
			"log_dir": postgres.GetLogDir(pgConfig),
			"follow":  pgLogFollow,
			"lines":   pgLogNum,
		}, func() error {
			return postgres.LogCat(postgres.GetLogDir(pgConfig), "", pgLogNum)
		})
	},
}

var pgLogListCmd = &cobra.Command{
	Use:         "list",
	Short:       "List log files",
	Aliases:     []string{"ls"},
	Annotations: ancsAnn("pig postgres log list", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePg, "pig postgres log list", args, map[string]interface{}{
			"log_dir": postgres.GetLogDir(pgConfig),
		}, func() error {
			return postgres.LogList(postgres.GetLogDir(pgConfig))
		})
	},
}

var pgLogTailCmd = &cobra.Command{
	Use:         "tail [file]",
	Short:       "Tail log file (follow mode)",
	Aliases:     []string{"t", "f"},
	Annotations: ancsAnn("pig postgres log tail", "query", "volatile", "safe", true, "safe", "none", "dbsu", 0),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(pgLogNum); err != nil {
			return err
		}
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres log tail",
				"log tail follow mode is not supported in structured output",
				"use 'pig pg log show -n N -o json' for structured snapshot",
				args,
				map[string]interface{}{"file": file, "lines": pgLogNum},
			)
		}
		return postgres.LogTail(postgres.GetLogDir(pgConfig), file, pgLogNum)
	},
}

var pgLogCatCmd = &cobra.Command{
	Use:         "show [file]",
	Short:       "Show log file content",
	Aliases:     []string{"cat", "c"},
	Annotations: ancsAnn("pig postgres log show", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(pgLogNum); err != nil {
			return err
		}
		if err := rejectUnsupportedLogOutputFormat("pig pg log show"); err != nil {
			return err
		}
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		if isJSONLogOutput() {
			return postgres.LogShowJSONL(postgres.GetLogDir(pgConfig), file, pgLogNum)
		}
		return runLegacyStructured(legacyModulePg, "pig postgres log show", args, map[string]interface{}{
			"log_dir": postgres.GetLogDir(pgConfig),
			"file":    file,
			"lines":   pgLogNum,
		}, func() error {
			return postgres.LogCat(postgres.GetLogDir(pgConfig), file, pgLogNum)
		})
	},
}

var pgLogLessCmd = &cobra.Command{
	Use:         "less [file]",
	Short:       "Open log file in less",
	Aliases:     []string{"vi", "v"},
	Annotations: ancsAnn("pig postgres log less", "query", "volatile", "safe", true, "safe", "none", "dbsu", 0),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres log less",
				"interactive log viewer is not supported in structured output",
				"use 'pig pg log show -n N -o json' for structured snapshot",
				args,
				map[string]interface{}{"file": file},
			)
		}
		return postgres.LogLess(postgres.GetLogDir(pgConfig), file)
	},
}

var pgLogGrepCmd = &cobra.Command{
	Use:         "grep <pattern> [file]",
	Short:       "Search log files",
	Aliases:     []string{"g", "search"},
	Annotations: ancsAnn("pig postgres log grep", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
	Args: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = false
		cmd.SilenceUsage = false
		return cobra.RangeArgs(1, 2)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 1 {
			file = args[1]
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres log grep",
				"log grep is not supported in structured output",
				"use VictoriaLogs for structured log filtering",
				args,
				map[string]interface{}{
					"log_dir":     postgres.GetLogDir(pgConfig),
					"pattern":     args[0],
					"file":        file,
					"ignore_case": pgLogGrepIgnoreCase,
					"context":     pgLogGrepContext,
				},
			)
		}
		return runLegacyStructured(legacyModulePg, "pig postgres log grep", args, map[string]interface{}{
			"log_dir":     postgres.GetLogDir(pgConfig),
			"pattern":     args[0],
			"file":        file,
			"ignore_case": pgLogGrepIgnoreCase,
			"context":     pgLogGrepContext,
		}, func() error {
			err := postgres.LogGrep(postgres.GetLogDir(pgConfig), args[0], file, pgLogGrepIgnoreCase, pgLogGrepContext)
			if utils.IsSilentExit(err) {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
			}
			return err
		})
	},
}

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
  pig pg vacuum mydb --schema myschema # vacuum tables in schema
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
		if pgMaintPlan {
			if err := postgres.ValidateMaintenanceOptions(pgMaintSchema, pgMaintTable); err != nil {
				return structuredParamError(
					output.MODULE_PG,
					"pig postgres vacuum",
					"invalid vacuum parameters",
					err.Error(),
					args,
					maintenanceParams(dbname, true),
				)
			}
			return handlePlanOutput(postgres.BuildVacuumPlan(pgConfig, dbname, opts))
		}
		if config.IsStructuredOutput() && pgMaintFull && !pgMaintYes {
			return structuredConfirmationError(
				output.CodePgConfirmationRequired,
				"pg vacuum --full requires explicit confirmation",
				"structured output mode does not prompt interactively; rerun with --yes to execute or --plan to preview",
				output.OperationMeta{
					Module:       "pg",
					Command:      "vacuum",
					Boundary:     "pg:local-instance",
					Risk:         "high",
					Confirmation: "required",
					Executed:     false,
					DryRun:       false,
				},
				[]output.NextAction{
					{Command: buildVacuumNextAction(dbname, true), Reason: "execute VACUUM FULL after explicit confirmation", Required: true},
					{Command: buildVacuumNextAction(dbname, false) + " --plan", Reason: "preview VACUUM FULL impact", Required: false},
				},
			)
		}
		if pgMaintFull {
			if err := requireTextHighRiskConfirmation(pgMaintYes,
				"VACUUM FULL rewrites relations and requires exclusive locks",
				"pg vacuum --full",
			); err != nil {
				return err
			}
		}
		return runLegacyStructured(legacyModulePg, "pig postgres vacuum", args, map[string]interface{}{
			"database": dbname,
			"all":      pgMaintAll,
			"schema":   pgMaintSchema,
			"table":    pgMaintTable,
			"verbose":  pgMaintVerbose,
			"full":     pgMaintFull,
			"yes":      pgMaintYes,
		}, func() error {
			return pgVacuumCommandExec(pgConfig, dbname, opts)
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
  pig pg repack mydb --schema myschema # repack tables in schema
  pig pg repack mydb -j 4        # parallel repack
  pig pg repack mydb --plan      # show repack plan without executing (recommended)`,
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
			Jobs: pgMaintJobs,
			Plan: pgMaintPlan,
		}
		if config.IsStructuredOutput() && pgMaintPlan {
			if err := postgres.ValidateMaintenanceOptions(pgMaintSchema, pgMaintTable); err != nil {
				return structuredParamError(
					output.MODULE_PG,
					"pig postgres repack",
					"invalid repack parameters",
					err.Error(),
					args,
					maintenanceParams(dbname, false),
				)
			}
			return handlePlanOutput(postgres.BuildRepackPlan(pgConfig, dbname, opts))
		}
		return runLegacyStructured(legacyModulePg, "pig postgres repack", args, map[string]interface{}{
			"database": dbname,
			"all":      pgMaintAll,
			"schema":   pgMaintSchema,
			"table":    pgMaintTable,
			"verbose":  pgMaintVerbose,
			"jobs":     pgMaintJobs,
			"plan":     pgMaintPlan,
		}, func() error {
			return postgres.Repack(pgConfig, dbname, opts)
		})
	},
}

func buildVacuumNextAction(dbname string, includeYes bool) string {
	parts := []string{"pig", "pg", "vacuum"}
	if dbname != "" {
		parts = append(parts, dbname)
	}
	if pgMaintAll {
		parts = append(parts, "--all")
	}
	if pgMaintSchema != "" {
		parts = append(parts, "--schema", pgMaintSchema)
	}
	if pgMaintTable != "" {
		parts = append(parts, "--table", pgMaintTable)
	}
	if pgMaintVerbose {
		parts = append(parts, "--verbose")
	}
	if pgMaintFull {
		parts = append(parts, "--full")
	}
	if includeYes {
		parts = append(parts, "--yes")
	}
	return strings.Join(parts, " ")
}

func maintenanceParams(dbname string, includeFull bool) map[string]interface{} {
	params := map[string]interface{}{
		"database": dbname,
		"all":      pgMaintAll,
		"schema":   pgMaintSchema,
		"table":    pgMaintTable,
		"verbose":  pgMaintVerbose,
		"plan":     pgMaintPlan,
	}
	if includeFull {
		params["full"] = pgMaintFull
	} else {
		params["jobs"] = pgMaintJobs
	}
	return params
}

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

func addPlanFlags(cmd *cobra.Command, plan *bool, yes *bool) {
	cmd.PersistentFlags().BoolVar(plan, "plan", false, "show execution plan without running")
	cmd.PersistentFlags().BoolVarP(yes, "yes", "y", false, "skip confirmation prompt")
}

// ============================================================================
// Tune Flags
// ============================================================================

var (
	pgTuneProfile string
	pgTuneCPU     int
	pgTuneMem     int
	pgTuneDisk    int
	pgTuneMaxConn int
	pgTuneShmemR  float64
)

// ============================================================================
// Subcommand: pig pg tune
// ============================================================================

var pgTuneCmd = &cobra.Command{
	Use:     "tune",
	Short:   "Generate optimized PostgreSQL parameters",
	Aliases: []string{"tuning"},
	Args:    cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if pgTuneCPU < 0 {
			return fmt.Errorf("cpu must be >= 0")
		}
		if pgTuneMem < 0 {
			return fmt.Errorf("mem must be >= 0")
		}
		if pgTuneDisk < 0 {
			return fmt.Errorf("disk must be >= 0")
		}
		if pgTuneMaxConn < 0 {
			return fmt.Errorf("max-conn must be >= 0")
		}
		if pgTuneShmemR < 0.1 || pgTuneShmemR > 0.4 {
			return fmt.Errorf("shmem-ratio must be between 0.1 and 0.4, got %.2f", pgTuneShmemR)
		}
		return nil
	},
	Annotations: ancsAnn("pig postgres tune", "action", "volatile", "restricted",
		true, "medium", "recommended", "dbsu", 5000),
	Example: `  pig pg tune                        # auto-detect, oltp profile, output params
  pig pg tune -p olap                 # use olap profile
  pig pg tune -c 8 -m 32768 -d 500   # override hardware detection
  pig pg tune -C 500                  # override max_connections
  pig pg tune -R 0.3                  # override shared_buffers ratio
  pig pg tune -o text                 # text output
  pig pg tune -o json                 # structured output (JSON)
  pig pg tune -o yaml                 # structured output (YAML)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.TuneOptions{
			Profile:    pgTuneProfile,
			CPU:        pgTuneCPU,
			MemMB:      pgTuneMem,
			DiskGB:     pgTuneDisk,
			MaxConn:    pgTuneMaxConn,
			ShmemRatio: pgTuneShmemR,
		}
		result := postgres.TuneResult(pgConfig, opts)
		return handleAuxResult(result)
	},
}

// ============================================================================
// Registration
// ============================================================================

func registerPgTuneCommands() {
	pgTuneCmd.Flags().StringVarP(&pgTuneProfile, "profile", "p", "oltp",
		"tuning profile: oltp, olap, tiny, crit")
	pgTuneCmd.Flags().IntVarP(&pgTuneCPU, "cpu", "c", 0,
		"CPU cores (0 = auto-detect)")
	pgTuneCmd.Flags().IntVarP(&pgTuneMem, "mem", "m", 0,
		"total memory in MB (0 = auto-detect)")
	pgTuneCmd.Flags().IntVarP(&pgTuneDisk, "disk", "d", 0,
		"data disk size in GB (0 = auto-detect)")
	pgTuneCmd.Flags().IntVarP(&pgTuneMaxConn, "max-conn", "C", 0,
		"override max_connections (0 = use default 100)")
	pgTuneCmd.Flags().Float64VarP(&pgTuneShmemR, "shmem-ratio", "R", 0.25,
		"shared_buffers as fraction of memory (0.1-0.4)")

	pgCmd.AddCommand(pgTuneCmd)
}
