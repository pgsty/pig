package sty

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"pig/cli/license"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

//go:embed assets/EULA.md
var embeddedEULA []byte

const DefaultDir = "~/pigsty"

type installOptions struct {
	Overwrite  bool
	AcceptEULA bool
}

// InstallPigsty installs pigsty to the specified directory.
// If targetDir is empty, it defaults to ~/pigsty.
// If overwrite is true, it will overwrite existing files.
// Returns error if installation fails.
func InstallPigsty(srcTarball []byte, targetDir string, overwrite bool) error {
	return installPigsty(srcTarball, targetDir, installOptions{Overwrite: overwrite})
}

// installPigsty stages a complete final tree beside the target and
// commits it only after extraction succeeds. Existing --force installations
// are cloned into staging first so protected and user-created files retain the
// historical overlay semantics.
func installPigsty(srcTarball []byte, targetDir string, options installOptions) error {
	if srcTarball == nil {
		return commandFailedf(output.CodeStyInitInvalidArgs, "source tarball not provided")
	}
	resolvedTarget, existing, targetExists, err := inspectInstallTarget(targetDir, options.Overwrite)
	if err != nil {
		return err
	}
	if targetExists {
		logrus.Warnf("overwriting existing directory transactionally: %s", resolvedTarget)
	}

	// Consent is resolved before creating a target, staging directory, or any
	// parent directory. --accept-eula is the only non-interactive acceptance.
	if license.Manager.Valid {
		if err := acceptProEULA(options.AcceptEULA); err != nil {
			return err
		}
	}

	parent := filepath.Dir(resolvedTarget)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to create install parent %s: %w", parent, err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(resolvedTarget)+".install-*")
	if err != nil {
		return fmt.Errorf("failed to create install staging directory: %w", err)
	}
	stageOwned := true
	defer func() {
		if stageOwned {
			_ = os.RemoveAll(stage)
		}
	}()

	rootMode := os.FileMode(0755)
	var preservedDirectoryModes []directoryMode
	if targetExists {
		rootMode = existing.Mode().Perm()
		preservedDirectoryModes, err = copyDirectoryContents(resolvedTarget, stage)
		if err != nil {
			return fmt.Errorf("failed to stage existing installation: %w", err)
		}
	}
	if err := extractPigsty(srcTarball, stage); err != nil {
		return fmt.Errorf("failed to stage pigsty archive: %w", err)
	}
	for i := len(preservedDirectoryModes) - 1; i >= 0; i-- {
		item := preservedDirectoryModes[i]
		if err := os.Chmod(item.path, item.mode); err != nil {
			return fmt.Errorf("failed to restore staged directory mode: %w", err)
		}
	}
	if license.Manager.Valid {
		licensePath := filepath.Join(stage, "EULA.md")
		if err := os.WriteFile(licensePath, embeddedEULA, 0644); err != nil {
			return fmt.Errorf("failed to stage EULA: %w", err)
		}
	}
	if err := os.Chmod(stage, rootMode); err != nil {
		return fmt.Errorf("failed to preserve install directory mode: %w", err)
	}
	if err := utils.SyncDirectory(stage); err != nil {
		return fmt.Errorf("failed to sync staged installation: %w", err)
	}
	if err := commitStagedInstall(stage, resolvedTarget, targetExists); err != nil {
		return err
	}
	stageOwned = false

	logrus.Infof("pigsty installed: %s", resolvedTarget)
	logrus.Infof("next steps:")
	logrus.Infof("  pig sty boot    # install ansible and prepare offline pkg")
	logrus.Infof("  pig sty conf -g # configure pigsty and generate config")
	logrus.Infof("  pig sty deploy  # deploy and provision env (DANGEROUS!)")

	return nil
}

func resolveInstallTarget(targetDir string) (string, error) {
	if targetDir == "" {
		targetDir = DefaultDir
	}
	if strings.HasPrefix(targetDir, "~/") {
		homeDir := config.HomeDir
		if homeDir == "" {
			var err error
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get user home directory: %w", err)
			}
		}
		targetDir = filepath.Join(homeDir, targetDir[2:])
	}
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve install target: %w", err)
	}
	return abs, nil
}

