package utils

import (
	"io"
	"os"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"
)

func TestCommandNonInteractiveMatrix(t *testing.T) {
	oldUser, oldFormat, oldIn, oldSummary := config.CurrentUser, config.OutputFormat, os.Stdin, sudoCommandSummaryOut
	t.Cleanup(func() {
		config.CurrentUser, config.OutputFormat, os.Stdin, sudoCommandSummaryOut = oldUser, oldFormat, oldIn, oldSummary
	})
	sudoCommandSummaryOut = io.Discard
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "sudo-argv")
	inputLog := filepath.Join(t.TempDir(), "input")
	scripts := map[string]string{
		"sudo": `printf '%s\n' "$@" > "$PIG_TEST_SUDO_ARGS"
while [ $# -gt 0 ]; do case "$1" in -n|-H|--) shift;; -u) shift 2;; *) break;; esac; done
exec "$@"`,
		"su": `printf '%s\n' "$@" > "$PIG_TEST_SUDO_ARGS"
exec sh -c "$4"`,
		"probe": `if IFS= read -r line; then printf '%s' "$line" > "$PIG_TEST_INPUT"; else : > "$PIG_TEST_INPUT"; fi`,
	}
	scripts["systemctl"] = scripts["probe"]
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIG_TEST_SUDO_ARGS", log)
	t.Setenv("PIG_TEST_INPUT", inputLog)
	t.Setenv("PIG_NO_SUDO", "")
	for _, identity := range []string{"caller", "root", "dbsu"} {
		for _, mode := range []string{"text", "env", "json", "yaml"} {
			t.Run(identity+"/"+mode, func(t *testing.T) {
				config.CurrentUser = identity
				config.OutputFormat = config.OUTPUT_TEXT
				t.Setenv("PIG_NON_INTERACTIVE", "")
				if mode == "env" {
					t.Setenv("PIG_NON_INTERACTIVE", "1")
				}
				if mode == "json" || mode == "yaml" {
					config.OutputFormat = mode
				}
				for _, op := range []struct {
					name       string
					run        func() error
					quiet, raw bool
				}{
					{"sudo", func() error { return SudoCommand([]string{"probe"}) }, false, false},
					{"quiet", func() error { return QuietSudoCommand([]string{"probe"}) }, true, false},
					{"summary", func() error { return SudoCommandSummary([]string{"probe"}, 5) }, false, false},
					{"systemctl", func() error { return RunSystemctl("restart", "fixture") }, false, false},
					{"dbsu", func() error { return DBSUCommand("dbsu", []string{"probe"}) }, false, false},
					{"native", func() error { return DBSUCommandPassthrough("dbsu", []string{"probe"}) }, false, true},
				} {
					t.Run(op.name, func(t *testing.T) {
						_ = os.Remove(log)
						in, err := os.CreateTemp(t.TempDir(), "stdin")
						if err != nil {
							t.Fatal(err)
						}
						defer in.Close()
						_, _ = in.WriteString("input sentinel\n")
						_, _ = in.Seek(0, 0)
						os.Stdin = in
						if err := op.run(); err != nil {
							t.Fatal(err)
						}
						got, _ := os.ReadFile(inputLog)
						wantInput := (mode == "text" && !op.quiet) || op.raw
						if (string(got) == "input sentinel") != wantInput {
							t.Errorf("stdin=%q, want attached=%v", got, wantInput)
						}
						argv, _ := os.ReadFile(log)
						isDBSU := op.name == "dbsu" || op.name == "native"
						usesSudo := (!isDBSU && os.Geteuid() != 0 && identity != "root") || (isDBSU && identity == "caller")
						if op.name == "systemctl" {
							usesSudo = os.Geteuid() != 0
						}
						if usesSudo && strings.HasPrefix(string(argv), "-n\n") != (mode != "text") {
							t.Errorf("sudo flags %q, noninteractive=%v", argv, mode != "text")
						}
						if isDBSU && identity == "dbsu" && len(argv) != 0 {
							t.Errorf("same DBSU escalated: %q", argv)
						}
					})
				}
			})
		}
	}
}

func TestExplicitDBSUDataInputSurvivesNonInteractive(t *testing.T) {
	oldUser, oldFormat := config.CurrentUser, config.OutputFormat
	t.Cleanup(func() { config.CurrentUser, config.OutputFormat = oldUser, oldFormat })
	config.CurrentUser = "dbsu"
	config.OutputFormat = config.OUTPUT_JSON
	t.Setenv("PIG_NON_INTERACTIVE", "1")
	path := filepath.Join(t.TempDir(), "payload")
	if err := DBSUCommandWithStdin("dbsu", []string{"sh", "-c", `cat > "$1"`, "data", path}, strings.NewReader("data sentinel")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "data sentinel" {
		t.Fatalf("lost explicit stdin: %q", got)
	}
}
