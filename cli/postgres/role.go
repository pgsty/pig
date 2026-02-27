/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

PostgreSQL role detection: determine if instance is primary or replica.
Uses multiple fallback strategies:
  1. SQL query (pg_is_in_recovery) - most reliable when PG is running
  2. Process detection (walreceiver, recovering) - works if PG is running
  3. Data directory inspection (standby.signal, recovery.signal, recovery.conf) - works even if PG is down
*/
package postgres

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// Role represents PostgreSQL instance role
type Role string

const (
	RolePrimary Role = "primary"
	RoleReplica Role = "replica"
	RoleUnknown Role = "unknown"
)

// RoleResult contains role detection result with metadata
type RoleResult struct {
	Role   Role   // primary, replica, or unknown
	Alive  bool   // whether PostgreSQL is running
	Source string // detection method used
}

// RoleOptions contains options for Role command
type RoleOptions struct {
	Verbose bool // show detailed detection process
}

// DetectRole detects PostgreSQL instance role using multiple strategies
func DetectRole(cfg *Config, opts *RoleOptions) (*RoleResult, error) {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)
	verbose := opts != nil && opts.Verbose

	// Strategy 1: Check processes first (quick, non-invasive)
	if verbose {
		fmt.Printf("%s[1] Checking PostgreSQL processes...%s\n", utils.ColorCyan, utils.ColorReset)
	}
	psResult := roleFromProcesses(dbsu, verbose)
	if verbose {
		fmt.Printf("    Process check: alive=%v, role=%s\n", psResult.Alive, psResult.Role)
	}

	// Strategy 2: If alive, try SQL query (most accurate)
	if psResult.Alive {
		if verbose {
			fmt.Printf("%s[2] Querying pg_is_in_recovery()...%s\n", utils.ColorCyan, utils.ColorReset)
		}
		sqlResult := roleFromSQL(cfg, dbsu, verbose)
		if sqlResult.Role != RoleUnknown {
			if verbose {
				fmt.Printf("    SQL query: role=%s\n", sqlResult.Role)
			}
			// SQL result is authoritative
			if sqlResult.Role != psResult.Role && psResult.Role != RoleUnknown {
				logrus.Warnf("SQL role (%s) differs from process role (%s), using SQL result",
					sqlResult.Role, psResult.Role)
			}
			return sqlResult, nil
		}
		// SQL failed, fall back to process result
		if psResult.Role != RoleUnknown {
			return psResult, nil
		}
	}

	// Strategy 3: Check data directory files (works when PG is down)
	if verbose {
		fmt.Printf("%s[3] Checking data directory files...%s\n", utils.ColorCyan, utils.ColorReset)
	}
	fileResult := roleFromDataDir(dbsu, dataDir, verbose)
	if verbose {
		fmt.Printf("    File check: alive=%v, role=%s\n", fileResult.Alive, fileResult.Role)
	}

	// Return file result if we got something
	if fileResult.Role != RoleUnknown {
		// Preserve alive status from process check
		if psResult.Alive {
			fileResult.Alive = true
		}
		return fileResult, nil
	}

	// If process check found PostgreSQL alive but we couldn't determine role
	if psResult.Alive {
		return &RoleResult{
			Role:   RoleUnknown,
			Alive:  true,
			Source: "none",
		}, nil
	}

	return &RoleResult{
		Role:   RoleUnknown,
		Alive:  false,
		Source: "none",
	}, nil
}

