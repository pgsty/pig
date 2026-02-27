/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Command layer for pig context - environment context snapshot for AI agents.
Business logic is delegated to cli/context package.

ANCS Annotations:

	type: query
	risk: safe
	os_user: current (attempts DBSU privilege escalation for more info)
	idempotent: true
	cost: 500
*/
package cmd

import (
	"fmt"

	"pig/cli/context"
	"pig/cli/ext"
	"pig/internal/config"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// ============================================================================
// Main Command: pig context
// ============================================================================

var moduleFlag string

var contextCmd = &cobra.Command{
	Use:         "context",
	Short:       "Show environment context snapshot",
	Aliases:     []string{"ctx"},
	GroupID:     "pigsty",
	Annotations: ancsAnn("pig context", "query", "volatile", "safe", true, "safe", "none", "current", 500),
	Long: `Collect and display a comprehensive environment context snapshot.

This command gathers information about the current environment including:
  - Host information (hostname, OS, architecture, kernel)
  - PostgreSQL status (running, version, port, role)
  - Patroni cluster status (if installed)
  - pgBackRest backup status (if configured)
  - Installed extensions

The output is designed for AI agents to quickly understand the environment
state in a single call. All components degrade gracefully if unavailable.

Examples:
  pig context              # text output (human-friendly)
  pig context -o json      # JSON output for agents
  pig context -o yaml      # YAML output for agents
  pig context -m postgres  # only postgres (host included by default)
  pig context -m postgres,!host  # exclude host explicitly
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		// Pre-detect PostgreSQL installations
		if err := ext.DetectPostgres(); err != nil {
			logrus.Debugf("DetectPostgres: %v", err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		modules := context.ParseModuleFilter(moduleFlag)
		// Get context result
		result := context.ContextResultWithModules(modules)

		// Structured output mode (YAML/JSON)
		if config.IsStructuredOutput() {
			return handleAuxResult(result)
		}

		// Text mode: use the Text() method on data
		if result != nil && result.Data != nil {
			if data, ok := result.Data.(*context.ContextResultData); ok {
				fmt.Print(data.Text())
				return nil
			}
		}

		// Fallback: just print the result
		return handleAuxResult(result)
	},
}

// ============================================================================
// Command Registration
// ============================================================================

func init() {
	contextCmd.Flags().StringVarP(&moduleFlag, "module", "m", "", "Filter output by module(s): host,postgres,patroni,pgbackrest,extensions (prefix with ! to exclude)")
}
