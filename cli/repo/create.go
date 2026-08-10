package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"time"

	"github.com/sirupsen/logrus"
)

// CreateRepoEL will create a local YUM repository in the specified directory
func CreateRepoEL(dir string) error {
	dir, err := absoluteRepoDir(dir)
	if err != nil {
		return err
	}
	logrus.Infof("create %s %s repo in %s", config.OSVendor, config.OSCode, dir)
	if _, err := exec.LookPath("sow"); err == nil {
		return createRepoWithSow(dir, "el", true)
	}
	return createRepoWithCreaterepo(dir)
}

// CreateRepoDEB will create a local APT repository in the specified directory
func CreateRepoDEB(dir string) error {
	dir, err := absoluteRepoDir(dir)
	if err != nil {
		return err
	}
	logrus.Infof("create %s %s repo in %s", config.OSVendor, config.OSCode, dir)
	if _, err := exec.LookPath("sow"); err == nil {
		return createRepoWithSow(dir, "deb", true)
	}
	return createRepoWithDpkg(dir)
}

// CreateRepoMAC creates a local RPM/DEB repository with SOW on macOS.
func CreateRepoMAC(dir string) error {
	dir, err := absoluteRepoDir(dir)
	if err != nil {
		return err
	}
	logrus.Infof("create %s repo in %s", config.OSVendor, dir)
	if _, err := exec.LookPath("sow"); err != nil {
		return fmt.Errorf("sow not found; repository creation on macOS requires sow in PATH")
	}
	return createRepoWithSow(dir, "macos", false)
}

func absoluteRepoDir(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repository path %q: %w", dir, err)
	}
	return absDir, nil
}

func createRepoWithSow(dir, repoType string, useSudo bool) error {
	logrus.Infof("use sow to create %s repository metadata", repoType)
	// --timeout limits SOW's lock wait, not repository indexing time.
	args := []string{"sow", "create", "--pigsty", "--timeout", "10m", "--", dir}
	run := utils.Command
	if useSudo {
		run = utils.SudoCommand
	}
	if err := run(args); err != nil {
		return fmt.Errorf("failed to create %s repo with sow: %w", repoType, err)
	}
	return verifyRepoComplete(dir)
}

func createRepoWithCreaterepo(dir string) error {
	if _, err := exec.LookPath("createrepo_c"); err != nil {
		return fmt.Errorf("neither sow nor createrepo_c found; install sow from the Pigsty INFRA repository or run: yum install -y createrepo_c")
	}

	script := createRepoCmdEL(dir)
	if config.OSMajor == 7 {
		script = createRepoCmdEL7(dir)
	}
	scriptPath, err := writeTempRepoScript(script)
	if err != nil {
		return err
	}
	logrus.Debugf("generate create el repo tmp script: %s", scriptPath)
	defer os.Remove(scriptPath)

	if err = utils.SudoCommand([]string{"sh", scriptPath}); err != nil {
		return fmt.Errorf("failed to create el repo with createrepo_c: %w", err)
	}
	return verifyRepoComplete(dir)
}

func createRepoWithDpkg(dir string) error {
	if _, err := exec.LookPath("dpkg-scanpackages"); err != nil {
		return fmt.Errorf("neither sow nor dpkg-scanpackages found; install sow from the Pigsty INFRA repository or run: apt install -y dpkg-dev")
	}

	scriptPath, err := writeTempRepoScript(createRepoCmdDEB(dir))
	if err != nil {
		return err
	}
	logrus.Debugf("generate create deb repo tmp script: %s", scriptPath)
	defer os.Remove(scriptPath)

	if err = utils.SudoCommand([]string{"sh", scriptPath}); err != nil {
		return fmt.Errorf("failed to create deb repo with dpkg-scanpackages: %w", err)
	}
	return verifyRepoComplete(dir)
}

func verifyRepoComplete(dir string) error {
	completePath := filepath.Join(dir, "repo_complete")
	info, err := os.Lstat(completePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("repository backend completed without completion marker %s", completePath)
		}
		return fmt.Errorf("failed to inspect repository completion marker %s: %w", completePath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("repository completion marker is not a regular file: %s", completePath)
	}
	logrus.Infof("repo created, check %s", completePath)
	return nil
}

// createRepoCmdEL will create a local YUM repository in the specified directory
func createRepoCmdEL(dir string) string {
	dirQ := utils.ShellQuoteArgs([]string{dir})
	completeQ := utils.ShellQuoteArgs([]string{filepath.Join(dir, "repo_complete")})
	return fmt.Sprintf(`#!/bin/bash
cd -- %s;
rm -rf proj-data*;
rm -rf patroni*3.0.4*;
rm -rf *docs*;
createrepo_c . ;
repo2module -s stable . modules.yaml;
modifyrepo_c --mdtype=modules modules.yaml repodata/;
md5sum *.rpm > %s
	`, dirQ, completeQ)
}

