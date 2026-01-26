/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Command layer for PostgreSQL server management.
Business logic is delegated to cli/postgres package.
*/
package cmd

import (
	"pig/cli/ext"
	"pig/cli/postgres"

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

	// restart flags
	pgRestartMode    string
	pgRestartTimeout int
	pgRestartNoWait  bool
	pgRestartOptions string

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
	Use:     "postgres",
	Short:   "Manage local PostgreSQL server & databases",
	Aliases: []string{"pg"},
	GroupID: "pigsty",
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
	Use:     "init [-- initdb-options...]",
	Short:   "Initialize PostgreSQL data directory",
	Aliases: []string{"initdb", "i"},
	Example: `  pig pg init                      # use default settings
  pig pg init -v 18                # use PostgreSQL 18
  pig pg init -D /data/pg18 -k     # specify datadir with checksums
  pig pg init -- --waldir=/wal     # pass extra options to initdb`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.InitOptions{
			Encoding:  pgInitEncoding,
			Locale:    pgInitLocale,
			Checksum:  pgInitChecksum,
			Force:     pgInitForce,
			ExtraArgs: args,
		}
		return postgres.InitDB(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg start
// ============================================================================

var pgStartCmd = &cobra.Command{
	Use:     "start",
	Short:   "Start PostgreSQL server",
	Aliases: []string{"boot", "up"},
	Example: `  pig pg start                     # start with defaults
  pig pg start -D /data/pg18       # specify data directory
  pig pg start -l /pg/log/pg.log   # redirect output to log file
  pig pg start -o "-p 5433"        # pass options to postgres
  pig pg start -y                  # force start (skip running check)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.StartOptions{
			LogFile: pgStartLog,
			Timeout: pgStartTimeout,
			NoWait:  pgStartNoWait,
			Options: pgStartOptions,
			Force:   pgStartYes,
		}
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
	Example: `  pig pg stop                      # fast stop (default)
  pig pg stop -m smart             # wait for clients to disconnect
  pig pg stop -m immediate         # immediate shutdown`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.StopOptions{
			Mode:    pgStopMode,
			Timeout: pgStopTimeout,
			NoWait:  pgStopNoWait,
		}
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
	Example: `  pig pg restart                   # fast restart
  pig pg restart -m immediate      # immediate restart
  pig pg restart -o "-p 5433"      # restart with new options`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.RestartOptions{
			Mode:    pgRestartMode,
			Timeout: pgRestartTimeout,
			NoWait:  pgRestartNoWait,
			Options: pgRestartOptions,
		}
		return postgres.Restart(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg reload
// ============================================================================

var pgReloadCmd = &cobra.Command{
	Use:     "reload",
	Short:   "Reload PostgreSQL configuration",
	Aliases: []string{"hup"},
	Example: `  pig pg reload                    # reload config (SIGHUP)
  pig pg reload -D /data/pg18      # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.Reload(pgConfig)
	},
}

// ============================================================================
// Subcommand: pig pg status
// ============================================================================

var pgStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show PostgreSQL server status",
	Aliases: []string{"st", "stat"},
	Example: `  pig pg status                    # check server status
  pig pg status -D /data/pg18      # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.Status(pgConfig)
	},
}

// ============================================================================
// Subcommand: pig pg promote
// ============================================================================

var pgPromoteCmd = &cobra.Command{
	Use:     "promote",
	Short:   "Promote standby to primary",
	Aliases: []string{"pro"},
	Example: `  pig pg promote                   # promote standby
  pig pg promote -D /data/pg18     # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.PromoteOptions{
			Timeout: pgPromoteTimeout,
			NoWait:  pgPromoteNoWait,
		}
		return postgres.Promote(pgConfig, opts)
	},
}

// ============================================================================
// Subcommand: pig pg role
// ============================================================================

var pgRoleCmd = &cobra.Command{
	Use:     "role",
	Short:   "Detect PostgreSQL instance role (primary or replica)",
	Aliases: []string{"r"},
	Example: `  pig pg role                     # output: primary, replica, or unknown
  pig pg role -V                  # verbose output with detection details
  pig pg role -D /data/pg18       # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.RoleOptions{
			Verbose: pgRoleVerbose,
		}
		return postgres.PrintRole(pgConfig, opts)
	},
}

