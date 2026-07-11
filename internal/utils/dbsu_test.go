package utils

import (
	"errors"
	"io"
	"os"
	"os/user"
	"strings"
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

func TestCaptureDBSUCommandSeparatesStreams(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	origUser := config.CurrentUser
	defer func() { config.CurrentUser = origUser }()
	config.CurrentUser = current.Username

	result, err := CaptureDBSUCommand(current.Username, []string{
		"sh", "-c", "printf stdout; printf stderr >&2; exit 7",
	})
	if err == nil {
		t.Fatal("CaptureDBSUCommand returned nil error, want exit error")
	}
	if result.Stdout != "stdout" {
		t.Fatalf("stdout = %q, want %q", result.Stdout, "stdout")
	}
	if result.Stderr != "stderr" {
		t.Fatalf("stderr = %q, want %q", result.Stderr, "stderr")
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestDBSUCommandOutputPreservesCombinedStreamOrder(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	origUser := config.CurrentUser
	defer func() { config.CurrentUser = origUser }()
	config.CurrentUser = current.Username

	out, err := DBSUCommandOutput(current.Username, []string{
		"sh", "-c", "printf out1; printf err1 >&2; printf out2; printf err2 >&2",
	})
	if err != nil {
		t.Fatalf("DBSUCommandOutput returned error: %v", err)
	}
	if out != "out1err1out2err2" {
		t.Fatalf("combined output = %q, want interleaved stream order", out)
	}
}

func TestDBSUCommandStdoutReturnsStderrAsErrorDetail(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	origUser := config.CurrentUser
	defer func() { config.CurrentUser = origUser }()
	config.CurrentUser = current.Username

	out, err := DBSUCommandStdout(current.Username, []string{
		"sh", "-c", "printf partial; printf diagnostic >&2; exit 3",
	})
	if out != "partial" {
		t.Fatalf("stdout = %q, want %q", out, "partial")
	}
	if err == nil || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("error = %v, want stderr detail", err)
	}
}

func TestBuildDBSUCommandHonorsNonInteractiveMode(t *testing.T) {
	origUser := config.CurrentUser
	defer func() { config.CurrentUser = origUser }()
	config.CurrentUser = "test-user"
	t.Setenv("PIG_NON_INTERACTIVE", "1")

	cmd, err := BuildDBSUCommand("postgres", []string{"psql", "-Atc", "select 1"})
	if err != nil {
		t.Fatalf("BuildDBSUCommand returned error: %v", err)
	}
	want := []string{"sudo", "-n", "-inu", "postgres", "--", "psql", "-Atc", "select 1"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("sudo args = %q, want %q", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("sudo args = %q, want %q", cmd.Args, want)
		}
	}
}

func TestDBSUCommandFailedAfterStreamingOutputIsSilent(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	origUser := config.CurrentUser
	defer func() { config.CurrentUser = origUser }()
	config.CurrentUser = current.Username

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW

	runErr := DBSUCommand(current.Username, []string{
		"sh",
		"-c",
		"printf 'Current cluster topology\n+ table row +\n'; printf 'Error: No candidates found to switchover to\n' >&2; exit 1",
	})

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr
	stdout, _ := io.ReadAll(stdoutR)
	stderr, _ := io.ReadAll(stderrR)
	_ = stdoutR.Close()
	_ = stderrR.Close()

	var exitErr *ExitCodeError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("DBSUCommand returned %T, want ExitCodeError: %v", runErr, runErr)
	}
	if exitErr.Code != 1 {
		t.Fatalf("ExitCodeError.Code = %d, want 1", exitErr.Code)
	}
	if !exitErr.Silent {
		t.Fatalf("ExitCodeError.Silent = false, want true for already-streamed output; stdout=%q stderr=%q err=%v", stdout, stderr, runErr)
	}
	if string(stdout) != "Current cluster topology\n+ table row +\n" {
		t.Fatalf("stdout = %q, want raw command stdout", string(stdout))
	}
	if string(stderr) != "Error: No candidates found to switchover to\n" {
		t.Fatalf("stderr = %q, want raw command stderr", string(stderr))
	}
}
