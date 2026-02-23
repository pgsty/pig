package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ReloadRepoCatalogWithResult downloads the latest repo catalog and returns a structured Result
// This function is used for YAML/JSON output modes
func ReloadRepoCatalogWithResult() *output.Result {
	startTime := time.Now()

	urls := []string{
		config.RepoPigstyIO + "/ext/data/repo.yml",
		config.RepoPigstyCC + "/ext/data/repo.yml",
	}

	// Success criteria: downloaded content must be a valid repo catalog.
	var repos []Repository
	validate := func(content []byte) error {
		var tmp []Repository
		if err := yaml.Unmarshal(content, &tmp); err != nil {
			return fmt.Errorf("failed to validate repo catalog: %w", err)
		}
		repos = tmp
		return nil
	}

	content, sourceURL, err := utils.FetchFirstValidWithTimeout(0, urls, validate)
	if err != nil {
		msg := "failed to download repo catalog"
		var ne interface{ Timeout() bool }
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			msg = "download timed out"
		}
		result := output.Fail(output.CodeRepoReloadFailed, msg)
		result.Detail = fmt.Sprintf("attempted:\n- %s\n- %s\nerrors:\n%s", urls[0], urls[1], strings.TrimSpace(err.Error()))
		result.Data = &RepoReloadData{
			DurationMs:   time.Since(startTime).Milliseconds(),
			DownloadedAt: time.Now().Format(time.RFC3339),
		}
		return result
	}

	repoNum := len(repos)
	catalogPath := filepath.Join(config.ConfigDir, "repo.yml")
	if err := os.WriteFile(catalogPath, content, 0644); err != nil {
		result := output.Fail(output.CodeRepoReloadFailed, fmt.Sprintf("failed to write repo catalog file: %v", err))
		result.Data = &RepoReloadData{
			SourceURL:    sourceURL,
			RepoCount:    repoNum,
			DurationMs:   time.Since(startTime).Milliseconds(),
			DownloadedAt: time.Now().Format(time.RFC3339),
		}
		return result
	}

	data := &RepoReloadData{
		SourceURL:    sourceURL,
		RepoCount:    repoNum,
		CatalogPath:  catalogPath,
		DownloadedAt: time.Now().Format(time.RFC3339),
		DurationMs:   time.Since(startTime).Milliseconds(),
	}

	message := fmt.Sprintf("Reloaded repo catalog: %d repositories from %s", repoNum, sourceURL)
	return output.OK(message, data)
}
