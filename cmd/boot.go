package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/utils"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	bootRegion  string
	bootPackage string
	booKeep     bool
)

// A thin wrapper around the bootstrap script
var bootCmd = &cobra.Command{
	Use:     "boot",
	Short:   "Bootstrap Pigsty",
	Aliases: []string{"b", "bootstrap"},
	Long: `Bootstrap pigsty with ./bootstrap script
https://pigsty.io/docs/setup/offline/#bootstrap
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unexpected argument: %v", args)
		}
		if config.PigstyHome == "" {
			logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
			os.Exit(1)
		}
		bootstrapPath := filepath.Join(config.PigstyHome, "bootstrap")
		if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
			logrus.Errorf("bootstrap script not found: %s", bootstrapPath)
			os.Exit(1)
		}

		cmdArgs := []string{bootstrapPath}
		if bootRegion != "" {
			cmdArgs = append(cmdArgs, "-r", bootRegion)
		}
		if bootPackage != "" {
			cmdArgs = append(cmdArgs, "-p", bootPackage)
		}
		if booKeep {
			cmdArgs = append(cmdArgs, "-k")
		}
		cmdArgs = append(cmdArgs, args...)
		os.Chdir(config.PigstyHome)
		logrus.Infof("bootstrap with: %s", strings.Join(cmdArgs, " "))
		err := utils.ShellCommand(cmdArgs)
		if err != nil {
			logrus.Errorf("bootstrap execution failed: %v", err)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	bootCmd.Flags().StringVarP(&bootRegion, "region", "r", "", "default,china,europe,...")
	bootCmd.Flags().StringVarP(&bootPackage, "path", "p", "", "offline package path")
	bootCmd.Flags().BoolVarP(&booKeep, "keep", "k", false, "keep existing repo")
	rootCmd.AddCommand(bootCmd)
}
