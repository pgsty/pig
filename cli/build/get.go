package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

type downloadTask struct {
	SrcURL  string
	DstPath string
}

const parallelWorkers = 8

// DownloadCodeTarball downloads extension source tarballs from repository
func DownloadCodeTarball(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided")
	}

	baseURL := config.RepoPigstyCC + "/ext/src"

	// Prepare tarball directory
	tarballDir := getTarballDir()
	if err := os.MkdirAll(tarballDir, 0755); err != nil {
		return fmt.Errorf("failed to create tarball directory: %v", err)
	}

	// Resolve arguments to download tasks
	tasks := resolveTasks(args, baseURL, tarballDir)
	if len(tasks) == 0 {
		logrus.Warn("nothing to download")
		return nil
	}

	// Download all tasks in parallel
	parallelDownload(tasks, parallelWorkers)
	return nil
}

// resolveTasks converts arguments to download tasks
func resolveTasks(args []string, baseURL, tarballDir string) []downloadTask {
	var tasks []downloadTask
	seen := make(map[string]bool) // Prevent duplicate downloads

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}

		// Try to resolve as extension name/pkg
		lowerArg := strings.ToLower(arg)
		var extension *ext.Extension

		// Check extension catalogs
		if e, ok := ext.Catalog.ExtNameMap[lowerArg]; ok {
			extension = e
		} else if e, ok := ext.Catalog.ExtAliasMap[lowerArg]; ok {
			extension = e
		}

		// Handle extension case
		if extension != nil {
			if extension.Source == "" {
				logrus.Warnf("extension '%s' has no source file", extension.Name)
				continue
			}
			if seen[extension.Source] {
				logrus.Debugf("skip duplicate: %s", extension.Source)
				continue
			}
			seen[extension.Source] = true

			srcURL := fmt.Sprintf("%s/%s", baseURL, extension.Source)
			dstPath := filepath.Join(tarballDir, extension.Source)
			tasks = append(tasks, downloadTask{
				SrcURL:  srcURL,
				DstPath: dstPath,
			})
			logrus.Debugf("resolved extension %s -> %s", arg, extension.Source)
		} else {
			// Treat as plain filename
			if seen[arg] {
				logrus.Debugf("skip duplicate: %s", arg)
				continue
			}
			seen[arg] = true

			srcURL := fmt.Sprintf("%s/%s", baseURL, arg)
			dstPath := filepath.Join(tarballDir, arg)
			tasks = append(tasks, downloadTask{
				SrcURL:  srcURL,
				DstPath: dstPath,
			})
			logrus.Debugf("resolved filename: %s", arg)
		}
	}

	return tasks
}

func getTarballDir() string {
	switch config.OSType {
	case config.DistroEL:
		return filepath.Join(config.HomeDir, "rpmbuild", "SOURCES")
	case config.DistroDEB:
		return filepath.Join(config.HomeDir, "deb", "tarball")
	default:
		return "/tmp"
	}
}

// parallelDownload downloads files concurrently with worker pool
func parallelDownload(tasks []downloadTask, workers int) {
	if len(tasks) == 0 {
		return
	}

	// Adjust worker count based on task count
	if workers > len(tasks) {
		workers = len(tasks)
	}

	var wg sync.WaitGroup
	taskCh := make(chan downloadTask, len(tasks))

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go downloadWorker(&wg, taskCh)
	}

	// Send tasks to workers
	for _, task := range tasks {
		taskCh <- task
	}
	close(taskCh)

	// Wait for completion
	wg.Wait()
}

func downloadWorker(wg *sync.WaitGroup, taskCh <-chan downloadTask) {
	defer wg.Done()
	for task := range taskCh {
		downloadFile(task)
	}
}

func downloadFile(task downloadTask) {
	// Extract filename from path for logging
	filename := filepath.Base(task.DstPath)
	logrus.Infof("downloading %s from %s", filename, task.SrcURL)

	err := utils.DownloadFile(task.SrcURL, task.DstPath)
	if err != nil {
		logrus.Errorf("failed to download %s: %v", filename, err)
	} else {
		logrus.Infof("downloaded %s to %s", filename, task.DstPath)
	}
}
