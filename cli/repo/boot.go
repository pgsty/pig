package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// Boot will bootstrap a local repo from offline package
func Boot(offlinePkg, targetDir string) error {
	logrus.Infof("booting repo from %s to %s", offlinePkg, targetDir)
	if targetDir == "" {
		targetDir = "/www"
	}
	if offlinePkg == "" {
		offlinePkg = "/tmp/pkg.tgz"
	}

	// check if offline package exists
	if _, err := os.Stat(offlinePkg); os.IsNotExist(err) {
		return fmt.Errorf("offline package not found: %s", offlinePkg)
	}

	// sudo mkdir -p targetDir
	if err := utils.SudoCommand([]string{"mkdir", "-p", targetDir}); err != nil {
		return fmt.Errorf("failed to create target directory: %s", err)
	}

	// extract package using tar
	if err := utils.SudoCommand([]string{"tar", "-xvzf", offlinePkg, "-C", targetDir}); err != nil {
		return fmt.Errorf("failed to extract package: %s", err)
	} else {
		logrus.Infof("repo bootstrapped from %s to %s", offlinePkg, targetDir)
	}

	// Check if targetDir/pigsty/repo_complete exists
	repoCompleteFile := filepath.Join(targetDir, "pigsty", "repo_complete")
	if _, err := os.Stat(repoCompleteFile); err == nil {
		logrus.Infof("%s found, add local repo config...", repoCompleteFile)

		if err := addLocalRepo(filepath.Join(targetDir, "pigsty")); err != nil {
			return err
		}
	}

	return nil
}

// addLocalRepo adds local repo config for yum/apt
func addLocalRepo(targetDir string) error {
	switch config.OSType {
	case config.DistroEL:
		logrus.Infof("add %s to %s", fmt.Sprintf("file://%s", targetDir), "/etc/yum.repos.d/local.repo")
		repoFile := "/etc/yum.repos.d/local.repo"
		repoContent := strings.Join([]string{
			"[pigsty-local]",
			"name=Pigsty Local Repo",
			fmt.Sprintf("baseurl=file://%s", targetDir),
			"enabled=1",
			"gpgcheck=0",
			"",
		}, "\n")
		return utils.PutFile(repoFile, []byte(repoContent))
	case config.DistroDEB:
		logrus.Infof("add %s to %s", fmt.Sprintf("file:%s ./", targetDir), "/etc/apt/sources.list.d/local.list")
		repoFile := "/etc/apt/sources.list.d/local.list"
		repoContent := fmt.Sprintf("deb [trusted=yes] file://%s ./", targetDir)
		return utils.PutFile(repoFile, []byte(repoContent))
	default:
		return fmt.Errorf("unsupported OS for adding local repo: %s", config.OSType)
	}
}
