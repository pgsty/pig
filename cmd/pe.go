/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"fmt"
	"io"
	"pig/cli/postgres"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultExporterPort = 9630
	defaultExporterHost = "127.0.0.1"
)

type exporterOptions struct {
	host string
	port int
}

var peCmd = newPgExporterCommand()

func newPgExporterCommand() *cobra.Command {
	opts := &exporterOptions{}
	cmd := &cobra.Command{
		Use:         "pg_exporter",
		Short:       "Manage pg_exporter and metrics",
		Annotations: ancsAnn("pig pg_exporter", "query", "stable", "safe", true, "safe", "none", "current", 100),
		Aliases:     []string{"pe", "pgexp", "pgexporter"},
		GroupID:     "pigsty",
		Long: `Manage pg_exporter and access PostgreSQL metrics.

pg_exporter is the Prometheus exporter for PostgreSQL metrics.

  pig pe get                     get all pg_ prefixed metrics
  pig pe list                    list available metric types
  pig pe stat                    show exporter statistics
  pig pe reload                  reload exporter configuration`,
	}

	cmd.PersistentFlags().StringVar(&opts.host, "host", "", "pg_exporter host (default: 127.0.0.1)")
	cmd.PersistentFlags().IntVarP(&opts.port, "port", "p", 0, "pg_exporter port (default: 9630)")
	cmd.AddCommand(
		newGetCommand(opts),
		newListCommand(opts),
		newReloadCommand(opts),
		newStatCommand(opts),
	)
	return cmd
}

func (opts *exporterOptions) url(path string) string {
	host := opts.host
	if host == "" {
		host = defaultExporterHost
	}
	port := opts.port
	if port == 0 {
		port = defaultExporterPort
	}
	return fmt.Sprintf("http://%s:%d%s", host, port, path)
}

func (opts *exporterOptions) params() map[string]interface{} {
	return map[string]interface{}{
		"host": opts.host,
		"port": opts.port,
	}
}

func newGetCommand(opts *exporterOptions) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get all PostgreSQL metrics",
		Annotations: ancsAnn("pig pg_exporter get", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLegacyStructured(output.MODULE_PE, "pig pg_exporter get", args, opts.params(), func() error {
				url := opts.url("/metrics")
				postgres.PrintHint([]string{"curl", url})
				body, err := getExporterBody(url)
				if err != nil {
					return fmt.Errorf("failed to fetch metrics: %w", err)
				}

				for _, line := range strings.Split(string(body), "\n") {
					if strings.HasPrefix(line, "pg_") || strings.HasPrefix(line, "# HELP pg_") || strings.HasPrefix(line, "# TYPE pg_") {
						fmt.Println(line)
					}
				}
				return nil
			})
		},
	}
}

func newListCommand(opts *exporterOptions) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List metric types",
		Annotations: ancsAnn("pig pg_exporter list", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLegacyStructured(output.MODULE_PE, "pig pg_exporter list", args, opts.params(), func() error {
				body, err := getExporterBody(opts.url("/metrics"))
				if err != nil {
					return fmt.Errorf("failed to fetch metrics: %w", err)
				}

				seen := make(map[string]bool)
				for _, line := range strings.Split(string(body), "\n") {
					if !strings.HasPrefix(line, "# HELP pg_") {
						continue
					}
					parts := strings.SplitN(line, " ", 4)
					if len(parts) < 3 || seen[parts[2]] {
						continue
					}
					seen[parts[2]] = true
					fmt.Println(line)
				}
				return nil
			})
		},
	}
}

func newReloadCommand(opts *exporterOptions) *cobra.Command {
	return &cobra.Command{
		Use:         "reload",
		Short:       "Reload pg_exporter configuration",
		Annotations: ancsAnn("pig pg_exporter reload", "action", "volatile", "restricted", true, "low", "none", "current", 1000),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLegacyStructured(output.MODULE_PE, "pig pg_exporter reload", args, opts.params(), func() error {
				url := opts.url("/reload")
				postgres.PrintHint([]string{"curl", url})
				body, err := getExporterBody(url)
				if err != nil {
					return fmt.Errorf("failed to reload: %w", err)
				}
				fmt.Println(string(body))
				return nil
			})
		},
	}
}

func newStatCommand(opts *exporterOptions) *cobra.Command {
	return &cobra.Command{
		Use:         "stat",
		Short:       "Show pg_exporter statistics",
		Annotations: ancsAnn("pig pg_exporter stat", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLegacyStructured(output.MODULE_PE, "pig pg_exporter stat", args, opts.params(), func() error {
				body, err := getExporterBody(opts.url("/stat"))
				if err != nil {
					return fmt.Errorf("failed to get stats: %w", err)
				}
				fmt.Println(string(body))
				return nil
			})
		},
	}
}

func getExporterBody(url string) ([]byte, error) {
	resp, err := utils.DefaultClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
