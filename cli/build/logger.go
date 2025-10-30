package build

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"strings"
)

// BuildResult represents the result of a build operation
type BuildResult struct {
	Success  bool
	Output   string
	LogPath  string
	Artifact string
	Size     int64
	Marker   string // Unique marker for log searching
}

// BuildLogger handles logging for build operations
type BuildLogger struct {
	logFile *os.File
	logPath string
}

// NewBuildLogger creates a new build logger
func NewBuildLogger(logName string, append bool) (*BuildLogger, error) {
	// Ensure log directory exists
	logDir := filepath.Join(config.HomeDir, "ext", "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	// Open log file (append or create)
	logPath := filepath.Join(logDir, logName)
	var logFile *os.File
	var err error

	if append {
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		logFile, err = os.Create(logPath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &BuildLogger{
		logFile: logFile,
		logPath: logPath,
	}, nil
}

// WriteMetadata writes build metadata to log file
func (bl *BuildLogger) WriteMetadata(lines ...string) {
	for _, line := range lines {
		fmt.Fprintln(bl.logFile, line)
	}
}

// WriteSeparator writes a separator to log file
func (bl *BuildLogger) WriteSeparator() {
	// Write 5 empty lines as separator
	for i := 0; i < 5; i++ {
		fmt.Fprintln(bl.logFile, "")
	}
}

// Close closes the log file
func (bl *BuildLogger) Close() {
	if bl.logFile != nil {
		bl.logFile.Close()
	}
}

// RunBuildCommand executes a build command with single-line output display
func RunBuildCommand(cmd *exec.Cmd, logName string, append bool, metadata []string, pkgName string, pgVer int) (*BuildResult, error) {
	// Create build logger
	logger, err := NewBuildLogger(logName, append)
	if err != nil {
		return nil, err
	}
	defer logger.Close()

	// Write metadata to log
	if len(metadata) > 0 {
		for _, line := range metadata {
			logger.WriteMetadata(line)
		}
		logger.WriteMetadata(strings.Repeat("=", 58))
	}

	// Create pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Process output
	done := make(chan error, 2)
	var lastLineShown bool

	// Process stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			// Write to log file
			fmt.Fprintln(logger.logFile, line)

			// Display single line scrolling
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				displayScrollingLine(trimmed, pgVer)
				lastLineShown = true
			}
		}
		done <- scanner.Err()
	}()

	// Process stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			// Write to log file
			fmt.Fprintln(logger.logFile, line)

			// Display single line scrolling
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				displayScrollingLine(trimmed, pgVer)
				lastLineShown = true
			}
		}
		done <- scanner.Err()
	}()

	// Wait for output processing
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil && err != io.EOF {
			// Clear the scrolling line
			if lastLineShown {
				fmt.Print("\r\033[K")
			}
			// Write end marker to log
			logger.WriteMetadata("", "#[END]"+strings.Repeat("=", 52), "", "", "", "")
			return &BuildResult{
				Success: false,
				LogPath: logger.logPath,
			}, err
		}
	}

	// Wait for command completion
	err = cmd.Wait()

	// Clear the scrolling line
	if lastLineShown {
		fmt.Print("\r\033[K")
	}

	// Write end marker to log
	logger.WriteMetadata("", "#[END]"+strings.Repeat("=", 52), "", "", "", "")

	result := &BuildResult{
		Success: err == nil,
		LogPath: logger.logPath,
	}

	return result, err
}

// displayScrollingLine displays a single line with truncation
func displayScrollingLine(line string, pgVer int) {
	// Prepare prefix
	prefix := ""
	if pgVer > 0 {
		prefix = fmt.Sprintf("[PG%d]  ", pgVer)
	}

	// Get terminal width or default
	maxLen := 70 - len(prefix)
	if len(line) > maxLen {
		line = "..." + line[len(line)-(maxLen-3):]
	}

	// Clear line and display
	fmt.Printf("\r\033[K%s%s", prefix, line)
}

