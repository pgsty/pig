package inventory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const maxAnsibleStderr = 8 * 1024

var sensitiveDiagnosticLine = regexp.MustCompile(`(?i)(password|passwd|secret|token|private[_ -]?key|access[_ -]?key|credential|proxy|dsn|connection[_ -]?(url|uri))`) //nolint:lll

// ValidateWithAnsible asks an installed ansible-inventory executable to parse
// a private temporary candidate. Stdout is always discarded because --list
// can contain resolved passwords. executable must be an already resolved path.
func ValidateWithAnsible(ctx context.Context, executable string, doc *Document) []Diagnostic {
	if doc == nil {
		return []Diagnostic{{Code: "ANSIBLE_DOCUMENT_NIL", Severity: SeverityError, Message: "cannot validate a nil inventory document"}}
	}
	if strings.TrimSpace(executable) == "" {
		return []Diagnostic{{Code: "ANSIBLE_NOT_FOUND", Severity: SeverityWarning, Message: "ansible-inventory is not installed; external inventory validation was skipped"}}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	temp, err := os.CreateTemp("", "pig-inventory-*.yml")
	if err != nil {
		return []Diagnostic{{Code: "ANSIBLE_TEMPFILE", Severity: SeverityError, Message: "failed to create private candidate for ansible-inventory"}}
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.Write(doc.source); err != nil {
		temp.Close()
		return []Diagnostic{{Code: "ANSIBLE_TEMPFILE", Severity: SeverityError, Message: "failed to write candidate used by ansible-inventory"}}
	}
	if err := temp.Close(); err != nil {
		return []Diagnostic{{Code: "ANSIBLE_TEMPFILE", Severity: SeverityError, Message: "failed to close candidate used by ansible-inventory"}}
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, executable, "-i", name, "--list")
	cmd.Stdout = io.Discard
	cmd.Stderr = &limitedBuffer{buffer: &stderr, remaining: maxAnsibleStderr}
	err = cmd.Run()
	if err == nil {
		return nil
	}

	exit := "unknown"
	var exitError *exec.ExitError
	if ctx.Err() != nil {
		exit = "timeout"
	} else if errors.As(err, &exitError) {
		exit = strconv.Itoa(exitError.ExitCode())
	}
	message := fmt.Sprintf("ansible-inventory rejected candidate (exit=%s)", exit)
	if detail := redactAnsibleStderr(stderr.String(), name); detail != "" {
		message += ": " + detail
	}
	return []Diagnostic{{Code: "ANSIBLE_INVENTORY_REJECTED", Severity: SeverityError, Message: message}}
}

// LookPathAndValidateWithAnsible resolves ansible-inventory and returns a
// warning diagnostic when it is not installed.
func LookPathAndValidateWithAnsible(ctx context.Context, doc *Document) []Diagnostic {
	executable, _ := exec.LookPath("ansible-inventory")
	return ValidateWithAnsible(ctx, executable, doc)
}

type limitedBuffer struct {
	buffer    *bytes.Buffer
	remaining int
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if w.remaining <= 0 {
		return original, nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	_, _ = w.buffer.Write(p)
	w.remaining -= len(p)
	return original, nil
}

func redactAnsibleStderr(stderr, tempPath string) string {
	stderr = strings.ReplaceAll(stderr, tempPath, "<candidate>")
	stderr = strings.ReplaceAll(stderr, filepath.Base(tempPath), "<candidate>")
	lines := strings.Split(stderr, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if sensitiveDiagnosticLine.MatchString(line) || (strings.Contains(line, "://") && strings.Contains(line, "@")) {
			clean = append(clean, "<redacted sensitive diagnostic line>")
			continue
		}
		if len(line) > 512 {
			line = line[:512] + "..."
		}
		clean = append(clean, line)
		if len(clean) == 3 {
			break
		}
	}
	return strings.Join(clean, " | ")
}
