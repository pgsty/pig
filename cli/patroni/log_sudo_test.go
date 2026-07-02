package patroni

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogTextUsesExactSudoJournalctlAndTailsLocally(t *testing.T) {
	tmp := t.TempDir()
	sudoArgs := filepath.Join(tmp, "sudo-args")
	directJournalctl := filepath.Join(tmp, "direct-journalctl")

	writeTestCommand(t, tmp, "sudo", "printf '%s\\n' \"$*\" > "+shellQuote(sudoArgs)+"\nprintf 'line1\\nline2\\nline3\\n'\n")
	writeTestCommand(t, tmp, "journalctl", ": > "+shellQuote(directJournalctl)+"\n")
	t.Setenv("PATH", tmp)

	var out bytes.Buffer
	withPatroniStdout(t, &out, func() {
		if err := Log(false, 2); err != nil {
			t.Fatalf("Log returned error: %v", err)
		}
	})

	if _, err := os.Stat(directJournalctl); err == nil {
		t.Fatal("Log should use sudo journalctl instead of calling journalctl directly")
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking direct journalctl marker: %v", err)
	}

	rawArgs, err := os.ReadFile(sudoArgs)
	if err != nil {
		t.Fatalf("sudo was not called: %v", err)
	}
	if got := strings.TrimSpace(string(rawArgs)); got != "/usr/bin/journalctl -u patroni" {
		t.Fatalf("sudo args = %q, want exact journalctl command", got)
	}
	if got := out.String(); got != "line2\nline3\n" {
		t.Fatalf("Log output = %q, want local tail of journal output", got)
	}
}

func TestFollowRowsSinceKeepsAllRowsAddedAfterInitialTail(t *testing.T) {
	rows := []string{"old1", "old2", "new1", "new2", "new3"}

	got, next := followRowsSince(rows, 2)

	if next != 5 {
		t.Fatalf("next seen = %d, want 5", next)
	}
	if strings.Join(got, "\n") != "new1\nnew2\nnew3" {
		t.Fatalf("follow rows = %v, want all rows after previous total", got)
	}
}
