package cmd

import (
	"fmt"
	"strings"

	"pig/cli/pgbackrest"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// Info Commands
// ============================================================================

var pbInfoRawOutput string
var pbInfoSet string
var pbInfoRaw bool

var pbInfoCmd = &cobra.Command{
	Use:         "info",
	Aliases:     []string{"i"},
	Short:       "Show backup repository info",
	Annotations: ancsAnn("pig pgbackrest info", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
	Long: `Display detailed information about the backup repository including
all backup sets, recovery window, WAL archive status, and backup list.

By default, displays a parsed and formatted view of backup information including:
  - Recovery window (earliest to latest recovery point)
  - WAL archive range
  - LSN range
  - Backup list with type, duration, size, and WAL range

Use --raw/-R for original pgbackrest output format.
Use --raw-output/-O to control raw output format (text/json).
Use -o json/yaml for structured output (Result wrapper with pgbackrest native JSON in data).`,
	Example: `
  pig pb info                      # detailed formatted output
  pig pb info -o json              # structured JSON output
  pig pb info -o yaml              # structured YAML output
  pig pb info -R                   # raw pgbackrest text output
  pig pb info --raw --raw-output json  # raw JSON output (pgbackrest native)
  pig pb info --set 20250101-*     # show specific backup set`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// raw-output only applies in --raw mode
		if !pbInfoRaw && strings.TrimSpace(pbInfoRawOutput) != "" {
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePbInvalidInfoParams, "--raw-output can only be used with --raw"),
				)
			}
			return fmt.Errorf("--raw-output can only be used with --raw")
		}

		// Raw mode: pass through to pgbackrest directly
		if pbInfoRaw {
			rawOutput, err := resolvePbInfoRawOutput()
			if err != nil {
				return err
			}
			return pgbackrest.Info(pbConfig, &pgbackrest.InfoOptions{
				Output: rawOutput,
				Set:    pbInfoSet,
				Raw:    true,
			})
		}

		// Structured output mode: use InfoResult
		if config.IsStructuredOutput() {
			result := pgbackrest.InfoResult(pbConfig, &pgbackrest.InfoOptions{
				Set: pbInfoSet,
			})
			return handleAuxResult(result)
		}

		// Text mode: use original Info function
		return pgbackrest.Info(pbConfig, &pgbackrest.InfoOptions{
			Set: pbInfoSet,
			Raw: false,
		})
	},
}

var pbLsCmd = &cobra.Command{
	Use:     "ls [type]",
	Aliases: []string{"l", "list"},
	Short:   "List backups, repositories, or stanzas",
	Annotations: mergeAnn(
		ancsAnn("pig pgbackrest ls", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
		map[string]string{
			"args.type.desc": "resource type to list",
			"args.type.type": "enum",
		},
	),
	Long: `List resources in the backup repository.

Types:
  backup  - List all backup sets (default)
  repo    - List configured repositories from config file
  stanza  - List all stanzas (aliases: cluster, cls)

Examples:
  pig pb ls                        # list all backups
  pig pb ls backup                 # list all backups (explicit)
  pig pb ls repo                   # list configured repositories
  pig pb ls stanza                 # list all stanzas`,
	RunE: func(cmd *cobra.Command, args []string) error {
		listType := ""
		if len(args) > 0 {
			listType = args[0]
		}
		return runLegacyStructured(legacyModulePb, "pig pgbackrest ls", args, map[string]interface{}{
			"type": listType,
		}, func() error {
			return pgbackrest.Ls(pbConfig, &pgbackrest.LsOptions{
				Type: listType,
			})
		})
	},
}

func resolvePbInfoRawOutput() (string, error) {
	if out := strings.ToLower(strings.TrimSpace(pbInfoRawOutput)); out != "" {
		switch out {
		case "text", "json":
			return out, nil
		default:
			return "", fmt.Errorf("invalid --raw-output value %q, must be text or json", pbInfoRawOutput)
		}
	}

	if !config.IsStructuredOutput() {
		return "", nil
	}
	switch config.OutputFormat {
	case config.OUTPUT_JSON, config.OUTPUT_JSON_PRETTY:
		return "json", nil
	case config.OUTPUT_YAML:
		return "", fmt.Errorf("raw mode does not support YAML output, use JSON or text")
	default:
		return "", nil
	}
}
