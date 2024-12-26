package utils

import (
	"fmt"
	"os"
	"os/exec"
	"pig/internal/config"
	"strings"
)

var (

	// TrySudo is a flag to try to run a command with sudo
	TrySudo = false
)

// ShellCommand runs a command without sudo
func ShellCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command to run")
	}
	if TrySudo {
		return SudoCommand(args)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SudoCommand runs a command with sudo if the current user is not root
func SudoCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command to run")
	}
	if config.CurrentUser != "root" {
		// insert sudo as first cmd arg
		args = append([]string{"sudo"}, args...)
	}

	// now split command and args again
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PadKV pads a key-value pair with spaces to the right
func PadKV(key string, value string) {
	fmt.Printf("%-16s : %s\n", key, value)
}

// PadRight pads a string with spaces to the right to a given length
func PadHeader(str string, length int) string {
	var buf strings.Builder
	buf.WriteByte('#')
	buf.WriteByte(' ')
	buf.WriteByte('[')
	buf.WriteString(str)
	buf.WriteByte(']')
	if buf.Len() >= length {
		return buf.String()
	}
	// pad with '='
	pad := length - buf.Len() - 1
	buf.WriteByte(' ')
	for i := 0; i < pad; i++ {
		buf.WriteByte('=')
	}
	return buf.String()
}
