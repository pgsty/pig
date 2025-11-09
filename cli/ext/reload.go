package ext

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
)

// ReloadCatalog downloads the latest extension catalog from the fastest available source
func ReloadExtensionCatalog() error {
	urls := []string{
		config.RepoPigstyIO + "/ext/data/extension.csv",
		config.RepoPigstyCC + "/ext/data/extension.csv",
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

			logrus.Infof("get latest extension catalog from %s", url)
			resultChan <- result{content, nil}
		}(url)
	}

	select {
	case res := <-resultChan:
		if res.err != nil {
			return res.err
		}

		ec, err := NewExtensionCatalog()
		if err != nil {
			logrus.Errorf("fail to create extension catalog: %v", err)
			return err
		}
		if err := ec.Load(res.content); err != nil {
			logrus.Errorf("fail to validate extension catalog: %v", err)
			return err
		}
		extNum := len(ec.Extensions)
		logrus.Infof("validate extension catalog, got %d extensions", extNum)
		catalogPath := filepath.Join(config.ConfigDir, "extension.csv")
		if err := os.WriteFile(catalogPath, res.content, 0644); err != nil {
			logrus.Errorf("Failed to write extension catalog file: %v", err)
			return err
		}
		logrus.Infof("write latest extension catalog to %s", catalogPath)
		return nil
	case <-ctx.Done():
		logrus.Error("timeout, failed to get latest extension catalog")
		return fmt.Errorf("download timed out")
	}
}
