package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"pig/internal/output"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func TestPgMaintenanceRejectsConnectionStringsBeforePlanningOrExecution(t *testing.T) {
	format := os.Getenv("PIG_TEST_MAINTENANCE_FORMAT")
	if format == "" {
		for _, format := range []string{"text", "json", "yaml"} {
			t.Run(format, func(t *testing.T) {
				child := exec.Command(os.Args[0], "-test.run=^TestPgMaintenanceRejectsConnectionStringsBeforePlanningOrExecution$")
				child.Env = append(os.Environ(), "PIG_TEST_MAINTENANCE_FORMAT="+format)
				if raw, err := child.CombinedOutput(); err != nil {
					t.Fatalf("%v\n%s", err, raw)
				}
			})
		}
		return
	}

	rootCmd.AddCommand(pgCmd)
	var diagnostics bytes.Buffer
	rootCmd.SetOut(&diagnostics)
	rootCmd.SetErr(&diagnostics)
	preRuns := 0
	pgCmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		preRuns++
		return fmt.Errorf("unexpected operational pre-run")
	}
	const secret = "MAINTENANCE_PASSWORD_SENTINEL"
	for _, database := range []string{
		"host=/unused port=6544 dbname=app password=" + secret,
		"postgresql://review:" + secret + "@localhost/app",
		"postgres://review:" + secret + "@localhost/app",
	} {
		for _, operation := range [][]string{
			{"vacuum"}, {"vacuum", "--full"}, {"vacuum", "--full", "--plan"},
			{"vacuum", "--full", "--yes"}, {"vacuum", "--all"},
			{"analyze"}, {"freeze"}, {"repack"}, {"repack", "--plan"}, {"repack", "--all"},
		} {
			pgMaintPlan, pgMaintFull, pgMaintYes, pgMaintAll = false, false, false, false
			diagnostics.Reset()
			args := append([]string{"pg"}, operation...)
			args = append(args, "-o", format, "--", database)
			rootCmd.SetArgs(args)
			var runErr error
			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() { runErr = rootCmd.Execute() })
			})
			if preRuns != 0 || runErr == nil {
				t.Fatalf("%v reached setup or execution: pre-runs=%d err=%v", operation, preRuns, runErr)
			}
			if strings.Contains(stdout+stderr+diagnostics.String()+fmt.Sprint(runErr), secret) {
				t.Fatalf("%v exposed the database credential", operation)
			}
			if format == "text" {
				if !strings.Contains(runErr.Error(), "connection string") {
					t.Fatalf("unexpected validation error: %v", runErr)
				}
				continue
			}
			var result output.Result
			var decodeErr error
			if format == "json" {
				decodeErr = json.Unmarshal([]byte(stdout), &result)
			} else {
				decodeErr = yaml.Unmarshal([]byte(stdout), &result)
			}
			if decodeErr != nil || result.Success || result.Code != output.GenericParamError(output.MODULE_PG) {
				t.Fatalf("%v returned an invalid parameter result: %v / %+v", operation, decodeErr, result)
			}
		}
	}
}
