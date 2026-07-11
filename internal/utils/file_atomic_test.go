package utils

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWritePreservesPreviousFileOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog")
	if err := os.WriteFile(path, []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("validation failed")
	err := AtomicWrite(path, 0600, func(w io.Writer) error {
		if _, err := w.Write([]byte("partial")); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("AtomicWrite error = %v, want %v", err, wantErr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous" {
		t.Fatalf("previous file was changed: %q", got)
	}
}

func TestAtomicWriteRejectsSymlinkAndPreservesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("target-content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	called := false
	err := AtomicWrite(link, 0644, func(w io.Writer) error {
		called = true
		_, err := w.Write([]byte("replacement"))
		return err
	})
	if err == nil {
		t.Fatal("AtomicWrite should reject an existing symlink")
	}
	if called {
		t.Fatal("write callback ran for a rejected symlink")
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("symlink changed: target=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "target-content" {
		t.Fatalf("symlink target changed: content=%q err=%v", got, err)
	}
}

func TestPutFileModeReplacesContentAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.yml")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := PutFileMode(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatalf("content = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0600 {
		t.Fatalf("mode = %04o, want 0600", gotMode)
	}
}

func TestPutFileModeRejectsSymlinkBeforeContentFastPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("same"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if err := PutFileMode(link, []byte("same"), 0600); err == nil {
		t.Fatal("PutFileMode should reject a symlink even when content matches")
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("symlink changed: target=%q err=%v", got, err)
	}
}

func TestWritePrivateTempFileUsesOwnerOnlyMode(t *testing.T) {
	path, err := writePrivateTempFile("pig-private-*", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("staging mode = %04o, want 0600", got)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "secret" {
		t.Fatalf("staged content = %q, err=%v", got, err)
	}
}

func TestPutFilePreservesExistingMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PutFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("mode = %04o, want preserved 0600", got)
	}
}

func TestIsPathWithinRejectsSiblingPrefixAndTraversal(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	for _, tt := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "base", target: base, want: true},
		{name: "child", target: filepath.Join(base, "repo", "file"), want: true},
		{name: "sibling prefix", target: filepath.Join(root, "base-evil", "file"), want: false},
		{name: "parent", target: filepath.Join(base, "..", "outside"), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsPathWithin(base, tt.target)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("IsPathWithin(%q, %q) = %v, want %v", base, tt.target, got, tt.want)
			}
		})
	}
}
