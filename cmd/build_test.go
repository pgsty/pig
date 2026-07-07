package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

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
		name string
		cmd  *cobra.Command
	}{
		{name: "pig build repo", cmd: buildRepoCmd},
		{name: "pig build tool", cmd: buildToolCmd},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flag := tt.cmd.Flags().Lookup("beta")
			if flag == nil {
				t.Fatalf("%s missing --beta flag", tt.name)
			}
			if flag.Hidden {
				t.Fatalf("%s --beta should be visible", tt.name)
			}
			if flag.Shorthand != "" {
				t.Fatalf("%s --beta shorthand = %q, want none", tt.name, flag.Shorthand)
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
