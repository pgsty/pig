/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Plan building for pg restart/stop commands.
*/
package postgres

import (
	"fmt"
	"strings"

	"pig/internal/output"
)

// ============================================================================
// BuildRestartPlan
// ============================================================================

// BuildRestartPlan constructs a structured execution plan for pg restart.
// It shows what will happen without actually executing the restart.
func BuildRestartPlan(cfg *Config, opts *RestartOptions) *output.Plan {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Check current state
	running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)

	// Get shutdown mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}

	// Build the plan
	return buildRestartPlanFromState(dataDir, running, pid, mode)
}

// buildRestartPlanFromState constructs a restart plan from given state.
// This is separated for easier testing.
func buildRestartPlanFromState(dataDir string, running bool, pid int, mode string) *output.Plan {
	actions := buildRestartActions(running, mode)
	affects := buildRestartAffects(dataDir, running, pid)
	expected := buildRestartExpected(dataDir, running)
	risks := buildRestartRisks(running)

	return &output.Plan{
		Command:  buildRestartCommand(mode),
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildRestartActions(running bool, mode string) []output.Action {
	actions := []output.Action{}
	step := 1

	if running {
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Stop PostgreSQL server (mode: %s)", mode),
		})
		step++
	}

	actions = append(actions, output.Action{
		Step:        step,
		Description: "Start PostgreSQL server",
	})

	return actions
}

func buildRestartAffects(dataDir string, running bool, pid int) []output.Resource {
	affects := []output.Resource{}

	// Data directory is always affected
	affects = append(affects, output.Resource{
		Type:   "directory",
		Name:   dataDir,
		Impact: "restart",
		Detail: "data directory",
	})

	// Active connections will be interrupted
	if running {
		affects = append(affects, output.Resource{
			Type:   "service",
			Name:   "postgresql",
			Impact: "restart",
			Detail: fmt.Sprintf("PID %d will be terminated", pid),
		})
		affects = append(affects, output.Resource{
			Type:   "connection",
			Name:   "active sessions",
			Impact: "terminate",
			Detail: "all client connections will be disconnected",
		})
	}

	return affects
}

func buildRestartExpected(dataDir string, running bool) string {
	if running {
		return fmt.Sprintf("PostgreSQL restarted (data_dir: %s)", dataDir)
	}
	return fmt.Sprintf("PostgreSQL started (data_dir: %s)", dataDir)
}

func buildRestartRisks(running bool) []string {
	if !running {
		return nil
	}
	return []string{
		"All active connections will be terminated",
		"In-flight transactions will be rolled back",
		"Write operations will be temporarily unavailable",
	}
}

func buildRestartCommand(mode string) string {
	return fmt.Sprintf("pig pg restart -m %s", mode)
}

// ============================================================================
// BuildStopPlan
// ============================================================================

// BuildStopPlan constructs a structured execution plan for pg stop.
// It shows what will happen without actually executing the stop.
func BuildStopPlan(cfg *Config, opts *StopOptions) *output.Plan {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Check current state
	running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)

	// Get shutdown mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}

	// Build the plan
	return buildStopPlanFromState(dataDir, running, pid, mode)
}

// buildStopPlanFromState constructs a stop plan from given state.
// This is separated for easier testing.
func buildStopPlanFromState(dataDir string, running bool, pid int, mode string) *output.Plan {
	actions := buildStopActions(running, mode)
	affects := buildStopAffects(dataDir, running, pid)
	expected := buildStopExpected(dataDir, running)
	risks := buildStopRisks(running, mode)

	return &output.Plan{
		Command:  buildStopCommand(mode),
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildStopActions(running bool, mode string) []output.Action {
	if !running {
		// Instance already stopped, no actions needed
		return []output.Action{}
	}

	return []output.Action{
		{
			Step:        1,
			Description: fmt.Sprintf("Stop PostgreSQL server (mode: %s)", mode),
		},
	}
}

func buildStopAffects(dataDir string, running bool, pid int) []output.Resource {
	affects := []output.Resource{}

	// Data directory
	affects = append(affects, output.Resource{
		Type:   "directory",
		Name:   dataDir,
		Impact: "stop",
		Detail: "data directory",
	})

	if running {
		affects = append(affects, output.Resource{
			Type:   "service",
			Name:   "postgresql",
			Impact: "stop",
			Detail: fmt.Sprintf("PID %d will be terminated", pid),
		})
		affects = append(affects, output.Resource{
			Type:   "connection",
			Name:   "active sessions",
			Impact: "terminate",
			Detail: "all client connections will be disconnected",
		})
	}

	return affects
}

func buildStopExpected(dataDir string, running bool) string {
	if !running {
		return fmt.Sprintf("PostgreSQL already stopped (data_dir: %s)", dataDir)
	}
	return fmt.Sprintf("PostgreSQL stopped (data_dir: %s)", dataDir)
}

func buildStopRisks(running bool, mode string) []string {
	if !running {
		return nil
	}

	risks := []string{
		"All active connections will be terminated",
		"Write operations will become unavailable",
	}

	switch mode {
	case "smart":
		risks = append(risks, "Server waits for clients to disconnect (may take time)")
	case "immediate":
		risks = append(risks, "Immediate shutdown may require recovery on next start")
	}

	return risks
}

func buildStopCommand(mode string) string {
	return fmt.Sprintf("pig pg stop -m %s", mode)
}
