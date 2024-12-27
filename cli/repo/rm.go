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

// // RemoveRepo removes the Pigsty repository from the system (sudo required)
func (rm *RepoManager) RemoveRepo(modules ...string) error {
	var rmFileList []string
	for _, module := range modules {
		if module != "" {
			rmFile := rm.getModulePath(module)
			if rmFile != "" {
				rmFileList = append(rmFileList, rmFile)
			}
		}
	}
	if len(rmFileList) == 0 {
		return fmt.Errorf("no module specified")
	}
	rmCmd := []string{"rm", "-f"}
	rmCmd = append(rmCmd, rmFileList...)
	logrus.Warnf("remove repo with: rm -f %s", strings.Join(rmCmd, " "))
	return utils.SudoCommand(rmCmd)
}

// BackupRepo makes a backup of the current repo files (sudo required)
func (rm *RepoManager) BackupRepo(modules ...string) error {
	logrus.Infof("backup repos: %s to %s", rm.RepoPattern, rm.BackupDir)

	// Create backup directory and move files using sudo
	if err := utils.SudoCommand([]string{"mkdir", "-p", rm.BackupDir}); err != nil {
		return err
	}
	files, err := filepath.Glob(rm.RepoPattern)
	if err != nil {
		return err
	}
	for _, file := range files {
		dest := filepath.Join(rm.BackupDir, filepath.Base(file))
		logrus.Debugf("mv -f %s %s", file, dest)
		if err := utils.SudoCommand([]string{"mv", "-f", file, dest}); err != nil {
			logrus.Errorf("failed to mv %s to %s: %v", file, dest, err)
		}
	}

	// for debian, also backup sources.list
	if config.OSType == config.DistroDEB {
		debSourcesList := "/etc/apt/sources.list"
		if fileInfo, err := os.Stat(debSourcesList); err == nil && fileInfo.Size() > 0 {
			if err := utils.SudoCommand([]string{"mv", "-f", debSourcesList, filepath.Join(rm.BackupDir, "sources.list")}); err != nil {
				return err
			}
			if err := utils.SudoCommand([]string{"touch", debSourcesList}); err != nil {
				return err
			}
		}
	}

	return nil
}
