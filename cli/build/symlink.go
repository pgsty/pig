package build

import (
	"os"
	"path/filepath"
	"pig/internal/config"

	"github.com/sirupsen/logrus"
)

// CreateSourceSymlinks creates symlinks from ~/ext/src to traditional build locations
func CreateSourceSymlinks() error {
	// Source directory (~/ext/src)
	srcDir := filepath.Join(config.HomeDir, "ext", "src")

	// Target directories based on OS type
	var targetDirs []string
	switch config.OSType {
	case config.DistroEL:
		targetDirs = append(targetDirs, filepath.Join(config.HomeDir, "rpmbuild", "SOURCES"))
	case config.DistroDEB:
		targetDirs = append(targetDirs, filepath.Join(config.HomeDir, "deb", "tarball"))
	}

	// Create symlinks for each target directory
	for _, targetDir := range targetDirs {
		if err := createDirSymlink(srcDir, targetDir); err != nil {
			logrus.Warnf("Failed to create symlink %s -> %s: %v", targetDir, srcDir, err)
		} else {
			logrus.Debugf("Created symlink: %s -> %s", targetDir, srcDir)
		}
	}

	return nil
}

// createDirSymlink creates a symbolic link from target to source
func createDirSymlink(srcDir, targetDir string) error {
	// Remove target if it exists (could be file, dir, or broken symlink)
	if err := os.RemoveAll(targetDir); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return err
	}

	// Create symlink
	return os.Symlink(srcDir, targetDir)
}

// EnsureSourceDirectory ensures ~/ext/src directory exists and sets up symlinks
func EnsureSourceDirectory() error {
	srcDir := filepath.Join(config.HomeDir, "ext", "src")

	// Create directory if not exists
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}

	// Create symlinks to traditional locations
	return CreateSourceSymlinks()
}