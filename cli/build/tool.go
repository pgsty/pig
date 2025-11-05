package build

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// essential build tools for different linux distros
var buildTools = map[string][]string{
	"el8": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "rpmdevtools", "dnf-utils", "pgdg-srpm-macros",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel", "postgresql1*-devel", "postgresql1*-server",
	},
	"el9": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "rpmdevtools", "dnf-utils", "pgdg-srpm-macros",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel", "postgresql1*-devel", "postgresql1*-server",
	},
	"el10": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "rpmdevtools", "dnf-utils", "pgdg-srpm-macros",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel", "postgresql1*-devel", "postgresql1*-server",
	},
	"d12": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "debhelper", "devscripts", "fakeroot",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "libxslt1-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev", "postgresql-all", "postgresql-server-dev-all",
	},
	"d13": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "debhelper", "devscripts", "fakeroot",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "libxslt1-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev", "postgresql-all", "postgresql-server-dev-all",
	},
	"u22": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "debhelper", "devscripts", "fakeroot",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "libxslt1-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev", "postgresql-all", "postgresql-server-dev-all",
	},
	"u24": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "debhelper", "devscripts", "fakeroot",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "libxslt1-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev", "postgresql-all", "postgresql-server-dev-all",
	},
}

// InstallBuildTools will install build dependencies for different distributions
func InstallBuildTools(mode string) error {
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

	tools := buildTools[distro]
	if mode == "mini" {
		// Filter out PostgreSQL related packages in mini mode
		filteredTools := make([]string, 0, len(tools))
		for _, tool := range tools {
			if !strings.Contains(strings.ToLower(tool), "postgresql") && !strings.Contains(strings.ToLower(tool), "pgdg") {
				filteredTools = append(filteredTools, tool)
			}
		}
		tools = filteredTools
	}

	installCmds = append(installCmds, tools...)
	logrus.Infof("install build utils: %s", strings.Join(installCmds, " "))
	return utils.SudoCommand(installCmds)
}
