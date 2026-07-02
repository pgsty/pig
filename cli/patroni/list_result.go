/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pt list structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pig/internal/output"
)

// PtListResultData contains Patroni cluster member list in a simplified, agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
type PtListResultData struct {
	Cluster string            `json:"cluster" yaml:"cluster"`
	Members []PtMemberSummary `json:"members" yaml:"members"`
}

// PtMemberSummary represents a single Patroni cluster member.
type PtMemberSummary struct {
	Member               string `json:"member" yaml:"member"`
	Host                 string `json:"host" yaml:"host"`
	Role                 string `json:"role" yaml:"role"`
	State                string `json:"state" yaml:"state"`
	TL                   int    `json:"tl" yaml:"tl"`
	Lag                  *int   `json:"lag" yaml:"lag"` // null for leader
	PendingRestart       bool   `json:"pending_restart,omitempty" yaml:"pending_restart,omitempty"`
	PendingRestartReason string `json:"pending_restart_reason,omitempty" yaml:"pending_restart_reason,omitempty"`
}

// Text returns a human-readable representation of the list result data.
func (d *PtListResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	if d.Cluster != "" {
		sb.WriteString(fmt.Sprintf("Cluster: %s\n", d.Cluster))
	}
	sb.WriteString(fmt.Sprintf("Members: %d\n", len(d.Members)))
	for _, m := range d.Members {
		lagStr := "null"
		if m.Lag != nil {
			lagStr = fmt.Sprintf("%d MB", *m.Lag)
		}
		sb.WriteString(fmt.Sprintf("  %-20s %-15s %-15s %-10s TL=%d Lag=%s",
			m.Member, m.Host, m.Role, m.State, m.TL, lagStr))
		if m.PendingRestart {
			sb.WriteString(" PendingRestart=true")
			if m.PendingRestartReason != "" {
				sb.WriteString(" Reason=")
				sb.WriteString(m.PendingRestartReason)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// PatroniListEntry represents the raw JSON output from patronictl list -f json.
// Note: patronictl uses PascalCase keys and "Lag in MB" with spaces. Lag and
// pending restart are loosely typed: patronictl renders them as numbers,
// booleans, or strings ("unknown", "*") depending on member state.
type PatroniListEntry struct {
	Cluster              string      `json:"Cluster"`
	Member               string      `json:"Member"`
	Host                 string      `json:"Host"`
	Role                 string      `json:"Role"`
	State                string      `json:"State"`
	TL                   int         `json:"TL"`
	LagInMB              interface{} `json:"Lag in MB"`
	PendingRestart       interface{} `json:"Pending restart"`
	PendingRestartReason string      `json:"Pending restart reason"`
}

// ListResult creates a structured result for pt list command.
// It executes patronictl list -f json and returns parsed cluster member data.
func ListResult(dbsu string, cluster string) *output.Result {
	binPath, err := patroniLookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH")
	}
	if _, err := patroniStat(DefaultConfigPath); err != nil && os.IsNotExist(err) {
		return output.Fail(output.CodePtConfigNotFound,
			fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath))
	}

	args := buildListResultArgs(binPath, cluster)
	jsonOutput, err := patroniDBSUCommandOutput(dbsu, args)
	if err != nil {
		if isPermissionDenied(err, jsonOutput) {
			return output.Fail(output.CodePtPermDenied, "Permission denied executing patronictl list").
				WithDetail(commandErrorDetail(jsonOutput, err))
		}
		if isConfigNotFound(err, jsonOutput) {
			return output.Fail(output.CodePtConfigNotFound,
				fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath)).
				WithDetail(commandErrorDetail(jsonOutput, err))
		}
		if !isServiceRunning() {
			return output.Fail(output.CodePtNotRunning, "Patroni service is not running").
				WithDetail(commandErrorDetail(jsonOutput, err))
		}
		return output.Fail(output.CodePtListFailed, "Failed to execute patronictl list").
			WithDetail(commandErrorDetail(jsonOutput, err))
	}

	data, err := parsePatroniListJSON(jsonOutput)
	if err != nil {
		return output.Fail(output.CodePtParseFailed, "Failed to parse patronictl list output").
			WithDetail(err.Error())
	}

	// Prefer the explicit argument, then the Cluster column parsed from the
	// patronictl JSON; only fall back to reading the Patroni config file.
	if cluster != "" {
		data.Cluster = cluster
	} else if data.Cluster == "" {
		data.Cluster = getClusterName(dbsu)
	}

	return output.OK("Patroni cluster members retrieved", data)
}

func buildListResultArgs(binPath string, cluster string) []string {
	args := []string{binPath, "-c", DefaultConfigPath, "list"}
	if cluster != "" {
		args = append(args, cluster)
	}
	return append(args, "-f", "json")
}

// parsePatroniListJSON parses the JSON output from patronictl list -f json.
func parsePatroniListJSON(jsonStr string) (*PtListResultData, error) {
	var entries []PatroniListEntry
	if err := json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		return nil, fmt.Errorf("failed to parse patronictl list JSON: %w", err)
	}

	data := &PtListResultData{
		Members: make([]PtMemberSummary, 0, len(entries)),
	}

	for _, e := range entries {
		if data.Cluster == "" {
			data.Cluster = e.Cluster
		}
		data.Members = append(data.Members, PtMemberSummary{
			Member:               e.Member,
			Host:                 e.Host,
			Role:                 normalizeRole(e.Role),
			State:                e.State,
			TL:                   e.TL,
			Lag:                  parseLagMB(e.LagInMB),
			PendingRestart:       parsePendingRestart(e.PendingRestart),
			PendingRestartReason: e.PendingRestartReason,
		})
	}

	return data, nil
}

