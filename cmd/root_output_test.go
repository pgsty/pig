package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

func TestEarlyOutputFlagFormsAndBoundaries(t *testing.T) {
	for _, format := range []string{"json", "yaml", "json-pretty", "text"} {
		for _, flags := range [][]string{
			{"-o" + format}, {"-o=" + format}, {"-o", format},
			{"--output=" + format}, {"--output", format},
		} {
			for _, prefix := range [][]string{nil, {"pg", "init"}} {
				args := append(append([]string{}, prefix...), flags...)
				if got := outputFormatFromArgs(args); got != format {
					t.Errorf("outputFormatFromArgs(%q) = %q, want %q", args, got, format)
				}
				args = append(append([]string{}, prefix...), append([]string{"--"}, flags...)...)
				if got := outputFormatFromArgs(args); got != config.OUTPUT_TEXT {
					t.Errorf("native args %q selected output %q", args, got)
				}
			}
		}
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"-oJSON"}, "json"},
		{[]string{"-ojson", "--", "-otext"}, "json"},
		{[]string{"-otext", "-oyaml"}, "yaml"},
		{[]string{"-ojson", "--output=text"}, "text"},
		{[]string{"-ojson", "-o"}, "json"},
		{[]string{"-o"}, "text"},
		{[]string{"--output"}, "text"},
		{[]string{"-oxml"}, "text"},
		{[]string{"pt", "list", "-ojson"}, "text"},
		{[]string{"patroni", "--", "list", "-ojson"}, "text"},
		{[]string{"-oyaml", "pt", "list", "-ojson"}, "yaml"},
		{[]string{"pt", "-oyaml", "list", "-ojson"}, "yaml"},
		{[]string{"pt", "status", "-ojson"}, "json"},
	} {
		if got := outputFormatFromArgs(test.args); got != test.want {
			t.Errorf("outputFormatFromArgs(%q) = %q, want %q", test.args, got, test.want)
		}
	}
}

func TestOutputHelpOrderingRespectsBoundaries(t *testing.T) {
	for _, test := range []struct{ args, want []string }{
		{[]string{"--help", "-ojson"}, []string{"-ojson", "--help"}},
		{[]string{"pg", "--help", "-o=json"}, []string{"pg", "-o=json", "--help"}},
		{[]string{"--help", "--", "-ojson"}, []string{"--help", "--", "-ojson"}},
		{[]string{"--", "--help", "-ojson"}, []string{"--", "--help", "-ojson"}},
		{[]string{"pt", "list", "--help", "-ojson"}, []string{"pt", "list", "--help", "-ojson"}},
	} {
		if got := reorderOutputBeforeHelp(test.args); !slices.Equal(got, test.want) {
			t.Errorf("reorderOutputBeforeHelp(%q) = %q, want %q", test.args, got, test.want)
		}
	}
}

// Exercise Execute itself in a child process to verify the OS exit code and both
// streams. Parsing must fail before any configuration or operational work.
func TestEarlyOutputParseErrors(t *testing.T) {
	const env = "PIG_TEST_EARLY_OUTPUT_ARGS"
	if raw := os.Getenv(env); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatal(err)
		}
		var guard func(*cobra.Command)
		guard = func(command *cobra.Command) {
			command.PersistentPreRunE = func(*cobra.Command, []string) error {
				return errors.New("unexpected operational initialization")
			}
			for _, sub := range command.Commands() {
				guard(sub)
			}
		}
		guard(rootCmd)
		os.Args = append([]string{"pig"}, args...)
		Execute()
		t.Fatal("invalid invocation succeeded")
	}

	for _, format := range []string{"json", "yaml", "json-pretty"} {
		for _, flags := range [][]string{
			{"-o" + format}, {"-o=" + format}, {"-o", format},
			{"--output=" + format}, {"--output", format},
		} {
			for _, invalid := range [][]string{
				{"nonexistent-command"}, {"pg", "status", "--bad"},
				{"pg", "status", "-Z"}, {"pg", "status", "unexpected"},
				{"pg", "stop", "--timeout=invalid"}, {"pg", "stop", "--timeout"},
			} {
				args := append(append([]string{}, flags...), invalid...)
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					raw, err := json.Marshal(args)
					if err != nil {
						t.Fatal(err)
					}
					child := exec.Command(os.Args[0], "-test.run=^TestEarlyOutputParseErrors$")
					child.Env = append(os.Environ(), env+"="+string(raw))
					var stdout, stderr bytes.Buffer
					child.Stdout, child.Stderr = &stdout, &stderr
					var exitErr *exec.ExitError
					if err := child.Run(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
						t.Fatalf("exit = %v; stdout=%s stderr=%s", err, &stdout, &stderr)
					}
					var result output.Result
					if format == "yaml" {
						decoder := yaml.NewDecoder(&stdout)
						err = decoder.Decode(&result)
						if err == nil && decoder.Decode(new(any)) != io.EOF {
							t.Fatal("stdout contains more than one YAML value")
						}
					} else {
						err = json.Unmarshal(stdout.Bytes(), &result)
					}
					if err != nil || result.Success || result.Code != output.CodeSystemInvalidArgs {
						t.Fatalf("parse error envelope = %+v, decode=%v, stdout=%s", result, err, &stdout)
					}
					if stderr.Len() != 0 {
						t.Fatalf("duplicate error or usage on stderr: %s", &stderr)
					}
				})
			}
		}
	}
}
