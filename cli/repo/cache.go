package repo

import (
	"archive/tar"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type cacheWalkContext struct {
	absBase   string
	tarWriter *tar.Writer
	total     int
	processed int
}

func (c *cacheWalkContext) walk(filePath string, info os.FileInfo, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}

	// Ensure file path is within absBase (no "../" or sibling-prefix escapes).
	inside, err := utils.IsPathWithin(c.absBase, filePath)
	if err != nil {
		return err
	}
	if !inside {
		return fmt.Errorf("file path escapes base directory: %s", filePath)
	}
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get abs path for %s: %v", filePath, err)
	}

	// Compute tar header name (relative to base dir).
	relPath, err := filepath.Rel(c.absBase, absFilePath)
	if err != nil {
		return fmt.Errorf("failed to get relative path for %s: %v", filePath, err)
	}

	var file *os.File
	if info.Mode().IsRegular() {
		file, err = os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}
	}

	linkTarget := ""
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err = os.Readlink(filePath)
		if err != nil {
			return fmt.Errorf("failed to read symlink %s: %v", filePath, err)
		}
	}

	// Create the header only after a regular source file has been opened. This
	// prevents a tar entry from promising bytes that can no longer be read.
	header, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		if file != nil {
			_ = file.Close()
		}
		return fmt.Errorf("failed to create file info header for %s: %v", filePath, err)
	}
	header.Name = relPath

	// Write header.
	if err := c.tarWriter.WriteHeader(header); err != nil {
		if file != nil {
			_ = file.Close()
		}
		return fmt.Errorf("failed to write tar header for %s: %v", filePath, err)
	}

	// If it's a regular file, write its content.
	if file != nil {
		_, copyErr := io.Copy(c.tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to copy file content %s: %w", filePath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close source file %s: %w", filePath, closeErr)
		}

		c.processed++
		printProgressAndFile(c.processed, c.total, relPath)
	}

	return nil
}

// Cache creates a compressed tarball of specified subdirectories under dirPath.
// 1) Closes source files immediately and propagates all close errors.
// 2) Preserves symbolic links and empty directories in the tarball.
// 3) Ensures no upper-level references (e.g. "../../") are included.
// 4) Prints a warning if a specified subdirectory or file does not exist, then skips it.
// 5) Shows progress based on the total number of regular files and prints the file being processed.
// 6) Prints total file size in GiB before starting the compression process.
func Cache(dirPath, pkgPath string, repos []string) error {
	logrus.Infof("caching repo %s %s to %s", dirPath, strings.Join(repos, ","), pkgPath)

	// Use defaults if not provided
	if dirPath == "" {
		dirPath = "/www"
	}
	if pkgPath == "" {
		pkgPath = "/tmp/pkg.tgz"
	}
	if len(repos) == 0 {
		repos = []string{"pigsty"}
	}

	// Get absolute base directory (used to prevent upper-level references)
	absBase, err := filepath.Abs(dirPath)
	if err != nil {
		return fmt.Errorf("failed to determine abs path of dirPath %s: %v", dirPath, err)
	}

	// Check if base dir exists
	baseInfo, err := os.Stat(absBase)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnf("base directory does not exist: %s", absBase)
			return fmt.Errorf("invalid base directory: %s", absBase)
		}
		return fmt.Errorf("failed to access base directory %s: %w", absBase, err)
	}
	if !baseInfo.IsDir() {
		logrus.Warnf("base directory is not a directory: %s", absBase)
		return fmt.Errorf("invalid base directory: %s", absBase)
	}
	if err := ensurePackageOutsideRepos(absBase, pkgPath, repos); err != nil {
		return err
	}

	// Count the total number of regular files and total size for progress & estimate
	totalFiles, totalSize, err := countFiles(absBase, repos)
	if err != nil {
		return fmt.Errorf("failed to count files: %v", err)
	}

	// Print estimated total file size in GiB (1 GiB = 1024^3 bytes)
	logrus.Infof("Estimated total file size: %.2f GiB", bytesToGiB(totalSize))

	// Validate the output path without removing a previous good package.
	packageMode, err := preparePackageOutput(pkgPath)
	if err != nil {
		return err
	}

	if err := utils.AtomicWrite(pkgPath, packageMode, func(out io.Writer) error {
		return writeCacheArchive(out, absBase, repos, totalFiles)
	}); err != nil {
		return err
	}

	// Only print newline and package info in text mode
	if !config.IsStructuredOutput() {
		fmt.Println()
		logrus.Infof("offline package created")
		if err := printFinalPackageInfo(pkgPath); err != nil {
			// The atomic replacement has already committed at this point. Metadata
			// is useful diagnostics, but failing to read it must not turn a
			// successfully created package into a reported cache failure.
				logrus.Warnf("offline package created but metadata inspection failed: %v", err)
		}
	}
	return nil
}

