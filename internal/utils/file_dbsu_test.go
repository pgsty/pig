package utils

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"pig/internal/config"
)

// This executes the real writer script, but never invokes host sudo or su.
func fakeDBSUWriter(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := config.CurrentUser
	config.CurrentUser = "pig-caller"
	t.Cleanup(func() { config.CurrentUser = old })
	sudo := `#!/bin/sh
[ "$1" != -n ] || shift
shift 4
if [ -n "$PIG_TEST_ATTACK_TARGET" ]; then
  ln -s "$PIG_TEST_VICTIM" "$PIG_TEST_ATTACK_TARGET.tmp.$$" || exit 1
fi
exec "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestWriteFileAsDBSUSafeReplacement(t *testing.T) {
	fakeDBSUWriter(t)
	for _, kind := range []string{"new", "regular", "directory", "symlink", "dangling", "hardlink", "predictable-temp"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "target with ' quotes")
			victim := filepath.Join(dir, "victim")
			if err := os.WriteFile(victim, []byte("untouched"), 0644); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "regular":
				if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(victim, path); err != nil {
					t.Fatal(err)
				}
			case "dangling":
				if err := os.Symlink(filepath.Join(dir, "missing"), path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(victim, path); err != nil {
					t.Fatal(err)
				}
			case "predictable-temp":
				t.Setenv("PIG_TEST_ATTACK_TARGET", path)
				t.Setenv("PIG_TEST_VICTIM", victim)
			}
			err := WriteFileAsDBSU(path, "new payload", "pig-dbsu")
			reject := kind == "directory" || kind == "symlink" || kind == "dangling"
			if (err != nil) != reject {
				t.Errorf("error=%v, reject=%v", err, reject)
			}
			if got, e := os.ReadFile(victim); e != nil || string(got) != "untouched" {
				t.Errorf("non-target changed: %q, %v", got, e)
			}
			if reject {
				if kind == "directory" {
					entries, _ := os.ReadDir(path)
					if len(entries) != 0 {
						t.Error("directory target received staged file")
					}
				} else if info, e := os.Lstat(path); e != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Error("symlink was replaced")
				}
			} else if err == nil {
				data, _ := os.ReadFile(path)
				info, _ := os.Stat(path)
				if string(data) != "new payload" || info.Mode().Perm() != 0600 {
					t.Errorf("content=%q mode=%v", data, info.Mode())
				}
				if got := int(info.Sys().(*syscall.Stat_t).Uid); got != os.Geteuid() {
					t.Errorf("owner=%d want effective uid %d", got, os.Geteuid())
				}
			}
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.Contains(e.Name(), ".tmp-") {
					t.Errorf("leaked temporary file: %s", e.Name())
				}
			}
		})
	}
}

func TestWriteFileAsDBSUFailureCleanup(t *testing.T) {
	bin := fakeDBSUWriter(t)
	// A partial stdin copy must leave the original destination intact and remove staging.
	if err := os.WriteFile(filepath.Join(bin, "cat"), []byte("#!/bin/sh\nprintf partial\nexit 27\n"), 0700); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAsDBSU(path, "payload", "pig-dbsu"); err == nil {
		t.Fatal("copy failure reported success")
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Fatalf("old target changed: %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("failed write leaked staging: %v", entries)
	}
}
