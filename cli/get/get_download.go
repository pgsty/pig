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
	"pig/internal/utils"
	"regexp"

	"github.com/sirupsen/logrus"
)

var pigstyVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?$`)

type progressReader struct {
	r        io.Reader
	total    int64
	filename string
	sizeMiB  float64

	readBytes   int64
	lastPercent int
	printed     bool
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n <= 0 {
		return n, err
	}

	p.readBytes += int64(n)
	if p.total > 0 {
		percent := int(float64(p.readBytes) / float64(p.total) * 100)
		if percent != p.lastPercent {
			fmt.Printf("\rGet %s %.1f MiB: %d%%", p.filename, p.sizeMiB, percent)
			p.lastPercent = percent
			p.printed = true
		}
	}
	return n, err
}

func ensureTargetDir(targetDir string) (string, error) {
	if targetDir == "" {
		var err error
		targetDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}
	return targetDir, nil
}

func md5sumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func shouldSkipOrRemoveExistingFile(targetPath string, expectedChecksum string) (skip bool, err error) {
	if _, err := os.Stat(targetPath); err != nil {
		return false, nil
	}

	if expectedChecksum == "" {
		logrus.Infof("file exists, skipping download: %s", targetPath)
		return true, nil
	}

	existingChecksum, err := md5sumFile(targetPath)
	if err != nil {
		return false, fmt.Errorf("failed to open existing file: %w", err)
	}

	if existingChecksum == expectedChecksum {
		logrus.Infof("file exists with matching checksum, skipping: %s", targetPath)
		return true, nil
	}

	logrus.Warnf("removing existing file with mismatched checksum: %s", targetPath)
	if err := os.Remove(targetPath); err != nil {
		return false, fmt.Errorf("failed to remove existing file: %w", err)
	}
	return false, nil
}

func downloadWithMD5(url, targetPath, filename string) (checksum string, sizeMiB float64, err error) {
	resp, err := utils.DefaultClient().Get(url)
	if err != nil {
		return "", 0, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	size := resp.ContentLength
	sizeMiB = float64(size) / 1024 / 1024

	h := md5.New()
	reader := &progressReader{
		r:        resp.Body,
		total:    size,
		filename: filename,
		sizeMiB:  sizeMiB,
	}

	n, err := io.CopyBuffer(io.MultiWriter(out, h), reader, make([]byte, 32*1024))
	if reader.printed {
		fmt.Println() // New line after progress bar.
	}
	if err != nil {
		return "", sizeMiB, fmt.Errorf("download error: %w", err)
	}
	if size <= 0 {
		sizeMiB = float64(n) / 1024 / 1024
	}
	return hex.EncodeToString(h.Sum(nil)), sizeMiB, nil
}

// DownloadSrc downloads the pigsty source package of specified version to target directory
func DownloadSrc(version string, targetDir string) error {
	// Get version info
	verInfo := IsValidVersion(version)
	if verInfo == nil {
		return fmt.Errorf("invalid version: %s", version)
	}
	version = verInfo.Version

	var err error
	targetDir, err = ensureTargetDir(targetDir)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("pigsty-%s.tgz", version)
	targetPath := filepath.Join(targetDir, filename)

	skip, err := shouldSkipOrRemoveExistingFile(targetPath, verInfo.Checksum)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	// Start download
	logrus.Infof("downloading: %s", verInfo.DownloadURL)
	downloadedChecksum, sizeMiB, err := downloadWithMD5(verInfo.DownloadURL, targetPath, filename)
	if err != nil {
		return err
	}

	// Verify checksum if available
	if verInfo.Checksum != "" {
		if downloadedChecksum != verInfo.Checksum {
			logrus.Warnf("removing file due to checksum mismatch")
			_ = os.Remove(targetPath)
			return fmt.Errorf("checksum mismatch: expected %s, got %s", verInfo.Checksum, downloadedChecksum)
		}
	}

	logrus.Infof("downloaded: %s (%.1f MiB, %s)", targetPath, sizeMiB, downloadedChecksum)
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
	if !pigstyVersionRe.MatchString(version) {
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
