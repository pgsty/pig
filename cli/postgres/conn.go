/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

PostgreSQL connection management: ps, kill
*/
package postgres

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"pig/internal/utils"
)

// identifierRegex validates PostgreSQL identifiers (usernames, database names)
// Allows alphanumeric, underscore, and dollar sign (PostgreSQL naming rules)
var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]*$`)

// validateIdentifier checks if a string is a valid PostgreSQL identifier
func validateIdentifier(s string) bool {
	if s == "" {
		return true // empty is allowed (means no filter)
	}
	return identifierRegex.MatchString(s)
}

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
		return fmt.Errorf("PostgreSQL not found: %w", err)
	}

	// Validate identifiers to prevent SQL injection
	if opts != nil {
		if !validateIdentifier(opts.User) {
			return fmt.Errorf("invalid username: %s", opts.User)
		}
		if !validateIdentifier(opts.Database) {
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

// Kill kills PostgreSQL connections (dry-run by default)
func Kill(cfg *Config, opts *KillOptions) error {
	dbsu := GetDbSU(cfg)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("PostgreSQL not found: %w", err)
	}

	// Validate identifiers to prevent SQL injection
	if opts != nil {
		if !validateIdentifier(opts.User) {
			return fmt.Errorf("invalid username: %s", opts.User)
		}
		if !validateIdentifier(opts.Db) {
			return fmt.Errorf("invalid database name: %s", opts.Db)
		}
		if !validateIdentifier(opts.State) {
			return fmt.Errorf("invalid state: %s", opts.State)
		}
		if !validateIdentifier(opts.Query) {
			return fmt.Errorf("invalid query pattern: %s (use simple alphanumeric patterns)", opts.Query)
		}
	}

	// Choose function: pg_cancel_backend or pg_terminate_backend
	killFunc := "pg_terminate_backend"
	if opts != nil && opts.Cancel {
		killFunc = "pg_cancel_backend"
	}

	for {
		var sql string
		if opts != nil && opts.Pid > 0 {
			// Kill specific PID
			sql = fmt.Sprintf("SELECT %s(%d)", killFunc, opts.Pid)
		} else {
			// Build WHERE clause
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
				conditions = append(conditions, fmt.Sprintf("state = '%s'", opts.State))
			}
			if opts != nil && opts.Query != "" {
				conditions = append(conditions, fmt.Sprintf("query ILIKE '%%%s%%'", opts.Query))
			}

			whereClause := strings.Join(conditions, " AND ")

			if opts != nil && opts.Execute {
				sql = fmt.Sprintf("SELECT %s(pid), pid, usename, datname, state FROM pg_stat_activity WHERE %s", killFunc, whereClause)
			} else {
				// Dry-run: just show what would be killed
				sql = fmt.Sprintf("SELECT pid, usename, datname, client_addr, state, LEFT(query, 40) AS query FROM pg_stat_activity WHERE %s", whereClause)
			}
		}

		if (opts == nil || !opts.Execute) && (opts == nil || opts.Pid == 0) {
			fmt.Printf("%s[DRY-RUN] Connections that would be killed:%s\n", ColorYellow, ColorReset)
		}

		cmdArgs := []string{pg.Psql(), "-d", "postgres", "-c", sql}
		PrintHint(cmdArgs)
		if err := utils.DBSUCommand(dbsu, cmdArgs); err != nil {
			return err
		}

		if (opts == nil || !opts.Execute) && (opts == nil || opts.Pid == 0) {
			fmt.Printf("\n%sUse -x/--execute to actually kill these connections%s\n", ColorYellow, ColorReset)
		}

		// Watch mode
		if opts == nil || opts.Watch <= 0 {
			break
		}
		fmt.Printf("\n%sWaiting %d seconds... (Ctrl+C to stop)%s\n", ColorCyan, opts.Watch, ColorReset)
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
		return fmt.Errorf("PostgreSQL not found: %w", err)
	}

	// Validate database name
	if !validateIdentifier(dbname) {
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
