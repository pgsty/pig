/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pb info structured output.
*/
package pgbackrest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pig/internal/output"
)

// Sample pgBackRest info JSON output for testing
var samplePgBackRestInfoJSON = `[
  {
    "archive": [
      {
        "database": {"id": 1, "repo-key": 1},
        "id": "17-1",
        "max": "000000010000000000000003",
        "min": "000000010000000000000001"
      }
    ],
    "backup": [
      {
        "annotation": null,
        "archive": {"start": "000000010000000000000001", "stop": "000000010000000000000001"},
        "backrest": {"format": 5, "version": "2.54.2"},
        "database": {"id": 1, "repo-key": 1},
        "error": false,
        "info": {
          "delta": 25719742,
          "repository": {"delta": 3123456, "delta-map": 0, "size": 3123456, "size-map": 0},
          "size": 25719742
        },
        "label": "20250101-120000F",
        "lsn": {"start": "0/2000028", "stop": "0/2000100"},
        "prior": null,
        "reference": null,
        "timestamp": {"start": 1735732800, "stop": 1735732860},
        "type": "full"
      },
      {
        "annotation": null,
        "archive": {"start": "000000010000000000000003", "stop": "000000010000000000000003"},
        "backrest": {"format": 5, "version": "2.54.2"},
        "database": {"id": 1, "repo-key": 1},
        "error": false,
        "info": {
          "delta": 1719742,
          "repository": {"delta": 123456, "delta-map": 0, "size": 3246912, "size-map": 0},
          "size": 25719742
        },
        "label": "20250102-120000I",
        "lsn": {"start": "0/3000028", "stop": "0/3000100"},
        "prior": "20250101-120000F",
        "reference": ["20250101-120000F"],
        "timestamp": {"start": 1735819200, "stop": 1735819260},
        "type": "incr"
      }
    ],
    "cipher": "none",
    "db": [
      {"id": 1, "repo-key": 1, "system-id": 7451234567890123456, "version": "17"}
    ],
    "name": "pg-meta",
    "repo": [
      {"cipher": "none", "key": 1, "status": {"code": 0, "message": "ok"}}
    ],
    "status": {"code": 0, "lock": {"backup": {"held": false}, "restore": {"held": false}}, "message": "ok"}
  }
]`

// TestPgBackRestInfoParse verifies the native info JSON schema still
// unmarshals into the package structs used for status branching.
func TestPgBackRestInfoParse(t *testing.T) {
	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(samplePgBackRestInfoJSON), &infos); err != nil {
		t.Fatalf("failed to parse sample info JSON: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 stanza, got %d", len(infos))
	}
	info := infos[0]
	if info.Name != "pg-meta" {
		t.Errorf("stanza name mismatch: got %q", info.Name)
	}
	if info.Status.Code != 0 {
		t.Errorf("status code mismatch: got %d", info.Status.Code)
	}
	if len(info.Backup) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(info.Backup))
	}
	if info.Backup[1].Prior == nil || *info.Backup[1].Prior != "20250101-120000F" {
		t.Errorf("incr backup prior mismatch: %v", info.Backup[1].Prior)
	}
}

// TestContainsAny tests the containsAny helper function.
func TestContainsAny(t *testing.T) {
	tests := []struct {
		s          string
		substrings []string
		want       bool
	}{
		{"config file not found", []string{"not found", "missing"}, true},
		{"config file not found", []string{"missing", "error"}, false},
		{"cannot detect stanza", []string{"no stanza found", "cannot detect stanza"}, true},
		{"", []string{"foo"}, false},
		{"foo", []string{}, false},
		{"abcdef", []string{"bcd"}, true},
	}

	for _, tt := range tests {
		got := containsAny(tt.s, tt.substrings...)
		if got != tt.want {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrings, got, tt.want)
		}
	}
}

func TestInfoResult_ConfigNotFound(t *testing.T) {
	cfg := &Config{
		ConfigPath: filepath.Join(t.TempDir(), "missing.conf"),
	}
	result := InfoResult(cfg, &InfoOptions{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatalf("expected failure result, got success: %v", result)
	}
	if result.Code != output.CodePbConfigNotFound {
		t.Fatalf("expected CodePbConfigNotFound, got %d", result.Code)
	}
}

func TestInfoResult_StanzaNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "pgbackrest.conf")
	content := []byte("[global]\nrepo1-path=/tmp\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg := &Config{
		ConfigPath: configPath,
	}
	result := InfoResult(cfg, &InfoOptions{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatalf("expected failure result, got success: %v", result)
	}
	if result.Code != output.CodePbStanzaNotFound {
		t.Fatalf("expected CodePbStanzaNotFound, got %d", result.Code)
	}
}
