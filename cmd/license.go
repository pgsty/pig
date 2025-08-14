/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/license"
	"pig/internal/config"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	dateFormat = "2006-01-02"
)

var (
	// License issue flags
	issueKey   string
	issueBy    string
	issueStart string
	issueType  string
	issueMonth int
	issueNode  int
)

// licenseCmd represents the top-level `license` command
var licenseCmd = &cobra.Command{
	Use:     "license",
	Short:   "Manage Pigsty Licenses",
	Aliases: []string{"lic", "l"},
	Hidden:  true,
	Long: `Description:
    $ pig license status
    $ pig license verify <jwt|path>
    $ pig license issue [-mnbst] <aud>
	$ pig license history
`,
}

// licenseStatusCmd shows the current license status
var licenseStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show pigsty license status",
	Aliases: []string{"st", "s"},
	RunE: func(cmd *cobra.Command, args []string) error {
		lic := viper.GetString("license")
		if lic == "" {
			logrus.Warnf("No active license configured")
			return nil
		}
		logrus.Debug("Verifying current license")
		if _, err := license.Manager.Validate(lic); err != nil {
			logrus.Errorf("Failed to verify the current license: %v", err)
			os.Exit(1)
		}
		logrus.Info("License verified successfully, Details:")
		license.Manager.Describe(lic)
		return nil
	},
}

// licenseIssueCmd issues a new license to a specified audience
var licenseIssueCmd = &cobra.Command{
	Use:     "issue <name>",
	Short:   "Issue a new pigsty license",
	Aliases: []string{"i", "iss"},
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Debug("Starting license issuance process")

		// Ensure audience name is provided
		if len(args) != 1 {
			logrus.Error("License audience name not provided")
			return fmt.Errorf("license audience name is required as the first arg")
		}
		issueName := args[0]

		// Ensure private key is provided or fallback to default
		if issueKey == "" {
			defaultPath := filepath.Join(config.HomeDir, ".ssh", "private.pem")
			logrus.Debugf("No private key path specified, checking default path: %s", defaultPath)
			if _, err := os.Stat(defaultPath); err == nil {
				issueKey = defaultPath
				logrus.Infof("Using default private key: %s", issueKey)
			} else {
				logrus.Error("No private key found for issuing license")
				return nil
			}
		}

		startDate, err := time.Parse(dateFormat, issueStart)
		if err != nil {
			logrus.Errorf("Invalid start date format '%s': %v", issueStart, err)
			return fmt.Errorf("invalid date format for %s, should be YYYY-MM-DD", issueStart)
		}

		// Validate license constraints
		if issueNode < 0 {
			logrus.Error("Invalid node limit: must be non-negative")
			return fmt.Errorf("node limit should be non-negative, 0 represents unlimited")
		}
		if issueNode == 0 {
			logrus.Infof("the default node=0 means unlimited nodes, beware")
		}
		if issueMonth < 0 {
			logrus.Error("Invalid month limit: must be non-negative")
			return fmt.Errorf("month limit should be positive, 0 represents unlimited")
		}
		if issueMonth == 0 {
			issueMonth = 1200 // 100-year (permanent) license
			logrus.Debug("No month limit specified. Using 1200 months (100 years)")
		}

		logrus.Infof("Issuing license: Name='%s', Issuer='%s', Type='%s', Start='%s', Months='%d', Nodes='%d'",
			issueName, issueBy, issueType, startDate.Format(dateFormat), issueMonth, issueNode)

		if err = license.Manager.SetPrivateKey(issueKey); err != nil {
			logrus.Errorf("Failed to load private key: %v", err)
			os.Exit(1)
		}

		lic, err := license.Manager.IssueLicense(issueBy, issueName, startDate, issueMonth, issueType, issueNode)
		if err != nil {
			logrus.Errorf("Failed to issue license: %v", err)
			os.Exit(1)
		}

		license.Manager.Describe(lic)
		return nil
	},
}

// licenseVerifyCmd verifies the validity of a given license
var licenseVerifyCmd = &cobra.Command{
	Use:     "verify <string|path>",
	Short:   "Verify a pigsty license",
	Aliases: []string{"v"},
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Debug("Starting license verification process")
		if len(args) != 1 {
			logrus.Error("JWT license string or path not provided")
			return fmt.Errorf("jwt license string|path is required as the first arg")
		}
		lic, err := license.GetLicense(args[0])
		if err != nil {
			logrus.Errorf("Failed to get license: %v", err)
			os.Exit(1)
		}

		if _, err := license.Manager.Validate(lic); err != nil {
			logrus.Errorf("Invalid license: %v", err)
			license.Manager.Describe(lic)
			os.Exit(1)
		}

		license.Manager.Describe(lic)
		logrus.Info("License verified successfully")
		return nil
	},
}

// licenseListCmd displays the license issue history
var licenseListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List of license issue",
	Aliases: []string{"l", "ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(license.Manager.LicenseType())
		logrus.Debug("Reading license history")
		license.ReadHistory()
		return nil
	},
}

// licenseAddCmd adds a license to the configuration file
var licenseAddCmd = &cobra.Command{
	Use:     "add <license>",
	Short:   "Add license to pigsty configuration",
	Aliases: []string{"a"},
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Debug("Starting license add process")
		if len(args) != 1 {
			logrus.Error("License string not provided")
			return fmt.Errorf("license string is required as the first arg")
		}

		// Get and validate the license
		lic, err := license.GetLicense(args[0])
		if err != nil {
			logrus.Errorf("Failed to get license: %v", err)
			return err
		}

		// Validate the license
		if _, err := license.Manager.Validate(lic); err != nil {
			logrus.Errorf("Invalid license: %v", err)
			return err
		}

		// Add license to config
		if err := license.AddLicense(lic); err != nil {
			logrus.Errorf("Failed to add license to config: %v", err)
			return err
		}

		logrus.Infof("License add to %s", config.ConfigFile)
		license.Manager.Describe(lic)
		return nil
	},
}

func init() {
	defaultDate := time.Now().Format(dateFormat)
	licenseIssueCmd.Flags().StringVarP(&issueKey, "key", "k", "", "Private key path")
	licenseIssueCmd.Flags().StringVarP(&issueBy, "by", "b", "pigsty", "License issuer")
	licenseIssueCmd.Flags().StringVarP(&issueStart, "start", "s", defaultDate, "License start date (YYYY-MM-DD)")
	licenseIssueCmd.Flags().StringVarP(&issueType, "type", "t", "pro", "License type")
	licenseIssueCmd.Flags().IntVarP(&issueMonth, "month", "m", 0, "License month limit (0 for unlimited)")
	licenseIssueCmd.Flags().IntVarP(&issueNode, "node", "n", 0, "License node limit (0 for unlimited)")

	licenseCmd.AddCommand(licenseStatusCmd)
	licenseCmd.AddCommand(licenseIssueCmd)
	licenseCmd.AddCommand(licenseVerifyCmd)
	licenseCmd.AddCommand(licenseListCmd)
	licenseCmd.AddCommand(licenseAddCmd)
}
