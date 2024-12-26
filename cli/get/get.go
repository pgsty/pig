package get

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"pig/internal/config"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var Timeout = 1500 * time.Millisecond

const (
	ViaIO = "pigsty.io"
	ViaCC = "pigsty.cc"
	ViaNA = "nowhere"
)

var (
	ioURL          = "https://repo.pigsty.io"
	ccURL          = "https://repo.pigsty.cc"
	Source         = ViaNA
	Region         = "default"
	InternetAccess = false
	LatestVersion  = config.PigstyVersion
	AllVersions    []VersionInfo
	Details        = false
)

// VersionInfo represents version metadata including checksum and download URL
type VersionInfo struct {
	Version     string
	Checksum    string
	DownloadURL string
}

// NetworkCondition probes repository endpoints and returns the fastest responding one
func NetworkCondition() string {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	type result struct {
		source string
		data   []VersionInfo
	}
	resultChan := make(chan result, 3)

	probeEndpoint := func(ctx context.Context, baseURL, tag string) {
		url := baseURL + "/src/checksums"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			if Details {
				fmt.Printf("%-10s request error\n", tag)
			}
			resultChan <- result{"", nil}
			return
		}

		client := &http.Client{Timeout: Timeout}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			if Details {
				fmt.Printf("%-10s unreachable\n", tag)
			}
			resultChan <- result{"", nil}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			versions, err := ParseChecksums(resp.Body, tag)
			if err != nil {
				if Details {
					fmt.Printf("%-10s parse error: %v\n", tag, err)
				}
				resultChan <- result{"", nil}
				return
			}
			if Details {
				fmt.Printf("%s  ping ok: %d ms\n", tag, time.Since(start).Milliseconds())
			}
			resultChan <- result{tag, versions}
			return
		}
		resultChan <- result{"", nil}
	}

	probeGoogle := func(ctx context.Context) {
		tag := "google.com"
		url := "https://www.google.com"
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			if Details {
				fmt.Printf("%-10s request error\n", tag)
			}
			resultChan <- result{"google", nil}
			return
		}
		client := &http.Client{Timeout: Timeout}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			if Details {
				fmt.Printf("%-10s request error\n", tag)
			}
			resultChan <- result{"google", nil}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if Details {
				fmt.Printf("%-10s ping ok: %d ms\n", tag, time.Since(start).Milliseconds())
			}
			resultChan <- result{"google_ok", nil}
		} else {
			if Details {
				fmt.Printf("%-10s ping fail: %d ms\n", tag, time.Since(start).Milliseconds())
			}
			resultChan <- result{"google_fail", nil}
		}
	}

	go probeGoogle(ctx)
	go probeEndpoint(ctx, ioURL, ViaIO)
	go probeEndpoint(ctx, ccURL, ViaCC)

	var ioData, ccData []VersionInfo
	ioSuccess := false
	ccSuccess := false
	googleSuccess := false
	receivedCount := 0

	for receivedCount < 3 {
		select {
		case res := <-resultChan:
			receivedCount++
			switch res.source {
			case ViaIO:
				if len(res.data) > 0 {
					ioSuccess = true
					ioData = res.data
				}
			case ViaCC:
				if len(res.data) > 0 {
					ccSuccess = true
					ccData = res.data
				}
			case "google_ok":
				googleSuccess = true
			}
		case <-ctx.Done():
			goto DONE
		}
	}

DONE:
	// Set Source based on IO/CC availability (IO has priority)
	if ioSuccess {
		Source = ViaIO
		AllVersions = ioData
	} else if ccSuccess {
		Source = ViaCC
		AllVersions = ccData
	} else {
		Source = ViaNA
		AllVersions = nil
	}

	InternetAccess = ioSuccess || ccSuccess || googleSuccess
	if InternetAccess && (!googleSuccess) {
		Region = "china"
	}

	// Find latest stable version from AllVersions
	for _, v := range AllVersions {
		if !strings.Contains(v.Version, "-") {
			LatestVersion = v.Version
			break
		}
	}

	if Details {
		fmt.Println("Internet Access   : ", InternetAccess)
		fmt.Println("Pigsty Repo       : ", Source)
		fmt.Println("Inferred Region   : ", Region)
		fmt.Println("Latest Pigsty Ver : ", LatestVersion)
	}

	return Source
}

