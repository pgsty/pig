package cmd

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"
	"testing"
)

func TestStructuredLegacyExecutionKeepsNonInteractivePolicy(t *testing.T) {
	oldFormat, oldUser, oldInput := config.OutputFormat, config.CurrentUser, os.Stdin
	t.Cleanup(func() { config.OutputFormat, config.CurrentUser, os.Stdin = oldFormat, oldUser, oldInput })
	config.OutputFormat = config.OUTPUT_JSON
	config.CurrentUser = "dbsu"
	t.Setenv("PIG_NON_INTERACTIVE", "")
	dir := t.TempDir()
	dest := filepath.Join(dir, "input")
	in, err := os.CreateTemp(dir, "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	_, _ = in.WriteString("must not read\n")
	_, _ = in.Seek(0, 0)
	os.Stdin = in
	raw := captureStdout(t, func() {
		if err := runLegacyStructured(990000, "fixture", nil, nil, func() error { return utils.DBSUCommand("dbsu", []string{"sh", "-c", `cat > "$1"`, "fixture", dest}) }); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(raw, `"success":true`) {
		t.Fatalf("result=%s", raw)
	}
	if got, _ := os.ReadFile(dest); len(got) != 0 {
		t.Fatalf("structured wrapper re-enabled stdin: %q", got)
	}
	if os.Getenv("PIG_NON_INTERACTIVE") != "" {
		t.Fatal("wrapper leaked environment state")
	}
}
