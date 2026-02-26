package ext

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pig/internal/config"
)

func installFakePackageManager(t *testing.T, cmdName string) {
	t.Helper()

	dir := t.TempDir()
	cmdPath := filepath.Join(dir, cmdName)
	if err := os.WriteFile(cmdPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to create fake %s command: %v", cmdName, err)
	}

	t.Setenv("PIG_NO_SUDO", "1")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func setupAliasPackageOpEnv(t *testing.T) func() {
	t.Helper()

	oldCatalog := Catalog
	oldOSType := config.OSType
	oldOSVersion := config.OSVersion
	oldOSCode := config.OSCode
	oldOSArch := config.OSArch

	config.OSType = config.DistroEL
	config.OSVersion = "9"
	config.OSCode = "el9"
	config.OSArch = "amd64"
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		Dependency: map[string][]string{},
		AliasMap:   map[string]string{},
	}

	installFakePackageManager(t, "dnf")

	return func() {
		Catalog = oldCatalog
		config.OSType = oldOSType
		config.OSVersion = oldOSVersion
		config.OSCode = oldOSCode
		config.OSArch = oldOSArch
	}
}

func TestRmExtensionsAliasReportsRemovedPackages(t *testing.T) {
	cleanup := setupAliasPackageOpEnv(t)
	defer cleanup()

	result := RmExtensions(17, []string{"pg17-devel"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success result, got code=%d message=%q", result.Code, result.Message)
	}

	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatalf("expected data type *ExtensionRmData, got %T", result.Data)
	}

	expected := []string{"postgresql17-devel"}
	if !reflect.DeepEqual(data.Packages, expected) {
		t.Fatalf("resolved packages mismatch:\nwant: %v\ngot:  %v", expected, data.Packages)
	}
	if !reflect.DeepEqual(data.Removed, expected) {
		t.Fatalf("removed list should use real packages, not alias:\nwant: %v\ngot:  %v", expected, data.Removed)
	}
}

func TestUpgradeExtensionsAliasReportsUpdatedPackages(t *testing.T) {
	cleanup := setupAliasPackageOpEnv(t)
	defer cleanup()

	result := UpgradeExtensions(17, []string{"pg17-devel"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success result, got code=%d message=%q", result.Code, result.Message)
	}

	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatalf("expected data type *ExtensionUpdateData, got %T", result.Data)
	}

	expected := []string{"postgresql17-devel"}
	if !reflect.DeepEqual(data.Packages, expected) {
		t.Fatalf("resolved packages mismatch:\nwant: %v\ngot:  %v", expected, data.Packages)
	}
	if !reflect.DeepEqual(data.Updated, expected) {
		t.Fatalf("updated list should use real packages, not alias:\nwant: %v\ngot:  %v", expected, data.Updated)
	}
}
