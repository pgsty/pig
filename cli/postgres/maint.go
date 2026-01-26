/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

PostgreSQL maintenance operations: vacuum, analyze, freeze, repack
*/
package postgres

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ============================================================================
// Common Maintenance Options
// ============================================================================

// MaintOptions contains common options for maintenance commands
type MaintOptions struct {
	All     bool   // process all databases
	Schema  string // schema name
	Table   string // table name
	Verbose bool   // verbose output
}

// identifierRegex validates PostgreSQL identifiers
var maintIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]*$`)

// validateMaintIdentifier checks if a string is a valid PostgreSQL identifier
func validateMaintIdentifier(s string) bool {
	if s == "" {
		return true
	}
	return maintIdentifierRegex.MatchString(s)
}

// ============================================================================
// SQL Execution Helpers
// ============================================================================

// RunPsqlMaintenance runs a maintenance SQL command
func RunPsqlMaintenance(cfg *Config, dbname, sql string) error {
	dbsu := GetDbSU(cfg)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("PostgreSQL not found: %w", err)
	}

	if dbname == "" {
		dbname = "postgres"
	}

	cmdArgs := []string{pg.Psql(), "-d", dbname, "-c", sql}
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// GetAllDatabases returns list of all user databases
func GetAllDatabases(cfg *Config) ([]string, error) {
	dbsu := GetDbSU(cfg)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return nil, err
	}

	// Build command args
	cmdArgs := []string{pg.Psql(), "-d", "postgres", "-t", "-A", "-c",
		"SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"}

	// Use proper DBSU handling (check if current user is already dbsu)
	var cmd *exec.Cmd
	if utils.IsDBSU(dbsu) {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		sudoArgs := append([]string{"-inu", dbsu, "--"}, cmdArgs...)
		cmd = exec.Command("sudo", sudoArgs...)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var dbs []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			dbs = append(dbs, line)
		}
	}
	return dbs, nil
}

// ============================================================================
// Vacuum
// ============================================================================

// VacuumOptions contains options for Vacuum command
type VacuumOptions struct {
	MaintOptions
	Full bool // VACUUM FULL (requires exclusive lock)
}

// Vacuum runs VACUUM on database tables
func Vacuum(cfg *Config, dbname string, opts *VacuumOptions) error {
	// Validate identifiers
	if opts != nil {
		if !validateMaintIdentifier(opts.Schema) {
			return fmt.Errorf("invalid schema name: %s", opts.Schema)
		}
		if !validateMaintIdentifier(opts.Table) {
			return fmt.Errorf("invalid table name: %s", opts.Table)
		}
	}

	// Build VACUUM options
	var vacOpts []string
	if opts != nil && opts.Verbose {
		vacOpts = append(vacOpts, "VERBOSE")
	}
	if opts != nil && opts.Full {
		vacOpts = append(vacOpts, "FULL")
	}
	optStr := ""
	if len(vacOpts) > 0 {
		optStr = "(" + strings.Join(vacOpts, ", ") + ")"
	}

	// Determine target
	if opts != nil && opts.All {
		// Vacuum all databases
		dbs, err := GetAllDatabases(cfg)
		if err != nil {
			return fmt.Errorf("failed to get databases: %w", err)
		}
		for _, db := range dbs {
			fmt.Printf("\n%s=== Vacuuming database: %s ===%s\n", ColorCyan, db, ColorReset)
			sql := fmt.Sprintf("VACUUM %s", optStr)
			if err := RunPsqlMaintenance(cfg, db, sql); err != nil {
				logrus.Warnf("vacuum %s failed: %v", db, err)
			}
		}
		return nil
	}

	if dbname == "" {
		dbname = "postgres"
	}

	var sql string
	if opts != nil && opts.Table != "" {
		// Vacuum specific table
		table := opts.Table
		if opts.Schema != "" {
			table = opts.Schema + "." + opts.Table
		}
		sql = fmt.Sprintf("VACUUM %s %s", optStr, table)
	} else if opts != nil && opts.Schema != "" {
		// Vacuum all tables in schema (need to iterate)
		sql = fmt.Sprintf(`DO $$ DECLARE r RECORD; BEGIN
FOR r IN SELECT schemaname, tablename FROM pg_tables WHERE schemaname = '%s'
LOOP EXECUTE 'VACUUM %s ' || quote_ident(r.schemaname) || '.' || quote_ident(r.tablename);
END LOOP; END $$`, opts.Schema, optStr)
	} else {
		sql = fmt.Sprintf("VACUUM %s", optStr)
	}

	return RunPsqlMaintenance(cfg, dbname, sql)
}

// ============================================================================
// Analyze
// ============================================================================

// Analyze runs ANALYZE on database tables
func Analyze(cfg *Config, dbname string, opts *MaintOptions) error {
	// Validate identifiers
	if opts != nil {
		if !validateMaintIdentifier(opts.Schema) {
			return fmt.Errorf("invalid schema name: %s", opts.Schema)
		}
		if !validateMaintIdentifier(opts.Table) {
			return fmt.Errorf("invalid table name: %s", opts.Table)
		}
	}

	optStr := ""
	if opts != nil && opts.Verbose {
		optStr = "(VERBOSE)"
	}

	if opts != nil && opts.All {
		dbs, err := GetAllDatabases(cfg)
		if err != nil {
			return fmt.Errorf("failed to get databases: %w", err)
		}
		for _, db := range dbs {
			fmt.Printf("\n%s=== Analyzing database: %s ===%s\n", ColorCyan, db, ColorReset)
			sql := fmt.Sprintf("ANALYZE %s", optStr)
			if err := RunPsqlMaintenance(cfg, db, sql); err != nil {
				logrus.Warnf("analyze %s failed: %v", db, err)
			}
		}
		return nil
	}

	if dbname == "" {
		dbname = "postgres"
	}

	var sql string
	if opts != nil && opts.Table != "" {
		table := opts.Table
		if opts.Schema != "" {
			table = opts.Schema + "." + opts.Table
		}
		sql = fmt.Sprintf("ANALYZE %s %s", optStr, table)
	} else if opts != nil && opts.Schema != "" {
		sql = fmt.Sprintf(`DO $$ DECLARE r RECORD; BEGIN
