/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Single interactive confirmation primitive for destructive (T2) operations.
*/
package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm is the ONLY interactive confirmation primitive for destructive (T2)
// operations (B38). It prints the warning to stderr and reads one line from
// stdin: "y" / "yes" (case-insensitive) proceeds, anything else aborts.
// EOF (closed stdin, e.g. cron) aborts — non-interactive callers must pass
// --yes. Piped input like `echo y | pig ...` is a valid wrapper pattern.
func Confirm(warning, action string) error {
	fmt.Fprintf(os.Stderr, "\n%sWARNING: %s%s\n", ColorYellow, warning, ColorReset)
	fmt.Fprintf(os.Stderr, "Continue with %s%s%s? [y/N]: ", ColorBold, action, ColorReset)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && len(input) == 0 {
		return fmt.Errorf("%s cancelled: %w", action, err)
	}
	if !ConfirmationAccepted(input) {
		fmt.Fprintf(os.Stderr, "\n%s cancelled.\n", action)
		return fmt.Errorf("%s cancelled by user", action)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// ConfirmationAccepted reports whether one line of user input means yes.
// The grammar is global and identical at every T2 prompt: y / yes, any case.
func ConfirmationAccepted(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	return s == "y" || s == "yes"
}
