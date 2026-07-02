package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestGenerateShorthandTable is a one-off generator (run manually) that dumps
// the current shorthand permit table for Appendix A and guard_test.go.
func TestGenerateShorthandTable(t *testing.T) {
	if os.Getenv("PIG_GEN_SHORTHANDS") == "" {
		t.Skip("set PIG_GEN_SHORTHANDS=1 to regenerate")
	}
	var lines []string
	rootOwned := map[*pflag.Flag]bool{}
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { rootOwned[f] = true })
	seen := map[*pflag.Flag]bool{}
	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		visitOwnShorthands(c, seen, func(f *pflag.Flag) {
			if !rootOwned[f] {
				lines = append(lines, fmt.Sprintf("%s\t-%s\t--%s", path, f.Shorthand, f.Name))
			}
		})
		for _, sub := range c.Commands() {
			walk(sub, path+" "+sub.Name())
		}
	}
	for _, name := range []string{"postgres", "pgbackrest", "patroni", "pitr"} {
		for _, c := range rootCmd.Commands() {
			if c.Name() == name {
				walk(c, name)
			}
		}
	}
	sort.Strings(lines)
	fmt.Println(strings.Join(lines, "\n"))
}
