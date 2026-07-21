/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pt status structured output result and DTO.
*/
package patroni

import (
	"fmt"
	"os/exec"
	"strings"

	"pig/internal/output"
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
	statusData := &PtStatusResultData{ServiceRunning: isServiceRunning()}
	statusData.Cluster = getClusterName(dbsu)

	listResult := ListResult(dbsu, nil)
	if !listResult.Success {
		listResult.Data = statusData
		if listResult.Code == output.CodePtListFailed {
			if !statusData.ServiceRunning {
				listResult.Code = output.CodePtNotRunning
				listResult.Message = "Patroni service is not running"
			} else {
				listResult.Code = output.CodePtStatusFailed
				listResult.Message = "Failed to get cluster status"
			}
		}
		return listResult
	}
	listData, ok := listResult.Data.(*PtListResultData)
	if !ok || listData == nil {
		return output.Fail(output.CodePtParseFailed, "Failed to parse patronictl output").WithData(statusData)
	}
	if statusData.Cluster == "" {
		statusData.Cluster = listData.Cluster
	}

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
