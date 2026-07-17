package cmdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveConnectionPrecedenceAndSafeMetadata(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, "connection")
	secret := "postgres://owner:file-secret@127.0.0.1/cmdb"
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveConnection(ResolveOptions{
		ConnectionFile: path,
		LookupEnv: func(string) (string, bool) {
			return "postgres://owner:env-secret@127.0.0.1/cmdb", true
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source() != ConnectionFileSource || resolved.value != secret {
		t.Fatalf("unexpected file resolution: source=%q", resolved.Source())
	}
	if strings.Contains(string(resolved.Source()), "secret") {
		t.Fatal("safe source metadata leaked connection value")
	}

	resolved, err = ResolveConnection(ResolveOptions{LookupEnv: func(name string) (string, bool) {
		return "postgres://owner:env-secret@127.0.0.1/cmdb", name == connectionEnv
	}})
	if err != nil || resolved.Source() != ConnectionEnvSource {
		t.Fatalf("unexpected environment resolution: source=%q err=%v", resolved.Source(), err)
	}
	resolved, err = ResolveConnection(ResolveOptions{LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil || resolved.Source() != ConnectionMetaSource || resolved.value != "service=meta" {
		t.Fatalf("unexpected service resolution: source=%q err=%v", resolved.Source(), err)
	}
	_, err = ResolveConnection(ResolveOptions{LookupEnv: func(string) (string, bool) { return " ", true }})
	if !IsCode(err, "CMDB_CONNECTION_INVALID") {
		t.Fatalf("empty selected environment tier fell through: %v", err)
	}
}

func TestConnectionFileFailsClosed(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	valid := filepath.Join(directory, "valid")
	if err := os.WriteFile(valid, []byte("service=meta\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		make func(t *testing.T) string
		code string
	}{
		{
			name: "group readable",
			make: func(t *testing.T) string {
				path := filepath.Join(directory, "mode")
				if err := os.WriteFile(path, []byte("service=meta\n"), 0o640); err != nil {
					t.Fatal(err)
				}
				return path
			},
			code: "CMDB_CONNECTION_FILE_PERMISSION",
		},
		{
			name: "file symlink",
			make: func(t *testing.T) string {
				path := filepath.Join(directory, "link")
				if err := os.Symlink(valid, path); err != nil {
					t.Fatal(err)
				}
				return path
			},
			code: "CMDB_CONNECTION_FILE_INVALID",
		},
		{
			name: "parent symlink",
			make: func(t *testing.T) string {
				realParent := filepath.Join(directory, "real")
				if err := os.Mkdir(realParent, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(realParent, "dsn"), []byte("service=meta\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				linkParent := filepath.Join(directory, "parent-link")
				if err := os.Symlink(realParent, linkParent); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(linkParent, "dsn")
			},
			code: "CMDB_CONNECTION_FILE_INVALID",
		},
		{
			name: "multiple lines",
			make: func(t *testing.T) string {
				path := filepath.Join(directory, "lines")
				if err := os.WriteFile(path, []byte("service=one\nservice=two\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			code: "CMDB_CONNECTION_FILE_INVALID",
		},
		{
			name: "too large",
			make: func(t *testing.T) string {
				path := filepath.Join(directory, "large")
				if err := os.WriteFile(path, []byte(strings.Repeat("x", connectionFileLimit+1)), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			code: "CMDB_CONNECTION_FILE_INVALID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolveConnection(ResolveOptions{ConnectionFile: test.make(t)})
			if !IsCode(err, test.code) {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
}

func symlinkFreeTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestConnectionErrorsNeverExposeSecret(t *testing.T) {
	secret := "unprintable-password"
	resolved := ResolvedConnection{source: ConnectionEnvSource, value: "postgres://owner:" + secret + "@%zz"}
	_, err := Open(t.Context(), resolved, OpenOptions{})
	if err == nil {
		t.Fatal("expected invalid connection error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "%zz") {
		t.Fatalf("connection error leaked unsafe input: %v", err)
	}
}

func TestTimeoutAndPoolBounds(t *testing.T) {
	got := normalizeTimeouts(Timeouts{
		Connect:     time.Hour,
		Statement:   time.Hour,
		Lock:        time.Hour,
		Transaction: time.Hour,
	})
	if got.Connect != 30*time.Second || got.Statement != 2*time.Minute ||
		got.Lock != 30*time.Second || got.Transaction != 5*time.Minute {
		t.Fatalf("timeouts were not bounded: %+v", got)
	}
	if fallback := normalizeTimeouts(Timeouts{}); fallback != defaultTimeouts {
		t.Fatalf("unexpected timeout defaults: %+v", fallback)
	}
}
