/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import "github.com/spf13/cobra"

func postgresPreRun(cmd *cobra.Command, args []string) error {
	if err := initAll(); err != nil {
		return err
	}
	applyStructuredOutputSilence(cmd)
	return nil
}

func addPlanFlags(cmd *cobra.Command, plan *bool, yes *bool) {
	cmd.PersistentFlags().BoolVar(plan, "plan", false, "show execution plan without running")
	cmd.PersistentFlags().BoolVar(plan, "dry-run", false, "alias for --plan")
	cmd.PersistentFlags().BoolVarP(yes, "yes", "y", false, "skip confirmation prompt")
}
