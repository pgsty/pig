package repo

import (
	"errors"
	"fmt"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
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

	catalogPath := filepath.Join(config.ConfigDir, "repo.yml")
	syncResult, err := utils.SyncFirstValidFile(0, urls, catalogPath, 0644, validate)
	if err != nil {
		var syncErr *utils.MirrorSyncError
		if errors.As(err, &syncErr) && syncErr.Stage == utils.MirrorSyncWrite {
			result := output.Fail(output.CodeRepoReloadFailed, fmt.Sprintf("failed to write repo catalog file: %v", syncErr.Err))
			result.Data = &RepoReloadData{
				SourceURL:    syncResult.SourceURL,
				RepoCount:    len(repos),
				DurationMs:   time.Since(startTime).Milliseconds(),
				DownloadedAt: time.Now().Format(time.RFC3339),
			}
			return result
		}
		msg := "failed to download repo catalog"
		if utils.IsTimeoutError(err) {
			msg = "download timed out"
		}
		result := output.Fail(output.CodeRepoReloadFailed, msg)
		result.Detail = utils.MirrorAttemptDetail(urls, err)
		result.Data = &RepoReloadData{
			DurationMs:   time.Since(startTime).Milliseconds(),
			DownloadedAt: time.Now().Format(time.RFC3339),
		}
		return result
	}

	repoNum := len(repos)

	data := &RepoReloadData{
		SourceURL:    syncResult.SourceURL,
		RepoCount:    repoNum,
		CatalogPath:  catalogPath,
		DownloadedAt: time.Now().Format(time.RFC3339),
		DurationMs:   time.Since(startTime).Milliseconds(),
	}

	message := fmt.Sprintf("Reloaded repo catalog: %d repositories from %s", repoNum, syncResult.SourceURL)
	return output.OK(message, data)
}
