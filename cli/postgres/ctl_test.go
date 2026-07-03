/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pg start/stop text-path idempotency (B06/B22, T9 semantics):
starting a running server and stopping a stopped server succeed with a
single informational line instead of failing.
*/
package postgres

import (
	"io"
	"os"
	"strings"
	"testing"

	"pig/internal/config"
)

// stubCtlChecks replaces the ctl state-check seams for one test.
func stubCtlChecks(t *testing.T, exists, initialized, running bool, pid int) {
	t.Helper()
	origDataDir := ctlCheckDataDir
	origRunning := ctlCheckRunning
	origRunningState := ctlCheckRunningState
	t.Cleanup(func() {
		ctlCheckDataDir = origDataDir
		ctlCheckRunning = origRunning
		ctlCheckRunningState = origRunningState
	})
	ctlCheckDataDir = func(dbsu, dataDir string) (bool, bool) { return exists, initialized }
	ctlCheckRunning = func(dbsu, dataDir string) (bool, int) { return running, pid }
	ctlCheckRunningState = func(dbsu, dataDir string) (bool, int, string, error) {
		return running, pid, "", nil
	}
}

func captureCtlStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe failed: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout failed: %v", err)
	}
	_ = r.Close()
	return string(out)
}

func TestStartTextAlreadyRunningIsIdempotentSuccess(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	config.OutputFormat = config.OUTPUT_TEXT

	stubCtlChecks(t, true, true, true, 4242)

	var startErr error
	out := captureCtlStdout(t, func() {
		startErr = Start(nil, &StartOptions{})
	})
	if startErr != nil {
		t.Fatalf("pg start on running server should succeed, got %v", startErr)
	}
	if !strings.Contains(out, "PostgreSQL is already running (pid 4242)") {
		t.Fatalf("expected already-running line, got %q", out)
	}
}

func TestStartTextUninitializedDataDirStillFails(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	config.OutputFormat = config.OUTPUT_TEXT

	stubCtlChecks(t, true, false, false, 0)

	err := Start(nil, &StartOptions{})
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("pg start on uninitialized dir should fail, got %v", err)
	}
}

func TestStopTextAlreadyStoppedIsIdempotentSuccess(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	config.OutputFormat = config.OUTPUT_TEXT

	stubCtlChecks(t, true, true, false, 0)

	var stopErr error
	out := captureCtlStdout(t, func() {
		stopErr = Stop(nil, &StopOptions{Mode: "fast"})
	})
	if stopErr != nil {
		t.Fatalf("pg stop on stopped server should succeed, got %v", stopErr)
	}
	if !strings.Contains(out, "PostgreSQL is already stopped") {
		t.Fatalf("expected already-stopped line, got %q", out)
	}
}

func TestStopTextInvalidModeStillFails(t *testing.T) {
	stubCtlChecks(t, true, true, false, 0)

	err := Stop(nil, &StopOptions{Mode: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "invalid stop mode") {
		t.Fatalf("pg stop with invalid mode should fail, got %v", err)
	}
}

func TestRestartTextStoppedFailsInsteadOfStarting(t *testing.T) {
	stubCtlChecks(t, true, true, false, 0)

	err := Restart(nil, &RestartOptions{Mode: "fast"})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("pg restart on stopped server should fail before pg_ctl restart, got %v", err)
	}
}

func TestRestartTextStatusCheckErrorIsNotReportedAsStopped(t *testing.T) {
	origRunning := ctlCheckRunning
	origRunningState := ctlCheckRunningState
	t.Cleanup(func() {
		ctlCheckRunning = origRunning
		ctlCheckRunningState = origRunningState
	})
	ctlCheckRunning = func(dbsu, dataDir string) (bool, int) {
		return false, 0
	}
	ctlCheckRunningState = func(dbsu, dataDir string) (bool, int, string, error) {
		return false, 0, "", os.ErrPermission
	}

	err := Restart(&Config{PgData: "/pg/data", DbSU: config.CurrentUser}, &RestartOptions{Mode: "fast"})
	if err == nil {
		t.Fatal("expected status check error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("restart should preserve status check error, got %v", err)
	}
	if strings.Contains(err.Error(), "use 'pig pg start'") {
		t.Fatalf("restart should not report permission errors as stopped instance: %v", err)
	}
}
