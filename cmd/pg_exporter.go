/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"pig/cli/postgres"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// pig pg_exporter (pe) - Manage pg_exporter metrics
// ============================================================================

const (
	DefaultExporterPort = 9630
	DefaultExporterHost = "127.0.0.1"
)

var (
	peHost string
	pePort int
)

func runPeLegacy(command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructured(output.MODULE_PE, command, args, params, fn)
}

// getExporterURL returns the base URL for pg_exporter
func getExporterURL(path string) string {
	host := peHost
	if host == "" {
		host = DefaultExporterHost
	}
	port := pePort
	if port == 0 {
		port = DefaultExporterPort
	}
	return fmt.Sprintf("http://%s:%d%s", host, port, path)
}

var peCmd = &cobra.Command{
	Use:   "pg_exporter",
	Short: "Manage pg_exporter and metrics",
	Annotations: map[string]string{
		"name":       "pig pg_exporter",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Aliases: []string{"pe", "pgexp", "pgexporter"},
	GroupID: "pigsty",
	Long: `Manage pg_exporter and access PostgreSQL metrics.

pg_exporter is the Prometheus exporter for PostgreSQL metrics.

  pig pe get                     get all pg_ prefixed metrics
  pig pe list                    list available metric types
  pig pe stat                    show exporter statistics
  pig pe reload                  reload exporter configuration`,
}

var peGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get all PostgreSQL metrics",
	Annotations: map[string]string{
		"name":       "pig pg_exporter get",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "5000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPeLegacy("pig pg_exporter get", args, map[string]interface{}{
			"host": peHost,
			"port": pePort,
		}, func() error {
			url := getExporterURL("/metrics")
			postgres.PrintHint([]string{"curl", url})
			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("failed to fetch metrics: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			// Filter lines starting with pg_
			lines := strings.Split(string(body), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "pg_") || strings.HasPrefix(line, "# HELP pg_") || strings.HasPrefix(line, "# TYPE pg_") {
					fmt.Println(line)
				}
			}
			return nil
		})
	},
}

var peListCmd = &cobra.Command{
	Use:   "list",
	Short: "List metric types",
	Annotations: map[string]string{
		"name":       "pig pg_exporter list",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "5000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPeLegacy("pig pg_exporter list", args, map[string]interface{}{
			"host": peHost,
			"port": pePort,
		}, func() error {
			url := getExporterURL("/metrics")
			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("failed to fetch metrics: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			// Extract unique metric names with HELP
			seen := make(map[string]bool)
			lines := strings.Split(string(body), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "# HELP pg_") {
					parts := strings.SplitN(line, " ", 4)
					if len(parts) >= 3 {
						name := parts[2]
						if !seen[name] {
							seen[name] = true
							fmt.Println(line)
						}
					}
				}
			}
			return nil
		})
	},
}

var peReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload pg_exporter configuration",
	Annotations: map[string]string{
		"name":       "pig pg_exporter reload",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "1000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPeLegacy("pig pg_exporter reload", args, map[string]interface{}{
			"host": peHost,
			"port": pePort,
		}, func() error {
			url := getExporterURL("/reload")
			postgres.PrintHint([]string{"curl", url})
			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("failed to reload: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			fmt.Println(string(body))
			return nil
		})
	},
}

var peStatCmd = &cobra.Command{
	Use:   "stat",
	Short: "Show pg_exporter statistics",
	Annotations: map[string]string{
		"name":       "pig pg_exporter stat",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "5000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPeLegacy("pig pg_exporter stat", args, map[string]interface{}{
			"host": peHost,
			"port": pePort,
		}, func() error {
			url := getExporterURL("/stat")
			postgres.PrintHint([]string{"curl", url})
			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("failed to get stats: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			fmt.Println(string(body))
			return nil
		})
	},
}

func init() {
	// Global flags
	peCmd.PersistentFlags().StringVar(&peHost, "host", "", "pg_exporter host (default: 127.0.0.1)")
	peCmd.PersistentFlags().IntVarP(&pePort, "port", "p", 0, "pg_exporter port (default: 9630)")

	// Register subcommands
	peCmd.AddCommand(peGetCmd)
	peCmd.AddCommand(peListCmd)
	peCmd.AddCommand(peReloadCmd)
	peCmd.AddCommand(peStatCmd)
}
