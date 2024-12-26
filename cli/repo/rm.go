package repo

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"
)

// RemoveRepo removes the Pigsty repository from the system
func RemoveRepo(modules ...string) error {
	var rmFileList []string
	for _, module := range modules {
		if module != "" {
			rmFileList = append(rmFileList, ModuleRepoPath(module))
		}
	}
	if len(rmFileList) == 0 {
		return fmt.Errorf("no module specified")
	}
	rmCmd := []string{"rm", "-f"}
	rmCmd = append(rmCmd, rmFileList...)
	logrus.Warnf("remove repo with: %s", strings.Join(rmCmd, " "))
	return utils.SudoCommand(rmCmd)
}

// BackupRepo makes a backup of the current repo files (sudo required)
func BackupRepo() error {
	var backupDir, repoPattern string

	if config.OSType == config.DistroEL {
		backupDir = "/etc/yum.repos.d/backup"
		repoPattern = "/etc/yum.repos.d/*.repo"
		logrus.Warn("old repos = moved to /etc/yum.repos.d/backup")
	} else if config.OSType == config.DistroDEB {
		backupDir = "/etc/apt/backup"
		repoPattern = "/etc/apt/sources.list.d/*"
		logrus.Warn("old repos = moved to /etc/apt/backup")
	}

	// Create backup directory and move files using sudo
	if err := utils.SudoCommand([]string{"mkdir", "-p", backupDir}); err != nil {
		return err
	}

	files, err := filepath.Glob(repoPattern)
	if err != nil {
		return err
	}

	for _, file := range files {
		dest := filepath.Join(backupDir, filepath.Base(file))
		logrus.Debugf("backup %s to %s", file, dest)
		if err := utils.SudoCommand([]string{"mv", "-f", file, dest}); err != nil {
			logrus.Errorf("failed to backup %s to %s: %v", file, dest, err)
		}
	}

	if config.OSType == config.DistroDEB {
		debSourcesList := "/etc/apt/sources.list"
		if fileInfo, err := os.Stat(debSourcesList); err == nil && fileInfo.Size() > 0 {
			if err := utils.SudoCommand([]string{"mv", "-f", debSourcesList, filepath.Join(backupDir, "sources.list")}); err != nil {
				return err
			}
			if err := utils.SudoCommand([]string{"touch", debSourcesList}); err != nil {
				return err
			}
		}
	}

	return nil
}
