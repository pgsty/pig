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

func SetupRust(pgrxVersion string, force bool) error {
	if pgrxVersion == "" {
		pgrxVersion = "0.16.1"
	}
	cargoBin := config.HomeDir + "/.cargo/bin/cargo"

	// install rustc if not installed
	logrus.Infof("Setting up rust and pgrx %s", pgrxVersion)
	if _, err := os.Stat(cargoBin); err == nil && !force {
		logrus.Info("Rust already installed, skip")
	} else {
		logrus.Info("Rust not found, installing Rust...")

		// download rustup script
		resp, err := http.Get("https://sh.rustup.rs")
		if err != nil {
			return fmt.Errorf("failed to download rustup script: %v", err)
		}
		defer resp.Body.Close()

		// write directly to /tmp/rust.sh
		script := "/tmp/rust.sh"
		out, err := os.Create(script)
		if err != nil {
			return fmt.Errorf("failed to create script file: %v", err)
		}
		// defer os.Remove(script)
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("failed to write rustup script: %v", err)
		}
		out.Close()
		// make it executable
		if err := os.Chmod(script, 0755); err != nil {
			return fmt.Errorf("failed to make script executable: %v", err)
		}
		// install rust
		if err := utils.Command([]string{script, "-y"}); err != nil {
			return fmt.Errorf("failed to run rustup script: %v", err)
		}
	}

	// install cargo-pgrx
	if err := utils.Command([]string{cargoBin, "install", "--locked", fmt.Sprintf("cargo-pgrx@%s", pgrxVersion)}); err != nil {
		return fmt.Errorf("failed to install cargo-pgrx %s: %v", pgrxVersion, err)
	} else {
		logrus.Infof("install pgrx %s with cargo", pgrxVersion)
	}

	switch config.OSType {
	case config.DistroEL:
		logrus.Infof("init pgrx %s for EL-based system", pgrxVersion)
		if err := utils.Command([]string{cargoBin, "pgrx", "init", "--pg13=/usr/pgsql-13/bin/pg_config", "--pg14=/usr/pgsql-14/bin/pg_config", "--pg15=/usr/pgsql-15/bin/pg_config", "--pg16=/usr/pgsql-16/bin/pg_config", "--pg17=/usr/pgsql-17/bin/pg_config", "--pg18=/usr/pgsql-18/bin/pg_config"}); err != nil {
			return fmt.Errorf("failed to initialize pgrx for EL distro: %v", err)
		}
	case config.DistroDEB:
		logrus.Infof("init pgrx %s for DEB-based system", pgrxVersion)
		if err := utils.Command([]string{cargoBin, "pgrx", "init", "--pg13=/usr/lib/postgresql/13/bin/pg_config", "--pg14=/usr/lib/postgresql/14/bin/pg_config", "--pg15=/usr/lib/postgresql/15/bin/pg_config", "--pg16=/usr/lib/postgresql/16/bin/pg_config", "--pg17=/usr/lib/postgresql/17/bin/pg_config", "--pg18=/usr/lib/postgresql/18/bin/pg_config"}); err != nil {
			return fmt.Errorf("failed to initialize pgrx for DEB distro: %v", err)
		}
	case config.DistroMAC:
		return fmt.Errorf("MacOs is not supported yet")
	default:
		return fmt.Errorf("unsupported operating system")
	}

	logrus.Info("now you can use pgrx to build your postgres extension")
	return nil
}
