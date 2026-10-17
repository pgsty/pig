/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Plan building for pg restart/stop commands.
*/
package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"
)

const patroniPgCtlPrimitiveRisk = "This pg_ctl primitive does not coordinate Patroni, DCS, failover, or client routing; use pig pt or pig pitr when Patroni manages this PGDATA"

// ============================================================================
// BuildRestartPlan
// ============================================================================

// BuildRestartPlan constructs a structured execution plan for pg restart.
// It shows what will happen without actually executing the restart.
func BuildRestartPlan(cfg *Config, opts *RestartOptions) *output.Plan {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Check current state
	running, pid, _, probeErr := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)

	// Get shutdown mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}

	plan := buildRestartPlanFromState(dataDir, running, pid, mode)
	plan.Command = buildRestartCommandFor(cfg, opts, true)
	plan.Boundary = pgLocalBoundary
	plan.Confirmation = "recommended"
	plan.Preconditions = append(plan.Preconditions,
		output.Check{Name: "data_dir", Status: "planned", Detail: dataDir},
		postgresRunningCheck(running, pid, probeErr),
		output.Check{Name: "boundary", Status: "local-only", Detail: "does not manage Patroni, DCS, VIP, or client routing"},
	)
	restartNext := output.NextAction{Command: buildRestartCommandFor(cfg, opts, false), Reason: "execute restart after reviewing the plan", Required: running}
	if !running {
		restartNext.Reason = "not running; restart execution will be refused until PostgreSQL is started"
	}
	if probeErr != nil {
		plan.Expected = "PostgreSQL running state is unknown; restart will be refused until it can be determined"
		restartNext.Reason = plan.Expected
	}
	plan.NextActions = append(plan.NextActions,
		restartNext,
		output.NextAction{Command: "pig pt restart --help", Reason: "review native Patroni-managed restart syntax when Patroni owns this instance", Required: false},
	)
	return plan
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

	if !running {
		return actions
	}

	actions = append(actions, output.Action{
		Step:        step,
		Description: fmt.Sprintf("Stop PostgreSQL server (mode: %s)", mode),
	})
	step++

	actions = append(actions, output.Action{
		Step:        step,
		Description: "Start PostgreSQL server",
	})

	return actions
}

func buildRestartAffects(dataDir string, running bool, pid int) []output.Resource {
	return buildLifecycleAffects("restart", dataDir, running, pid)
}

func buildLifecycleAffects(impact, dataDir string, running bool, pid int) []output.Resource {
	affects := []output.Resource{}

	affects = append(affects, output.Resource{
		Type:   "directory",
		Name:   dataDir,
		Impact: impact,
		Detail: "data directory",
	})

	if running {
		affects = append(affects, output.Resource{
			Type:   "service",
			Name:   "postgresql",
			Impact: impact,
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
	return fmt.Sprintf("PostgreSQL is not running; restart will be refused (data_dir: %s; use 'pig pg start' to start a stopped server)", dataDir)
}

func buildRestartRisks(running bool) []string {
	if !running {
		return nil
	}
	return []string{
		"All active connections will be terminated",
		"In-flight transactions will be rolled back",
		"Write operations will be temporarily unavailable",
		patroniPgCtlPrimitiveRisk,
	}
}

func buildRestartCommand(mode string) string {
	return buildRestartCommandFor(nil, &RestartOptions{Mode: mode}, false)
}

func buildRestartCommandFor(cfg *Config, opts *RestartOptions, includePlan bool) string {
	args := appendPgTargetCommandFlags([]string{"pig", "pg", "restart"}, cfg)
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}
	args = append(args, "-m", mode)
	if opts != nil && opts.Timeout > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Timeout))
	}
	if opts != nil && opts.NoWait {
		args = append(args, "--no-wait")
	}
	if opts != nil && opts.Options != "" {
		args = append(args, "-O", opts.Options)
	}
	if includePlan {
		args = append(args, "--plan")
	}
	return utils.ShellQuoteArgs(args)
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
	running, pid, _, probeErr := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)

	// Get shutdown mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}

	plan := buildStopPlanFromState(dataDir, running, pid, mode)
	plan.Command = buildStopCommandFor(cfg, opts, true)
	plan.Boundary = pgLocalBoundary
	plan.Confirmation = "recommended"
	plan.Preconditions = append(plan.Preconditions,
		output.Check{Name: "data_dir", Status: "planned", Detail: dataDir},
		postgresRunningCheck(running, pid, probeErr),
		output.Check{Name: "boundary", Status: "local-only", Detail: "does not manage Patroni, DCS, VIP, or client routing"},
	)
	stopNext := output.NextAction{Command: buildStopCommandFor(cfg, opts, false), Reason: "execute stop after reviewing the plan", Required: running}
	if probeErr != nil {
		plan.Expected = "PostgreSQL running state is unknown; stop will be refused until it can be determined"
		stopNext.Reason = plan.Expected
	}
	plan.NextActions = append(plan.NextActions,
		stopNext,
		output.NextAction{Command: "pig pt pause --help", Reason: "review native Patroni pause syntax when Patroni owns this instance", Required: false},
	)
	return plan
}

func postgresRunningCheck(running bool, pid int, err error) output.Check {
	check := output.Check{Name: "postgres running", Status: boolStatus(running), Detail: "not running"}
	if err != nil {
		check.Status, check.Detail = "unknown", "running state is unknown: "+err.Error()
	} else if running {
		check.Detail = fmt.Sprintf("pid=%d", pid)
	}
	return check
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
	return buildLifecycleAffects("stop", dataDir, running, pid)
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
		patroniPgCtlPrimitiveRisk,
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
	return buildStopCommandFor(nil, &StopOptions{Mode: mode}, false)
}

func buildStopCommandFor(cfg *Config, opts *StopOptions, includePlan bool) string {
	args := appendPgTargetCommandFlags([]string{"pig", "pg", "stop"}, cfg)
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}
	args = append(args, "-m", mode)
	if opts != nil && opts.Timeout > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Timeout))
	}
	if opts != nil && opts.NoWait {
		args = append(args, "--no-wait")
	}
	if includePlan {
		args = append(args, "--plan")
	}
	return utils.ShellQuoteArgs(args)
}

func appendPgTargetCommandFlags(args []string, cfg *Config) []string {
	if cfg == nil {
		return args
	}
	if cfg.PgVersion > 0 {
		args = append(args, "--version", strconv.Itoa(cfg.PgVersion))
	}
	if cfg.PgData != "" {
		args = append(args, "-D", cfg.PgData)
	}
	if cfg.DbSU != "" {
		args = append(args, "--dbsu", cfg.DbSU)
	}
	return args
}
