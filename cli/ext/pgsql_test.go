package ext

import (
	"reflect"
	"testing"
)

func TestPG19IsInstallableButNotStableDefault(t *testing.T) {
	if !IsInstallablePGMajor(19) {
		t.Fatal("expected PG19 to be accepted as an installable beta major")
	}
	if IsActivePGMajor(19) {
		t.Fatal("expected PG19 to stay out of the stable active display window")
	}
	if got := PostgresLatestMajorVersion(); got != 18 {
		t.Fatalf("expected stable default latest PG major to remain 18, got %d", got)
	}
	if got, want := PostgresActiveMajorVersions, []int{18, 17, 16, 15, 14}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stable active versions changed: got %v want %v", got, want)
	}
	if got, want := PostgresInstallableMajorVersions, []int{19, 18, 17, 16, 15, 14}; !reflect.DeepEqual(got, want) {
		t.Fatalf("installable versions mismatch: got %v want %v", got, want)
	}
}

func TestLatestInstallPrefersStableOverBeta(t *testing.T) {
	pg17 := &PostgresInstall{MajorVersion: 17}
	pg18 := &PostgresInstall{MajorVersion: 18}
	pg19 := &PostgresInstall{MajorVersion: 19}

	if got := latestInstall(map[int]*PostgresInstall{17: pg17, 18: pg18, 19: pg19}); got != pg18 {
		t.Fatalf("expected stable PG18 to win over beta PG19, got %+v", got)
	}
	if got := latestInstall(map[int]*PostgresInstall{17: pg17, 18: pg18}); got != pg18 {
		t.Fatalf("expected latest stable PG18, got %+v", got)
	}
	if got := latestInstall(map[int]*PostgresInstall{19: pg19}); got != pg19 {
		t.Fatalf("expected beta-only host to fall back to PG19, got %+v", got)
	}
	if got := latestInstall(nil); got != nil {
		t.Fatalf("expected nil for empty install map, got %+v", got)
	}
}
