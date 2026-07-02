package cmd

// Guard tests (T1/T2/T11) enforcing docs/refactor/ops_cli_redesign_2026-07-02.md
// house rules over the pg/pb/pt/pitr command surface.

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// opsTopCommands scopes the guards to the ops surface under redesign.
var opsTopCommands = []string{"postgres", "pgbackrest", "patroni", "pitr"}

func opsCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	var out []*cobra.Command
	for _, name := range opsTopCommands {
		found := false
		for _, c := range rootCmd.Commands() {
			if c.Name() == name {
				out = append(out, c)
				found = true
			}
		}
		if !found {
			t.Fatalf("top-level command %q not registered", name)
		}
	}
	return out
}

// TestNoDuplicateSiblingAliases (T1): under any parent, no two children may
// share a name or alias — Cobra resolves duplicates silently by registration
// order, which is how `pt l` and `pb l` once meant opposite things (B02).
func TestNoDuplicateSiblingAliases(t *testing.T) {
	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		seen := map[string]string{}
		for _, sub := range c.Commands() {
			keys := append([]string{sub.Name()}, sub.Aliases...)
			for _, k := range keys {
				if prev, dup := seen[k]; dup {
					t.Errorf("%s: token %q claimed by both %q and %q", path, k, prev, sub.Name())
				} else {
					seen[k] = sub.Name()
				}
			}
		}
		for _, sub := range c.Commands() {
			walk(sub, path+" "+sub.Name())
		}
	}
	walk(rootCmd, "pig")
}

// TestNoCrossLayerAliasCollision (T1): a subcommand alias must never equal a
// different top-level command's name (the retired `pb restore` alias "pitr"
// put the unmanaged primitive one token away from the orchestrator, B01).
func TestNoCrossLayerAliasCollision(t *testing.T) {
	topNames := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		topNames[c.Name()] = true
	}
	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		for _, sub := range c.Commands() {
			for _, alias := range sub.Aliases {
				if topNames[alias] {
					t.Errorf("%s %s: alias %q shadows top-level command %q", path, sub.Name(), alias, alias)
				}
			}
			walk(sub, path+" "+sub.Name())
		}
	}
	for _, c := range opsCommands(t) {
		walk(c, c.Name())
	}
}

// visitOwnShorthands visits flags declared BY cmd itself (local + persistent),
// without calling LocalFlags()/InheritedFlags() — those merge parent persistent
// flags into cmd.Flags() as a side effect and would poison sibling tests.
// seen dedupes by *pflag.Flag pointer so inherited/merged flags count only at
// their defining command.
func visitOwnShorthands(c *cobra.Command, seen map[*pflag.Flag]bool, fn func(f *pflag.Flag)) {
	visit := func(f *pflag.Flag) {
		if f.Shorthand == "" || seen[f] {
			return
		}
		seen[f] = true
		fn(f)
	}
	c.Flags().VisitAll(visit)
	c.PersistentFlags().VisitAll(visit)
}

// shorthandPermitTable (T2 / Appendix A): every short flag on the ops surface
// must appear here. Adding or changing a shorthand REQUIRES adjudication under
// the R3 conflict rule (cross-family reuse only when misuse fails validation
// before any side effect) and a matching Appendix A + breaking-change entry.
var shorthandPermitTable = map[string]string{
	"patroni -U":             "dbsu",
	"patroni failover -y":    "yes",
	"patroni list -W":        "watch",
	"patroni list -w":        "interval",
	"patroni log -f":         "follow",
	"patroni log -n":         "lines",
	"patroni log show -n":    "lines",
	"patroni log tail -f":    "follow",
	"patroni log tail -n":    "lines",
	"patroni reinit -y":      "yes",
	"patroni restart -p":     "pending",
	"patroni restart -r":     "role",
	"patroni restart -y":     "yes",
	"patroni switchover -y":  "yes",
	"pgbackrest -U":          "dbsu",
	"pgbackrest -c":          "config",
	"pgbackrest -r":          "repo",
	"pgbackrest -s":          "stanza",
	"pgbackrest backup -f":   "force",
	"pgbackrest create -f":   "force",
	"pgbackrest delete -y":   "yes",
	"pgbackrest expire -y":   "yes",
	"pgbackrest info -R":     "raw",
	"pgbackrest log -f":      "follow",
	"pgbackrest log -n":      "lines",
	"pgbackrest log show -n": "lines",
	"pgbackrest log tail -f": "follow",
	"pgbackrest log tail -n": "lines",
	"pgbackrest restore -D":  "data",
	"pgbackrest restore -I":  "immediate",
	"pgbackrest restore -T":  "target-timeline",
	"pgbackrest restore -X":  "exclusive",
	"pgbackrest restore -b":  "set",
	"pgbackrest restore -d":  "default",
	"pgbackrest restore -t":  "time",
	"pgbackrest restore -y":  "yes",
	"pgbackrest stop -f":     "force",
	"pitr -D":                "data",
	"pitr -I":                "immediate",
	"pitr -T":                "target-timeline",
	"pitr -U":                "dbsu",
	"pitr -X":                "exclusive",
	"pitr -b":                "set",
	"pitr -c":                "config",
	"pitr -d":                "default",
	"pitr -r":                "repo",
	"pitr -s":                "stanza",
	"pitr -t":                "time",
	"pitr -y":                "yes",
	"postgres -D":            "data",
	"postgres -U":            "dbsu",
	"postgres -v":            "version",
	"postgres analyze -V":    "verbose",
	"postgres analyze -a":    "all",
	"postgres analyze -t":    "table",
	"postgres clone -y":      "yes",
	"postgres fork -f":       "force",
	"postgres fork -r":       "run",
	"postgres fork -s":       "start",
	"postgres fork -t":       "timeout",
	"postgres fork -y":       "yes",
	"postgres fork init -f":  "force",
	"postgres fork init -r":  "run",
	"postgres fork init -s":  "start",
	"postgres fork init -t":  "timeout",
	"postgres fork rm -f":    "force",
	"postgres fork rm -m":    "mode",
	"postgres fork rm -t":    "timeout",
	"postgres fork start -t": "timeout",
	"postgres fork stop -m":  "mode",
	"postgres fork stop -t":  "timeout",
	"postgres freeze -V":     "verbose",
	"postgres freeze -a":     "all",
	"postgres freeze -t":     "table",
	"postgres init -E":       "encoding",
	"postgres init -f":       "force",
	"postgres init -k":       "data-checksum",
	"postgres init -y":       "yes",
	"postgres kill -a":       "all",
	"postgres kill -c":       "cancel",
	"postgres kill -d":       "database",
	"postgres kill -q":       "query",
	"postgres kill -s":       "state",
	"postgres kill -u":       "user",
	"postgres kill -x":       "execute",
	"postgres log -f":        "follow",
	"postgres log -n":        "lines",
	"postgres log grep -C":   "context",
	"postgres log tail -f":   "follow",
	"postgres promote -t":    "timeout",
	"postgres promote -y":    "yes",
	"postgres ps -a":         "all",
	"postgres ps -d":         "database",
	"postgres ps -u":         "user",
	"postgres psql -c":       "command",
	"postgres psql -f":       "file",
	"postgres repack -V":     "verbose",
	"postgres repack -a":     "all",
	"postgres repack -j":     "jobs",
	"postgres repack -t":     "table",
	"postgres restart -O":    "options",
	"postgres restart -m":    "mode",
	"postgres restart -t":    "timeout",
	"postgres role -V":       "verbose",
	"postgres start -O":      "options",
	"postgres start -l":      "log",
	"postgres start -t":      "timeout",
	"postgres stop -m":       "mode",
	"postgres stop -t":       "timeout",
	"postgres tune -C":       "max-conn",
	"postgres tune -R":       "shmem-ratio",
	"postgres tune -c":       "cpu",
	"postgres tune -d":       "disk",
	"postgres tune -m":       "mem",
	"postgres tune -p":       "profile",
	"postgres vacuum -F":     "full",
	"postgres vacuum -V":     "verbose",
	"postgres vacuum -a":     "all",
	"postgres vacuum -t":     "table",
	"postgres vacuum -y":     "yes",
}

