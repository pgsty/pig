package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRepositoryHasNoLocalDocumentationTree(t *testing.T) {
	if _, err := os.Stat("docs"); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			t.Fatalf("stat local docs tree: %v", err)
		}
		t.Fatal("local docs tree exists; canonical documentation belongs at https://pig.pgsty.com/")
	}
}

func TestClaudeGuidanceIsRelativeSymlinkToAgents(t *testing.T) {
	info, err := os.Lstat("CLAUDE.md")
	if err != nil {
		t.Fatalf("lstat CLAUDE.md: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("CLAUDE.md must be a symbolic link to AGENTS.md, not duplicated guidance")
	}
	target, err := os.Readlink("CLAUDE.md")
	if err != nil {
		t.Fatalf("read CLAUDE.md symlink: %v", err)
	}
	if target != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink target = %q, want %q", target, "AGENTS.md")
	}

	raw, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	guidance := string(raw)
	for _, required := range []string{
		"https://pig.pgsty.com/docs/",
		"https://pig.pgsty.com/design/",
		"https://pig.pgsty.com/release/",
		"## Companion Documentation",
	} {
		if !strings.Contains(guidance, required) {
			t.Fatalf("AGENTS.md/CLAUDE.md is missing required guidance %q", required)
		}
	}
	for _, forbidden := range []string{
		strings.Join([]string{"docs", "spec", ""}, "/"),
		strings.Join([]string{"Command", "Specs:", "docs"}, " "),
		strings.Join([]string{"Designing", "Docs:", "docs"}, " "),
	} {
		if strings.Contains(guidance, forbidden) {
			t.Fatalf("CLAUDE.md restores retired local documentation reference %q", forbidden)
		}
	}
}

func TestGitignoreDoesNotHideDocumentation(t *testing.T) {
	raw, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	forbidden := map[string]bool{
		"CLAUDE.md":                                      true,
		strings.Join([]string{"docs", "*"}, "/"):         true,
		strings.Join([]string{"docs", "spec", "*"}, "/"): true,
		"docs/":   true,
		"docs/**": true,
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if forbidden[strings.TrimSpace(line)] {
			t.Fatalf(".gitignore hides canonical-guidance drift with pattern %q", line)
		}
	}
}

func TestDocumentationTargetsUseCompanionRepository(t *testing.T) {
	raw, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(raw)
	for _, required := range []string{
		"DOCS_DIR?=../pig.pgsty.com",
		"docs-check: docs-guard",
		"$(MAKE) -C \"$(DOCS_DIR)\" check",
	} {
		if !strings.Contains(makefile, required) {
			t.Fatalf("Makefile is missing companion-documentation target %q", required)
		}
	}
	for _, forbidden := range []string{"docs-serve:\n\thugo serve", "docs-build:\n\thugo --minify"} {
		if strings.Contains(makefile, forbidden) {
			t.Fatalf("Makefile restored source-local Hugo target %q", forbidden)
		}
	}
}
