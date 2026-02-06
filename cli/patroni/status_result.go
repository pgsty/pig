/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pt status structured output result and DTO.
*/
package patroni

import (
	"fmt"
	"os/exec"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"
)

// PtStatusResultData contains comprehensive Patroni cluster status in an agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
type PtStatusResultData struct {
	Cluster        string            `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Leader         string            `json:"leader,omitempty" yaml:"leader,omitempty"`
	Members        []PtMemberSummary `json:"members,omitempty" yaml:"members,omitempty"`
	Timeline       int               `json:"timeline,omitempty" yaml:"timeline,omitempty"`
	MemberCount    int               `json:"member_count" yaml:"member_count"`
	ServiceRunning bool              `json:"service_running" yaml:"service_running"`
}

// Text returns a human-readable representation of the status result data.
func (d *PtStatusResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	if d.Cluster != "" {
		sb.WriteString(fmt.Sprintf("Cluster: %s\n", d.Cluster))
	}
	sb.WriteString(fmt.Sprintf("Service Running: %v\n", d.ServiceRunning))
	if d.Leader != "" {
		sb.WriteString(fmt.Sprintf("Leader: %s\n", d.Leader))
	}
	if d.Timeline > 0 {
		sb.WriteString(fmt.Sprintf("Timeline: %d\n", d.Timeline))
	}
	sb.WriteString(fmt.Sprintf("Members: %d\n", d.MemberCount))
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

// StatusResult creates a structured result for pt status command.
// It collects comprehensive cluster status: service state, cluster name, members, leader, timeline.
func StatusResult(dbsu string) *output.Result {
	statusData := &PtStatusResultData{}

	// 1. Check patronictl existence
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH").
			WithData(statusData)
	}

	// 2. Check service status
	statusData.ServiceRunning = isServiceRunning()

	// 3. Get cluster name (graceful degradation)
	statusData.Cluster = getClusterName(dbsu)

	// 4. Get cluster member information
	args := []string{binPath, "-c", DefaultConfigPath, "list", "-f", "json"}
	jsonOutput, err := utils.DBSUCommandOutput(dbsu, args)
	if err != nil {
		if isPermissionDenied(err, jsonOutput) {
			return output.Fail(output.CodePtPermDenied, "Permission denied executing patronictl list").
				WithData(statusData).WithDetail(commandErrorDetail(jsonOutput, err))
		}
		if !statusData.ServiceRunning {
			return output.Fail(output.CodePtNotRunning, "Patroni service is not running").
				WithData(statusData).WithDetail(commandErrorDetail(jsonOutput, err))
		}
		return output.Fail(output.CodePtStatusFailed, "Failed to get cluster status").
			WithData(statusData).WithDetail(commandErrorDetail(jsonOutput, err))
	}

	// 5. Parse JSON — reuse parsePatroniListJSON from list_result.go
	listData, err := parsePatroniListJSON(jsonOutput)
	if err != nil {
		return output.Fail(output.CodePtParseFailed, "Failed to parse patronictl output").
			WithData(statusData).WithDetail(err.Error())
	}

	// 6. Build complete status data
	statusData.Members = listData.Members
	statusData.MemberCount = len(listData.Members)
	statusData.Leader, statusData.Timeline = extractLeaderAndTimeline(listData.Members)

	// If service is not running, return state error with partial data
	if !statusData.ServiceRunning {
		return output.Fail(output.CodePtNotRunning, "Patroni service is not running").
			WithData(statusData)
	}

	return output.OK("Patroni cluster status retrieved", statusData)
}

// isServiceRunning checks if the Patroni systemd service is active.
func isServiceRunning() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "patroni")
	return cmd.Run() == nil
}

// extractLeaderAndTimeline extracts the leader name and timeline from a member list.
// Returns empty string and 0 if no leader is found.
func extractLeaderAndTimeline(members []PtMemberSummary) (string, int) {
	for _, m := range members {
		if m.Role == "leader" || m.Role == "standby_leader" {
			return m.Member, m.TL
		}
	}
	return "", 0
}
