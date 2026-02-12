package repo

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

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
		module = strings.TrimSpace(module)
		if module == "" {
			continue
		}
		if _, ok := manager.Module[module]; !ok {
			data.RemovedRepos = append(data.RemovedRepos, &RemovedRepoItem{
				Module:   module,
				FilePath: "",
				Success:  false,
				Error:    fmt.Sprintf("module not found: %s", module),
			})
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
	return buildRmReposResult(data, len(modules))
}

// buildRmReposResult computes the final RmRepos result from collected operation data.
func buildRmReposResult(data *RepoRmData, moduleCount int) *output.Result {
	successCount := 0
	failCount := 0
	for _, item := range data.RemovedRepos {
		if item != nil && item.Success {
			successCount++
		} else {
			failCount++
		}
	}

	if successCount == 0 && len(data.RemovedRepos) > 0 {
		return output.Fail(output.CodeRepoRemoveFailed,
			fmt.Sprintf("failed to remove all %d modules", moduleCount)).WithData(data)
	}

	// Keep update failure as a hard failure for automation safety.
	if data.UpdateResult != nil && !data.UpdateResult.Success {
		msg := "cache update failed"
		if data.UpdateResult.Error != "" {
			msg = fmt.Sprintf("cache update failed: %s", data.UpdateResult.Error)
		}
		result := output.Fail(output.CodeRepoCacheUpdateFailed, msg).WithData(data)
		if failCount > 0 {
			result.Detail = fmt.Sprintf("failed: %d module(s)", failCount)
		}
		return result
	}

	message := fmt.Sprintf("Removed %d module(s)", successCount)
	result := output.OK(message, data)
	if failCount > 0 {
		result.Detail = fmt.Sprintf("failed: %d module(s)", failCount)
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
