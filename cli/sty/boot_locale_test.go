package sty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pig/internal/config"
)

func setupBootstrapLocaleTest(t *testing.T) string {
	t.Helper()
	origVendor, origVersion := config.OSVendor, config.OSVersion
	origFile, origLookPath, origRun := bootLocaleGenFile, bootLocaleLookPath, bootLocaleRun
	t.Cleanup(func() {
		config.OSVendor, config.OSVersion = origVendor, origVersion
		bootLocaleGenFile, bootLocaleLookPath, bootLocaleRun = origFile, origLookPath, origRun
	})
	config.OSVendor, config.OSVersion = "debian", "12"
	bootLocaleGenFile = filepath.Join(t.TempDir(), "locale.gen")
	if err := os.WriteFile(bootLocaleGenFile, []byte("# C.UTF-8 UTF-8\n# en_US.UTF-8 UTF-8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bootLocaleLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	return bootLocaleGenFile
}

func TestNeedsDebianBootstrapLocale(t *testing.T) {
	origVendor, origVersion := config.OSVendor, config.OSVersion
	t.Cleanup(func() { config.OSVendor, config.OSVersion = origVendor, origVersion })
	for _, tc := range []struct {
		vendor, version string
		want            bool
	}{
		{vendor: "debian", version: "12", want: true},
		{vendor: "Debian", version: "13", want: true},
		{vendor: "debian", version: "11"},
		{vendor: "ubuntu", version: "12"},
	} {
		config.OSVendor, config.OSVersion = tc.vendor, tc.version
		if got := needsDebianBootstrapLocale(); got != tc.want {
			t.Fatalf("needsDebianBootstrapLocale(%q, %q)=%t, want %t", tc.vendor, tc.version, got, tc.want)
		}
	}
}

func TestEnableBootstrapLocaleUncommentsDefinition(t *testing.T) {
	path := setupBootstrapLocaleTest(t)
	if err := enableBootstrapLocale(path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "\nen_US.UTF-8 UTF-8\n") || strings.Contains(string(content), "# en_US.UTF-8") {
		t.Fatalf("locale.gen = %q", content)
	}
	if err := enableBootstrapLocale(path); err != nil {
		t.Fatalf("idempotent enable failed: %v", err)
	}
}

func TestEnsureDebianBootstrapLocaleMatchesDockerSequence(t *testing.T) {
	path := setupBootstrapLocaleTest(t)
	var calls [][]string
	localeCalls := 0
	bootLocaleRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		call := append([]string{filepath.Base(name)}, args...)
		calls = append(calls, call)
		if filepath.Base(name) == "locale" {
			localeCalls++
			if localeCalls == 1 {
				return []byte("C\nC.utf8\n"), nil
			}
			return []byte("C\nC.utf8\nen_US.utf8\n"), nil
		}
		return nil, nil
	}
	repaired, err := repairDebianBootstrapLocale(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("missing locale was not reported as repaired")
	}
	want := [][]string{
		{"locale", "-a"},
		{"locale-gen", "en_US.UTF-8"},
		{"update-locale", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8"},
		{"locale", "-a"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("locale calls = %v, want %v", calls, want)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "en_US.UTF-8 UTF-8") {
		t.Fatalf("locale definition was not enabled: %q, %v", content, err)
	}
}

func TestEnsureDebianBootstrapLocaleSkipsExistingLocale(t *testing.T) {
	setupBootstrapLocaleTest(t)
	bootLocaleGenFile = filepath.Join(t.TempDir(), "missing-locale.gen")
	var calls [][]string
	bootLocaleRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{filepath.Base(name)}, args...))
		return []byte("C\nen_US.utf8\n"), nil
	}
	repaired, err := repairDebianBootstrapLocale(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repaired {
		t.Fatal("existing locale was reported as repaired")
	}
	want := [][]string{{"locale", "-a"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("locale calls = %v, want %v", calls, want)
	}
}

func TestEnsureDebianBootstrapLocaleRequiresGeneratedLocale(t *testing.T) {
	setupBootstrapLocaleTest(t)
	bootLocaleRun = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if filepath.Base(name) == "locale" {
			return []byte("C\nC.utf8\n"), nil
		}
		return nil, nil
	}
	if _, err := repairDebianBootstrapLocale(context.Background()); err == nil || !strings.Contains(err.Error(), "was not generated") {
		t.Fatalf("missing generated locale error = %v", err)
	}
}

func TestEnsureDebianBootstrapLocaleReportsCommandFailure(t *testing.T) {
	setupBootstrapLocaleTest(t)
	bootLocaleRun = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if filepath.Base(name) == "locale-gen" {
			return []byte("generation failed"), errors.New("exit 1")
		}
		return nil, nil
	}
	if _, err := repairDebianBootstrapLocale(context.Background()); err == nil || !strings.Contains(err.Error(), "generation failed") {
		t.Fatalf("locale-gen failure = %v", err)
	}
}