// roleFromProcesses checks PostgreSQL processes to determine role
func roleFromProcesses(dbsu string, verbose bool) *RoleResult {
	result := &RoleResult{
		Role:   RoleUnknown,
		Alive:  false,
		Source: "ps",
	}

	// Get processes for postgres user
	cmd := exec.Command("ps", "h", "-u", dbsu, "-o", "command")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		if verbose {
			fmt.Printf("    ps command failed: %v\n", err)
		}
		return result
	}

	lines := strings.Split(out.String(), "\n")
	hasRecovery := false
	hasWalreceiver := false
	hasMainProcess := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for main postgres process
		if (strings.Contains(line, "post") && strings.Contains(line, "-D")) ||
			(strings.Contains(line, "postgres:") && strings.Contains(line, "checkpointer")) {
			hasMainProcess = true
		}

		// Check for background processes indicating alive
		if strings.Contains(line, "postgres:") {
			for _, proc := range []string{"logger", "checkpointer", "background writer", "stats collector", "walwriter"} {
				if strings.Contains(line, proc) {
					hasMainProcess = true
					break
				}
			}

			// Check for recovery indicators (replica)
			if strings.Contains(line, "walreceiver") {
				hasWalreceiver = true
				hasRecovery = true
			}
			if strings.Contains(line, "recovering") {
				hasRecovery = true
			}

			// walsender indicates this node has replicas (but doesn't determine role)
			if strings.Contains(line, "walsender") {
				hasMainProcess = true
			}
		}
	}

	if verbose {
		fmt.Printf("    hasMainProcess=%v, hasRecovery=%v, hasWalreceiver=%v\n",
			hasMainProcess, hasRecovery, hasWalreceiver)
	}

	result.Alive = hasMainProcess
	if hasMainProcess {
		if hasRecovery {
			result.Role = RoleReplica
		} else {
			result.Role = RolePrimary
		}
	}

	return result
}

// roleFromSQL queries PostgreSQL to determine role
func roleFromSQL(cfg *Config, dbsu string, verbose bool) *RoleResult {
	result := &RoleResult{
		Role:   RoleUnknown,
		Alive:  false,
		Source: "psql",
	}

	// Find PostgreSQL installation
	pg, err := GetPgInstall(cfg)
	if err != nil {
		if verbose {
			fmt.Printf("    postgresql not found: %v\n", err)
		}
		return result
	}

	// Build psql command to check pg_is_in_recovery()
	cmdArgs := []string{pg.Psql(), "-AXtqw", "-d", "postgres", "-c", "SELECT pg_is_in_recovery()"}

	// Use utils.DBSUCommandOutput for consistent privilege escalation
	output, err := utils.DBSUCommandOutput(dbsu, cmdArgs)
	if err != nil {
		if verbose {
			fmt.Printf("    psql command failed: %v\n", err)
		}
		return result
	}

	output = strings.TrimSpace(output)
	result.Alive = true

	switch output {
	case "f":
		result.Role = RolePrimary
	case "t":
		result.Role = RoleReplica
	default:
		if verbose {
			fmt.Printf("    unexpected pg_is_in_recovery() result: %q\n", output)
		}
	}

	return result
}

// roleFromDataDir checks data directory files to determine role
func roleFromDataDir(dbsu, dataDir string, verbose bool) *RoleResult {
	result := &RoleResult{
		Role:   RoleUnknown,
		Alive:  false,
		Source: "pgdata",
	}

	// Check if data directory exists and is initialized as dbsu
	exists, initialized := CheckDataDirAsDBSU(dbsu, dataDir)
	if !exists || !initialized {
		if verbose {
			fmt.Printf("    data directory %s not found or not initialized\n", dataDir)
		}
		return result
	}

	// List directory contents as dbsu
	output, err := utils.DBSUCommandOutput(dbsu, []string{"ls", "-1", dataDir})
	if err != nil {
		if verbose {
			fmt.Printf("    cannot list data directory %s: %v\n", dataDir, err)
		}
		return result
	}

	files := strings.Split(strings.TrimSpace(output), "\n")
	if len(files) < utils.MinDataDirFileCount {
		if verbose {
			fmt.Printf("    data directory has only %d files (expected >= %d), likely not initialized\n",
				len(files), utils.MinDataDirFileCount)
		}
		return result
	}

	// Default to primary if it's a valid data directory
	result.Role = RolePrimary

	for _, name := range files {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// Check for postmaster.pid (indicates potentially running)
		if name == "postmaster.pid" {
			result.Alive = true // May be stale, but indicates possible running state
		}

		// PostgreSQL 12+ uses standby.signal and recovery.signal
		if name == "standby.signal" || name == "recovery.signal" {
			result.Role = RoleReplica
			if verbose {
				fmt.Printf("    found %s -> replica\n", name)
			}
			return result
		}

		// PostgreSQL < 12 uses recovery.conf
		if name == "recovery.conf" {
			// Check if it contains primary_conninfo or restore_command
			if checkRecoveryConfAsDBSU(dbsu, filepath.Join(dataDir, name), verbose) {
				result.Role = RoleReplica
				return result
			}
		}
	}

	// Also check postgresql.auto.conf for primary_conninfo (PG12+)
	autoConfPath := filepath.Join(dataDir, "postgresql.auto.conf")
	if fileExistsAsDBSU(dbsu, autoConfPath) {
		if checkAutoConfForReplicationAsDBSU(dbsu, autoConfPath, verbose) {
			// Has replication config, but need standby.signal to be replica
			// If we didn't find standby.signal, this is a primary with replication slot
			if verbose {
				fmt.Printf("    found primary_conninfo in auto.conf but no standby.signal -> primary\n")
			}
		}
	}

	return result
}