func inspectInstallTarget(targetDir string, overwrite bool) (string, os.FileInfo, bool, error) {
	resolved, err := resolveInstallTarget(targetDir)
	if err != nil {
		return "", nil, false, err
	}
	info, err := os.Lstat(resolved)
	if os.IsNotExist(err) {
		return resolved, nil, false, nil
	}
	if err != nil {
		return resolved, nil, false, fmt.Errorf("failed to inspect target %s: %w", resolved, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return resolved, info, true, commandFailedf(output.CodeStyInitTargetExists, "install target is not a directory: %s", resolved)
	}
	if !overwrite {
		return resolved, info, true, commandFailedf(output.CodeStyInitTargetExists, "directory already exists: %s (use -f to overwrite)", resolved)
	}
	return resolved, info, true, nil
}

func acceptProEULA(accepted bool) error {
	if accepted {
		return nil
	}
	if os.Getenv("PIG_NON_INTERACTIVE") != "" || !utils.StdinIsTerminal() {
		return commandFailedf(output.CodeStyInitEULARequired, "Pigsty Pro EULA acceptance required; review the EULA and rerun with --accept-eula")
	}

	fmt.Println("##############################################################################")
	fmt.Println(string(embeddedEULA))
	fmt.Println("##############################################################################")
	logrus.Warnf("to proceed with Pigsty Pro installation, you must accept the EULA")
	logrus.Infof("do you accept the terms of the EULA? (yes/no)")
	fmt.Printf("> ")
	response, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read EULA response: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(response)) {
	case "yes", "y", "ok", "true":
		logrus.Infof("EULA accepted, proceeding with installation")
		return nil
	case "no", "n", "nay", "off", "false":
		return commandFailedf(output.CodeStyInitEULARequired, "EULA not accepted")
	default:
		return commandFailedf(output.CodeStyInitEULARequired, "invalid EULA response")
	}
}

type directoryMode struct {
	path string
	mode os.FileMode
}

func copyDirectoryContents(src, dst string) ([]directoryMode, error) {
	var directories []directoryMode
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			directories = append(directories, directoryMode{path: target, mode: info.Mode().Perm()})
			return nil
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyRegularFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported existing file type: %s", path)
		}
	})
	if err != nil {
		return nil, err
	}
	return directories, nil
}

func copyRegularFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func commitStagedInstall(stage, target string, replacing bool) error {
	parent := filepath.Dir(target)
	if !replacing {
		if err := os.Rename(stage, target); err != nil {
			return fmt.Errorf("failed to commit installation: %w", err)
		}
		return utils.SyncDirectory(parent)
	}

	backup, err := unusedSiblingPath(parent, "."+filepath.Base(target)+".backup-*")
	if err != nil {
		return fmt.Errorf("failed to reserve install rollback path: %w", err)
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("failed to stage current installation for rollback: %w", err)
	}
	if err := os.Rename(stage, target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			return fmt.Errorf("failed to commit installation: %v (rollback also failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("failed to commit installation: %w", err)
	}
	if err := utils.SyncDirectory(parent); err != nil {
		return fmt.Errorf("installation committed but parent sync failed: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		logrus.Warnf("installation committed; failed to remove rollback directory %s: %v", backup, err)
	}
	return nil
}

func unusedSiblingPath(parent, pattern string) (string, error) {
	path, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

// extractPigsty extracts pigsty source from embedded tarball to destination directory.
// It handles files, directories and symlinks while preserving file modes.
// Protected files like pigsty.yml and files/pki/* are not overwritten if they exist.
func extractPigsty(data []byte, dst string) error {
	buf := bytes.NewReader(data)
	gzr, err := gzip.NewReader(buf)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("failed to resolve destination path: %w", err)
	}

	tarReader := tar.NewReader(gzr)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}
		if header == nil {
			continue
		}

		// Resolve and validate archive path before extraction.
		relPath, target, err := resolveArchiveTargetPath(dst, header.Name)
		if err != nil {
			return err
		}
		if relPath == "" {
			continue
		}

		// Skip protected files and directories
		if isProtectedFile(relPath, dst) {
			logrus.Debugf("skipping protected file: %s", relPath)
			continue
		}

		if err := ensureNoSymlinkInPath(dstAbs, target); err != nil {
			return err
		}

		if err := extractTarEntry(header, target, dstAbs, tarReader); err != nil {
			return fmt.Errorf("failed to extract %s: %w", target, err)
		}
	}
	if _, err := io.Copy(io.Discard, gzr); err != nil {
		return fmt.Errorf("failed to validate gzip trailer: %w", err)
	}
	return nil
}

