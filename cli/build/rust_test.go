package build

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetupRustMirrorWritesCargoConfigWhenCargoAlreadyInstalled(t *testing.T) {
	homeDir := t.TempDir()
	cargoBinDir := filepath.Join(homeDir, ".cargo", "bin")
	if err := os.MkdirAll(cargoBinDir, 0o755); err != nil {
		t.Fatalf("failed to create cargo bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cargoBinDir, "cargo"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake cargo: %v", err)
	}

	var fetched []string
	var commands [][]string
	deps := rustSetupDeps{
		homeDir: homeDir,
		fetchScript: func(url string) (io.ReadCloser, error) {
			fetched = append(fetched, url)
			return nil, errors.New("rustup should not be fetched when cargo exists")
		},
		runCommand: func(args []string, env []string) error {
			commands = append(commands, append([]string(nil), args...))
			return nil
		},
	}

	if err := setupRust(false, true, deps); err != nil {
		t.Fatalf("setupRust returned error: %v", err)
	}
	if len(fetched) != 0 {
		t.Fatalf("rustup should not be fetched when cargo exists, got %v", fetched)
	}
	if len(commands) != 0 {
		t.Fatalf("rustup should not run when cargo exists, got %v", commands)
	}
	assertCargoMirrorConfig(t, filepath.Join(homeDir, ".cargo", "config.toml"))
}

func TestSetupRustMirrorInstallsWithMirrorBootstrapAndEnv(t *testing.T) {
	homeDir := t.TempDir()

	var fetched []string
	var gotArgs []string
	var gotEnv []string
	deps := rustSetupDeps{
		homeDir: homeDir,
		fetchScript: func(url string) (io.ReadCloser, error) {
			fetched = append(fetched, url)
			return io.NopCloser(strings.NewReader("#!/bin/sh\n")), nil
		},
		runCommand: func(args []string, env []string) error {
			gotArgs = append([]string(nil), args...)
			gotEnv = append([]string(nil), env...)
			if len(args) != 2 || args[1] != "-y" {
				t.Fatalf("rustup args = %v, want <script> -y", args)
			}
			info, err := os.Stat(args[0])
			if err != nil {
				t.Fatalf("rustup script does not exist: %v", err)
			}
			if info.Mode()&0o111 == 0 {
				t.Fatalf("rustup script should be executable, mode=%v", info.Mode())
			}
			if err := os.MkdirAll(filepath.Join(homeDir, ".cargo", "bin"), 0o755); err != nil {
				t.Fatalf("failed to create fake cargo dir: %v", err)
			}
			return os.WriteFile(filepath.Join(homeDir, ".cargo", "bin", "cargo"), []byte("#!/bin/sh\n"), 0o755)
		},
	}

	if err := setupRust(false, true, deps); err != nil {
		t.Fatalf("setupRust returned error: %v", err)
	}
	if !reflect.DeepEqual(fetched, []string{rustupMirrorScriptURL}) {
		t.Fatalf("fetched urls = %v, want mirror bootstrap only", fetched)
	}
	if len(gotArgs) == 0 {
		t.Fatalf("expected rustup command to run")
	}
	if !containsEnv(gotEnv, "RUSTUP_DIST_SERVER=https://rsproxy.cn") {
		t.Fatalf("missing RUSTUP_DIST_SERVER in env: %v", gotEnv)
	}
	if !containsEnv(gotEnv, "RUSTUP_UPDATE_ROOT=https://rsproxy.cn/rustup") {
		t.Fatalf("missing RUSTUP_UPDATE_ROOT in env: %v", gotEnv)
	}
	assertCargoMirrorConfig(t, filepath.Join(homeDir, ".cargo", "config.toml"))
}

func TestSetupRustMirrorFallsBackToDefaultBootstrap(t *testing.T) {
	homeDir := t.TempDir()

	var fetched []string
	deps := rustSetupDeps{
		homeDir: homeDir,
		fetchScript: func(url string) (io.ReadCloser, error) {
			fetched = append(fetched, url)
			if url == rustupMirrorScriptURL {
				return nil, errors.New("mirror unavailable")
			}
			return io.NopCloser(strings.NewReader("#!/bin/sh\n")), nil
		},
		runCommand: func(args []string, env []string) error {
			if err := os.MkdirAll(filepath.Join(homeDir, ".cargo", "bin"), 0o755); err != nil {
				t.Fatalf("failed to create fake cargo dir: %v", err)
			}
			return os.WriteFile(filepath.Join(homeDir, ".cargo", "bin", "cargo"), []byte("#!/bin/sh\n"), 0o755)
		},
	}

	if err := setupRust(false, true, deps); err != nil {
		t.Fatalf("setupRust returned error: %v", err)
	}
	want := []string{rustupMirrorScriptURL, rustupDefaultScriptURL}
	if !reflect.DeepEqual(fetched, want) {
		t.Fatalf("fetched urls = %v, want %v", fetched, want)
	}
}

func assertCargoMirrorConfig(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read cargo config: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		`replace-with = "rsproxy-sparse"`,
		`registry = "sparse+https://rsproxy.cn/index/"`,
		"git-fetch-with-cli = true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("cargo config missing %q:\n%s", want, text)
		}
	}
}

func containsEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
