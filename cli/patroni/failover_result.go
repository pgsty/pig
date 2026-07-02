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

// FailoverResult executes patronictl failover and returns a structured
// result. Confirmation is owned by the cmd-layer gate (B04); patronictl
// always receives --force and never prompts.
func FailoverResult(dbsu string, opts *FailoverOptions) *output.Result {
	if opts == nil {
		opts = &FailoverOptions{}
	}

	// Patroni's REST API only performs failover to an explicit candidate; the
	// cmd layer validates this too, but keep the invariant at the API boundary.
	if opts.Candidate == "" {
		return output.Fail(output.GenericParamError(output.MODULE_PT),
			"failover requires an explicit candidate").
			WithDetail("set FailoverOptions.Candidate (--candidate <member>)")
	}

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

	// 3. Resolve cluster name and build command arguments
	cluster, err := resolveClusterName(dbsu, "failover")
	if err != nil {
		return clusterNameErrorResult(err)
	}
	args := buildFailoverResultArgs(binPath, cluster, opts)

	// 4. Execute and capture output
	cmdOutput, err := patroniDBSUCommandOutput(dbsu, args)

	data := &PtFailoverResultData{
		Command:   strings.Join(args, " "),
		Output:    strings.TrimSpace(cmdOutput),
		Candidate: opts.Candidate,
		Force:     true, // no-prompt invariant (B04): patronictl always runs --force
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