func writeCacheArchive(out io.Writer, absBase string, repos []string, totalFiles int) (err error) {
	gzipWriter, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}
	tarWriter := tar.NewWriter(gzipWriter)
	defer func() {
		if closeErr := tarWriter.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("failed to close tar archive: %w", closeErr)
		}
		if closeErr := gzipWriter.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("failed to close gzip archive: %w", closeErr)
		}
	}()

	walkCtx := &cacheWalkContext{absBase: absBase, tarWriter: tarWriter, total: totalFiles}
	for _, subDirName := range repos {
		subDirPath, pathErr := repoPath(absBase, subDirName)
		if pathErr != nil {
			return pathErr
		}
		if _, statErr := os.Stat(subDirPath); os.IsNotExist(statErr) {
			logrus.Warnf("subdirectory does not exist, skipping: %s", subDirPath)
			continue
		} else if statErr != nil {
			return fmt.Errorf("failed to access repository path %s: %w", subDirPath, statErr)
		}
		if walkErr := filepath.Walk(subDirPath, walkCtx.walk); walkErr != nil {
			return fmt.Errorf("failed to walk subdirectory %s: %w", subDirPath, walkErr)
		}
	}
	return nil
}

// printFinalPackageInfo prints the final package path, size in GiB, and its MD5 checksum.
func printFinalPackageInfo(pkgPath string) error {
	pkgPath, size, checksum, err := packageInfo(pkgPath)
	if err != nil {
		return err
	}

	logrus.Infof("offline package path : %s", pkgPath)
	logrus.Infof("offline package size : %.2f GiB", bytesToGiB(size))
	logrus.Infof("offline package md5  : %s", checksum)

	osarch := config.OSArch
	switch config.OSArch {
	case "amd64":
		osarch = "x86_64"
	case "arm64":
		osarch = "aarch64"
	}
	inferName := fmt.Sprintf("pigsty-pkg-v%s.%s.%s.tgz", config.PigstyVersion, config.OSCode, osarch)
	logrus.Infof("offline package arch : %s", osarch)
	logrus.Infof("recommended pkg name : %s", inferName)
	return nil
}

// countFiles returns the total number of regular files and the total size (bytes) in the specified subdirectories.
func countFiles(absBase string, repos []string) (int, int64, error) {
	var totalFiles int
	var totalSize int64

	for _, subDirName := range repos {
		subDirPath, err := repoPath(absBase, subDirName)
		if err != nil {
			return 0, 0, err
		}
		info, err := os.Stat(subDirPath)
		if os.IsNotExist(err) {
			logrus.Warnf("subdirectory does not exist for counting, skipping: %s", subDirPath)
			continue
		}
		if err != nil {
			return 0, 0, err
		}

		// If the path is actually a file, count it as well
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				totalFiles++
				totalSize += info.Size()
			}
			continue
		}

		// Otherwise, walk the directory
		err = filepath.Walk(subDirPath, func(filePath string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.Mode().IsRegular() {
				totalFiles++
				totalSize += info.Size()
			}
			return nil
		})
		if err != nil {
			return 0, 0, err
		}
	}
	return totalFiles, totalSize, nil
}

