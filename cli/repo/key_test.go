package repo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestInstallPigstyKeyUsesEmbeddedKey(t *testing.T) {
	origType, origPath, origPut := config.OSType, pigstyAPTKeyPath, putPigstyKey
	t.Cleanup(func() {
		config.OSType, pigstyAPTKeyPath, putPigstyKey = origType, origPath, origPut
	})
	config.OSType = config.DistroDEB
	pigstyAPTKeyPath = filepath.Join(t.TempDir(), "keyrings", "pigsty.asc")
	putPigstyKey = func(path string, content []byte, mode os.FileMode) error {
		if path != pigstyAPTKeyPath || mode != 0o644 || !bytes.Equal(content, embeddedPigstyKey) {
			t.Fatalf("unexpected key write: path=%q mode=%o bytes=%d", path, mode, len(content))
		}
		return nil
	}
	if err := InstallPigstyKey(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedAPTKeyUsesArmoredExtension(t *testing.T) {
	if !bytes.HasPrefix(embeddedPigstyKey, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----")) {
		t.Fatal("embedded Pigsty key is not ASCII-armored")
	}
	if filepath.Ext(pigstyAPTKeyPath) != ".asc" {
		t.Fatalf("ASCII-armored Pigsty key path = %q", pigstyAPTKeyPath)
	}
}

func TestInstallPigstyKeyReusesExistingFile(t *testing.T) {
	_, keyPath := setupAddKeyTest(t, config.DistroDEB, "d12", "x86_64")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const content = "existing administrator-managed key\n"
	if err := os.WriteFile(keyPath, []byte(content), 0o444); err != nil {
		t.Fatal(err)
	}
	putPigstyKey = func(string, []byte, os.FileMode) error {
		t.Fatal("existing key reached the writer")
		return nil
	}
	if err := InstallPigstyKey(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(keyPath)
	if err != nil || string(got) != content {
		t.Fatalf("existing key changed: %q, %v", got, err)
	}
}

func TestInstallPigstyKeyAttemptsInstallWhenStatDenied(t *testing.T) {
	_, keyPath := setupAddKeyTest(t, config.DistroDEB, "d12", "x86_64")
	statPigstyKey = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	if err := InstallPigstyKey(); err != nil {
		t.Fatal(err)
	}
	assertInstalledPigstyKey(t, keyPath)
}

func TestInstallPigstyKeyRejectsNonRegularDestination(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			_, keyPath := setupAddKeyTest(t, config.DistroDEB, "d12", "aarch64")
			if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			if kind == "directory" {
				err = os.Mkdir(keyPath, 0o755)
			} else {
				err = os.Symlink(filepath.Join(t.TempDir(), "missing"), keyPath)
			}
			if err != nil {
				t.Fatal(err)
			}
			putPigstyKey = func(string, []byte, os.FileMode) error {
				t.Fatal("non-regular key destination reached the writer")
				return nil
			}
			if err := InstallPigstyKey(); err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("%s destination: %v", kind, err)
			}
		})
	}
}

func TestManagerEnablesSignedPigstyRepos(t *testing.T) {
	manager, _ := setupAddKeyTest(t, config.DistroDEB, "u26", "aarch64")
	if _, err := manager.preparePigstyKey([]string{"pigsty"}, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pigsty-infra", "pigsty-pgsql"} {
		repository := manager.Map[name]
		if repository == nil || repository.Meta["signed-by"] != pigstyAPTKeyPath {
			t.Fatalf("%s signing metadata = %+v", name, repository)
		}
		if _, trusted := repository.Meta["trusted"]; trusted {
			t.Fatalf("%s retained trusted=yes with signed-by", name)
		}
	}
}