FOR r IN SELECT schemaname, tablename FROM pg_tables WHERE schemaname = '%s'
LOOP EXECUTE 'ANALYZE %s ' || quote_ident(r.schemaname) || '.' || quote_ident(r.tablename);
END LOOP; END $$`, opts.Schema, optStr)
	} else {
		sql = fmt.Sprintf("ANALYZE %s", optStr)
	}

	return RunPsqlMaintenance(cfg, dbname, sql)
}

// ============================================================================
// Freeze
// ============================================================================

// FreezeOptions contains options for Freeze command
type FreezeOptions struct {
	All     bool
	Schema  string
	Table   string
	Verbose bool
}

// Freeze runs VACUUM FREEZE on database
func Freeze(cfg *Config, dbname string, opts *FreezeOptions) error {
	// Validate identifiers
	if opts != nil {
		if !validateMaintIdentifier(opts.Schema) {
			return fmt.Errorf("invalid schema name: %s", opts.Schema)
		}
		if !validateMaintIdentifier(opts.Table) {
			return fmt.Errorf("invalid table name: %s", opts.Table)
		}
	}

	vacOpts := []string{"FREEZE"}
	if opts != nil && opts.Verbose {
		vacOpts = append(vacOpts, "VERBOSE")
	}
	optStr := "(" + strings.Join(vacOpts, ", ") + ")"

	if opts != nil && opts.All {
		dbs, err := GetAllDatabases(cfg)
		if err != nil {
			return fmt.Errorf("failed to get databases: %w", err)
		}
		for _, db := range dbs {
			fmt.Printf("\n%s=== Freezing database: %s ===%s\n", ColorCyan, db, ColorReset)
			sql := fmt.Sprintf("VACUUM %s", optStr)
			if err := RunPsqlMaintenance(cfg, db, sql); err != nil {
				logrus.Warnf("freeze %s failed: %v", db, err)
			}
		}
		return nil
	}

	if dbname == "" {
		dbname = "postgres"
	}

	var sql string
	if opts != nil && opts.Table != "" {
		table := opts.Table
		if opts.Schema != "" {
			table = opts.Schema + "." + opts.Table
		}
		sql = fmt.Sprintf("VACUUM %s %s", optStr, table)
	} else if opts != nil && opts.Schema != "" {
		sql = fmt.Sprintf(`DO $$ DECLARE r RECORD; BEGIN
FOR r IN SELECT schemaname, tablename FROM pg_tables WHERE schemaname = '%s'
LOOP EXECUTE 'VACUUM %s ' || quote_ident(r.schemaname) || '.' || quote_ident(r.tablename);
END LOOP; END $$`, opts.Schema, optStr)
	} else {
		sql = fmt.Sprintf("VACUUM %s", optStr)
	}

	return RunPsqlMaintenance(cfg, dbname, sql)
}

// ============================================================================
// Repack
// ============================================================================

// RepackOptions contains options for Repack command
type RepackOptions struct {
	MaintOptions
	Jobs   int  // number of parallel jobs
	DryRun bool // show what would be repacked
}

// Repack runs pg_repack on database tables
func Repack(cfg *Config, dbname string, opts *RepackOptions) error {
	// Validate identifiers
	if opts != nil {
		if !validateMaintIdentifier(opts.Schema) {
			return fmt.Errorf("invalid schema name: %s", opts.Schema)
		}
		if !validateMaintIdentifier(opts.Table) {
			return fmt.Errorf("invalid table name: %s", opts.Table)
		}
	}

	dbsu := GetDbSU(cfg)

	// Check if pg_repack exists
	if _, err := exec.LookPath("pg_repack"); err != nil {
		return fmt.Errorf("pg_repack not found in PATH (install with: pig ext add pg_repack)")
	}

	// Build pg_repack command
	cmdArgs := []string{"pg_repack"}

	if opts != nil && opts.DryRun {
		cmdArgs = append(cmdArgs, "-N")
	}
	if opts != nil && opts.Jobs > 1 {
		cmdArgs = append(cmdArgs, "-j", strconv.Itoa(opts.Jobs))
	}

	if opts != nil && opts.All {
		cmdArgs = append(cmdArgs, "-a")
	} else {
		if dbname == "" {
			dbname = "postgres"
		}
		cmdArgs = append(cmdArgs, "-d", dbname)

		if opts != nil && opts.Table != "" {
			table := opts.Table
			if opts.Schema != "" {
				table = opts.Schema + "." + opts.Table
			}
			cmdArgs = append(cmdArgs, "-t", table)
		} else if opts != nil && opts.Schema != "" {
			cmdArgs = append(cmdArgs, "-c", opts.Schema)
		}
	}

	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}
