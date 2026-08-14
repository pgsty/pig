package get

import (
	"archive/tar"
	"compress/gzip"
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
	"strings"

	"github.com/sirupsen/logrus"
)

var pigstyVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?$`)

var downloadProgressOut io.Writer = os.Stdout

const downloadProgressWidth = 16

type progressReader struct {
	r        io.Reader
	total    int64
	filename string
	sizeMiB  float64

	readBytes   int64
	lastBytes   int64
	lastPercent int
	lastWidth   int
	printed     bool
	out         io.Writer
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n <= 0 {
		return n, err
	}

	p.readBytes += int64(n)
	if p.out == nil || config.IsStructuredOutput() {
		return n, err
	}
	if p.total > 0 {
		percent := int(float64(p.readBytes) / float64(p.total) * 100)
		percent = min(percent, 100)
		if percent != p.lastPercent || !p.printed {
			p.renderDeterminate(percent)
			p.lastPercent = percent
		}
	} else if !p.printed || p.readBytes-p.lastBytes >= 256*1024 {
		p.renderIndeterminate()
		p.lastBytes = p.readBytes
	}
	return n, err
}

func (p *progressReader) renderDeterminate(percent int) {
	filled := percent * downloadProgressWidth / 100
	bar := strings.Repeat("=", filled)
	if filled < downloadProgressWidth {
		bar += ">" + strings.Repeat(" ", downloadProgressWidth-filled-1)
	}
	readMiB := float64(p.readBytes) / 1024 / 1024
	p.render(fmt.Sprintf("download %s [%s] %3d%% %5.1f/%5.1f MiB", p.filename, bar, percent, readMiB, p.sizeMiB))
}

func (p *progressReader) renderIndeterminate() {
	position := int((p.readBytes / (256 * 1024)) % downloadProgressWidth)
	bar := strings.Repeat(" ", position) + ">" + strings.Repeat(" ", downloadProgressWidth-position-1)
	readMiB := float64(p.readBytes) / 1024 / 1024
	p.render(fmt.Sprintf("download %s [%s] %5.1f MiB", p.filename, bar, readMiB))
}

func (p *progressReader) render(line string) {
	width := len([]rune(line))
	padding := ""
	if width < p.lastWidth {
		padding = strings.Repeat(" ", p.lastWidth-width)
	}
	_, _ = io.WriteString(p.out, "\r"+line+padding)
	p.lastWidth = width
	p.printed = true
}

func (p *progressReader) finish() {
	if !p.printed || p.out == nil {
		return
	}
	_, _ = io.WriteString(p.out, "\r"+strings.Repeat(" ", p.lastWidth)+"\r")
	p.printed = false
	p.lastWidth = 0
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

func shouldSkipExistingFile(targetPath string, expectedChecksum string) (bool, error) {
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to inspect existing file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("download target is not a regular file: %s", targetPath)
	}

	if expectedChecksum != "" {
		existingChecksum, err := md5sumFile(targetPath)
		if err != nil {
			return false, fmt.Errorf("failed to checksum existing file: %w", err)
		}
		if strings.EqualFold(existingChecksum, expectedChecksum) {
			logrus.Infof("file exists with matching checksum, skipping: %s", targetPath)
			return true, nil
		}
		logrus.Warnf("existing file checksum differs; preserving it until replacement is verified: %s", targetPath)
		return false, nil
	}

	if err := validateTarGzip(targetPath); err == nil {
		logrus.Infof("complete source archive exists, skipping download: %s", targetPath)
		return true, nil
	}
	logrus.Warnf("existing source archive is incomplete or invalid; preserving it until replacement is verified: %s", targetPath)
	return false, nil
}

// validateTarGzip consumes the complete gzip stream, including its checksum,
// and requires at least one tar entry. It is the integrity fallback for
// releases whose checksum metadata is unavailable.
func validateTarGzip(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer gz.Close()

	entries := 0
	reader := tar.NewReader(gz)
	for {
		_, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid tar stream: %w", err)
		}
		entries++
	}
	if entries == 0 {
		return fmt.Errorf("empty tar archive")
	}
	// tar.Reader stops after the archive terminator. Draining gzip validates
	// the trailer checksum and catches a truncated compressed stream.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return fmt.Errorf("invalid gzip trailer: %w", err)
	}
	return nil
}

func downloadToTemp(url, targetDir, filename string) (tempPath, checksum string, sizeMiB float64, err error) {
	resp, err := utils.DefaultClient().Get(url)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.CreateTemp(targetDir, "."+filename+".part-*")
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create temporary output file: %w", err)
	}
	tempPath = out.Name()
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	size := resp.ContentLength
	sizeMiB = float64(size) / 1024 / 1024

	h := md5.New()
	reader := &progressReader{
		r:        resp.Body,
		total:    size,
		filename: filename,
		sizeMiB:  sizeMiB,
		out:      downloadProgressOut,
	}

	n, err := io.CopyBuffer(io.MultiWriter(out, h), reader, make([]byte, 32*1024))
	reader.finish()
	if err != nil {
		return "", "", sizeMiB, fmt.Errorf("download error: %w", err)
	}
	if size <= 0 {
		sizeMiB = float64(n) / 1024 / 1024
	}
	if err := out.Sync(); err != nil {
		return "", "", sizeMiB, fmt.Errorf("failed to sync temporary download: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", "", sizeMiB, fmt.Errorf("failed to close temporary download: %w", err)
	}
	committed = true
	return tempPath, hex.EncodeToString(h.Sum(nil)), sizeMiB, nil
}

func commitDownload(tempPath, targetPath string) error {
	if err := os.Chmod(tempPath, 0644); err != nil {
		return fmt.Errorf("failed to set download permissions: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("failed to commit download: %w", err)
	}
	dir, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return fmt.Errorf("failed to open download directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("failed to sync download directory: %w", err)
	}
	return nil
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

	skip, err := shouldSkipExistingFile(targetPath, verInfo.Checksum)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	// Start download
	logrus.Infof("downloading: %s", verInfo.DownloadURL)
	tempPath, downloadedChecksum, sizeMiB, err := downloadToTemp(verInfo.DownloadURL, targetDir, filename)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	// Verify checksum if available
	if verInfo.Checksum != "" {
		if !strings.EqualFold(downloadedChecksum, verInfo.Checksum) {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", verInfo.Checksum, downloadedChecksum)
		}
	}
	if err := validateTarGzip(tempPath); err != nil {
		return fmt.Errorf("downloaded source archive is invalid: %w", err)
	}
	if err := commitDownload(tempPath, targetPath); err != nil {
		return err
	}
	committed = true

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
