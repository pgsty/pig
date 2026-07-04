package patroni

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"pig/internal/config"
)

func TestLogTextUsesLocalFileDirectlyWhenAlreadyDBSU(t *testing.T) {
	tmp := t.TempDir()
	logDir := filepath.Join(tmp, "log")
	if err := os.Mkdir(logDir, 0755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, DefaultLogFile), []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	fakeBin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(fakeBin, 0755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	sudoMarker := filepath.Join(tmp, "sudo-called")
	suMarker := filepath.Join(tmp, "su-called")
	writeTestCommand(t, fakeBin, "sudo", ": > "+shellQuote(sudoMarker)+"\nexit 1\n")
	writeTestCommand(t, fakeBin, "su", ": > "+shellQuote(suMarker)+"\nexit 1\n")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	origUser := config.CurrentUser
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.CurrentUser = origUser
		config.OutputFormat = origFormat
	})
	config.CurrentUser = "postgres"
	config.OutputFormat = config.OUTPUT_TEXT

	var out bytes.Buffer
	withPatroniStdout(t, &out, func() {
		if err := LogCat(logDir, "postgres", 2); err != nil {
			t.Fatalf("LogCat returned error: %v", err)
		}
	})

	if _, err := os.Stat(sudoMarker); err == nil {
		t.Fatal("LogCat should not call sudo when current user is already DBSU")
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking sudo marker: %v", err)
	}
	if _, err := os.Stat(suMarker); err == nil {
		t.Fatal("LogCat should not call su when current user is already DBSU")
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking su marker: %v", err)
	}
	if got := out.String(); got != "line2\nline3\n" {
		t.Fatalf("LogCat output = %q, want local tail of log file", got)
	}
}
