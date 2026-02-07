package get

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"pig/internal/config"
	"regexp"

	"github.com/sirupsen/logrus"
)

// DownloadSrc downloads the pigsty source package of specified version to target directory
func DownloadSrc(version string, targetDir string) error {
	// Get version info
	verInfo := IsValidVersion(version)
	if verInfo == nil {
		return fmt.Errorf("invalid version: %s", version)
	}

	// Use current directory if targetDir is empty
	if targetDir == "" {
		var err error
		targetDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Create target directory if not exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	filename := fmt.Sprintf("pigsty-%s.tgz", version)
	targetPath := filepath.Join(targetDir, filename)

	// Check if file already exists
	if _, err := os.Stat(targetPath); err == nil {
		// If we have checksum info, verify existing file
		if verInfo.Checksum != "" {
			f, err := os.Open(targetPath)
			if err != nil {
				return fmt.Errorf("failed to open existing file: %w", err)
			}
			defer f.Close()

			h := md5.New()
			if _, err := io.Copy(h, f); err != nil {
				return fmt.Errorf("failed to calculate checksum: %w", err)
			}
			existingChecksum := hex.EncodeToString(h.Sum(nil))

			if existingChecksum == verInfo.Checksum {
				logrus.Infof("file exists with matching checksum, skipping: %s", targetPath)
				return nil
			}

			// Remove existing file with mismatched checksum
			logrus.Warnf("removing existing file with mismatched checksum: %s", targetPath)
			if err := os.Remove(targetPath); err != nil {
				return fmt.Errorf("failed to remove existing file: %w", err)
			}
		} else {
			// No checksum available, skip if file exists
			logrus.Infof("file exists, skipping download: %s", targetPath)
			return nil
		}
	}

	// Start download
	logrus.Infof("downloading: %s", verInfo.DownloadURL)
	resp, err := http.Get(verInfo.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// Create output file
	out, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	// Setup progress tracking
	size := resp.ContentLength
	progress := 0
	lastProgress := 0
	sizeInMiB := float64(size) / 1024 / 1024

	// Create hash writer to verify checksum
	h := md5.New()
	buf := make([]byte, 32*1024)

	// Copy data with progress
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			// Write to file and hash
			if _, err := out.Write(buf[:n]); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
			if _, err := h.Write(buf[:n]); err != nil {
				return fmt.Errorf("failed to calculate checksum: %w", err)
			}

			// Update progress
			progress += n
			currentProgress := int(float64(progress) / float64(size) * 100)
			if currentProgress != lastProgress {
				fmt.Printf("\rGet %s %.1f MiB: %d%%", filename, sizeInMiB, currentProgress)
				lastProgress = currentProgress
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("download error: %w", err)
		}
	}
	fmt.Println() // New line after progress bar

	// Verify checksum if available
	downloadedChecksum := hex.EncodeToString(h.Sum(nil))
	if verInfo.Checksum != "" {
		if downloadedChecksum != verInfo.Checksum {
			logrus.Warnf("removing file due to checksum mismatch")
			os.Remove(targetPath)
			return fmt.Errorf("checksum mismatch: expected %s, got %s", verInfo.Checksum, downloadedChecksum)
		}
	}

	logrus.Infof("downloaded: %s (%.1f MiB, %s)", targetPath, sizeInMiB, downloadedChecksum)
	return nil
}

// IsValidVersion checks if a version string matches the expected format
// Format: vX.Y.Z[-{a|b|c|alpha|beta|rc}N]
// Returns a VersionInfo with download URL based on region
func IsValidVersion(version string) *VersionInfo {
	// Ensure version has 'v' prefix if it starts with a number
	if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
		version = "v" + version
	}

	// Check if version matches expected format
	re := regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?$`)
	if !re.MatchString(version) {
		return nil
	}

	// First check if version exists in AllVersions cache
	for _, v := range AllVersions {
		if v.Version == version {
			return &v
		}
	}

	// If not in cache, create VersionInfo based on region
	// Choose repository based on Source/Region (default to io)
	var baseURL string
	if Source == ViaCC || Region == "china" {
		baseURL = config.RepoPigstyCC
	} else {
		baseURL = config.RepoPigstyIO
	}

	filename := fmt.Sprintf("pigsty-%s.tgz", version)
	return &VersionInfo{
		Version:     version,
		DownloadURL: fmt.Sprintf("%s/src/%s", baseURL, filename),
		// Checksum will be empty for versions not in cache
		Checksum: "",
	}
}
