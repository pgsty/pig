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

// SpecialSourceMapping defines special case mappings for non-extension packages
var SpecialSourceMapping = map[string][]string{
	"scws":        {"scws-1.2.3.tar.bz2"},
	"openhalodb":  {"openhalodb-1.0.tar.gz"},
	"cloudberry":  {"apache-cloudberry-2.0.0-incubating-src.tar.gz"},
	"babelfishpg": {"babelfishpg-17.8-5.5.0.tar.gz"},
	"babelfish":   {"babelfishpg-17.8-5.5.0.tar.gz"},
	"antlr4":      {"antlr4-cpp-runtime-4.13.2-source.zip"},
	"oriolepg":    {"oriolepg-17.16.tar.gz"},
	"orioledb":    {"orioledb-beta14.tar.gz"},
	"agensgraph":  {"agensgraph-2.16.0.tar.gz"},
	"agentsgraph": {"agensgraph-2.16.0.tar.gz"},
	"pgedge":      {"postgresql-17.9.tar.gz", "spock-5.0.5.tar.gz"},
	"hunspell":    {"hunspell-1.0.tar.gz"},
	"libfq":       {"libfq-0.6.1.tar.gz"},

	// Multi-version PostgreSQL source packages
	"libfepgutils": {
		"postgresql-14.19.tar.gz",
		"postgresql-15.14.tar.gz",
		"postgresql-16.10.tar.gz",
		"postgresql-17.6.tar.gz",
		"postgresql-18.0.tar.gz",
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
	// 1. Try as extension
	if ext, err := ResolvePackage(pkg); err == nil && ext.Source != "" {
		return splitAndCleanSources(ext.Source)
	}

	// 2. Check special mapping
	if mapped, exists := SpecialSourceMapping[pkg]; exists {
		var sources []string
		for _, item := range mapped {
			sources = append(sources, splitAndCleanSources(item)...)
		}
		return dedupeSources(sources)
	}

	// 3. Treat as filename
	return []string{pkg}
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
