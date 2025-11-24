package repo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"pig/internal/config"
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