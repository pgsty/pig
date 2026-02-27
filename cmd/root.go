/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"pig/internal/ancs"
	"pig/internal/config"
	"pig/internal/output"
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
	Annotations: map[string]string{
		"flags.output.choices": "text,yaml,json,json-pretty",
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		return nil
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

	// Ensure structured output runs non-interactively to avoid sudo prompts.
	if config.IsStructuredOutput() && os.Getenv("PIG_NON_INTERACTIVE") == "" {
		_ = os.Setenv("PIG_NON_INTERACTIVE", "1")
	}
}

// applyStructuredOutputSilence toggles Cobra error/usage printing based on
// the parsed global output format.
func applyStructuredOutputSilence(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	structured := config.IsStructuredOutput()
	if root := cmd.Root(); root != nil {
		root.SilenceUsage = structured
		root.SilenceErrors = structured
	}
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
	args := reorderOutputBeforeHelp(os.Args[1:])
	rootCmd.SetArgs(args)
	prepareEarlyOutputSettings(args)

	// Setup ANCS-aware help after all commands are registered to avoid recursion.
	ancs.SetupHelp(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		if exitCode, handled := emitStructuredExecutionError(err, args); handled {
			os.Exit(exitCode)
		}
		if shouldLogExecutionError(err) {
			logrus.WithError(err).Error("command execution failed")
		}
		// Preserve subprocess exit codes using ExitCode helper
		os.Exit(utils.ExitCode(err))
	}
}

func prepareEarlyOutputSettings(args []string) {
	config.OutputFormat = outputFormatFromArgs(args)
	if config.IsStructuredOutput() && os.Getenv("PIG_NON_INTERACTIVE") == "" {
		_ = os.Setenv("PIG_NON_INTERACTIVE", "1")
	}
	applyStructuredOutputSilence(rootCmd)
}

func emitStructuredExecutionError(err error, args []string) (int, bool) {
	if err == nil || !isStructuredOutputRequested(args) {
		return 0, false
	}
	var exitCodeErr *utils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return 0, false
	}

	code := output.CodeSystemCommandFailed
	if isUsageExecutionError(err) {
		code = output.CodeSystemInvalidArgs
	}
	result := output.Fail(code, err.Error())
	if printErr := output.Print(result); printErr != nil {
		return 0, false
	}
	return result.ExitCode(), true
}

func shouldLogExecutionError(err error) bool {
	if err == nil {
		return false
	}
	if !config.IsStructuredOutput() {
		return true
	}
	var exitCodeErr *utils.ExitCodeError
	return !errors.As(err, &exitCodeErr)
}

func isUsageExecutionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "flag needs an argument") ||
		strings.Contains(msg, "accepts ") ||
		strings.Contains(msg, "requires at least") ||
		strings.Contains(msg, "requires at most") ||
		strings.Contains(msg, "requires exactly") ||
		strings.Contains(msg, "invalid argument")
}

func reorderOutputBeforeHelp(args []string) []string {
	if len(args) < 3 {
		return args
	}

	helpIdx := -1
	for i, arg := range args {
		if arg == "--help" || arg == "-h" {
			helpIdx = i
			break
		}
	}
	if helpIdx < 0 {
		return args
	}

	outStart, outEnd := findOutputFlagSpan(args, helpIdx+1)
	if outStart < 0 || outEnd <= outStart {
		return args
	}

	outSeg := append([]string{}, args[outStart:outEnd]...)
	remaining := make([]string, 0, len(args)-len(outSeg))
	remaining = append(remaining, args[:outStart]...)
	remaining = append(remaining, args[outEnd:]...)

	if helpIdx > len(remaining) {
		helpIdx = len(remaining)
	}
	reordered := make([]string, 0, len(args))
	reordered = append(reordered, remaining[:helpIdx]...)
	reordered = append(reordered, outSeg...)
	reordered = append(reordered, remaining[helpIdx:]...)
	return reordered
}

func findOutputFlagSpan(args []string, start int) (int, int) {
	for i := start; i < len(args); i++ {
		arg := args[i]
		if arg == "-o" || arg == "--output" {
			if i+1 < len(args) {
				return i, i + 2
			}
			return -1, -1
		}
		if strings.HasPrefix(arg, "--output=") || strings.HasPrefix(arg, "-o=") {
			return i, i + 1
		}
	}
	return -1, -1
}

func isStructuredOutputRequested(args []string) bool {
	format := outputFormatFromArgs(args)
	switch format {
	case config.OUTPUT_YAML, config.OUTPUT_JSON, config.OUTPUT_JSON_PRETTY:
		return true
	default:
		return false
	}
}

func outputFormatFromArgs(args []string) string {
	format := config.OUTPUT_TEXT
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-o" || arg == "--output":
			if i+1 < len(args) {
				format = validateOutputFormat(args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--output="):
			format = validateOutputFormat(strings.TrimPrefix(arg, "--output="))
		case strings.HasPrefix(arg, "-o="):
			format = validateOutputFormat(strings.TrimPrefix(arg, "-o="))
		}
	}
	return format
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error, fatal, panic")
	rootCmd.PersistentFlags().StringVar(&logPath, "log-path", "", "log file path, terminal by default")
	rootCmd.PersistentFlags().StringVarP(&inventory, "inventory", "i", "", "config inventory path")
	rootCmd.PersistentFlags().StringVarP(&pigstyHome, "home", "H", "", "pigsty home path")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "output format: text, yaml, json, json-pretty")

	rootCmd.AddGroup(
		&cobra.Group{ID: "pgext", Title: "PostgreSQL Extension Manager"},
		&cobra.Group{ID: "pigsty", Title: "Pigsty Management Commands"},
	)
	rootCmd.AddCommand(
		contextCmd,
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
