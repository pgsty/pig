package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"pig/cli/build"
	"pig/internal/config"

	"github.com/spf13/cobra"
)

func TestBuildPgrxDefaultVersion(t *testing.T) {
	const want = "0.19.3"
	if build.DefaultPgrxVersion != want {
		t.Fatalf("default pgrx version = %q, want %q", build.DefaultPgrxVersion, want)
	}
	flag := buildPgrxCmd.PersistentFlags().Lookup("pgrx")
	if flag == nil {
		t.Fatal("pig build pgrx missing --pgrx flag")
	}
	if flag.DefValue != want {
		t.Fatalf("pig build pgrx --pgrx default = %q, want %q", flag.DefValue, want)
	}
}

func TestBuildRustMirrorFlagVisible(t *testing.T) {
	flag := buildRustCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatalf("pig build rust missing --mirror flag")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("pig build rust --mirror shorthand = %q, want m", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatalf("pig build rust --mirror should be visible")
	}
}

func TestBuildBetaFlagsVisible(t *testing.T) {
	for _, tt := range []struct {
		name      string
		cmd       *cobra.Command
		shorthand string
	}{
		{name: "pig build repo", cmd: buildRepoCmd, shorthand: "b"},
		{name: "pig build tool", cmd: buildToolCmd, shorthand: "b"},
		{name: "pig build pgrx", cmd: buildPgrxCmd, shorthand: "b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flag := tt.cmd.Flags().Lookup("beta")
			if flag == nil {
				t.Fatalf("%s missing --beta flag", tt.name)
			}
			if flag.Hidden {
				t.Fatalf("%s --beta should be visible", tt.name)
			}
			if flag.Shorthand != tt.shorthand {
				t.Fatalf("%s --beta shorthand = %q, want %q", tt.name, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestBuildDebugPackageFlags(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "pig build ext", cmd: buildExtCmd},
		{name: "pig build pkg", cmd: buildPkgCmd},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nodbg := tt.cmd.Flags().Lookup("nodbg")
			if nodbg == nil {
				t.Fatalf("%s missing --nodbg flag", tt.name)
			}
			if nodbg.DefValue != "false" || nodbg.Hidden {
				t.Fatalf("%s --nodbg default=%q hidden=%v, want false and visible", tt.name, nodbg.DefValue, nodbg.Hidden)
			}

			symbol := tt.cmd.Flags().Lookup("symbol")
			if symbol == nil {
				t.Fatalf("%s should retain --symbol for command-line compatibility", tt.name)
			}
			if symbol.Shorthand != "s" || symbol.Deprecated == "" || !symbol.Hidden {
				t.Fatalf("%s legacy --symbol shorthand=%q deprecated=%q hidden=%v", tt.name, symbol.Shorthand, symbol.Deprecated, symbol.Hidden)
			}
		})
	}
}

func TestBuildRepoModulesAddsBeta(t *testing.T) {
	tests := []struct {
		name string
		args []string
		beta bool
		want []string
	}{
		{name: "default", args: nil, beta: false, want: nil},
		{name: "default beta", args: nil, beta: true, want: []string{"all", "beta"}},
		{name: "explicit beta", args: []string{"node,pgsql"}, beta: true, want: []string{"node,pgsql", "beta"}},
		{name: "deduplicate beta", args: []string{"all", "beta"}, beta: true, want: []string{"all", "beta"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRepoModules(tt.args, tt.beta)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildRepoModules(%v, %v) = %v, want %v", tt.args, tt.beta, got, tt.want)
			}
		})
	}
}

func TestBuildProxyMachineContractMatchesSideEffects(t *testing.T) {
	if buildProxyCmd.Use != "proxy [id@remote] [local]" {
		t.Fatalf("build proxy use = %q, want optional operands", buildProxyCmd.Use)
	}
	want := map[string]string{
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"os_user":    "root",
	}
	for key, expected := range want {
		if got := buildProxyCmd.Annotations[key]; got != expected {
			t.Errorf("build proxy annotation %s = %q, want %q", key, got, expected)
		}
	}
}

func TestBuildProxyStructuredOutputRedactsCredential(t *testing.T) {
	original := config.OutputFormat
	config.OutputFormat = config.OUTPUT_JSON
	t.Cleanup(func() { config.OutputFormat = original })

	const credential = "SECRET-UUID-abcd-1234"
	var runErr error
	raw := captureStdout(t, func() {
		runErr = buildProxyCmd.RunE(buildProxyCmd, []string{credential})
	})
	if runErr == nil {
		t.Fatal("malformed proxy target unexpectedly succeeded")
	}
	if strings.Contains(raw, credential) {
		t.Fatalf("structured proxy result leaked credential: %s", raw)
	}
	var payload struct {
		Data legacyCommandData `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode structured proxy result: %v\n%s", err, raw)
	}
	if !reflect.DeepEqual(payload.Data.Args, []string{"<redacted>"}) {
		t.Fatalf("structured proxy args = %#v, want explicit redaction", payload.Data.Args)
	}
}
