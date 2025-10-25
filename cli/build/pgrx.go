package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

func SetupPgrx(pgrxVersion string, pgVersions string) error {
	if pgrxVersion == "" {
		pgrxVersion = "0.16.1"
	}
	cargoBin := config.HomeDir + "/.cargo/bin/cargo"

	// Check if cargo is installed
	if _, err := os.Stat(cargoBin); err != nil {
		return fmt.Errorf("cargo not found at %s, please install rust first with: pig build rust", cargoBin)
	}

	// Check if pgrx is already installed, always install/update
	checkCmd := exec.Command(cargoBin, "pgrx", "--version")
	output, err := checkCmd.CombinedOutput()
	if err == nil {
		currentVersion := strings.TrimSpace(string(output))
		if strings.Contains(currentVersion, pgrxVersion) {
			logrus.Infof("cargo-pgrx %s already installed", pgrxVersion)
		} else {
			logrus.Infof("Current pgrx version: %s, updating to %s", currentVersion, pgrxVersion)
			if err := installPgrx(cargoBin, pgrxVersion); err != nil {
				return err
			}
		}
	} else {
		logrus.Infof("Installing cargo-pgrx %s", pgrxVersion)
		if err := installPgrx(cargoBin, pgrxVersion); err != nil {
			return err
		}
	}

	// Initialize pgrx based on OS type and pgVersions parameter
	logrus.Infof("Initializing pgrx %s", pgrxVersion)

	// Special case: if pgVersions is "init", run cargo pgrx init without any arguments
	if pgVersions == "init" {
		logrus.Info("Running cargo pgrx init without arguments")
		args := []string{cargoBin, "pgrx", "init"}
		logrus.Debugf("Executing command: %s", strings.Join(args, " "))
		if err := utils.Command(args); err != nil {
			return fmt.Errorf("failed to initialize pgrx: %v", err)
		}
		logrus.Info("pgrx initialized successfully, you can now use it to build PostgreSQL extensions")
		return nil
	}

	// Parse pgVersions if specified
	var versions []string
	if pgVersions != "" {
		versions = strings.Split(pgVersions, ",")
		for i, v := range versions {
			versions[i] = strings.TrimSpace(v)
		}
	}

	// Build pgrx init arguments based on OS type
	var initArgs []string
	switch config.OSType {
	case config.DistroEL:
		initArgs = buildELPgrxArgs(cargoBin, versions)
	case config.DistroDEB:
		initArgs = buildDEBPgrxArgs(cargoBin, versions)
	case config.DistroMAC:
		initArgs = buildMacPgrxArgs(cargoBin, versions)
	default:
		return fmt.Errorf("unsupported operating system: %s", config.OSType)
	}

	// Execute pgrx init command
	logrus.Debugf("Executing command: %s", strings.Join(initArgs, " "))
	if err := utils.Command(initArgs); err != nil {
		return fmt.Errorf("failed to initialize pgrx: %v", err)
	}

	logrus.Info("pgrx initialized successfully, you can now use it to build PostgreSQL extensions")
	return nil
}

func installPgrx(cargoBin string, pgrxVersion string) error {
	if err := utils.Command([]string{cargoBin, "install", "--locked", fmt.Sprintf("cargo-pgrx@%s", pgrxVersion)}); err != nil {
		return fmt.Errorf("failed to install cargo-pgrx %s: %v", pgrxVersion, err)
	}
	logrus.Infof("Successfully installed cargo-pgrx %s", pgrxVersion)
	return nil
}

func buildELPgrxArgs(cargoBin string, versions []string) []string {
	args := []string{cargoBin, "pgrx", "init"}

	// If versions are specified, use them
	if len(versions) > 0 {
		for _, ver := range versions {
			pgConfig := fmt.Sprintf("/usr/pgsql-%s/bin/pg_config", ver)
			args = append(args, fmt.Sprintf("--pg%s=%s", ver, pgConfig))
		}
	} else {
		// Auto-detect installed versions
		logrus.Info("Auto-detecting PostgreSQL installations for EL-based system")
		for _, ver := range []string{"13", "14", "15", "16", "17", "18"} {
			pgConfig := fmt.Sprintf("/usr/pgsql-%s/bin/pg_config", ver)
			if _, err := os.Stat(pgConfig); err == nil {
				args = append(args, fmt.Sprintf("--pg%s=%s", ver, pgConfig))
				logrus.Debugf("Found PostgreSQL %s at %s", ver, pgConfig)
			}
		}
	}

	return args
}

