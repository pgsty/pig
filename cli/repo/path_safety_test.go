package repo

import (
	"testing"

	"pig/internal/config"
)

func TestIsSafeModuleName(t *testing.T) {
	tests := []struct {
		name   string
		module string
		want   bool
	}{
		{name: "normal", module: "pigsty", want: true},
		{name: "with_dash", module: "pgdg-common", want: true},
		{name: "with_underscore", module: "my_repo", want: true},
		{name: "empty", module: "", want: false},
		{name: "dotdot", module: "../tmp", want: false},
		{name: "slash", module: "a/b", want: false},
		{name: "backslash", module: `a\b`, want: false},
		{name: "dotdot_in_middle", module: "a..b", want: false},
		{name: "space", module: "bad name", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeModuleName(tt.module); got != tt.want {
				t.Fatalf("isSafeModuleName(%q) = %v, want %v", tt.module, got, tt.want)
			}
		})
	}
}

func TestGetModulePathRejectsTraversal(t *testing.T) {
	oldType := config.OSType
	defer func() { config.OSType = oldType }()

	config.OSType = config.DistroEL
	m := &Manager{RepoDir: "/etc/yum.repos.d"}

	if got := m.getModulePath("../../tmp/poc"); got != "" {
		t.Fatalf("expected empty path for traversal module, got %q", got)
	}
}

func TestGetModulePathValidModule(t *testing.T) {
	oldType := config.OSType
	defer func() { config.OSType = oldType }()

	config.OSType = config.DistroDEB
	m := &Manager{RepoDir: "/etc/apt/sources.list.d"}

	got := m.getModulePath("pigsty")
	want := "/etc/apt/sources.list.d/pigsty.list"
	if got != want {
		t.Fatalf("getModulePath() = %q, want %q", got, want)
	}
}
