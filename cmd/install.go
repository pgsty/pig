/*
Copyright © 2024 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/get"
	"pig/cli/install"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	path  string
	force bool
)

// installCmd represents the installation command
var installCmd = &cobra.Command{
	Use:     "install",
	Short:   "install pigsty",
	Aliases: []string{"init", "i"},
	Long: `
Description:
    pig install [-p path] [-v version] [-d download_dir] [-f]
    -p | --path    : where to install, ~/pigsty by default
    -f | --force   : force overwrite existing pigsty dir
    -v | --version : pigsty version, embedded by default
    -d | --dir     : download directory, /tmp by default

Examples:
    pig install                   # install to ~/pigsty with embedded version
    pig install -f                # install and OVERWRITE existing pigsty dir
    pig install -p /tmp/pigsty    # install to another location /tmp/pigsty
    pig install -v 3.1            # get & install specific version v3.1.0
`,
	RunE: func(cmd *cobra.Command, args []string) error {

		if version == "" {
			logrus.Debugf("install embedded pigsty %s to %s with force=%v", PigstyVersion, path, force)
			err := install.InstallPigsty(nil, path, force)
			if err != nil {
				logrus.Errorf("failed to install pigsty: %v", err)
			}
			return nil
		}

		// if version is explicit given, always download & install from remote
		get.NetworkCondition()
		if get.AllVersions == nil {
			logrus.Errorf("Fail to get pigsty version list")
			os.Exit(1)
		}
		version = get.CompleteVersion(version)
		if ver := get.IsValidVersion(version); ver == nil {
			logrus.Errorf("invalid pigsty version: %v", version)
			return nil
		} else {
			logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, downloadDir)
			err := get.DownloadSrc(version, downloadDir)
			if err != nil {
				logrus.Errorf("failed to download pigsty src %s: %v", version, err)
				os.Exit(2)
			}
		}

		// downloaded, then extract & install it
		srcTarball, err := install.LoadPigstySrc(filepath.Join(downloadDir, fmt.Sprintf("pigsty-%s.tgz", version)))
		if err != nil {
			logrus.Errorf("failed to load pigsty src %s: %v", version, err)
			os.Exit(3)
		}
		err = install.InstallPigsty(srcTarball, path, force)
		if err != nil {
			logrus.Errorf("failed to install pigsty src %s: %v", version, err)
		}
		return nil
	},
}

func init() {
	installCmd.Flags().StringVarP(&path, "path", "p", "~/pigsty", "target diectory")
	installCmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite existing pigsty (false by default)")
	installCmd.Flags().StringVarP(&version, "version", "v", "", "pigsty version")
	installCmd.Flags().StringVarP(&downloadDir, "dir", "d", "/tmp", "download directory")
	rootCmd.AddCommand(installCmd)
}
