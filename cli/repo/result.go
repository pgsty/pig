/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package repo

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"strings"
)

/********************
 * Data Transfer Objects (DTOs) for ANCS Output
 * These structures are used for structured YAML/JSON output
 ********************/

// OSEnvironment represents the operating system environment
type OSEnvironment struct {
	Code  string `json:"code" yaml:"code"`
	Arch  string `json:"arch" yaml:"arch"`
	Type  string `json:"type" yaml:"type"`
	Major int    `json:"major" yaml:"major"`
}

// RepoSummary is a compact representation of a repository for list output
type RepoSummary struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Module      string   `json:"module" yaml:"module"`
	Releases    []int    `json:"releases" yaml:"releases"`
	Arch        []string `json:"arch" yaml:"arch"`
	BaseURL     string   `json:"baseurl" yaml:"baseurl"`
	Available   bool     `json:"available" yaml:"available"`
}

// RepoListData is the DTO for repo list command
type RepoListData struct {
	OSEnv     *OSEnvironment     `json:"os_env" yaml:"os_env"`
	ShowAll   bool               `json:"show_all,omitempty" yaml:"show_all,omitempty"`
	RepoCount int                `json:"repo_count" yaml:"repo_count"`
	Repos     []*RepoSummary     `json:"repos" yaml:"repos"`
	Modules   map[string][]string `json:"modules" yaml:"modules"`
}

// RepoDetail contains full repository information for info output
type RepoDetail struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Module      string            `json:"module" yaml:"module"`
	Releases    []int             `json:"releases" yaml:"releases"`
	Arch        []string          `json:"arch" yaml:"arch"`
	BaseURL     map[string]string `json:"baseurl" yaml:"baseurl"`
	Meta        map[string]string `json:"meta,omitempty" yaml:"meta,omitempty"`
	Minor       bool              `json:"minor,omitempty" yaml:"minor,omitempty"`
	Available   bool              `json:"available" yaml:"available"`
	Content     string            `json:"content,omitempty" yaml:"content,omitempty"`
}

// RepoInfoData is the DTO for repo info command
type RepoInfoData struct {
	Requested []string      `json:"requested" yaml:"requested"`
	Repos     []*RepoDetail `json:"repos" yaml:"repos"`
}

// RepoStatusData is the DTO for repo status command
type RepoStatusData struct {
	OSType      string   `json:"os_type" yaml:"os_type"`
	RepoDir     string   `json:"repo_dir" yaml:"repo_dir"`
	RepoFiles   []string `json:"repo_files" yaml:"repo_files"`
	ActiveRepos []string `json:"active_repos,omitempty" yaml:"active_repos,omitempty"`
}

/********************
 * Conversion Methods
 ********************/

// ToSummary converts a Repository to RepoSummary
func (r *Repository) ToSummary() *RepoSummary {
	if r == nil {
		return nil
	}
	return &RepoSummary{
		Name:        r.Name,
		Description: r.Description,
		Module:      r.Module,
		Releases:    r.Releases,
		Arch:        r.Arch,
		BaseURL:     r.BaseURL["default"],
		Available:   r.AvailableInCurrentOS(),
	}
}

// ToDetail converts a Repository to RepoDetail
func (r *Repository) ToDetail() *RepoDetail {
	if r == nil {
		return nil
	}
	return &RepoDetail{
		Name:        r.Name,
		Description: r.Description,
		Module:      r.Module,
		Releases:    r.Releases,
		Arch:        r.Arch,
		BaseURL:     r.BaseURL,
		Meta:        r.Meta,
		Minor:       r.Minor,
		Available:   r.AvailableInCurrentOS(),
		Content:     r.Content("default"),
	}
}

/********************
 * Result Constructors
 ********************/

// ListRepos returns a structured Result for the repo list command
func ListRepos(showAll bool) *output.Result {
	rm, err := NewManager()
	if err != nil {
		return output.Fail(output.CodeRepoManagerError, fmt.Sprintf("failed to get repo manager: %v", err))
	}

	osEnv := &OSEnvironment{
		Code:  rm.OsDistroCode,
		Arch:  rm.OsArch,
		Type:  rm.OsType,
		Major: rm.OsMajorVersion,
	}

	var repos []*RepoSummary
	if showAll {
		// Return all repos with availability flag
		for _, r := range rm.Data {
			if r == nil {
				continue
			}
			repos = append(repos, r.ToSummary())
		}
	} else {
		// Return only available repos
		for _, r := range rm.List {
			if r == nil {
				continue
			}
			repos = append(repos, r.ToSummary())
		}
	}

	// Build modules map
	modules := make(map[string][]string)
	for _, module := range rm.ModuleOrder() {
		modules[module] = rm.Module[module]
	}

	data := &RepoListData{
		OSEnv:     osEnv,
		ShowAll:   showAll,
		RepoCount: len(repos),
		Repos:     repos,
		Modules:   modules,
	}

	message := fmt.Sprintf("Found %d repositories", data.RepoCount)
	if showAll {
		message = fmt.Sprintf("Found %d repositories (all, including unavailable)", data.RepoCount)
	}

	return output.OK(message, data)
}

