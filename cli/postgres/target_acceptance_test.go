package postgres

import (
	"os"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"testing"
)

func TestPsqlExplicitStdinInNonInteractiveModes(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	oldInited, oldActive, oldFormat, oldInput := ext.Inited, ext.Active, config.OutputFormat, os.Stdin
	t.Cleanup(func() {
		ext.Inited, ext.Active, config.OutputFormat, os.Stdin = oldInited, oldActive, oldFormat, oldInput
	})
	bin := t.TempDir()
	output := filepath.Join(t.TempDir(), "sql")
	if err := os.WriteFile(filepath.Join(bin, "psql"), []byte("#!/bin/sh\ncat > \"$PIG_TEST_SQL_INPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin}
	t.Setenv("PIG_TEST_SQL_INPUT", output)
	for _, mode := range []string{"env", "json", "yaml"} {
		t.Run(mode, func(t *testing.T) {
			config.OutputFormat = config.OUTPUT_TEXT
			t.Setenv("PIG_NON_INTERACTIVE", "")
			if mode == "env" {
				t.Setenv("PIG_NON_INTERACTIVE", "1")
			} else {
				config.OutputFormat = mode
			}
			input, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			const sql = "select 'data stdin sentinel';\n"
			_, _ = input.WriteString(sql)
			_, _ = input.Seek(0, 0)
			os.Stdin = input
			if err := Psql(&Config{DbSU: dbsu}, "postgres", &PsqlOptions{File: "-"}); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(output); err != nil || string(got) != sql {
				t.Fatalf("explicit psql input lost: %q, %v", got, err)
			}
		})
	}
}
