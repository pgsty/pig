/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pt list structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"pig/internal/output"

	"gopkg.in/yaml.v3"
)

// PtListResultData contains Patroni cluster member list in a simplified, agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
type PtListResultData struct {
	Cluster string            `json:"cluster" yaml:"cluster"`
	Members []PtMemberSummary `json:"members" yaml:"members"`
}

// PtMemberSummary represents a single Patroni cluster member.
type PtMemberSummary struct {
	Cluster              string `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Group                *int   `json:"group,omitempty" yaml:"group,omitempty"`
	Member               string `json:"member" yaml:"member"`
	Host                 string `json:"host" yaml:"host"`
	Role                 string `json:"role" yaml:"role"`
	State                string `json:"state" yaml:"state"`
	TL                   int    `json:"tl" yaml:"tl"`
	Lag                  *int   `json:"lag" yaml:"lag"` // null for leader
	ReceiveLSN           string `json:"receive_lsn,omitempty" yaml:"receive_lsn,omitempty"`
	ReceiveLag           *int   `json:"receive_lag,omitempty" yaml:"receive_lag,omitempty"`
	ReplayLSN            string `json:"replay_lsn,omitempty" yaml:"replay_lsn,omitempty"`
	ReplayLag            *int   `json:"replay_lag,omitempty" yaml:"replay_lag,omitempty"`
	PendingRestart       bool   `json:"pending_restart,omitempty" yaml:"pending_restart,omitempty"`
	PendingRestartReason string `json:"pending_restart_reason,omitempty" yaml:"pending_restart_reason,omitempty"`
	ScheduledRestart     string `json:"scheduled_restart,omitempty" yaml:"scheduled_restart,omitempty"`
	Tags                 any    `json:"tags,omitempty" yaml:"tags,omitempty"`
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
// Patronictl renders several fields as either numbers or empty/sentinel strings;
// interface fields keep parsing compatible with both 4.1 and older output.
type PatroniListEntry struct {
	Cluster              string      `json:"Cluster"`
	Group                interface{} `json:"Group"`
	Member               string      `json:"Member"`
	Host                 string      `json:"Host"`
	Role                 string      `json:"Role"`
	State                string      `json:"State"`
	TL                   interface{} `json:"TL"`
	LagInMB              interface{} `json:"Lag in MB"`
	ReceiveLSN           interface{} `json:"Receive LSN"`
	ReceiveLag           interface{} `json:"Receive Lag"`
	ReplayLSN            interface{} `json:"Replay LSN"`
	ReplayLag            interface{} `json:"Replay Lag"`
	PendingRestart       interface{} `json:"Pending restart"`
	PendingRestartReason string      `json:"Pending restart reason"`
	ScheduledRestart     interface{} `json:"Scheduled restart"`
	Tags                 interface{} `json:"Tags"`
}

// ListResult creates a structured result for pt list command.
// It executes patronictl list -f json and returns parsed cluster member data.
func ListResult(dbsu string, group *int, clusters ...string) *output.Result {
	binPath, err := patroniLookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH")
	}
	clusters = nonEmptyStrings(clusters)
	args := buildListResultArgs(binPath, group, clusters...)
	jsonOutput, err := patroniDBSUCommandStdout(dbsu, args)
	if err != nil {
		if isPermissionDenied(err, jsonOutput) {
			return output.Fail(output.CodePtPermDenied, "Permission denied executing patronictl list").
				WithDetail(commandErrorDetail(jsonOutput, err))
		}
		if isConfigNotFound(err, jsonOutput) {
			return output.Fail(output.CodePtConfigNotFound,
				fmt.Sprintf("Patroni config not found: %s", configFile())).
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
	if len(clusters) == 1 {
		data.Cluster = clusters[0]
	} else if len(clusters) == 0 && data.Cluster == "" {
		data.Cluster = getClusterName(dbsu)
	}

	return output.OK("Patroni cluster members retrieved", data)
}

func buildListResultArgs(binPath string, group *int, clusters ...string) []string {
	args := patronictlArgs(binPath, "list")
	args = append(args, nonEmptyStrings(clusters)...)
	if group != nil {
		args = append(args, "--group", fmt.Sprint(*group))
	}
	return append(args, "-f", "json")
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
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
	clusters := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Cluster != "" {
			clusters[entry.Cluster] = struct{}{}
		}
	}
	multipleClusters := len(clusters) > 1

	for _, e := range entries {
		if data.Cluster == "" && !multipleClusters {
			data.Cluster = e.Cluster
		}
		receiveLag := parseLagMB(e.ReceiveLag)
		replayLag := parseLagMB(e.ReplayLag)
		data.Members = append(data.Members, PtMemberSummary{
			Cluster:              mapCluster(e.Cluster, multipleClusters),
			Group:                parseLagMB(e.Group),
			Member:               e.Member,
			Host:                 e.Host,
			Role:                 normalizeRole(e.Role),
			State:                e.State,
			TL:                   parseInt(e.TL),
			Lag:                  firstLag(replayLag, receiveLag, parseLagMB(e.LagInMB)),
			ReceiveLSN:           parseString(e.ReceiveLSN),
			ReceiveLag:           receiveLag,
			ReplayLSN:            parseString(e.ReplayLSN),
			ReplayLag:            replayLag,
			PendingRestart:       parsePendingRestart(e.PendingRestart),
			PendingRestartReason: e.PendingRestartReason,
			ScheduledRestart:     parseString(e.ScheduledRestart),
			Tags:                 nonEmptyValue(e.Tags),
		})
	}

	return data, nil
}

func mapCluster(cluster string, multiple bool) string {
	if multiple {
		return cluster
	}
	return ""
}

// parseLagMB tolerates the loose lag typing of patronictl JSON: numeric lag is
// returned as MB, anything unparseable (leader, "unknown") becomes nil.
func parseLagMB(value interface{}) *int {
	if n, ok := parseNumber(value); ok {
		return &n
	}
	return nil
}

func parseInt(value interface{}) int {
	n, _ := parseNumber(value)
	return n
}

func parseNumber(value interface{}) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
	}
	return 0, false
}

func firstLag(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func parseString(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func nonEmptyValue(value interface{}) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
	case map[string]interface{}:
		if len(v) == 0 {
			return nil
		}
	}
	return value
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

// parseClusterNameFromConfig extracts and validates the top-level YAML scope.
func parseClusterNameFromConfig(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("%w in %s", errClusterScopeMissing, configFile())
	}
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return "", fmt.Errorf("%w in %s: %v", errClusterScopeInvalid, configFile(), err)
	}
	value, ok := config["scope"]
	if !ok {
		return "", fmt.Errorf("%w in %s", errClusterScopeMissing, configFile())
	}
	cluster, ok := value.(string)
	if !ok {
		if value == nil {
			return "", fmt.Errorf("%w in %s", errClusterScopeEmpty, configFile())
		}
		return "", fmt.Errorf("%w in %s", errClusterScopeInvalid, configFile())
	}
	if cluster == "" {
		return "", fmt.Errorf("%w in %s", errClusterScopeEmpty, configFile())
	}
	if err := validateResolvedClusterName(cluster); err != nil {
		return "", err
	}
	return cluster, nil
}
