package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

const (
	RPMGitRepo = "https://github.com/pgsty/rpm.git"
	DEBGitRepo = "https://github.com/pgsty/deb.git"
)

// Spec configuration for build environments
type specConfig struct {
	Type      string // "rpm" or "deb"
	Tarball   string // tarball filename
	TargetDir string // target directory name
}

// GetSpecRepo manages build spec repository
// Modes: sync (default), new (overwrite), git (legacy)
func GetSpecRepo(args ...string) error {
	mode := "sync"
	if len(args) > 0 {
		mode = args[0]
	}

	// Get configuration based on OS
	spec := getSpecConfig()
	if spec == nil {
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	switch mode {
	case "git":
		return specGitMode(spec)
	case "new":
		return specNewMode(spec)
	default:
		return specSyncMode(spec)
	}
}

// getSpecConfig returns spec configuration based on OS type
func getSpecConfig() *specConfig {
	switch config.OSType {
	case config.DistroEL:
		return &specConfig{"rpm", "rpm.tgz", "rpmbuild"}
	case config.DistroDEB:
		return &specConfig{"deb", "deb.tgz", "deb"}
	default:
		return nil
	}
}

// specSyncMode: Download and incremental sync via rsync
func specSyncMode(spec *specConfig) error {
	logrus.Info("Syncing build spec repository (incremental mode)")
	return syncSpec(spec, false)
}

// specNewMode: Download and reset to default state via rsync --delete
func specNewMode(spec *specConfig) error {
	logrus.Info("Resetting build spec repository to default state")
	return syncSpec(spec, true)
}

// syncSpec: Common sync implementation with optional --delete flag
func syncSpec(spec *specConfig, reset bool) error {
	// Setup paths - use ~/ext as base directory
	extDir := filepath.Join(config.HomeDir, "ext")
	tarballPath := filepath.Join(extDir, spec.Tarball)
	tempExtractDir := filepath.Join(extDir, spec.TargetDir)
	targetDir := filepath.Join(config.HomeDir, spec.TargetDir)

	// Ensure ext directory exists (including log subdirectory)
	if err := os.MkdirAll(extDir, 0755); err != nil {
		return fmt.Errorf("failed to create ext dir: %w", err)
	}
	logDir := filepath.Join(extDir, "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	// Download tarball (skip if exists)
	if err := downloadTarball(spec.Tarball, tarballPath); err != nil {
		return err
	}

	// Extract to temp directory
	if err := extractToDir(tarballPath, tempExtractDir); err != nil {
		return err
	}

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target dir: %w", err)
	}

	// Build rsync command
	rsyncCmd := []string{"rsync", "-az"}
	if reset {
		// Add --delete to reset directory to default state
		rsyncCmd = append(rsyncCmd, "--delete")
		logrus.Infof("Resetting %s to default state", targetDir)
	} else {
		logrus.Infof("Syncing changes to %s", targetDir)
	}
	rsyncCmd = append(rsyncCmd, tempExtractDir+"/", targetDir+"/")

	// Execute rsync
	if err := utils.Command(rsyncCmd); err != nil {
		return fmt.Errorf("failed to rsync: %w", err)
	}

	// Post-setup
	if err := postSetup(spec); err != nil {
		return err
	}

	// Create source symlinks for compatibility
	if err := EnsureSourceDirectory(); err != nil {
		logrus.Warnf("Failed to setup source directory symlinks: %v", err)
	}

	action := "synced"
	if reset {
		action = "reset"
	}
	logrus.Infof("Successfully %s %s spec to %s", action, spec.Type, targetDir)
	return nil
}

// specGitMode: Legacy git clone method
func specGitMode(spec *specConfig) error {
	logrus.Info("Using git clone method for build spec")

	switch spec.Type {
	case "rpm":
		return gitCloneRPM()
	case "deb":
		return gitCloneDEB()
	default:
		return fmt.Errorf("unknown spec type: %s", spec.Type)
	}
}

// downloadTarball downloads spec tarball if not already present
func downloadTarball(filename, localPath string) error {
	// Check if already exists
	if info, err := os.Stat(localPath); err == nil {
		logrus.Infof("Using existing tarball: %s (%.2f MB)",
			localPath, float64(info.Size())/(1024*1024))
		return nil
	}

	// Construct download URL
	baseURL := config.RepoPigstyCC
	url := fmt.Sprintf("%s/ext/spec/%s", baseURL, filename)

	logrus.Infof("Downloading %s from %s", filename, url)
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
	switch spec.Type {
	case "rpm":
		targetDir := filepath.Join(config.HomeDir, spec.TargetDir)

		// Setup RPM build tree structure
		if err := utils.Command([]string{"rpmdev-setuptree"}); err != nil {
			logrus.Debugf("rpmdev-setuptree failed: %v", err)
		}

		// Fix ownership to current user
		if config.CurrentUser != "" && config.CurrentUser != "root" {
			logrus.Debugf("Fixing ownership of %s to %s", targetDir, config.CurrentUser)
			chownCmd := []string{"chown", "-R",
				fmt.Sprintf("%s:%s", config.CurrentUser, config.CurrentUser),
				targetDir}
			if err := utils.SudoCommand(chownCmd); err != nil {
				logrus.Warnf("Failed to fix ownership: %v", err)
			}
		}

	case "deb":
		// Create additional directories for DEB
		dirs := []string{
			filepath.Join(config.HomeDir, spec.TargetDir, "tarball"),
			"/tmp/deb",
		}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create %s: %w", dir, err)
			}
		}
	}
	return nil
}

// Legacy git clone implementations

func gitCloneRPM() error {
	tempDir := "/tmp/rpm"
	targetDir := filepath.Join(config.HomeDir, "rpmbuild")

	// Clean temp directory
	os.RemoveAll(tempDir)

	// Clone repository
	logrus.Infof("Cloning RPM repository from %s", RPMGitRepo)
	if err := utils.Command([]string{"git", "clone", RPMGitRepo, tempDir}); err != nil {
		return fmt.Errorf("failed to clone: %w", err)
	}

	// Setup RPM build tree
	utils.Command([]string{"rpmdev-setuptree"})
	rsyncCmd := []string{"rsync", "-a", filepath.Join(tempDir, "rpmbuild") + "/", targetDir + "/"}
	if err := utils.Command(rsyncCmd); err != nil {
		return fmt.Errorf("failed to rsync: %w", err)
	}

	// Fix ownership to current user (chown -R)
	if config.CurrentUser != "" && config.CurrentUser != "root" {
		logrus.Debugf("Fixing ownership of %s to %s", targetDir, config.CurrentUser)
		chownCmd := []string{"chown", "-R", fmt.Sprintf("%s:%s", config.CurrentUser, config.CurrentUser), targetDir}
		if err := utils.SudoCommand(chownCmd); err != nil {
			logrus.Warnf("Failed to fix ownership: %v", err)
		}
	}
	logrus.Infof("RPM build environment ready at %s", targetDir)
	return nil
}

func gitCloneDEB() error {
	targetDir := filepath.Join(config.HomeDir, "deb")

	// Clean and clone
	os.RemoveAll(targetDir)

	logrus.Infof("Cloning DEB repository from %s", DEBGitRepo)
	if err := utils.Command([]string{"git", "clone", DEBGitRepo, targetDir}); err != nil {
		return fmt.Errorf("failed to clone: %w", err)
	}

	// Create additional directories
	dirs := []string{
		filepath.Join(targetDir, "tarball"),
		"/tmp/deb",
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
	}

	logrus.Infof("DEB build environment ready at %s", targetDir)
	return nil
}
