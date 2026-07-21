package patroni

import (
	"strings"

	"pig/internal/output"
	"pig/internal/utils"
)

// BuildSetPlan renders the exact logical Patronictl argv without executing it.
// --yes deliberately has no representation here: a plan never mutates state.
func BuildSetPlan(pairs []SetPair) *output.Plan {
	nativeCommand := utils.ShellQuoteArgs(patronictlArgs("patronictl", BuildSetArgs(pairs, false)...))
	pairTokens := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		pairTokens = append(pairTokens, pair.Token)
	}
	executeParts := append([]string{"pig", "pt", "set"}, pairTokens...)
	executeParts = append(executeParts, "--yes")

	pgPairs := PostgreSQLSetTokens(pairs)
	nextActions := []output.NextAction{{
		Command:  utils.ShellQuoteArgs(executeParts),
		Reason:   "apply the reviewed Patroni configuration change",
		Required: true,
	}}
	nextActions = append(nextActions, SetNextActions(pgPairs)...)

	return &output.Plan{
		Command:      nativeCommand,
		Boundary:     "pt:dcs-config",
		Confirmation: "patronictl",
		Actions: []output.Action{
			{Step: 1, Description: "Use the selected Patronictl configuration"},
			{Step: 2, Description: "Run one ordered patronictl edit-config operation"},
		},
		Affects: []output.Resource{{
			Type:   "dcs_config",
			Name:   "patroni",
			Impact: "update",
			Detail: strings.Join(pairTokens, ", "),
		}},
		Expected: "Patronictl displays and applies one dynamic-configuration diff",
		Risks: []string{
			"DCS configuration mistakes can affect every cluster member.",
			"This plan contains caller-supplied configuration values and may contain sensitive data.",
		},
		Preconditions: []output.Check{
			{Name: "patroni config", Status: "selected", Detail: ConfigFile()},
			{Name: "patronictl command", Status: "planned", Detail: nativeCommand},
		},
		Verifications: []output.Check{{
			Name: "cluster members", Status: "manual", Detail: "pig pt list",
		}},
		NextActions: nextActions,
	}
}

// SetNextActions analyzes only PostgreSQL-routed pairs.
func SetNextActions(pgPairs []string) []output.NextAction {
	if len(pgPairs) == 0 {
		return nil
	}
	analysis := AnalyzePGConfigPairs(pgPairs)
	reason := "check cluster members after the configuration change"
	if analysis.RequiresRestart {
		reason = "identify CLUSTER before applying pending PostgreSQL restart parameters with " +
			"pig pt restart CLUSTER --pending: " + strings.Join(analysis.RestartParams, ", ")
	}
	return []output.NextAction{{Command: "pig pt list", Reason: reason, Required: false}}
}
