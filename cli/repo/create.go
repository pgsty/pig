package repo

import (
	"fmt"
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
	logrus.Infof("create %s %s repo in %s", config.OSVendor, config.OSCode, dir)

	// check createrepo_c exists, if not, hint to install it and exit
	if _, err := exec.LookPath("createrepo_c"); err != nil {
		return fmt.Errorf("createrepo_c not found, please install it first: yum install -y createrepo_c")
	}

	// generate the create repo script to tmp dir, and run it with sudo command
	script := createRepoCmdEL(dir)
	if config.OSMajor == 7 {
		script = createRepoCmdEL7(dir)
	}

	// write repo script to unique temp file
	scriptPath, err := writeTempRepoScript(script)
	if err != nil {
		return err
	}
	logrus.Debugf("generate create el repo tmp script: %s", scriptPath)
	defer os.Remove(scriptPath)

	// run the script with sudo
	if err = utils.SudoCommand([]string{"sh", scriptPath}); err != nil {
		return fmt.Errorf("failed to create el repo: %v", err)
	} else {
		logrus.Infof("repo created, check %s", filepath.Join(dir, "repo_complete"))
	}
	return nil
}

// CreateRepoDEB will create a local APT repository in the specified directory
func CreateRepoDEB(dir string) error {
	logrus.Infof("create %s %s repo in %s", config.OSVendor, config.OSCode, dir)

	// check dpkg-scanpackages exists, if not, hint to install it and exit
	if _, err := exec.LookPath("dpkg-scanpackages"); err != nil {
		return fmt.Errorf("dpkg-scanpackages not found, please install it first: apt install -y dpkg-dev")
	}

	// generate the create repo script to tmp dir, and run it with sudo command
	script := createRepoCmdDEB(dir)

	// write repo script to unique temp file
	scriptPath, err := writeTempRepoScript(script)
	if err != nil {
		return err
	}
	logrus.Debugf("generate create deb repo tmp script: %s", scriptPath)
	defer os.Remove(scriptPath)

	// run the script with sudo
	if err = utils.SudoCommand([]string{"sh", scriptPath}); err != nil {
		return fmt.Errorf("failed to create deb repo: %v", err)
	} else {
		logrus.Infof("repo created, check %s", filepath.Join(dir, "repo_complete"))
	}
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

// CreateReposWithResult creates local YUM/APT repositories and returns a structured Result
// This function is used for YAML/JSON output modes
func CreateReposWithResult(repos []string) *output.Result {
	startTime := time.Now()

	// Check OS support
	if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
		return output.Fail(output.CodeRepoUnsupportedOS,
			fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
	}

	// Use default if no repos specified
	if len(repos) == 0 {
		repos = []string{"/www/pigsty"}
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
		dirPath = "/www/pigsty"
	}

	// Check if source directory exists, create if not
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			if err = utils.SudoCommand([]string{"mkdir", "-p", dirPath}); err != nil {
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
