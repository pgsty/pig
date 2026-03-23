/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Command layer for PostgreSQL parameter tuning.
Business logic is delegated to cli/postgres package.
*/
package cmd

import (
	"fmt"
	"pig/cli/postgres"

	"github.com/spf13/cobra"
)

// ============================================================================
// Tune Flags
// ============================================================================

var (
	pgTuneProfile string
	pgTuneCPU     int
	pgTuneMem     int
	pgTuneDisk    int
	pgTuneMaxConn int
	pgTuneShmemR  float64
)

// ============================================================================
// Subcommand: pig pg tune
// ============================================================================

var pgTuneCmd = &cobra.Command{
	Use:     "tune",
	Short:   "Generate optimized PostgreSQL parameters",
	Aliases: []string{"tuning"},
	Args:    cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if pgTuneCPU < 0 {
			return fmt.Errorf("cpu must be >= 0")
		}
		if pgTuneMem < 0 {
			return fmt.Errorf("mem must be >= 0")
		}
		if pgTuneDisk < 0 {
			return fmt.Errorf("disk must be >= 0")
		}
		if pgTuneMaxConn < 0 {
			return fmt.Errorf("max-conn must be >= 0")
		}
		if pgTuneShmemR < 0.1 || pgTuneShmemR > 0.4 {
			return fmt.Errorf("shmem-ratio must be between 0.1 and 0.4, got %.2f", pgTuneShmemR)
		}
		return nil
	},
	Annotations: ancsAnn("pig postgres tune", "action", "volatile", "restricted",
		true, "medium", "recommended", "dbsu", 5000),
	Example: `  pig pg tune                        # auto-detect, oltp profile, output params
  pig pg tune -p olap                 # use olap profile
  pig pg tune -c 8 -m 32768 -d 500   # override hardware detection
  pig pg tune -C 500                  # override max_connections
  pig pg tune -R 0.3                  # override shared_buffers ratio
  pig pg tune -o text                 # text output
  pig pg tune -o json                 # structured output (JSON)
  pig pg tune -o yaml                 # structured output (YAML)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := &postgres.TuneOptions{
				Profile:    pgTuneProfile,
				CPU:        pgTuneCPU,
				MemMB:      pgTuneMem,
				DiskGB:     pgTuneDisk,
				MaxConn:    pgTuneMaxConn,
				ShmemRatio: pgTuneShmemR,
			}
			result := postgres.TuneResult(pgConfig, opts)
			return handleAuxResult(result)
		},
	}

// ============================================================================
// Registration
// ============================================================================

func registerPgTuneCommands() {
	pgTuneCmd.Flags().StringVarP(&pgTuneProfile, "profile", "p", "oltp",
		"tuning profile: oltp, olap, tiny, crit")
	pgTuneCmd.Flags().IntVarP(&pgTuneCPU, "cpu", "c", 0,
		"CPU cores (0 = auto-detect)")
	pgTuneCmd.Flags().IntVarP(&pgTuneMem, "mem", "m", 0,
		"total memory in MB (0 = auto-detect)")
	pgTuneCmd.Flags().IntVarP(&pgTuneDisk, "disk", "d", 0,
		"data disk size in GB (0 = auto-detect)")
	pgTuneCmd.Flags().IntVarP(&pgTuneMaxConn, "max-conn", "C", 0,
		"override max_connections (0 = use default 100)")
	pgTuneCmd.Flags().Float64VarP(&pgTuneShmemR, "shmem-ratio", "R", 0.25,
		"shared_buffers as fraction of memory (0.1-0.4)")

	pgCmd.AddCommand(pgTuneCmd)
}
