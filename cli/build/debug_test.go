package build

import (
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
)

func TestRPMDebugPackagesDefaultAndOptOut(t *testing.T) {
	builder := &ExtensionBuilder{
		OSType:   config.DistroEL,
		SpecPath: "/tmp/example.spec",
		Extension: &ext.Extension{
			Name: "example",
		},
	}

	cmd, _ := builder.createRPMBuildCommand(17, nil)
	if got := strings.Join(cmd.Args, " "); strings.Contains(got, "debug_package %{nil}") {
		t.Fatalf("default RPM command disables debug packages: %s", got)
	}

	builder.NoDebug = true
	cmd, _ = builder.createRPMBuildCommand(17, nil)
	if got := strings.Join(cmd.Args, " "); !strings.Contains(got, "--define debug_package %{nil}") {
		t.Fatalf("--nodbg RPM command does not disable debug packages: %s", got)
	}
}

func TestDEBDebugPackagesDefaultAndOptOut(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"DEB_BUILD_OPTIONS=parallel=4 nocheck",
		"EXAMPLE=value",
	}

	defaultEnv := buildCommandEnvironment(base, config.DistroDEB, false, "/custom/bin")
	if got, _ := environmentValue(defaultEnv, "DEB_BUILD_OPTIONS"); got != "parallel=4 nocheck" {
		t.Fatalf("default DEB_BUILD_OPTIONS = %q, want caller value unchanged", got)
	}
	if got, _ := environmentValue(defaultEnv, "PATH"); got != "/custom/bin" {
		t.Fatalf("build PATH = %q, want /custom/bin", got)
	}

	noDebugEnv := buildCommandEnvironment(base, config.DistroDEB, true, "/custom/bin")
	if got, _ := environmentValue(noDebugEnv, "DEB_BUILD_OPTIONS"); got != "parallel=4 nocheck noautodbgsym" {
		t.Fatalf("--nodbg DEB_BUILD_OPTIONS = %q", got)
	}

	alreadyDisabled := []string{"DEB_BUILD_OPTIONS=noautodbgsym parallel=2"}
	noDebugEnv = buildCommandEnvironment(alreadyDisabled, config.DistroDEB, true, "/custom/bin")
	if got, _ := environmentValue(noDebugEnv, "DEB_BUILD_OPTIONS"); got != "noautodbgsym parallel=2" {
		t.Fatalf("duplicate --nodbg DEB_BUILD_OPTIONS = %q", got)
	}
}
