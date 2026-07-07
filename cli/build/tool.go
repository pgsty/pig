package build

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// essential build tools for different linux distros
var buildTools = map[string][]string{
	"el8": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "rpmdevtools", "dnf-utils", "pgdg-srpm-macros",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel",
	},
	"el9": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "rpmdevtools", "dnf-utils", "pgdg-srpm-macros",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "krb5-devel",
	},
	"el10": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "rpmdevtools", "dnf-utils", "pgdg-srpm-macros",
		"readline-devel", "zlib-devel", "libxml2-devel", "lz4-devel", "libzstd-devel", "openssl-devel", "krb5-devel",
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
	"u26": {
		"make", "cmake", "ninja-build", "pkg-config", "lld", "git", "lz4", "unzip", "ncdu", "rsync", "vray", "jq", "debhelper", "devscripts", "fakeroot",
		"libreadline-dev", "zlib1g-dev", "libxml2-dev", "libxslt1-dev", "liblz4-dev", "libzstd-dev", "libkrb5-dev", "postgresql-all", "postgresql-server-dev-all",
	},
}

func postgresELBuildPackages() []string {
	versions := ext.PostgresActiveMajorVersionsAsc()
	packages := make([]string, 0, len(versions)*2)
	for _, version := range versions {
		packages = append(packages,
			fmt.Sprintf("postgresql%d-devel", version),
			fmt.Sprintf("postgresql%d-server", version),
		)
	}
	return packages
}

func postgresBetaBuildPackages() []string {
	switch config.OSType {
	case config.DistroEL:
		return []string{
			fmt.Sprintf("postgresql%d-devel", ext.PostgresBetaMajorVersion),
			fmt.Sprintf("postgresql%d-server", ext.PostgresBetaMajorVersion),
		}
	case config.DistroDEB:
		return []string{
			fmt.Sprintf("postgresql-%d", ext.PostgresBetaMajorVersion),
			fmt.Sprintf("postgresql-server-dev-%d", ext.PostgresBetaMajorVersion),
		}
	default:
		return nil
	}
}

func buildToolInstallCommand(mode string, includeBeta bool) ([]string, error) {
	distro := config.OSCode
	tools, ok := buildTools[distro]
	if !ok {
		return nil, fmt.Errorf("unsupported distribution: %s", distro)
	}

	tools = append([]string(nil), tools...)
	if config.OSType == config.DistroEL {
		tools = append(tools, postgresELBuildPackages()...)
	}
	if includeBeta {
		tools = append(tools, postgresBetaBuildPackages()...)
	}

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

	pkgMgr := ext.PackageManagerCmd()
	switch config.OSType {
	case config.DistroEL, config.DistroDEB:
		if pkgMgr == "" {
			return nil, fmt.Errorf("unsupported distribution: %s", config.OSType)
		}
	default:
		return nil, fmt.Errorf("unsupported distribution: %s", config.OSType)
	}

	installCmds := []string{pkgMgr, "install", "-y"}
	installCmds = append(installCmds, tools...)
	return installCmds, nil
}

// InstallBuildTools will install build dependencies for different distributions.
func InstallBuildTools(mode string) error {
	return InstallBuildToolsWithBeta(mode, false)
}

// InstallBuildToolsWithBeta will install build dependencies with optional beta PostgreSQL packages.
func InstallBuildToolsWithBeta(mode string, includeBeta bool) error {
	distro := config.OSCode
	logrus.Infof("prepare building environment for distro %s", distro)

	installCmds, err := buildToolInstallCommand(mode, includeBeta)
	if err != nil {
		return err
	}
	logrus.Infof("install build utils: %s", strings.Join(installCmds, " "))
	return utils.SudoCommand(installCmds)
}
