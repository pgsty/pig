package repo

import (
	"testing"

	"pig/internal/config"
)

func TestResolveModulePathForRm_AllowsDottedModuleOnEL(t *testing.T) {
	oldType := config.OSType
	defer func() { config.OSType = oldType }()

	config.OSType = config.DistroEL
	m := &Manager{RepoDir: "/etc/yum.repos.d"}

	path, err := resolveModulePathForRm(m, "foo.bar")
	if err != nil {
		t.Fatalf("resolveModulePathForRm returned error: %v", err)
	}
	if path != "/etc/yum.repos.d/foo.bar.repo" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestResolveModulePathForRm_RejectsTraversal(t *testing.T) {
	oldType := config.OSType
	defer func() { config.OSType = oldType }()

	config.OSType = config.DistroDEB
	m := &Manager{RepoDir: "/etc/apt/sources.list.d"}

	if _, err := resolveModulePathForRm(m, "../evil"); err == nil {
		t.Fatalf("expected traversal module to be rejected")
	}
	if _, err := resolveModulePathForRm(m, `a\b`); err == nil {
		t.Fatalf("expected backslash module to be rejected")
	}
	if _, err := resolveModulePathForRm(m, "a/b"); err == nil {
		t.Fatalf("expected slash module to be rejected")
	}
}
