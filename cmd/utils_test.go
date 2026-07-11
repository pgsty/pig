package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

func TestRunLegacyStructuredCapturesCommandOutput(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()
	config.OutputFormat = config.OUTPUT_JSON

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	runErr := runLegacyStructured(990000, "test command", []string{"arg1"}, map[string]interface{}{"k": "v"}, func() error {
		fmt.Println("legacy text output")
		return nil
	})

	_ = w.Close()
	os.Stdout = origStdout
	raw, _ := io.ReadAll(r)
	_ = r.Close()

	if runErr != nil {
		t.Fatalf("runLegacyStructured() error = %v", runErr)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(trimJSONSpace(raw), &payload); err != nil {
		t.Fatalf("invalid json output: %v, raw=%q", err, string(raw))
	}

	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected success=true, got payload=%v", payload)
	}

	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if cmd, _ := data["command"].(string); cmd != "test command" {
		t.Fatalf("expected command 'test command', got %q", cmd)
	}
	if captured, _ := data["captured_output"].(string); !strings.Contains(captured, "legacy text output") {
		t.Fatalf("expected captured output, got %q", captured)
	}
}

func TestRunLogSnapshotDispatchesByOutputPolicy(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()

	jsonCalls, textCalls := 0, 0
	jsonl := func() error { jsonCalls++; return nil }
	text := func() error { textCalls++; return nil }

	config.OutputFormat = config.OUTPUT_TEXT
	if err := runLogSnapshot("test log", 10, jsonl, text); err != nil {
		t.Fatalf("text snapshot returned error: %v", err)
	}
	config.OutputFormat = config.OUTPUT_JSON
	if err := runLogSnapshot("test log", 10, jsonl, text); err != nil {
		t.Fatalf("json snapshot returned error: %v", err)
	}
	if jsonCalls != 1 || textCalls != 1 {
		t.Fatalf("dispatch calls json=%d text=%d, want 1 each", jsonCalls, textCalls)
	}

	config.OutputFormat = config.OUTPUT_YAML
	if err := runLogSnapshot("test log", 10, jsonl, text); err == nil {
		t.Fatal("yaml snapshot should be rejected")
	}
	if err := runLogSnapshot("test log", 0, jsonl, text); err == nil {
		t.Fatal("non-positive line count should be rejected")
	}
	if jsonCalls != 1 || textCalls != 1 {
		t.Fatalf("rejected snapshots executed a backend: json=%d text=%d", jsonCalls, textCalls)
	}
}

