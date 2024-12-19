package cmd

import (
	"pig/cli/boot"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var bootCmd = &cobra.Command{
	Use:   "boot",
	Short: "Bootstrap pigsty",
	Long: `Bootstrap pigsty with ./bootstrap script
https://pigsty.io/docs/setup/offline/#bootstrap
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Info("Starting pigsty bootstrap")
		if err := boot.Bootstrap(); err != nil {
			logrus.Errorf("Bootstrap failed: %v", err)
			return err
		}
		logrus.Info("Bootstrap completed successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(bootCmd)
}
