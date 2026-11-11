package sty

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestBootstrapReexecPreservesOnlyInteractiveTextInput(t *testing.T) {
	setupBootTest(t)
	if err := os.MkdirAll(bootRepoDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bootRepoDir, "repo_complete"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	originalFormat, originalStdin := config.OutputFormat, os.Stdin
	t.Cleanup(func() { config.OutputFormat, os.Stdin = originalFormat, originalStdin })
	t.Setenv("PIG_NO_SUDO", "")
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PIG_TEST_REEXEC_ARGS\"\nif IFS= read -r line; then printf '%s' \"$line\" > \"$PIG_TEST_REEXEC_INPUT\"; else printf eof > \"$PIG_TEST_REEXEC_INPUT\"; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		name, format, nonInteractive, wantInput string
		wantNoPrompt                            bool
	}{
		{"interactive text", config.OUTPUT_TEXT, "", "answer", false},
		{"noninteractive text", config.OUTPUT_TEXT, "1", "eof", true},
		{"structured", config.OUTPUT_JSON, "", "eof", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.OutputFormat = tc.format
			t.Setenv("PIG_NON_INTERACTIVE", tc.nonInteractive)
			dir := t.TempDir()
			inputPath, argsPath := filepath.Join(dir, "input"), filepath.Join(dir, "args")
			t.Setenv("PIG_TEST_REEXEC_INPUT", inputPath)
			t.Setenv("PIG_TEST_REEXEC_ARGS", argsPath)
			stdinPath := filepath.Join(dir, "stdin")
			if err := os.WriteFile(stdinPath, []byte("answer\n"), 0600); err != nil {
				t.Fatal(err)
			}
			stdin, err := os.Open(stdinPath)
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			os.Stdin = stdin
			outcome := ReexecBootstrapAsRoot([]string{"sty", "boot"}, io.Discard)
			if outcome.Err != nil || !outcome.Started {
				t.Fatalf("reexec failed: %+v", outcome)
			}
			input, err := os.ReadFile(inputPath)
			if err != nil || string(input) != tc.wantInput {
				t.Fatalf("stdin = %q, %v; want %q", input, err, tc.wantInput)
			}
			args, err := os.ReadFile(argsPath)
			if err != nil || strings.HasPrefix(string(args), "-n\n") != tc.wantNoPrompt {
				t.Fatalf("sudo args = %q, %v", args, err)
			}
		})
	}
}
