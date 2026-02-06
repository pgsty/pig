/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pt failover structured output result and DTO.
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

// PtFailoverResultData contains failover execution result in an agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
type PtFailoverResultData struct {
	Command   string `json:"command" yaml:"command"`
	Output    string `json:"output" yaml:"output"`
	Candidate string `json:"candidate,omitempty" yaml:"candidate,omitempty"`
	Force     bool   `json:"force" yaml:"force"`
}

// Text returns a human-readable representation of the failover result data.
func (d *PtFailoverResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Command: %s\n", d.Command))
	if d.Candidate != "" {
		sb.WriteString(fmt.Sprintf("Candidate: %s\n", d.Candidate))
	}
	sb.WriteString(fmt.Sprintf("Force: %v\n", d.Force))
	if d.Output != "" {
		sb.WriteString(fmt.Sprintf("Output:\n%s\n", d.Output))
	}
	return sb.String()
}

// FailoverResult executes patronictl failover and returns a structured result.
// It requires --force (opts.Force=true) since structured output mode cannot handle
// interactive confirmation prompts.
func FailoverResult(dbsu string, opts *FailoverOptions) *output.Result {
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
		return output.Fail(output.CodePtFailoverNeedForce,
			"failover requires --force (-f) flag in structured output mode")
	}

	// 4. Build command arguments
	args := buildFailoverResultArgs(binPath, opts)

	// 5. Execute and capture output
	cmdOutput, err := utils.DBSUCommandOutput(dbsu, args)

	data := &PtFailoverResultData{
		Command:   strings.Join(args, " "),
		Output:    strings.TrimSpace(cmdOutput),
		Candidate: opts.Candidate,
		Force:     opts.Force,
	}

	if err != nil {
		if isPermissionDenied(err, cmdOutput) {
			return output.Fail(output.CodePtPermDenied,
				"Permission denied executing patronictl failover").
				WithDetail(commandErrorDetail(cmdOutput, err)).WithData(data)
		}
		return output.Fail(output.CodePtFailoverFailed,
			"Failover failed").WithDetail(commandErrorDetail(cmdOutput, err)).WithData(data)
	}

	return output.OK("Failover completed successfully", data)
}

// buildFailoverResultArgs builds the patronictl failover command arguments
// for structured output mode (always includes --force).
func buildFailoverResultArgs(binPath string, opts *FailoverOptions) []string {
	args := []string{binPath, "-c", DefaultConfigPath, "failover", "--force"}
	if opts == nil {
		return args
	}
	if opts.Candidate != "" {
		args = append(args, "--candidate", opts.Candidate)
	}
	return args
}
