package ext

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"testing"
)

func TestDetectPostgresIsIdempotent(t *testing.T) {
	originalOSType := config.OSType
	originalSearch := PostgresElSearchPath
	originalInstalls, originalPathMap := Installs, PathMap
	originalActive, originalPostgres := Active, Postgres
	originalInited := Inited
	originalGetActive, originalNewInstall := getActivePgConfig, newPostgresInstall
	t.Cleanup(func() {
		config.OSType = originalOSType
		PostgresElSearchPath = originalSearch
		Installs, PathMap = originalInstalls, originalPathMap
		Active, Postgres, Inited = originalActive, originalPostgres, originalInited
		getActivePgConfig, newPostgresInstall = originalGetActive, originalNewInstall
	})

	root := t.TempDir()
	pattern := filepath.Join(root, "%s", "pg_config")
	path := filepath.Join(root, "18", "pg_config")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	config.OSType = config.DistroEL
	PostgresElSearchPath = []string{pattern}
	Installs = map[int]*PostgresInstall{17: {MajorVersion: 17, PgConfigPath: "/stale"}}
	getActivePgConfig = func() (string, error) { return path, nil }
	newPostgresInstall = func(got string) (*PostgresInstall, error) {
		return &PostgresInstall{PgConfig: got, PgConfigPath: got, MajorVersion: 18}, nil
	}

	for i := 0; i < 2; i++ {
		if err := DetectPostgres(); err != nil {
			t.Fatal(err)
		}
		if len(Installs) != 1 || Installs[18] == nil || len(PathMap) != 1 || Active == nil || Active.MajorVersion != 18 {
			t.Fatalf("detection %d state: installs=%#v paths=%#v active=%#v", i+1, Installs, PathMap, Active)
		}
	}
}

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
