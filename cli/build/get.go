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
func DownloadCodeTarball(args []string, forceDownload bool) error {
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
	tasks := resolveTasks(args, baseURL, tarballDir, forceDownload)
	if len(tasks) == 0 {
		logrus.Warn("nothing to download")
		return nil
	}

	// Download all tasks in parallel
	parallelDownload(tasks, parallelWorkers, forceDownload)
	return nil
}

// resolveTasks converts arguments to download tasks
func resolveTasks(args []string, baseURL, tarballDir string, forceDownload bool) []downloadTask {
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
		} else if e, ok := ext.Catalog.ExtPkgMap[lowerArg]; ok {
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

			// Check if file exists and skip if not forcing
			if !forceDownload {
				if _, err := os.Stat(dstPath); err == nil {
					logrus.Infof("file already exists, skipping: %s", extension.Source)
					continue
				}
			}

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

			// Check if file exists and skip if not forcing
			if !forceDownload {
				if _, err := os.Stat(dstPath); err == nil {
					logrus.Infof("file already exists, skipping: %s", arg)
					continue
				}
			}

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
func parallelDownload(tasks []downloadTask, workers int, forceDownload bool) {
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
		go downloadWorker(&wg, taskCh, forceDownload)
	}

	// Send tasks to workers
	for _, task := range tasks {
		taskCh <- task
	}
	close(taskCh)

	// Wait for completion
	wg.Wait()
}

func downloadWorker(wg *sync.WaitGroup, taskCh <-chan downloadTask, forceDownload bool) {
	defer wg.Done()
	for task := range taskCh {
		downloadFile(task, forceDownload)
	}
}

func downloadFile(task downloadTask, forceDownload bool) {
	// Extract filename from path for logging
	filename := filepath.Base(task.DstPath)

	// If force download, remove existing file first
	if forceDownload {
		if _, err := os.Stat(task.DstPath); err == nil {
			logrus.Debugf("force download enabled, removing existing file: %s", filename)
			os.Remove(task.DstPath)
		}
	}

	logrus.Infof("downloading %s from %s", filename, task.SrcURL)

	err := utils.DownloadFile(task.SrcURL, task.DstPath)
	if err != nil {
		logrus.Errorf("failed to download %s: %v", filename, err)
	} else {
		logrus.Infof("downloaded %s to %s", filename, task.DstPath)
	}
}
