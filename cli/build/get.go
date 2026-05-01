// Package build - source.go handles source code download for packages
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// SpecialSourceMapping defines authoritative source bundles for packages that
// need manual overrides or supplemental source artifacts beyond catalog Source.
var SpecialSourceMapping = map[string][]string{
	"scws":       {"scws-1.2.3.tar.bz2"},
	"openhalodb": {"openhalodb-1.0.tar.gz"},
	"cloudberry": {
		"apache-cloudberry-2.1.0-incubating-src.tar.gz",
		"psutil-5.7.0.tar.gz",
		"PyGreSQL-5.2.tar.gz",
		"PyYAML-5.4.1.tar.gz",
		"cloudberry-2.1.0-rpm-patches.tar.gz",
		"apache-cloudberry-backup-2.1.0-incubating-src.tar.gz",
		"apache-cloudberry-pxf-2.1.0-incubating-src.tar.gz",
		"cloudberry-pxf-2.1.0-rpm-patches.tar.gz",
	},
	"cloudberry-backup": {"apache-cloudberry-backup-2.1.0-incubating-src.tar.gz"},
	"cloudberry-pxf": {
		"apache-cloudberry-pxf-2.1.0-incubating-src.tar.gz",
		"cloudberry-pxf-2.1.0-rpm-patches.tar.gz",
	},
	"babelfishpg": {"babelfishpg-17.8-5.5.0.tar.gz"},
	"babelfish":   {"babelfishpg-17.8-5.5.0.tar.gz"},
	"antlr4":      {"antlr4-cpp-runtime-4.13.2-source.zip"},
	"oriolepg":    {"postgres-patches17_18.tar.gz"},
	"orioledb":    {"orioledb-beta15.tar.gz"},
	"agensgraph":  {"agensgraph-2.16.0.tar.gz"},
	"agentsgraph": {"agensgraph-2.16.0.tar.gz"},
	"pgedge":      {"postgresql-18.3.tar.gz", "spock-5.0.6.tar.gz"},
	"hunspell":    {"hunspell-1.0.tar.gz"},
	"libfq":       {"libfq-0.6.1.tar.gz"},
	"pdu":         {"pdu-3.0.25.12.tar.gz"},
	"pgdog":       {"pgdog-0.1.32.tar.gz"},
	"inchi":       {"inchi-1.07.3.tar.gz"},
	"graphblas":   {"graphblas-10.2.0.tar.gz"},
	"lagraph":     {"lagraph-1.2.1.tar.gz"},
	"rdkit": {
		"rdkit_202503.6.orig.tar.xz",
		"better-enums-0.11.3-enum.h",
		"rapidjson-1.1.0.tar.gz",
		"inchi-1.07.3.tar.gz",
	},
	"onesparse": {
		"onesparse-1.0.0.tar.gz",
		"graphblas-10.2.0.tar.gz",
		"lagraph-1.2.1.tar.gz",
	},

	// Multi-version PostgreSQL source packages
	"libfepgutils": {
		"postgresql-14.22.tar.gz",
		"postgresql-15.17.tar.gz",
		"postgresql-16.13.tar.gz",
		"postgresql-17.9.tar.gz",
		"postgresql-18.3.tar.gz",
	},
}

// DownloadSource downloads source tarball for a single package
func DownloadSource(pkg string, force bool, mirror bool) error {
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[GET SOURCE] %s", pkg)
	logrus.Info(strings.Repeat("=", 58))

	// Ensure source directory exists
	srcDir := filepath.Join(config.HomeDir, "ext", "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %v", err)
	}

	// Get source files for this package
	sources := getSourceFiles(pkg)
	if len(sources) == 0 {
		logrus.Infof("No source files for %s", pkg)
		return nil
	}

	// Prefer the requested base URL, but fallback to the other one to reduce flakiness
	// during mirror switching or CDN propagation.
	baseURLs := []string{config.RepoPigstyIO, config.RepoPigstyCC}
	if mirror {
		baseURLs = []string{config.RepoPigstyCC, config.RepoPigstyIO}
	}

	// Download each source file
	for _, src := range sources {
		if err := downloadFile(src, srcDir, force, baseURLs); err != nil {
			return err
		}
	}

	return nil
}

// DownloadSources processes multiple packages
func DownloadSources(packages []string, force bool, mirror bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	var failed []string

	for _, pkg := range packages {
		if err := DownloadSource(pkg, force, mirror); err != nil {
			logrus.Errorf("Failed to download %s: %v", pkg, err)
			failed = append(failed, pkg)
			continue
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d of %d package(s) failed to download: %s", len(failed), len(packages), strings.Join(failed, ", "))
	}
	return nil
}

// Get source files for a package
func getSourceFiles(pkg string) []string {
	// 1. Prefer authoritative special bundles, even for extension packages, so
	// stale catalog Source entries can be overridden with the exact build set.
	if ext, err := ResolvePackage(pkg); err == nil {
		if special := getSpecialSources(pkg, ext.Name, ext.Pkg); len(special) > 0 {
			return special
		}
		if ext.Source != "" {
			return splitAndCleanSources(ext.Source)
		}
	}

	// 2. Check special mapping
	if special := getSpecialSources(pkg); len(special) > 0 {
		return special
	}

	// 3. Treat as filename
	return []string{pkg}
}

func getSpecialSources(keys ...string) []string {
	var sources []string
	seenKeys := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, seen := seenKeys[key]; seen {
			continue
		}
		seenKeys[key] = struct{}{}
		mapped, exists := SpecialSourceMapping[key]
		if !exists {
			continue
		}
		for _, item := range mapped {
			sources = append(sources, splitAndCleanSources(item)...)
		}
	}
	return dedupeSources(sources)
}

// splitAndCleanSources splits source field by whitespace and removes empty items.
func splitAndCleanSources(raw string) []string {
	parts := strings.Fields(raw)
	sources := make([]string, 0, len(parts))
	for _, part := range parts {
		if src := strings.TrimSpace(part); src != "" {
			sources = append(sources, src)
		}
	}
	return sources
}

func dedupeSources(sources []string) []string {
	if len(sources) <= 1 {
		return sources
	}
	result := make([]string, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		if _, exists := seen[src]; exists {
			continue
		}
		seen[src] = struct{}{}
		result = append(result, src)
	}
	return result
}

// Download a single file
func downloadFile(filename, dstDir string, force bool, baseURLs []string) error {
	dstPath := filepath.Join(dstDir, filename)

	// Check if exists
	if !force {
		if _, err := os.Stat(dstPath); err == nil {
			logrus.Infof("Already exists: %s", dstPath)
			return nil
		}
	} else {
		os.Remove(dstPath)
	}

	if len(baseURLs) == 0 {
		baseURLs = []string{config.RepoPigstyIO, config.RepoPigstyCC}
	}

	var lastErr error
	for _, baseURL := range baseURLs {
		url := fmt.Sprintf("%s/ext/src/%s", baseURL, filename)
		logrus.Infof("Downloading from %s", url)
		if err := utils.DownloadFile(url, dstPath); err != nil {
			lastErr = err
			logrus.Debugf("download attempt failed: %s: %v", url, err)
			continue
		}
		logrus.Infof("Downloaded to %s", dstPath)
		return nil
	}

	return fmt.Errorf("failed to download %s from %s: %v", filename, strings.Join(baseURLs, ", "), lastErr)
}
