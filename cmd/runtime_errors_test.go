package cmd

import (
	"encoding/json"
	"io"
	"os"
	"os/user"
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
		{"pg fork", func(command *cobra.Command) error {
			return runFork(command, &postgres.Options{Kind: postgres.KindInstance, DbSU: "pigtest", Yes: true, Instance: postgres.InstanceOptions{Name: "bad name"}})
		}, "fork name", output.CodeForkInvalidArgs, output.ExitCode(output.CodeForkInvalidArgs)},
		{"pg fork plan", func(command *cobra.Command) error {
			return runFork(command, &postgres.Options{Kind: postgres.KindInstance, DbSU: "pigtest", Plan: true, Instance: postgres.InstanceOptions{Name: "bad name"}})
		}, "fork name", output.CodeForkInvalidArgs, output.ExitCode(output.CodeForkInvalidArgs)},
		{"pg fork action", func(command *cobra.Command) error {
			return runForkAction(command, "fork stop", func() (postgres.ResultData, error) {
				return postgres.ResultData{}, &postgres.ForkError{Code: output.CodeForkSourceNotFound, Err: os.ErrNotExist}
			})
		}, "file does not exist", output.CodeForkSourceNotFound, output.ExitCode(output.CodeForkSourceNotFound)},
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

func TestPgStatusTextModeSuppressesUsageAndDuplicateErrors(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("detect current user: %v", err)
	}
	origFormat, origUser := config.OutputFormat, config.CurrentUser
	origPg := *pgConfig
	origChanged := map[string]bool{}
	for _, name := range []string{"data", "dbsu", "version"} {
		origChanged[name] = pgCmd.PersistentFlags().Lookup(name).Changed
	}
	origSilenceUsage, origSilenceErrors := pgStatusCmd.SilenceUsage, pgStatusCmd.SilenceErrors
	origRootSilenceUsage, origRootSilenceErrors := rootCmd.SilenceUsage, rootCmd.SilenceErrors
	t.Cleanup(func() {
		config.OutputFormat, config.CurrentUser = origFormat, origUser
		*pgConfig = origPg
		pgStatusCmd.SilenceUsage, pgStatusCmd.SilenceErrors = origSilenceUsage, origSilenceErrors
		rootCmd.SilenceUsage, rootCmd.SilenceErrors = origRootSilenceUsage, origRootSilenceErrors
		rootCmd.SetArgs(nil)
		for name, changed := range origChanged {
			pgCmd.PersistentFlags().Lookup(name).Changed = changed
		}
	})

	binDir := t.TempDir()
	for name, body := range map[string]string{
		"ps":        "#!/bin/sh\nprintf 'PID PPID START COMMAND\\n'\n",
		"systemctl": "#!/bin/sh\nprintf 'inactive\\n'\nexit 3\n",
	} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)

	config.OutputFormat = config.OUTPUT_TEXT
	config.CurrentUser = current.Username
	missingDir := filepath.Join(t.TempDir(), "missing-pgdata")

	var runErr error
	var stdout string
	args := []string{"pg", "status", "-D", missingDir, "-U", current.Username, "-v", "999"}
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			rootCmd.SetArgs(args)
			prepareEarlyOutputSettings(args)
			wrapSilentExitSilence(rootCmd)
			runErr = rootCmd.Execute()
		})
	})

	if runErr == nil {
		t.Fatal("pg status on missing data directory returned nil error")
	}
	wantExit := output.ExitCode(output.CodePgStatusDataDirNotFound)
	if got := utils.ExitCode(runErr); got != wantExit {
		t.Fatalf("pg status exit = %d, want %d (err=%v)", got, wantExit, runErr)
	}
	if !strings.Contains(stdout, "PostgreSQL Status Summary") || !strings.Contains(stdout, "[pg_ctl status]") {
		t.Fatalf("pg status missing summary sections on stdout: %q", stdout)
	}
	if strings.Contains(stderr, "Error:") || strings.Contains(stderr, "Usage:") {
		t.Fatalf("pg status leaked Cobra Error/Usage to stderr: %q", stderr)
	}
	if !utils.IsSilentExit(runErr) || shouldLogExecutionError(runErr) {
		t.Fatalf("pg status returned non-silent error %#v; root logger would emit duplicate ERRO line", runErr)
	}
}
