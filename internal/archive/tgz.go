// Package archive contains the tar/gzip functionality Pig needs so bootstrap
// never depends on host tar or gzip executables.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxEntries = 100_000
	maxFile    = int64(16 << 30)
	maxTotal   = int64(256 << 30)
	maxStream  = int64(300 << 30)
)

// ExtractTarGz extracts a Pigsty offline package below dst without invoking a
// host tar or gzip binary. Offline packages must contain one pigsty/ root and
// only directories and regular files.
func ExtractTarGz(filename, dst string) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("inspect offline package: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("offline package is not a regular file: %s", filename)
	}
	root, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve extraction directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open offline package: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	limited := &io.LimitedReader{R: gz, N: maxStream + 1}
	tr := tar.NewReader(limited)

	entries := 0
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = gz.Close()
			return fmt.Errorf("read tar stream: %w", err)
		}
		entries++
		if entries > maxEntries {
			_ = gz.Close()
			return fmt.Errorf("offline package has more than %d entries", maxEntries)
		}
		target, err := offlineTarget(root, hdr.Name)
		if err != nil {
			_ = gz.Close()
			return err
		}
		mode := os.FileMode(hdr.Mode) & 0o777
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				_ = gz.Close()
				return fmt.Errorf("create %s: %w", hdr.Name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size < 0 || hdr.Size > maxFile || total > maxTotal-hdr.Size {
				_ = gz.Close()
				return fmt.Errorf("offline package entry is too large: %s", hdr.Name)
			}
			total += hdr.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = gz.Close()
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				_ = gz.Close()
				return fmt.Errorf("create %s: %w", hdr.Name, err)
			}
			_, copyErr := io.CopyN(out, tr, hdr.Size)
			closeErr := out.Close()
			if copyErr != nil {
				_ = gz.Close()
				return fmt.Errorf("extract %s: %w", hdr.Name, copyErr)
			}
			if closeErr != nil {
				_ = gz.Close()
				return fmt.Errorf("close %s: %w", hdr.Name, closeErr)
			}
		default:
			_ = gz.Close()
			return fmt.Errorf("unsupported tar entry type %d: %s", hdr.Typeflag, hdr.Name)
		}
	}
	if _, err := io.Copy(io.Discard, limited); err != nil {
		_ = gz.Close()
		return fmt.Errorf("validate gzip stream: %w", err)
	}
	if limited.N == 0 {
		_ = gz.Close()
		return fmt.Errorf("offline package expands beyond %d bytes", maxStream)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("validate gzip trailer: %w", err)
	}
	if entries == 0 {
		return errors.New("offline package is empty")
	}
	return nil
}

func offlineTarget(root, name string) (string, error) {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe tar path: %q", name)
	}
	parts := strings.SplitN(filepath.ToSlash(clean), "/", 2)
	if parts[0] != "pigsty" {
		return "", fmt.Errorf("offline package must use a single pigsty/ root: %s", name)
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("tar path escapes destination: %s", name)
	}
	return target, nil
}
