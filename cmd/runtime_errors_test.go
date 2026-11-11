package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/cli/pgbackrest"
	"pig/cli/pitr"
	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

func TestRuntimePrecheckErrorsRemainVisible(t *testing.T) {
	origFormat, origUser := config.OutputFormat, config.CurrentUser
	origPITR, origPB := *pitrOpts, pbConfig
	origDefault, origYes, origPlan := pbRestoreDefault, pbRestoreYes, pbRestorePlan
	origActive, origPostgres, origInited := ext.Active, ext.Postgres, ext.Inited
	t.Cleanup(func() {
		config.OutputFormat, config.CurrentUser = origFormat, origUser
		*pitrOpts, pbConfig = origPITR, origPB
		pbRestoreDefault, pbRestoreYes, pbRestorePlan = origDefault, origYes, origPlan
		ext.Active, ext.Postgres, ext.Inited = origActive, origPostgres, origInited
	})
	config.CurrentUser = "pigtest"
	missingConfig := filepath.Join(t.TempDir(), "missing-pgbackrest.conf")
	pbConfig = &pgbackrest.Config{ConfigPath: missingConfig, DbSU: "pigtest"}
	pbRestoreDefault, pbRestoreYes, pbRestorePlan = true, true, false

	// Only this fake psql may execute. Matching DBSU avoids sudo/su entirely.
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "psql"), []byte("#!/bin/sh\nprintf 'fake connection refused\\n' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ext.Active, ext.Inited = &ext.PostgresInstall{BinPath: binDir}, true

	for _, test := range []struct {
		name     string
		run      func(*cobra.Command) error
		message  string
		code     int
		textExit int
	}{
		{"pitr", func(command *cobra.Command) error {
			*pitrOpts = pitr.Options{Default: true, Yes: true, ConfigPath: missingConfig, DbSU: "pigtest"}
			return pitrCmd.RunE(command, nil)
		}, "config file not found", output.CodePITRPrecheckFailed, output.ExitCode(output.CodePITRPrecheckFailed)},
		{"pitr plan", func(command *cobra.Command) error {
			*pitrOpts = pitr.Options{Default: true, Plan: true, ConfigPath: missingConfig, DbSU: "pigtest"}
			return pitrCmd.RunE(command, nil)
		}, "config file not found", output.CodePITRPrecheckFailed, output.ExitCode(output.CodePITRPrecheckFailed)},
		{"pb restore", func(command *cobra.Command) error {
			return pbRestoreCmd.RunE(command, nil)
		}, "config file not found", output.CodePbConfigNotFound, 1},
		{"pg clone", func(command *cobra.Command) error {
			return runClone(command, &postgres.CloneOptions{SourceDB: "source", DestDB: "target", DbSU: "pigtest", Yes: true})
		}, "fake connection refused", output.CodeForkPrecheckFailed, output.ExitCode(output.CodeForkPrecheckFailed)},
		{"pg clone plan", func(command *cobra.Command) error {
			return runClone(command, &postgres.CloneOptions{SourceDB: "source", DestDB: "target", DbSU: "pigtest", Plan: true})
		}, "fake connection refused", output.CodeForkPrecheckFailed, output.ExitCode(output.CodeForkPrecheckFailed)},
	} {
		for _, format := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON, config.OUTPUT_YAML} {
			t.Run(test.name+"/"+format, func(t *testing.T) {
				config.OutputFormat = format
				command := &cobra.Command{Use: test.name}
				var runErr error
				var stdout string
				stderr := captureStderr(t, func() {
					stdout = captureStdout(t, func() { runErr = test.run(command) })
				})
				if runErr == nil {
					t.Fatal("precheck failure returned success")
				}
				if format == config.OUTPUT_TEXT {
					if stdout != "" || strings.Count(stderr, "Error: ") != 1 || !strings.Contains(stderr, test.message) || strings.Contains(stderr, "Usage:") {
						t.Fatalf("precheck diagnostics stdout=%q stderr=%q", stdout, stderr)
					}
					if !command.SilenceUsage || !command.SilenceErrors || !utils.IsSilentExit(runErr) || shouldLogExecutionError(runErr) {
						t.Fatal("Cobra/root logger may repeat the runtime failure")
					}
					if utils.ExitCode(runErr) != test.textExit {
						t.Fatalf("text exit=%d, want %d", utils.ExitCode(runErr), test.textExit)
					}
					return
				}
				var result output.Result
				var err error
				if format == config.OUTPUT_YAML {
					decoder := yaml.NewDecoder(strings.NewReader(stdout))
					err = decoder.Decode(&result)
					if err == nil && decoder.Decode(new(any)) != io.EOF {
						t.Fatal("stdout contains more than one YAML value")
					}
				} else {
					err = json.Unmarshal([]byte(stdout), &result)
				}
				if err != nil || result.Success || result.Code != test.code || utils.ExitCode(runErr) != output.ExitCode(test.code) {
					t.Fatalf("failure envelope=%+v decode=%v runErr=%v", result, err, runErr)
				}
				if stderr != "" || !strings.Contains(result.Message+result.Detail, test.message) {
					t.Fatalf("structured failure lost diagnostics or leaked text: result=%+v stderr=%q", result, stderr)
				}
			})
		}
	}
}
