package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"time"

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

// RmRepos removes repository modules and returns a structured Result
// This function is used for YAML/JSON output modes
// If modules is empty, it backs up all repositories
func RmRepos(modules []string, doUpdate bool) *output.Result {
	startTime := time.Now()

	// Check OS support
	if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
		return output.Fail(output.CodeRepoUnsupportedOS,
			fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
	}

	// Initialize manager
	manager, err := NewManager()
	if err != nil {
		return output.Fail(output.CodeRepoManagerError, fmt.Sprintf("failed to get repo manager: %v", err))
	}

	// Build OS environment info
	osEnv := &OSEnvironment{
		Code:  manager.OsDistroCode,
		Arch:  manager.OsArch,
		Type:  manager.OsType,
		Major: manager.OsMajorVersion,
	}

	// Prepare data structure
	data := &RepoRmData{
		OSEnv:            osEnv,
		RequestedModules: modules,
		RemovedRepos:     []*RemovedRepoItem{},
	}

	// If no modules specified, backup all repos
	if len(modules) == 0 {
		backupInfo, err := backupReposStructured(manager)
		if err != nil {
			data.DurationMs = time.Since(startTime).Milliseconds()
			return output.Fail(output.CodeRepoBackupFailed,
				fmt.Sprintf("failed to backup repos: %v", err)).WithData(data)
		}
		data.BackupInfo = backupInfo
		data.DurationMs = time.Since(startTime).Milliseconds()

		message := "Backed up all repositories"
		if backupInfo != nil && len(backupInfo.BackedUpFiles) > 0 {
			message = fmt.Sprintf("Backed up %d repository files to %s", len(backupInfo.BackedUpFiles), backupInfo.BackupDir)
		}
		return output.OK(message, data)
	}

	// Remove specified modules
	for _, module := range modules {
		if module == "" {
			continue
		}

		filePath := manager.getModulePath(module)
		if filePath == "" {
			data.RemovedRepos = append(data.RemovedRepos, &RemovedRepoItem{
				Module:   module,
				FilePath: "",
				Success:  false,
				Error:    "failed to determine module path",
			})
			continue
		}

		// Execute rm command
		rmCmd := []string{"rm", "-f", filePath}
		logrus.Debugf("removing repo with: %s", strings.Join(rmCmd, " "))
		err := utils.SudoCommand(rmCmd)
		if err != nil {
			data.RemovedRepos = append(data.RemovedRepos, &RemovedRepoItem{
				Module:   module,
				FilePath: filePath,
				Success:  false,
				Error:    err.Error(),
			})
		} else {
			data.RemovedRepos = append(data.RemovedRepos, &RemovedRepoItem{
				Module:   module,
				FilePath: filePath,
				Success:  true,
			})
		}
	}

	// Handle cache update if doUpdate is true
	if doUpdate {
		updateCmd := strings.Join(manager.UpdateCmd, " ")
		err := utils.SudoCommand(manager.UpdateCmd)
		if err != nil {
			data.UpdateResult = &UpdateCacheResult{
				Command: updateCmd,
				Success: false,
				Error:   err.Error(),
			}
		} else {
			data.UpdateResult = &UpdateCacheResult{
				Command: updateCmd,
				Success: true,
			}
		}
	}

	data.DurationMs = time.Since(startTime).Milliseconds()

	// Count successes and failures
	successCount := 0
	failCount := 0
	for _, item := range data.RemovedRepos {
		if item != nil && item.Success {
			successCount++
		} else {
			failCount++
		}
	}

	// Determine overall result
	if successCount == 0 && len(data.RemovedRepos) > 0 {
		return output.Fail(output.CodeRepoRemoveFailed,
			fmt.Sprintf("failed to remove all %d modules", len(modules))).WithData(data)
	}

	message := fmt.Sprintf("Removed %d module(s)", successCount)
	result := output.OK(message, data)
	if failCount > 0 {
		result.Detail = fmt.Sprintf("failed: %d module(s)", failCount)
	}
	if data.UpdateResult != nil && !data.UpdateResult.Success {
		if result.Detail != "" {
			result.Detail += "; "
		}
		result.Detail += "cache update failed"
	}
	return result
}

// UpdateCache updates the package manager cache and returns a structured Result
// This function is used for YAML/JSON output modes
func UpdateCache() *output.Result {
	startTime := time.Now()

	// Check OS support
	if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
		return output.Fail(output.CodeRepoUnsupportedOS,
			fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
	}

	// Initialize manager
	manager, err := NewManager()
	if err != nil {
		return output.Fail(output.CodeRepoManagerError, fmt.Sprintf("failed to get repo manager: %v", err))
	}

	// Build OS environment info
	osEnv := &OSEnvironment{
		Code:  manager.OsDistroCode,
		Arch:  manager.OsArch,
		Type:  manager.OsType,
		Major: manager.OsMajorVersion,
	}

	updateCmd := strings.Join(manager.UpdateCmd, " ")

	// Prepare data structure
	data := &RepoUpdateData{
		OSEnv:   osEnv,
		Command: updateCmd,
	}

	// Execute update command
	err = utils.SudoCommand(manager.UpdateCmd)
	data.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		data.Success = false
		data.Error = err.Error()
		return output.Fail(output.CodeRepoCacheUpdateFailed,
			fmt.Sprintf("failed to update cache: %v", err)).WithData(data)
	}

	data.Success = true
	return output.OK("Cache updated successfully", data)
}
