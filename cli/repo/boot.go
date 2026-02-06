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

// addLocalRepo adds local repo config for yum/apt
func addLocalRepo(targetDir string) error {
	switch config.OSType {
	case config.DistroEL:
		logrus.Infof("add %s to %s", fmt.Sprintf("file://%s", targetDir), "/etc/yum.repos.d/local.repo")
		repoFile := "/etc/yum.repos.d/local.repo"
		repoContent := strings.Join([]string{
			"[pigsty-local]",
			"name=Pigsty Local Repo",
			fmt.Sprintf("baseurl=file://%s", targetDir),
			"enabled=1",
			"gpgcheck=0",
			"",
		}, "\n")
		return utils.PutFile(repoFile, []byte(repoContent))
	case config.DistroDEB:
		logrus.Infof("add %s to %s", fmt.Sprintf("file:%s ./", targetDir), "/etc/apt/sources.list.d/local.list")
		repoFile := "/etc/apt/sources.list.d/local.list"
		repoContent := fmt.Sprintf("deb [trusted=yes] file://%s ./", targetDir)
		return utils.PutFile(repoFile, []byte(repoContent))
	default:
		return fmt.Errorf("unsupported OS for adding local repo: %s", config.OSType)
	}
}

// getLocalRepoPath returns the local repo config file path based on OS type
func getLocalRepoPath() string {
	switch config.OSType {
	case config.DistroEL:
		return "/etc/yum.repos.d/local.repo"
	case config.DistroDEB:
		return "/etc/apt/sources.list.d/local.list"
	default:
		return ""
	}
}

// BootWithResult bootstraps a local repo from offline package and returns a structured Result
// This function is used for YAML/JSON output modes
func BootWithResult(offlinePkg, targetDir string) *output.Result {
	startTime := time.Now()

	// Use defaults if not provided
	if targetDir == "" {
		targetDir = "/www"
	}
	if offlinePkg == "" {
		offlinePkg = "/tmp/pkg.tgz"
	}

	// Build OS environment info
	osEnv := &OSEnvironment{
		Code:  config.OSCode,
		Arch:  config.OSArch,
		Type:  config.OSType,
		Major: config.OSMajor,
	}

	// Prepare data structure
	data := &RepoBootData{
		OSEnv:     osEnv,
		SourcePkg: offlinePkg,
		TargetDir: targetDir,
	}

	// Check if offline package exists
	if _, err := os.Stat(offlinePkg); os.IsNotExist(err) {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoPackageNotFound,
			fmt.Sprintf("offline package not found: %s", offlinePkg)).WithData(data)
	}

	// Create target directory
	if err := utils.SudoCommand([]string{"mkdir", "-p", targetDir}); err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed,
			fmt.Sprintf("failed to create target directory: %v", err)).WithData(data)
	}

	// Extract package using tar
	if err := utils.SudoCommand([]string{"tar", "-xvzf", offlinePkg, "-C", targetDir}); err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed,
			fmt.Sprintf("failed to extract package: %v", err)).WithData(data)
	}

	// List extracted directories (top-level only for brevity)
	entries, _ := os.ReadDir(targetDir)
	for _, entry := range entries {
		if entry.IsDir() {
			data.ExtractedFiles = append(data.ExtractedFiles, entry.Name())
		}
	}

	// Check if targetDir/pigsty/repo_complete exists
	repoCompleteFile := filepath.Join(targetDir, "pigsty", "repo_complete")
	if _, err := os.Stat(repoCompleteFile); err == nil {
		pigstyDir := filepath.Join(targetDir, "pigsty")
		data.LocalRepoPath = getLocalRepoPath()
		if err := addLocalRepo(pigstyDir); err != nil {
			data.DurationMs = time.Since(startTime).Milliseconds()
			return output.Fail(output.CodeRepoBootFailed,
				fmt.Sprintf("failed to add local repo config: %v", err)).WithData(data)
		}
		data.LocalRepoAdded = true
	}

	data.DurationMs = time.Since(startTime).Milliseconds()

	message := fmt.Sprintf("Bootstrapped repo from %s to %s", offlinePkg, targetDir)
	if data.LocalRepoAdded {
		message += fmt.Sprintf(", local repo config added: %s", data.LocalRepoPath)
	}

	return output.OK(message, data)
}
