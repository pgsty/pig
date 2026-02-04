package utils

import (
	"fmt"
	"os"
	"strings"
)

// ANSI color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorBlue   = "\033[34m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorRed    = "\033[31m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

// PrintHint prints a command hint to stderr in blue.
func PrintHint(cmdArgs []string) {
	fmt.Fprintf(os.Stderr, "%s$ %s%s\n", ColorBlue, strings.Join(cmdArgs, " "), ColorReset)
}

// PrintWarn prints a warning message to stderr in yellow.
func PrintWarn(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%sWARNING: %s%s\n", ColorYellow, fmt.Sprintf(format, args...), ColorReset)
}

// PrintError prints an error message to stderr in red.
func PrintError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%sERROR: %s%s\n", ColorRed, fmt.Sprintf(format, args...), ColorReset)
}

// PrintInfo prints an info message to stderr in cyan.
func PrintInfo(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s%s%s\n", ColorCyan, fmt.Sprintf(format, args...), ColorReset)
}

// PrintSuccess prints a success message to stderr in green.
func PrintSuccess(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s%s%s\n", ColorGreen, fmt.Sprintf(format, args...), ColorReset)
}

// PrintSection prints a section header to stderr.
func PrintSection(title string) {
	fmt.Fprintf(os.Stderr, "\n%s=== %s ===%s\n", ColorCyan, title, ColorReset)
}
