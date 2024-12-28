package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

var buildTools = map[string][]string{

	"el8": {
		"rpmdevtools", "createrepo_c", "wget", "dnf-utils", "dnf-plugins-core", "sshpass", "modulemd-tools", "ninja-build", "openssl-devel", "pkg-config", "make", "cmake",
		"pgdg-srpm-macros", "postgresql1*-devel", "postgresql1*-server", "readline-devel", "zlib-devel", "lz4", "lz4-devel", "libzstd-devel", "openssl-devel",
	},
	"el9": {
		"rpmdevtools", "createrepo_c", "wget", "dnf-utils", "dnf-plugins-core", "sshpass", "modulemd-tools", "ninja-build", "openssl-devel", "pkg-config", "make", "cmake",
		"pgdg-srpm-macros", "postgresql1*-devel", "postgresql1*-server", "readline-devel", "zlib-devel", "lz4", "lz4-devel", "libzstd-devel", "openssl-devel",
	},
	"d12": {
		"lz4", "unzip", "wget", "patch", "bash", "lsof", "sshpass", "debhelper", "devscripts", "fakeroot", "pkg-config", "make", "cmake", "ncdu", "rsync",
		"postgresql-all", "postgresql-server-dev-all", "libreadline-dev", "flex", "bison", "libxml2-dev", "libxml2-utils", "xsltproc", "libc++-dev", "libc++abi-dev", "libglib2.0-dev", "libtinfo5", "libstdc++-12-dev", "liblz4-dev", "ninja-build",
	},
	"u22": {
		"lz4", "unzip", "wget", "patch", "bash", "lsof", "sshpass", "debhelper", "devscripts", "fakeroot", "pkg-config", "make", "cmake", "ncdu", "rsync",
		"postgresql-all", "postgresql-server-dev-all", "libreadline-dev", "flex", "bison", "libxml2-dev", "libxml2-utils", "xsltproc", "libc++-dev", "libc++abi-dev", "libglib2.0-dev", "libtinfo6", "libstdc++-12-dev", "liblz4-dev", "ninja-build",
	},
	"u24": {
		"lz4", "unzip", "wget", "patch", "bash", "lsof", "sshpass", "debhelper", "devscripts", "fakeroot", "pkg-config", "make", "cmake", "ncdu", "rsync",
		"postgresql-all", "postgresql-server-dev-all", "libreadline-dev", "flex", "bison", "libxml2-dev", "libxml2-utils", "xsltproc", "libc++-dev", "libc++abi-dev", "libglib2.0-dev", "libtinfo6", "libstdc++-12-dev", "liblz4-dev", "ninja-build",
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