func repoPath(absBase, name string) (string, error) {
	path := filepath.Join(absBase, name)
	inside, err := utils.IsPathWithin(absBase, path)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("repository path escapes base directory: %s", name)
	}
	realBase, err := resolvePathWithMissingSuffix(absBase)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory %s: %w", absBase, err)
	}
	realPath, err := resolvePathWithMissingSuffix(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repository path %s: %w", path, err)
	}
	inside, err = utils.IsPathWithin(realBase, realPath)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("repository path resolves outside base directory: %s", name)
	}
	return path, nil
}

func ensurePackageOutsideRepos(absBase, pkgPath string, repos []string) error {
	absPkg, err := filepath.Abs(pkgPath)
	if err != nil {
		return fmt.Errorf("failed to resolve package path %s: %w", pkgPath, err)
	}
	realParent, err := resolvePathWithMissingSuffix(filepath.Dir(absPkg))
	if err != nil {
		return fmt.Errorf("failed to resolve package parent %s: %w", filepath.Dir(absPkg), err)
	}
	absPkg = filepath.Join(realParent, filepath.Base(absPkg))
	for _, name := range repos {
		inputPath, err := repoPath(absBase, name)
		if err != nil {
			return err
		}
		inputPath, err = resolvePathWithMissingSuffix(inputPath)
		if err != nil {
			return fmt.Errorf("failed to resolve repository path %s: %w", inputPath, err)
		}
		inside, err := utils.IsPathWithin(inputPath, absPkg)
		if err != nil {
			return err
		}
		if inside {
			return fmt.Errorf("package path must be outside cached repository %s: %s", name, absPkg)
		}
	}
	return nil
}

