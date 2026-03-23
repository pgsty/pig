package postgres

import (
	"os"
	"os/user"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"
)

func init() {
	// Ensure config.CurrentUser is set so IsDBSU works in tests
	if u, err := user.Current(); err == nil {
		config.CurrentUser = u.Username
	}
}

// testDBSU returns the current username so ReadFileAsDBSU/WriteFileAsDBSU
// bypass privilege escalation in tests.
func testDBSU() string {
	return config.CurrentUser
}

func TestReadAutoConf_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql.auto.conf")
	content := "# comment\nshared_buffers = '8192MB'\nwork_mem = '64MB'\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	params, lines, err := ReadAutoConf(path, testDBSU())
	if err != nil {
		t.Fatal(err)
	}

	if params["shared_buffers"] != "8192MB" {
		t.Errorf("shared_buffers: got %q", params["shared_buffers"])
	}
	if params["work_mem"] != "64MB" {
		t.Errorf("work_mem: got %q", params["work_mem"])
	}
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines, got %d", len(lines))
	}
}

func TestWriteAutoConf_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql.auto.conf")

	params := map[string]string{
		"shared_buffers": "4096MB",
		"work_mem":       "32MB",
	}

	if err := WriteAutoConf(path, testDBSU(), params, "pig pg tune: test"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "# pig pg tune: test") {
		t.Error("missing header comment")
	}
	if !strings.Contains(content, "shared_buffers = '4096MB'") {
		t.Error("missing shared_buffers")
	}
	if !strings.Contains(content, "work_mem = '32MB'") {
		t.Error("missing work_mem")
	}
}

func TestWriteAutoConf_MergeExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql.auto.conf")

	// Pre-populate with existing content
	existing := "# user comment\nshared_buffers = '2048MB'\nmax_connections = '50'\n"
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	// Write new params: update shared_buffers, add work_mem, leave max_connections alone
	params := map[string]string{
		"shared_buffers": "8192MB",
		"work_mem":       "64MB",
	}

	if err := WriteAutoConf(path, testDBSU(), params, "pig pg tune: merge test"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// shared_buffers should be updated
	if !strings.Contains(content, "shared_buffers = '8192MB'") {
		t.Error("shared_buffers not updated")
	}
	// Old value should be gone
	if strings.Contains(content, "2048MB") {
		t.Error("old shared_buffers value still present")
	}
	// max_connections should be preserved
	if !strings.Contains(content, "max_connections = '50'") {
		t.Error("max_connections should be preserved")
	}
	// work_mem should be appended
	if !strings.Contains(content, "work_mem = '64MB'") {
		t.Error("work_mem not appended")
	}
	// User comment should be preserved
	if !strings.Contains(content, "# user comment") {
		t.Error("user comment should be preserved")
	}
}

func TestWriteAutoConf_EmptyParams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql.auto.conf")

	if err := WriteAutoConf(path, "", map[string]string{}, "header"); err != nil {
		t.Fatal(err)
	}

	// File should not be created
	if _, err := os.Stat(path); err == nil {
		t.Error("file should not be created for empty params")
	}
}

func TestWriteAutoConf_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql.auto.conf")

	params := map[string]string{
		"shared_buffers": "4096MB",
		"work_mem":       "32MB",
	}

	// Write twice
	if err := WriteAutoConf(path, testDBSU(), params, "pig pg tune: test"); err != nil {
		t.Fatal(err)
	}
	if err := WriteAutoConf(path, testDBSU(), params, "pig pg tune: test"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should have exactly one of each parameter
	if strings.Count(content, "shared_buffers") != 1 {
		t.Errorf("shared_buffers appears %d times", strings.Count(content, "shared_buffers"))
	}
	if strings.Count(content, "work_mem") != 1 {
		t.Errorf("work_mem appears %d times", strings.Count(content, "work_mem"))
	}
	// Should have exactly one header
	if strings.Count(content, "# pig pg tune:") != 1 {
		t.Errorf("header appears %d times", strings.Count(content, "# pig pg tune:"))
	}
}
