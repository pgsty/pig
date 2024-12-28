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

// RemoveRepo will remove the repo files (sudo required)
func (m *Manager) RemoveRepo(modules ...string) error {
	var rmFileList []string
	for _, module := range modules {
		if module != "" {
			rmFile := m.getModulePath(module)
			if rmFile != "" {
				rmFileList = append(rmFileList, rmFile)
			}
		}
	}
	if len(rmFileList) == 0 {
		return fmt.Errorf("no module specified")
	}
	rmCmd := []string{"m", "-f"}
	rmCmd = append(rmCmd, rmFileList...)
	logrus.Warnf("remove repo with: m -f %s", strings.Join(rmCmd, " "))
	return utils.SudoCommand(rmCmd)
}

// BackupRepo makes a backup of the current repo files (sudo required)
func (m *Manager) BackupRepo() error {
	logrus.Infof("backup repos: %s to %s", m.RepoPattern, m.BackupDir)

	// Create backup directory and move files using sudo
	if err := utils.SudoCommand([]string{"mkdir", "-p", m.BackupDir}); err != nil {
		return err
	}
	files, err := filepath.Glob(m.RepoPattern)
	if err != nil {
		return err
	}
	for _, file := range files {
		dest := filepath.Join(m.BackupDir, filepath.Base(file))
		logrus.Debugf("mv -f %s %s", file, dest)
		if err := utils.SudoCommand([]string{"mv", "-f", file, dest}); err != nil {
			logrus.Errorf("failed to mv %s to %s: %v", file, dest, err)
		}
	}

	// for debian, also backup sources.list
	if config.OSType == config.DistroDEB {
		debSourcesList := "/etc/apt/sources.list"
		if fileInfo, err := os.Stat(debSourcesList); err == nil && fileInfo.Size() > 0 {
			if err := utils.SudoCommand([]string{"mv", "-f", debSourcesList, filepath.Join(m.BackupDir, "sources.list")}); err != nil {
				return err
			}
			if err := utils.SudoCommand([]string{"touch", debSourcesList}); err != nil {
				return err
			}
		}
	}

	return nil
}
