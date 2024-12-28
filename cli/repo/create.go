package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CreateRepos will create a local YUM/APT repository in the
func CreateRepos(repos ...string) error {
	logrus.Infof("create repo for %s", strings.Join(repos, ","))
	for _, repo := range repos {
		if err := Create(repo); err != nil {
			return err
		}
	}
	return nil
}

// Create will create a local YUM/APT repository in the specified directory
func Create(dirPath string) error {
	if dirPath == "" {
		dirPath = "/www/pigsty"
	}
	// check if source directory exists
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			if err = utils.SudoCommand([]string{"mkdir", "-p", dirPath}); err != nil {
				return fmt.Errorf("failed to create repo dir %s: %v", dirPath, err)
			}
		} else {
			return fmt.Errorf("failed to check repo dir %s: %v", dirPath, err)
		}
	}

	switch config.OSType {
	case config.DistroEL:
		return CreateRepoEL(dirPath)
	case config.DistroDEB:
		return CreateRepoDEB(dirPath)
	}
	return fmt.Errorf("unsupported OS type: %s", config.OSType)
}

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

	// generate tmp file name with timestamp
	tmpFile := fmt.Sprintf("create_repo_%s.sh", time.Now().Format("20240101120000"))
	scriptPath := filepath.Join(os.TempDir(), tmpFile)
	logrus.Debugf("generate create el repo tmp script: %s", scriptPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("failed to write tmp create repo script to %s: %s", scriptPath, err)
	}
	defer os.Remove(scriptPath)

	// run the script with sudo
	err := utils.SudoCommand([]string{"sh", scriptPath})
	if err != nil {
		return fmt.Errorf("failed to create el repo: %v", err)
	} else {
		logrus.Infof("repo created, check %s", filepath.Join(dir, "repo_complete"))
	}
	return nil
}

// CreateRepoDEB will create a local APT repository in the specified directory
func CreateRepoDEB(dir string) error {
	logrus.Infof("create %s %s repo in %s", config.OSVendor, config.OSCode, dir)

	// chekc dpkg-scanpackages exists, if not, hint to install it and exit
	if _, err := exec.LookPath("dpkg-scanpackages"); err != nil {
		return fmt.Errorf("dpkg-scanpackages not found, please install it first: apt install -y dpkg-dev")
	}

	// generate the create repo script to tmp dir, and run it with sudo command
	script := createRepoCmdDEB(dir)

	// generate tmp file name with timestamp
	tmpFile := fmt.Sprintf("create_repo_%s.sh", time.Now().Format("20240101120000"))
	scriptPath := filepath.Join(os.TempDir(), tmpFile)
	logrus.Debugf("generate create deb repo tmp script: %s", scriptPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("failed to write tmp create repo script to %s: %s", scriptPath, err)
	}

	// run the script with sudo
	err := utils.SudoCommand([]string{"sh", scriptPath})
	if err != nil {
		return fmt.Errorf("failed to create deb repo: %v", err)
	} else {
		logrus.Infof("repo created, check %s", filepath.Join(dir, "repo_complete"))
	}
	return nil
}

// createRepoCmdEL will create a local YUM repository in the specified directory
func createRepoCmdEL(dir string) string {
	return fmt.Sprintf(`#!/bin/bash
cd "%s";
rm -rf proj-data*;
rm -rf patroni*3.0.4*;
rm -rf *docs*;
createrepo_c . ;
repo2module -s stable . modules.yaml;
modifyrepo_c --mdtype=modules modules.yaml repodata/;
md5sum *.rpm > "%s"
	`, dir, filepath.Join(dir, "repo_complete"))
}

// createRepoCmdEL7 will create a local YUM repository in the specified directory
func createRepoCmdEL7(dir string) string {
	return fmt.Sprintf(`#!/bin/bash
cd "%s";
rm -f *.i686.rpm;
rm -rf patroni*3.0.4*;
rm -rf *docs*;
createrepo_c . ;
md5sum *.rpm > "%s"
	`, dir, filepath.Join(dir, "repo_complete"))
}

// createRepoCmdDEB will create a local APT repository in the specified directory
func createRepoCmdDEB(dir string) string {
	return fmt.Sprintf(`#!/bin/bash
cd "%s";
rm -f *i386.deb;
rm -rf Packages.gz;
dpkg-scanpackages . /dev/null | gzip -9c > Packages.gz;
md5sum *.deb > "%s";
	`, dir, filepath.Join(dir, "repo_complete"))
}
