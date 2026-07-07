package build

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
)

func withBuildToolTestEnv(t *testing.T, osType, osVersion, osCode string, activePG []int) func() {
	t.Helper()

	oldOSType := config.OSType
	oldOSVersion := config.OSVersion
	oldOSCode := config.OSCode
	oldActivePG := ext.PostgresActiveMajorVersions

	config.OSType = osType
	config.OSVersion = osVersion
	config.OSCode = osCode
	ext.PostgresActiveMajorVersions = append([]int(nil), activePG...)

	return func() {
		config.OSType = oldOSType
		config.OSVersion = oldOSVersion
		config.OSCode = oldOSCode
		ext.PostgresActiveMajorVersions = oldActivePG
	}
}

func TestBuildToolInstallCommandELEnumeratesExactPostgresPackages(t *testing.T) {
	activePG := []int{18, 17, 16, 15, 14}
	wantPGPackages := []string{
		"postgresql14-devel", "postgresql14-server",
		"postgresql15-devel", "postgresql15-server",
		"postgresql16-devel", "postgresql16-server",
		"postgresql17-devel", "postgresql17-server",
		"postgresql18-devel", "postgresql18-server",
	}

	tests := []struct {
		osCode    string
		osVersion string
	}{
		{osCode: "el8", osVersion: "8"},
		{osCode: "el9", osVersion: "9"},
		{osCode: "el10", osVersion: "10"},
	}

	for _, tt := range tests {
		t.Run(tt.osCode, func(t *testing.T) {
			cleanup := withBuildToolTestEnv(t, config.DistroEL, tt.osVersion, tt.osCode, activePG)
			defer cleanup()

			got, err := buildToolInstallCommand("", false)
			if err != nil {
				t.Fatalf("buildToolInstallCommand() returned error: %v", err)
			}
			if len(got) < 3 || !reflect.DeepEqual(got[:3], []string{"dnf", "install", "-y"}) {
				t.Fatalf("install command prefix = %v, want dnf install -y", got)
			}

			for _, pkg := range wantPGPackages {
				if !slices.Contains(got, pkg) {
					t.Fatalf("%s command missing %s:\n%v", tt.osCode, pkg, got)
				}
			}
			for _, pkg := range []string{
				fmt.Sprintf("postgresql%d-devel", ext.PostgresBetaMajorVersion),
				fmt.Sprintf("postgresql%d-server", ext.PostgresBetaMajorVersion),
			} {
				if slices.Contains(got, pkg) {
					t.Fatalf("%s default command should not include beta package %s:\n%v", tt.osCode, pkg, got)
				}
			}
			for _, token := range got {
				if strings.HasPrefix(token, "postgresql") && strings.Contains(token, "*") {
					t.Fatalf("%s command contains PostgreSQL package wildcard %q:\n%v", tt.osCode, token, got)
				}
				if token == "postgresql1"+"*-devel" || token == "postgresql1"+"*-server" {
					t.Fatalf("%s command contains legacy wildcard package %q:\n%v", tt.osCode, token, got)
				}
			}

			if slices.Contains(got, "--setopt=appstream.exclude=postgresql*") {
				t.Fatalf("%s command should not add appstream PostgreSQL exclude after exact package enumeration:\n%v", tt.osCode, got)
			}
			if tt.osCode == "el10" {
				t.Logf("install command: %s", strings.Join(got, " "))
			}
		})
	}
}

func TestInstallBuildToolsAPISignatures(t *testing.T) {
	var install func(string) error = InstallBuildTools
	var installBeta func(string, bool) error = InstallBuildToolsWithBeta
	_ = install
	_ = installBeta
}

func TestBuildToolInstallCommandUsesActivePGVersionList(t *testing.T) {
	cleanup := withBuildToolTestEnv(t, config.DistroEL, "9", "el9", []int{16, 14})
	defer cleanup()

	got, err := buildToolInstallCommand("", false)
	if err != nil {
		t.Fatalf("buildToolInstallCommand() returned error: %v", err)
	}

	want := []string{
		"postgresql14-devel", "postgresql14-server",
		"postgresql16-devel", "postgresql16-server",
	}
	for _, pkg := range want {
		if !slices.Contains(got, pkg) {
			t.Fatalf("command missing active PG package %s:\n%v", pkg, got)
		}
	}
	for _, version := range []int{15, 17, 18} {
		for _, suffix := range []string{"devel", "server"} {
			pkg := fmt.Sprintf("postgresql%d-%s", version, suffix)
			if slices.Contains(got, pkg) {
				t.Fatalf("command should not include inactive PG package %s:\n%v", pkg, got)
			}
		}
	}
}

func TestBuildToolInstallCommandMiniDropsPostgresBuildDependencies(t *testing.T) {
	cleanup := withBuildToolTestEnv(t, config.DistroEL, "10", "el10", []int{18, 17, 16, 15, 14})
	defer cleanup()

	got, err := buildToolInstallCommand("mini", false)
	if err != nil {
		t.Fatalf("buildToolInstallCommand(mini) returned error: %v", err)
	}

	for _, token := range got {
		if strings.Contains(strings.ToLower(token), "postgresql") || strings.Contains(strings.ToLower(token), "pgdg") {
			t.Fatalf("mini command should not include PostgreSQL/PGDG dependency %q:\n%v", token, got)
		}
	}
}

func TestBuildToolInstallCommandBetaAddsInstallableBetaPackages(t *testing.T) {
	cleanup := withBuildToolTestEnv(t, config.DistroEL, "10", "el10", []int{18, 17, 16, 15, 14})
	defer cleanup()

	got, err := buildToolInstallCommand("", true)
	if err != nil {
		t.Fatalf("buildToolInstallCommand(beta) returned error: %v", err)
	}

	for _, pkg := range []string{
		fmt.Sprintf("postgresql%d-devel", ext.PostgresBetaMajorVersion),
		fmt.Sprintf("postgresql%d-server", ext.PostgresBetaMajorVersion),
	} {
		if !slices.Contains(got, pkg) {
			t.Fatalf("beta command missing %s:\n%v", pkg, got)
		}
	}
}

func TestBuildToolInstallCommandBetaAddsDebianBetaPackages(t *testing.T) {
	cleanup := withBuildToolTestEnv(t, config.DistroDEB, "12", "d12", []int{18, 17, 16, 15, 14})
	defer cleanup()

	got, err := buildToolInstallCommand("", true)
	if err != nil {
		t.Fatalf("buildToolInstallCommand(deb,beta) returned error: %v", err)
	}

	for _, pkg := range []string{
		fmt.Sprintf("postgresql-%d", ext.PostgresBetaMajorVersion),
		fmt.Sprintf("postgresql-server-dev-%d", ext.PostgresBetaMajorVersion),
	} {
		if !slices.Contains(got, pkg) {
			t.Fatalf("debian beta command missing %s:\n%v", pkg, got)
		}
	}
}

func TestBuildToolInstallCommandBetaMiniStillDropsPostgresBuildDependencies(t *testing.T) {
	cleanup := withBuildToolTestEnv(t, config.DistroEL, "10", "el10", []int{18, 17, 16, 15, 14})
	defer cleanup()

	got, err := buildToolInstallCommand("mini", true)
	if err != nil {
		t.Fatalf("buildToolInstallCommand(mini,beta) returned error: %v", err)
	}

	for _, token := range got {
		if strings.Contains(strings.ToLower(token), "postgresql") || strings.Contains(strings.ToLower(token), "pgdg") {
			t.Fatalf("mini beta command should not include PostgreSQL/PGDG dependency %q:\n%v", token, got)
		}
	}
}
