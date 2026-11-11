package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

const extRmSafetyProcessEnv = "PIG_TEST_EXT_RM_SAFETY_CASE"

type extRmSafetyInvocation struct {
	Distro string
	Dir    string
	Args   []string
}

// Execute in a child process to verify real exit codes and keep Cobra flags,
// output settings, catalog state, and PostgreSQL discovery isolated per case.
func TestExtRmSafetyCommandProcess(t *testing.T) {
	raw := os.Getenv(extRmSafetyProcessEnv)
	if raw == "" {
		return
	}
	var invocation extRmSafetyInvocation
	if err := json.Unmarshal([]byte(raw), &invocation); err != nil {
		t.Fatal(err)
	}

	// PATH contains only recording fakes, with no pg_config. Disable fixed
	// discovery paths as well so no installed PostgreSQL files are inspected.
	ext.PostgresElSearchPath = nil
	ext.PostgresDEBSearchPath = nil
	ext.PostgresMACSearchPath = nil
	extCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		config.ConfigDir = invocation.Dir
		config.OSType = invocation.Distro
		config.OSArch = "amd64"
		config.CurrentUser = "pig-ext-rm-test"
		if invocation.Distro == config.DistroEL {
			config.OSCode, config.OSVersion = "el9", "9"
		} else {
			config.OSCode, config.OSVersion = "d12", "12"
		}
		if err := initOutputFormat(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		return ext.ReloadCatalog()
	}
	os.Args = append([]string{"pig"}, invocation.Args...)
	Execute()
	os.Exit(0) // Do not append the Go test runner's PASS line to command stdout.
}

func runExtRmSafetyCommand(t *testing.T, distro string, args ...string) (string, string, int) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "package-manager-calls")
	script := []byte("#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$PIG_TEST_EXT_RM_CALLS\"\nexit 99\n")
	for _, name := range []string{"apt", "apt-get", "dnf", "yum", "sudo", "rpm", "dpkg"} {
		if err := os.WriteFile(filepath.Join(bin, name), script, 0700); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(extRmSafetyInvocation{Distro: distro, Dir: dir, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestExtRmSafetyCommandProcess$")
	child.Dir = dir
	child.Env = append(os.Environ(),
		extRmSafetyProcessEnv+"="+string(raw),
		"PATH="+bin,
		"PIG_TEST_EXT_RM_CALLS="+calls,
		"PIG_NO_SUDO=0",
		"PIG_NON_INTERACTIVE=1",
		"NO_COLOR=1",
	)
	var stdout, stderr bytes.Buffer
	child.Stdout, child.Stderr = &stdout, &stderr
	err = child.Run()
	if recorded, readErr := os.ReadFile(calls); readErr == nil {
		t.Fatalf("unexpected sudo/package-manager invocation: %s", recorded)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("check package-manager calls: %v", readErr)
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run command: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), exitCode
}

func decodeExtRmSafetyOutput(t *testing.T, format, stdout string, value any) {
	t.Helper()
	var decoder interface{ Decode(any) error }
	if format == "yaml" {
		decoder = yaml.NewDecoder(strings.NewReader(stdout))
	} else {
		decoder = json.NewDecoder(strings.NewReader(stdout))
	}
	if err := decoder.Decode(value); err != nil {
		t.Fatalf("decode %s: %v; stdout=%s", format, err, stdout)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("stdout must contain exactly one %s value, trailing decode=%v; stdout=%s", format, err, stdout)
	}
}

func assertExtRmSafetyBlocked(t *testing.T, distro, format string, plan bool, args []string, want ...string) {
	t.Helper()
	command := append([]string{"ext", "rm", "-o", format}, args...)
	if plan {
		command = append(command, "--plan")
	}
	stdout, stderr, exitCode := runExtRmSafetyCommand(t, distro, command...)
	if exitCode != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	for _, detail := range want {
		if !strings.Contains(stdout, detail) {
			t.Errorf("missing rejection detail %q in stdout: %s", detail, stdout)
		}
	}
	if (plan || format != "text") && stderr != "" {
		t.Errorf("duplicate diagnostics on stderr: %s", stderr)
	}
	if format == "text" {
		if plan {
			if strings.Count(stdout, "Execution Plan") != 1 || !strings.Contains(stdout, "Preconditions:") || !strings.Contains(stdout, "failed") {
				t.Fatalf("missing blocked text plan: %s", stdout)
			}
			if strings.Contains(stdout, "\nActions:") || strings.Contains(stdout, "Code: 100102") {
				t.Fatalf("blocked plan contains actions or a second error result: %s", stdout)
			}
		} else if !strings.Contains(stdout, "Code: 100102") {
			t.Fatalf("missing protected-package error code: %s", stdout)
		}
		return
	}
	if plan {
		var got output.Plan
		decodeExtRmSafetyOutput(t, format, stdout, &got)
		if got.API != output.APIVersion || !strings.HasPrefix(got.Command, "pig ext rm ") || len(got.Actions) != 0 {
			t.Fatalf("invalid blocked plan: %+v", got)
		}
		for _, check := range got.Preconditions {
			if check.Status == "failed" {
				return
			}
		}
		t.Fatalf("blocked plan has no failed precondition: %+v", got)
	}
	var result output.Result
	decodeExtRmSafetyOutput(t, format, stdout, &result)
	if result.Success || result.Code != output.CodeExtensionProtectedPackage || result.ExitCode() != 2 {
		t.Fatalf("wrong protected-package result: %+v", result)
	}
}

func TestExtRmSafetyBlocked(t *testing.T) {
	for _, distro := range []string{config.DistroDEB, config.DistroEL} {
		pkg, alias := "postgresql-18", "pg18-server"
		if distro == config.DistroEL {
			pkg, alias = "postgresql18-contrib", "pg18-contrib"
		}
		for _, format := range []string{"text", "json", "yaml"} {
			for _, plan := range []bool{false, true} {
				for _, yes := range []string{"", "-y", "--yes"} {
					t.Run(fmt.Sprintf("%s/%s/plan=%t/yes=%s", distro, format, plan, yes), func(t *testing.T) {
						args := []string{"pg_trgm", "-v", "18"}
						if yes != "" {
							args = append(args, yes)
						}
						assertExtRmSafetyBlocked(t, distro, format, plan, args, "pg_trgm", pkg, alias)
					})
				}
			}
		}
	}
}

func TestExtRmSafetyProtectedOperands(t *testing.T) {
	for _, distro := range []string{config.DistroDEB, config.DistroEL} {
		for _, test := range []struct {
			name string
			args []string
			want []string
		}{
			{"server extension", []string{"plpgsql", "-v", "18"}, []string{"plpgsql", "pg18-server"}},
			{"vendor extension", []string{"spock", "-v", "18"}, []string{"spock", "pgedge-18", "pgedge"}},
			{"OrioleDB collision", []string{"orioledb", "-v", "17"}, []string{"orioledb", "orioledb-17", "orioledb-server"}},
			{"alias first", []string{"pg18-server", "pg_trgm", "-v", "18"}, []string{"pg_trgm"}},
			{"alias last", []string{"pg_trgm", "pg18-server", "-v", "18"}, []string{"pg_trgm"}},
		} {
			for _, plan := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/plan=%t", distro, test.name, plan), func(t *testing.T) {
					args := append(append([]string{}, test.args...), "-y")
					assertExtRmSafetyBlocked(t, distro, "json", plan, args, test.want...)
				})
			}
		}
	}
}

