package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var moduleNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var (
	newAddRepoManager        = NewManager
	runAddRepoCommand        = utils.SudoCommand
	runAddRepoCommandSummary = func(args []string) error { return utils.SudoCommandSummary(args, 4) }
	addRepoModule            = func(manager *Manager, module string) error { return manager.AddModule(module) }
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
	module = strings.TrimSpace(module)
	if !isSafeModuleName(module) {
		logrus.Warnf("unsafe module name rejected: %q", module)
		return ""
	}

	suffix := ""
	switch config.OSType {
	case config.DistroEL:
		suffix = ".repo"
	case config.DistroDEB:
		suffix = ".list"
	default:
		return ""
	}

	targetPath := filepath.Join(m.RepoDir, module+suffix)
	if !isPathWithinBase(m.RepoDir, targetPath) {
		logrus.Warnf("refusing path outside repo directory: %q", targetPath)
		return ""
	}
	return targetPath
}

func isSafeModuleName(module string) bool {
	if module == "" {
		return false
	}
	if strings.Contains(module, "..") || strings.Contains(module, "/") || strings.Contains(module, `\`) {
		return false
	}
	return moduleNamePattern.MatchString(module)
}

func isPathWithinBase(basePath, targetPath string) bool {
	within, err := utils.IsPathWithin(basePath, targetPath)
	return err == nil && within
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
	// Compact only removes adjacent duplicates, so sort first.
	slices.Sort(modules)
	modules = slices.Compact(modules)
	return modules
}

// ExpandModuleArgs splits comma-separated operands and rejects empty entries.
func ExpandModuleArgs(args []string) ([]string, error) {
	var newArgs []string
	for _, arg := range args {
		for _, part := range strings.Split(arg, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				return nil, fmt.Errorf("repository module list contains an empty name")
			}
			newArgs = append(newArgs, trimmed)
		}
	}
	return newArgs, nil
}

type AddOptions struct {
	Region   string
	Remove   bool
	Update   bool
	Mirror   bool
	GPGCheck bool
	Quiet    bool
}

// ConfigureRepos is the shared entry point for CLI workflows that add or set
// repository modules. It owns the default module and comma-expansion rules so
// callers do not need to duplicate command-layer argument normalization.
func ConfigureRepos(args []string, opts AddOptions) *output.Result {
	if len(args) == 0 {
		args = []string{"all"}
	}
	return AddReposWithOptions(args, opts)
}

func AddReposWithOptions(modules []string, opts AddOptions) *output.Result {
	startTime := time.Now()
	modules, err := ExpandModuleArgs(modules)
	if err != nil {
		return output.Fail(output.CodeRepoInvalidArgs, err.Error())
	}

	// Check OS support
	if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
		return output.Fail(output.CodeRepoUnsupportedOS,
			fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
	}

	// Initialize manager
	manager, err := newAddRepoManager()
	if err != nil {
		return output.Fail(output.CodeRepoManagerError, fmt.Sprintf("failed to get repo manager: %v", err))
	}
	if len(manager.List) == 0 {
		return output.Fail(output.CodeRepoUnsupportedOS,
			fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
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
		Region:           opts.Region,
		Mirror:           opts.Mirror,
		RequestedModules: modules,
		ExpandedModules:  expandedModules,
		AddedRepos:       []*AddedRepoItem{},
		Failed:           []*FailedRepoItem{},
	}

	data.Failed = missingRepoModules(manager, expandedModules)

	// If all modules are invalid, return failure
	if len(data.Failed) == len(expandedModules) {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoModuleNotFound,
			fmt.Sprintf("all modules not found: %v", expandedModules)).WithData(data)
	}
	// Replacement is all-or-nothing. Reject a mixed valid/invalid request before
	// moving any active definitions; additive mode keeps its historical partial
	// success behavior.
	if opts.Remove && len(data.Failed) > 0 {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoModuleNotFound,
			fmt.Sprintf("repository replacement contains %d unknown module(s)", len(data.Failed))).WithData(data)
	}

	// Resolve the repository route before moving active definitions. This keeps
	// network detection outside the destructive replacement window.
	configureRepoRoute(manager, opts)
	data.Region = manager.Region

	warning, err := manager.preparePigstyKey(expandedModules, opts.GPGCheck)
	if err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoAddFailed, err.Error()).WithData(data)
	}
	if warning != "" {
		data.Warnings = append(data.Warnings, warning)
		logrus.Warn(warning)
	}

	// Handle backup if doRemove is true
	if opts.Remove {
		backupInfo, err := backupReposStructured(manager)
		data.BackupInfo = backupInfo
		if err != nil {
			data.DurationMs = time.Since(startTime).Milliseconds()
			result := output.Fail(output.CodeRepoBackupFailed,
				fmt.Sprintf("failed to backup repos: %v", err)).WithData(data)
			if backupInfo != nil {
				return rollbackReplacement(manager, data, result, startTime)
			}
			return result
		}
	}

	addRepoModules(manager, expandedModules, data, opts.Remove)
	if opts.Remove && len(data.Failed) > 0 {
		data.DurationMs = time.Since(startTime).Milliseconds()
		result := output.Fail(output.CodeRepoAddFailed,
			fmt.Sprintf("failed to replace repositories: %d module(s) could not be added", len(data.Failed))).WithData(data)
		return rollbackReplacement(manager, data, result, startTime)
	}

	// Handle cache update if doUpdate is true
	if opts.Update {
		data.UpdateResult = updateRepoCache(manager, opts.Quiet)
	}

	data.DurationMs = time.Since(startTime).Milliseconds()
	result := buildAddReposResult(data, expandedModules)
	if opts.Remove && !result.Success {
		return rollbackReplacement(manager, data, result, startTime)
	}
	return result
}

func missingRepoModules(manager *Manager, modules []string) []*FailedRepoItem {
	var failed []*FailedRepoItem
	for _, module := range modules {
		if _, ok := manager.Module[module]; !ok {
			failed = append(failed, &FailedRepoItem{
				Module: module, Error: fmt.Sprintf("module not found: %s", module), Code: output.CodeRepoModuleNotFound,
			})
		}
	}
	return failed
}

func configureRepoRoute(manager *Manager, opts AddOptions) {
	if opts.Mirror {
		applyPGDGMirrorRoute(manager)
	} else {
		manager.DetectRegion(opts.Region)
	}
}

func addRepoModules(manager *Manager, modules []string, data *RepoAddData, stopOnFailure bool) {
	failed := make(map[string]bool, len(data.Failed))
	for _, item := range data.Failed {
		failed[item.Module] = true
	}
	for _, module := range modules {
		if failed[module] {
			continue
		}
		repos := manager.Module[module]
		path := manager.getModulePath(module)
		if err := addRepoModule(manager, module); err != nil {
			data.Failed = append(data.Failed, &FailedRepoItem{Module: module, Error: err.Error(), Code: output.CodeRepoAddFailed})
			failed[module] = true
			if stopOnFailure {
				return
			}
			continue
		}
		data.AddedRepos = append(data.AddedRepos, &AddedRepoItem{
			Module: module, FilePath: path, Repos: repos,
		})
	}
}

func updateRepoCache(manager *Manager, quiet bool) *UpdateCacheResult {
	run := runAddRepoCommand
	if quiet {
		run = runAddRepoCommandSummary
	}
	result := &UpdateCacheResult{Command: strings.Join(manager.UpdateCmd, " ")}
	if err := run(manager.UpdateCmd); err != nil {
		result.Error = err.Error()
		return result
	}
	result.Success = true
	return result
}

func rollbackReplacement(manager *Manager, data *RepoAddData, result *output.Result, startTime time.Time) *output.Result {
	rollback, err := rollbackReposStructured(manager, data)
	data.RollbackInfo = rollback
	data.DurationMs = time.Since(startTime).Milliseconds()
	if err != nil {
		rollbackDetail := fmt.Sprintf("repository rollback failed: %v", err)
		if result.Detail == "" {
			result.Detail = rollbackDetail
		} else {
			result.Detail += "; " + rollbackDetail
		}
		return result
	}
	if result.Detail == "" {
		result.Detail = "previous repository definitions restored"
	} else {
		result.Detail += "; previous repository definitions restored"
	}
	return result
}

func rollbackReposStructured(manager *Manager, data *RepoAddData) (*RollbackInfo, error) {
	if data == nil {
		info := &RollbackInfo{Error: "repository replacement metadata is unavailable"}
		return info, errors.New(info.Error)
	}
	addedPaths := make([]string, 0, len(data.AddedRepos))
	for _, item := range data.AddedRepos {
		if item == nil {
			continue
		}
		addedPaths = append(addedPaths, item.FilePath)
	}
	return restoreReposStructured(manager, data.BackupInfo, addedPaths)
}

// RestoreRepos removes repository definitions created by a failed replacement
// and restores the exact active paths recorded by BackupInfo.
func RestoreRepos(backup *BackupInfo, addedPaths ...string) (*RollbackInfo, error) {
	manager, err := NewManager()
	if err != nil {
		info := &RollbackInfo{Error: fmt.Sprintf("initialize repository manager: %v", err)}
		return info, errors.New(info.Error)
	}
	return restoreReposStructured(manager, backup, addedPaths)
}

func restoreReposStructured(manager *Manager, backup *BackupInfo, addedPaths []string) (*RollbackInfo, error) {
	info := &RollbackInfo{}
	if manager == nil || backup == nil {
		info.Error = "repository backup metadata is unavailable"
		return info, errors.New(info.Error)
	}
	backupDir := filepath.Clean(backup.BackupDir)
	if backupDir != filepath.Clean(manager.BackupDir) {
		info.Error = "repository backup directory does not match manager configuration"
		return info, errors.New(info.Error)
	}

	var rollbackErrors []error
	seenAdded := make(map[string]bool)
	for _, addedPath := range addedPaths {
		path := filepath.Clean(addedPath)
		if seenAdded[path] {
			continue
		}
		seenAdded[path] = true
		if !isPathWithinBase(manager.RepoDir, path) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("refusing unsafe failed repository path %s", path))
			continue
		}
		if err := runAddRepoCommand([]string{"rm", "-f", path}); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("remove failed repository file %s: %w", path, err))
			continue
		}
		info.RemovedFiles = append(info.RemovedFiles, path)
	}

	for _, original := range backup.BackedUpFiles {
		original = filepath.Clean(original)
		if !validRepoRestoreTarget(manager, original) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("refusing unsafe repository restore target %s", original))
			continue
		}
		source := backupPathForRepoFile(manager, original)
		if !isPathWithinBase(backupDir, source) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("refusing unsafe repository backup path %s", source))
			continue
		}
		if err := runAddRepoCommand([]string{"mv", "-f", source, original}); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore repository file %s: %w", original, err))
			continue
		}
		info.RestoredFiles = append(info.RestoredFiles, original)
	}

	if err := errors.Join(rollbackErrors...); err != nil {
		info.Error = err.Error()
		return info, err
	}
	info.Success = true
	return info, nil
}

func validRepoRestoreTarget(manager *Manager, path string) bool {
	if manager == nil {
		return false
	}
	if isPathWithinBase(manager.RepoDir, path) {
		return true
	}
	return config.OSType == config.DistroDEB && path == "/etc/apt/sources.list"
}

func backupPathForRepoFile(manager *Manager, original string) string {
	name := filepath.Base(original)
	if config.OSType == config.DistroDEB && filepath.Clean(original) == "/etc/apt/sources.list" {
		name = ".pig-root-sources.list"
	}
	return filepath.Join(manager.BackupDir, name)
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
	if len(data.Failed) > 0 {
		return output.Fail(output.CodeRepoAddFailed,
			fmt.Sprintf("added %d module(s), but %d requested module(s) failed", len(data.AddedRepos), len(data.Failed))).
			WithData(data).
			WithDetail("partial repository changes were applied; inspect failed entries before retrying")
	}

	message := fmt.Sprintf("Added %d modules (%d repos)", len(data.AddedRepos), countRepos(data.AddedRepos))
	return output.OK(message, data)
}

// backupReposStructured performs backup and returns structured info
func backupReposStructured(manager *Manager) (*BackupInfo, error) {
	logrus.Infof("backup repos from %s to %s", manager.RepoDir, manager.BackupDir)

	// Create backup directory
	if err := runAddRepoCommand([]string{"mkdir", "-p", manager.BackupDir}); err != nil {
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
	debSourcesList := ""
	if config.OSType == config.DistroDEB {
		const path = "/etc/apt/sources.list"
		if fileInfo, err := os.Stat(path); err == nil && fileInfo.Size() > 0 {
			debSourcesList = path
		}
	}
	backupTargets := make(map[string]string, len(filesToMove)+1)
	for _, source := range append(append([]string(nil), filesToMove...), debSourcesList) {
		if source == "" {
			continue
		}
		target := backupPathForRepoFile(manager, source)
		if previous, exists := backupTargets[target]; exists {
			return nil, fmt.Errorf("repository backup path collision: %s and %s", previous, source)
		}
		backupTargets[target] = source
	}

	backupInfo := &BackupInfo{
		BackupDir:     manager.BackupDir,
		BackedUpFiles: make([]string, 0, len(filesToMove)+1),
	}

	// Move files individually so a partial failure can be rolled back from the
	// exact set that already reached the backup directory.
	for _, source := range filesToMove {
		target := backupPathForRepoFile(manager, source)
		logrus.Debugf("mv -f %s %s", filepath.Base(source), target)
		if err := runAddRepoCommand([]string{"mv", "-f", source, target}); err != nil {
			return backupInfo, fmt.Errorf("failed to move repo file %s: %w", source, err)
		}
		backupInfo.BackedUpFiles = append(backupInfo.BackedUpFiles, source)
	}

	// For debian, also backup sources.list
	if debSourcesList != "" {
		if err := runAddRepoCommand([]string{"mv", "-f", debSourcesList, backupPathForRepoFile(manager, debSourcesList)}); err != nil {
			return backupInfo, err
		}
		backupInfo.BackedUpFiles = append(backupInfo.BackedUpFiles, debSourcesList)
		if err := runAddRepoCommand([]string{"touch", debSourcesList}); err != nil {
			return backupInfo, err
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

func applyPGDGMirrorRoute(m *Manager) {
	if m == nil {
		return
	}
	// The bundled China definitions are authoritative: PGDG uses Tencent Cloud
	// first, with any deliberately declared fallback retained for compatibility.
	m.Region = "china"
}
