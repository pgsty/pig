package output

import (
	"fmt"
	"os"
	"strings"
)

// Texter is an interface for data objects that can render themselves as text.
// When Result.Data implements this interface, formatText will append the data's
// text representation after the message line.
type Texter interface {
	Text() string
}

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

// Text symbols for output
const (
	symbolSuccess = "✓"
	symbolFailure = "✗"
)

// Text returns a human-readable text representation of the Result.
// It formats the output with status indicator (✓/✗), message, and optional detail.
// Returns an empty string if the receiver is nil.
func (r *Result) Text() string {
	if r == nil {
		return ""
	}
	return r.formatText("", "")
}

// ColorText returns a colored text representation of the Result for terminal output.
// Success messages use green, errors use red, warnings (category 6-7) use yellow.
// Returns an empty string if the receiver is nil.
// Respects NO_COLOR environment variable, TERM=dumb, and non-TTY output.
func (r *Result) ColorText() string {
	if r == nil {
		return ""
	}

	// Check if color is disabled
	if !isColorEnabled() {
		return r.Text()
	}

	// Determine color based on status and category
	color := r.getColor()
	return r.formatText(color, colorReset)
}

// getColor returns the appropriate ANSI color code for this Result.
func (r *Result) getColor() string {
	// Check for warning category (state=6, config=7)
	category := (r.Code % 10000) / 100
	if category == 6 || category == 7 {
		return colorYellow
	}
	if r.Success {
		return colorGreen
	}
	return colorRed
}

// formatText formats the Result as text with optional color codes.
// If colorStart and colorEnd are empty, no color codes are added.
func (r *Result) formatText(colorStart, colorEnd string) string {
	var sb strings.Builder

	// Status indicator and message
	if r.Success {
		sb.WriteString(colorStart)
		sb.WriteString(symbolSuccess)
		sb.WriteString(colorEnd)
		sb.WriteString(" ")
	} else {
		sb.WriteString(colorStart)
		sb.WriteString(symbolFailure)
		sb.WriteString(colorEnd)
		sb.WriteString(" ")
	}
	sb.WriteString(r.Message)

	// Optional data text - if Data implements Texter, append its text output
	if r.Data != nil {
		if texter, ok := r.Data.(Texter); ok {
			if dataText := texter.Text(); dataText != "" {
				sb.WriteString("\n")
				sb.WriteString(dataText)
			}
		}
	}

	// Optional detail
	if r.Detail != "" {
		sb.WriteString("\n  ")
		sb.WriteString(r.Detail)
	}

	// Optional code for failures
	if !r.Success && r.Code != 0 {
		sb.WriteString(fmt.Sprintf("\n  Code: %d", r.Code))
	}

	return sb.String()
}

// isColorEnabled checks if terminal color output should be enabled.
// Returns false if NO_COLOR is set, TERM=dumb, or stdout is not a TTY.
func isColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// Check if stdout is a TTY
	if !isTerminal(os.Stdout) {
		return false
	}
	return true
}

// isTerminal checks if the given file is a terminal.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// RenderTable formats headers and rows into a simple text table.
// Returns an empty string if headers are empty.
func RenderTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Calculate column widths using proper Unicode width
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = stringWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && stringWidth(cell) > widths[i] {
				widths[i] = stringWidth(cell)
			}
		}
	}

	var sb strings.Builder

	// Header row
	for i, h := range headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(padRight(h, widths[i]))
	}
	sb.WriteString("\n")

	// Separator line using Unicode box-drawing character
	totalWidth := 0
	for i, w := range widths {
		if i > 0 {
			totalWidth += 2 // spacing
		}
		totalWidth += w
	}
	sb.WriteString(strings.Repeat("─", totalWidth))
	sb.WriteString("\n")

	// Data rows
	for _, row := range rows {
		for i := 0; i < len(headers); i++ {
			if i > 0 {
				sb.WriteString("  ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			sb.WriteString(padRight(cell, widths[i]))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// padRight pads a string to the specified width with spaces on the right.
// Uses display width for proper CJK character support.
func padRight(s string, width int) string {
	displayWidth := stringWidth(s)
	if displayWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-displayWidth)
}

// stringWidth returns the display width of a string.
// ASCII characters count as 1, CJK wide characters count as 2.
func stringWidth(s string) int {
	width := 0
	for _, r := range s {
		width += runeWidth(r)
	}
	return width
}

// runeWidth returns the display width of a rune.
// CJK wide characters return 2, others return 1.
//
// Limitations: This is a simplified implementation that covers common CJK characters.
// It does not handle all Unicode edge cases including:
// - Emoji (U+1F300-U+1F9FF) which are typically width 2
// - Zero-width characters (U+200B-U+200F) which should be width 0
// - Combining characters and grapheme clusters
// For production use with complex Unicode, consider using go-runewidth library.
func runeWidth(r rune) int {
	// CJK Unified Ideographs and common wide character ranges
	if r >= 0x1100 && r <= 0x115F || // Hangul Jamo
		r >= 0x2E80 && r <= 0x9FFF || // CJK Radicals, Ideographs
		r >= 0xAC00 && r <= 0xD7A3 || // Hangul Syllables
		r >= 0xF900 && r <= 0xFAFF || // CJK Compatibility Ideographs
		r >= 0xFE10 && r <= 0xFE1F || // Vertical Forms
		r >= 0xFE30 && r <= 0xFE6F || // CJK Compatibility Forms
		r >= 0xFF00 && r <= 0xFF60 || // Fullwidth Forms
		r >= 0xFFE0 && r <= 0xFFE6 || // Fullwidth Forms
		r >= 0x20000 && r <= 0x2FFFF { // CJK Extension
		return 2
	}
	return 1
}
