/*
Copyright © 2024 Ruohang Feng <rh@vonng.com>
*/
package cmd

//
//import (
//	"os"
//	"pig/cli/get"
//	"pig/cli/license"
//
//	"github.com/sirupsen/logrus"
//	"github.com/spf13/cobra"
//)
//
////var (
////	downloadDir string
////	version     string
////)
//
//// getCmd represents the installation command
//var getCmd = &cobra.Command{
//	Use:     "download",
//	Short:   "Download Pigsty",
//	Aliases: []string{"get", "g", "down"},
//	GroupID: "pigsty",
//	Long: `
//Description:
//    pig download info [-v ver] list available versions [since ver]
//    pig download src  [-v ver] download pigsty source package
//    pig download pkg  [-v ver] download pigsty offline package (pro)
//`,
//}
//
//// getListCmd represents the list command
//var getListCmd = &cobra.Command{
//	Use:     "list",
//	Short:   "get pigsty available versions",
//	Aliases: []string{"l", "info"},
//	RunE: func(cmd *cobra.Command, args []string) error {
//		get.NetworkCondition()
//		if get.AllVersions == nil {
//			logrus.Errorf("Fail to list pigsty versions")
//			os.Exit(1)
//		}
//
//		since := "v3.0.0"
//		if get.IsValidVersion(get.CompleteVersion(version)) != nil {
//			since = version
//		}
//		logrus.Infof("Latest versions since %s ,from %s", get.CompleteVersion(version), get.Source)
//		get.PirntAllVersions(since)
//		return nil
//	},
//}
//
//// getSrcCmd represents the src command
//var getSrcCmd = &cobra.Command{
//	Use:     "src",
//	Short:   "download pigsty source package",
//	Aliases: []string{"s"},
//	RunE: func(cmd *cobra.Command, args []string) error {
//		get.NetworkCondition()
//		if get.AllVersions == nil {
//			logrus.Errorf("Fail to get pigsty version list")
//			os.Exit(1)
//		}
//		if completeVer := get.CompleteVersion(version); completeVer != version {
//			logrus.Debugf("Complete pigsty version from %s to %s", version, completeVer)
//			version = completeVer
//		}
//
//		ver := get.IsValidVersion(version)
//		if ver == nil {
//			logrus.Errorf("Invalid version: %s", version)
//			os.Exit(1)
//		} else {
//			logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, downloadDir)
//		}
//
//		logrus.Debugf("Download pigsty src %s to %s", version, downloadDir)
//		err := get.DownloadSrc(version, downloadDir)
//		if err != nil {
//			logrus.Errorf("failed to download pigsty src: %v", err)
//		}
//		return nil
//	},
//}
//
//// getPkgCmd represents the pkg command
//var getPkgCmd = &cobra.Command{
//	Use:     "pkg",
//	Short:   "download pigsty offline package",
//	Aliases: []string{"p"},
//	RunE: func(cmd *cobra.Command, args []string) error {
//		get.NetworkCondition()
//		if get.AllVersions == nil {
//			logrus.Errorf("Fail to get pigsty version list")
//			os.Exit(1)
//		}
//
//		if completeVer := get.CompleteVersion(version); completeVer != version {
//			logrus.Debugf("Complete pigsty version from %s to %s", version, completeVer)
//			version = completeVer
//		}
//
//		ver := get.IsValidVersion(version)
//		if ver == nil {
//			logrus.Errorf("Invalid version: %s", version)
//			os.Exit(1)
//		}
//
//		if license.Manager.Valid {
//			logrus.Infof("Get pigsty pkg %s from %s to %s", ver.Version, ver.DownloadURL, downloadDir)
//			logrus.Warnf("TBD: download pigsty pkg is not implemented yet")
//		} else {
//			logrus.Errorf("Invalid license, download pigsty pkg is only available on pro version")
//			os.Exit(1)
//		}
//		return nil
//	},
//}
//
//func init() {
//	getSrcCmd.Flags().StringVarP(&version, "version", "v", "", "pigsty src version")
//	getListCmd.Flags().StringVarP(&version, "version", "v", "", "print version since")
//	getPkgCmd.Flags().StringVarP(&version, "version", "v", "", "pigsty pkg version")
//
//	getSrcCmd.Flags().StringVarP(&downloadDir, "dir", "d", "/tmp", "download directory")
//	getPkgCmd.Flags().StringVarP(&downloadDir, "dir", "d", "/tmp", "download directory")
//	getCmd.AddCommand(getListCmd)
//	getCmd.AddCommand(getSrcCmd)
//	getCmd.AddCommand(getPkgCmd)
//}
