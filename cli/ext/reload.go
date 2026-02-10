package ext

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
)

// ReloadCatalogResult returns a structured Result for the ext reload command
func ReloadCatalogResult() *output.Result {
	startTime := time.Now()

	urls := []string{
		config.RepoPigstyIO + "/ext/data/extension.csv",
		config.RepoPigstyCC + "/ext/data/extension.csv",
	}

	ctx := context.Background()

	// Success criteria: downloaded content must be a valid extension catalog.
	var extNum int
	validate := func(content []byte) error {
		ec := &ExtensionCatalog{DataPath: "downloaded"}
		if err := ec.Load(content); err != nil {
			return fmt.Errorf("failed to validate extension catalog: %w", err)
		}
		extNum = len(ec.Extensions)
		return nil
	}

	content, sourceURL, err := utils.FetchFirstValid(ctx, urls, validate)
	if err != nil {
		msg := "failed to download extension catalog"
		var ne interface{ Timeout() bool }
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			msg = "download timed out"
		}
		result := output.Fail(output.CodeExtensionReloadFailed, msg)
		result.Detail = fmt.Sprintf("attempted:\n- %s\n- %s\nerrors:\n%s", urls[0], urls[1], strings.TrimSpace(err.Error()))
		result.Data = &ReloadResultData{
			DurationMs:   time.Since(startTime).Milliseconds(),
			DownloadedAt: time.Now().Format(time.RFC3339),
		}
		return result
	}

	catalogPath := filepath.Join(config.ConfigDir, "extension.csv")
	if err := os.WriteFile(catalogPath, content, 0644); err != nil {
		result := output.Fail(output.CodeExtensionReloadFailed, fmt.Sprintf("failed to write extension catalog file: %v", err))
		result.Data = &ReloadResultData{
			SourceURL:      sourceURL,
			ExtensionCount: extNum,
			DurationMs:     time.Since(startTime).Milliseconds(),
			DownloadedAt:   time.Now().Format(time.RFC3339),
		}
		return result
	}

	data := &ReloadResultData{
		SourceURL:      sourceURL,
		ExtensionCount: extNum,
		CatalogPath:    catalogPath,
		DownloadedAt:   time.Now().Format(time.RFC3339),
		DurationMs:     time.Since(startTime).Milliseconds(),
	}

	message := fmt.Sprintf("Reloaded extension catalog: %d extensions from %s", extNum, sourceURL)
	return output.OK(message, data)
}
