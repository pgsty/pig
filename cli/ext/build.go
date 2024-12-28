package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// Common build dependencies for different distributions
var (
	commonBuildDeps = []string{
		"gcc", "make", "git", "curl", "wget", "tar", "gzip", "bzip2", "xz",
		"autoconf", "automake", "libtool", "pkg-config", "patch",
	}

	elBuildDeps = []string{
		"postgresql-devel", "postgresql-server-devel",
		"gcc-c++", "clang", "llvm", "cmake", "bison", "flex", "readline-devel",
		"zlib-devel", "openssl-devel", "libxml2-devel", "libxslt-devel",
		"perl-ExtUtils-Embed", "python3-devel", "tcl-devel", "pam-devel",
		"krb5-devel", "openldap-devel", "gettext-devel", "uuid-devel",
	}

	debBuildDeps = []string{
		"postgresql-server-dev-all",
		"g++", "clang", "llvm", "cmake", "bison", "flex", "libreadline-dev",
		"zlib1g-dev", "libssl-dev", "libxml2-dev", "libxslt1-dev",
		"libperl-dev", "python3-dev", "tcl-dev", "libpam0g-dev",
		"libkrb5-dev", "libldap2-dev", "gettext", "uuid-dev",
	}
)

// BuildExtensions installs build dependencies for extensions
func BuildExtensions(pgVer int, names []string, yes bool) error {
	logrus.Debugf("setting up build environment: pgVer=%d, names=%s, yes=%v", pgVer, strings.Join(names, ", "), yes)

	var installCmds []string
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, []string{"yum", "install"}...)
		if config.OSVersion == "8" || config.OSVersion == "9" {
			installCmds[0] = "dnf"
		}
		if yes {
			installCmds = append(installCmds, "-y")
		}
		// Add EL-specific build dependencies
		installCmds = append(installCmds, commonBuildDeps...)
		installCmds = append(installCmds, elBuildDeps...)
		if pgVer != 0 {
			installCmds = append(installCmds,
				fmt.Sprintf("postgresql%d-devel", pgVer),
				fmt.Sprintf("postgresql%d-server-devel", pgVer),
			)
		}

	case config.DistroDEB:
		installCmds = append(installCmds, []string{"apt-get", "install"}...)
		if yes {
			installCmds = append(installCmds, "-y")
		}
		// Add Debian-specific build dependencies
		installCmds = append(installCmds, commonBuildDeps...)
		installCmds = append(installCmds, debBuildDeps...)
		if pgVer != 0 {
			installCmds = append(installCmds, fmt.Sprintf("postgresql-%d-dev", pgVer))
		}

	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	// Add extension-specific build dependencies if specified
	if len(names) > 0 {
		Catalog.LoadAliasMap(config.OSType)
		var pkgNames []string
		for _, name := range names {
			ext, ok := Catalog.ExtNameMap[name]
			if !ok {
				ext, ok = Catalog.ExtAliasMap[name]
			}
			if !ok {
				logrus.Debugf("cannot find '%s' in extension name or alias", name)
				continue
			}

			// Add extension package with -devel suffix for RPM or -dev suffix for DEB
			pkgName := ext.PackageName(pgVer)
			if pkgName == "" {
				logrus.Warnf("no package found for extension %s", ext.Name)
				continue
			}
			logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)

			switch config.OSType {
			case config.DistroEL:
				pkgNames = append(pkgNames, processPkgName(pkgName+"-devel", pgVer)...)
			case config.DistroDEB:
				pkgNames = append(pkgNames, processPkgName(pkgName+"-dev", pgVer)...)
			}
		}
		installCmds = append(installCmds, pkgNames...)
	}

	logrus.Infof("installing build dependencies: %s", strings.Join(installCmds, " "))
	return utils.SudoCommand(installCmds)
}
