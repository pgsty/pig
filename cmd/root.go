/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"pig/internal/config"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// log level parameters
var (
	logLevel  string
	logPath   string
	inventory string
	debug     bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pig",
	Short: "Pigsty CLI",
	Long: `pig - the cli for PostgreSQL & Pigsty

Usage:

  repo      manage apt/yum repo  	add | rm | list | set  | update
  ext       manage pg extension  	add | rm | list | info | status
  get       download pigsty      	list | src  | pkg
  init      install pigsty
  status    show pig, os, pg status
  version   show version information
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
	config.InitConfig(inventory)
	return nil
}

// initLogger will init logger according to logLevel and logPath
func initLogger(level string, path string) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
		logrus.Warnf("invalid log level: %q, fall back to default 'INFO': %v", level, err)
	}
	logrus.SetLevel(lvl)

	// write to file if path is not empty
	if path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("fail to open log file %s: %v", path, err)
		}
		logrus.SetOutput(f)
		logrus.Infof("log redirect to: %s", path)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
		logrus.Debugf("File logger init at level %s", lvl.String())
	} else {
		logrus.SetOutput(os.Stderr)
		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "15:04:05",
			FullTimestamp:   true,
		})

		logrus.Debugf("Stderr logger init at level %s", lvl.String())
	}
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	fmt.Println(viper.GetString("all.vars.region"))
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error, fatal, panic")
	rootCmd.PersistentFlags().StringVar(&logPath, "log-path", "", "log file path, terminal by default")
	rootCmd.PersistentFlags().StringVarP(&inventory, "inventory", "i", "", "config inventory path")

	rootCmd.AddGroup(
		&cobra.Group{ID: "pgext", Title: "PostgreSQL Extension Manager"},
		&cobra.Group{ID: "pigsty", Title: "Pigsty Management Commands"},
	)
	rootCmd.AddCommand(
		repoCmd,
		extCmd,
		installCmd,
		getCmd,
		bootCmd,
		configureCmd,
		statusCmd,
		licenseCmd,
		versionCmd,
	)
}
