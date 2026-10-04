/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

File operations with privilege escalation fallback.
*/
package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// AtomicWrite writes a file through a temporary sibling and atomically replaces
// the destination only after the complete payload has been flushed and closed.
// The parent directory is synced after rename. The previous destination is
// preserved when an error occurs before the rename. Existing symlinks are
// rejected instead of followed or replaced. A successful write replaces the
// destination inode, so ownership becomes that of the executing (or privileged)
// user; callers that must preserve ownership need a different primitive.
func AtomicWrite(path string, mode os.FileMode, write func(io.Writer) error) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("file path is empty")
	}
	if write == nil {
		return fmt.Errorf("write callback is nil")
	}
	if _, err := existingFileMode(path, mode); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for %q: %w", path, err)
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()

	if err := write(tmp); err != nil {
		return fmt.Errorf("failed to write temporary file for %q: %w", path, err)
	}
	if err := tmp.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("failed to set temporary file mode for %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file for %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return fmt.Errorf("failed to close temporary file for %q: %w", path, err)
	}
	closed = true

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace file %q: %w", path, err)
	}
	if err := SyncDirectory(dir); err != nil {
		// Rename is the commit point and cannot be safely rolled back. Surface a
		// durability warning without turning a successful replacement into an
		// error whose "previous file preserved" guarantee would be false.
		logrus.Warnf("file %q replaced but parent directory sync failed: %v", path, err)
	}
	return nil
}

// SyncDirectory flushes directory metadata after a rename or removal.
func SyncDirectory(dir string) error {
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("failed to open parent directory %q: %w", dir, err)
	}
	syncErr := dirFile.Sync()
	closeErr := dirFile.Close()
	if syncErr != nil {
		return fmt.Errorf("failed to sync parent directory %q: %w", dir, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close parent directory %q: %w", dir, closeErr)
	}
	return nil
}

// AtomicWriteFile atomically replaces path with content using mode.
func AtomicWriteFile(path string, content []byte, mode os.FileMode) error {
	return AtomicWrite(path, mode, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
}

// existingFileMode returns the permission mode of an existing destination or
// fallback when it does not exist. Symlinks are rejected so callers cannot
// accidentally replace a link or write through it while inspecting its target.
func existingFileMode(path string, fallback os.FileMode) (os.FileMode, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fallback.Perm(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to inspect destination %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("refusing to replace symlink destination %q", path)
	}
	return info.Mode().Perm(), nil
}

// writePrivateTempFile stages content in the system temporary directory with
// owner-only permissions. The caller remains responsible for removing the path.
func writePrivateTempFile(pattern string, content []byte) (string, error) {
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	closed := false
	keep := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := tmp.Write(content); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return "", err
	}
	closed = true
	keep = true
	return tmpPath, nil
}

// PutFileMode atomically writes content and falls back to a privileged sibling
// install plus rename when the destination directory is not writable.
func PutFileMode(path string, content []byte, mode os.FileMode) error {
	logrus.Debugf("put file %q with mode %04o", path, mode.Perm())
	if _, err := existingFileMode(path, mode); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}

	if data, err := os.ReadFile(path); err == nil && bytes.Equal(data, content) {
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm() == mode.Perm() {
			return nil
		}
	}

	if err := AtomicWriteFile(path, content, mode); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrPermission) {
		return err
	}

	tmpPath, err := writePrivateTempFile(filepath.Base(path)+".tmp-*", content)
	if err != nil {
		return fmt.Errorf("failed to stage privileged-write content: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	modeArg := fmt.Sprintf("%04o", mode.Perm())
	privilegedTmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-"+filepath.Base(tmpPath))
	// Repeat the destination check with the privileges used for installation.
	// This also covers directories the caller cannot traverse. Pass paths as
	// arguments, never shell source, and continue to reject symlinks/special files.
	if err := SudoCommand([]string{"sh", "-c", `[ ! -L "$1" ] && { [ ! -e "$1" ] || [ -f "$1" ]; }`, "pig-write", path}); err != nil {
		return fmt.Errorf("refusing inaccessible or non-regular privileged destination %q: %w", path, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = QuietSudoCommand([]string{"rm", "-f", privilegedTmp})
		}
	}()
	if err := SudoCommand([]string{"install", "-m", modeArg, tmpPath, privilegedTmp}); err != nil {
		return fmt.Errorf("failed to install privileged temporary file %q: %w", privilegedTmp, err)
	}
	if err := SudoCommand([]string{"mv", "-f", privilegedTmp, path}); err != nil {
		return fmt.Errorf("failed to atomically replace privileged file %q: %w", path, err)
	}
	committed = true
	if err := SyncDirectory(filepath.Dir(path)); err != nil {
		logrus.Warnf("file %q replaced but parent directory sync failed: %v", path, err)
	}
	return nil
}

// IsPathWithin reports whether target is base itself or a lexical descendant of
// base. Unlike a string-prefix check, sibling names such as base-evil are rejected.
func IsPathWithin(base, target string) (bool, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false, fmt.Errorf("failed to resolve base path %q: %w", base, err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false, fmt.Errorf("failed to resolve target path %q: %w", target, err)
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false, fmt.Errorf("failed to compare paths %q and %q: %w", base, target, err)
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))), nil
}

// ReadFileAsDBSU reads a file using DBSU privilege escalation if needed.
// Execution strategy:
//   - If current user is DBSU: read directly
//   - Otherwise: use DBSUCommandStdout with cat
func ReadFileAsDBSU(path, dbsu string) (string, error) {
	if dbsu == "" {
		dbsu = GetDBSU("")
	}

	logrus.Debugf("ReadFileAsDBSU: path=%s, dbsu=%s, isDBSU=%v", path, dbsu, IsDBSU(dbsu))

	// If current user is DBSU, read directly
	if IsDBSU(dbsu) {
		logrus.Debugf("reading file directly as DBSU")
		content, err := os.ReadFile(path)
		if err != nil {
			logrus.Debugf("direct read failed: %v", err)
			return "", err
		}
		logrus.Debugf("direct read succeeded, content length=%d", len(content))
		return string(content), nil
	}

	// Use DBSUCommandStdout for privilege escalation
	logrus.Debugf("reading file via privilege escalation")
	return DBSUCommandStdout(dbsu, []string{"cat", path})
}

// WriteFileAsDBSU writes content to a file using DBSU privilege escalation.
// Execution strategy:
//   - If current user is DBSU: write directly with os.WriteFile
//   - Otherwise: pipe content through tee via buildDBSUCmd
func WriteFileAsDBSU(path, content, dbsu string) error {
	if dbsu == "" {
		dbsu = GetDBSU("")
	}

	logrus.Debugf("WriteFileAsDBSU: path=%s, dbsu=%s, isDBSU=%v", path, dbsu, IsDBSU(dbsu))

	if IsDBSU(dbsu) {
		logrus.Debugf("writing file directly as DBSU")
		return os.WriteFile(path, []byte(content), 0600)
	}

	logrus.Debugf("writing file via privilege escalation")
	cmd := buildDBSUCmd(dbsu, []string{"tee", path})
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = io.Discard
	return cmd.Run()
}
