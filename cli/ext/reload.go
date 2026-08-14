package ext

import (
	"errors"
	"fmt"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"time"
)

// ReloadCatalogResult returns a structured Result for the ext reload command
func ReloadCatalogResult(mirror ...bool) *output.Result {
	startTime := time.Now()
	useMirror := len(mirror) > 0 && mirror[0]
	urls := extensionCatalogURLs(useMirror)

	// Success criteria: downloaded content must be a valid extension catalog.
	var extNum int
	validate := func(content []byte) error {
		ec := &ExtensionCatalog{DataPath: "downloaded"}
		if err := ec.Load(content); err != nil {
			return fmt.Errorf("failed to validate extension catalog: %w", err)
		}
		extNum = ec.ExtensionCount()
		return nil
	}

	catalogPath := filepath.Join(config.ConfigDir, "extension.csv")
	syncResult, err := utils.SyncFirstValidFile(0, urls, catalogPath, 0644, validate)
	if err != nil {
		var syncErr *utils.MirrorSyncError
		if errors.As(err, &syncErr) && syncErr.Stage == utils.MirrorSyncWrite {
			result := output.Fail(output.CodeExtensionReloadFailed, fmt.Sprintf("failed to write extension catalog file: %v", syncErr.Err))
			result.Data = &ReloadResultData{
				SourceURL:      syncResult.SourceURL,
				ExtensionCount: extNum,
				DurationMs:     time.Since(startTime).Milliseconds(),
				DownloadedAt:   time.Now().Format(time.RFC3339),
			}
			return result
		}
		msg := "failed to download extension catalog"
		if utils.IsTimeoutError(err) {
			msg = "download timed out"
		}
		result := output.Fail(output.CodeExtensionReloadFailed, msg)
		result.Detail = utils.MirrorAttemptDetail(urls, err)
		result.Data = &ReloadResultData{
			DurationMs:   time.Since(startTime).Milliseconds(),
			DownloadedAt: time.Now().Format(time.RFC3339),
		}
		return result
	}

	data := &ReloadResultData{
		SourceURL:      syncResult.SourceURL,
		ExtensionCount: extNum,
		CatalogPath:    catalogPath,
		DownloadedAt:   time.Now().Format(time.RFC3339),
		DurationMs:     time.Since(startTime).Milliseconds(),
	}

	message := fmt.Sprintf("Reloaded extension catalog: %d extensions from %s", extNum, syncResult.SourceURL)
	return output.OK(message, data)
}

func extensionCatalogURLs(mirror bool) []string {
	if mirror {
		return []string{config.RepoPigstyCC + "/ext/data/extension.csv"}
	}
	return []string{
		config.RepoPigstyIO + "/ext/data/extension.csv",
		config.RepoPigstyCC + "/ext/data/extension.csv",
	}
}
