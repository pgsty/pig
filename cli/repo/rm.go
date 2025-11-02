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
	rmCmd := []string{"rm", "-f"}
	rmCmd = append(rmCmd, rmFileList...)
	logrus.Warnf("remove repo with: rm -f %s", strings.Join(rmCmd, " "))
	return utils.SudoCommand(rmCmd)
}

// BackupRepo makes a backup of the current repo files (sudo required)
func (m *Manager) BackupRepo() error {
	logrus.Infof("backup repos from %s to %s", m.RepoDir, m.BackupDir)

	// Create backup directory and move files using sudo
	if err := utils.SudoCommand([]string{"mkdir", "-p", m.BackupDir}); err != nil {
		return err
	}

	// Scan directory and collect all regular files
	entries, err := os.ReadDir(m.RepoDir)
	if err != nil {
		return fmt.Errorf("failed to read repo directory: %w", err)
	}

	var filesToMove []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(m.RepoDir, entry.Name())
		if strings.Contains(fullPath, m.BackupDir) {
			continue
		}
		filesToMove = append(filesToMove, fullPath)
	}

	// Move all files in one command if any exist
	if len(filesToMove) > 0 {
		mvCmd := []string{"mv", "-f"}
		mvCmd = append(mvCmd, filesToMove...)
		mvCmd = append(mvCmd, m.BackupDir)
		logrus.Debugf("mv -f %s... %s", filepath.Base(filesToMove[0]), m.BackupDir)
		if err := utils.SudoCommand(mvCmd); err != nil {
			return fmt.Errorf("failed to move repo files: %w", err)
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