func buildDEBPgrxArgs(cargoBin string, versions []string) []string {
	args := []string{cargoBin, "pgrx", "init"}

	// If versions are specified, use them
	if len(versions) > 0 {
		for _, ver := range versions {
			pgConfig := fmt.Sprintf("/usr/lib/postgresql/%s/bin/pg_config", ver)
			args = append(args, fmt.Sprintf("--pg%s=%s", ver, pgConfig))
		}
	} else {
		// Auto-detect installed versions
		logrus.Info("Auto-detecting PostgreSQL installations for DEB-based system")
		for _, ver := range []string{"13", "14", "15", "16", "17", "18"} {
			pgConfig := fmt.Sprintf("/usr/lib/postgresql/%s/bin/pg_config", ver)
			if _, err := os.Stat(pgConfig); err == nil {
				args = append(args, fmt.Sprintf("--pg%s=%s", ver, pgConfig))
				logrus.Debugf("Found PostgreSQL %s at %s", ver, pgConfig)
			}
		}
	}

	return args
}

func buildMacPgrxArgs(cargoBin string, versions []string) []string {
	args := []string{cargoBin, "pgrx", "init"}

	// If versions are specified, use them but need to find actual paths
	if len(versions) > 0 {
		logrus.Info("Finding PostgreSQL installations for specified versions on macOS")
		for _, ver := range versions {
			pgConfig := findMacPGConfig(ver)
			if pgConfig != "" {
				args = append(args, fmt.Sprintf("--pg%s=%s", ver, pgConfig))
				logrus.Debugf("Found PostgreSQL %s at %s", ver, pgConfig)
			} else {
				logrus.Warnf("PostgreSQL %s not found on macOS", ver)
			}
		}
	} else {
		// Auto-detect installed versions
		logrus.Info("Auto-detecting PostgreSQL installations for macOS")
		for _, ver := range []string{"13", "14", "15", "16", "17", "18"} {
			pgConfig := findMacPGConfig(ver)
			if pgConfig != "" {
				args = append(args, fmt.Sprintf("--pg%s=%s", ver, pgConfig))
				logrus.Debugf("Found PostgreSQL %s at %s", ver, pgConfig)
			}
		}
	}

	// If no versions found, try default location
	if len(args) == 3 { // Only has cargo, pgrx, init
		logrus.Warn("No PostgreSQL installations found via Homebrew, trying default locations")
		if _, err := os.Stat("/opt/homebrew/bin/pg_config"); err == nil {
			// Don't add any version-specific arguments, let pgrx use default
			logrus.Info("Found default PostgreSQL installation, using cargo pgrx init without version arguments")
		} else if _, err := os.Stat("/usr/local/bin/pg_config"); err == nil {
			logrus.Info("Found default PostgreSQL installation, using cargo pgrx init without version arguments")
		} else {
			logrus.Error("No PostgreSQL installations found, please install PostgreSQL first")
		}
	}

	return args
}

// findMacPGConfig finds the pg_config path for a specific PostgreSQL version on macOS
func findMacPGConfig(majorVersion string) string {
	// Check common Homebrew locations
	patterns := []string{
		fmt.Sprintf("/opt/homebrew/Cellar/postgresql@%s/*/bin/pg_config", majorVersion),
		fmt.Sprintf("/usr/local/Cellar/postgresql@%s/*/bin/pg_config", majorVersion),
		fmt.Sprintf("/opt/homebrew/opt/postgresql@%s/bin/pg_config", majorVersion),
		fmt.Sprintf("/usr/local/opt/postgresql@%s/bin/pg_config", majorVersion),
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		if len(matches) > 0 {
			// If multiple versions found, get the latest one
			if len(matches) > 1 {
				matches = sortByVersion(matches)
			}
			return matches[len(matches)-1] // Return the latest version
		}
	}

	return ""
}

// sortByVersion sorts paths containing version numbers
func sortByVersion(paths []string) []string {
	type pathVersion struct {
		path    string
		version []int
	}

	var pvs []pathVersion

	for _, p := range paths {
		// Extract version from path like "/opt/homebrew/Cellar/postgresql@17/17.6/bin/pg_config"
		parts := strings.Split(p, "/")
		for _, part := range parts {
			// Look for version pattern like "17.6" or "17.6.1"
			if strings.Contains(part, ".") {
				versionParts := strings.Split(part, ".")
				var versionInts []int
				allNumeric := true
				for _, vp := range versionParts {
					if num, err := strconv.Atoi(vp); err == nil {
						versionInts = append(versionInts, num)
					} else {
						allNumeric = false
						break
					}
				}
				if allNumeric && len(versionInts) > 0 {
					pvs = append(pvs, pathVersion{path: p, version: versionInts})
					break
				}
			}
		}
	}

	// If we couldn't parse versions, return original order
	if len(pvs) != len(paths) {
		return paths
	}

	// Sort by version
	sort.Slice(pvs, func(i, j int) bool {
		vi, vj := pvs[i].version, pvs[j].version
		for k := 0; k < len(vi) && k < len(vj); k++ {
			if vi[k] != vj[k] {
				return vi[k] < vj[k]
			}
		}
		return len(vi) < len(vj)
	})

	result := make([]string, len(pvs))
	for i, pv := range pvs {
		result[i] = pv.path
	}

	return result
}