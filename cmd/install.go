/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"pig/cli/ext"
	"pig/cli/install"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	installPgVer         int
	installPgConfig      string
	installYes           bool
	installNoTranslation bool
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install packages using native package manager",
	Annotations: map[string]string{
		"name":       "pig install",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "30000",
	},
	Aliases: []string{"i", "ins"},
	Long: `pig install - Install packages using native package manager with alias translation

This command acts like a smart wrapper around yum/dnf/apt-get, providing automatic
package name translation for known PostgreSQL extensions and other aliases.

Examples:
  pig install pg_duckdb                # install extension with translation
  pig install postgresql17             # install directly without translation  
  pig install pg17                     # translate pg17 alias to postgresql packages
  pig ins nginx htop                   # install multiple packages
  pig i pg_vector -y                   # auto confirm installation
  pig install somepackage -n           # disable translation, use package name as-is
`,
	Example: `
  pig install pg_duckdb                # install PostgreSQL extension pg_duckdb
  pig install pg17                     # install PostgreSQL 17 kernel packages
  pig install nginx htop vim           # install multiple system packages
  pig install postgresql17-server -y   # auto confirm installation
  pig install unknown-package -n       # disable translation for unknown packages
  pig install pg_vector=1.0.0          # install specific version
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExtLegacy("pig install", args, map[string]interface{}{
			"version":        installPgVer,
			"path":           installPgConfig,
			"yes":            installYes,
			"no_translation": installNoTranslation,
		}, func() error {
			var pgVer int
			if !installNoTranslation {
				probed, err := installProbeVersion()
				if err != nil {
					return err
				}
				pgVer = probed
			}
			if err := install.InstallPackages(pgVer, args, installYes, installNoTranslation); err != nil {
				logrus.Errorf("failed to install packages: %v", err)
				return err
			}
			return nil
		})
	},
}

// installProbeVersion returns the PostgreSQL version to use
func installProbeVersion() (int, error) {
	// check args
	if installPgVer != 0 && installPgConfig != "" {
		return 0, fmt.Errorf("both pg version and pg_config path are specified, please specify only one")
	}

	// detect postgres installation, but don't fail if not found
	err := ext.DetectPostgres()
	if err != nil {
		logrus.Debugf("failed to detect PostgreSQL: %v", err)
	}

	// if pg version is specified, try if we can find the actual installation
	if installPgVer != 0 {
		_, err := ext.GetPostgres(strconv.Itoa(installPgVer))
		if err != nil {
			logrus.Debugf("PostgreSQL installation %d not found: %v , but it's ok", installPgVer, err)
			// if version is explicitly given, we can fallback without any installation
		}
		return installPgVer, nil
	}

	// if pg_config is specified, we must find the actual installation, to get the major version
	if installPgConfig != "" {
		_, err := ext.GetPostgres(installPgConfig)
		if err != nil {
			return 0, fmt.Errorf("failed to get PostgreSQL by pg_config path %s: %w", installPgConfig, err)
		} else {
			return ext.Postgres.MajorVersion, nil
		}
	}

	// if none given, we can fall back to active installation, or if we can't infer the version, we can fallback to no version tabulate
	if ext.Active != nil {
		logrus.Debugf("fallback to active PostgreSQL: %d", ext.Active.MajorVersion)
		ext.Postgres = ext.Active
		return ext.Active.MajorVersion, nil
	} else {
		logrus.Debugf("no active PostgreSQL found, fall back to the latest Major %d", ext.PostgresLatestMajorVersion)
		return ext.PostgresLatestMajorVersion, nil // 18 by default
	}
}

func init() {
	installCmd.Flags().IntVarP(&installPgVer, "version", "v", 0, "specify PostgreSQL major version for package translation")
	installCmd.PersistentFlags().StringVarP(&installPgConfig, "path", "p", "", "specify a postgres by pg_config path")

	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "auto confirm installation")
	installCmd.Flags().BoolVarP(&installNoTranslation, "no-translation", "n", false, "disable package name translation, use names as-is")
}