func TestRunLogStreamDispatchesByOutputPolicy(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()

	t.Run("text invokes stream", func(t *testing.T) {
		config.OutputFormat = config.OUTPUT_TEXT
		wantErr := errors.New("stream stopped")
		streamCalls := 0
		err := runLogStream(output.MODULE_PT, "pig pt log tail", "pig pt log show", 10, nil, nil, func() error {
			streamCalls++
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("runLogStream() error = %v, want %v", err, wantErr)
		}
		if streamCalls != 1 {
			t.Fatalf("stream calls = %d, want 1", streamCalls)
		}
	})

	t.Run("invalid lines reject before stream", func(t *testing.T) {
		config.OutputFormat = config.OUTPUT_TEXT
		streamCalls := 0
		err := runLogStream(output.MODULE_PT, "pig pt log tail", "pig pt log show", 0, nil, nil, func() error {
			streamCalls++
			return nil
		})
		if err == nil || err.Error() != "lines must be positive" {
			t.Fatalf("runLogStream() error = %v, want lines validation error", err)
		}
		if streamCalls != 0 {
			t.Fatalf("invalid lines executed stream %d time(s)", streamCalls)
		}
	})

	t.Run("structured returns typed error without stream", func(t *testing.T) {
		config.OutputFormat = config.OUTPUT_JSON
		streamCalls := 0
		var runErr error
		raw := captureStdout(t, func() {
			runErr = runLogStream(
				output.MODULE_PT,
				"pig pt log tail",
				"pig pt log show",
				10,
				[]string{"patroni.log"},
				map[string]interface{}{"lines": 10},
				func() error {
					streamCalls++
					return nil
				},
			)
		})

		var exitErr *utils.ExitCodeError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("runLogStream() error = %T %v, want *utils.ExitCodeError", runErr, runErr)
		}
		wantCode := output.GenericParamError(output.MODULE_PT)
		if exitErr.Code != output.ExitCode(wantCode) {
			t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitCode(wantCode))
		}
		if streamCalls != 0 {
			t.Fatalf("structured output executed stream %d time(s)", streamCalls)
		}

		var payload struct {
			Success bool              `json:"success"`
			Code    int               `json:"code"`
			Message string            `json:"message"`
			Detail  string            `json:"detail"`
			Data    legacyCommandData `json:"data"`
		}
		if err := json.Unmarshal(trimJSONSpace([]byte(raw)), &payload); err != nil {
			t.Fatalf("invalid JSON output: %v, raw=%q", err, raw)
		}
		if payload.Success || payload.Code != wantCode {
			t.Fatalf("payload success=%v code=%d, want false/%d", payload.Success, payload.Code, wantCode)
		}
		if payload.Message != "streaming log output is not supported in structured output" {
			t.Fatalf("message = %q", payload.Message)
		}
		if !strings.Contains(payload.Detail, "pig pt log show -n N -o json") {
			t.Fatalf("detail = %q, want snapshot guidance", payload.Detail)
		}
		if payload.Data.Command != "pig pt log tail" || len(payload.Data.Args) != 1 || payload.Data.Args[0] != "patroni.log" {
			t.Fatalf("unexpected command data: %+v", payload.Data)
		}
	})
}

func TestRunLegacyStructuredCapturesLogrusAndRestoresOutput(t *testing.T) {
	origFormat := config.OutputFormat
	origLogOut := logrus.StandardLogger().Out
	origLevel := logrus.GetLevel()
	defer func() {
		config.OutputFormat = origFormat
		logrus.SetOutput(origLogOut)
		logrus.SetLevel(origLevel)
	}()
	config.OutputFormat = config.OUTPUT_JSON
	logrus.SetLevel(logrus.InfoLevel)

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	runErr := runLegacyStructured(990000, "test command", nil, nil, func() error {
		logrus.Warn("legacy logrus output")
		return nil
	})

	_ = w.Close()
	os.Stdout = origStdout
	raw, _ := io.ReadAll(r)
	_ = r.Close()

	if runErr != nil {
		t.Fatalf("runLegacyStructured() error = %v", runErr)
	}
	if logrus.StandardLogger().Out != origLogOut {
		t.Fatal("runLegacyStructured should restore logrus output")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(trimJSONSpace(raw), &payload); err != nil {
		t.Fatalf("invalid json output: %v, raw=%q", err, string(raw))
	}
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if captured, _ := data["captured_output"].(string); !strings.Contains(captured, "legacy logrus output") {
		t.Fatalf("expected captured logrus output, got %q", captured)
	}
}

func TestCaptureLegacyOutputRestoresStateAfterPanic(t *testing.T) {
	origStdout := os.Stdout
	origStderr := os.Stderr
	origFormat := config.OutputFormat
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
		config.OutputFormat = origFormat
	}()

	config.OutputFormat = config.OUTPUT_JSON
	var recovered interface{}
	func() {
		defer func() {
			recovered = recover()
		}()
		_, _, _ = captureLegacyOutput(func() error {
			panic("legacy panic")
		}, 1024)
	}()

	if recovered != "legacy panic" {
		t.Fatalf("captureLegacyOutput should re-panic, got %v", recovered)
	}
	if os.Stdout != origStdout {
		t.Fatal("captureLegacyOutput should restore stdout after panic")
	}
	if os.Stderr != origStderr {
		t.Fatal("captureLegacyOutput should restore stderr after panic")
	}
	if config.OutputFormat != config.OUTPUT_JSON {
		t.Fatalf("captureLegacyOutput should restore output format, got %q", config.OutputFormat)
	}
}

func trimJSONSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
