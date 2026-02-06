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
	"sort"
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
	OSEnv     *OSEnvironment      `json:"os_env" yaml:"os_env"`
	ShowAll   bool                `json:"show_all,omitempty" yaml:"show_all,omitempty"`
	RepoCount int                 `json:"repo_count" yaml:"repo_count"`
	Repos     []*RepoSummary      `json:"repos" yaml:"repos"`
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
 * Text Methods for DTOs
 ********************/

// Text returns a human-readable representation of RepoListData
// that matches the output quality of the old List()/ListAll() functions.
func (r *RepoListData) Text() string {
	if r == nil {
		return ""
	}
	var sb strings.Builder

	// OS environment header
	if r.OSEnv != nil {
		sb.WriteString(fmt.Sprintf("os_environment: {code: %s, arch: %s, type: %s, major: %d}\n",
			r.OSEnv.Code, r.OSEnv.Arch, r.OSEnv.Type, r.OSEnv.Major))
	}

	if r.ShowAll {
		// Show all repos with available/unavailable markers
		sb.WriteString(fmt.Sprintf("repo_rawdata:  # {total: %d}\n", r.RepoCount))
		for _, repo := range r.Repos {
			if repo == nil {
				continue
			}
			marker := "o"
			if !repo.Available {
				marker = "x"
			}
			sb.WriteString(fmt.Sprintf("  %s { name: %-14s ,description: %-20s ,module: %-8s ,releases: %s ,arch: %s ,baseurl: '%s' }\n",
				marker, repo.Name, fmt.Sprintf("'%s'", repo.Description), repo.Module,
				formatIntSlice(repo.Releases), formatStrSlice(repo.Arch), repo.BaseURL))
		}
	} else {
		// Show available repos
		sb.WriteString(fmt.Sprintf("repo_upstream:  # Available Repo: %d\n", r.RepoCount))
		for _, repo := range r.Repos {
			if repo == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - { name: %-14s ,description: %-20s ,module: %-8s ,releases: %s ,arch: %s ,baseurl: '%s' }\n",
				repo.Name, fmt.Sprintf("'%s'", repo.Description), repo.Module,
				formatIntSlice(repo.Releases), formatStrSlice(repo.Arch), repo.BaseURL))
		}

		// Modules section
		if len(r.Modules) > 0 {
			// Sort module names for deterministic output
			moduleNames := make([]string, 0, len(r.Modules))
			for name := range r.Modules {
				moduleNames = append(moduleNames, name)
			}
			sort.Strings(moduleNames)
			sb.WriteString(fmt.Sprintf("repo_modules:   # Available Modules: %d\n", len(r.Modules)))
			for _, name := range moduleNames {
				sb.WriteString(fmt.Sprintf("  - %-10s: %s\n", name, strings.Join(r.Modules[name], ", ")))
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Text returns a human-readable representation of RepoInfoData
// that matches the output quality of the old Info() function.
func (r *RepoInfoData) Text() string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	for i, repo := range r.Repos {
		if repo == nil {
			continue
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("#-------------------------------------------------\n")
		sb.WriteString(fmt.Sprintf("Name       : %s\n", repo.Name))
		sb.WriteString(fmt.Sprintf("Summary    : %s\n", repo.Description))
		avail := "No"
		if repo.Available {
			avail = "Yes"
		}
		sb.WriteString(fmt.Sprintf("Available  : %s\n", avail))
		sb.WriteString(fmt.Sprintf("Module     : %s\n", repo.Module))
		sb.WriteString(fmt.Sprintf("OS Arch    : %s\n", formatStrSlice(repo.Arch)))
		sb.WriteString(fmt.Sprintf("OS Distro  : %s\n", formatIntSlice(repo.Releases)))
		// Meta
		metaStr := ""
		if len(repo.Meta) > 0 {
			parts := make([]string, 0, len(repo.Meta))
			for k, v := range repo.Meta {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			metaStr = strings.Join(parts, " ")
		}
		sb.WriteString(fmt.Sprintf("Meta       : %s\n", metaStr))
		// Base URL
		defaultURL := ""
		if repo.BaseURL != nil {
			defaultURL = repo.BaseURL["default"]
		}
		sb.WriteString(fmt.Sprintf("Base URL   : %s\n", defaultURL))
		// Additional regions
		if len(repo.BaseURL) > 1 {
			for key, value := range repo.BaseURL {
				if key != "default" {
					sb.WriteString(fmt.Sprintf("%10s : %s\n", key, value))
				}
			}
		}
		// Content
		if repo.Content != "" {
			sb.WriteString(fmt.Sprintf("\n# default repo content\n%s\n", repo.Content))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Text returns a human-readable representation of RepoStatusData
func (r *RepoStatusData) Text() string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Repo Dir: %s\n", r.RepoDir))
	if len(r.RepoFiles) > 0 {
		sb.WriteString("Repo Files:\n")
		for _, f := range r.RepoFiles {
			sb.WriteString(fmt.Sprintf("  %s\n", f))
		}
	} else {
		sb.WriteString("Repo Files: (none)\n")
	}
	if len(r.ActiveRepos) > 0 {
		sb.WriteString("Active Repos:\n")
		for _, r := range r.ActiveRepos {
			sb.WriteString(fmt.Sprintf("  %s\n", r))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Text returns a human-readable representation of RepoAddData
func (r *RepoAddData) Text() string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	if r.BackupInfo != nil && len(r.BackupInfo.BackedUpFiles) > 0 {
		sb.WriteString(fmt.Sprintf("Backed up %d files to %s\n", len(r.BackupInfo.BackedUpFiles), r.BackupInfo.BackupDir))
	}
	for _, item := range r.AddedRepos {
		if item == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("Added module: %s -> %s  repos: %s\n", item.Module, item.FilePath, strings.Join(item.Repos, ", ")))
	}
	for _, item := range r.Failed {
		if item == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("Failed module: %s -> %s\n", item.Module, item.Error))
	}
	if r.UpdateResult != nil {
		if r.UpdateResult.Success {
			sb.WriteString(fmt.Sprintf("Cache updated: %s\n", r.UpdateResult.Command))
		} else {
			sb.WriteString(fmt.Sprintf("Cache update failed: %s (%s)\n", r.UpdateResult.Error, r.UpdateResult.Command))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatIntSlice formats an int slice to a compact inline string like [7,8,9]
func formatIntSlice(rs []int) string {
	if len(rs) == 0 {
		return "[]"
	}
	parts := make([]string, len(rs))
	for i, v := range rs {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// formatStrSlice formats a string slice to a compact inline string like [x86_64, aarch64]
func formatStrSlice(a []string) string {
	if len(a) == 0 {
		return "[]"
	}
	return "[" + strings.Join(a, ", ") + "]"
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
		return output.Fail(output.CodeRepoInvalidArgs, "repo or module name is required")
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

/********************
 * Data Transfer Objects (DTOs) for repo create command
 ********************/

// RepoCreateData is the DTO for repo create command
type RepoCreateData struct {
	OSEnv        *OSEnvironment     `json:"os_env" yaml:"os_env"`
	RepoDirs     []string           `json:"repo_dirs" yaml:"repo_dirs"`
	CreatedRepos []*CreatedRepoItem `json:"created_repos" yaml:"created_repos"`
	FailedRepos  []*FailedRepoItem  `json:"failed_repos,omitempty" yaml:"failed_repos,omitempty"`
	DurationMs   int64              `json:"duration_ms" yaml:"duration_ms"`
}

// CreatedRepoItem represents a successfully created repository
type CreatedRepoItem struct {
	Path         string `json:"path" yaml:"path"`
	RepoType     string `json:"repo_type" yaml:"repo_type"`
	CompleteFile string `json:"complete_file" yaml:"complete_file"`
}

// Text returns a human-readable representation of CreatedRepoItem
func (c *CreatedRepoItem) Text() string {
	if c == nil {
		return ""
	}
	return c.Path
}

// Text returns a human-readable representation of FailedRepoItem
func (f *FailedRepoItem) Text() string {
	if f == nil {
		return ""
	}
	if f.Module != "" {
		return fmt.Sprintf("%s (error: %s)", f.Module, f.Error)
	}
	return f.Error
}

// Text returns a human-readable representation of RepoCreateData
func (r *RepoCreateData) Text() string {
	if r == nil {
		return ""
	}
	successCount := len(r.CreatedRepos)
	failCount := len(r.FailedRepos)
	if failCount > 0 {
		return fmt.Sprintf("Created %d repos, failed %d", successCount, failCount)
	}
	return fmt.Sprintf("Created %d repos", successCount)
}

/********************
 * Data Transfer Objects (DTOs) for repo boot command
 ********************/

// RepoBootData is the DTO for repo boot command
type RepoBootData struct {
	OSEnv          *OSEnvironment `json:"os_env" yaml:"os_env"`
	SourcePkg      string         `json:"source_pkg" yaml:"source_pkg"`
	TargetDir      string         `json:"target_dir" yaml:"target_dir"`
	ExtractedFiles []string       `json:"extracted_files,omitempty" yaml:"extracted_files,omitempty"`
	LocalRepoAdded bool           `json:"local_repo_added" yaml:"local_repo_added"`
	LocalRepoPath  string         `json:"local_repo_path,omitempty" yaml:"local_repo_path,omitempty"`
	DurationMs     int64          `json:"duration_ms" yaml:"duration_ms"`
}

// Text returns a human-readable representation of RepoBootData
func (r *RepoBootData) Text() string {
	if r == nil {
		return ""
	}
	if r.LocalRepoAdded {
		return fmt.Sprintf("Bootstrapped from %s to %s, local repo added: %s", r.SourcePkg, r.TargetDir, r.LocalRepoPath)
	}
	return fmt.Sprintf("Bootstrapped from %s to %s", r.SourcePkg, r.TargetDir)
}

/********************
 * Data Transfer Objects (DTOs) for repo cache command
 ********************/

// RepoCacheData is the DTO for repo cache command
type RepoCacheData struct {
	OSEnv         *OSEnvironment `json:"os_env" yaml:"os_env"`
	SourceDir     string         `json:"source_dir" yaml:"source_dir"`
	TargetPkg     string         `json:"target_pkg" yaml:"target_pkg"`
	IncludedRepos []string       `json:"included_repos" yaml:"included_repos"`
	PackageSize   int64          `json:"package_size" yaml:"package_size"`
	PackageMD5    string         `json:"package_md5,omitempty" yaml:"package_md5,omitempty"`
	DurationMs    int64          `json:"duration_ms" yaml:"duration_ms"`
}

// Text returns a human-readable representation of RepoCacheData
func (r *RepoCacheData) Text() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("Cached %d repos to %s (%.2f GiB)", len(r.IncludedRepos), r.TargetPkg, float64(r.PackageSize)/(1024.0*1024.0*1024.0))
}

/********************
 * Data Transfer Objects (DTOs) for repo reload command
 ********************/

// RepoReloadData is the DTO for repo reload command
type RepoReloadData struct {
	SourceURL    string `json:"source_url" yaml:"source_url"`
	RepoCount    int    `json:"repo_count" yaml:"repo_count"`
	CatalogPath  string `json:"catalog_path,omitempty" yaml:"catalog_path,omitempty"`
	DownloadedAt string `json:"downloaded_at,omitempty" yaml:"downloaded_at,omitempty"`
	DurationMs   int64  `json:"duration_ms" yaml:"duration_ms"`
}

// Text returns a human-readable representation of RepoReloadData
func (r *RepoReloadData) Text() string {
	if r == nil {
		return ""
	}
	if r.CatalogPath != "" {
		return fmt.Sprintf("Reloaded %d repos from %s to %s", r.RepoCount, r.SourceURL, r.CatalogPath)
	}
	return fmt.Sprintf("Reloaded %d repos from %s", r.RepoCount, r.SourceURL)
}
