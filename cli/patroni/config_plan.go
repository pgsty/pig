package patroni

import (
	"strings"

	"pig/internal/output"
)

// BuildConfigPlan returns a side-effect-free primitive plan for Patroni DCS config changes.
func BuildConfigPlan(action string, kvPairs []string) *output.Plan {
	scope := "patroni"
	setFlag := "-s"
	analysis := PGConfigAnalysis{}
	if action == "pg" {
		scope = "postgresql.parameters"
		setFlag = "-p"
	}

	normalized := normalizeConfigPairs(kvPairs)
	if action == "pg" {
		analysis = AnalyzePGConfigPairs(normalized)
	}
	pairDetail := strings.Join(normalized, ", ")
	if pairDetail == "" {
		pairDetail = "no key=value pairs provided"
	}
	nextActions := configPlanNextActions(action, analysis)
	risks := []string{"DCS configuration mistakes can affect every cluster member."}
	if action == "pg" {
		if analysis.RequiresRestart {
			risks = append(risks, "PostgreSQL restart is required for: "+strings.Join(analysis.RestartParams, ", "))
		} else {
			risks = append(risks, "Some PostgreSQL parameters require reload or restart before they take effect.")
		}
	} else {
		risks = append(risks, "Patroni dynamic configuration changes cluster behavior on every member.")
	}

	return &output.Plan{
		Command:      buildConfigCommand(action, normalized),
		Boundary:     "pt:dcs-config",
		Confirmation: "recommended",
		Actions: []output.Action{
			{Step: 1, Description: "Validate key=value pairs for Patroni dynamic configuration"},
			{Step: 2, Description: "Apply DCS config changes with patronictl edit-config --force"},
			{Step: 3, Description: "Report whether PostgreSQL reload or restart should be considered"},
		},
		Affects: []output.Resource{
			{Type: "dcs_config", Name: scope, Impact: "update", Detail: pairDetail},
		},
		Expected: "Patroni dynamic configuration is updated in DCS; members apply changes according to Patroni/PostgreSQL rules",
		Risks:    risks,
		Preconditions: []output.Check{
			{Name: "config pairs", Status: "planned", Detail: pairDetail},
			{Name: "patroni config", Status: "required", Detail: DefaultConfigPath},
			{Name: "patronictl command", Status: "planned", Detail: "edit-config --force " + setFlag},
		},
		Verifications: []output.Check{
			{Name: "show config", Status: "manual", Detail: "pig pt config show"},
			{Name: "member state", Status: "manual", Detail: "pig pt list"},
		},
		NextActions: nextActions,
	}
}

// ConfigPGNextActions returns machine-readable follow-up commands for a
// PostgreSQL DCS parameter change.
func ConfigPGNextActions(kvPairs []string) []output.NextAction {
	return configPlanNextActions("pg", AnalyzePGConfigPairs(normalizeConfigPairs(kvPairs)))
}

func configPlanNextActions(action string, analysis PGConfigAnalysis) []output.NextAction {
	if action == "pg" && analysis.RequiresRestart {
		return []output.NextAction{
			{Command: "pig pt list", Reason: "check members and pending restart state after config change", Required: false},
			{Command: "pig pt restart --pending", Reason: "apply PostgreSQL postmaster parameters: " + strings.Join(analysis.RestartParams, ", "), Required: true},
		}
	}
	return []output.NextAction{
		{Command: "pig pt list", Reason: "check cluster member state after config change", Required: false},
		{Command: "pig pt config show", Reason: "verify DCS config after change", Required: false},
	}
}

func buildConfigCommand(action string, kvPairs []string) string {
	parts := []string{"pig", "pt", "config", action}
	parts = append(parts, kvPairs...)
	parts = append(parts, "--plan")
	return strings.Join(parts, " ")
}

func normalizeConfigPairs(kvPairs []string) []string {
	if len(kvPairs) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(kvPairs))
	for _, pair := range kvPairs {
		pair = strings.TrimSpace(pair)
		if pair != "" {
			pairs = append(pairs, pair)
		}
	}
	return pairs
}
