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

// RenderPlan outputs the Plan to stdout based on the global output format.
// For text format, it uses plain "text" output.
func RenderPlan(plan *Plan) error {
	return PrintPlanTo(os.Stdout, plan)
}

// PrintPlanTo outputs the Plan to the specified writer based on the global output format.
// Returns an error if the Plan is nil or rendering fails.
func PrintPlanTo(w io.Writer, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("cannot print nil Plan")
	}

	format := config.OutputFormat

	data, err := plan.Render(format)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, string(data))
	return nil
}
