package repo

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestManagerEnablesSignedPigstyRepos(t *testing.T) {
	origType, origCode, origArch := config.OSType, config.OSCode, config.OSArch
	t.Cleanup(func() { config.OSType, config.OSCode, config.OSArch = origType, origCode, origArch })
	config.OSType, config.OSCode, config.OSArch = config.DistroDEB, "u26", "aarch64"
	manager, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.enablePigstyGPGCheck()
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
