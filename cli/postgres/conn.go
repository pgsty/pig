/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

PostgreSQL connection management: ps, kill
*/
package postgres

import (
	"fmt"
	"strings"
	"time"

	"pig/internal/utils"
)

// ============================================================================
// Connection List (ps)
// ============================================================================

// PsOptions contains options for Ps command
type PsOptions struct {
	All      bool   // show all connections (including system)
	User     string // filter by user
	Database string // filter by database
}

// Ps shows PostgreSQL connections
func Ps(cfg *Config, opts *PsOptions) error {
	dbsu := GetDbSU(cfg)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Validate identifiers to prevent SQL injection
	if opts != nil {
		if !ValidateIdentifier(opts.User) {
			return fmt.Errorf("invalid username: %s", opts.User)
		}
		if !ValidateIdentifier(opts.Database) {
			return fmt.Errorf("invalid database name: %s", opts.Database)
		}
	}

	// Build SQL query
	sql := `SELECT pid, usename AS user, datname AS db,
       CASE WHEN client_addr IS NULL THEN 'local' ELSE client_addr::text END AS client,
       state, COALESCE(LEFT(query, 50), '') AS query
  FROM pg_stat_activity WHERE pid <> pg_backend_pid()`

	if opts == nil || !opts.All {
		sql += " AND backend_type = 'client backend'"
	}
	if opts != nil && opts.User != "" {
		sql += fmt.Sprintf(" AND usename = '%s'", opts.User)
	}
	if opts != nil && opts.Database != "" {
		sql += fmt.Sprintf(" AND datname = '%s'", opts.Database)
	}
	sql += " ORDER BY state, pid"

	cmdArgs := []string{pg.Psql(), "-d", "postgres", "-c", sql}
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// ============================================================================
// Connection Kill
// ============================================================================

// KillOptions contains options for Kill command
type KillOptions struct {
	Execute bool   // actually kill (default is dry-run)
	Pid     int    // kill specific PID
	User    string // filter by user
	Db      string // filter by database
	State   string // filter by state (idle/active/idle in transaction)
	Query   string // filter by query pattern
	All     bool   // include replication connections
	Cancel  bool   // cancel query instead of terminate
	Watch   int    // repeat every N seconds
}

func validateKillOptions(opts *KillOptions) error {
	// Validate inputs to prevent SQL injection.
	if opts == nil {
		return nil
	}

	if !ValidateIdentifier(opts.User) {
		return fmt.Errorf("invalid username: %s", opts.User)
	}
	if !ValidateIdentifier(opts.Db) {
		return fmt.Errorf("invalid database name: %s", opts.Db)
	}
	// State can contain spaces (e.g., "idle in transaction"), validate against known values.
	if !utils.ValidateConnectionState(opts.State) {
		return fmt.Errorf("invalid state: %s (valid: active, idle, idle in transaction)", opts.State)
	}
	// Query pattern: allow alphanumeric, spaces, and wildcards; escape SQL special chars.
	if !utils.ValidateSQLLikePattern(opts.Query) {
		return fmt.Errorf("invalid query pattern: %s (use alphanumeric characters, spaces, and wildcards)", opts.Query)
	}
	return nil
}

func pickKillFunc(opts *KillOptions) string {
	// Choose function: pg_cancel_backend or pg_terminate_backend.
	if opts != nil && opts.Cancel {
		return "pg_cancel_backend"
	}
	return "pg_terminate_backend"
}

func buildKillWhereClause(opts *KillOptions) string {
	conditions := []string{"pid <> pg_backend_pid()"}
	if opts == nil || !opts.All {
		conditions = append(conditions, "backend_type = 'client backend'")
	}
	if opts != nil && opts.User != "" {
		conditions = append(conditions, fmt.Sprintf("usename = '%s'", opts.User))
	}
	if opts != nil && opts.Db != "" {
		conditions = append(conditions, fmt.Sprintf("datname = '%s'", opts.Db))
	}
	if opts != nil && opts.State != "" {
		conditions = append(conditions, fmt.Sprintf("state = '%s'", utils.EscapeSQLString(opts.State)))
	}
	if opts != nil && opts.Query != "" {
		// Escape LIKE wildcards and single quotes for safe pattern matching.
		escapedQuery := utils.EscapeSQLLikePattern(opts.Query)
		conditions = append(conditions, fmt.Sprintf("query ILIKE '%%%s%%' ESCAPE '\\\\'", escapedQuery))
	}
	return strings.Join(conditions, " AND ")
}

func buildKillSQL(killFunc string, opts *KillOptions) string {
	if opts != nil && opts.Pid > 0 {
		return fmt.Sprintf("SELECT %s(%d)", killFunc, opts.Pid)
	}

	whereClause := buildKillWhereClause(opts)
	if opts != nil && opts.Execute {
		return fmt.Sprintf("SELECT %s(pid), pid, usename, datname, state FROM pg_stat_activity WHERE %s", killFunc, whereClause)
	}

	// Dry-run: just show what would be killed.
	return fmt.Sprintf("SELECT pid, usename, datname, client_addr, state, LEFT(query, 40) AS query FROM pg_stat_activity WHERE %s", whereClause)
}

func shouldShowKillDryRunBanner(opts *KillOptions) bool {
	return (opts == nil || !opts.Execute) && (opts == nil || opts.Pid == 0)
}

// Kill kills PostgreSQL connections (dry-run by default)
func Kill(cfg *Config, opts *KillOptions) error {
	dbsu := GetDbSU(cfg)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	if err := validateKillOptions(opts); err != nil {
		return err
	}

	killFunc := pickKillFunc(opts)

	for {
		sql := buildKillSQL(killFunc, opts)
		showDryRunBanner := shouldShowKillDryRunBanner(opts)

		if showDryRunBanner {
			fmt.Printf("%s[DRY-RUN] Connections that would be killed:%s\n", utils.ColorYellow, utils.ColorReset)
		}

		cmdArgs := []string{pg.Psql(), "-d", "postgres", "-c", sql}
		PrintHint(cmdArgs)
		if err := utils.DBSUCommand(dbsu, cmdArgs); err != nil {
			return err
		}

		if showDryRunBanner {
			fmt.Printf("\n%sUse -x/--execute to actually kill these connections%s\n", utils.ColorYellow, utils.ColorReset)
		}

		// Watch mode
		if opts == nil || opts.Watch <= 0 {
			break
		}
		fmt.Printf("\n%sWaiting %d seconds... (Ctrl+C to stop)%s\n", utils.ColorCyan, opts.Watch, utils.ColorReset)
		time.Sleep(time.Duration(opts.Watch) * time.Second)
	}

	return nil
}

// ============================================================================
// Interactive psql Session
// ============================================================================

// PsqlOptions contains options for Psql command
type PsqlOptions struct {
	Command string // -c: run single command
	File    string // -f: run commands from file
}

// Psql starts an interactive psql session
func Psql(cfg *Config, dbname string, opts *PsqlOptions) error {
	dbsu := GetDbSU(cfg)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Validate database name
	if !ValidateIdentifier(dbname) {
		return fmt.Errorf("invalid database name: %s", dbname)
	}

	// Default to postgres database
	if dbname == "" {
		dbname = "postgres"
	}

	// Build psql command
	cmdArgs := []string{pg.Psql(), "-d", dbname}

	// Add options
	if opts != nil && opts.Command != "" {
		cmdArgs = append(cmdArgs, "-c", opts.Command)
	}
	if opts != nil && opts.File != "" {
		cmdArgs = append(cmdArgs, "-f", opts.File)
	}

	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}
