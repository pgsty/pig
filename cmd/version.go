package cmd

import (
	"fmt"
	"pig/internal/config"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Show pig version info",
	Aliases: []string{"v"},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pig version %s %s/%s\n", config.PigVersion, config.GOOS, config.GOARCH)
		fmt.Printf("build: %s %s %s\n", config.Branch, config.Revision, config.BuildDate)
	},
}
