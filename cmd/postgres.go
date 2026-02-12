/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Command layer for PostgreSQL server management.
Business logic is delegated to cli/postgres package.
*/
package cmd

import (
	"pig/cli/ext"
	"pig/cli/postgres"
	"pig/internal/config"

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

	// start flags
	pgStartLog     string
	pgStartTimeout int
	pgStartNoWait  bool
	pgStartOptions string
	pgStartYes     bool

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

	// log flags
	pgLogNum            int
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

	// maintenance flags
	pgMaintAll     bool
	pgMaintSchema  string
	pgMaintTable   string
	pgMaintVerbose bool
	pgMaintFull    bool
	pgMaintJobs    int
	pgMaintDryRun  bool

	// role flags
	pgRoleVerbose bool
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
	Long: `Manage local PostgreSQL server and databases.

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

Utilities:
  pig pg log <list|tail|cat|less|grep>      view PostgreSQL logs
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		// Pre-detect PostgreSQL installations
		if err := ext.DetectPostgres(); err != nil {
			logrus.Debugf("DetectPostgres: %v", err)
		}
		return nil
	},
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

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.InitResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.InitDB(pgConfig, opts)
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
	Example: `  pig pg start                     # start with defaults
  pig pg start -D /data/pg18       # specify data directory
  pig pg start -l /pg/log/pg.log   # redirect output to log file
  pig pg start -O "-p 5433"        # pass options to postgres
  pig pg start -y                  # force start (skip running check)
  pig pg start -o json             # structured output (JSON)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.StartOptions{
			LogFile: pgStartLog,
			Timeout: pgStartTimeout,
			NoWait:  pgStartNoWait,
			Options: pgStartOptions,
			Force:   pgStartYes,
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

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			result := postgres.PromoteResult(pgConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: preserve existing behavior
		return postgres.Promote(pgConfig, opts)
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

func init() {
	// Global flags for all pg subcommands
	pgCmd.PersistentFlags().IntVarP(&pgConfig.PgVersion, "version", "v", 0, "PostgreSQL major version")
	pgCmd.PersistentFlags().StringVarP(&pgConfig.PgData, "data", "D", "", "data directory (default: /pg/data)")
	pgCmd.PersistentFlags().StringVarP(&pgConfig.DbSU, "dbsu", "U", "", "database superuser (default: $PIG_DBSU or postgres)")

	registerPgControlCommands()
	registerPgLogCommands()
	registerPgConnectionCommands()
	registerPgMaintenanceCommands()
	registerPgServiceCommands()
}

func registerPgControlCommands() {
	// init subcommand flags
	pgInitCmd.Flags().StringVarP(&pgInitEncoding, "encoding", "E", "", "database encoding (default: UTF8)")
	pgInitCmd.Flags().StringVar(&pgInitLocale, "locale", "", "locale setting (default: C)")
	pgInitCmd.Flags().BoolVarP(&pgInitChecksum, "data-checksum", "k", false, "enable data checksums")
	pgInitCmd.Flags().BoolVarP(&pgInitForce, "force", "f", false, "force init, remove existing data directory (DANGEROUS)")

	// start subcommand flags
	pgStartCmd.Flags().StringVarP(&pgStartLog, "log", "l", "", "redirect stdout/stderr to log file")
	pgStartCmd.Flags().IntVarP(&pgStartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStartCmd.Flags().BoolVarP(&pgStartNoWait, "no-wait", "W", false, "do not wait for startup")
	pgStartCmd.Flags().StringVarP(&pgStartOptions, "options", "O", "", "options passed to postgres")
	pgStartCmd.Flags().BoolVarP(&pgStartYes, "yes", "y", false, "force start even if already running")

	// stop subcommand flags
	pgStopCmd.Flags().StringVarP(&pgStopMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgStopCmd.Flags().IntVarP(&pgStopTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStopCmd.Flags().BoolVarP(&pgStopNoWait, "no-wait", "W", false, "do not wait for shutdown")
	pgStopCmd.Flags().BoolVar(&pgStopPlan, "plan", false, "preview stop plan without executing")

	// restart subcommand flags
	pgRestartCmd.Flags().StringVarP(&pgRestartMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgRestartCmd.Flags().IntVarP(&pgRestartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgRestartCmd.Flags().BoolVarP(&pgRestartNoWait, "no-wait", "W", false, "do not wait for restart")
	pgRestartCmd.Flags().StringVarP(&pgRestartOptions, "options", "O", "", "options passed to postgres")
	pgRestartCmd.Flags().BoolVar(&pgRestartPlan, "plan", false, "preview restart plan without executing")

	// promote subcommand flags
	pgPromoteCmd.Flags().IntVarP(&pgPromoteTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgPromoteCmd.Flags().BoolVarP(&pgPromoteNoWait, "no-wait", "W", false, "do not wait for promotion")

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
	pgLogCmd.PersistentFlags().IntVarP(&pgLogNum, "lines", "n", 0, "number of lines")
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
	pgKillCmd.Flags().IntVarP(&pgKillWatch, "watch", "w", 0, "repeat every N seconds")
	pgCmd.AddCommand(pgKillCmd)
}

func addPgMaintFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&pgMaintAll, "all", "a", false, "process all databases")
	cmd.Flags().StringVarP(&pgMaintSchema, "schema", "n", "", "schema name")
	cmd.Flags().StringVarP(&pgMaintTable, "table", "t", "", "table name")
	cmd.Flags().BoolVarP(&pgMaintVerbose, "verbose", "V", false, "verbose output")
}

func registerPgMaintenanceCommands() {
	// vacuum command
	addPgMaintFlags(pgVacuumCmd)
	pgVacuumCmd.Flags().BoolVarP(&pgMaintFull, "full", "F", false, "VACUUM FULL (requires exclusive lock)")
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
	pgRepackCmd.Flags().BoolVarP(&pgMaintDryRun, "dry-run", "N", false, "show what would be repacked")
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
