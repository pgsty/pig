package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// AddModule handles adding a single module to the system
func (m *Manager) AddModule(module string) error {
	modulePath := m.getModulePath(module)
	if modulePath == "" {
		return fmt.Errorf("failed to determine module path: %s", module)
	}
	moduleContent := m.getModuleContent(module)
	if err := utils.PutFile(modulePath, []byte(moduleContent)); err != nil {
		return fmt.Errorf("failed to write module file: %w", err)
	}
	return nil
}

// getModulePath returns the path to the repository configuration file for a given module
func (m *Manager) getModulePath(module string) string {
	switch config.OSType {
	case config.DistroEL:
		return filepath.Join(m.RepoDir, fmt.Sprintf("%s.repo", module))
	case config.DistroDEB:
		return filepath.Join(m.RepoDir, fmt.Sprintf("%s.list", module))
	default:
		return ""
	}
}

// getModuleContent returns the multiple repo content together
func (m *Manager) getModuleContent(module string) string {
	var moduleContent string
	if module, ok := m.Module[module]; ok {
		for _, repoName := range module {
			if repo, ok := m.Map[repoName]; ok {
				if repo.Available(config.OSCode, config.OSArch) {
					logrus.Debugf("repo %s is available for %s.%s: %v", repoName, config.OSCode, config.OSArch, repo)
					moduleContent += repo.Content(m.Region) + "\n"
				} else {
					logrus.Debugf("repo %s is not available for %s.%s: %v", repoName, config.OSCode, config.OSArch, repo)
				}
			}
		}
	}
	return moduleContent
}

// normalizeModules normalizes the module list, deduplicates and sorts
func (m *Manager) normalizeModules(modules ...string) []string {
	// if "all" in modules, replace it with node, pgsql
	if slices.Contains(modules, "all") {
		modules = append(modules, "node", "pgsql", "infra")
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "all"
		})
	}
	// if "pgsql" in modules, remove "pgdg", since pgdg is a subset of pgsql
	if slices.Contains(modules, "pgsql") {
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "pgdg"
		})
	}
	modules = slices.Compact(modules)
	slices.Sort(modules)
	return modules
}

// ExpandModuleArgs will split the input arguments by comma if necessary
func ExpandModuleArgs(args []string) []string {
	var newArgs []string
	for _, arg := range args {
		if strings.Contains(arg, ",") {
			newArgs = append(newArgs, strings.Split(arg, ",")...)
		} else {
			newArgs = append(newArgs, arg)
		}
	}
	return newArgs
}

