package repo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ReloadRepoCatalog downloads the latest repo catalog from the fastest available source
func ReloadRepoCatalog() error {
	urls := []string{
		config.RepoPigstyIO + "/ext/data/repo.yml",
		config.RepoPigstyCC + "/ext/data/repo.yml",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	type result struct {
		content []byte
		err     error
	}

	resultChan := make(chan result, len(urls))

	for _, url := range urls {
		go func(url string) {
			logrus.Debugf("Attempting to download from %s", url)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				logrus.Errorf("Failed to create request for %s: %v", url, err)
				resultChan <- result{nil, err}
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				logrus.Errorf("Failed to download from %s: %v", url, err)
				resultChan <- result{nil, err}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				err := fmt.Errorf("bad status: %s", resp.Status)
				logrus.Errorf("Failed to download from %s: %v", url, err)
				resultChan <- result{nil, err}
				return
			}

			content, err := io.ReadAll(resp.Body)
			if err != nil {
				logrus.Errorf("Failed to read response from %s: %v", url, err)
				resultChan <- result{nil, err}
				return
			}

			logrus.Infof("get latest repo catalog from %s", url)
			resultChan <- result{content, nil}
		}(url)
	}

	select {
	case res := <-resultChan:
		if res.err != nil {
			return res.err
		}

		// Validate the downloaded content
		var repos []Repository
		if err := yaml.Unmarshal(res.content, &repos); err != nil {
			logrus.Errorf("fail to validate repo catalog: %v", err)
			return err
		}
		repoNum := len(repos)
		logrus.Infof("validate repo catalog, got %d repositories", repoNum)

		// Write to local config file
		catalogPath := filepath.Join(config.ConfigDir, "repo.yml")
		if err := os.WriteFile(catalogPath, res.content, 0644); err != nil {
			logrus.Errorf("Failed to write repo catalog file: %v", err)
			return err
		}
		logrus.Infof("write latest repo catalog to %s", catalogPath)
		return nil
	case <-ctx.Done():
		logrus.Error("timeout, failed to get latest repo catalog")
		return fmt.Errorf("download timed out")
	}
}

// ReloadRepoCatalogWithResult downloads the latest repo catalog and returns a structured Result
// This function is used for YAML/JSON output modes
func ReloadRepoCatalogWithResult() *output.Result {
	startTime := time.Now()

	urls := []string{
		config.RepoPigstyIO + "/ext/data/repo.yml",
		config.RepoPigstyCC + "/ext/data/repo.yml",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	type downloadResult struct {
		content   []byte
		sourceURL string
		err       error
	}

	resultChan := make(chan downloadResult, len(urls))

	for _, url := range urls {
		go func(url string) {
			logrus.Debugf("Attempting to download from %s", url)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				logrus.Errorf("Failed to create request for %s: %v", url, err)
				resultChan <- downloadResult{nil, url, err}
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				logrus.Errorf("Failed to download from %s: %v", url, err)
				resultChan <- downloadResult{nil, url, err}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				err := fmt.Errorf("bad status: %s", resp.Status)
				logrus.Errorf("Failed to download from %s: %v", url, err)
				resultChan <- downloadResult{nil, url, err}
				return
			}

			content, err := io.ReadAll(resp.Body)
			if err != nil {
				logrus.Errorf("Failed to read response from %s: %v", url, err)
				resultChan <- downloadResult{nil, url, err}
				return
			}

			logrus.Infof("get latest repo catalog from %s", url)
			resultChan <- downloadResult{content, url, nil}
		}(url)
	}

	select {
	case res := <-resultChan:
		if res.err != nil {
			result := output.Fail(output.CodeRepoReloadFailed, res.err.Error())
			result.Data = &RepoReloadData{
				SourceURL:    res.sourceURL,
				DurationMs:   time.Since(startTime).Milliseconds(),
				DownloadedAt: time.Now().Format(time.RFC3339),
			}
			return result
		}

		// Validate the downloaded content
		var repos []Repository
		if err := yaml.Unmarshal(res.content, &repos); err != nil {
			result := output.Fail(output.CodeRepoReloadFailed, fmt.Sprintf("failed to validate repo catalog: %v", err))
			result.Data = &RepoReloadData{
				SourceURL:    res.sourceURL,
				DurationMs:   time.Since(startTime).Milliseconds(),
				DownloadedAt: time.Now().Format(time.RFC3339),
			}
			return result
		}

		repoNum := len(repos)
		catalogPath := filepath.Join(config.ConfigDir, "repo.yml")
		if err := os.WriteFile(catalogPath, res.content, 0644); err != nil {
			result := output.Fail(output.CodeRepoReloadFailed, fmt.Sprintf("failed to write repo catalog file: %v", err))
			result.Data = &RepoReloadData{
				SourceURL:    res.sourceURL,
				RepoCount:    repoNum,
				DurationMs:   time.Since(startTime).Milliseconds(),
				DownloadedAt: time.Now().Format(time.RFC3339),
			}
			return result
		}

		data := &RepoReloadData{
			SourceURL:    res.sourceURL,
			RepoCount:    repoNum,
			CatalogPath:  catalogPath,
			DownloadedAt: time.Now().Format(time.RFC3339),
			DurationMs:   time.Since(startTime).Milliseconds(),
		}

		message := fmt.Sprintf("Reloaded repo catalog: %d repositories from %s", repoNum, res.sourceURL)
		return output.OK(message, data)

	case <-ctx.Done():
		result := output.Fail(output.CodeRepoReloadFailed, "download timed out")
		result.Data = &RepoReloadData{
			DurationMs:   time.Since(startTime).Milliseconds(),
			DownloadedAt: time.Now().Format(time.RFC3339),
		}
		return result
	}
}