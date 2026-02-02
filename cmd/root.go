/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"os"
	"pig/internal/ancs"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// log level parameters
var (
	logLevel     string
	logPath      string
	inventory    string
	pigstyHome   string
	debug        bool
	logFile      *os.File // log file handle, kept open during program lifetime
	outputFormat string   // output format: text, yaml, json
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pig",
	Short: "Postgres Install Guide",
	Long:  `pig - the Linux Package Manager for PostgreSQL`,
	Example: `
  pig repo add -ru            # overwrite existing repo & update cache
  pig install pg17            # install postgresql 17 PGDG package
  pig install pg_duckdb       # install certain postgresql extension
  pig install pgactive -v 18  # install extension for specifc pg major

  check https://pgext.cloud for details
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initAll()
	},
}

func initAll() error {
	if debug {
		logLevel = "debug"
	}
	if err := initLogger(logLevel, logPath); err != nil {
		return err
	}
	config.InitConfig(inventory, pigstyHome)
	initOutputFormat()
	return nil
}

// validateOutputFormat validates and normalizes the output format.
// Returns the normalized format (lowercase) if valid, otherwise returns "text".
func validateOutputFormat(format string) string {
	normalized := strings.ToLower(strings.TrimSpace(format))
	for _, valid := range config.ValidOutputFormats {
		if normalized == valid {
			return normalized
		}
	}
	return config.OUTPUT_TEXT
}

// initOutputFormat validates the outputFormat flag and syncs it to config.OutputFormat.
func initOutputFormat() {
	validated := validateOutputFormat(outputFormat)
	if validated != strings.ToLower(outputFormat) && outputFormat != "" {
		logrus.Warnf("invalid output format %q, using %q", outputFormat, validated)
	}
	config.OutputFormat = validated
}

// initLogger will init logger according to logLevel and logPath
func initLogger(level string, path string) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
		logrus.Warnf("invalid log level %q, using INFO", level)
	}
	logrus.SetLevel(lvl)

	// write to file if path is not empty
	if path != "" {
		// Close previous log file if exists (prevent leak on re-initialization)
		if logFile != nil {
			logFile.Close()
		}

		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %w", path, err)
		}
		logFile = f // Save file handle for later cleanup
		logrus.SetOutput(f)
		logrus.Infof("log output: %s", path)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
		logrus.Debugf("file logger initialized at level %s", lvl.String())
	} else {
		logrus.SetOutput(os.Stderr)
		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "15:04:05",
			FullTimestamp:   true,
		})

		logrus.Debugf("stderr logger initialized at level %s", lvl.String())
	}
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.WithError(err).Error("command execution failed")
		// Preserve subprocess exit codes using ExitCode helper
		os.Exit(utils.ExitCode(err))
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error, fatal, panic")
	rootCmd.PersistentFlags().StringVar(&logPath, "log-path", "", "log file path, terminal by default")
	rootCmd.PersistentFlags().StringVarP(&inventory, "inventory", "i", "", "config inventory path")
	rootCmd.PersistentFlags().StringVarP(&pigstyHome, "home", "H", "", "pigsty home path")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "output format: text, yaml, json, json-pretty")

	// Setup ANCS-aware help function for structured help output
	ancs.SetupHelp(rootCmd)

	rootCmd.AddGroup(
		&cobra.Group{ID: "pgext", Title: "PostgreSQL Extension Manager"},
		&cobra.Group{ID: "pigsty", Title: "Pigsty Management Commands"},
	)
	rootCmd.AddCommand(
		extCmd,
		repoCmd,
		buildCmd,
		installCmd,

		pgCmd,
		patroniCmd,
		pbCmd,
		pitrCmd,
		peCmd,
		doCmd,
		styCmd,

		statusCmd,
		licenseCmd,
		versionCmd,
		updateCmd,
	)
}
