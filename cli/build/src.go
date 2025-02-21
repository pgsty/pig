package build

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

type downloadTask struct {
	SrcURL  string
	DstPath string
}

const parallelWorkers = 8

// DownloadExtensions downloads extension source tarballs from repository
func DownloadSourceTarball(args []string) error {
	downloadAll := len(args) == 1 && args[0] == "all"
	prefix := ""
	if !downloadAll && len(args) == 1 && args[0] != "" {
		prefix = args[0]
	}

	source := get.NetworkCondition()
	var baseURL string
	switch source {
	case get.ViaIO:
		baseURL = "https://repo.pigsty.io/pkg/ext"
	case get.ViaCC:
		baseURL = "https://repo.pigsty.cc/pkg/ext"
	default:
		return fmt.Errorf("no network access, please check your network settings")
	}

	// Get package list
	packages, err := getPackageList(baseURL)
	if err != nil {
		return err
	}

	// Determine tarball directory based on OS
	tarballDir := getTarballDir()
	if err := os.MkdirAll(tarballDir, 0755); err != nil {
		return fmt.Errorf("failed to create tarball directory: %v", err)
	}

	// Create download tasks
	tasks := createDownloadTasks(baseURL, tarballDir, packages, downloadAll, prefix)
	if len(tasks) == 0 {
		logrus.Info("No packages to download")
		return nil
	}

	parallelDownload(tasks, parallelWorkers)
	return nil
}

func getPackageList(baseURL string) ([]string, error) {
	metaURL := fmt.Sprintf("%s/README.md", baseURL)
	resp, err := http.Get(metaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get packages list: %v", err)
	}
	defer resp.Body.Close()

	var packages []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		packages = append(packages, scanner.Text())
	}
	return packages, scanner.Err()
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

func createDownloadTasks(baseURL, tarballDir string, packages []string, downloadAll bool, prefix string) []downloadTask {
	var tasks []downloadTask
	skipPrefixes := []string{"pg_duckdb", "pg_mooncake", "omnigres", "plv8"}

	for _, pkg := range packages {
		if !downloadAll {
			// If prefix is specified, only download packages with that prefix
			if prefix != "" {
				if !strings.HasPrefix(pkg, prefix) {
					continue
				}
			} else {
				// Otherwise use the default skip logic
				skip := false
				for _, skipPrefix := range skipPrefixes {
					if strings.HasPrefix(pkg, skipPrefix) {
						logrus.Debugf("skipping %s due to size limit", pkg)
						skip = true
						break
					}
				}
				if skip {
					continue
				}
			}
		}

		srcURL := fmt.Sprintf("%s/%s", baseURL, pkg)
		dstPath := filepath.Join(tarballDir, pkg)
		logrus.Infof("scheduling download task %s -> %s", srcURL, dstPath)
		tasks = append(tasks, downloadTask{SrcURL: srcURL, DstPath: dstPath})
	}
	return tasks
}

// parallelDownload launches workers to download tarballs concurrently
func parallelDownload(tasks []downloadTask, workers int) {
	if len(tasks) == 0 {
		return
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

	// Wait for all downloads to complete
	wg.Wait()
	logrus.Infof("All %d downloads completed", len(tasks))
}

func downloadWorker(wg *sync.WaitGroup, taskCh <-chan downloadTask) {
	defer wg.Done()
	for task := range taskCh {
		if err := downloadFile(task); err != nil {
			logrus.Errorf("Failed to process %s: %v", task.SrcURL, err)
		}
	}
}

func downloadFile(task downloadTask) error {
	resp, err := http.Get(task.SrcURL)
	if err != nil {
		return fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	out, err := os.Create(task.DstPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}

	logrus.Infof("Downloaded %s", task.DstPath)
	return nil
}
