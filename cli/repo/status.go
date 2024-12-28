package repo

import (
	"fmt"
	"os"
	"pig/internal/config"
	"pig/internal/utils"
	"runtime"
)

// Status shows the current repo status
func Status() error {
	switch config.OSType {
	case config.DistroEL:
		return StatusEL()
	case config.DistroDEB:
		return StatusDebian()
	default:
		return fmt.Errorf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull)
	}
}

// StatusEL lists the RHEL OS family repository
func StatusEL() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if config.OSType != config.DistroEL { // check if EL distro
		return fmt.Errorf("can not add rpm repo to %s distro", config.OSType)
	}

	utils.PadHeader("ls /etc/yum.repos.d/", 48)
	if err := utils.ShellCommand([]string{"ls", "/etc/yum.repos.d/"}); err != nil {
		return err
	}

	utils.PadHeader("yum repolist", 48)
	if err := utils.ShellCommand([]string{"yum", "repolist"}); err != nil {
		return err
	}
	return nil
}

// StatusDebian lists the Debian OS family repository
func StatusDebian() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if config.OSType != config.DistroDEB { // check if DEB distro
		return fmt.Errorf("can not add deb repo to %s distro", config.OSType)
	}

	utils.PadHeader("ls /etc/apt/sources.list.d/", 48)
	if err := utils.ShellCommand([]string{"ls", "/etc/apt/sources.list.d/"}); err != nil {
		return err
	}

	// also check /etc/apt/sources.list if exists and non-empty, print it
	if fileInfo, err := os.Stat("/etc/apt/sources.list"); err == nil {
		if fileInfo.Size() > 0 {
			utils.PadHeader("/etc/apt/sources.list", 48)
			if err := utils.ShellCommand([]string{"cat", "/etc/apt/sources.list"}); err != nil {
				return err
			}
		}
	}

	utils.PadHeader("apt-cache policy", 48)
	if err := utils.ShellCommand([]string{"apt-cache", "policy"}); err != nil {
		return err
	}
	return nil
}
