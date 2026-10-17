/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pg status structured output result and DTO.
*/
package postgres

import (
	"errors"
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
	Running       bool              `json:"running" yaml:"running"`
	PID           int               `json:"pid,omitempty" yaml:"pid,omitempty"`
	Version       int               `json:"version,omitempty" yaml:"version,omitempty"`
	DataDir       string            `json:"data_dir" yaml:"data_dir"`
	Port          int               `json:"port,omitempty" yaml:"port,omitempty"`
	UptimeSeconds int64             `json:"uptime_seconds,omitempty" yaml:"uptime_seconds,omitempty"`
	ControlData   map[string]string `json:"control_data,omitempty" yaml:"control_data,omitempty"`
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
			"Unable to determine PostgreSQL running state").
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
	attachPgControlData(statusData, cfg, dbsu, dataDir)

	// If not running, return state error with partial data
	if !running {
		return output.Fail(output.CodePgStatusNotRunning,
			"PostgreSQL is not running").
			WithData(statusData)
	}

	// PostgreSQL is running - read port and uptime from postmaster.pid
	info := readPostmasterPidInfo(dbsu, dataDir, pidContent)
	statusData.Port = info.Port
	if !info.StartTime.IsZero() {
		statusData.UptimeSeconds = int64(time.Since(info.StartTime).Seconds())
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
// Returns zero values if postmaster.pid cannot be read or parsed.
func readPostmasterPidInfo(dbsu, dataDir, pidContent string) PostmasterPidInfo {
	content := pidContent
	if strings.TrimSpace(content) == "" {
		pidFile := filepath.Join(dataDir, "postmaster.pid")
		// Read postmaster.pid content using DBSU privilege escalation
		fileContent, err := utils.ReadFileAsDBSU(pidFile, dbsu)
		if err != nil {
			logrus.Debugf("cannot read postmaster.pid: %v", err)
			return PostmasterPidInfo{}
		}
		content = fileContent
	}
	info, err := ParsePostmasterPidInfo(content)
	if err != nil {
		logrus.Debugf("cannot parse postmaster.pid: %v", err)
		return PostmasterPidInfo{}
	}
	return info
}

// PostmasterPidInfo captures the postmaster.pid fields used for local
// instance binding.
type PostmasterPidInfo struct {
	Port      int
	StartTime time.Time
	SocketDir string
}

// ReadPostmasterPidInfoAsDBSU reads postmaster.pid from a data directory using
// DBSU privileges.
func ReadPostmasterPidInfoAsDBSU(dbsu, dataDir string) (PostmasterPidInfo, error) {
	running, pid, content, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		return PostmasterPidInfo{}, err
	}
	if !running || pid <= 0 {
		return PostmasterPidInfo{}, fmt.Errorf("target PostgreSQL is not running")
	}
	if postmasterPIDDataDir(content) == "" || !postmasterPIDMatchesDataDirAsDBSU(dbsu, content, dataDir) {
		return PostmasterPidInfo{}, fmt.Errorf("postmaster.pid data directory does not match requested directory")
	}
	return ParsePostmasterPidInfo(content)
}

// ParsePostmasterPidInfo parses postmaster.pid connection fields. Port is
// required; socket directory and start time are optional evidence.
func ParsePostmasterPidInfo(content string) (PostmasterPidInfo, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 4 {
		return PostmasterPidInfo{}, fmt.Errorf("postmaster.pid has fewer than 4 lines")
	}
	socketDir := ""
	if len(lines) >= 5 {
		socketDir = strings.TrimSpace(lines[4])
	}

	portStr := strings.TrimSpace(lines[3])
	port, err := strconv.Atoi(portStr)
	if err != nil || !validPort(port) {
		if err == nil {
			err = fmt.Errorf("invalid port %d", port)
		}
		return PostmasterPidInfo{}, fmt.Errorf("parse postmaster.pid port: %w", err)
	}

	startTime, err := parsePostmasterStartTime(strings.TrimSpace(lines[2]))
	if err != nil {
		startTime = time.Time{}
	}

	return PostmasterPidInfo{
		Port:      port,
		StartTime: startTime,
		SocketDir: socketDir,
	}, nil
}

