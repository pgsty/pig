package cmd

import (
	"fmt"
	"pig/cli/get"
	"pig/cli/license"
	"pig/cli/pgsql"
	"pig/internal/config"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"s", "st"},
	Short:   "Show current pigsty status",
	Long: `Display current pigsty status, including:
    - License information
    - Configuration files
    - Environment variables
    - Network conditions
    - System information`,
	Run: func(cmd *cobra.Command, args []string) {

		logPathStr := "stderr"
		if logPath != "" {
			logPathStr = logPath
		}

		fmt.Println("\nConfiguration ==========")
		fmt.Printf("Version         = %s\n", config.PigVersion)
		fmt.Printf("Log Level       = %s\n", logLevel)
		fmt.Printf("Log Path        = %s\n", logPathStr)
		fmt.Printf("Config File     = %s\n", config.ConfigFile)

		fmt.Println("\nOS Environment =====")
		fmt.Printf("OS Short Code   = %s\n", config.OSCode)
		fmt.Printf("OS Architecture	= %s\n", config.OSArch)
		fmt.Printf("OS Package Type = %s\n", config.OSType)
		fmt.Printf("OS Vendor ID    = %s\n", config.OSVendor)
		fmt.Printf("Version (Major) = %s\n", config.OSVersion)
		fmt.Printf("Version (Full)  = %s\n", config.OSVersionFull)
		fmt.Printf("Version (Code)  = %s\n", config.OSVersionCode)

		fmt.Println("\nPG Environment =====")
		pgsql.DetectInstalledPostgres()
		if len(pgsql.Installs) > 0 {
			for _, v := range pgsql.Installs {
				if v == pgsql.Active {
					fmt.Printf("%s (Active)\n", v.String())
				} else {
					fmt.Printf("%s\n", v.String())
				}
			}
		} else {
			fmt.Println("No PostgreSQL installation found")
		}
		if pgsql.Active != nil {
			fmt.Printf("Postgres (Active) :  %s\n", pgsql.Active.Version)
			fmt.Printf("Binary Path       :  %s\n", pgsql.Active.BinPath)
			fmt.Printf("Library Path      :  %s\n", pgsql.Active.LibPath)
			fmt.Printf("Extension Path    :  %s\n", pgsql.Active.ExtPath)
		}

		fmt.Println("\nPigsty Config =====")
		fmt.Printf("Inventory       = %s\n", config.PigstyConfig)
		fmt.Printf("Pigsty Home     = %s\n", config.PigstyHome)
		fmt.Printf("Pigsty Embedded = %s\n", config.PigstyVersion)
		if license.Manager.Active != nil && license.Manager.Active.Claims != nil {
			// fmt.Println("\n===== License Information =====")
			license.Manager.Hide = true
			license.Manager.DescribeDefault()
		}

		fmt.Println("\nNetwork Conditions =====")
		get.NetworkCondition()
		if !get.InternetAccess {
			fmt.Println("Internet Access = No")
			return
		}
		fmt.Println("Internet Access = Yes")
		fmt.Println("Pigsty Repo     = ", get.Source)
		fmt.Println("Inferred Region = ", get.Region)
		fmt.Println("Latest Pigsty   = ", get.LatestVersion)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
