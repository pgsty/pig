package get

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"pig/internal/config"
	"pig/internal/utils"
	"regexp"

	"github.com/sirupsen/logrus"
)

func UpdatePig(pigVer, region string) error {
	var err error
	if region == "" {
		NetworkCondition()
		logrus.Debugf("region is set to: %s", Region)
		region = Region
	}
	baseURL := "https://repo.pigsty.io"
	if region == "china" {
		baseURL = "https://repo.pigsty.cc"
	}

	// Fetch the latest version if not specified
	if pigVer == "" {
		pigVer, err = getLatestPigVersion(region)
		if err != nil || !ValidVersion(pigVer) {
			return fmt.Errorf("failed to fetch latest version: %v", err)
		}
		logrus.Infof("get latest pig version: %s", pigVer)
	} else {
		if !ValidVersion(pigVer) {
			return fmt.Errorf("invalid pig version given: %s", pigVer)
		}
		logrus.Infof("update pig to desired version %s", pigVer)
	}

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
		logrus.Debugf("osarch=%s, pigVer=%s", osarch, pigVer)
		filename = fmt.Sprintf("pig-%s-1.%s.rpm", pigVer, osarch)
		packageURL = fmt.Sprintf("%s/pkg/pig/v%s/%s", baseURL, pigVer, filename)
	case config.DistroDEB:
		logrus.Debugf("osarch=%s, pigVer=%s", config.OSArch, pigVer)
		filename = fmt.Sprintf("pig_%s_%s.deb", pigVer, config.OSArch)
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
	if err := downloadFile(packageURL, downloadTo); err != nil {
		return fmt.Errorf("failed to download package: %v", err)
	}

	logrus.Infof("pig %s package downloaded to %s", config.OSType, downloadTo)

	// run sudo shell command to remove current package and install the new one
	switch config.OSType {
	case config.DistroEL:
		if err := utils.SudoCommand([]string{"yum", "remove", "-y", "pig"}); err != nil {
			logrus.Warnf("failed to remove current package: %v", err)
		}
		return utils.SudoCommand([]string{"rpm", "-i", downloadTo})
	case config.DistroDEB:
		if err := utils.SudoCommand([]string{"apt", "remove", "-y", "pig"}); err != nil {
			logrus.Warnf("failed to remove current package: %v", err)
		}
		return utils.SudoCommand([]string{"dpkg", "-i", downloadTo})

	}
	return nil
}

func getLatestPigVersion(region string) (string, error) {
	latestURL := "https://repo.pigsty.io/pkg/pig/latest"
	if region == "china" {
		latestURL = "https://repo.pigsty.cc/pkg/pig/latest"
	}
	resp, err := http.Get(latestURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version: %v", err)
	}
	return string(body), nil
}

func downloadFile(srcURL, destPath string) error {
	// Start download
	resp, err := http.Get(srcURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create output file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer out.Close()

	// write to output file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to output file: %v", err)
	}

	return nil
}

func ValidVersion(version string) bool {
	re := regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?$`)
	return re.MatchString(version)
}

func PrintInstallMethod() {
	if Region == "china" {
		fmt.Printf("\nInstall the latest pig (china mirror)\nncurl -fsSL https://repo.pigsty.cc/get | bash\n\n")
	} else {
		fmt.Printf("\nInstall the latest pig\ncurl -fsSL https://repo.pigsty.io/get | bash\n\n")
	}
}
