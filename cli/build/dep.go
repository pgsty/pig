package build

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

func InstallExtensionDeps(args []string, pgVersions string) error {
	switch config.OSType {
	case config.DistroEL:
		return installRpmDeps(args, pgVersions)
	case config.DistroDEB:
		return installDebDeps(args)
	default:
		return fmt.Errorf("unsupported operating system")
	}
}

func installRpmDeps(args []string, pgVersions string) error {
	if len(args) == 0 {
		return fmt.Errorf("no extensions specified")
	}

	specsDir := filepath.Join(config.HomeDir, "rpmbuild", "SPECS")
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		return fmt.Errorf("SPECS directory not found at %s, please run `pig build spec` first", specsDir)
	}

	// Resolve extensions to packages
	packages := resolveExtensionPackages(args)
	if len(packages) == 0 {
		return fmt.Errorf("no valid packages found")
	}

	// Process each package
	for _, pkg := range packages {
		specFile := filepath.Join(specsDir, pkg+".spec")

		// Check spec file exists
		if _, err := os.Stat(specFile); os.IsNotExist(err) {
			logrus.Errorf("spec file not found: %s", specFile)
			continue
		}

		// Determine PG version
		pgVer := resolvePgVersion(pkg, pgVersions)
		if pgVer == "" {
			logrus.Warnf("could not determine PG version for %s", pkg)
			continue
		}

		// Build and execute command
		cmd := []string{
			"dnf", "builddep", "-y",
			"--define", fmt.Sprintf("pgmajorversion %s", pgVer),
			specFile,
		}

		logrus.Infof("installing dependencies for %s (pg%s)", pkg, pgVer)
		logrus.Debugf("executing: sudo %s", strings.Join(cmd, " "))

		if err := utils.SudoCommand(cmd); err != nil {
			logrus.Errorf("failed to install dependencies for %s: %v", pkg, err)
			return err
		}

		logrus.Infof("successfully installed dependencies for %s", pkg)
	}

	return nil
}

// resolveExtensionPackages converts extension names to package names
func resolveExtensionPackages(args []string) []string {
	seen := make(map[string]bool)
	var packages []string

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}

		// Try to resolve as extension
		lowerArg := strings.ToLower(arg)
		var pkg string

		// Check extension catalogs
		if e, ok := ext.Catalog.ExtNameMap[lowerArg]; ok && e.Lead {
			pkg = e.Pkg
		} else if e, ok := ext.Catalog.ExtPkgMap[lowerArg]; ok && e.Lead {
			pkg = e.Pkg
		} else {
			// Treat as package name directly
			pkg = arg
		}

		if pkg != "" && !seen[pkg] {
			packages = append(packages, pkg)
			seen[pkg] = true
			logrus.Debugf("resolved %s -> package %s", arg, pkg)
		}
	}

	return packages
}

// resolvePgVersion determines the PG version to use
func resolvePgVersion(pkg string, pgVersions string) string {
	// If explicitly specified, use the first version
	if pgVersions != "" {
		versions := strings.Split(pgVersions, ",")
		if len(versions) > 0 {
			return strings.TrimSpace(versions[0])
		}
	}

	// Auto-detect from extension metadata
	lowerPkg := strings.ToLower(pkg)
	for _, e := range ext.Catalog.Extensions {
		if strings.ToLower(e.Pkg) == lowerPkg && e.Lead {
			// Find max version from RpmPg field
			maxVer := 0
			for _, ver := range e.RpmPg {
				if v, err := strconv.Atoi(ver); err == nil && v > maxVer {
					maxVer = v
				}
			}
			if maxVer > 0 {
				return strconv.Itoa(maxVer)
			}
			break
		}
	}

	// Default fallback
	return "16"
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
