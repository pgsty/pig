/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pt failover structured output result and DTO.
*/
package patroni

import (
	"fmt"
	"os"
	"strings"

	"pig/internal/output"
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
	binPath, err := patroniLookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH")
	}

	// 2. Check config file existence
	if _, err := patroniStat(DefaultConfigPath); os.IsNotExist(err) {
		return output.Fail(output.CodePtConfigNotFound,
			fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath))
	}

	// 3. Structured output mode requires --force (cannot handle interactive prompts)
	if opts == nil || !opts.Force {
		return output.Fail(output.CodePtConfirmationRequired,
			"failover requires --force (-f) flag in structured output mode").
			WithNextActions(
				output.NextAction{Command: "pig pt failover ... --force", Reason: "execute failover after explicit confirmation", Required: true},
				output.NextAction{Command: "pig pt failover ... --plan", Reason: "preview failover without executing", Required: false},
			)
	}

	// 4. Resolve cluster name and build command arguments
	cluster, err := patroniGetClusterName(dbsu)
	if err != nil {
		return clusterNameErrorResult(err)
	}
	if err := validateResolvedClusterName(cluster); err != nil {
		return clusterNameErrorResult(err)
	}
	args := buildFailoverResultArgs(binPath, cluster, opts)

	// 5. Execute and capture output
	cmdOutput, err := patroniDBSUCommandOutput(dbsu, args)

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
func buildFailoverResultArgs(binPath string, cluster string, opts *FailoverOptions) []string {
	args := []string{binPath, "-c", DefaultConfigPath, "failover", cluster, "--force"}
	if opts == nil {
		return args
	}
	if opts.Candidate != "" {
		args = append(args, "--candidate", opts.Candidate)
	}
	return args
}
