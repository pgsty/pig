package sty

import (
	"errors"
	"os"
	"path/filepath"
	"pig/internal/output"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestResolveTemplatePathSearchOrder(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
		files             []string
	}{
		{"root", "full", "full", []string{"full", "ha/full"}},
		{"ha", "full", "ha/full", []string{"ha/full", "fork/full"}},
		{"fork", "full", "fork/full", []string{"fork/full", "app/full"}},
		{"app", "full", "app/full", []string{"app/full", "demo/full"}},
		{"demo", "full", "demo/full", []string{"demo/full", "build/full"}},
		{"build", "full", "build/full", []string{"build/full"}},
		{"explicit", "demo/full", "demo/full", []string{"full", "ha/full", "demo/full"}},
		{"prefixed short name", "./conf/full.yml", "ha/full", []string{"ha/full"}},
		{"prefixed path", "./conf/ha/full.yml", "ha/full", []string{"full", "ha/full"}},
		{"custom path", "custom/full", "custom/full", []string{"custom/full"}},
		{"no recursive search", "full", "", []string{"custom/full", "ha/nested/full"}},
		{"no explicit fallback", "fork/full", "", []string{"full", "ha/full"}},
		{"halo", "halo", "fork/halo", []string{"fork/halo", "demo/mysql"}},
		{"mysql", "mysql", "demo/mysql", []string{"fork/halo", "demo/mysql"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			for _, mode := range test.files {
				writeConfigureTemplate(t, home, mode, "all: {}\n")
			}
			mode, path, err := resolveTemplatePath(home, test.input)
			if test.want == "" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("expected missing template, got %q, %q, %v", mode, path, err)
				}
				return
			}
			wantPath := filepath.Join(home, "conf", filepath.FromSlash(test.want)+".yml")
			if err != nil || mode != test.want || path != wantPath {
				t.Fatalf("resolved %q, %q, %v; want %q, %q", mode, path, err, test.want, wantPath)
			}
		})
	}
}

func TestResolveTemplatePathSkipsDirectories(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf", "full.yml"), 0755); err != nil {
		t.Fatal(err)
	}
	writeConfigureTemplate(t, home, "ha/full", "all: {}\n")
	mode, _, err := resolveTemplatePath(home, "full")
	if err != nil || mode != "ha/full" {
		t.Fatalf("resolved %q, %v; want ha/full", mode, err)
	}
}

func TestConfigureNativeResolvedModeControlsBehavior(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	// Exercise the native pipeline without invoking local tools or SSH.
	t.Setenv("PATH", t.TempDir())
	goosFn = func() string { return "linux" }
	currentUserFn = func() string { return "root" }
	lookPathFn = func(name string) (string, error) { return "/test/" + name, nil }
	var adminChecks int
	runCommandFn = func(_ time.Duration, name string, args ...string) error {
		if name == "ssh" && slices.Contains(args, "sudo") {
			adminChecks++
		}
		return nil
	}
	const template = "all:\n  vars:\n    admin_ip: 10.10.10.10\n    region: default\n    pg_version: 17\n    pg_packages: [pg17-main]\n"
	for _, test := range []struct {
		input, canonical string
		pinned, build    bool
	}{
		{"full", "ha/full", false, false},
		{"./conf/ha/full.yml", "ha/full", false, false},
		{"oss", "build/oss", false, true},
		{"build/oss", "build/oss", false, true},
		{"mssql", "fork/mssql", true, false},
		{"fork/mssql.yml", "fork/mssql", true, false},
		{"polar", "fork/polar", true, false},
		{"fork/polar", "fork/polar", true, false},
		{"mssql", "mssql", true, false},
		{"polar", "polar", true, false},
		{"app/mssql", "app/mssql", false, false},
	} {
		t.Run(test.canonical+"/"+test.input, func(t *testing.T) {
			home := t.TempDir()
			writeConfigureTemplate(t, home, test.canonical, template)
			adminChecks = 0
			locale := false
			result := ConfigureNative(ConfigureOptions{
				PigstyHome: home, Mode: test.input, Region: "default", PrimaryIP: "192.168.0.10",
				PGVersion: "16", CPUCount: 8, LocaleAvailable: &locale, NonInteractive: true,
			})
			if !result.Success {
				t.Fatalf("configure failed: %+v", result)
			}
			data := result.Data.(*ConfigureData)
			if data.Mode != test.canonical || data.TemplatePath != filepath.Join(home, "conf", test.canonical+".yml") {
				t.Fatalf("result does not identify the selected template: %+v", data)
			}
			wantIP, wantChecks, wantVersion := "192.168.0.10", 1, "16"
			if test.build {
				wantIP, wantChecks = defaultPrimaryIP, 0
				if !slices.Contains(data.Warnings, "admin ssh/sudo check skipped for build/* templates") || len(data.IPReplacements) != 0 {
					t.Fatalf("build behavior lost after resolution: %+v", data)
				}
			}
			if test.pinned {
				wantVersion = "17"
				if !strings.Contains(strings.Join(data.Warnings, "\n"), test.canonical+" template pins PostgreSQL 17") {
					t.Fatalf("missing pinned-version warning: %v", data.Warnings)
				}
			}
			if data.PrimaryIP != wantIP || adminChecks != wantChecks || data.EffectivePGVersion != wantVersion {
				t.Fatalf("mode behavior: ip=%s, admin checks=%d, version=%s", data.PrimaryIP, adminChecks, data.EffectivePGVersion)
			}
			rendered, err := os.ReadFile(filepath.Join(home, "pigsty.yml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"admin_ip: " + wantIP, "pg_version: " + wantVersion, "pg" + wantVersion + "-main"} {
				if !strings.Contains(string(rendered), want) {
					t.Fatalf("output lacks %q:\n%s", want, rendered)
				}
			}
		})
	}
}

func TestConfigureNativeDoesNotFallbackFromInvalidTemplate(t *testing.T) {
	home := t.TempDir()
	writeConfigureTemplate(t, home, "full", "wrong_root: {}\n")
	writeConfigureTemplate(t, home, "ha/full", "all: {}\n")
	result := ConfigureNative(ConfigureOptions{PigstyHome: home, Mode: "full", Region: "default", Skip: true, DisablePreflight: true})
	if result.Success || result.Code != output.CodeStyConfigureFailed {
		t.Fatalf("expected invalid root template to fail, got %+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, "pigsty.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed configure wrote output: %v", err)
	}
}

func writeConfigureTemplate(t *testing.T, home, mode, content string) {
	t.Helper()
	path := filepath.Join(home, "conf", filepath.FromSlash(mode)+".yml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write template %s: %v", mode, err)
	}
}
