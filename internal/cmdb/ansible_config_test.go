package cmdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectAnsibleConfigStrictSourceAndRelativeBase(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	configDirectory := filepath.Join(directory, "project")
	if err := os.Mkdir(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDirectory, "ansible.cfg")
	body := []byte("# retain\r\n[DEFAULTS] ; one\r\nforks = 10\r\n  inventory\t=\t'../static inventory.yml'  # keep\r\n[ssh_connection]\r\npipelining = True\r\n")
	if err := os.WriteFile(path, body, 0o640); err != nil {
		t.Fatal(err)
	}

	source, err := InspectAnsibleConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if source.ConfigPath != path || source.Literal != "'../static inventory.yml'" ||
		source.Locator != filepath.Join(directory, "static inventory.yml") ||
		source.RevisionSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("source = %+v", source)
	}
	document, err := readAnsibleConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if document.mode.Perm() != 0o640 || document.ownerUID != uint32(os.Geteuid()) {
		t.Fatalf("config metadata = mode %v uid %d", document.mode.Perm(), document.ownerUID)
	}
}

func TestPlanAnsibleConfigPatchChangesOnlyValueSpanAndReparses(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, "ansible.cfg")
	body := []byte("# before\r\n[defaults]\r\n  inventory = old.yml  ; exact comment\r\nforks=5\r\n# after\r\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := readAnsibleConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "runtime dir", "inventory#native")
	patch, err := planAnsibleConfigPatch(document, target)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(body, []byte("old.yml"), []byte("'"+target+"'"), 1)
	if !bytes.Equal(patch.candidate, want) {
		t.Fatalf("candidate changed bytes outside value span\n got: %q\nwant: %q", patch.candidate, want)
	}
	if patch.nextLiteral != "'"+target+"'" || patch.nextLocator != target ||
		patch.afterDigest != sha256.Sum256(want) {
		t.Fatalf("patch metadata = literal %q locator %q digest %x", patch.nextLiteral, patch.nextLocator, patch.afterDigest)
	}
	if bytes.Count(patch.candidate, []byte("\r\n")) != bytes.Count(body, []byte("\r\n")) ||
		bytes.Contains(bytes.ReplaceAll(patch.candidate, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("CRLF style was not preserved")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, body) {
		t.Fatalf("planning mutated config: %v, %q", err, current)
	}
}

func TestAnsibleConfigRejectsAmbiguousAndDynamicGrammarWithoutMutation(t *testing.T) {
	tests := map[string][]byte{
		"missing defaults":     []byte("[other]\ninventory = static.yml\n"),
		"duplicate defaults":   []byte("[defaults]\ninventory=a.yml\n[DEFAULTS]\ninventory=b.yml\n"),
		"missing inventory":    []byte("[defaults]\nforks=5\n"),
		"duplicate inventory":  []byte("[defaults]\ninventory=a.yml\ninventory=b.yml\n"),
		"source list":          []byte("[defaults]\ninventory=a.yml,b.yml\n"),
		"shell variable":       []byte("[defaults]\ninventory=$HOME/a.yml\n"),
		"interpolation":        []byte("[defaults]\ninventory=%(here)s/a.yml\n"),
		"jinja":                []byte("[defaults]\ninventory={{ source }}\n"),
		"escape":               []byte("[defaults]\ninventory=a\\ b.yml\n"),
		"continuation":         []byte("[defaults]\ninventory=a.yml\\\n  b.yml\n"),
		"unclosed quote":       []byte("[defaults]\ninventory='a.yml\n"),
		"suffix":               []byte("[defaults]\ninventory=a.yml another.yml\n"),
		"bare carriage return": []byte("[defaults]\rinventory=a.yml\r"),
		"nul":                  []byte("[defaults]\ninventory=a\x00.yml\n"),
		"invalid utf8":         {0xff, '\n'},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			directory := symlinkFreeTempDir(t)
			path := filepath.Join(directory, "ansible.cfg")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := InspectAnsibleConfig(path)
			if !IsCode(err, "CMDB_ANSIBLE_CONFIG_INVALID") || err.Error() != "CMDB_ANSIBLE_CONFIG_INVALID: "+AnsibleConfigManualInstruction(path) {
				t.Fatalf("error = %v", err)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(after, body) {
				t.Fatalf("rejected config changed: %v, %q", readErr, after)
			}
		})
	}
}

func TestAnsibleConfigRejectsSymlinkDirectoryAndBound(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	valid := filepath.Join(directory, "valid.cfg")
	if err := os.WriteFile(valid, []byte("[defaults]\ninventory=a.yml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.cfg")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectAnsibleConfig(link); !IsCode(err, "CMDB_ANSIBLE_CONFIG_INVALID") {
		t.Fatalf("symlink error = %v", err)
	}
	if _, err := InspectAnsibleConfig(directory); !IsCode(err, "CMDB_ANSIBLE_CONFIG_INVALID") {
		t.Fatalf("directory error = %v", err)
	}
	large := filepath.Join(directory, "large.cfg")
	if err := os.WriteFile(large, bytes.Repeat([]byte{'x'}, maxAnsibleConfigSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectAnsibleConfig(large); !IsCode(err, "CMDB_ANSIBLE_CONFIG_INVALID") {
		t.Fatalf("large file error = %v", err)
	}
}

func TestRenderAnsibleInventoryLiteralIsUnambiguous(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	configPath := filepath.Join(directory, "ansible.cfg")
	plain := filepath.Join(directory, "inventory.yml")
	if got, err := renderAnsibleInventoryLiteral(configPath, plain); err != nil || got != plain {
		t.Fatalf("plain literal = %q, %v", got, err)
	}
	spaced := filepath.Join(directory, "static source.yml")
	if got, err := renderAnsibleInventoryLiteral(configPath, spaced); err != nil || got != "'"+spaced+"'" {
		t.Fatalf("spaced literal = %q, %v", got, err)
	}
	singleQuote := filepath.Join(directory, "source'one.yml")
	if got, err := renderAnsibleInventoryLiteral(configPath, singleQuote); err != nil || got != `"`+singleQuote+`"` {
		t.Fatalf("single-quote literal = %q, %v", got, err)
	}
	for _, invalid := range []string{
		filepath.Join(directory, "a,b.yml"),
		filepath.Join(directory, `a\\b.yml`),
		filepath.Join(directory, `a$b.yml`),
		filepath.Join(directory, `a'"b.yml`),
	} {
		if got, err := renderAnsibleInventoryLiteral(configPath, invalid); err == nil || got != "" {
			t.Fatalf("invalid target %q rendered as %q, %v", invalid, got, err)
		}
	}
	if _, err := renderAnsibleInventoryLiteral(configPath, "relative.yml"); err == nil {
		t.Fatal("relative replacement target was accepted")
	}
}

func TestAnsibleConfigManualInstructionIsExact(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, "ansible.cfg")
	want := "edit " + path + " manually so [defaults] contains exactly one literal inventory = <path> directive"
	if got := AnsibleConfigManualInstruction(path); got != want || strings.Count(got, "inventory = <path>") != 1 {
		t.Fatalf("manual instruction = %q", got)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("instruction unexpectedly touched config: %v", err)
	}
}
