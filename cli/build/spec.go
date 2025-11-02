package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

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
func SpecDirSetup(force bool) error {
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
	return syncSpec(spec, force)
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
// In force mode, aggressively removes any existing file/dir/symlink at linkPath
func createSymlink(target, linkPath string, force bool) error {
	// Remove existing file/dir/symlink at linkPath
	if force {
		// Force mode: ignore removal errors
		_ = os.RemoveAll(linkPath)
	} else {
		// Normal mode: return error if removal fails
		if err := os.RemoveAll(linkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing path: %w", err)
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

// syncSpec: Download tarball and perform incremental sync via rsync
func syncSpec(spec *specConfig, force bool) error {
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
	if err := downloadTarball(spec.Tarball, tarballPath, force); err != nil {
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
func downloadTarball(filename, localPath string, force bool) error {
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

	// Construct download URL
	baseURL := config.RepoPigstyCC
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
