package install

import (
	"slices"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestInstallPackagesLocalScope(t *testing.T) {
	origRun := runPackageInstall
	origQuery := runPackageQuery
	origType, origCode, origVersion := config.OSType, config.OSCode, config.OSVersion
	t.Cleanup(func() {
		runPackageInstall = origRun
		runPackageQuery = origQuery
		config.OSType, config.OSCode, config.OSVersion = origType, origCode, origVersion
	})

	var calls [][]string
	runPackageInstall = func(args []string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	var queryName string
	var queryArgs []string
	runPackageQuery = func(name string, args ...string) ([]byte, error) {
		queryName, queryArgs = name, append([]string(nil), args...)
		start := slices.Index(args, "--qf=%{name}") + 1
		return []byte(strings.Join(args[start:], "\n")), nil
	}
	config.OSType, config.OSCode, config.OSVersion = config.DistroEL, "el10", "10"
	if err := InstallPackagesWithOptions(0, []string{"node-bootstrap"}, PackageOptions{Yes: true, LocalOnly: true}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("package calls = %v", calls)
	}
	for _, want := range []string{"dnf", "--disablerepo=*", "--enablerepo=pigsty-local", "makecache"} {
		if !slices.Contains(calls[0], want) {
			t.Fatalf("local refresh argv missing %q: %v", want, calls[0])
		}
	}
	for _, want := range []string{"dnf", "--disablerepo=*", "--enablerepo=pigsty-local", "install", "-y", "ansible"} {
		if !slices.Contains(calls[1], want) {
			t.Fatalf("local install argv missing %q: %v", want, calls[1])
		}
	}
	if queryName != "dnf" {
		t.Fatalf("local query command = %q", queryName)
	}
	for _, want := range []string{"--disablerepo=*", "--enablerepo=pigsty-local", "repoquery", "--available", "ansible"} {
		if !slices.Contains(queryArgs, want) {
			t.Fatalf("local query argv missing %q: %v", want, queryArgs)
		}
	}
}

func TestInstallPackagesAPTUsesNativeRetries(t *testing.T) {
	origRun := runPackageInstall
	origType, origCode, origVersion := config.OSType, config.OSCode, config.OSVersion
	t.Cleanup(func() {
		runPackageInstall = origRun
		config.OSType, config.OSCode, config.OSVersion = origType, origCode, origVersion
	})

	var call []string
	runPackageInstall = func(args []string) error {
		call = append([]string(nil), args...)
		return nil
	}
	config.OSType, config.OSCode, config.OSVersion = config.DistroDEB, "u24", "24"
	if err := InstallPackagesWithOptions(0, []string{"tar"}, PackageOptions{Yes: true, NoTranslation: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"apt-get", "Acquire::Retries=3", "install", "-y", "tar"} {
		if !slices.Contains(call, want) {
			t.Fatalf("apt install argv missing %q: %v", want, call)
		}
	}
}

func TestVerifyLocalPackagesAvailableRejectsMissingAPTArchive(t *testing.T) {
	origQuery := runPackageQuery
	origType := config.OSType
	t.Cleanup(func() {
		runPackageQuery = origQuery
		config.OSType = origType
	})
	config.OSType = config.DistroDEB
	runPackageQuery = func(name string, args ...string) ([]byte, error) {
		if name != "apt-cache" || !slices.Contains(args, "madison") {
			t.Fatalf("APT package query = %s %v", name, args)
		}
		return []byte("ansible | 13.1.0 | file:/www/pigsty ./ Packages\n"), nil
	}
	err := verifyLocalPackagesAvailable("apt-get", []string{"ansible", "sow"}, PackageOptions{LocalOnly: true})
	if err == nil || !strings.Contains(err.Error(), "sow") {
		t.Fatalf("missing local package result = %v", err)
	}
}