// GetRepoInfo returns a structured Result for the repo info command
func GetRepoInfo(args []string) *output.Result {
	if len(args) == 0 {
		return output.Fail(output.CodeRepoNotFound, "repo or module name is required")
	}

	rm, err := NewManager()
	if err != nil {
		return output.Fail(output.CodeRepoManagerError, fmt.Sprintf("failed to get repo manager: %v", err))
	}

	// Expand modules to repo names
	var repoList []string
	repoDedupe := make(map[string]bool)

	for _, arg := range args {
		if rm.Module[arg] != nil {
			// Treat it as module name
			for _, repoName := range rm.Module[arg] {
				if !repoDedupe[repoName] {
					repoList = append(repoList, repoName)
					repoDedupe[repoName] = true
				}
			}
		} else {
			// Treat it as repo name
			if !repoDedupe[arg] {
				repoList = append(repoList, arg)
				repoDedupe[arg] = true
			}
		}
	}

	// Find repo details
	var repos []*RepoDetail
	var notFound []string

	for _, name := range repoList {
		found := false
		for _, r := range rm.Data {
			if r.Name == name {
				repos = append(repos, r.ToDetail())
				found = true
				break
			}
		}
		if !found {
			notFound = append(notFound, name)
		}
	}

	if len(repos) == 0 {
		return output.Fail(output.CodeRepoNotFound, fmt.Sprintf("repositories not found: %v", notFound))
	}

	data := &RepoInfoData{
		Requested: args,
		Repos:     repos,
	}

	message := fmt.Sprintf("Found %d repositories", len(repos))
	result := output.OK(message, data)
	if len(notFound) > 0 {
		result.Detail = fmt.Sprintf("not found: %v", notFound)
	}
	return result
}

// GetRepoStatus returns a structured Result for the repo status command
func GetRepoStatus() *output.Result {
	osType := config.OSType

	if osType != config.DistroEL && osType != config.DistroDEB {
		return output.Fail(output.CodeRepoUnsupportedOS, fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
	}

	var repoDir string
	var repoPattern string

	switch osType {
	case config.DistroEL:
		repoDir = "/etc/yum.repos.d"
		repoPattern = "/etc/yum.repos.d/*.repo"
	case config.DistroDEB:
		repoDir = "/etc/apt/sources.list.d"
		repoPattern = "/etc/apt/sources.list.d/*.list"
	}

	// Get repo files
	repoFiles, err := listRepoFiles(repoPattern)
	if err != nil {
		return output.Fail(output.CodeRepoManagerError, fmt.Sprintf("failed to list repo files: %v", err))
	}

	// Get active repos (requires system command, return empty on error)
	activeRepos, _ := getActiveRepos(osType)

	data := &RepoStatusData{
		OSType:      osType,
		RepoDir:     repoDir,
		RepoFiles:   repoFiles,
		ActiveRepos: activeRepos,
	}

	message := fmt.Sprintf("Repository status: %s", osType)
	return output.OK(message, data)
}

// listRepoFiles lists repository files matching the given pattern
func listRepoFiles(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// getActiveRepos returns the list of active repositories from the package manager
func getActiveRepos(osType string) ([]string, error) {
	switch osType {
	case config.DistroEL:
		return getActiveReposEL()
	case config.DistroDEB:
		return getActiveReposDEB()
	default:
		return []string{}, nil
	}
}

// getActiveReposEL parses `yum repolist` output to get active repo IDs
func getActiveReposEL() ([]string, error) {
	cmd := exec.Command("yum", "repolist", "-q")
	out, err := cmd.Output()
	if err != nil {
		return []string{}, err
	}

	var repos []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		// Skip header line (repo id, repo name)
		if lineNum == 1 && strings.Contains(strings.ToLower(line), "repo") {
			continue
		}
		// Parse repo ID (first column before spaces)
		fields := strings.Fields(line)
		if len(fields) > 0 {
			repos = append(repos, fields[0])
		}
	}
	return repos, nil
}

// getActiveReposDEB parses repo files to get active repo names
// Note: apt-cache policy output is complex, so we extract from sources.list.d files
func getActiveReposDEB() ([]string, error) {
	cmd := exec.Command("apt-cache", "policy")
	out, err := cmd.Output()
	if err != nil {
		return []string{}, err
	}

	// Parse lines like: "500 http://repo.pigsty.io/apt/... noble/main amd64 Packages"
	// Extract unique repo URLs/names
	repoSet := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Look for lines with priority (any number) and URL
		// apt-cache policy output format: "<priority> <url> <distribution> <component> <arch> Packages"
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Check if first field is a numeric priority (any positive integer)
			if isNumericPriority(fields[0]) {
				url := fields[1]
				// Extract repo identifier from URL
				if strings.Contains(url, "://") {
					// Use the hostname + path as identifier
					repoSet[url] = true
				}
			}
		}
	}

	repos := make([]string, 0, len(repoSet))
	for repo := range repoSet {
		repos = append(repos, repo)
	}
	return repos, nil
}

