/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pg status structured output result and DTO.
*/
package postgres

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// PgStatusResultData contains PostgreSQL instance status information.
// This struct is used as the Data field in output.Result for structured output.
type PgStatusResultData struct {
	Running       bool   `json:"running" yaml:"running"`
	PID           int    `json:"pid,omitempty" yaml:"pid,omitempty"`
	Version       int    `json:"version,omitempty" yaml:"version,omitempty"`
	DataDir       string `json:"data_dir" yaml:"data_dir"`
	Port          int    `json:"port,omitempty" yaml:"port,omitempty"`
	UptimeSeconds int64  `json:"uptime_seconds,omitempty" yaml:"uptime_seconds,omitempty"`
}

// StatusResult creates a structured result for pg status command.
// It collects PostgreSQL status information and returns it in a Result structure.
// Returns nil-safe Result on all paths.
func StatusResult(cfg *Config) *output.Result {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Initialize result data with data_dir always set
	statusData := &PgStatusResultData{
		Running: false,
		DataDir: dataDir,
	}

	// Check if data directory exists and is initialized
	exists, initialized, err := checkDataDirStateAsDBSU(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgStatusPermissionDenied,
			"Permission denied checking PostgreSQL data directory").
			WithData(statusData).
			WithDetail(err.Error())
	}
	if !exists {
		return output.Fail(output.CodePgStatusDataDirNotFound,
			"PostgreSQL data directory not found").
			WithData(statusData).
			WithDetail("data_dir=" + dataDir)
	}
	if !initialized {
		return output.Fail(output.CodePgStatusNotInitialized,
			"PostgreSQL data directory not initialized").
			WithData(statusData).
			WithDetail("data_dir=" + dataDir + " (no PG_VERSION file)")
	}

	// Check if PostgreSQL is running
	running, pid, pidContent, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return output.Fail(output.CodePgStatusPermissionDenied,
			"Permission denied checking PostgreSQL status").
			WithData(statusData).
			WithDetail(err.Error())
	}
	statusData.Running = running
	statusData.PID = pid

	// Get PostgreSQL version from PG_VERSION file
	if ver, err := ReadPgVersionAsDBSU(dbsu, dataDir); err == nil {
		statusData.Version = ver
	} else {
		logrus.Debugf("failed to read PG_VERSION: %v", err)
	}

	// If not running, return state error with partial data
	if !running {
		return output.Fail(output.CodePgStatusNotRunning,
			"PostgreSQL is not running").
			WithData(statusData)
	}

	// PostgreSQL is running - read port and uptime from postmaster.pid
	port, startTime := readPostmasterPidInfo(dbsu, dataDir, pidContent)
	statusData.Port = port
	if !startTime.IsZero() {
		statusData.UptimeSeconds = int64(time.Since(startTime).Seconds())
	}

	return output.OK("PostgreSQL is running", statusData)
}

// readPostmasterPidInfo reads port and start time from postmaster.pid file.
// postmaster.pid format (by line):
//
//	1: PID
//	2: Data directory
//	3: Start timestamp (Unix epoch or timestamp string)
//	4: Port
//	5: Unix socket directory
//	6: Listen addresses
//	7: Shared memory key
//
// Returns port (0 if cannot read) and start time (zero time if cannot parse).
func readPostmasterPidInfo(dbsu, dataDir, pidContent string) (port int, startTime time.Time) {
	content := pidContent
	if strings.TrimSpace(content) == "" {
		pidFile := filepath.Join(dataDir, "postmaster.pid")
		// Read postmaster.pid content using DBSU privilege escalation
		fileContent, err := utils.ReadFileAsDBSU(pidFile, dbsu)
		if err != nil {
			logrus.Debugf("cannot read postmaster.pid: %v", err)
			return 0, time.Time{}
		}
		content = fileContent
	}
	return parsePostmasterPidInfo(content)
}