func parsePostmasterStartTime(startTimeStr string) (time.Time, error) {
	// Try parsing as Unix epoch first
	if epoch, err := strconv.ParseInt(startTimeStr, 10, 64); err == nil {
		return time.Unix(epoch, 0), nil
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
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("parse postmaster.pid start time: %s", startTimeStr)
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
		if errors.Is(readErr, os.ErrNotExist) || isNotFoundOutput(content) || isNotFoundOutput(readErr.Error()) {
			return false, 0, "", nil
		}
		return false, 0, "", fmt.Errorf("cannot read postmaster.pid: %w", readErr)
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return false, 0, content, fmt.Errorf("postmaster.pid is empty")
	}

	parsedPID, parseErr := strconv.Atoi(strings.TrimSpace(lines[0]))
	if parseErr != nil || parsedPID <= 0 {
		return false, 0, content, fmt.Errorf("postmaster.pid has no valid positive PID")
	}
	if postmasterPIDDataDir(content) == "" {
		return false, parsedPID, content, fmt.Errorf("postmaster.pid has no data directory; running state is unknown")
	}
	if !postmasterPIDMatchesDataDirAsDBSU(dbsu, content, dataDir) {
		logrus.Debugf("postmaster.pid belongs to another data directory")
		return false, parsedPID, content, nil
	}

	running, probeErr := checkProcessRunningAsDBSUWithError(dbsu, parsedPID)
	if !running && probeErr == nil {
		logrus.Debugf("process %d not running (stale pid file)", parsedPID)
		return false, parsedPID, content, nil
	}
	// EPERM alone cannot prove absence. Independent process identity can prove
	// that a PID was reused, even when the new owner cannot be signalled.
	command, err := postgresDBSUCommandOutput(dbsu, []string{"ps", "-p", strconv.Itoa(parsedPID), "-o", "comm="})
	if err != nil {
		return false, parsedPID, content, fmt.Errorf("cannot identify postmaster process %d: %w", parsedPID, err)
	}
	if strings.TrimSpace(command) == "" {
		return false, parsedPID, content, fmt.Errorf("cannot identify postmaster process %d", parsedPID)
	}
	name := filepath.Base(strings.TrimSpace(command))
	if name != "postgres" && name != "postmaster" {
		return false, parsedPID, content, nil
	}
	if probeErr != nil {
		return false, parsedPID, content, probeErr
	}
	return true, parsedPID, content, nil
}

func checkProcessRunningAsDBSUWithError(dbsu string, pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid postmaster PID")
	}
	if utils.IsDBSU(dbsu) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("cannot probe postmaster PID %d: %w", pid, err)
		}
		return true, nil
	}
	// Keep stderr in the error so EPERM and privilege failures cannot become
	// evidence that the process has exited. Only ESRCH proves absence.
	out, err := postgresDBSUCommandOutput(dbsu, []string{"kill", "-0", strconv.Itoa(pid)})
	if err != nil {
		if strings.Contains(strings.ToLower(out+err.Error()), "no such process") {
			return false, nil
		}
		return false, fmt.Errorf("cannot probe postmaster PID %d: %w", pid, err)
	}
	return true, nil
}

func testPathAsDBSU(dbsu, flag, path string) (bool, error) {
	output, err := utils.DBSUCommandOutput(dbsu, []string{"test", flag, path})
	if err == nil {
		return true, nil
	}

	output = strings.TrimSpace(output)
	if isPermissionOutput(output) || isPermissionErr(err) {
		return false, fmt.Errorf("permission denied: %s", output)
	}
	if isNotFoundOutput(output) {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
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
