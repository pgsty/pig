package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"pig/internal/output"
)

// ValidateKillOptions exposes the same validation used by Kill for plan callers.
func ValidateKillOptions(opts *KillOptions) error {
	return validateKillOptions(opts)
}

// BuildKillPlan returns a side-effect-free primitive plan for pg kill.
func BuildKillPlan(cfg *Config, opts *KillOptions) *output.Plan {
	if opts == nil {
		opts = &KillOptions{}
	}
	killFunc := pickKillFunc(opts)
	sql := buildKillSQL(killFunc, opts)
	filterDetail := killFilterDetail(opts)
	actionVerb := "list matching sessions"
	impact := "inspect"
	expected := "Matching sessions are listed; no backend is terminated"
	if opts.Execute {
		actionVerb = "execute " + killFunc + " on matching sessions"
		impact = "terminate"
		if opts.Cancel {
			impact = "cancel"
			expected = "Matching queries are cancelled on the local PostgreSQL instance"
		} else {
			expected = "Matching sessions are terminated on the local PostgreSQL instance"
		}
	}

	risks := []string{
		"This primitive only targets the local PostgreSQL instance; it does not coordinate Patroni or application traffic.",
	}
	if opts.Execute {
		risks = append(risks,
			"Terminated sessions lose in-flight transactions.",
			"Applications may reconnect immediately unless traffic is drained elsewhere.",
		)
	}

	nextActions := []output.NextAction{
		{Command: "pig pg ps", Reason: "verify remaining local PostgreSQL sessions", Required: false},
	}
	if !opts.Execute {
		nextActions = append([]output.NextAction{
			{Command: buildKillCommand(opts, true, false), Reason: "execute this kill plan after review", Required: true},
		}, nextActions...)
	}

	return &output.Plan{
		Command:      buildKillCommand(opts, false, true),
		Boundary:     "pg:local-instance",
		Confirmation: "recommended",
		Actions: []output.Action{
			{Step: 1, Description: "Validate pg_stat_activity filters"},
			{Step: 2, Description: actionVerb},
			{Step: 3, Description: "Return native psql output for matching sessions"},
		},
		Affects: []output.Resource{
			{Type: "database", Name: "postgres", Impact: "query", Detail: "pg_stat_activity"},
			{Type: "connection", Name: filterDetail, Impact: impact, Detail: killFunc},
		},
		Expected: expected,
		Risks:    risks,
		Preconditions: []output.Check{
			{Name: "filters", Status: "planned", Detail: filterDetail},
			{Name: "sql", Status: "planned", Detail: sql},
			{Name: "boundary", Status: "local-only", Detail: "does not manage Patroni, load balancers, or client draining"},
		},
		Verifications: []output.Check{
			{Name: "session list", Status: "manual", Detail: "pig pg ps"},
		},
		NextActions: nextActions,
	}
}

func buildKillCommand(opts *KillOptions, includeExecute bool, includePlan bool) string {
	parts := []string{"pig", "pg", "kill"}
	if opts == nil {
		if includePlan {
			parts = append(parts, "--plan")
		}
		return strings.Join(parts, " ")
	}
	if opts.Pid > 0 {
		parts = append(parts, "--pid", strconv.Itoa(opts.Pid))
	}
	if opts.User != "" {
		parts = append(parts, "--user", quotePgArg(opts.User))
	}
	if opts.Db != "" {
		parts = append(parts, "--database", quotePgArg(opts.Db))
	}
	if opts.State != "" {
		parts = append(parts, "--state", quotePgArg(opts.State))
	}
	if opts.Query != "" {
		parts = append(parts, "--query", quotePgArg(opts.Query))
	}
	if opts.All {
		parts = append(parts, "--all")
	}
	if opts.Cancel {
		parts = append(parts, "--cancel")
	}
	if includeExecute || opts.Execute {
		parts = append(parts, "--execute")
	}
	if opts.Watch > 0 {
		parts = append(parts, "--watch", strconv.Itoa(opts.Watch))
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

func killFilterDetail(opts *KillOptions) string {
	if opts == nil {
		return "client backends except current session"
	}
	parts := make([]string, 0, 8)
	if opts.Pid > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", opts.Pid))
	}
	if opts.User != "" {
		parts = append(parts, "user="+opts.User)
	}
	if opts.Db != "" {
		parts = append(parts, "database="+opts.Db)
	}
	if opts.State != "" {
		parts = append(parts, "state="+opts.State)
	}
	if opts.Query != "" {
		parts = append(parts, "query~="+opts.Query)
	}
	if opts.All {
		parts = append(parts, "all_backends=true")
	} else if opts.Pid == 0 {
		parts = append(parts, "backend_type=client backend")
	}
	if opts.Cancel {
		parts = append(parts, "mode=cancel")
	} else {
		parts = append(parts, "mode=terminate")
	}
	if opts.Execute {
		parts = append(parts, "execute=true")
	} else {
		parts = append(parts, "execute=false")
	}
	if len(parts) == 0 {
		return "client backends except current session"
	}
	return strings.Join(parts, ", ")
}

func quotePgArg(arg string) string {
	if arg == "" {
		return arg
	}
	if strings.ContainsAny(arg, " \t\n\"'\\") {
		return strconv.Quote(arg)
	}
	return arg
}