// ============================================================================
// Log Commands
// ============================================================================

var pgLogCmd = &cobra.Command{
	Use:     "log",
	Short:   "View PostgreSQL log files",
	Aliases: []string{"l"},
	Long: `View and search PostgreSQL log files in /pg/log/postgres directory.

  pig pg log list              # list log files
  pig pg log tail              # tail -f latest log
  pig pg log cat [-n 100]      # show last N lines
  pig pg log less              # open in less
  pig pg log grep <pattern>    # search logs`,
}

var pgLogListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List log files",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.LogList(postgres.GetLogDir(pgConfig))
	},
}

var pgLogTailCmd = &cobra.Command{
	Use:     "tail [file]",
	Short:   "Tail log file (follow mode)",
	Aliases: []string{"t", "f"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		return postgres.LogTail(postgres.GetLogDir(pgConfig), file, pgLogNum)
	},
}

var pgLogCatCmd = &cobra.Command{
	Use:     "cat [file]",
	Short:   "Output log file content",
	Aliases: []string{"c"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		return postgres.LogCat(postgres.GetLogDir(pgConfig), file, pgLogNum)
	},
}

var pgLogLessCmd = &cobra.Command{
	Use:     "less [file]",
	Short:   "Open log file in less",
	Aliases: []string{"vi", "v"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		return postgres.LogLess(postgres.GetLogDir(pgConfig), file)
	},
}

var pgLogGrepCmd = &cobra.Command{
	Use:     "grep <pattern> [file]",
	Short:   "Search log files",
	Aliases: []string{"g", "search"},
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 1 {
			file = args[1]
		}
		return postgres.LogGrep(postgres.GetLogDir(pgConfig), args[0], file, pgLogGrepIgnoreCase, pgLogGrepContext)
	},
}

// ============================================================================
// Connection Commands
// ============================================================================

var pgPsqlCmd = &cobra.Command{
	Use:     "psql [dbname]",
	Short:   "Connect to PostgreSQL database via psql",
	Aliases: []string{"sql", "connect"},
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
		return postgres.Psql(pgConfig, dbname, opts)
	},
}

var pgPsCmd = &cobra.Command{
	Use:     "ps",
	Short:   "Show PostgreSQL connections",
	Aliases: []string{"activity", "act"},
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
		return postgres.Ps(pgConfig, opts)
	},
}

var pgKillCmd = &cobra.Command{
	Use:     "kill",
	Short:   "Kill PostgreSQL connections (dry-run by default)",
	Aliases: []string{"k"},
	Example: `  pig pg kill                    # show what would be killed (dry-run)
  pig pg kill -x                 # actually kill connections
  pig pg kill --pid 12345 -x     # kill specific PID
  pig pg kill -u admin -x        # kill connections by user
  pig pg kill -d mydb -x         # kill connections to database
  pig pg kill -s idle -x         # kill idle connections
  pig pg kill --cancel -x        # cancel queries instead of terminate
  pig pg kill -w 5 -x            # repeat every 5 seconds`,
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
		return postgres.Kill(pgConfig, opts)
	},
}

// ============================================================================
// Maintenance Commands
// ============================================================================

var pgVacuumCmd = &cobra.Command{
	Use:     "vacuum [dbname]",
	Short:   "Vacuum database tables",
	Aliases: []string{"vac", "vc"},
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
		return postgres.Vacuum(pgConfig, dbname, opts)
	},
}

var pgAnalyzeCmd = &cobra.Command{
	Use:     "analyze [dbname]",
	Short:   "Analyze database tables",
	Aliases: []string{"ana", "az"},
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
		return postgres.Analyze(pgConfig, dbname, opts)
	},
}

var pgFreezeCmd = &cobra.Command{
	Use:   "freeze [dbname]",
	Short: "Vacuum freeze database",
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
		return postgres.Freeze(pgConfig, dbname, opts)
	},
}

