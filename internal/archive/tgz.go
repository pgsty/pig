// Package archive contains the tar/gzip functionality Pig needs so bootstrap
// never depends on host tar or gzip executables.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxEntries = 100_000
	maxFile    = int64(16 << 30)
	maxTotal   = int64(256 << 30)
	maxStream  = int64(300 << 30)
)

// ExtractTarGz extracts a repository bundle below dst without invoking a host
// tar or gzip binary. One or more safe top-level entries are accepted; callers
// validate required content before committing the staged tree.
func ExtractTarGz(filename, dst string) error {
	return extractTarGzFile(filename, dst, extractionPolicy{label: "offline package"})
}

// ExtractTarGzRoot extracts a source archive whose directories and regular
// files must all live below requiredRoot.
func ExtractTarGzRoot(filename, dst, requiredRoot string) error {
	if requiredRoot != strings.TrimSpace(requiredRoot) || requiredRoot == "" || requiredRoot == "." || requiredRoot == ".." || filepath.Base(requiredRoot) != requiredRoot {
		return fmt.Errorf("invalid archive root: %q", requiredRoot)
	}
	return extractTarGzFile(filename, dst, extractionPolicy{label: "archive", requiredRoot: requiredRoot, preserveTimes: true})
}

type extractionPolicy struct {
	label         string
	requiredRoot  string
	preserveTimes bool
}

type extractedDirectory struct {
	path    string
	name    string
	modTime time.Time
}

type extractionResult struct {
	entries     int
	directories []extractedDirectory
}

func extractTarGzFile(filename, dst string, policy extractionPolicy) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", policy.label, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is not a regular file: %s", policy.label, filename)
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
		return fmt.Errorf("open %s: %w", policy.label, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	limited := &io.LimitedReader{R: gz, N: maxStream + 1}
	tr := tar.NewReader(limited)
	result, err := extractTarEntries(tr, root, policy)
	if err != nil {
		_ = gz.Close()
		return err
	}
	if _, err := io.Copy(io.Discard, limited); err != nil {
		_ = gz.Close()
		return fmt.Errorf("validate gzip stream: %w", err)
	}
	if limited.N == 0 {
		_ = gz.Close()
		return fmt.Errorf("%s expands beyond %d bytes", policy.label, maxStream)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("validate gzip trailer: %w", err)
	}
	if result.entries == 0 {
		return fmt.Errorf("%s is empty", policy.label)
	}
	if err := restoreDirectoryTimes(result.directories); err != nil {
		return err
	}
	return nil
}

func extractTarEntries(reader *tar.Reader, root string, policy extractionPolicy) (extractionResult, error) {
	var result extractionResult
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return result, fmt.Errorf("read tar stream: %w", err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		result.entries++
		if result.entries > maxEntries {
			return result, fmt.Errorf("%s has more than %d entries", policy.label, maxEntries)
		}
		target, err := archiveTarget(root, header.Name, policy)
		if err != nil {
			return result, err
		}
		if err := extractTarEntry(reader, root, target, header, policy, &total); err != nil {
			return result, err
		}
		if policy.preserveTimes && header.Typeflag == tar.TypeDir && !header.ModTime.IsZero() {
			result.directories = append(result.directories, extractedDirectory{path: target, name: header.Name, modTime: header.ModTime})
		}
	}
}

func restoreDirectoryTimes(directories []extractedDirectory) error {
	for _, directory := range directories {
		if err := os.Chtimes(directory.path, directory.modTime, directory.modTime); err != nil {
			return fmt.Errorf("restore modification time for %s: %w", directory.name, err)
		}
	}
	return nil
}

func extractTarEntry(reader *tar.Reader, root, target string, header *tar.Header, policy extractionPolicy, total *int64) error {
	if err := rejectSymlinkParents(root, target); err != nil {
		return err
	}
	mode := os.FileMode(header.Mode) & 0o777
	switch header.Typeflag {
	case tar.TypeDir:
		return extractTarDirectory(target, header.Name, mode)
	case tar.TypeReg, 0:
		return extractTarRegular(reader, target, header, mode, policy, total)
	default:
		return fmt.Errorf("unsupported tar entry type %d: %s", header.Typeflag, header.Name)
	}
}

func extractTarDirectory(target, name string, mode os.FileMode) error {
	if info, err := os.Lstat(target); err == nil && !info.IsDir() {
		return fmt.Errorf("archive directory conflicts with existing path: %s", name)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", name, err)
	}
	if err := os.MkdirAll(target, mode|0o700); err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	return nil
}

func extractTarRegular(reader *tar.Reader, target string, header *tar.Header, mode os.FileMode, policy extractionPolicy, total *int64) error {
	if header.Size < 0 || header.Size > maxFile || *total > maxTotal-header.Size {
		return fmt.Errorf("%s entry is too large: %s", policy.label, header.Name)
	}
	*total += header.Size
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", header.Name, err)
	}
	_, copyErr := io.CopyN(out, reader, header.Size)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("extract %s: %w", header.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", header.Name, closeErr)
	}
	if policy.preserveTimes && !header.ModTime.IsZero() {
		if err := os.Chtimes(target, header.ModTime, header.ModTime); err != nil {
			return fmt.Errorf("restore modification time for %s: %w", header.Name, err)
		}
	}
	return nil
}

func rejectSymlinkParents(root, target string) error {
	relative, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("archive path escapes destination: %s", target)
	}
	current := root
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to extract through symlink path component: %s", current)
		}
	}
	return nil
}

func archiveTarget(root, name string, policy extractionPolicy) (string, error) {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe tar path: %q", name)
	}
	parts := strings.SplitN(filepath.ToSlash(clean), "/", 2)
	if policy.requiredRoot != "" && parts[0] != policy.requiredRoot {
		return "", fmt.Errorf("%s must use a single %s/ root: %s", policy.label, policy.requiredRoot, name)
	}
	target := filepath.Join(root, clean)
	if !pathWithin(root, target) {
		return "", fmt.Errorf("tar path escapes destination: %s", name)
	}
	return target, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
