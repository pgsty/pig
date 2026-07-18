package cmdb

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAnsibleConfigLocatesOneLiteralWithoutNormalizing(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	project := filepath.Join(directory, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "ansible.cfg")
	body := []byte("# retain\r\n[DEFAULTS] ; one\r\nforks = 10\r\n  inventory\t=\t'../static inventory.yml'  # keep\r\n")
	if err := os.WriteFile(path, body, 0o640); err != nil {
		t.Fatal(err)
	}
	document, err := readAnsibleConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if document.path != path || document.literal != "'../static inventory.yml'" ||
		document.locator != filepath.Join(directory, "static inventory.yml") || document.mode.Perm() != 0o640 {
		t.Fatalf("document = %+v", document)
	}
	if !bytes.Equal(document.body, body) {
		t.Fatal("read normalized ansible.cfg bytes")
	}
}

func TestAnsibleConfigPatchChangesOnlyLiteralAndRejectsStaleCommit(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, "ansible.cfg")
	body := []byte("# before\r\n[defaults]\r\n  inventory = old.yml  ; exact comment\r\nforks=5\r\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := readAnsibleConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "pigsty.yml")
	patch, err := planAnsibleConfigLiteralPatch(document, "pigsty.yml", target)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(body, []byte("old.yml"), []byte("pigsty.yml"), 1)
	if !bytes.Equal(patch.candidate, want) {
		t.Fatalf("candidate changed bytes outside literal\n got: %q\nwant: %q", patch.candidate, want)
	}
	changed := bytes.Replace(body, []byte("forks=5"), []byte("forks=6"), 1)
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitAnsibleConfigPatch(patch); !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("stale commit error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, changed) {
		t.Fatalf("stale commit clobbered source: %v, %q", err, after)
	}
}

func TestAnsibleConfigRejectsAmbiguousOrDynamicGrammar(t *testing.T) {
	tests := map[string][]byte{
		"missing defaults":    []byte("[other]\ninventory = static.yml\n"),
		"duplicate defaults":  []byte("[defaults]\ninventory=a.yml\n[DEFAULTS]\ninventory=b.yml\n"),
		"missing inventory":   []byte("[defaults]\nforks=5\n"),
		"duplicate inventory": []byte("[defaults]\ninventory=a.yml\ninventory=b.yml\n"),
		"source list":         []byte("[defaults]\ninventory=a.yml,b.yml\n"),
		"interpolation":       []byte("[defaults]\ninventory=%(here)s/a.yml\n"),
		"jinja":               []byte("[defaults]\ninventory={{ source }}\n"),
		"continuation":        []byte("[defaults]\ninventory=a.yml\\\n  b.yml\n"),
		"unclosed quote":      []byte("[defaults]\ninventory='a.yml\n"),
		"invalid utf8":        {0xff, '\n'},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			directory := symlinkFreeTempDir(t)
			path := filepath.Join(directory, "ansible.cfg")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readAnsibleConfig(path)
			if !IsCode(err, "CMDB_ANSIBLE_CONFIG_INVALID") || !strings.Contains(err.Error(), "inventory = <path>") {
				t.Fatalf("error = %v", err)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(after, body) {
				t.Fatalf("rejected config changed: %v, %q", readErr, after)
			}
		})
	}
}

func TestAnsibleConfigRejectsSymlinkAndOversizeFile(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	valid := filepath.Join(directory, "valid.cfg")
	if err := os.WriteFile(valid, []byte("[defaults]\ninventory=a.yml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.cfg")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(directory, "large.cfg")
	if err := os.WriteFile(large, bytes.Repeat([]byte{'x'}, maxAnsibleConfigSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, directory, large} {
		if _, err := readAnsibleConfig(path); !IsCode(err, "CMDB_ANSIBLE_CONFIG_INVALID") {
			t.Fatalf("%s error = %v", path, err)
		}
	}
}
