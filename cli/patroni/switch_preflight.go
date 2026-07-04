package patroni

import (
	"fmt"
	"strings"

	"pig/internal/output"
)

// SwitchPreflight is the topology snapshot required before switchover/failover.
type SwitchPreflight struct {
	Cluster    string            `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Leader     string            `json:"leader,omitempty" yaml:"leader,omitempty"`
	Candidates []string          `json:"candidates,omitempty" yaml:"candidates,omitempty"`
	Paused     bool              `json:"paused" yaml:"paused"`
	Members    []PtMemberSummary `json:"members,omitempty" yaml:"members,omitempty"`
}

// LoadSwitchPreflight reads the current Patroni topology and pause flag using
// the same structured list/config surfaces exposed by `pig pt list/config`.
func LoadSwitchPreflight(dbsu string) (*SwitchPreflight, *output.Result) {
	listResult := ListResult(dbsu, "")
	if listResult == nil {
		return nil, output.Fail(output.GenericOpFailed(output.MODULE_PT), "Patroni topology preflight failed").
			WithDetail("nil list result")
	}
	if !listResult.Success {
		return nil, listResult
	}
	listData, ok := listResult.Data.(*PtListResultData)
	if !ok || listData == nil {
		return nil, output.Fail(output.CodePtParseFailed, "Failed to parse Patroni topology preflight").
			WithDetail(fmt.Sprintf("unexpected list result data %T", listResult.Data))
	}

	configResult := ConfigShowResult(dbsu)
	if configResult == nil {
		return nil, output.Fail(output.GenericOpFailed(output.MODULE_PT), "Patroni pause preflight failed").
			WithDetail("nil config result")
	}
	if !configResult.Success {
		return nil, configResult
	}
	configData, ok := configResult.Data.(*PtConfigResultData)
	if !ok || configData == nil {
		return nil, output.Fail(output.CodePtParseFailed, "Failed to parse Patroni pause preflight").
			WithDetail(fmt.Sprintf("unexpected config result data %T", configResult.Data))
	}

	return BuildSwitchPreflight(listData, configData), nil
}

// BuildSwitchPreflight derives the operator-facing switch summary from already
// parsed Patroni list/config data.
func BuildSwitchPreflight(listData *PtListResultData, configData *PtConfigResultData) *SwitchPreflight {
	state := &SwitchPreflight{}
	if listData != nil {
		state.Cluster = listData.Cluster
		state.Members = append([]PtMemberSummary(nil), listData.Members...)
		for _, member := range listData.Members {
			role := normalizeRole(member.Role)
			if isLeaderRole(role) && state.Leader == "" {
				state.Leader = member.Member
				continue
			}
			if isSwitchCandidate(role, member.State) {
				state.Candidates = append(state.Candidates, member.Member)
			}
		}
	}
	if configData != nil {
		state.Paused = configPauseEnabled(configData.Raw)
	}
	return state
}

func isLeaderRole(role string) bool {
	return role == "leader" || role == "standby_leader"
}

func isSwitchCandidate(role string, state string) bool {
	if role == "" || isLeaderRole(role) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "running", "streaming":
		return true
	default:
		return false
	}
}

func configPauseEnabled(raw map[string]interface{}) bool {
	for _, key := range []string{"pause", "paused", "Pause", "Paused", "pause_mode", "Pause mode"} {
		if truthy(raw[key]) {
			return true
		}
	}
	return false
}

func truthy(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "t", "true", "y", "yes", "on", "paused":
			return true
		default:
			return false
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}
