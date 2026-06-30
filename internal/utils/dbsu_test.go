package utils

import (
	"os"
	"os/user"
	"testing"

	"pig/internal/config"
)

func TestCommandWritersStructured(t *testing.T) {
	stdout, stderr := commandWriters(true, false)
	if stdout != os.Stderr {
		t.Fatalf("structured mode should route stdout to stderr, got %T", stdout)
	}
	if stderr != os.Stderr {
		t.Fatalf("structured mode should keep stderr on stderr, got %T", stderr)
	}
}

func TestCommandWritersStructuredPreserveStdout(t *testing.T) {
	stdout, stderr := commandWriters(true, true)
	if stdout != os.Stdout {
		t.Fatalf("preserve stdout mode should keep stdout on stdout, got %T", stdout)
	}
	if stderr != os.Stderr {
		t.Fatalf("preserve stdout mode should keep stderr on stderr, got %T", stderr)
	}
}

func TestCommandWritersTextMode(t *testing.T) {
	stdout, stderr := commandWriters(false, false)
	if stdout != os.Stdout {
		t.Fatalf("text mode should write stdout to stdout, got %T", stdout)
	}
	if stderr != os.Stderr {
		t.Fatalf("text mode should write stderr to stderr, got %T", stderr)
	}
}

func TestDBSUCommandStdoutSeparatesStderrOnSuccess(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	origUser := config.CurrentUser
	defer func() { config.CurrentUser = origUser }()
	config.CurrentUser = current.Username

	out, err := DBSUCommandStdout(current.Username, []string{"sh", "-c", "printf stdout; printf stderr >&2"})
	if err != nil {
		t.Fatalf("DBSUCommandStdout returned error: %v", err)
	}
	if out != "stdout" {
		t.Fatalf("DBSUCommandStdout output = %q, want stdout only", out)
	}
}