// resolvePathWithMissingSuffix resolves symlinks in the longest existing
// ancestor and then restores any missing path components. filepath.EvalSymlinks
// alone returns ENOENT for paths such as link/new, which would otherwise hide
// that link points inside a cached repository.
func resolvePathWithMissingSuffix(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		_, err = os.Lstat(current)
		if err == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", evalErr
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// printProgressAndFile prints a file-based progress indicator with the current file name.
// The terminal line is cleared on each update to avoid leftover characters.
// In structured output mode (YAML/JSON), progress is suppressed to avoid polluting output.
func printProgressAndFile(processed, total int, currentFile string) {
	if total == 0 {
		return
	}
	// Skip progress output in structured output mode to avoid polluting YAML/JSON
	if config.IsStructuredOutput() {
		return
	}
	percent := float64(processed) / float64(total) * 100
	// \r    : Move cursor to the beginning of the line
	// \033[K: Clear from cursor to the end of the line (ANSI escape sequence)
	fmt.Printf("\r\033[KProgress: %d/%d (%.1f%%)  Adding: %s", processed, total, percent, currentFile)
	_ = os.Stdout.Sync() // Force flush
}

// bytesToGiB converts bytes to GiB (1 GiB = 1024^3 bytes).
func bytesToGiB(b int64) float64 {
	return float64(b) / (1024.0 * 1024.0 * 1024.0)
}

// preparePackageOutput validates the package path, creates its parent, and
// returns the existing permission mode (or 0644 for a new package). It preserves
// a pre-existing package until the replacement is ready and refuses dangerous
// targets such as "/", directories, and symlinks.
func preparePackageOutput(pkgPath string) (os.FileMode, error) {
	if pkgPath == "" {
		return 0, fmt.Errorf("package path is empty")
	}

	absPath, err := filepath.Abs(filepath.Clean(pkgPath))
	if err != nil {
		return 0, fmt.Errorf("failed to resolve package path %s: %v", pkgPath, err)
	}
	if absPath == string(os.PathSeparator) {
		return 0, fmt.Errorf("invalid package path: %s", pkgPath)
	}

	mode := os.FileMode(0644)
	info, err := os.Lstat(absPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("package path points to a symbolic link: %s", absPath)
		}
		if info.IsDir() {
			return 0, fmt.Errorf("package path points to a directory: %s", absPath)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("failed to inspect package path %s: %v", absPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return 0, fmt.Errorf("failed to create package parent directory: %v", err)
	}
	return mode, nil
}

func packageInfo(pkgPath string) (string, int64, string, error) {
	absPath, err := filepath.Abs(pkgPath)
	if err != nil {
		return "", 0, "", err
	}
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return "", 0, "", err
	}
	f, err := os.Open(absPath)
	if err != nil {
		return "", 0, "", err
	}
	hash := md5.New()
	_, copyErr := io.Copy(hash, f)
	closeErr := f.Close()
	if copyErr != nil {
		return "", 0, "", copyErr
	}
	if closeErr != nil {
		return "", 0, "", closeErr
	}
	return absPath, fileInfo.Size(), fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// CacheWithResult creates an offline package from local repo and returns a structured Result
// This function is used for YAML/JSON output modes
func CacheWithResult(dirPath, pkgPath string, repos []string) *output.Result {
	startTime := time.Now()

	// Use defaults if not provided
	if dirPath == "" {
		dirPath = "/www"
	}
	if pkgPath == "" {
		pkgPath = "/tmp/pkg.tgz"
	}
	if len(repos) == 0 {
		repos = []string{"pigsty"}
	}

	// Build OS environment info
	osEnv := &OSEnvironment{
		Code:  config.OSCode,
		Arch:  config.OSArch,
		Type:  config.OSType,
		Major: config.OSMajor,
	}

	// Prepare data structure
	data := &RepoCacheData{
		OSEnv:         osEnv,
		SourceDir:     dirPath,
		TargetPkg:     pkgPath,
		IncludedRepos: repos,
	}

	// Get absolute base directory
	absBase, err := filepath.Abs(dirPath)
	if err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoCacheFailed,
			fmt.Sprintf("failed to determine abs path of dirPath %s: %v", dirPath, err)).WithData(data)
	}

	// Check if base dir exists
	baseInfo, err := os.Stat(absBase)
	if err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		if os.IsNotExist(err) {
			return output.Fail(output.CodeRepoDirNotFound,
				fmt.Sprintf("invalid base directory: %s", absBase)).WithData(data)
		}
		return output.Fail(output.CodeRepoCacheFailed,
			fmt.Sprintf("failed to access base directory %s: %v", absBase, err)).WithData(data)
	}
	if !baseInfo.IsDir() {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoDirNotFound,
			fmt.Sprintf("invalid base directory: %s", absBase)).WithData(data)
	}

	// Perform the cache operation (reuse existing Cache function logic, but capture result)
	err = Cache(dirPath, pkgPath, repos)
	data.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		return output.Fail(output.CodeRepoCacheFailed, err.Error()).WithData(data)
	}

	// Cache has already atomically committed the package. Resolve its target
	// path independently so it remains useful even when optional metadata
	// inspection fails (for example because the preserved mode is unreadable).
	if absPkg, absErr := filepath.Abs(pkgPath); absErr == nil {
		data.TargetPkg = absPkg
	}

	absPkg, size, checksum, inspectErr := packageInfo(pkgPath)
	if inspectErr == nil {
		data.TargetPkg = absPkg
		data.PackageSize = size
		data.PackageMD5 = checksum
	}
	return cacheResultAfterCommit(data, inspectErr)
}

// cacheResultAfterCommit applies the post-commit reporting policy. Once the
// package has been atomically replaced, metadata inspection is best-effort and
// cannot change the operation back to a failure.
func cacheResultAfterCommit(data *RepoCacheData, inspectErr error) *output.Result {
	message := fmt.Sprintf("Created offline package %s", data.TargetPkg)
	if inspectErr == nil {
		message += fmt.Sprintf(" (%.2f GiB)", bytesToGiB(data.PackageSize))
		return output.OK(message, data)
	}
	return output.OK(message, data).
		WithDetail(fmt.Sprintf("package created but metadata inspection failed: %v", inspectErr))
}
