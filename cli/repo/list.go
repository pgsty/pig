package repo

import (
	"fmt"
	"os"
	"pig/internal/config"
	"pig/internal/utils"
)

// ListRepo lists the active repository according to the OS type
func ListRepo() error {
	switch config.OSType {
	case config.DistroEL:
		return ListELRepo()
	case config.DistroDEB:
		return ListDebianRepo()
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
}

// ListELRepo lists the RHEL OS family repository
func ListELRepo() error {
	if err := RpmPrecheck(); err != nil {
		return err
	}
	fmt.Println("\n======== ls /etc/yum.repos.d/ ")
	if err := utils.ShellCommand([]string{"ls", "/etc/yum.repos.d/"}); err != nil {
		return err
	}
	fmt.Println("\n======== yum repolist")
	if err := utils.ShellCommand([]string{"yum", "repolist"}); err != nil {
		return err
	}
	return nil
}

// ListDebianRepo lists the Debian OS family repository
func ListDebianRepo() error {
	if err := DebPrecheck(); err != nil {
		return err
	}

	fmt.Println("\n======== ls /etc/apt/sources.list.d/")
	if err := utils.ShellCommand([]string{"ls", "/etc/apt/sources.list.d/"}); err != nil {
		return err
	}

	// also check /etc/apt/sources.list if exists and non-empty, print it
	if fileInfo, err := os.Stat("/etc/apt/sources.list"); err == nil {
		if fileInfo.Size() > 0 {
			fmt.Println("\n======== /etc/apt/sources.list")
			if err := utils.ShellCommand([]string{"cat", "/etc/apt/sources.list"}); err != nil {
				return err
			}
		}
	}

	fmt.Println("\n===== [apt-cache policy] =========================")
	if err := utils.ShellCommand([]string{"apt-cache", "policy"}); err != nil {
		return err
	}
	return nil
}
