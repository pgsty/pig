package build

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"testing"
	"time"
)

func TestFindBuiltPackageArtifactRPM(t *testing.T) {
	home := t.TempDir()
	oldHomeDir := config.HomeDir
	oldOSType := config.OSType
	config.HomeDir = home
	config.OSType = config.DistroEL
	defer func() {
		config.HomeDir = oldHomeDir
		config.OSType = oldOSType
	}()

	oldArtifact := filepath.Join(home, "ext", "pkg", "x86_64", "cloudberry-2.1.0-2PIGSTY.el9.x86_64.rpm")
	newArtifact := filepath.Join(home, "rpmbuild", "RPMS", "x86_64", "cloudberry-2.1.0-3PIGSTY.el9.x86_64.rpm")
	pxfArtifact := filepath.Join(home, "rpmbuild", "RPMS", "x86_64", "cloudberry-pxf-2.1.0-3PIGSTY.el9.x86_64.rpm")
	writeArtifact(t, oldArtifact, time.Unix(100, 0))
	writeArtifact(t, newArtifact, time.Unix(200, 0))
	writeArtifact(t, pxfArtifact, time.Unix(300, 0))

	got, err := findBuiltPackageArtifact("cloudberry")
	if err != nil {
		t.Fatalf("findBuiltPackageArtifact returned error: %v", err)
	}
	if got != newArtifact {
		t.Fatalf("findBuiltPackageArtifact = %q, expected %q", got, newArtifact)
	}
}

func TestFindBuiltPackageArtifactDEB(t *testing.T) {
	home := t.TempDir()
	oldHomeDir := config.HomeDir
	oldOSType := config.OSType
	config.HomeDir = home
	config.OSType = config.DistroDEB
	defer func() {
		config.HomeDir = oldHomeDir
		config.OSType = oldOSType
	}()

	artifact := filepath.Join(home, "ext", "pkg", "cloudberry_2.1.0-2PIGSTY_amd64.deb")
	pxfArtifact := filepath.Join(home, "ext", "pkg", "cloudberry-pxf_2.1.0-2PIGSTY_amd64.deb")
	writeArtifact(t, artifact, time.Unix(100, 0))
	writeArtifact(t, pxfArtifact, time.Unix(200, 0))

	got, err := findBuiltPackageArtifact("cloudberry")
	if err != nil {
		t.Fatalf("findBuiltPackageArtifact returned error: %v", err)
	}
	if got != artifact {
		t.Fatalf("findBuiltPackageArtifact = %q, expected %q", got, artifact)
	}
}

func writeArtifact(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create artifact directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("artifact"), 0644); err != nil {
		t.Fatalf("failed to write artifact: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("failed to set artifact time: %v", err)
	}
}
