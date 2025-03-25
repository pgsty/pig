package cmd

import (
	"fmt"
	"pig/internal/config"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Show pig version info",
	Aliases: []string{"v"},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Pig version: %s\n", config.PigVersion)
		fmt.Printf("Go version: %s\n", runtime.Version())
	},
}