var pgRepackCmd = &cobra.Command{
	Use:     "repack [dbname]",
	Short:   "Repack database tables (requires pg_repack)",
	Aliases: []string{"rp"},
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
		return postgres.Repack(pgConfig, dbname, opts)
	},
}

// ============================================================================
// Service Management Commands (via systemctl) - pig pg svc
// ============================================================================

var pgSvcCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"svc", "s"},
	Short:   "Manage postgres systemd service",
	Long: `Manage the PostgreSQL systemd service.

These commands control the postgres service via systemctl. Unlike the pg_ctl
commands (pig pg start/stop/restart/reload), these operate through systemd.

Use these commands when PostgreSQL is managed as a systemd service.
For direct pg_ctl operations, use the parent commands instead.`,
}

var pgSvcStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"up"},
	Short:   "Start postgres systemd service",
	Example: `  pig pg svc start                 # systemctl start postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.RunSystemctl("start", postgres.DefaultSystemdService)
	},
}

var pgSvcStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"dn", "down"},
	Short:   "Stop postgres systemd service",
	Example: `  pig pg svc stop                  # systemctl stop postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.RunSystemctl("stop", postgres.DefaultSystemdService)
	},
}

var pgSvcRestartCmd = &cobra.Command{
	Use:     "restart",
	Aliases: []string{"rt"},
	Short:   "Restart postgres systemd service",
	Example: `  pig pg svc restart               # systemctl restart postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.RunSystemctl("restart", postgres.DefaultSystemdService)
	},
}

var pgSvcReloadCmd = &cobra.Command{
	Use:     "reload",
	Aliases: []string{"rl", "hup"},
	Short:   "Reload postgres systemd service",
	Example: `  pig pg svc reload                # systemctl reload postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.RunSystemctl("reload", postgres.DefaultSystemdService)
	},
}

var pgSvcStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "stat"},
	Short:   "Show postgres systemd service status",
	Example: `  pig pg svc status                # systemctl status postgres`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postgres.RunSystemctl("status", postgres.DefaultSystemdService)
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

	// init subcommand flags
	pgInitCmd.Flags().StringVarP(&pgInitEncoding, "encoding", "E", "", "database encoding (default: UTF8)")
	pgInitCmd.Flags().StringVar(&pgInitLocale, "locale", "", "locale setting (default: C)")
	pgInitCmd.Flags().BoolVarP(&pgInitChecksum, "data-checksum", "k", false, "enable data checksums")
	pgInitCmd.Flags().BoolVarP(&pgInitForce, "force", "f", false, "force init, remove existing data directory (DANGEROUS)")

	// start subcommand flags
	pgStartCmd.Flags().StringVarP(&pgStartLog, "log", "l", "", "redirect stdout/stderr to log file")
	pgStartCmd.Flags().IntVarP(&pgStartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStartCmd.Flags().BoolVarP(&pgStartNoWait, "no-wait", "W", false, "do not wait for startup")
	pgStartCmd.Flags().StringVarP(&pgStartOptions, "options", "o", "", "options passed to postgres")
	pgStartCmd.Flags().BoolVarP(&pgStartYes, "yes", "y", false, "force start even if already running")

	// stop subcommand flags
	pgStopCmd.Flags().StringVarP(&pgStopMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgStopCmd.Flags().IntVarP(&pgStopTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStopCmd.Flags().BoolVarP(&pgStopNoWait, "no-wait", "W", false, "do not wait for shutdown")

	// restart subcommand flags
	pgRestartCmd.Flags().StringVarP(&pgRestartMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgRestartCmd.Flags().IntVarP(&pgRestartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgRestartCmd.Flags().BoolVarP(&pgRestartNoWait, "no-wait", "W", false, "do not wait for restart")
	pgRestartCmd.Flags().StringVarP(&pgRestartOptions, "options", "o", "", "options passed to postgres")

	// promote subcommand flags
	pgPromoteCmd.Flags().IntVarP(&pgPromoteTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgPromoteCmd.Flags().BoolVarP(&pgPromoteNoWait, "no-wait", "W", false, "do not wait for promotion")

	// role subcommand flags
	pgRoleCmd.Flags().BoolVarP(&pgRoleVerbose, "verbose", "V", false, "show detailed detection process")

	// Register subcommands - Phase 1
	pgCmd.AddCommand(pgInitCmd)
	pgCmd.AddCommand(pgStartCmd)
	pgCmd.AddCommand(pgStopCmd)
	pgCmd.AddCommand(pgRestartCmd)
	pgCmd.AddCommand(pgReloadCmd)
	pgCmd.AddCommand(pgStatusCmd)
	pgCmd.AddCommand(pgPromoteCmd)
	pgCmd.AddCommand(pgRoleCmd)

	// ========== Phase 2 Commands ==========

	// Log command flags
	pgLogCmd.PersistentFlags().StringVar(&pgConfig.LogDir, "log-dir", "", "log directory (default: /pg/log/postgres)")
	pgLogCmd.PersistentFlags().IntVarP(&pgLogNum, "lines", "n", 0, "number of lines")
	pgLogGrepCmd.Flags().BoolVarP(&pgLogGrepIgnoreCase, "ignore-case", "i", false, "ignore case")
	pgLogGrepCmd.Flags().IntVarP(&pgLogGrepContext, "context", "C", 0, "show N lines of context")

	// Log subcommands
	pgLogCmd.AddCommand(pgLogListCmd)
	pgLogCmd.AddCommand(pgLogTailCmd)
	pgLogCmd.AddCommand(pgLogCatCmd)
	pgLogCmd.AddCommand(pgLogLessCmd)
	pgLogCmd.AddCommand(pgLogGrepCmd)
	pgCmd.AddCommand(pgLogCmd)

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

	// Maintenance command shared flags helper
	addMaintFlags := func(cmd *cobra.Command) {
		cmd.Flags().BoolVarP(&pgMaintAll, "all", "a", false, "process all databases")
		cmd.Flags().StringVarP(&pgMaintSchema, "schema", "n", "", "schema name")
		cmd.Flags().StringVarP(&pgMaintTable, "table", "t", "", "table name")
		cmd.Flags().BoolVarP(&pgMaintVerbose, "verbose", "V", false, "verbose output")
	}

	// vacuum command
	addMaintFlags(pgVacuumCmd)
	pgVacuumCmd.Flags().BoolVarP(&pgMaintFull, "full", "F", false, "VACUUM FULL (requires exclusive lock)")
	pgCmd.AddCommand(pgVacuumCmd)

	// analyze command
	addMaintFlags(pgAnalyzeCmd)
	pgCmd.AddCommand(pgAnalyzeCmd)

	// freeze command
	pgFreezeCmd.Flags().BoolVarP(&pgMaintAll, "all", "a", false, "process all databases")
	pgFreezeCmd.Flags().StringVarP(&pgMaintSchema, "schema", "n", "", "schema name")
	pgFreezeCmd.Flags().StringVarP(&pgMaintTable, "table", "t", "", "table name")
	pgFreezeCmd.Flags().BoolVarP(&pgMaintVerbose, "verbose", "V", false, "verbose output")
	pgCmd.AddCommand(pgFreezeCmd)

	// repack command
	addMaintFlags(pgRepackCmd)
	pgRepackCmd.Flags().IntVarP(&pgMaintJobs, "jobs", "j", 1, "number of parallel jobs")
	pgRepackCmd.Flags().BoolVarP(&pgMaintDryRun, "dry-run", "N", false, "show what would be repacked")
	pgCmd.AddCommand(pgRepackCmd)

	// ========== Service Management Commands (systemctl) ==========
	pgSvcCmd.AddCommand(
		pgSvcStartCmd,
		pgSvcStopCmd,
		pgSvcRestartCmd,
		pgSvcReloadCmd,
		pgSvcStatusCmd,
	)
	pgCmd.AddCommand(pgSvcCmd)
}
