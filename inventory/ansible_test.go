package inventory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateWithAnsibleDiscardsAndRedactsSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	doc, err := Parse("candidate.yml", []byte("all:\n  vars:\n    password: top-secret\n"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	success := filepath.Join(dir, "ansible-success")
	if err := os.WriteFile(success, []byte("#!/bin/sh\n[ \"$#\" = 3 ] && [ \"$1\" = -i ] && [ \"$3\" = --list ] || { echo 'unexpected arguments' >&2; exit 8; }\nmode=$(stat -c '%a' \"$2\" 2>/dev/null || stat -f '%Lp' \"$2\")\n[ \"$mode\" = 600 ] || { echo \"candidate mode $mode\" >&2; exit 9; }\necho 'password: leaked-on-stdout'\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if diagnostics := ValidateWithAnsible(context.Background(), success, doc); len(diagnostics) != 0 {
		t.Fatalf("success diagnostics = %+v", diagnostics)
	}

	failure := filepath.Join(dir, "ansible-failure")
	if err := os.WriteFile(failure, []byte("#!/bin/sh\necho 'password: leaked-on-stderr' >&2\necho \"failed at $2\" >&2\necho 'arbitrary-value: leaked-unlabeled' >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	diagnostics := ValidateWithAnsible(context.Background(), failure, doc)
	if len(diagnostics) != 1 || diagnostics[0].Code != "ANSIBLE_INVENTORY_REJECTED" {
		t.Fatalf("failure diagnostics = %+v", diagnostics)
	}
	message := diagnostics[0].Message
	if strings.Contains(message, "top-secret") || strings.Contains(message, "leaked-on-stderr") || strings.Contains(message, "leaked-unlabeled") || strings.Contains(message, dir) {
		t.Fatalf("diagnostic leaked a secret or private path: %q", message)
	}
	if !strings.Contains(message, "exit=7") || !strings.Contains(message, "<candidate>") {
		t.Fatalf("diagnostic lost safe failure context: %q", message)
	}
}

func TestValidateWithAnsibleMissingAndTimeout(t *testing.T) {
	doc, err := Parse("candidate.yml", []byte("all: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := ValidateWithAnsible(context.Background(), "", doc)
	if len(diagnostics) != 1 || diagnostics[0].Code != "ANSIBLE_NOT_FOUND" || diagnostics[0].Severity != SeverityWarning {
		t.Fatalf("missing executable diagnostics = %+v", diagnostics)
	}
	if runtime.GOOS == "windows" {
		return
	}
	script := filepath.Join(t.TempDir(), "ansible-timeout")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	diagnostics = ValidateWithAnsible(ctx, script, doc)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "exit=timeout") {
		t.Fatalf("timeout diagnostics = %+v", diagnostics)
	}
}

func TestPigstyConfCorpusWithAnsible(t *testing.T) {
	if os.Getenv("PIG_INVENTORY_CORPUS_ANSIBLE") != "1" {
		t.Skip("set PIG_INVENTORY_CORPUS_ANSIBLE=1 to run external corpus validation")
	}
	executable, err := exec.LookPath("ansible-inventory")
	if err != nil {
		t.Skip("ansible-inventory is not installed")
	}
	root := pigstyConfCorpus(t)
	for _, path := range pigstyValidationCorpusFiles(t, root) {
		doc, err := Load(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		diagnostics := ValidateWithAnsible(context.Background(), executable, doc)
		if HasErrors(diagnostics) {
			t.Errorf("%s: %+v", path, diagnostics)
		}

		vars, ok := doc.Lookup(AllVars())
		if !ok || vars.Kind != MappingValue || len(vars.Mapping) == 0 {
			t.Errorf("%s: all.vars has no insertion anchor", path)
			continue
		}
		anchor, ok := vars.Mapping[0].Key.Value.(string)
		if !ok || anchor == "" {
			t.Errorf("%s: first all.vars key is not a string", path)
			continue
		}
		editor := doc.Edit()
		if err := editor.Insert(AllVars(), "sty_inventory_roundtrip_probe", true, AfterKey(anchor)); err != nil {
			t.Errorf("%s: insert probe: %v", path, err)
			continue
		}
		candidate, _, err := editor.Commit()
		if err != nil {
			t.Errorf("%s: commit probe: %v", path, err)
			continue
		}
		diagnostics = ValidateWithAnsible(context.Background(), executable, candidate)
		if HasErrors(diagnostics) {
			t.Errorf("%s after styled insertion: %+v", path, diagnostics)
		}
	}
}