// createRepoCmdEL7 will create a local YUM repository in the specified directory
func createRepoCmdEL7(dir string) string {
	dirQ := utils.ShellQuoteArgs([]string{dir})
	completeQ := utils.ShellQuoteArgs([]string{filepath.Join(dir, "repo_complete")})
	return fmt.Sprintf(`#!/bin/bash
cd -- %s;
rm -f *.i686.rpm;
rm -rf patroni*3.0.4*;
rm -rf *docs*;
createrepo_c . ;
md5sum *.rpm > %s
	`, dirQ, completeQ)
}

// createRepoCmdDEB will create a local APT repository in the specified directory
func createRepoCmdDEB(dir string) string {
	dirQ := utils.ShellQuoteArgs([]string{dir})
	completeQ := utils.ShellQuoteArgs([]string{filepath.Join(dir, "repo_complete")})
	return fmt.Sprintf(`#!/bin/bash
cd -- %s;
rm -f *i386.deb;
rm -rf Packages.gz;
dpkg-scanpackages . /dev/null | gzip -9c > Packages.gz;
md5sum *.deb > %s;
	`, dirQ, completeQ)
}

func writeTempRepoScript(script string) (string, error) {
	tmp, err := os.CreateTemp("", "create_repo_*.sh")
	if err != nil {
		return "", fmt.Errorf("failed to create tmp repo script file: %w", err)
	}
	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to write tmp repo script file %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to close tmp repo script file %s: %w", tmp.Name(), err)
	}
	return tmp.Name(), nil
}

// CreateReposWithResult creates local RPM/DEB repositories and returns a structured Result
// This function is used for YAML/JSON output modes
func CreateReposWithResult(repos []string) *output.Result {
	startTime := time.Now()

	// Check OS support
	if config.OSType != config.DistroEL && config.OSType != config.DistroDEB && config.OSType != config.DistroMAC {
		return output.Fail(output.CodeRepoUnsupportedOS,
			fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
	}

	// Linux keeps the historical /www/pigsty default. macOS defaults to the
	// current directory so local repository creation does not require sudo.
	if len(repos) == 0 {
		repos = []string{defaultCreateRepoDir()}
	}

	// Build OS environment info
	osEnv := &OSEnvironment{
		Code:  config.OSCode,
		Arch:  config.OSArch,
		Type:  config.OSType,
		Major: config.OSMajor,
	}

	// Prepare data structure
	data := &RepoCreateData{
		OSEnv:        osEnv,
		RepoDirs:     repos,
		CreatedRepos: []*CreatedRepoItem{},
		FailedRepos:  []*FailedRepoItem{},
	}

	// Create each repo
	for _, repoDir := range repos {
		item, err := createRepoWithItem(repoDir)
		if err != nil {
			data.FailedRepos = append(data.FailedRepos, &FailedRepoItem{
				Module: repoDir,
				Error:  err.Error(),
				Code:   output.CodeRepoCreateFailed,
			})
		} else {
			data.CreatedRepos = append(data.CreatedRepos, item)
		}
	}

	data.DurationMs = time.Since(startTime).Milliseconds()

	// Determine overall result
	successCount := len(data.CreatedRepos)
	failCount := len(data.FailedRepos)

	if successCount == 0 && failCount > 0 {
		return output.Fail(output.CodeRepoCreateFailed,
			fmt.Sprintf("failed to create all %d repos", failCount)).WithData(data)
	}

	message := fmt.Sprintf("Created %d local repository(s)", successCount)
	result := output.OK(message, data)
	if failCount > 0 {
		result.Detail = fmt.Sprintf("failed: %d repo(s)", failCount)
	}
	return result
}

// createRepoWithItem creates a single local repo and returns CreatedRepoItem
func createRepoWithItem(dirPath string) (*CreatedRepoItem, error) {
	if dirPath == "" {
		dirPath = defaultCreateRepoDir()
	}

	// Check if source directory exists, create if not
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			if config.OSType == config.DistroMAC {
				err = os.MkdirAll(dirPath, 0o755)
			} else {
				err = utils.SudoCommand([]string{"mkdir", "-p", dirPath})
			}
			if err != nil {
				return nil, fmt.Errorf("failed to create repo dir %s: %v", dirPath, err)
			}
		} else {
			return nil, fmt.Errorf("failed to check repo dir %s: %v", dirPath, err)
		}
	}

	var repoType string
	var err error

	switch config.OSType {
	case config.DistroEL:
		repoType = "yum"
		err = CreateRepoEL(dirPath)
	case config.DistroDEB:
		repoType = "apt"
		err = CreateRepoDEB(dirPath)
	case config.DistroMAC:
		repoType = "rpm/deb"
		err = CreateRepoMAC(dirPath)
	default:
		return nil, fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	if err != nil {
		return nil, err
	}

	return &CreatedRepoItem{
		Path:         dirPath,
		RepoType:     repoType,
		CompleteFile: filepath.Join(dirPath, "repo_complete"),
	}, nil
}

func defaultCreateRepoDir() string {
	if config.OSType == config.DistroMAC {
		return "."
	}
	return "/www/pigsty"
}
