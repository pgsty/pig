package output

import (
	"fmt"
	"io"
	"os"
	"pig/internal/config"
)

// Print outputs the Result to stdout based on the global config.OutputFormat.
// For text format, it uses "text-color" which automatically handles TTY detection.
// Returns an error if the Result is nil or rendering fails.
func Print(r *Result) error {
	return PrintTo(os.Stdout, r)
}

// PrintTo outputs the Result to the specified writer based on the global config.OutputFormat.
// For text format, it uses "text-color" which automatically handles TTY detection.
// Returns an error if the Result is nil or rendering fails.
//
// Design decision: When the user specifies "-o text" (or uses the default), we route
// through "text-color" rather than plain "text". The ColorText() method internally
// checks for TTY/NO_COLOR/TERM=dumb and falls back to plain Text() when appropriate.
// This ensures users always get the best output for their terminal without needing
// to explicitly choose between "text" and "text-color".
func PrintTo(w io.Writer, r *Result) error {
	if r == nil {
		return fmt.Errorf("cannot print nil Result")
	}

	// Route "text" format through "text-color" for automatic TTY detection.
	// ColorText() will fall back to plain Text() if color is disabled.
	format := config.OutputFormat
	if format == config.OUTPUT_TEXT {
		format = "text-color"
	}

	data, err := r.Render(format)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, string(data))
	return nil
}

// PrintData is a convenience function that creates a successful Result with the given
// data and message, then prints it according to the global output format.
func PrintData(data interface{}, message string) error {
	return Print(OK(message, data))
}

// PrintError is a convenience function that creates a failed Result with the given
// code and message, then prints it according to the global output format.
func PrintError(code int, message string) error {
	return Print(Fail(code, message))
}
