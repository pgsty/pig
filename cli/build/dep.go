package build

import (
	"fmt"
	"os"
	"path"
	"pig/internal/config"
	"pig/internal/utils"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

func InstallExtensionDeps(args []string) error {
	switch config.OSType {
	case config.DistroEL:
		return installRpmDeps(args)
	case config.DistroDEB:
		return installDebDeps(args)
	default:
		return fmt.Errorf("unsupported operating system")
	}
}

func installRpmDeps(extlist []string) error {
	workDir := config.HomeDir + "/rpmbuild/"
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("rpmbuild directory not found, please run `pig build spec` first")
	}
	os.Chdir(workDir)

	logrus.Infof("install dependencies for extensions: %s in %s", strings.Join(extlist, ","), workDir)
	for _, ext := range extlist {
		logrus.Infof("################ %s install begin in %s", ext, workDir)
		err := utils.Command([]string{"./dep", ext})
		if err != nil {
			logrus.Errorf("################  %s install failed: %v", ext, err)
			return err
		} else {
			logrus.Infof("################  %s install success", ext)
		}
	}

	return nil
}

// deb does not support install deb dependencies easily, so we need to extract build-depends from control file
func installDebDeps(extlist []string) error {
	workDir := config.HomeDir + "/deb/"
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("deb directory not found, please run `pig build spec` first")
	}
	os.Chdir(workDir)

	logrus.Infof("install dependencies for extensions: %s in %s", strings.Join(extlist, ","), workDir)

	// Collect all dependencies from all extensions
	allDeps := make(map[string]struct{})
	for _, ext := range extlist {
		debExtName := strings.ReplaceAll(ext, "_", "-")
		controFile := path.Join(workDir, debExtName, "debian", "control.in")
		controFile2 := path.Join(workDir, debExtName, "debian", "control.in1")

		// Check if control file exists
		if _, err := os.Stat(controFile); os.IsNotExist(err) {
			logrus.Debugf("main control file template not found: %s", controFile)
			if _, err := os.Stat(controFile2); os.IsNotExist(err) {
				logrus.Warnf("control file not found: %s, %s", controFile, controFile2)
				continue
			} else {
				controFile = controFile2
			}
		}

		deps, err := extractBuildDependencies(controFile, ext)
		if err != nil {
			logrus.Errorf("Failed to extract build dependencies for %s: %v", ext, err)
			return err
		}

		// Add dependencies to the map for deduplication
		for _, dep := range deps {
			allDeps[dep] = struct{}{}
		}

		logrus.Debugf("Build-Depends for %s: %s", ext, strings.Join(deps, ", "))
	}

	// Convert map to sorted slice
	var uniqueDeps []string
	for dep := range allDeps {
		// dont' want postgresql-all and debhelper-compat here
		if dep == "postgresql-all" || dep == "debhelper-compat" {
			continue
		}
		uniqueDeps = append(uniqueDeps, dep)
	}

	// Sort dependencies alphabetically
	if len(uniqueDeps) > 0 {
		sort.Strings(uniqueDeps)

		// Prepare and execute installation command
		installCmd := []string{"apt", "install", "-y"}
		installCmd = append(installCmd, uniqueDeps...)

		logrus.Infof("Installing all dependencies: %s", strings.Join(uniqueDeps, ", "))
		logrus.Infof("apt install -y %s", strings.Join(uniqueDeps, " "))
		err := utils.SudoCommand(installCmd)
		if err != nil {
			logrus.Errorf("Failed to install dependencies: %v", err)
			return err
		}
		logrus.Infof("Successfully installed all dependencies")
	} else {
		logrus.Infof("No dependencies found for the specified extensions")
	}

	return nil
}

func extractBuildDependencies(controFile string, ext string) (res []string, err error) {
	content, err := os.ReadFile(controFile)
	if err != nil {
		logrus.Errorf("Failed to read control file %s: %v", controFile, err)
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	var found bool
	var deps []string
	for _, line := range lines {
		if strings.HasPrefix(line, "Build-Depends:") {
			depStr := strings.TrimPrefix(line, "Build-Depends:")
			logrus.Infof("Build-Depends for %s: %s", ext, depStr)
			found = true
			deps = strings.Split(depStr, ",")
			break
		}
	}
	if found {
		for _, dep := range deps {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			// Remove version constraints in parentheses
			if idx := strings.Index(dep, "("); idx > 0 {
				dep = strings.TrimSpace(dep[:idx])
			}
			dep = strings.Trim(dep, "()")
			res = append(res, dep)
		}
	} else {
		logrus.Warnf("Build-Depends for %s not found", ext)
	}
	return res, nil
}