func TestShorthandGrammar(t *testing.T) {
	// root persistent shorthands may not be redefined by any subcommand
	rootReserved := map[string]bool{"o": true, "i": true, "H": true}

	// flags inherited from root (merged into child Flags() by earlier cobra
	// operations) are identified by pointer and skipped
	rootOwned := map[*pflag.Flag]bool{}
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { rootOwned[f] = true })

	seen := map[*pflag.Flag]bool{}
	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		visitOwnShorthands(c, seen, func(f *pflag.Flag) {
			if rootOwned[f] {
				return // inherited root persistent flag, not a local definition
			}
			if rootReserved[f.Shorthand] {
				t.Errorf("%s: -%s (--%s) redefines a root persistent shorthand", path, f.Shorthand, f.Name)
			}
			key := path + " -" + f.Shorthand
			long, ok := shorthandPermitTable[key]
			if !ok {
				t.Errorf("unadjudicated shorthand %s (--%s): add it to shorthandPermitTable and Appendix A after R3 review", key, f.Name)
				return
			}
			if long != f.Name {
				t.Errorf("%s: -%s bound to --%s, permit table says --%s", path, f.Shorthand, f.Name, long)
			}
		})
		for _, sub := range c.Commands() {
			walk(sub, path+" "+sub.Name())
		}
	}
	for _, c := range opsCommands(t) {
		walk(c, c.Name())
	}
}

// TestT2GateFlagLint (T11 initial): every destructive (T2) command must expose
// its confirmation-skip flag with usage text that says so. Gate flags marked
// --force flip to --yes with B04/B05.
func TestT2GateFlagLint(t *testing.T) {
	gates := []struct {
		path []string
		flag string
	}{
		{[]string{"postgres", "init"}, "yes"},
		{[]string{"postgres", "promote"}, "yes"},
		{[]string{"postgres", "vacuum"}, "yes"},
		{[]string{"postgres", "clone"}, "yes"},
		{[]string{"postgres", "fork"}, "yes"},
		{[]string{"pgbackrest", "restore"}, "yes"},
		{[]string{"pgbackrest", "expire"}, "yes"},
		{[]string{"pgbackrest", "delete"}, "yes"}, // B05: --yes is the gate
		{[]string{"pitr"}, "yes"},
		{[]string{"patroni", "restart"}, "yes"}, // B04: pig owns confirmation
		{[]string{"patroni", "reinit"}, "yes"},
		{[]string{"patroni", "switchover"}, "yes"},
		{[]string{"patroni", "failover"}, "yes"},
	}
	for _, g := range gates {
		cmd, _, err := rootCmd.Find(g.path)
		if err != nil || cmd == nil {
			t.Errorf("T2 command %v not found: %v", g.path, err)
			continue
		}
		flag := cmd.Flags().Lookup(g.flag)
		if flag == nil {
			flag = cmd.PersistentFlags().Lookup(g.flag)
		}
		if flag == nil {
			t.Errorf("%v: T2 gate flag --%s missing", g.path, g.flag)
			continue
		}
		if !strings.Contains(strings.ToLower(flag.Usage), "confirm") {
			t.Errorf("%v: --%s usage %q must mention confirmation", g.path, g.flag, flag.Usage)
		}
	}
}
