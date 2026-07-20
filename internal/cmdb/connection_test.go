package cmdb

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	resolved := ResolvedConnection{source: ConnectionURLSource, value: "postgres://owner:" + secret + "@%zz"}
	_, err := Open(t.Context(), resolved, OpenOptions{})
	if err == nil {
		t.Fatal("expected invalid connection error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "%zz") {
		t.Fatalf("connection error leaked unsafe input: %v", err)
	}
}

func TestTimeoutBounds(t *testing.T) {
	got := normalizeTimeouts(Timeouts{
		Connect: time.Hour, Statement: time.Hour, Lock: time.Hour, Transaction: time.Hour,
	})
	if got.Connect != 30*time.Second || got.Statement != 2*time.Minute ||
		got.Lock != 30*time.Second || got.Transaction != 5*time.Minute {
		t.Fatalf("timeouts were not bounded: %+v", got)
	}
	if fallback := normalizeTimeouts(Timeouts{}); fallback != defaultTimeouts {
		t.Fatalf("unexpected timeout defaults: %+v", fallback)
	}
}
