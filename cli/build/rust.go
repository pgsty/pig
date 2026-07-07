package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

const (
	rustupDefaultScriptURL = "https://sh.rustup.rs"
	rustupMirrorScriptURL  = "https://rsproxy.cn/rustup-init.sh"
	rustupMirrorDistServer = "https://rsproxy.cn"
	rustupMirrorUpdateRoot = "https://rsproxy.cn/rustup"
)

type rustSetupDeps struct {
	homeDir     string
	fetchScript func(url string) (io.ReadCloser, error)
	runCommand  func(args []string, env []string) error
}

func SetupRust(force bool, mirror bool) error {
	return setupRust(force, mirror, rustSetupDeps{
		homeDir:     config.HomeDir,
		fetchScript: fetchRustupScript,
		runCommand:  utils.CommandWithEnv,
	})
}

func setupRust(force bool, mirror bool, deps rustSetupDeps) error {
	if deps.homeDir == "" {
		deps.homeDir = config.HomeDir
	}
	if deps.fetchScript == nil {
		deps.fetchScript = fetchRustupScript
	}
	if deps.runCommand == nil {
		deps.runCommand = utils.CommandWithEnv
	}

	cargoBin := filepath.Join(deps.homeDir, ".cargo", "bin", "cargo")

	if mirror {
		if err := writeCargoMirrorConfig(deps.homeDir); err != nil {
			return err
		}
	}

	// Check if rust is already installed
	if _, err := os.Stat(cargoBin); err == nil && !force {
		logrus.Info("Rust already installed, skipping installation")
		return nil
	}

	logrus.Info("Installing Rust...")

	scriptURLs := []string{rustupDefaultScriptURL}
	if mirror {
		scriptURLs = []string{rustupMirrorScriptURL, rustupDefaultScriptURL}
	}

	script, cleanup, err := downloadRustupScript(scriptURLs, deps.fetchScript)
	if err != nil {
		return err
	}
	defer cleanup()

	// Install rust with -y flag to avoid interactive prompts
	logrus.Info("Running rustup installation script...")
	env := []string(nil)
	if mirror {
		env = append(env,
			"RUSTUP_DIST_SERVER="+rustupMirrorDistServer,
			"RUSTUP_UPDATE_ROOT="+rustupMirrorUpdateRoot,
		)
	}
	if err := deps.runCommand([]string{script, "-y"}, env); err != nil {
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

func fetchRustupScript(url string) (io.ReadCloser, error) {
	resp, err := utils.DefaultClient().Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	return resp.Body, nil
}

func downloadRustupScript(urls []string, fetch func(url string) (io.ReadCloser, error)) (string, func(), error) {
	var lastErr error
	for _, url := range urls {
		body, err := fetch(url)
		if err != nil {
			lastErr = err
			logrus.Warnf("failed to download rustup script from %s: %v", url, err)
			continue
		}

		script, cleanup, err := writeRustupScript(body)
		if err != nil {
			body.Close()
			return "", nil, err
		}
		if err := body.Close(); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to close rustup script response: %v", err)
		}
		return script, cleanup, nil
	}
	if lastErr != nil {
		return "", nil, fmt.Errorf("failed to download rustup script: %v", lastErr)
	}
	return "", nil, fmt.Errorf("failed to download rustup script: no URLs configured")
}

func writeRustupScript(body io.Reader) (string, func(), error) {
	out, err := os.CreateTemp("", "pig-rustup-*.sh")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create script file: %v", err)
	}
	script := out.Name()
	cleanup := func() { _ = os.Remove(script) }

	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		cleanup()
		return "", nil, fmt.Errorf("failed to write rustup script: %v", err)
	}
	if err := out.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to close rustup script: %v", err)
	}
	if err := os.Chmod(script, 0755); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to make script executable: %v", err)
	}
	return script, cleanup, nil
}

func writeCargoMirrorConfig(homeDir string) error {
	cargoDir := filepath.Join(homeDir, ".cargo")
	if err := os.MkdirAll(cargoDir, 0755); err != nil {
		return fmt.Errorf("failed to create cargo config directory: %v", err)
	}

	content := []byte(`[source.crates-io]
replace-with = "rsproxy-sparse"

[source.rsproxy-sparse]
registry = "sparse+https://rsproxy.cn/index/"

[net]
git-fetch-with-cli = true
`)
	configPath := filepath.Join(cargoDir, "config.toml")
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write cargo mirror config: %v", err)
	}
	logrus.Infof("Cargo mirror config written to %s", configPath)
	return nil
}
