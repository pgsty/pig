package cmd

import (
	"fmt"
	"pig/cli/ext"
	"pig/cli/get"
	"pig/cli/license"
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "info"},
	Short:   "Show Environment Status",
	Long: `Display current pigsty status info, including:
    - Configuration
    - OS Environment
	- PG Environment
    - Network Conditions
`,
	Run: func(cmd *cobra.Command, args []string) {
		logPathStr := "stderr"
		if logPath != "" {
			logPathStr = logPath
		}
		padding := 50
		fmt.Println(utils.PadHeader("Configuration", padding))
		utils.PadKV("Pig Version", config.PigVersion)
		utils.PadKV("Pig Config", config.ConfigFile)
		utils.PadKV("Log Level", logLevel)
		utils.PadKV("Log Path", logPathStr)

		fmt.Println("\n" + utils.PadHeader("OS Environment", padding))
		utils.PadKV("OS Distro Code", config.OSCode)
		utils.PadKV("OS OSArch", config.OSArch)
		utils.PadKV("OS Package Type", config.OSType)
		utils.PadKV("OS Vendor ID", config.OSVendor)
		utils.PadKV("OS Version", config.OSVersion)
		utils.PadKV("OS Version Full", config.OSVersionFull)
		utils.PadKV("OS Version Code", config.OSVersionCode)

		fmt.Println("\n" + utils.PadHeader("PG Environment", padding))
		ext.PostgresInstallSummary()

		fmt.Println("\n" + utils.PadHeader("Pigsty Environment", padding))
		if config.PigstyConfig != "" {
			utils.PadKV("Inventory Path", config.PigstyConfig)
		} else {
			utils.PadKV("Inventory Path", "Not Found")
		}
		if config.PigstyHome != "" {
			utils.PadKV("Pigsty Home", config.PigstyHome)
		} else {
			utils.PadKV("Pigsty Home", "Not Found")
		}
		if license.Manager.Active != nil && license.Manager.Active.Claims != nil {
			fmt.Printf("Active License:\n")
			license.Manager.Hide = true
			license.Manager.DescribeDefault()
		}

		fmt.Println("\n" + utils.PadHeader("Network Conditions", padding))
		get.Details = true
		get.NetworkCondition()
	},
}
