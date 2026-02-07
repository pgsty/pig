package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"pig/internal/config"
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
	if err := json.Unmarshal(bytesTrimSpace(raw), &payload); err != nil {
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

func TestStyConfOutputFileFlagKeepsGlobalOutputFlag(t *testing.T) {
	flag := pigstyConfCmd.Flags().Lookup("output-file")
	if flag == nil {
		t.Fatal("expected --output-file flag on pig sty conf")
	}
	if flag.Shorthand != "O" {
		t.Fatalf("expected shorthand -O for output-file, got -%s", flag.Shorthand)
	}
	if local := pigstyConfCmd.Flags().Lookup("output"); local != nil {
		t.Fatalf("did not expect local --output flag on pig sty conf, got %+v", local)
	}
	if inherited := pigstyConfCmd.InheritedFlags().Lookup("output"); inherited == nil {
		t.Fatal("expected inherited global --output flag on pig sty conf")
	}
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
