package utils

import (
	"os"
	"testing"
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