func TestExtRmSafetyDebianContribAlias(t *testing.T) {
	for _, operands := range [][]string{
		{"pg18-contrib"},
		{"pg18-contrib", "vector"},
		{"vector", "pg18-contrib"},
	} {
		for _, format := range []string{"text", "json", "yaml"} {
			for _, plan := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/plan=%t", strings.Join(operands, "+"), format, plan), func(t *testing.T) {
					args := append(append([]string{}, operands...), "-v", "18", "-y")
					assertExtRmSafetyBlocked(t, config.DistroDEB, format, plan, args, "pg18-contrib")
				})
			}
		}
	}
}

func TestExtRmSafetyExplicitAliasPlans(t *testing.T) {
	for _, test := range []struct {
		name     string
		distro   string
		args     []string
		packages []string
	}{
		{"Debian server", config.DistroDEB, []string{"pg18-server", "-v", "17"}, []string{"postgresql-18"}},
		{"EL server", config.DistroEL, []string{"pg18-server", "-v", "17"}, []string{"postgresql18-server", "postgresql18-libs", "postgresql18-contrib"}},
		{"EL contrib", config.DistroEL, []string{"pg18-contrib", "-v", "18"}, []string{"postgresql18-contrib"}},
		{"Debian OrioleDB", config.DistroDEB, []string{"orioledb-server", "-v", "17"}, []string{"orioledb-17"}},
		{"EL OrioleDB", config.DistroEL, []string{"orioledb-server", "-v", "17"}, []string{"orioledb-17"}},
	} {
		for _, format := range []string{"text", "json", "yaml"} {
			t.Run(test.name+"/"+format, func(t *testing.T) {
				args := append([]string{"ext", "rm", "--plan", "-o", format}, test.args...)
				stdout, stderr, exitCode := runExtRmSafetyCommand(t, test.distro, args...)
				if exitCode != 0 || stderr != "" {
					t.Fatalf("exit=%d; stdout=%s stderr=%s", exitCode, stdout, stderr)
				}
				if format == "text" {
					if strings.Count(stdout, "Execution Plan") != 1 || !strings.Contains(stdout, "\nActions:") {
						t.Fatalf("missing executable alias plan: %s", stdout)
					}
					for _, pkg := range test.packages {
						if !strings.Contains(stdout, pkg) {
							t.Errorf("package %s missing from plan: %s", pkg, stdout)
						}
					}
					return
				}
				var plan output.Plan
				decodeExtRmSafetyOutput(t, format, stdout, &plan)
				if len(plan.Actions) == 0 {
					t.Fatalf("alias plan has no actions: %+v", plan)
				}
				for _, check := range plan.Preconditions {
					if check.Status == "failed" {
						t.Errorf("explicit alias was blocked: %+v", check)
					}
				}
				var packages []string
				for _, resource := range plan.Affects {
					if resource.Type == "package" {
						packages = append(packages, resource.Name)
					}
				}
				want := slices.Clone(test.packages)
				slices.Sort(packages)
				slices.Sort(want)
				if !slices.Equal(packages, want) {
					t.Fatalf("planned packages=%v, want %v", packages, want)
				}
			})
		}
	}
}

func TestExtRmSafetyHelp(t *testing.T) {
	stdout, stderr, exitCode := runExtRmSafetyCommand(t, config.DistroEL, "ext", "rm", "--help")
	if exitCode != 0 || stderr != "" {
		t.Fatalf("help exit=%d; stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	for _, detail := range []string{"explicit", "--yes", "does not bypass", "pg18-contrib", "EL only", "orioledb-server"} {
		if !strings.Contains(stdout, detail) {
			t.Errorf("help missing %q: %s", detail, stdout)
		}
	}
}