func resolveArchiveTargetPath(dst, archivePath string) (string, string, error) {
	normalized := strings.TrimPrefix(strings.ReplaceAll(archivePath, "\\", "/"), "/")
	parts := strings.SplitN(normalized, "/", 2)
	if len(parts) <= 1 {
		return "", "", nil
	}

	relPath := filepath.Clean(strings.TrimLeft(parts[1], "/"))
	if relPath == "." || relPath == "" {
		return "", "", nil
	}
	if filepath.IsAbs(relPath) || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("invalid archive entry path: %s", archivePath)
	}

	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve destination path: %w", err)
	}
	target := filepath.Join(dstAbs, relPath)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve target path: %w", err)
	}
	relToBase, err := filepath.Rel(dstAbs, targetAbs)
	if err != nil {
		return "", "", fmt.Errorf("failed to validate target path: %w", err)
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("archive entry escapes destination: %s", archivePath)
	}
	return relPath, targetAbs, nil
}

// isProtectedFile checks if a file should be protected from overwriting
func isProtectedFile(relPath string, dst string) bool {
	// fmt.Println(relPath)
	switch {
	case filepath.Base(relPath) == "pigsty.yml" && regularFileExists(filepath.Join(dst, "pigsty.yml")):
		return true
	case strings.HasPrefix(relPath, "files/pki") && !strings.HasSuffix(relPath, "/"):
		return true
	default:
		return false
	}
}

// extractTarEntry handles extraction of a single tar entry
func extractTarEntry(header *tar.Header, target, dstAbs string, reader *tar.Reader) error {
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(header.Mode))

	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		if err := ensureRegularFileTarget(target); err != nil {
			return err
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		if _, err := io.Copy(f, reader); err != nil {
			_ = f.Close()
			return fmt.Errorf("failed to write file: %w", err)
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return fmt.Errorf("failed to sync file: %w", err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("failed to close file: %w", err)
		}
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for symlink: %w", err)
		}
		if err := validateSymlinkLinkname(dstAbs, target, header.Linkname); err != nil {
			return err
		}
		if err := ensureSymlinkTargetPath(target); err != nil {
			return err
		}
		os.Remove(target) // Remove existing symlink if any
		if err := os.Symlink(header.Linkname, target); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}

	default:
		logrus.Debugf("skipping unsupported file type %d: %s", header.Typeflag, target)
	}

	return nil
}

func validateSymlinkLinkname(base, linkPath, linkname string) error {
	if linkname == "" || filepath.IsAbs(linkname) {
		return fmt.Errorf("invalid symlink target: %s", linkname)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), linkname))
	ok, err := utils.IsPathWithin(base, resolved)
	if err != nil {
		return fmt.Errorf("failed to validate symlink target: %w", err)
	}
	if !ok {
		return fmt.Errorf("symlink target escapes destination: %s -> %s", linkPath, linkname)
	}
	return nil
}

func ensureNoSymlinkInPath(base, target string) error {
	parent := filepath.Dir(target)
	rel, err := filepath.Rel(base, parent)
	if err != nil {
		return fmt.Errorf("failed to validate extraction path: %w", err)
	}
	if rel == "." {
		return nil
	}

	cur := base
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("failed to inspect extraction path %s: %w", cur, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to extract through symlink path component: %s", cur)
		}
	}
	return nil
}

func ensureRegularFileTarget(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect target file %s: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symlink target: %s", target)
	}
	if info.IsDir() {
		return fmt.Errorf("refusing to overwrite directory with file: %s", target)
	}
	return nil
}

func ensureSymlinkTargetPath(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect symlink target path %s: %w", target, err)
	}
	if info.IsDir() {
		return fmt.Errorf("refusing to replace directory with symlink: %s", target)
	}
	return nil
}

// regularFileExists checks if a path exists and is a regular file.
func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
