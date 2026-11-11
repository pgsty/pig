package main

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Release artifacts are built by GoReleaser, whose ldflags override the source
// defaults, so every place that states a version has to move together.
func TestReleaseMetadataVersionsAgree(t *testing.T) {
	read := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(raw)
	}
	find := func(name, text, pattern string) string {
		match := regexp.MustCompile(pattern).FindStringSubmatch(text)
		if match == nil {
			t.Fatalf("%s not found with pattern %q", name, pattern)
		}
		return match[1]
	}
	makefile, source := read("Makefile"), read("internal/config/config.go")
	goreleaser, readme := read(".goreleaser.yml"), read("README.md")

	pig := find("Makefile VERSION", makefile, `(?m)^VERSION=v(\d+\.\d+\.\d+)$`)
	pigsty := find("Makefile PIGSTY_VERSION", makefile, `(?m)^PIGSTY_VERSION:=(\d+\.\d+\.\d+)$`)
	for _, check := range []struct{ name, got, want string }{
		{"config.go PigVersion", find("config.go PigVersion", source, `(?m)^\s*PigVersion\s*=\s*"([^"]+)"`), pig},
		{"config.go PigstyVersion", find("config.go PigstyVersion", source, `(?m)^\s*PigstyVersion\s*=\s*"([^"]+)"`), pigsty},
		{".goreleaser.yml PigstyVersion", find(".goreleaser.yml PigstyVersion", goreleaser, `config\.PigstyVersion=([0-9.]+)'`), pigsty},
		{"README version badge", find("README version badge", readme, `badge/version-v([0-9.]+)-`), pig},
		{"README Pigsty badge", find("README Pigsty badge", readme, `badge/Pigsty-v([0-9.]+)-`), pigsty},
	} {
		if check.got != check.want {
			t.Errorf("%s = %q, want %q (Makefile is the reference)", check.name, check.got, check.want)
		}
	}
}

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
