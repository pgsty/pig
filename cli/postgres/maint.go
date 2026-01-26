/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

PostgreSQL maintenance operations: vacuum, analyze, freeze, repack
*/
package postgres

import (
	"fmt"
	"os/exec"
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

	// Use DBSUCommandOutput for proper handling of all user types (dbsu, root, sudo user)
	output, err := utils.DBSUCommandOutput(dbsu, cmdArgs)
	if err != nil {
		return nil, err
	}

	var dbs []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" {
			dbs = append(dbs, line)
		}
	}
	return dbs, nil
}

// ============================================================================
// Common Maintenance Executor
// ============================================================================

// maintTask represents a maintenance task configuration
type maintTask struct {
	command   string // SQL command name (VACUUM, ANALYZE)
	options   string // SQL options string (e.g., "(VERBOSE, FREEZE)")
	taskName  string // Display name for logging
	schema    string // Target schema (optional)
	table     string // Target table (optional)
}

// runMaintTask executes a maintenance task on a single database
func runMaintTask(cfg *Config, dbname string, task *maintTask) error {
	var sql string
	if task.table != "" {
		// Specific table
		table := task.table
		if task.schema != "" {
			table = task.schema + "." + task.table
		}
		sql = fmt.Sprintf("%s %s %s", task.command, task.options, table)
	} else if task.schema != "" {
		// All tables in schema (need to iterate via DO block)
		sql = fmt.Sprintf(`DO $$ DECLARE r RECORD; BEGIN
FOR r IN SELECT schemaname, tablename FROM pg_tables WHERE schemaname = '%s'
LOOP EXECUTE '%s %s ' || quote_ident(r.schemaname) || '.' || quote_ident(r.tablename);
END LOOP; END $$`, task.schema, task.command, task.options)
	} else {
		// Entire database
		sql = fmt.Sprintf("%s %s", task.command, task.options)
	}

	return RunPsqlMaintenance(cfg, dbname, sql)
}

// runMaintAllDatabases executes a maintenance task on all databases
func runMaintAllDatabases(cfg *Config, task *maintTask) error {
	dbs, err := GetAllDatabases(cfg)
	if err != nil {
		return fmt.Errorf("failed to get databases: %w", err)
	}
	for _, db := range dbs {
		fmt.Printf("\n%s=== %s database: %s ===%s\n", ColorCyan, task.taskName, db, ColorReset)
		sql := fmt.Sprintf("%s %s", task.command, task.options)
		if err := RunPsqlMaintenance(cfg, db, sql); err != nil {
			logrus.Warnf("%s %s failed: %v", strings.ToLower(task.taskName), db, err)
		}
	}
	return nil
}

// validateMaintOptions validates common maintenance options
func validateMaintOptions(schema, table string) error {
	if !ValidateIdentifier(schema) {
		return fmt.Errorf("invalid schema name: %s", schema)
	}
	if !ValidateIdentifier(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	return nil
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
	// Get effective options
	var schema, table string
	var all, verbose, full bool
	if opts != nil {
		schema, table = opts.Schema, opts.Table
		all, verbose, full = opts.All, opts.Verbose, opts.Full
	}

	// Validate identifiers
	if err := validateMaintOptions(schema, table); err != nil {
		return err
	}

	// Build VACUUM options
	var vacOpts []string
	if verbose {
		vacOpts = append(vacOpts, "VERBOSE")
	}
	if full {
		vacOpts = append(vacOpts, "FULL")
	}
	optStr := ""
	if len(vacOpts) > 0 {
		optStr = "(" + strings.Join(vacOpts, ", ") + ")"
	}

	task := &maintTask{
		command:  "VACUUM",
		options:  optStr,
		taskName: "Vacuuming",
		schema:   schema,
		table:    table,
	}

	if all {
		return runMaintAllDatabases(cfg, task)
	}

	if dbname == "" {
		dbname = "postgres"
	}
	return runMaintTask(cfg, dbname, task)
}

// ============================================================================
// Analyze
// ============================================================================

// Analyze runs ANALYZE on database tables
func Analyze(cfg *Config, dbname string, opts *MaintOptions) error {
	// Get effective options
	var schema, table string
	var all, verbose bool
	if opts != nil {
		schema, table = opts.Schema, opts.Table
		all, verbose = opts.All, opts.Verbose
	}

	// Validate identifiers
	if err := validateMaintOptions(schema, table); err != nil {
		return err
	}

	optStr := ""
	if verbose {
		optStr = "(VERBOSE)"
	}

	task := &maintTask{
		command:  "ANALYZE",
		options:  optStr,
		taskName: "Analyzing",
		schema:   schema,
		table:    table,
	}

	if all {
		return runMaintAllDatabases(cfg, task)
	}

	if dbname == "" {
		dbname = "postgres"
	}
	return runMaintTask(cfg, dbname, task)
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
	// Get effective options
	var schema, table string
	var all, verbose bool
	if opts != nil {
		schema, table = opts.Schema, opts.Table
		all, verbose = opts.All, opts.Verbose
	}

	// Validate identifiers
	if err := validateMaintOptions(schema, table); err != nil {
		return err
	}

	// Build VACUUM FREEZE options
	vacOpts := []string{"FREEZE"}
	if verbose {
		vacOpts = append(vacOpts, "VERBOSE")
	}
	optStr := "(" + strings.Join(vacOpts, ", ") + ")"

	task := &maintTask{
		command:  "VACUUM",
		options:  optStr,
		taskName: "Freezing",
		schema:   schema,
		table:    table,
	}

	if all {
		return runMaintAllDatabases(cfg, task)
	}

	if dbname == "" {
		dbname = "postgres"
	}
	return runMaintTask(cfg, dbname, task)
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
	// Get effective options
	var schema, table string
	var all bool
	var jobs int
	var dryRun bool
	if opts != nil {
		schema, table = opts.Schema, opts.Table
		all = opts.All
		jobs = opts.Jobs
		dryRun = opts.DryRun
	}

	// Validate identifiers
	if err := validateMaintOptions(schema, table); err != nil {
		return err
	}

	dbsu := GetDbSU(cfg)

	// Check if pg_repack exists
	if _, err := exec.LookPath("pg_repack"); err != nil {
		return fmt.Errorf("pg_repack not found in PATH (install with: pig ext add pg_repack)")
	}

	// Build pg_repack command
	cmdArgs := []string{"pg_repack"}

	if dryRun {
		cmdArgs = append(cmdArgs, "-N")
	}
	if jobs > 1 {
		cmdArgs = append(cmdArgs, "-j", strconv.Itoa(jobs))
	}

	if all {
		cmdArgs = append(cmdArgs, "-a")
	} else {
		if dbname == "" {
			dbname = "postgres"
		}
		cmdArgs = append(cmdArgs, "-d", dbname)

		if table != "" {
			t := table
			if schema != "" {
				t = schema + "." + table
			}
			cmdArgs = append(cmdArgs, "-t", t)
		} else if schema != "" {
			cmdArgs = append(cmdArgs, "-c", schema)
		}
	}

	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}
