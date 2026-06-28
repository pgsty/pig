package cmd

import (
	"strings"
	"testing"
)

func TestPgCloneCommandIsRegistered(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	if pgClone == nil || pgClone.Name() != "clone" {
		t.Fatalf("pg clone command = %v, want clone", pgClone)
	}
}

func TestPgCloneAcceptsOptionalDestinationDatabase(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}

	for _, args := range [][]string{{"app"}, {"app", "app_1"}} {
		if err := pgClone.Args(pgClone, args); err != nil {
			t.Fatalf("pg clone Args(%v) returned error: %v", args, err)
		}
	}

	if err := pgClone.Args(pgClone, nil); err == nil {
		t.Fatal("pg clone should reject missing source database")
	}
	if err := pgClone.Args(pgClone, []string{"app", "app_1", "extra"}); err == nil {
		t.Fatal("pg clone should reject extra positional arguments")
	}
}

func TestPgCloneSupportsPlanAndDryRun(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	if pgClone.PersistentFlags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg clone command")
	}
	if pgClone.PersistentFlags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run alias not found on pg clone command")
	}
}

func TestPgCloneDoesNotExposeInstanceOnlyFlags(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	for _, name := range []string{"no-start", "replace", "mode", "no-kill", "strategy", "tablespace"} {
		if pgClone.Flags().Lookup(name) != nil {
			t.Fatalf("pg clone should not expose --%s", name)
		}
	}
}

func TestPgCloneExposesMinimalCloneFlags(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	for _, name := range []string{"owner", "conn-limit", "port", "conn-db"} {
		if pgClone.Flags().Lookup(name) == nil {
			t.Fatalf("pg clone should expose --%s", name)
		}
	}
}

func TestPgCloneConnLimitHelpMentionsUnlimited(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	flag := pgClone.Flags().Lookup("conn-limit")
	if flag == nil {
		t.Fatal("pg clone should expose --conn-limit")
	}
	if !strings.Contains(flag.Usage, "-1 = no limit") {
		t.Fatalf("--conn-limit usage = %q, want -1 semantics", flag.Usage)
	}
}
