package build

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"syscall"

	"github.com/sirupsen/logrus"
)

var renamePath = os.Rename

// Spec configuration for build environments
type specConfig struct {
	Type      string   // "rpm" or "deb"
	Tarball   string   // tarball filename
	TargetDir string   // target directory name (e.g., "rpmbuild" or "debbuild")
	SubDirs   []string // subdirectories to create
	PkgDir    string   // package output directory (e.g., "RPMS" or "DEBS")
	SrcDir    string   // source directory (always "SOURCES")
}

// SpecDirSetup manages build spec fhs
func SpecDirSetup(force bool, mirror bool) error {
	var spec *specConfig
	switch config.OSType {
	case config.DistroEL:
		spec = &specConfig{
			Type:      "rpm",
			Tarball:   "rpmbuild.tar.gz",
			TargetDir: "rpmbuild",
			SubDirs:   []string{"SPECS", "RPMS", "SOURCES", "BUILD", "BUILDROOT", "SRPMS"},
			PkgDir:    "RPMS",
			SrcDir:    "SOURCES",
		}
	case config.DistroDEB:
		spec = &specConfig{
			Type:      "deb",
			Tarball:   "debbuild.tar.gz",
			TargetDir: "debbuild",
			SubDirs:   []string{"SPECS", "DEBS", "SOURCES", "BUILD"},
			PkgDir:    "DEBS",
			SrcDir:    "SOURCES",
		}
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
	return syncSpec(spec, force, mirror)
}

// setupBuildDirs creates complete directory structure and symlinks
// Real directories: ~/ext/{pkg,src,log,tmp}
// Symlinks in build dir point to ~/ext:
// - ~/rpmbuild/RPMS -> ~/ext/pkg (or ~/debbuild/DEBS -> ~/ext/pkg)
// - ~/rpmbuild/SOURCES -> ~/ext/src (or ~/debbuild/SOURCES -> ~/ext/src)
func setupBuildDirs(spec *specConfig, force bool) error {
	extDir := filepath.Join(config.HomeDir, "ext")
	buildDir := filepath.Join(config.HomeDir, spec.TargetDir)

	// 1. Create real directories under ~/ext
	extPkgDir := filepath.Join(extDir, "pkg")
	extSrcDir := filepath.Join(extDir, "src")
	extLogDir := filepath.Join(extDir, "log")
	extTmpDir := filepath.Join(extDir, "tmp")

	for _, dir := range []string{extPkgDir, extSrcDir, extLogDir, extTmpDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	// 2. Create build subdirectories (skip PkgDir and SrcDir which will be symlinks)
	for _, subDir := range spec.SubDirs {
		if subDir == spec.PkgDir || subDir == spec.SrcDir {
			continue // These will be created as symlinks below
		}
		dir := filepath.Join(buildDir, subDir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	// 3. Create symlinks in build dir pointing to ~/ext
	buildPkgLink := filepath.Join(buildDir, spec.PkgDir)
	buildSrcLink := filepath.Join(buildDir, spec.SrcDir)

	// Create: ~/rpmbuild/RPMS -> ~/ext/pkg (or ~/debbuild/DEBS -> ~/ext/pkg)
	if err := createSymlink(extPkgDir, buildPkgLink, force); err != nil {
		return fmt.Errorf("failed to create pkg symlink %s -> %s: %w", buildPkgLink, extPkgDir, err)
	}
	logrus.Debugf("Created symlink: %s -> %s", buildPkgLink, extPkgDir)

	// Create: ~/rpmbuild/SOURCES -> ~/ext/src (or ~/debbuild/SOURCES -> ~/ext/src)
	if err := createSymlink(extSrcDir, buildSrcLink, force); err != nil {
		return fmt.Errorf("failed to create src symlink %s -> %s: %w", buildSrcLink, extSrcDir, err)
	}
	logrus.Debugf("Created symlink: %s -> %s", buildSrcLink, extSrcDir)

	logrus.Infof("Build directory structure created at %s", buildDir)
	return nil
}

// createSymlink creates a symbolic link: linkPath -> target
// In force mode, aggressively removes any existing file/dir/symlink at linkPath.
// In non-force mode, existing directories are migrated into target first, then
// replaced by symlink to preserve data while keeping link semantics.
func createSymlink(target, linkPath string, force bool) error {
	if info, err := os.Lstat(linkPath); err == nil {
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			existingTarget, readErr := os.Readlink(linkPath)
			if readErr == nil && existingTarget == target {
				return nil
			}
			if err := os.Remove(linkPath); err != nil {
				return fmt.Errorf("failed to replace existing symlink: %w", err)
			}
		case info.IsDir():
			if !force {
				// Preserve existing content by moving it into target before relinking.
				if err := migrateDirIntoTarget(linkPath, target); err != nil {
					return err
				}
				if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove existing directory after migration: %w", err)
				}
				break
			}
			if err := os.RemoveAll(linkPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing directory: %w", err)
			}
		default:
			if !force {
				return fmt.Errorf("existing path is not a symlink: %s (use --force to replace)", linkPath)
			}
			if err := os.RemoveAll(linkPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing path: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect existing path: %w", err)
	}

	// Remove existing path in force mode when lstat says it does not exist now.
	if force {
		if err := os.RemoveAll(linkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clean link path: %w", err)
		}
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(linkPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	// Create symlink: linkPath -> target
	return os.Symlink(target, linkPath)
}

func migrateDirIntoTarget(srcDir, targetDir string) error {
	if srcDir == targetDir {
		return nil
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}
	backupDir := filepath.Join(targetDir, ".migrated_from_"+filepath.Base(srcDir))

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read existing directory %s: %w", srcDir, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(targetDir, entry.Name())

		if _, err := os.Lstat(dstPath); err == nil {
			// Keep target entry untouched; move source entry into backup to avoid data loss.
			if err := os.MkdirAll(backupDir, 0755); err != nil {
				return fmt.Errorf("failed to create migration backup directory %s: %w", backupDir, err)
			}
			backupPath, err := uniqueBackupPath(filepath.Join(backupDir, entry.Name()))
			if err != nil {
				return fmt.Errorf("failed to allocate migration backup path: %w", err)
			}
			if err := os.Rename(srcPath, backupPath); err != nil {
				return fmt.Errorf("failed to backup conflicting entry %s to %s: %w", srcPath, backupPath, err)
			}
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect migration target %s: %w", dstPath, err)
		}

		if err := movePath(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to migrate %s to %s: %w", srcPath, dstPath, err)
		}
	}
	return nil
}

func movePath(src, dst string) error {
	if err := renamePath(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := copyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	switch mode := info.Mode(); {
	case mode&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	case mode.IsRegular():
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Close()
	default:
		return fmt.Errorf("unsupported file mode for cross-device move: %s", info.Mode().String())
	}
}

func uniqueBackupPath(path string) (string, error) {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path, nil
	} else if err != nil {
		return "", err
	}

	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to allocate backup path for %s", path)
}

// syncSpec: Download tarball and perform incremental sync via rsync
func syncSpec(spec *specConfig, force bool, mirror bool) error {
	logrus.Info("sync extension specs")

	// Setup paths
	extDir := filepath.Join(config.HomeDir, "ext")
	tarballPath := filepath.Join(extDir, spec.Tarball)
	tempExtractDir := filepath.Join(extDir, spec.TargetDir)
	targetDir := filepath.Join(config.HomeDir, spec.TargetDir)

	// 1. Setup complete directory structure and symlinks first
	logrus.Infof("create spec dir at %s", targetDir)
	if err := setupBuildDirs(spec, force); err != nil {
		return fmt.Errorf("failed to setup spec dir: %w", err)
	}

	// 2. Download tarball (force re-download if requested)
	if err := downloadTarball(spec.Tarball, tarballPath, force, mirror); err != nil {
		return err
	}

	// 3. Extract to temp directory (clean first in force mode)
	if force {
		_ = os.RemoveAll(tempExtractDir) // Ignore errors in force mode
	}
	if err := extractToDir(tarballPath, tempExtractDir); err != nil {
		return err
	}

	// 4. Incremental sync via rsync
	logrus.Debugf("sync changes to %s", targetDir)
	rsyncCmd := []string{"rsync", "-az", tempExtractDir + "/", targetDir + "/"}
	if err := utils.Command(rsyncCmd); err != nil {
		return fmt.Errorf("failed to rsync: %w", err)
	}

	// 5. Post-setup (OS-specific tasks)
	if err := postSetup(spec); err != nil {
		return err
	}

	logrus.Infof("%s spec ready at %s", spec.Type, targetDir)
	return nil
}

// downloadTarball downloads spec tarball if not already present
func downloadTarball(filename, localPath string, force bool, mirror bool) error {
	// If force is true, remove existing file
	if force {
		if err := os.RemoveAll(localPath); err != nil && !os.IsNotExist(err) {
			logrus.Warnf("fail to remove existing tarball: %v", err)
		} else if err == nil {
			logrus.Infof("remove existing spec tarball and re-download: %s", localPath)
		}
	}

	// Check if already exists (and not forcing)
	if !force {
		if info, err := os.Stat(localPath); err == nil {
			logrus.Infof("found existing tarball: %s (%.2f MB)",
				localPath, float64(info.Size())/(1024*1024))
			return nil
		}
	}

	// Construct download URL (default: pigsty.io, mirror: pigsty.cc)
	baseURL := config.RepoPigstyIO
	if mirror {
		baseURL = config.RepoPigstyCC
	}
	url := fmt.Sprintf("%s/ext/spec/%s", baseURL, filename)

	logrus.Debugf("download %s from %s", filename, url)
	return utils.DownloadFile(url, localPath)
}

// extractToDir extracts tarball to specified directory
func extractToDir(tarballPath, targetDir string) error {
	// Clean and recreate target
	if err := os.RemoveAll(targetDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clean dir: %w", err)
	}

	// Extract (tar will create the directory)
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	logrus.Debugf("Extracting %s to %s", tarballPath, parentDir)
	cmd := []string{"tar", "-xzf", tarballPath, "-C", parentDir}
	return utils.Command(cmd)
}

// postSetup performs OS-specific post-installation setup
func postSetup(spec *specConfig) error {
	targetDir := filepath.Join(config.HomeDir, spec.TargetDir)

	switch spec.Type {
	case "rpm":
		// Setup RPM build tree structure (may create additional macros/config)
		if err := utils.Command([]string{"rpmdev-setuptree"}); err != nil {
			logrus.Debugf("rpmdev-setuptree failed (non-critical): %v", err)
		}

		// Fix ownership to current user
		if config.CurrentUser != "" && config.CurrentUser != "root" {
			logrus.Debugf("fixing ownership of %s to %s", targetDir, config.CurrentUser)
			chownCmd := []string{"chown", "-R",
				fmt.Sprintf("%s:%s", config.CurrentUser, config.CurrentUser),
				targetDir}
			if err := utils.SudoCommand(chownCmd); err != nil {
				logrus.Warnf("Failed to fix ownership: %v", err)
			}
		}

	case "deb":
		// Create /tmp/deb working directory for Debian builds
		tmpDir := "/tmp/deb"
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", tmpDir, err)
		}
		logrus.Debugf("create temporary build directory: %s", tmpDir)
	}
	return nil
}
