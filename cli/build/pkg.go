// Package build - pipeline.go contains complete build pipeline for packages
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

var cloudberryBuildComponents = []string{"cloudberry", "cloudberry-backup", "cloudberry-pxf"}

// BuildPackage runs complete build pipeline for a single package
func BuildPackage(pkg string, pgVersions string, withSymbol bool, mirror bool) error {
	if normalizeBuildName(pkg) == "cloudberry" {
		return BuildCloudberryPackage(pgVersions, withSymbol, mirror)
	}

	fmt.Printf("\n")
	logrus.Info(strings.Repeat("#", 58))
	logrus.Infof("[BUILD PKG] %s", pkg)
	logrus.Info(strings.Repeat("#", 58))

	// Step 1: Download source
	if err := DownloadSource(pkg, false, mirror); err != nil {
		// Source is a hard dependency for building; do not proceed when missing.
		return fmt.Errorf("source download failed for %s: %w", pkg, err)
	}

	// Step 2: Install dependencies
	if err := InstallDeps(pkg, pgVersions); err != nil {
		// Deps may already be installed; keep going, but surface the error clearly.
		logrus.Warnf("Dependency install error for %s: %v", pkg, err)
	}

	// Step 3: Build package
	if err := BuildExtension(pkg, pgVersions, withSymbol); err != nil {
		return fmt.Errorf("build failed for %s: %w", pkg, err)
	}

	logrus.Info(strings.Repeat("#", 58))
	return nil
}

// BuildCloudberryPackage builds the Cloudberry suite in dependency order.
func BuildCloudberryPackage(pgVersions string, withSymbol bool, mirror bool) error {
	fmt.Printf("\n")
	logrus.Info(strings.Repeat("#", 58))
	logrus.Info("[BUILD PKG] cloudberry suite")
	logrus.Info(strings.Repeat("#", 58))

	if err := DownloadSource("cloudberry", false, mirror); err != nil {
		return fmt.Errorf("source download failed for cloudberry suite: %w", err)
	}

	for _, component := range cloudberryBuildComponents {
		if err := InstallDeps(component, pgVersions); err != nil {
			logrus.Warnf("Dependency install error for %s: %v", component, err)
		}
		if err := BuildExtension(component, pgVersions, withSymbol); err != nil {
			return fmt.Errorf("build failed for %s: %w", component, err)
		}
		if component == "cloudberry" {
			if err := installBuiltPackage("cloudberry"); err != nil {
				return err
			}
		}
	}

	logrus.Info(strings.Repeat("#", 58))
	return nil
}

func installBuiltPackage(pkg string) error {
	artifact, err := findBuiltPackageArtifact(pkg)
	if err != nil {
		return err
	}

	logrus.Infof("installing local build artifact: %s", artifact)
	switch config.OSType {
	case config.DistroEL:
		return utils.SudoCommand([]string{"dnf", "install", "-y", artifact})
	case config.DistroDEB:
		return utils.SudoCommand([]string{"apt-get", "install", "-y", artifact})
	default:
		return fmt.Errorf("unsupported OS type for package install: %s", config.OSType)
	}
}

type packageArtifact struct {
	path    string
	modTime int64
}

func findBuiltPackageArtifact(pkg string) (string, error) {
	var patterns []string
	switch config.OSType {
	case config.DistroEL:
		patterns = []string{
			filepath.Join(config.HomeDir, "ext", "pkg", "*", pkg+"-[0-9]*.rpm"),
			filepath.Join(config.HomeDir, "ext", "pkg", pkg+"-[0-9]*.rpm"),
			filepath.Join(config.HomeDir, "rpmbuild", "RPMS", "*", pkg+"-[0-9]*.rpm"),
		}
	case config.DistroDEB:
		patterns = []string{
			filepath.Join(config.HomeDir, "ext", "pkg", pkg+"_[0-9]*.deb"),
			filepath.Join(config.HomeDir, "debbuild", pkg+"_[0-9]*.deb"),
		}
	default:
		return "", fmt.Errorf("unsupported OS type for artifact lookup: %s", config.OSType)
	}

	seen := make(map[string]struct{})
	var artifacts []packageArtifact
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logrus.Debugf("glob pattern error for %s: %v", pattern, err)
			continue
		}
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			artifacts = append(artifacts, packageArtifact{path: match, modTime: info.ModTime().UnixNano()})
		}
	}
	if len(artifacts) == 0 {
		return "", fmt.Errorf("built package artifact not found for %s", pkg)
	}

	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].modTime == artifacts[j].modTime {
			return artifacts[i].path > artifacts[j].path
		}
		return artifacts[i].modTime > artifacts[j].modTime
	})
	return artifacts[0].path, nil
}

// BuildPackages processes multiple packages sequentially
func BuildPackages(packages []string, pgVersions string, withSymbol bool, mirror bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	success := 0
	total := len(packages)
	var failed []string

	// Process each package completely before moving to next
	for _, pkg := range packages {
		if err := BuildPackage(pkg, pgVersions, withSymbol, mirror); err != nil {
			failed = append(failed, pkg)
			logrus.Errorf("Failed to build %s: %v", pkg, err)
			continue
		}
		success++
		fmt.Printf("\n\n")
	}

	// Final summary
	if success == total {
		logrus.Infof("All %d packages build successfully", total)
	} else if success > 0 {
		logrus.Warnf("%d of %d packages build completed", success, total)
	} else {
		logrus.Errorf("All %d packages build failed", total)
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d of %d package(s) failed: %s", len(failed), total, strings.Join(failed, ", "))
	}
	return nil
}