// fileExistsAsDBSU checks if a file exists as the database superuser
func fileExistsAsDBSU(dbsu, path string) bool {
	_, err := utils.DBSUCommandOutput(dbsu, []string{"test", "-f", path})
	return err == nil
}

// checkRecoveryConfAsDBSU checks if recovery.conf indicates replica mode (as dbsu)
func checkRecoveryConfAsDBSU(dbsu, path string, verbose bool) bool {
	output, err := utils.DBSUCommandOutput(dbsu, []string{"cat", path})
	if err != nil {
		return false
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Check for replica indicators
		if strings.HasPrefix(line, "primary_conninfo") ||
			strings.HasPrefix(line, "restore_command") ||
			strings.HasPrefix(line, "standby_mode") {
			if verbose {
				fmt.Printf("    found in recovery.conf: %s\n", line)
			}
			return true
		}
	}
	return false
}

// checkAutoConfForReplicationAsDBSU checks if postgresql.auto.conf has replication settings (as dbsu)
func checkAutoConfForReplicationAsDBSU(dbsu, path string, verbose bool) bool {
	output, err := utils.DBSUCommandOutput(dbsu, []string{"cat", path})
	if err != nil {
		return false
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "primary_conninfo") {
			if verbose {
				fmt.Printf("    found primary_conninfo in postgresql.auto.conf\n")
			}
			return true
		}
	}
	return false
}

// PrintRole outputs the role detection result
func PrintRole(cfg *Config, opts *RoleOptions) error {
	result, err := DetectRole(cfg, opts)
	if err != nil {
		return err
	}

	verbose := opts != nil && opts.Verbose

	if verbose {
		fmt.Printf("\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
		fmt.Printf("%s PostgreSQL Instance Role%s\n", utils.ColorBold, utils.ColorReset)
		fmt.Printf("%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)

		// Status
		aliveStatus := fmt.Sprintf("%sdown%s", utils.ColorRed, utils.ColorReset)
		if result.Alive {
			aliveStatus = fmt.Sprintf("%srunning%s", utils.ColorGreen, utils.ColorReset)
		}
		fmt.Printf("  Status:  %s\n", aliveStatus)

		// Role with color
		var roleStatus string
		switch result.Role {
		case RolePrimary:
			roleStatus = fmt.Sprintf("%sprimary%s", utils.ColorGreen, utils.ColorReset)
		case RoleReplica:
			roleStatus = fmt.Sprintf("%sreplica%s", utils.ColorYellow, utils.ColorReset)
		default:
			roleStatus = fmt.Sprintf("%sunknown%s", utils.ColorRed, utils.ColorReset)
		}
		fmt.Printf("  Role:    %s\n", roleStatus)
		fmt.Printf("  Source:  %s\n", result.Source)
		fmt.Printf("%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	} else {
		// Simple output for scripting
		fmt.Println(result.Role)
	}

	return nil
}
