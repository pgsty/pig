package cmd

import (
	"fmt"
	statuscli "pig/cli/status"
	"pig/internal/config"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Show pig version info",
	Annotations: map[string]string{
		"name":       "pig version",
		"type":       "query",
		"volatility": "immutable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Aliases: []string{"v"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.IsStructuredOutput() {
			result := statuscli.GetVersionResult()
			return handleAuxResult(result)
		}
		fmt.Printf("pig version %s %s/%s\n", config.PigVersion, config.GOOS, config.GOARCH)
		fmt.Printf("build: %s %s %s\n", config.Branch, config.Revision, config.BuildDate)
		return nil
	},
}
