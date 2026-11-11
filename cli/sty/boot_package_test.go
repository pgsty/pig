package sty

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestResolveBootPackageValidatesSharedResolverSelection(t *testing.T) {
	for _, relative := range []string{"var/tmp/pkg.tgz", "tmp/pkg.tgz"} {
		t.Run(relative, func(t *testing.T) {
			setupBootTest(t)
			t.Setenv("SUDO_UID", strconv.Itoa(os.Getuid()))
			selected := filepath.Join(t.TempDir(), relative)
			if err := os.MkdirAll(filepath.Dir(selected), 0755); err != nil {
				t.Fatal(err)
			}
			if err := writeTinyFile(selected); err != nil {
				t.Fatal(err)
			}
			bootResolvePackage = func(path string) (string, error) {
				if path != "" {
					t.Fatalf("automatic discovery requires empty input, got %q", path)
				}
				return selected, nil
			}
			got, err := resolveBootPackage("")
			if err != nil || got != selected {
				t.Fatalf("selected automatic package = %q, %v; want %q", got, err, selected)
			}

			if err := os.Chmod(selected, 0666); err != nil {
				t.Fatal(err)
			}
			data := &BootData{}
			got, _, failure := resolveBootstrapOfflineSource("", data)
			if got != "" || failure != nil || len(data.Warnings) != 1 || !strings.Contains(data.Warnings[0], selected) {
				t.Fatalf("unsafe selection: path=%q failure=%+v warnings=%v", got, failure, data.Warnings)
			}
			if err := os.Remove(selected); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "missing.tgz"), selected); err != nil {
				t.Fatal(err)
			}
			if _, err := resolveBootPackage(""); !errors.Is(err, errUntrustedAutomaticPackage) {
				t.Fatalf("automatic symlink error = %v", err)
			}
		})
	}
}

func TestResolveBootPackagePropagatesSharedResolverErrors(t *testing.T) {
	setupBootTest(t)
	selected := filepath.Join(t.TempDir(), "pkg.tgz")
	for _, cause := range []error{os.ErrNotExist, os.ErrPermission} {
		bootResolvePackage = func(string) (string, error) {
			return selected, &os.PathError{Op: "lstat", Path: selected, Err: cause}
		}
		got, err := resolveBootPackage("")
		if got != "" {
			t.Fatalf("resolver error selected %q", got)
		}
		if errors.Is(cause, os.ErrNotExist) {
			if err != nil {
				t.Fatalf("absent automatic package should permit online boot: %v", err)
			}
		} else if !errors.Is(err, cause) {
			t.Fatalf("inspection failure lost: %v", err)
		}
	}
}

func TestResolveBootPackageExplicitPathBypassesAutomaticDiscovery(t *testing.T) {
	setupBootTest(t)
	bootResolvePackage = func(string) (string, error) {
		t.Fatal("explicit package triggered automatic discovery")
		return "", nil
	}
	explicit := filepath.Join(t.TempDir(), "pkg.tgz")
	if _, err := resolveBootPackage(explicit); err == nil {
		t.Fatal("missing explicit package was accepted")
	}
	if err := writeTinyFile(explicit); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveBootPackage(explicit); err != nil || got != explicit {
		t.Fatalf("explicit package = %q, %v; want %q", got, err, explicit)
	}
}