// isNumericPriority checks if a string is a valid apt priority (positive integer)
func isNumericPriority(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

/********************
 * Data Transfer Objects (DTOs) for repo add/set commands
 ********************/

// RepoAddData is the DTO for repo add/set command
type RepoAddData struct {
	OSEnv            *OSEnvironment     `json:"os_env" yaml:"os_env"`
	Region           string             `json:"region" yaml:"region"`
	RequestedModules []string           `json:"requested_modules" yaml:"requested_modules"`
	ExpandedModules  []string           `json:"expanded_modules" yaml:"expanded_modules"`
	AddedRepos       []*AddedRepoItem   `json:"added_repos" yaml:"added_repos"`
	Failed           []*FailedRepoItem  `json:"failed,omitempty" yaml:"failed,omitempty"`
	BackupInfo       *BackupInfo        `json:"backup_info,omitempty" yaml:"backup_info,omitempty"`
	UpdateResult     *UpdateCacheResult `json:"update_result,omitempty" yaml:"update_result,omitempty"`
	DurationMs       int64              `json:"duration_ms" yaml:"duration_ms"`
}

// AddedRepoItem represents a successfully added repository
type AddedRepoItem struct {
	Module   string   `json:"module" yaml:"module"`
	FilePath string   `json:"file_path" yaml:"file_path"`
	Repos    []string `json:"repos" yaml:"repos"`
}

// FailedRepoItem represents a failed repository add operation
type FailedRepoItem struct {
	Module string `json:"module" yaml:"module"`
	Error  string `json:"error" yaml:"error"`
	Code   int    `json:"code" yaml:"code"`
}

// BackupInfo contains information about backed up repository files
type BackupInfo struct {
	BackupDir     string   `json:"backup_dir" yaml:"backup_dir"`
	BackedUpFiles []string `json:"backed_up_files" yaml:"backed_up_files"`
}

// UpdateCacheResult contains the result of cache update operation
type UpdateCacheResult struct {
	Command string `json:"command" yaml:"command"`
	Success bool   `json:"success" yaml:"success"`
	Error   string `json:"error,omitempty" yaml:"error,omitempty"`
}

// IsEmpty returns true if BackupInfo is nil or has no backed up files
func (b *BackupInfo) IsEmpty() bool {
	if b == nil {
		return true
	}
	return len(b.BackedUpFiles) == 0
}

/********************
 * Data Transfer Objects (DTOs) for repo rm command
 ********************/

// RepoRmData is the DTO for repo rm command
type RepoRmData struct {
	OSEnv            *OSEnvironment     `json:"os_env" yaml:"os_env"`
	RequestedModules []string           `json:"requested_modules" yaml:"requested_modules"`
	RemovedRepos     []*RemovedRepoItem `json:"removed_repos" yaml:"removed_repos"`
	BackupInfo       *BackupInfo        `json:"backup_info,omitempty" yaml:"backup_info,omitempty"`
	UpdateResult     *UpdateCacheResult `json:"update_result,omitempty" yaml:"update_result,omitempty"`
	DurationMs       int64              `json:"duration_ms" yaml:"duration_ms"`
}

// RemovedRepoItem represents a successfully removed repository file
type RemovedRepoItem struct {
	Module   string `json:"module" yaml:"module"`
	FilePath string `json:"file_path" yaml:"file_path"`
	Success  bool   `json:"success" yaml:"success"`
	Error    string `json:"error,omitempty" yaml:"error,omitempty"`
}

// Text returns a human-readable representation of RemovedRepoItem
func (r *RemovedRepoItem) Text() string {
	if r == nil {
		return ""
	}
	if r.Success {
		return r.FilePath
	}
	return fmt.Sprintf("%s (error: %s)", r.FilePath, r.Error)
}

/********************
 * Data Transfer Objects (DTOs) for repo update command
 ********************/

// RepoUpdateData is the DTO for repo update command
type RepoUpdateData struct {
	OSEnv      *OSEnvironment `json:"os_env" yaml:"os_env"`
	Command    string         `json:"command" yaml:"command"`
	Success    bool           `json:"success" yaml:"success"`
	Error      string         `json:"error,omitempty" yaml:"error,omitempty"`
	DurationMs int64          `json:"duration_ms" yaml:"duration_ms"`
}

// Text returns a human-readable representation of RepoRmData
func (r *RepoRmData) Text() string {
	if r == nil {
		return ""
	}
	if r.BackupInfo != nil && len(r.BackupInfo.BackedUpFiles) > 0 {
		return fmt.Sprintf("Backed up %d files to %s", len(r.BackupInfo.BackedUpFiles), r.BackupInfo.BackupDir)
	}
	successCount := 0
	for _, item := range r.RemovedRepos {
		if item != nil && item.Success {
			successCount++
		}
	}
	return fmt.Sprintf("Removed %d module(s)", successCount)
}

// Text returns a human-readable representation of RepoUpdateData
func (r *RepoUpdateData) Text() string {
	if r == nil {
		return ""
	}
	if r.Success {
		return fmt.Sprintf("Cache updated successfully (command: %s)", r.Command)
	}
	return fmt.Sprintf("Cache update failed: %s (command: %s)", r.Error, r.Command)
}
