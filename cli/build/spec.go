package build

import (
	"fmt"
	"os"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

const (
	RPM_REPO = "https://github.com/pgsty/rpm.git"
	DEB_REPO = "https://github.com/pgsty/deb.git"
)

// InitBuildEnv will install build dependencies for different distributions
func GetSpecRepo() error {
	switch config.OSType {
	case config.DistroEL:
		return SetupELBuildEnv()
	case config.DistroDEB:
		return SetupDEBBuildEnv()
	default:
		return fmt.Errorf("unsupported distribution: %s", config.OSType)
	}
}

func SetupELBuildEnv() error {
	targetDir := config.HomeDir + "/rpm"
	// check if targetDir exists, skip if exists
	if _, err := os.Stat(targetDir); err == nil {
		logrus.Infof("rpm repo already exists in %s, skip", targetDir)
		return nil
	}
	cloneCmd := []string{"git", "clone", RPM_REPO, targetDir}
	if err := utils.Command(cloneCmd); err != nil {
		return fmt.Errorf("failed to clone rpm repo: %v", err)
	}

	// run rpmdev-setuptree
	setuptreeCmd := []string{"rpmdev-setuptree"}
	if err := utils.Command(setuptreeCmd); err != nil {
		return fmt.Errorf("failed to run rpmdev-setuptree: %v", err)
	}

	// copy targetDir/rpmbuild/* to ~/rpmbuild
	rsyncCmd := []string{"rsync", "-av", targetDir + "/rpmbuild/", config.HomeDir + "/rpmbuild/"}
	if err := utils.Command(rsyncCmd); err != nil {
		return fmt.Errorf("failed to rsync rpmbuild: %v", err)
	}

	logrus.Infof("$ cd ~/rpmbuild")
	return nil
}

func SetupDEBBuildEnv() error {
	targetDir := config.HomeDir + "/deb"
	// check if targetDir exists, skip if exists
	if _, err := os.Stat(targetDir); err == nil {
		logrus.Infof("deb repo already exists in %s, skip", targetDir)
		return nil
	}
	cloneCmd := []string{"git", "clone", DEB_REPO, targetDir}
	if err := utils.Command(cloneCmd); err != nil {
		return fmt.Errorf("failed to clone deb repo: %v", err)
	}

	// mkdir ~/deb/tarball ~/deb/ /tmp/deb
	mkdirCmd := []string{"mkdir", "-p", targetDir + "/tarball", targetDir + "/", "/tmp/deb"}
	if err := utils.Command(mkdirCmd); err != nil {
		return fmt.Errorf("failed to mkdir: %v", err)
	}

	logrus.Infof("$ cd ~/deb")
	return nil
}
