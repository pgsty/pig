package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// InitEnvironment initializes complete build environment
// Performs: spec + repo + tool + rust + pgrx
func InitEnvironment() error {
	logrus.Info(utils.PadHeader("Initializing Build Environment", 80))

	// Step 1: Setup spec and directories
	logrus.Info("[1/5] Setting up build spec and directories")
	if err := GetSpecRepo(); err != nil {
		return fmt.Errorf("failed to setup spec: %w", err)
	}

	// Step 2: Initialize repositories
	logrus.Info("[2/5] Initializing package repositories")
	if err := initRepo(); err != nil {
		return fmt.Errorf("failed to setup repositories: %w", err)
	}

	// Step 3: Install build tools
	logrus.Info("[3/5] Installing build tools")
	if err := InstallBuildTools(""); err != nil {
		return fmt.Errorf("failed to install build tools: %w", err)
	}

	// Step 4: Setup Rust (optional but recommended)
	logrus.Info("[4/5] Setting up Rust toolchain")
	if err := SetupRust(false); err != nil {
		logrus.Warnf("Rust setup failed (optional): %v", err)
	}

	// Step 5: Setup pgrx (optional, requires Rust)
	logrus.Info("[5/5] Setting up pgrx")
	if err := SetupPgrx("0.16.1", ""); err != nil {
		logrus.Warnf("pgrx setup failed (optional): %v", err)
	}

	// Create build directories and symlinks
	if err := createBuildDirectories(); err != nil {
		return fmt.Errorf("failed to create build directories: %w", err)
	}

	// Setup source directory symlinks
	if err := EnsureSourceDirectory(); err != nil {
		logrus.Warnf("Failed to setup source directory symlinks: %v", err)
	}

	logrus.Info(utils.PadHeader("Build Environment Ready", 80))
	logrus.Info("✓ Build spec configured")
	logrus.Info("✓ Repositories initialized")
	logrus.Info("✓ Build tools installed")
	logrus.Info("✓ Build directories created at ~/ext")
	logrus.Info("")
	logrus.Info("You can now build extensions with: pig build pkg <extension>")

	return nil
}

// initRepo initializes required repositories
func initRepo() error {
	// Remove existing repos
	if err := utils.SudoCommand([]string{"rm", "-rf", "/etc/yum.repos.d/*.repo"}); err != nil {
		logrus.Debugf("Failed to remove existing repos: %v", err)
	}

	// Add required repos based on OS
	switch config.OSType {
	case config.DistroEL:
		// Add PGDG and Pigsty repos
		repos := []string{"pgdg", "pigsty"}
		for _, repo := range repos {
			if err := utils.SudoCommand([]string{"yum", "install", "-y",
				fmt.Sprintf("https://repo.pigsty.io/yum/repo/%s-repo.rpm", repo)}); err != nil {
				return fmt.Errorf("failed to add %s repo: %w", repo, err)
			}
		}
		// Update repo cache
		if err := utils.SudoCommand([]string{"yum", "makecache"}); err != nil {
			logrus.Warnf("Failed to update yum cache: %v", err)
		}

	case config.DistroDEB:
		// Add PGDG and Pigsty repos for Debian/Ubuntu
		// TODO: Implement Debian/Ubuntu repo setup
		logrus.Info("Debian/Ubuntu repository setup")
	}

	return nil
}

// createBuildDirectories creates necessary directories for building
func createBuildDirectories() error {
	// Create ~/ext directory structure
	extDir := filepath.Join(config.HomeDir, "ext")
	logDir := filepath.Join(extDir, "log")
	srcDir := filepath.Join(extDir, "src")

	dirs := []string{extDir, logDir, srcDir}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		logrus.Debugf("Created directory: %s", dir)
	}

	return nil
}