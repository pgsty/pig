package cmd

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pig/internal/output"
)

// Run each Cobra invocation in its own process so flags and configuration cannot
// leak between cases or into the command race suite.
func TestPgPlanAndConfirmationPreserveTarget(t *testing.T) {
	if raw := os.Getenv("PIG_TEST_TARGET_PLAN_ARGS"); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatal(err)
		}
		rootCmd.AddCommand(pgCmd)
		rootCmd.SetArgs(args)
		rootCmd.SetOut(io.Discard)
		rootCmd.SetErr(io.Discard)
		var runErr error
		stdout := captureStdout(t, func() { runErr = rootCmd.Execute() })
		var commands []string
		if slices.Contains(args, "--plan") {
			var plan output.Plan
			if err := json.Unmarshal([]byte(stdout), &plan); err != nil || runErr != nil {
				t.Fatalf("plan: %v / %v: %s", runErr, err, stdout)
			}
			if (args[1] == "promote" || args[1] == "vacuum") && plan.Confirmation != "required" {
				t.Errorf("confirmation=%q", plan.Confirmation)
			}
			commands = append(commands, plan.Command)
			for _, check := range plan.Verifications {
				if strings.HasPrefix(check.Detail, "pig pg ") {
					commands = append(commands, check.Detail)
				}
			}
			for _, action := range plan.NextActions {
				commands = append(commands, action.Command)
			}
		} else {
			var result output.Result
			if err := json.Unmarshal([]byte(stdout), &result); err != nil || runErr == nil || result.Code != output.CodePgConfirmationRequired {
				t.Fatalf("confirmation: %v / %v: %s", runErr, err, stdout)
			}
			for _, action := range result.NextActions {
				commands = append(commands, action.Command)
			}
		}
		for _, command := range commands {
			if !strings.HasPrefix(command, "pig pg ") {
				continue
			}
			// Only shell argument expansion runs; the generated pig command is not executed.
			raw, err := exec.Command("sh", "-c", "set -- "+command+`; printf '%s\000' "$@"`).Output()
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
			for _, flag := range []string{"-D", "--dbsu", "--version"} {
				wantAt, gotAt := slices.Index(args, flag), slices.Index(got, flag)
				if wantAt < 0 {
					if gotAt >= 0 {
						t.Errorf("omitted %s added to %q", flag, command)
					}
				} else if gotAt < 0 || gotAt+1 >= len(got) || got[gotAt+1] != args[wantAt+1] {
					t.Errorf("%s lost in %q: %v", flag, command, got)
				}
			}
			if len(got) > 2 && got[2] == args[1] {
				for _, flag := range []string{"--timeout", "--no-wait", "--full", "--all", "--jobs"} {
					if at := slices.Index(args, flag); at >= 0 {
						idx := slices.Index(got, flag)
						if idx < 0 || ((flag == "--timeout" || flag == "--jobs") && (idx+1 >= len(got) || got[idx+1] != args[at+1])) {
							t.Errorf("%s lost in %q", flag, command)
						}
					}
				}
			}
		}
		return
	}
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", "/pg/data", filepath.Join(t.TempDir(), "pg data 'quoted' $(printf marker)")} {
		for _, operation := range [][]string{
			{"promote", "--timeout", "17", "--no-wait"},
			{"vacuum", "app", "--full", "--all"},
			{"repack", "app", "--jobs", "3", "--all"},
		} {
			for _, plan := range []bool{true, false} {
				if !plan && operation[0] == "repack" {
					continue
				}
				args := append([]string{"pg"}, operation...)
				args = append(args, "--dbsu", account.Username, "--version", "18", "-o", "json")
				if target != "" {
					args = append(args, "-D", target)
				}
				if plan {
					args = append(args, "--plan")
				}
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					raw, _ := json.Marshal(args)
					child := exec.Command(os.Args[0], "-test.run=^TestPgPlanAndConfirmationPreserveTarget$")
					child.Env = append(os.Environ(), "PIG_TEST_TARGET_PLAN_ARGS="+string(raw))
					if out, err := child.CombinedOutput(); err != nil {
						t.Fatalf("%v\n%s", err, out)
					}
				})
			}
		}
	}
}
