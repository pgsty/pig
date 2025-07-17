/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/install"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	installPgVer         int
	installYes           bool
	installNoTranslation bool
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:     "install",
	Short:   "Install packages using native package manager",
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
		pgVer := installPgVer
		if pgVer == 0 {
			// Try to detect from active PostgreSQL installation
			pgVer = 17 // fallback to latest if not detected
		}

		if err := install.InstallPackages(pgVer, args, installYes, installNoTranslation); err != nil {
			logrus.Errorf("failed to install packages: %v", err)
			return nil
		}
		return nil
	},
}

func init() {
	installCmd.Flags().IntVarP(&installPgVer, "version", "v", 0, "specify PostgreSQL major version for package translation")
	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "auto confirm installation")
	installCmd.Flags().BoolVarP(&installNoTranslation, "no-translation", "n", false, "disable package name translation, use names as-is")
}