// parseLagMB tolerates the loose lag typing of patronictl JSON: numeric lag is
// returned as MB, anything unparseable (leader, "unknown") becomes nil.
func parseLagMB(value interface{}) *int {
	switch v := value.(type) {
	case float64:
		n := int(v)
		return &n
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return &n
		}
	}
	return nil
}

func parsePendingRestart(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		v = strings.TrimSpace(v)
		return v != "" && v != "false" && v != "0"
	case float64:
		return v != 0
	default:
		return false
	}
}

// getClusterName reads the cluster name (scope) from the Patroni config file.
// Returns empty string if the config file cannot be read or parsed.
func getClusterName(dbsu string) string {
	cluster, err := GetClusterName(dbsu)
	if err != nil {
		return ""
	}
	return cluster
}

// parseClusterNameFromConfig extracts a simple top-level `scope:` scalar.
// It intentionally does not implement full YAML; uncommon forms such as block
// scalars and anchors are rejected by validateResolvedClusterName.
func parseClusterNameFromConfig(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("%w in %s", errClusterScopeMissing, DefaultConfigPath)
	}

	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "scope:") {
			continue
		}
		cluster := cleanScopeValue(strings.TrimPrefix(line, "scope:"))
		if cluster == "" {
			return "", fmt.Errorf("%w in %s", errClusterScopeEmpty, DefaultConfigPath)
		}
		if err := validateResolvedClusterName(cluster); err != nil {
			return "", err
		}
		return cluster, nil
	}
	return "", fmt.Errorf("%w in %s", errClusterScopeMissing, DefaultConfigPath)
}

func cleanScopeValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	quote := value[0]
	if quote == '"' || quote == '\'' {
		if end := strings.IndexByte(value[1:], quote); end >= 0 {
			end++
			inner := value[1:end]
			rest := strings.TrimSpace(value[end+1:])
			if rest == "" || strings.HasPrefix(rest, "#") {
				return inner
			}
		}
		return value
	}

	for i := 1; i < len(value); i++ {
		if value[i] == '#' && (value[i-1] == ' ' || value[i-1] == '\t') {
			return strings.TrimSpace(value[:i])
		}
	}
	return value
}
