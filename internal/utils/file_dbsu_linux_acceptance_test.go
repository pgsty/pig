//go:build linux

package utils

import (
	"os"
	"os/user"
	"path/filepath"
	"pig/internal/config"
	"strconv"
	"syscall"
	"testing"
)

// Opt-in in a disposable Linux container: genuine uid/permission evidence,
// separate from the fake-sudo unit tests. Never uses sudo or a password cache.
func TestWriteFileAsDBSULinuxIdentity(t *testing.T) {
	dbsu := os.Getenv("PIG_TEST_REAL_DBSU")
	if dbsu == "" || os.Geteuid() != 0 {
		t.Skip("requires disposable root Linux environment and PIG_TEST_REAL_DBSU")
	}
	account, err := user.Lookup(dbsu)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := strconv.Atoi(account.Uid)
	old := config.CurrentUser
	config.CurrentUser = "root"
	t.Cleanup(func() { config.CurrentUser = old })
	dir, err := os.MkdirTemp("", "pig-dbsu-permissions-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700); _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("untouched"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAsDBSU(target, "first", dbsu); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 || int(info.Sys().(*syscall.Stat_t).Uid) != uid {
		t.Fatalf("mode/owner=%v/%d", info.Mode(), info.Sys().(*syscall.Stat_t).Uid)
	}
	// Replacing a hardlink must preserve the old inode at every other name.
	if err := os.Link(target, filepath.Join(dir, "old-inode")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAsDBSU(target, "second", dbsu); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "old-inode")); string(data) != "first" {
		t.Fatal("non-target hardlink changed")
	}
	for _, kind := range []string{"directory", "symlink", "permission"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(dir, kind)
			switch kind {
			case "directory":
				err = os.Mkdir(path, 0777)
			case "symlink":
				err = os.Symlink(victim, path)
			case "permission":
				path = target
				err = os.Chmod(dir, 0555)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = WriteFileAsDBSU(path, "must not be written", dbsu)
			_ = os.Chmod(dir, 0777)
			if err == nil {
				t.Fatalf("%s reported success", kind)
			}
			if got, _ := os.ReadFile(victim); string(got) != "untouched" {
				t.Fatal("victim changed")
			}
			if got, _ := os.ReadFile(target); string(got) != "second" {
				t.Fatal("original target changed")
			}
			if kind == "directory" {
				entries, _ := os.ReadDir(path)
				if len(entries) != 0 {
					t.Fatal("directory received temporary file")
				}
			}
		})
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 5 {
		t.Fatalf("temporary leak: %v", entries)
	}
	t.Logf("real Linux su -> %s uid=%d: 0600 ownership, exact replacement, hardlink preservation, directory/symlink/permission failures verified", dbsu, uid)
}
