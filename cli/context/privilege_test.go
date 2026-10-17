package context

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"
)

// Context probes must never consult the host's real sudo policy or password
// cache. Individual tests may replace these deny-only fixtures with captures.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pig-context-test-*")
	if err != nil {
		panic(err)
	}
	for _, name := range []string{"sudo", "su"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
			panic(err)
		}
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestContextPermissionProbePrivilegeMatrix(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-denied fixture requires an ordinary uid")
	}
	oldUser, oldFormat := config.CurrentUser, config.OutputFormat
	t.Cleanup(func() { config.CurrentUser, config.OutputFormat = oldUser, oldFormat })
	dir := t.TempDir()
	protected := filepath.Join(dir, "protected")
	_ = os.Mkdir(protected, 0700)
	path := filepath.Join(protected, "file")
	_ = os.WriteFile(path, []byte("data"), 0600)
	_ = os.Chmod(protected, 0000)
	t.Cleanup(func() { _ = os.Chmod(protected, 0700) })
	bin := t.TempDir()
	capture := filepath.Join(dir, "sudo-args")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PIG_TEST_CONTEXT_ARGS\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIG_TEST_CONTEXT_ARGS", capture)
	for _, mode := range []string{"text", "env", "json", "yaml"} {
		t.Run(mode, func(t *testing.T) {
			config.CurrentUser = "caller"
			config.OutputFormat = config.OUTPUT_TEXT
			t.Setenv("PIG_NON_INTERACTIVE", "")
			if mode == "env" {
				t.Setenv("PIG_NON_INTERACTIVE", "1")
			}
			if mode == "json" || mode == "yaml" {
				config.OutputFormat = mode
			}
			if exists, err := checkFileExistsAsDBSU("dbsu", path); exists || err != nil {
				t.Fatalf("exists=%v err=%v", exists, err)
			}
			args, _ := os.ReadFile(capture)
			if strings.HasPrefix(string(args), "-n\n") != (mode != "text") {
				t.Errorf("sudo flags=%q", args)
			}
			_ = os.Remove(capture)
			config.CurrentUser = "dbsu"
			if _, err := checkFileExistsAsDBSU("dbsu", path); err == nil {
				t.Fatal("same DBSU permission failure should remain a failure")
			}
			if _, err := os.Stat(capture); !os.IsNotExist(err) {
				t.Fatal("same DBSU invoked sudo")
			}
		})
	}
}
