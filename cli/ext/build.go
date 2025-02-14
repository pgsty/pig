package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// essential build tools for different linux distros
var buildTools = map[string][]string{
	"el8": {
		"make", "cmake", "ninja-build", "pkg-config", "git", "lz4", "unzip", "ncdu", "rsync",
		"rpmdevtools", "dnf-utils", "pgdg-srpm-macros", "postgresql1*-devel", "postgresql1*-server",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel",
	},
	"el9": {
		"make", "cmake", "ninja-build", "pkg-config", "git", "lz4", "unzip", "ncdu", "rsync",
		"rpmdevtools", "dnf-utils", "pgdg-srpm-macros", "postgresql1*-devel", "postgresql1*-server",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel",
	},
	"d12": {
		"make", "cmake", "ninja-build", "pkg-config", "git", "lz4", "unzip", "ncdu", "rsync",
		"debhelper", "devscripts", "fakeroot", "postgresql-all", "postgresql-server-dev-all",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "liblz·4-dev", "libzstd-dev", "libkrb5-dev",
	},
	"u22": {
		"make", "cmake", "ninja-build", "pkg-config", "git", "lz4", "unzip", "ncdu", "rsync",
		"debhelper", "devscripts", "fakeroot", "postgresql-all", "postgresql-server-dev-all",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev",
	},
	"u24": {
		"make", "cmake", "ninja-build", "pkg-config", "git", "lz4", "unzip", "ncdu", "rsync",
		"debhelper", "devscripts", "fakeroot", "postgresql-all", "postgresql-server-dev-all",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev",
	},
}

// BuildEnv will install build dependencies for different distributions
func BuildEnv() error {
	distro := config.OSCode
	logrus.Infof("prepare building environment for distro %s", distro)

	if buildTools[distro] == nil {
		return fmt.Errorf("unsupported distribution: %s", distro)
	}

	var installCmds []string
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, []string{"yum", "install", "-y"}...)
		if config.OSVersion == "8" || config.OSVersion == "9" {
			installCmds[0] = "dnf"
		}
	case config.DistroDEB:
		installCmds = append(installCmds, []string{"apt-get", "install", "-y"}...)
	default:
		return fmt.Errorf("unsupported distribution: %s", config.OSType)
	}
	installCmds = append(installCmds, buildTools[distro]...)
	return utils.SudoCommand(installCmds)
}
