package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// Download source tarballs for packages
func DownloadCodeTarball(args []string, force bool) error {
	if len(args) == 0 {
		return fmt.Errorf("no packages specified")
	}

	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[GET SOURCE] %s", strings.Join(args, ", "))
	logrus.Info(strings.Repeat("=", 58))

	// Ensure directories exist
	srcDir := filepath.Join(config.HomeDir, "ext", "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %v", err)
	}

	// Collect source files to download
	sources := collectSources(args)
	if len(sources) == 0 {
		logrus.Info("No source files to download")
		return nil
	}

	// Download in parallel
	downloadSources(sources, srcDir, force)
	return nil
}

// Collect source files from package names
func collectSources(packages []string) []string {
	seen := make(map[string]bool)
	var sources []string

	for _, pkg := range packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}

		// 1. Try as extension
		if ext, err := ResolvePackage(pkg); err == nil && ext.Source != "" {
			if !seen[ext.Source] {
				sources = append(sources, ext.Source)
				seen[ext.Source] = true
			}
			continue
		}

		// 2. Check special mapping
		if mapped, exists := SpecialSourceMapping[pkg]; exists {
			for _, src := range mapped {
				if !seen[src] {
					sources = append(sources, src)
					seen[src] = true
				}
			}
			continue
		}

		// 3. Treat as plain filename
		if !seen[pkg] {
			sources = append(sources, pkg)
			seen[pkg] = true
		}
	}

	return sources
}

// Download source files in parallel
func downloadSources(sources []string, dstDir string, force bool) {
	var wg sync.WaitGroup
	ch := make(chan string, len(sources))

	// Start 8 workers
	workers := 8
	if len(sources) < workers {
		workers = len(sources)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for src := range ch {
				downloadSource(src, dstDir, force)
			}
		}()
	}

	// Queue downloads
	for _, src := range sources {
		ch <- src
	}
	close(ch)

	wg.Wait()
}

// Download a single source file
func downloadSource(filename, dstDir string, force bool) {
	dstPath := filepath.Join(dstDir, filename)

	// Check if exists
	if !force {
		if _, err := os.Stat(dstPath); err == nil {
			logrus.Infof("✓ Already exists: %s", filename)
			return
		}
	}

	// Remove if forcing
	if force {
		os.Remove(dstPath)
	}

	// Download
	url := fmt.Sprintf("%s/ext/src/%s", config.RepoPigstyCC, filename)
	logrus.Infof("Downloading %s", filename)

	if err := utils.DownloadFile(url, dstPath); err != nil {
		logrus.Errorf("Failed to download %s: %v", filename, err)
	} else {
		logrus.Infof("Downloaded %s", filename)
	}
}