// GetAllVersions fetches and displays all available versions from the active repository
func GetAllVersions(print bool) error {
	baseURL := map[string]string{
		ViaIO: ioURL,
		ViaCC: ccURL,
	}[Source]

	if baseURL == "" {
		return fmt.Errorf("bad network condition")
	}

	if AllVersions == nil {
		// If not populated yet, fetch now
		versions, err := FetchChecksums(baseURL + "/src/checksums")
		if err != nil {
			return fmt.Errorf("failed to fetch versions: %v", err)
		}
		AllVersions = versions
	}

	if print {
		const format = "%-10s\t%-36s\t%s\n"
		fmt.Printf(format, "VERSION", "CHECKSUM", "DOWNLOAD URL")
		fmt.Println(strings.Repeat("-", 90))
		for _, v := range AllVersions {
			if CompareVersions(v.Version, "v3.0.0") >= 0 {
				fmt.Printf(format, v.Version, v.Checksum, v.DownloadURL)
			}
		}
	}

	return nil
}

func PirntAllVersions(since string) {
	if since == "" {
		since = "v3.0.0"
	}
	if AllVersions == nil {
		// if AllVersions is not populated, fetch it, and silently return if failed
		NetworkCondition()
		if AllVersions == nil {
			return
		}
	}
	const format = "%-10s\t%-36s\t%s\n"
	fmt.Printf(format, "VERSION", "CHECKSUM", "DOWNLOAD URL")
	fmt.Println(strings.Repeat("-", 90))
	for _, v := range AllVersions {
		if CompareVersions(v.Version, since) >= 0 {
			fmt.Printf(format, v.Version, v.Checksum, v.DownloadURL)
		}
	}
}

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
			return fmt.Errorf("failed to get current directory: %v", err)
		}
	}

	// Create target directory if not exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %v", err)
	}

	filename := fmt.Sprintf("pigsty-%s.tgz", version)
	targetPath := filepath.Join(targetDir, filename)

	// Check if file already exists
	if _, err := os.Stat(targetPath); err == nil {
		// Calculate MD5 checksum of existing file
		f, err := os.Open(targetPath)
		if err != nil {
			return fmt.Errorf("failed to open existing file: %v", err)
		}
		defer f.Close()

		h := md5.New()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("failed to calculate md5 checksum: %v", err)
		}
		existingChecksum := hex.EncodeToString(h.Sum(nil))

		if existingChecksum == verInfo.Checksum {
			logrus.Infof("File %s exists with same md5 %s, skip...", targetPath, existingChecksum)
			return nil
		}

		// Remove existing file with mismatched checksum
		logrus.Warnf("Removing existing file %s with mismatched checksum", targetPath)
		if err := os.Remove(targetPath); err != nil {
			return fmt.Errorf("failed to remove existing file: %v", err)
		}
	}

	// Start download
	resp, err := http.Get(verInfo.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create output file
	out, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
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
				return fmt.Errorf("failed to write to file: %v", err)
			}
			if _, err := h.Write(buf[:n]); err != nil {
				return fmt.Errorf("failed to update hash: %v", err)
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
			return fmt.Errorf("error during download: %v", err)
		}
	}
	fmt.Println() // New line after progress bar

	// Verify checksum
	downloadedChecksum := hex.EncodeToString(h.Sum(nil))
	if downloadedChecksum != verInfo.Checksum {
		logrus.Warnf("Removing downloaded file due to md5 checksum mismatch")
		os.Remove(targetPath)
		return fmt.Errorf("md5 checksum mismatch: expected %s, got %s", verInfo.Checksum, downloadedChecksum)
	}

	logrus.Infof("Downloaded: %s %.1f MiB %s", targetPath, sizeInMiB, downloadedChecksum)
	return nil
}

