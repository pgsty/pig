/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pt list structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"

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
	Member string `json:"member" yaml:"member"`
	Host   string `json:"host" yaml:"host"`
	Role   string `json:"role" yaml:"role"`
	State  string `json:"state" yaml:"state"`
	TL     int    `json:"tl" yaml:"tl"`
	Lag    *int   `json:"lag" yaml:"lag"` // null for leader
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
		sb.WriteString(fmt.Sprintf("  %-20s %-15s %-15s %-10s TL=%d Lag=%s\n",
			m.Member, m.Host, m.Role, m.State, m.TL, lagStr))
	}
	return sb.String()
}

// PatroniListEntry represents the raw JSON output from patronictl list -f json.
// Note: patronictl uses PascalCase keys and "Lag in MB" with spaces.
type PatroniListEntry struct {
	Member  string `json:"Member"`
	Host    string `json:"Host"`
	Role    string `json:"Role"`
	State   string `json:"State"`
	TL      int    `json:"TL"`
	LagInMB *int   `json:"Lag in MB"`
}

// PatroniYAMLConfig represents the minimal config needed to extract scope (cluster name).
type PatroniYAMLConfig struct {
	Scope string `yaml:"scope"`
}

// ListResult creates a structured result for pt list command.
// It executes patronictl list -f json and returns parsed cluster member data.
func ListResult(dbsu string) *output.Result {
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH")
	}
	if _, err := os.Stat(DefaultConfigPath); err != nil && os.IsNotExist(err) {
		return output.Fail(output.CodePtConfigNotFound,
			fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath))
	}

	args := []string{binPath, "-c", DefaultConfigPath, "list", "-f", "json"}
	jsonOutput, err := utils.DBSUCommandOutput(dbsu, args)
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

	// Try to get cluster name from config
	data.Cluster = getClusterName(dbsu)

	return output.OK("Patroni cluster members retrieved", data)
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
		data.Members = append(data.Members, PtMemberSummary{
			Member: e.Member,
			Host:   e.Host,
			Role:   normalizeRole(e.Role),
			State:  e.State,
			TL:     e.TL,
			Lag:    e.LagInMB,
		})
	}

	return data, nil
}

// getClusterName reads the cluster name (scope) from the Patroni config file.
// Returns empty string if the config file cannot be read or parsed.
func getClusterName(dbsu string) string {
	content, err := os.ReadFile(DefaultConfigPath)
	if err != nil {
		// Try reading with DBSU privileges
		if dbsu == "" {
			dbsu = utils.GetDBSU("")
		}
		contentStr, err := utils.DBSUCommandOutput(dbsu, []string{"cat", DefaultConfigPath})
		if err != nil {
			return ""
		}
		content = []byte(contentStr)
	}
	return parseClusterNameFromYAML(string(content))
}

// parseClusterNameFromYAML extracts the scope field from Patroni YAML config content.
func parseClusterNameFromYAML(content string) string {
	if content == "" {
		return ""
	}
	var cfg PatroniYAMLConfig
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return ""
	}
	return cfg.Scope
}
