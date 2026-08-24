package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/archive"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultRepoTargetDir = "/www"
	defaultRepoDataDir   = "/data/nginx"
)

// addLocalRepo adds local repo config for dnf/apt
func addLocalRepo(targetDir string) error {
	switch config.OSType {
	case config.DistroEL:
		logrus.Infof("add %s to %s", fmt.Sprintf("file://%s", targetDir), "/etc/yum.repos.d/local.repo")
		repoFile := "/etc/yum.repos.d/local.repo"
		return utils.PutFile(repoFile, []byte(elLocalRepoContent(targetDir)))
	case config.DistroDEB:
		logrus.Infof("add %s to %s", fmt.Sprintf("file:%s ./", targetDir), "/etc/apt/sources.list.d/local.list")
		repoFile := "/etc/apt/sources.list.d/local.list"
		repoContent := fmt.Sprintf("deb [trusted=yes] file://%s ./", targetDir)
		return utils.PutFile(repoFile, []byte(repoContent))
	default:
		return fmt.Errorf("unsupported OS for adding local repo: %s", config.OSType)
	}
}

func elLocalRepoContent(targetDir string) string {
	lines := []string{
		"[pigsty-local]",
		"name=Pigsty Local Repo",
		fmt.Sprintf("baseurl=file://%s", targetDir),
		"enabled=1",
	}
	if config.OSVersion == "8" || config.OSVersion == "9" {
		lines = append(lines, "module_hotfixes=1")
	}
	return strings.Join(append(lines, "gpgcheck=0", ""), "\n")
}

// AddLocalRepo exposes the existing canonical local-repository writer to
// higher-level workflows such as native sty boot.
func AddLocalRepo(targetDir string) error { return addLocalRepo(targetDir) }

var (
	repoBootEUID     = os.Geteuid
	repoBootAddLocal = addLocalRepo
)

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

// LocalRepoPath returns the canonical definition written by AddLocalRepo.
func LocalRepoPath() string { return getLocalRepoPath() }

// BootWithResult bootstraps a local repo from an offline package and enables
// its package-manager definition.
func BootWithResult(offlinePkg, targetDir string) *output.Result {
	return bootWithResult(offlinePkg, targetDir, true)
}

// ExtractOfflineWithResult commits an offline repository tree without writing
// package-manager definitions. Higher-level replacement workflows use this so
// existing definitions can be backed up before local.repo is changed.
func ExtractOfflineWithResult(offlinePkg, targetDir string) *output.Result {
	return bootWithResult(offlinePkg, targetDir, false)
}

func bootWithResult(offlinePkg, targetDir string, configureLocal bool) *output.Result {
	startTime := time.Now()

	// Use defaults if not provided
	if targetDir == "" {
		targetDir = defaultRepoTargetDir
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

	pigstyDir := filepath.Join(targetDir, "pigsty")
	reuseCommitted := false
	if info, err := os.Lstat(pigstyDir); err == nil && info.IsDir() && regularFileAt(filepath.Join(pigstyDir, "repo_complete")) {
		reuseCommitted = true
	}

	// Check if offline package exists
	if _, err := os.Stat(offlinePkg); !reuseCommitted && os.IsNotExist(err) {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoPackageNotFound,
			fmt.Sprintf("offline package not found: %s", offlinePkg)).WithData(data)
	}
	if repoBootEUID() != 0 {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed,
			"native repository extraction requires root; run pig repo boot with sudo").WithData(data)
	}

	// Match Pigsty's repository layout on a fresh host: /www is a stable
	// convenience link while repository data lives below /data/nginx. Existing
	// directories and valid directory symlinks are preserved for compatibility.
	if err := ensureRepoTargetDir(targetDir); err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed,
			fmt.Sprintf("failed to prepare target directory: %v", err)).WithData(data)
	}

	// Stream through Go's gzip/tar readers. Neither native sty boot nor direct
	// repo boot invokes or installs host tar/gzip executables.
	reusedExisting, extractErr := extractOfflineRepo(offlinePkg, targetDir, pigstyDir, reuseCommitted)
	if extractErr != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed,
			fmt.Sprintf("failed to extract package: %v", extractErr)).WithData(data)
	}

	// List extracted directories (top-level only for brevity)
	entries, _ := os.ReadDir(targetDir)
	for _, entry := range entries {
		if entry.IsDir() {
			data.ExtractedFiles = append(data.ExtractedFiles, entry.Name())
		}
	}

	return finishRepoBoot(data, offlinePkg, targetDir, pigstyDir, configureLocal, reusedExisting, startTime)
}

