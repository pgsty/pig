package pgbackrest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestLsResultBackupEmbedsNativePgBackRestJSON(t *testing.T) {
	configPath := writePgBackRestConfig(t, `
[global]
repo1-path=/backup

[pg-meta]
pg1-path=/pg/data
pg1-port=5432
`)
	installFakePgBackRest(t, samplePgBackRestInfoJSON)
	withDirectDBSU(t, "testuser")

	result := LsResult(&Config{ConfigPath: configPath, DbSU: "testuser"}, &LsOptions{Type: "backup"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success result, got %v", result)
	}

	out, err := result.JSON()
	if err != nil {
		t.Fatalf("result JSON failed: %v", err)
	}
	if strings.Contains(string(out), "captured_output") {
		t.Fatalf("pb ls structured output should not expose captured_output: %s", out)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal result json failed: %v", err)
	}
	data := decoded["data"].(map[string]any)
	if data["type"] != "backup" {
		t.Fatalf("data.type = %v, want backup", data["type"])
	}
	backups, ok := data["backups"].([]any)
	if !ok {
		t.Fatalf("data.backups should be native array, got %T", data["backups"])
	}
	if len(backups) != 1 {
		t.Fatalf("backup stanza count = %d, want 1", len(backups))
	}
	first := backups[0].(map[string]any)
	if first["name"] != "pg-meta" {
		t.Fatalf("backup stanza name = %v, want pg-meta", first["name"])
	}
}

func TestLsResultRepoParsesConfigWithoutSecrets(t *testing.T) {
	configPath := writePgBackRestConfig(t, `
[global]
repo1-type=s3
repo1-s3-bucket=pgsql
repo1-s3-endpoint=sss.pigsty
repo1-s3-key=AKIA...
repo1-s3-key-secret=super-secret
repo2-path=/backup

[pg-meta]
pg1-path=/pg/data
`)

	result := LsResult(&Config{ConfigPath: configPath, DbSU: "testuser"}, &LsOptions{Type: "repo"})
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %v", result)
	}
	out, err := result.JSON()
	if err != nil {
		t.Fatalf("result JSON failed: %v", err)
	}
	if strings.Contains(string(out), "super-secret") || strings.Contains(string(out), "AKIA") {
		t.Fatalf("repo list should not leak credential fields: %s", out)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal result json failed: %v", err)
	}
	data := decoded["data"].(map[string]any)
	repos, ok := data["repositories"].([]any)
	if !ok {
		t.Fatalf("data.repositories should be array, got %T", data["repositories"])
	}
	if len(repos) != 2 {
		t.Fatalf("repo count = %d, want 2", len(repos))
	}
	repo1 := repos[0].(map[string]any)
	if repo1["name"] != "repo1" || repo1["type"] != "s3" {
		t.Fatalf("repo1 parsed incorrectly: %v", repo1)
	}
	if repo1["bucket"] != "pgsql" || repo1["endpoint"] != "sss.pigsty" || repo1["uri"] != "s3://pgsql" {
		t.Fatalf("repo1 location fields parsed incorrectly: %v", repo1)
	}
	repo2 := repos[1].(map[string]any)
	if repo2["type"] != "posix" || repo2["path"] != "/backup" {
		t.Fatalf("repo2 parsed incorrectly: %v", repo2)
	}
}

func TestLsResultStanzaParsesConfig(t *testing.T) {
	configPath := writePgBackRestConfig(t, `
[global]
repo1-path=/backup

[pg-meta]
pg1-path=/pg/data
pg1-port=5432

[pg-test]
pg1-path=/data/test
`)

	result := LsResult(&Config{ConfigPath: configPath, DbSU: "testuser"}, &LsOptions{Type: "stanza"})
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %v", result)
	}
	out, err := result.JSON()
	if err != nil {
		t.Fatalf("result JSON failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal result json failed: %v", err)
	}
	data := decoded["data"].(map[string]any)
	stanzas, ok := data["stanzas"].([]any)
	if !ok {
		t.Fatalf("data.stanzas should be array, got %T", data["stanzas"])
	}
	if len(stanzas) != 2 {
		t.Fatalf("stanza count = %d, want 2", len(stanzas))
	}
	first := stanzas[0].(map[string]any)
	if first["name"] != "pg-meta" || first["pg_path"] != "/pg/data" || first["pg_port"] != "5432" {
		t.Fatalf("first stanza parsed incorrectly: %v", first)
	}
	second := stanzas[1].(map[string]any)
	if second["name"] != "pg-test" || second["pg_path"] != "/data/test" || second["pg_port"] != "5432" {
		t.Fatalf("second stanza parsed incorrectly: %v", second)
	}
}

func writePgBackRestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pgbackrest.conf")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	return path
}

func installFakePgBackRest(t *testing.T, stdout string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pgbackrest")
	script := "#!/bin/sh\ncat <<'JSON'\n" + stdout + "\nJSON\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pgbackrest failed: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func withDirectDBSU(t *testing.T, user string) {
	t.Helper()
	orig := config.CurrentUser
	config.CurrentUser = user
	t.Cleanup(func() { config.CurrentUser = orig })
}
