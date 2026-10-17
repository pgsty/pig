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
	"strconv"
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

	// Strategy 1: Bind process evidence to the requested postmaster.pid.
	if verbose {
		fmt.Printf("%s[1] Checking PostgreSQL processes...%s\n", utils.ColorCyan, utils.ColorReset)
	}
	running, targetPID, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return nil, fmt.Errorf("cannot determine PostgreSQL running state: %w", err)
	}
	psResult := &RoleResult{Role: RoleUnknown, Source: "ps"}
	if running {
		psResult = roleFromProcesses(dbsu, targetPID, verbose)
		psResult.Alive = true
	}
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
	fileResult.Alive = running
	if verbose {
		fmt.Printf("    File check: alive=%v, role=%s\n", fileResult.Alive, fileResult.Role)
	}

	// Return file result if we got something
	if fileResult.Role != RoleUnknown {
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

// roleFromProcesses checks only the requested postmaster and its descendants.
func roleFromProcesses(dbsu string, targetPID int, verbose bool) *RoleResult {
	result := &RoleResult{
		Role:   RoleUnknown,
		Alive:  false,
		Source: "ps",
	}

	if targetPID <= 0 {
		return result
	}
	cmd := exec.Command("ps", "h", "-u", dbsu, "-o", "pid=,ppid=,command=")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		if verbose {
			fmt.Printf("    ps command failed: %v\n", err)
		}
		return result
	}

	evidence := inspectPostgresProcessTree(out.String(), targetPID)

	if verbose {
		fmt.Printf("    hasMainProcess=%v, hasRecovery=%v, hasWalreceiver=%v\n",
			evidence.alive, evidence.recovery, evidence.walreceiver)
	}

	result.Alive = evidence.alive
	if evidence.alive {
		if evidence.recovery {
			result.Role = RoleReplica
		} else {
			result.Role = RolePrimary
		}
	}

	return result
}

type postgresProcessEvidence struct {
	alive       bool
	recovery    bool
	walreceiver bool
}

type postgresProcess struct {
	pid     int
	ppid    int
	command string
}

func inspectPostgresProcessTree(output string, targetPID int) postgresProcessEvidence {
	processes := make(map[int]postgresProcess)
	for _, raw := range strings.Split(output, "\n") {
		fields := strings.Fields(raw)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil {
			continue
		}
		processes[pid] = postgresProcess{pid: pid, ppid: ppid, command: strings.Join(fields[2:], " ")}
	}
	if _, exists := processes[targetPID]; !exists {
		return postgresProcessEvidence{}
	}

	selected := map[int]struct{}{targetPID: {}}
	for changed := true; changed; {
		changed = false
		for pid, process := range processes {
			if _, exists := selected[pid]; exists {
				continue
			}
			if _, parentSelected := selected[process.ppid]; parentSelected {
				selected[pid] = struct{}{}
				changed = true
			}
		}
	}

	evidence := postgresProcessEvidence{alive: true}
	for pid := range selected {
		command := strings.ToLower(processes[pid].command)
		if strings.Contains(command, "walreceiver") {
			evidence.walreceiver = true
			evidence.recovery = true
		}
		if strings.Contains(command, "recovering") {
			evidence.recovery = true
		}
	}
	return evidence
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

	dataDir := GetPgData(cfg)
	postmaster, err := ReadPostmasterPidInfoAsDBSU(dbsu, dataDir)
	if err != nil {
		if verbose {
			fmt.Printf("    cannot read target postmaster.pid: %v\n", err)
		}
		return result
	}
	cmdArgs := roleSQLArgs(pg.Psql(), postmaster)

	// Capture stdout only so diagnostics cannot corrupt the scalar result.
	output, err := utils.DBSUCommandStdout(dbsu, cmdArgs)
	if err != nil {
		if verbose {
			fmt.Printf("    psql command failed: %v\n", err)
		}
		return result
	}

	connectedDataDir, role, ok := parseRoleSQLResult(output)
	if !ok {
		if verbose {
			fmt.Printf("    unexpected pg_is_in_recovery() result: %q\n", output)
		}
		return result
	}
	if !sameDataDirAsDBSU(dbsu, connectedDataDir, dataDir) {
		if verbose {
			fmt.Printf("    connected data directory %s does not match target %s\n", connectedDataDir, dataDir)
		}
		return result
	}
	result.Alive = true
	result.Role = role
	return result
}

func roleSQLArgs(psql string, info PostmasterPidInfo) []string {
	args := []string{psql, "-AXtqw", "-F", "\t", "-p", strconv.Itoa(info.Port)}
	if info.SocketDir != "" {
		args = append(args, "-h", info.SocketDir)
	}
	return append(args, "-d", "postgres", "-c", "SELECT current_setting('data_directory'), pg_is_in_recovery()")
}

func parseRoleSQLResult(output string) (string, Role, bool) {
	fields := strings.Split(strings.TrimSpace(output), "\t")
	if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" {
		return "", RoleUnknown, false
	}
	switch strings.TrimSpace(fields[1]) {
	case "f":
		return strings.TrimSpace(fields[0]), RolePrimary, true
	case "t":
		return strings.TrimSpace(fields[0]), RoleReplica, true
	default:
		return "", RoleUnknown, false
	}
}

func sameDataDirAsDBSU(dbsu, left, right string) bool {
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	leftResolved, leftErr := resolvePathAsDBSU(dbsu, left)
	rightResolved, rightErr := resolvePathAsDBSU(dbsu, right)
	return leftErr == nil && rightErr == nil && leftResolved != "" && rightResolved != "" &&
		filepath.Clean(leftResolved) == filepath.Clean(rightResolved)
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
	output, err := utils.DBSUCommandStdout(dbsu, []string{"ls", "-1", dataDir})
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

	result.Role = roleFromDataFiles(dbsu, dataDir, files, verbose)
	if result.Role == RoleReplica {
		return result
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

func roleFromDataFiles(dbsu, dataDir string, files []string, verbose bool) Role {
	for _, raw := range files {
		name := strings.TrimSpace(raw)
		switch name {
		case "standby.signal", "recovery.signal":
			if verbose {
				fmt.Printf("    found %s -> replica\n", name)
			}
			return RoleReplica
		case "recovery.conf":
			if checkRecoveryConfAsDBSU(dbsu, filepath.Join(dataDir, name), verbose) {
				return RoleReplica
			}
		}
	}
	return RolePrimary
}

// fileExistsAsDBSU checks if a file exists as the database superuser
func fileExistsAsDBSU(dbsu, path string) bool {
	_, err := utils.DBSUCommandStdout(dbsu, []string{"test", "-f", path})
	return err == nil
}

// checkRecoveryConfAsDBSU checks if recovery.conf indicates replica mode (as dbsu)
func checkRecoveryConfAsDBSU(dbsu, path string, verbose bool) bool {
	output, err := utils.DBSUCommandStdout(dbsu, []string{"cat", path})
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
	output, err := utils.DBSUCommandStdout(dbsu, []string{"cat", path})
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
