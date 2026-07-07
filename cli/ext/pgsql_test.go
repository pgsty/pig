package ext

import (
	"reflect"
	"testing"
)

func TestBetaPGIsInstallableButNotStableDefault(t *testing.T) {
	if PostgresBetaMajorVersion != 19 {
		t.Fatalf("expected beta PG major placeholder to be 19, got %d", PostgresBetaMajorVersion)
	}
	if !IsInstallablePGMajor(PostgresBetaMajorVersion) {
		t.Fatalf("expected PG%d to be accepted as an installable beta major", PostgresBetaMajorVersion)
	}
	if IsActivePGMajor(PostgresBetaMajorVersion) {
		t.Fatalf("expected PG%d to stay out of the stable active display window", PostgresBetaMajorVersion)
	}
	if got := PostgresLatestMajorVersion(); got != 18 {
		t.Fatalf("expected stable default latest PG major to remain 18, got %d", got)
	}
	if got, want := PostgresActiveMajorVersions, []int{18, 17, 16, 15, 14}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stable active versions changed: got %v want %v", got, want)
	}
	if got, want := PostgresInstallableMajorVersions, []int{PostgresBetaMajorVersion, 18, 17, 16, 15, 14}; !reflect.DeepEqual(got, want) {
		t.Fatalf("installable versions mismatch: got %v want %v", got, want)
	}
}

func TestIsBetaPGMajorUsesPlaceholderAndStableWindow(t *testing.T) {
	if !IsBetaPGMajor(PostgresBetaMajorVersion) {
		t.Fatalf("expected PG%d to be treated as the current beta major", PostgresBetaMajorVersion)
	}
	if IsBetaPGMajor(PostgresLatestMajorVersion()) {
		t.Fatalf("expected latest stable PG%d not to be treated as beta", PostgresLatestMajorVersion())
	}

	oldActive := PostgresActiveMajorVersions
	defer func() { PostgresActiveMajorVersions = oldActive }()
	PostgresActiveMajorVersions = append([]int{PostgresBetaMajorVersion}, oldActive...)
	if IsBetaPGMajor(PostgresBetaMajorVersion) {
		t.Fatalf("expected PG%d to stop being beta once it enters the stable window", PostgresBetaMajorVersion)
	}
}

func TestLatestInstallPrefersStableOverBeta(t *testing.T) {
	pg17 := &PostgresInstall{MajorVersion: 17}
	pg18 := &PostgresInstall{MajorVersion: 18}
	pgBeta := &PostgresInstall{MajorVersion: PostgresBetaMajorVersion}

	if got := latestInstall(map[int]*PostgresInstall{17: pg17, 18: pg18, PostgresBetaMajorVersion: pgBeta}); got != pg18 {
		t.Fatalf("expected stable PG18 to win over beta PG%d, got %+v", PostgresBetaMajorVersion, got)
	}
	if got := latestInstall(map[int]*PostgresInstall{17: pg17, 18: pg18}); got != pg18 {
		t.Fatalf("expected latest stable PG18, got %+v", got)
	}
	if got := latestInstall(map[int]*PostgresInstall{PostgresBetaMajorVersion: pgBeta}); got != pgBeta {
		t.Fatalf("expected beta-only host to fall back to PG%d, got %+v", PostgresBetaMajorVersion, got)
	}
	if got := latestInstall(nil); got != nil {
		t.Fatalf("expected nil for empty install map, got %+v", got)
	}
}
