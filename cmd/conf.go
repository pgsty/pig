package cmd

import (
	"os"
	"path/filepath"
	"pig/cli/utils"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	confName           string
	confIP             string
	confVer            string
	confRegion         string
	confSkip           bool
	confProxy          bool
	confNonInteractive bool
)

// A thin wrapper around the configure script
var configureCmd = &cobra.Command{
	Use:     "configure",
	Short:   "Configure Pigsty",
	Aliases: []string{"c"},
	Long: `Configure pigsty with ./configure
https://pigsty.io/docs/setup/install/#configure
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.PigstyHome == "" {
			logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
			os.Exit(1)
		}
		configurePath := filepath.Join(config.PigstyHome, "configure")
		if _, err := os.Stat(configurePath); os.IsNotExist(err) {
			logrus.Errorf("configure script not found: %s", configurePath)
			os.Exit(1)
		}

		cmdArgs := []string{configurePath}
		if confName != "" {
			cmdArgs = append(cmdArgs, "-c", confName)
		}
		if confIP != "" {
			cmdArgs = append(cmdArgs, "-i", confIP)
		}
		if confVer != "" {
			cmdArgs = append(cmdArgs, "-v", confVer)
		}
		if confRegion != "" {
			cmdArgs = append(cmdArgs, "-r", confRegion)
		}
		if confSkip {
			cmdArgs = append(cmdArgs, "-s")
		}
		if confProxy {
			cmdArgs = append(cmdArgs, "-p")
		}
		cmdArgs = append(cmdArgs, args...)
		os.Chdir(config.PigstyHome)
		logrus.Infof("configure with: %s", strings.Join(cmdArgs, " "))
		err := utils.ShellCommand(cmdArgs)
		if err != nil {
			logrus.Errorf("configure execution failed: %v", err)
			os.Exit(1)
		}
		return nil

	},
}

func init() {
	configureCmd.Flags().StringVarP(&confName, "conf", "c", "", "config template name")
	configureCmd.Flags().StringVarP(&confIP, "ip", "i", "", "primary ip address")
	configureCmd.Flags().StringVarP(&confVer, "version", "v", "", "postgres major version")
	configureCmd.Flags().StringVarP(&confRegion, "region", "r", "", "upstream repo region")
	configureCmd.Flags().BoolVarP(&confSkip, "skip", "s", false, "skip ip probe")
	configureCmd.Flags().BoolVarP(&confProxy, "proxy", "p", false, "configure proxy env")
	configureCmd.Flags().BoolVarP(&confNonInteractive, "non-interactive", "n", false, "configure non-interactive")
	rootCmd.AddCommand(configureCmd)
}