func extractOfflineRepo(offlinePkg, targetDir, pigstyDir string, reuseCommitted bool) (bool, error) {
	if reuseCommitted {
		return true, nil
	}
	if info, err := os.Lstat(pigstyDir); err == nil {
		if info.IsDir() && regularFileAt(filepath.Join(pigstyDir, "repo_complete")) {
			return true, nil
		}
		return false, fmt.Errorf("target already exists without a valid repo; remove or move it before retrying: %s", pigstyDir)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	stage, err := os.MkdirTemp(targetDir, ".pig-repo-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(stage)
	if err := archive.ExtractTarGz(offlinePkg, stage); err != nil {
		return false, err
	}
	stagedRepo := filepath.Join(stage, "pigsty")
	if !regularFileAt(filepath.Join(stagedRepo, "repo_complete")) {
		return false, fmt.Errorf("offline package is missing pigsty/repo_complete")
	}
	return false, os.Rename(stagedRepo, pigstyDir)
}

func finishRepoBoot(data *RepoBootData, offlinePkg, targetDir, pigstyDir string, configureLocal, reusedExisting bool, startTime time.Time) *output.Result {
	repoCompleteFile := filepath.Join(pigstyDir, "repo_complete")
	if !regularFileAt(repoCompleteFile) {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed, fmt.Sprintf("offline package is missing %s", repoCompleteFile)).WithData(data)
	}
	if !configureLocal {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.OK(repoBootMessage("Extracted", offlinePkg, targetDir, reusedExisting), data)
	}
	data.LocalRepoPath = getLocalRepoPath()
	if err := repoBootAddLocal(pigstyDir); err != nil {
		data.DurationMs = time.Since(startTime).Milliseconds()
		return output.Fail(output.CodeRepoBootFailed, fmt.Sprintf("failed to add local repo config: %v", err)).WithData(data)
	}
	data.LocalRepoAdded = true
	data.DurationMs = time.Since(startTime).Milliseconds()
	message := repoBootMessage("Bootstrapped", offlinePkg, targetDir, reusedExisting)
	return output.OK(message+fmt.Sprintf(", local repo config added: %s", data.LocalRepoPath), data)
}

func repoBootMessage(verb, offlinePkg, targetDir string, reusedExisting bool) string {
	if reusedExisting {
		return fmt.Sprintf("Reused existing repo at %s", filepath.Join(targetDir, "pigsty"))
	}
	return fmt.Sprintf("%s repo from %s to %s", verb, offlinePkg, targetDir)
}

func ensureRepoTargetDir(targetDir string) error {
	return ensureRepoTargetLayout(targetDir, defaultRepoTargetDir, defaultRepoDataDir)
}

func ensureRepoTargetLayout(targetDir, repoHome, repoData string) error {
	if filepath.Clean(targetDir) != filepath.Clean(repoHome) {
		return os.MkdirAll(targetDir, 0o755)
	}

	info, err := os.Lstat(repoHome)
	if err == nil {
		return validateRepoHome(repoHome, info)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect repository home %s: %w", repoHome, err)
	}
	if err := os.MkdirAll(repoData, 0o755); err != nil {
		return fmt.Errorf("create repository data directory %s: %w", repoData, err)
	}
	if err := os.Symlink(repoData, repoHome); err != nil {
		if os.IsExist(err) {
			info, statErr := os.Lstat(repoHome)
			if statErr == nil {
				return validateRepoHome(repoHome, info)
			}
		}
		return fmt.Errorf("link repository home %s to %s: %w", repoHome, repoData, err)
	}
	return nil
}

func validateRepoHome(repoHome string, info os.FileInfo) error {
	if info.IsDir() {
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Stat(repoHome)
		if err != nil {
			return fmt.Errorf("repository home symlink %s is not usable: %w", repoHome, err)
		}
		if target.IsDir() {
			return nil
		}
	}
	return fmt.Errorf("repository home %s exists but is not a directory or directory symlink", repoHome)
}

func regularFileAt(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}
