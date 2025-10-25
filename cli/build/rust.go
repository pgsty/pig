package build

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

func SetupRust(force bool) error {
	cargoBin := config.HomeDir + "/.cargo/bin/cargo"

	// Check if rust is already installed
	if _, err := os.Stat(cargoBin); err == nil && !force {
		logrus.Info("Rust already installed, skipping installation")
		return nil
	}

	logrus.Info("Installing Rust...")

	// Download rustup script
	resp, err := http.Get("https://sh.rustup.rs")
	if err != nil {
		return fmt.Errorf("failed to download rustup script: %v", err)
	}
	defer resp.Body.Close()

	// Determine script path based on OS
	var script string
	if config.OSType == config.DistroMAC {
		// macOS uses /var/folders for temporary files
		script = "/tmp/rustup.sh"
	} else {
		script = "/tmp/rust.sh"
	}

	// Write script to file
	out, err := os.Create(script)
	if err != nil {
		return fmt.Errorf("failed to create script file: %v", err)
	}
	defer os.Remove(script) // Clean up after installation

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return fmt.Errorf("failed to write rustup script: %v", err)
	}
	out.Close()

	// Make script executable
	if err := os.Chmod(script, 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %v", err)
	}

	// Install rust with -y flag to avoid interactive prompts
	logrus.Info("Running rustup installation script...")
	if err := utils.Command([]string{script, "-y"}); err != nil {
		return fmt.Errorf("failed to run rustup script: %v", err)
	}

	// Verify installation
	if _, err := os.Stat(cargoBin); err != nil {
		return fmt.Errorf("rust installation verification failed, cargo not found at %s", cargoBin)
	}

	logrus.Info("Rust installed successfully")
	logrus.Info("Run 'pig build pgrx' to set up pgrx for building PostgreSQL extensions")
	return nil
}
