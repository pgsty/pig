package ext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanExtensions_NoCatalog(t *testing.T) {
	tmp := t.TempDir()

	libDir := filepath.Join(tmp, "lib")
	extDir := filepath.Join(tmp, "share", "extension")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.MkdirAll(extDir, 0755); err != nil {
		t.Fatalf("mkdir ext: %v", err)
	}

	// Minimal shared library + control file pair.
	if err := os.WriteFile(filepath.Join(libDir, "testext.so"), []byte(""), 0644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	control := "default_version = '1.0'\ncomment = 'test extension'\n"
	if err := os.WriteFile(filepath.Join(extDir, "testext.control"), []byte(control), 0644); err != nil {
		t.Fatalf("write control: %v", err)
	}

	oldCatalog := Catalog
	Catalog = nil
	defer func() {
		Catalog = oldCatalog
	}()

	pi := &PostgresInstall{
		LibPath: libDir,
		ExtPath: extDir,
	}
	if err := pi.ScanExtensions(); err != nil {
		t.Fatalf("ScanExtensions() error: %v", err)
	}

	if got := len(pi.Extensions); got != 1 {
		t.Fatalf("Extensions=%d, want 1", got)
	}
	if pi.ExtensionMap["testext"] == nil {
		t.Fatalf("ExtensionMap missing testext")
	}
	if !pi.SharedLibs["testext"] {
		t.Fatalf("SharedLibs[testext]=false, want true")
	}
	if pi.Extensions[0].Libraries == nil || !pi.Extensions[0].Libraries["testext"] {
		t.Fatalf("expected Libraries[testext]=true")
	}
}
