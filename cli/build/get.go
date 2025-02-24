package build

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"pig/cli/get"
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
		// Default to 'std' behavior when no arguments provided
		args = []string{"std"}
	}

	// Handle special cases
	if len(args) == 1 {
		switch args[0] {
		case "all":
			// Download all packages
			return downloadWithPrefixes([]string{}, true)
		case "std":
			// Download standard packages (excluding large ones)
			return downloadWithPrefixes([]string{}, false)
		}
	}

	// Handle multiple prefixes
	return downloadWithPrefixes(args, false)
}

func downloadWithPrefixes(prefixes []string, downloadAll bool) error {
	source := get.NetworkCondition()
	var baseURL string
	switch source {
	case get.ViaIO:
		baseURL = config.RepoPigstyIO + "/pkg/ext"
	case get.ViaCC:
		baseURL = config.RepoPigstyCC + "/pkg/ext"
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
	tasks := createDownloadTasks(baseURL, tarballDir, packages, downloadAll, prefixes)
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

func createDownloadTasks(baseURL, tarballDir string, packages []string, downloadAll bool, prefixes []string) []downloadTask {
	var tasks []downloadTask
	skipPrefixes := []string{"pg_duckdb", "pg_mooncake", "omnigres", "plv8"}

	for _, pkg := range packages {
		if !downloadAll {
			if len(prefixes) > 0 {
				// Check if package matches any of the provided prefixes
				matched := false
				for _, prefix := range prefixes {
					if strings.HasPrefix(pkg, prefix) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			} else {
				// Use the default skip logic when no prefixes specified (std mode)
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
		logrus.Debugf("scheduling download task %s -> %s", srcURL, dstPath)
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
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go downloadWorker(&wg, taskCh)
	}

	// Send tasks to workers and wait for completion
	for _, task := range tasks {
		taskCh <- task
	}
	close(taskCh)
	wg.Wait()
	logrus.Infof("All %d downloads completed", len(tasks))
}

func downloadWorker(wg *sync.WaitGroup, taskCh <-chan downloadTask) {
	defer wg.Done()
	for task := range taskCh {
		_ = downloadFile(task)
	}
}

func downloadFile(task downloadTask) error {
	err := utils.DownloadFile(task.SrcURL, task.DstPath)
	if err != nil {
		logrus.Errorf("fail to download %s: %v", task.SrcURL, err)
	} else {
		logrus.Infof("fetch %s", task.DstPath)
	}
	return err
}