// parsePostmasterPidInfo parses port and start time from postmaster.pid content.
// Returns port (0 if cannot parse) and start time (zero time if cannot parse).
func parsePostmasterPidInfo(content string) (port int, startTime time.Time) {
	lines := strings.Split(content, "\n")
	if len(lines) < 4 {
		logrus.Debugf("postmaster.pid has fewer than 4 lines")
		return 0, time.Time{}
	}

	// Parse port (line 4, 0-indexed as 3)
	portStr := strings.TrimSpace(lines[3])
	port, err := strconv.Atoi(portStr)
	if err != nil {
		logrus.Debugf("cannot parse port from postmaster.pid: %v", err)
		port = 0
	}

	// Parse start time (line 3, 0-indexed as 2)
	// Can be Unix epoch (seconds) or timestamp string
	startTimeStr := strings.TrimSpace(lines[2])

	// Try parsing as Unix epoch first
	if epoch, err := strconv.ParseInt(startTimeStr, 10, 64); err == nil {
		startTime = time.Unix(epoch, 0)
		return port, startTime
	}

	// Try parsing as timestamp string (various formats PostgreSQL might use)
	// Common format: "2025-01-01 12:00:00 UTC"
	timeFormats := []string{
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.UnixDate,
	}
	for _, format := range timeFormats {
		if t, err := time.Parse(format, startTimeStr); err == nil {
			startTime = t
			return port, startTime
		}
	}

	logrus.Debugf("cannot parse start time from postmaster.pid: %s", startTimeStr)
	return port, time.Time{}
}

// checkDataDirStateAsDBSU checks data directory existence and initialization with permission awareness.
func checkDataDirStateAsDBSU(dbsu, dataDir string) (exists, initialized bool, err error) {
	exists, err = testPathAsDBSU(dbsu, "-d", dataDir)
	if err != nil || !exists {
		return exists, false, err
	}
	initialized, err = testPathAsDBSU(dbsu, "-f", filepath.Join(dataDir, "PG_VERSION"))
	return exists, initialized, err
}

// checkPostgresRunningAsDBSUWithError checks running status and preserves permission errors.
func checkPostgresRunningAsDBSUWithError(dbsu, dataDir string) (running bool, pid int, pidContent string, err error) {
	pidFile := filepath.Join(dataDir, "postmaster.pid")
	content, readErr := utils.ReadFileAsDBSU(pidFile, dbsu)
	if readErr != nil {
		if isPermissionOutput(content) || isPermissionErr(readErr) {
			return false, 0, "", fmt.Errorf("permission denied reading %s: %s", pidFile, strings.TrimSpace(content))
		}
		if isNotFoundOutput(content) {
			return false, 0, "", nil
		}
		// Treat other read errors as non-running but log for debugging
		logrus.Debugf("cannot read postmaster.pid: %v", readErr)
		return false, 0, "", nil
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		logrus.Debugf("postmaster.pid is empty")
		return false, 0, content, nil
	}

	parsedPID, parseErr := strconv.Atoi(strings.TrimSpace(lines[0]))
	if parseErr != nil {
		logrus.Debugf("cannot parse PID from postmaster.pid: %v", parseErr)
		return false, 0, content, nil
	}

	running, err = checkProcessRunningAsDBSUWithError(dbsu, parsedPID)
	if err != nil {
		return false, parsedPID, content, err
	}
	if !running {
		logrus.Debugf("process %d not running (stale pid file)", parsedPID)
		return false, parsedPID, content, nil
	}
	return true, parsedPID, content, nil
}

func checkProcessRunningAsDBSUWithError(dbsu string, pid int) (bool, error) {
	// If current user is DBSU, use direct signal check
	if utils.IsDBSU(dbsu) {
		process, err := os.FindProcess(pid)
		if err != nil {
			return false, nil
		}
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return false, nil
		}
		return true, nil
	}

	// Use kill -0 via DBSU privilege escalation
	output, err := utils.DBSUCommandOutput(dbsu, []string{"kill", "-0", strconv.Itoa(pid)})
	if err != nil {
		if isPermissionOutput(output) || isPermissionErr(err) {
			return false, fmt.Errorf("permission denied running kill -0: %s", strings.TrimSpace(output))
		}
		return false, nil
	}
	return true, nil
}

func testPathAsDBSU(dbsu, flag, path string) (bool, error) {
	cmd := buildTestCmd(dbsu, flag, path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	output := strings.TrimSpace(out.String())
	if isPermissionOutput(output) || isPermissionErr(err) {
		return false, fmt.Errorf("permission denied: %s", output)
	}
	if isNotFoundOutput(output) {
		return false, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
	}
	return false, fmt.Errorf("test %s %s failed: %s", flag, path, output)
}

func isPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "not permitted") ||
		strings.Contains(msg, "not allowed") ||
		strings.Contains(msg, "sudo")
}

func isPermissionOutput(output string) bool {
	msg := strings.ToLower(output)
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "not permitted") ||
		strings.Contains(msg, "not allowed") ||
		strings.Contains(msg, "sudo:") ||
		strings.Contains(msg, "a password is required") ||
		strings.Contains(msg, "no tty present") ||
		strings.Contains(msg, "a terminal is required")
}

func isNotFoundOutput(output string) bool {
	msg := strings.ToLower(output)
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "not found")
}
