package cmd

import (
	"fmt"
	"os"
	"pig/cli/get"
	"pig/cli/license"
	"pig/cli/pgsql"
	"pig/internal/config"
	"text/tabwriter"
	"time"

	"github.com/sirupsen/logrus"

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

		fmt.Println("===== Configuration =====")
		fmt.Printf("Version         = %s\n", config.PigVersion)
		fmt.Printf("Log Level       = %s\n", logLevel)
		fmt.Printf("Log Path        = %s\n", logPathStr)
		fmt.Printf("Config File     = %s\n", config.ConfigFile)

		fmt.Println("\n===== OS Environment =====")
		fmt.Printf("OS Short Code   = %s\n", config.OSCode)
		fmt.Printf("OS Architecture	= %s\n", config.OSArch)
		fmt.Printf("OS Package Type = %s\n", config.OSType)
		fmt.Printf("OS Vendor ID    = %s\n", config.OSVendor)
		fmt.Printf("Version (Major) = %s\n", config.OSVersion)
		fmt.Printf("Version (Full)  = %s\n", config.OSVersionFull)
		fmt.Printf("Version (Code)  = %s\n", config.OSVersionCode)

		fmt.Println("\n===== PG Environment =====")
		pgsql.DetectInstalledPostgres()
		if len(pgsql.Installs) > 0 {
			for _, v := range pgsql.Installs {
				if v == pgsql.Active {
					fmt.Printf("%s (Active)\n", v.String())
				} else {
					fmt.Printf("%s\n", v.String())
				}
			}
		}

		fmt.Println("\n===== Pigsty Config =====")
		fmt.Printf("Inventory       = %s\n", config.PigstyConfig)
		fmt.Printf("Pigsty Home     = %s\n", config.PigstyHome)
		fmt.Printf("Pigsty Embedded = %s\n", config.PigstyVersion)
		if license.Manager.Active != nil && license.Manager.Active.Claims != nil {
			// fmt.Println("\n===== License Information =====")
			license.Manager.Hide = true
			license.Manager.DescribeDefault()
		}

		fmt.Println("\n===== Network Conditions =====")
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

func DectectPG() {
	if err := pgsql.DetectPostgres(); err != nil {
		logrus.Debugf("No PostgreSQL installation detected: %v", err)
	}
	if pgsql.Active == nil {
		return
	}
	fmt.Printf("Detected PostgreSQL Version: %d.%d\n", pgsql.Active.MajorVersion, pgsql.Active.MinorVersion)
	fmt.Printf("PostgreSQL Bin Path: %s\n", pgsql.Active.BinPath)
	fmt.Printf("PostgreSQL Extension Path: %s\n", pgsql.Active.ExtensionPath)

	logrus.Debugf("Detected PostgreSQL %d.%d: bin=%s ext=%s", pgsql.Active.MajorVersion, pgsql.Active.MinorVersion, pgsql.Active.BinPath, pgsql.Active.ExtensionPath)

	pgsql.Active.ScanExtensions()

	// tabulate extensions and shared libraries
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// tabulate extensions
	fmt.Fprintln(table, "Extension\tVersion\tInstalled At\tLibrary\tDescription")
	for _, ext := range pgsql.Active.Extensions {
		description := ext.Description
		if len(description) > 64 {
			description = description[:64]
		}
		library := ""
		if ext.Library != nil {
			library = ext.Library.Path
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", ext.Name, ext.Version, ext.InstalledAt.Format(time.DateTime), library, description)
	}
	table.Flush()

	fmt.Println("\n===== Unmatched Shared Libraries =====")
	for _, lib := range pgsql.Active.UnmatchedLibs {
		fmt.Printf("%s\t%s\t%d\t%s\n", lib.Name, lib.Path, lib.Size, lib.CreatedAt.Format(time.RFC3339))
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
