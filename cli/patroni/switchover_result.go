/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pt switchover structured output result and DTO.
*/
package patroni

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"
)

// PtSwitchoverResultData contains switchover execution result in an agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
type PtSwitchoverResultData struct {
	Command   string `json:"command" yaml:"command"`
	Output    string `json:"output" yaml:"output"`
	Leader    string `json:"leader,omitempty" yaml:"leader,omitempty"`
	Candidate string `json:"candidate,omitempty" yaml:"candidate,omitempty"`
}

// Text returns a human-readable representation of the switchover result data.
func (d *PtSwitchoverResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Command: %s\n", d.Command))
	if d.Leader != "" {
		sb.WriteString(fmt.Sprintf("Leader: %s\n", d.Leader))
	}
	if d.Candidate != "" {
		sb.WriteString(fmt.Sprintf("Candidate: %s\n", d.Candidate))
	}
	if d.Output != "" {
		sb.WriteString(fmt.Sprintf("Output:\n%s\n", d.Output))
	}
	return sb.String()
}

// SwitchoverResult executes patronictl switchover and returns a structured result.
// It requires --force (opts.Force=true) since structured output mode cannot handle
// interactive confirmation prompts.
func SwitchoverResult(dbsu string, opts *SwitchoverOptions) *output.Result {
	// 1. Check patronictl existence
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH")
	}

	// 2. Check config file existence
	if _, err := os.Stat(DefaultConfigPath); os.IsNotExist(err) {
		return output.Fail(output.CodePtConfigNotFound,
			fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath))
	}

	// 3. Structured output mode requires --force (cannot handle interactive prompts)
	if opts == nil || !opts.Force {
		return output.Fail(output.CodePtSwitchoverNeedForce,
			"switchover requires --force (-f) flag in structured output mode")
	}

	// 4. Build command arguments
	args := buildSwitchoverResultArgs(binPath, opts)

	// 5. Execute and capture output
	cmdOutput, err := utils.DBSUCommandOutput(dbsu, args)

	data := &PtSwitchoverResultData{
		Command:   strings.Join(args, " "),
		Output:    strings.TrimSpace(cmdOutput),
		Leader:    opts.Leader,
		Candidate: opts.Candidate,
	}

	if err != nil {
		if isPermissionDenied(err, cmdOutput) {
			return output.Fail(output.CodePtPermDenied,
				"Permission denied executing patronictl switchover").
				WithDetail(commandErrorDetail(cmdOutput, err)).WithData(data)
		}
		return output.Fail(output.CodePtSwitchoverFailed,
			"Switchover failed").WithDetail(commandErrorDetail(cmdOutput, err)).WithData(data)
	}

	return output.OK("Switchover completed successfully", data)
}

// buildSwitchoverResultArgs builds the patronictl switchover command arguments
// for structured output mode (always includes --force).
func buildSwitchoverResultArgs(binPath string, opts *SwitchoverOptions) []string {
	args := []string{binPath, "-c", DefaultConfigPath, "switchover", "--force"}
	if opts == nil {
		return args
	}
	if opts.Leader != "" {
		args = append(args, "--leader", opts.Leader)
	}
	if opts.Candidate != "" {
		args = append(args, "--candidate", opts.Candidate)
	}
	if opts.Scheduled != "" {
		args = append(args, "--scheduled", opts.Scheduled)
	}
	return args
}
