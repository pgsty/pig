package get

import (
	"bufio"
	"crypto/sha256"
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

// UpdatePig will self-upgrade pig itself
func UpdatePig(pigVer, region string) error {
	var err error
	if region == "" {
		NetworkCondition()
		logrus.Debugf("region is set to: %s", Region)
		region = Region
	}
	baseURL := config.RepoPigstyIO
	if region == "china" {
		baseURL = config.RepoPigstyCC
	}

	// Fetch the latest version if not specified
	if pigVer == "" {
		pigVer, err = getLatestPigVersion(region)
		if err != nil {
			return fmt.Errorf("failed to fetch latest version: %w", err)
		}
		logrus.Infof("get latest pig version: %s", pigVer)
	} else {
		if !ValidVersion(pigVer) {
			return fmt.Errorf("invalid pig version given: %s", pigVer)
		}
		logrus.Infof("update pig to desired version %s", pigVer)
	}
	pigVer = strings.TrimPrefix(pigVer, "v")

	if pigVer == config.PigVersion {
		logrus.Infof("pig %s already installed, reinstall", pigVer)
	} else {
		logrus.Infof("install pig %s", pigVer)
	}

	// Construct the package URL
	var filename, packageURL, downloadTo string
	switch config.OSType {
	case config.DistroEL:
		osarch := config.OSArch
		switch osarch {
		case "amd64", "x86_64":
			osarch = "x86_64"
		case "arm64", "aarch64":
			osarch = "aarch64"
		default:
			return fmt.Errorf("unsupported arch: %s on %s %s", config.OSArch, config.OSType, config.OSCode)
		}
		logrus.Debugf("osarch=%s, pig=%s", osarch, pigVer)
		filename = fmt.Sprintf("pig-%s-1.%s.rpm", pigVer, osarch)
		packageURL = fmt.Sprintf("%s/pkg/pig/v%s/%s", baseURL, pigVer, filename)
	case config.DistroDEB:
		logrus.Debugf("osarch=%s, pig=%s", config.OSArch, pigVer)
		filename = fmt.Sprintf("pig_%s-1_%s.deb", pigVer, config.OSArch)
		packageURL = fmt.Sprintf("%s/pkg/pig/v%s/%s", baseURL, pigVer, filename)
	case config.DistroMAC:
		PrintInstallMethod()
		return fmt.Errorf("macos is not supported yet")
	}
	downloadTo = fmt.Sprintf("/tmp/%s", filename)

	logrus.Infof("wipe destination file %s", downloadTo)
	if err := utils.DelFile(downloadTo); err != nil {
		return fmt.Errorf("failed to wipe destination file: %v", err)
	}

	logrus.Infof("downloading pig %s package from %s to %s", config.OSType, packageURL, downloadTo)
	if err := utils.DownloadFile(packageURL, downloadTo); err != nil {
		return fmt.Errorf("failed to download package: %v", err)
	}
	checksumsURLs := []string{
		fmt.Sprintf("%s/pkg/pig/v%s/checksums.txt", baseURL, pigVer),
		fmt.Sprintf("https://github.com/pgsty/pig/releases/download/v%s/checksums.txt", pigVer),
	}
	if err := verifyReleaseChecksum(checksumsURLs, filename, downloadTo); err != nil {
		return fmt.Errorf("failed to verify package checksum: %w", err)
	}
	logrus.Infof("pig %s package downloaded to %s", config.OSType, downloadTo)

	installCommand, err := utils.LocalPackageUpgradeCommand(config.OSType, downloadTo)
	if err != nil {
		return err
	}
	return utils.SudoCommand(installCommand)
}

const maxReleaseChecksumsBytes = 1 << 20

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func verifyReleaseChecksum(checksumsURLs []string, filename, packagePath string) error {
	var failures []string
	for _, checksumsURL := range checksumsURLs {
		expected, err := fetchReleaseChecksum(checksumsURL, filename)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", checksumsURL, err))
			continue
		}
		return verifyFileSHA256(packagePath, filename, expected)
	}
	return fmt.Errorf("no usable checksum source for %s: %s", filename, strings.Join(failures, "; "))
}

func fetchReleaseChecksum(checksumsURL, filename string) (string, error) {
	resp, err := utils.DefaultClient().Get(checksumsURL)
	if err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksums: unexpected HTTP status %s", resp.Status)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseChecksumsBytes+1))
	if err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}
	if len(content) > maxReleaseChecksumsBytes {
		return "", fmt.Errorf("checksums file exceeds %d bytes", maxReleaseChecksumsBytes)
	}

	expected := ""
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		candidate := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(candidate) == filename {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("parse checksums: %w", err)
	}
	if !sha256Pattern.MatchString(expected) {
		return "", fmt.Errorf("no valid SHA-256 checksum found for %s", filename)
	}
	return expected, nil
}

func verifyFileSHA256(packagePath, filename, expected string) error {
	f, err := os.Open(packagePath)
	if err != nil {
		return fmt.Errorf("open package: %w", err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("hash package: %w", err)
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA-256 mismatch for %s: got %s, expected %s", filename, actual, expected)
	}
	return nil
}

func getLatestPigVersion(region string) (string, error) {
	latestURL := config.RepoPigstyIO + "/pkg/pig/latest"
	if region == "china" {
		latestURL = config.RepoPigstyCC + "/pkg/pig/latest"
	}
	return fetchLatestPigVersion(latestURL)
}

func fetchLatestPigVersion(latestURL string) (string, error) {
	resp, err := utils.DefaultClient().Get(latestURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read latest version: %w", err)
	}
	version := strings.TrimSpace(string(body))
	if !ValidVersion(version) {
		return "", fmt.Errorf("invalid version response: %q", version)
	}
	return version, nil
}

func ValidVersion(version string) bool {
	re := regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?$`)
	return re.MatchString(version)
}

func PrintInstallMethod() {
	if Region == "china" {
		fmt.Printf("\nInstall the latest pig (china mirror)\ncurl -fsSL %s/get | bash\n\n", config.RepoPigstyCC)
	} else {
		fmt.Printf("\nInstall the latest pig\ncurl -fsSL %s/get | bash\n\n", config.RepoPigstyIO)
	}
}