// IsValidVersion checks if a version string matches the expected format
// Format: vX.Y.Z[-{a|b|c|alpha|beta|rc}N]
func IsValidVersion(version string) *VersionInfo {
	// First check if version exists in AllVersions
	re := regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?$`)
	if !re.MatchString(version) {
		return nil
	}

	for _, v := range AllVersions {
		if v.Version == version {
			return &v
		}
	}
	return nil
}

// FetchChecksums retrieves and parses the checksums file from the specified URL
func FetchChecksums(url string) ([]VersionInfo, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch checksums: %v", err)
	}
	defer resp.Body.Close()

	return ParseChecksums(resp.Body, Source)
}

// ParseChecksums parses checksum file content into VersionInfo structs
func ParseChecksums(r io.Reader, source string) ([]VersionInfo, error) {
	var versions []VersionInfo
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}

		checksum, filename := fields[0], fields[1]
		version, err := GetVerFromName(filename)
		if err != nil {
			continue
		}

		var downloadURL string
		if source == ViaIO {
			downloadURL = fmt.Sprintf("%s/src/%s", ioURL, filename)
		} else {
			downloadURL = fmt.Sprintf("%s/src/%s", ccURL, filename)
		}
		versions = append(versions, VersionInfo{
			Version:     version,
			Checksum:    checksum,
			DownloadURL: downloadURL,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read checksums: %v", err)
	}

	// Sort versions in descending order
	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

// GetVerFromName extracts semantic version from filename
// Format: pigsty-vX.Y.Z[-{a|b|c|alpha|beta|rc}N].tgz
func GetVerFromName(filename string) (string, error) {
	re := regexp.MustCompile(`^pigsty-(v\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?)\.tgz$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return "", fmt.Errorf("invalid filename format: %s", filename)
	}
	return matches[1], nil
}

// CompareVersions compares two semantic versions
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	v1Parts := strings.Split(v1, "-")
	v2Parts := strings.Split(v2, "-")

	if cmp := compareMainVersion(v1Parts[0], v2Parts[0]); cmp != 0 {
		return cmp
	}
	return comparePreRelease(v1Parts, v2Parts)
}

// compareMainVersion compares the main version numbers (X.Y.Z)
func compareMainVersion(v1, v2 string) int {
	nums1 := strings.Split(v1, ".")
	nums2 := strings.Split(v2, ".")

	for i := 0; i < len(nums1) || i < len(nums2); i++ {
		n1, n2 := 0, 0
		if i < len(nums1) {
			n1, _ = strconv.Atoi(nums1[i])
		}
		if i < len(nums2) {
			n2, _ = strconv.Atoi(nums2[i])
		}
		if n1 != n2 {
			return n1 - n2
		}
	}
	return 0
}

// CompleteVersion will complete half-baked version string into latest match stable version
func CompleteVersion(version string) string {
	if version == "latest" {
		return LatestVersion
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	// If it's already a valid version, return as-is
	if IsValidVersion(version) != nil {
		return version
	}

	// Split version into parts
	prefix := version

	// Find highest matching stable version
	var highest string
	for _, v := range AllVersions {
		if strings.Contains(v.Version, "-") {
			continue // Skip pre-release versions
		}
		// Check if version matches our prefix
		if strings.HasPrefix(v.Version, prefix) {
			if highest == "" || CompareVersions(v.Version, highest) > 0 {
				highest = v.Version
			}
		}
	}
	if highest != "" {
		return highest
	}
	return version
}

// comparePreRelease compares pre-release versions
// Priority: release > rc/c > beta/b > alpha/a
func comparePreRelease(v1Parts, v2Parts []string) int {
	// Release versions take precedence
	switch {
	case len(v1Parts) == 1 && len(v2Parts) == 1:
		return 0
	case len(v1Parts) == 1:
		return 1
	case len(v2Parts) == 1:
		return -1
	}

	type preRelease struct {
		typ string
		num int
	}

	parse := func(s string) preRelease {
		var typ string
		var num int
		switch {
		case strings.HasPrefix(s, "alpha"):
			typ = "a"
			num, _ = strconv.Atoi(s[5:])
		case strings.HasPrefix(s, "beta"):
			typ = "b"
			num, _ = strconv.Atoi(s[4:])
		case strings.HasPrefix(s, "rc"):
			typ = "c"
			num, _ = strconv.Atoi(s[2:])
		default:
			typ = s[0:1]
			num, _ = strconv.Atoi(s[1:])
		}
		return preRelease{typ, num}
	}

	pr1 := parse(v1Parts[1])
	pr2 := parse(v2Parts[1])

	if pr1.typ != pr2.typ {
		if pr1.typ > pr2.typ {
			return 1
		}
		return -1
	}

	return pr1.num - pr2.num
}
