package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"pig/internal/config"

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