// AddRepos adds repositories and returns a structured Result
// This function is used for YAML/JSON output modes
func AddRepos(modules []string, region string, doRemove, doUpdate bool) *output.Result {
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

	// Expand and normalize modules
	expandedModules := manager.normalizeModules(modules...)

	// Prepare data structure
	data := &RepoAddData{
		OSEnv:            osEnv,
		Region:           region,
		RequestedModules: modules,
		ExpandedModules:  expandedModules,
		AddedRepos:       []*AddedRepoItem{},
		Failed:           []*FailedRepoItem{},
	}

	// Validate modules exist
	for _, module := range expandedModules {
		if _, ok := manager.Module[module]; !ok {
			data.Failed = append(data.Failed, &FailedRepoItem{
				Module: module,
				Error:  fmt.Sprintf("module not found: %s", module),
				Code:   output.CodeRepoModuleNotFound,
			})
		}
	}

	// If all modules are invalid, return failure
	if len(data.Failed) == len(expandedModules) {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoModuleNotFound,
			fmt.Sprintf("all modules not found: %v", expandedModules)).WithData(data)
	}

	// Handle backup if doRemove is true
	if doRemove {
		backupInfo, err := backupReposStructured(manager)
		if err != nil {
			data.DurationMs = time.Since(startTime).Milliseconds()
			return output.Fail(output.CodeRepoBackupFailed,
				fmt.Sprintf("failed to backup repos: %v", err)).WithData(data)
		}
		data.BackupInfo = backupInfo
	}

	// Detect region
	manager.DetectRegion(region)
	data.Region = manager.Region

	// Add each module
	for _, module := range expandedModules {
		// Skip already failed modules
		alreadyFailed := false
		for _, f := range data.Failed {
			if f.Module == module {
				alreadyFailed = true
				break
			}
		}
		if alreadyFailed {
			continue
		}

		// Get module info before adding
		repoNames := manager.Module[module]
		filePath := manager.getModulePath(module)

		if err := manager.AddModule(module); err != nil {
			data.Failed = append(data.Failed, &FailedRepoItem{
				Module: module,
				Error:  err.Error(),
				Code:   output.CodeRepoAddFailed,
			})
		} else {
			data.AddedRepos = append(data.AddedRepos, &AddedRepoItem{
				Module:   module,
				FilePath: filePath,
				Repos:    repoNames,
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
	return buildAddReposResult(data, expandedModules)
}

// buildAddReposResult computes the final AddRepos result from collected operation data.
func buildAddReposResult(data *RepoAddData, expandedModules []string) *output.Result {
	if len(data.AddedRepos) == 0 {
		return output.Fail(output.CodeRepoAddFailed,
			fmt.Sprintf("failed to add all %d modules", len(expandedModules))).WithData(data)
	}

	// Keep update failure as a hard failure for automation safety.
	if data.UpdateResult != nil && !data.UpdateResult.Success {
		msg := "cache update failed"
		if data.UpdateResult.Error != "" {
			msg = fmt.Sprintf("cache update failed: %s", data.UpdateResult.Error)
		}
		result := output.Fail(output.CodeRepoCacheUpdateFailed, msg).WithData(data)
		if len(data.Failed) > 0 {
			result.Detail = fmt.Sprintf("failed: %d modules", len(data.Failed))
		}
		return result
	}

	message := fmt.Sprintf("Added %d modules (%d repos)", len(data.AddedRepos), countRepos(data.AddedRepos))
	result := output.OK(message, data)
	if len(data.Failed) > 0 {
		result.Detail = fmt.Sprintf("failed: %d modules", len(data.Failed))
	}
	return result
}

// backupReposStructured performs backup and returns structured info
func backupReposStructured(manager *Manager) (*BackupInfo, error) {
	logrus.Infof("backup repos from %s to %s", manager.RepoDir, manager.BackupDir)

	// Create backup directory
	if err := utils.SudoCommand([]string{"mkdir", "-p", manager.BackupDir}); err != nil {
		return nil, err
	}

	// Scan directory and collect all regular files
	entries, err := os.ReadDir(manager.RepoDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read repo directory: %w", err)
	}

	var filesToMove []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(manager.RepoDir, entry.Name())
		if strings.Contains(fullPath, manager.BackupDir) {
			continue
		}
		filesToMove = append(filesToMove, fullPath)
	}

	backupInfo := &BackupInfo{
		BackupDir:     manager.BackupDir,
		BackedUpFiles: filesToMove,
	}

	// Move all files in one command if any exist
	if len(filesToMove) > 0 {
		mvCmd := []string{"mv", "-f"}
		mvCmd = append(mvCmd, filesToMove...)
		mvCmd = append(mvCmd, manager.BackupDir)
		logrus.Debugf("mv -f %s... %s", filepath.Base(filesToMove[0]), manager.BackupDir)
		if err := utils.SudoCommand(mvCmd); err != nil {
			return nil, fmt.Errorf("failed to move repo files: %w", err)
		}
	}

	// For debian, also backup sources.list
	if config.OSType == config.DistroDEB {
		debSourcesList := "/etc/apt/sources.list"
		if fileInfo, err := os.Stat(debSourcesList); err == nil && fileInfo.Size() > 0 {
			if err := utils.SudoCommand([]string{"mv", "-f", debSourcesList, filepath.Join(manager.BackupDir, "sources.list")}); err != nil {
				return nil, err
			}
			if err := utils.SudoCommand([]string{"touch", debSourcesList}); err != nil {
				return nil, err
			}
			backupInfo.BackedUpFiles = append(backupInfo.BackedUpFiles, debSourcesList)
		}
	}

	return backupInfo, nil
}

// countRepos counts total number of repos in AddedRepoItems
func countRepos(items []*AddedRepoItem) int {
	count := 0
	for _, item := range items {
		if item != nil {
			count += len(item.Repos)
		}
	}
	return count
}
