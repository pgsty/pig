package output

import (
	"fmt"
	"io"
	"os"
	"pig/internal/config"
)

// Print outputs the Result to stdout based on the global config.OutputFormat.
// For text format, it uses plain "text" output (no ANSI colors).
// Returns an error if the Result is nil or rendering fails.
func Print(r *Result) error {
	return PrintTo(os.Stdout, r)
}

// PrintTo outputs the Result to the specified writer based on the global config.OutputFormat.
// For text format, it uses plain "text" output (no ANSI colors).
// Returns an error if the Result is nil or rendering fails.
//
// Design decision: "text" output is always plain text. Users can opt into
// colored output explicitly via "text-color" when needed.
func PrintTo(w io.Writer, r *Result) error {
	if r == nil {
		return fmt.Errorf("cannot print nil Result")
	}

	format := config.OutputFormat

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
