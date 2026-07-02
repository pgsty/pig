package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const legacyOutputCaptureLimit = 64 * 1024

type legacyCommandData struct {
	Command         string                 `json:"command" yaml:"command"`
	Args            []string               `json:"args,omitempty" yaml:"args,omitempty"`
	Params          map[string]interface{} `json:"params,omitempty" yaml:"params,omitempty"`
	CapturedOutput  string                 `json:"captured_output,omitempty" yaml:"captured_output,omitempty"`
	OutputTruncated bool                   `json:"output_truncated,omitempty" yaml:"output_truncated,omitempty"`
}

type captureResult struct {
	output    string
	truncated bool
	readErr   error
}

type primitiveContractData struct {
	Operation    output.OperationMeta `json:"operation" yaml:"operation"`
	Prechecks    []output.Check       `json:"prechecks,omitempty" yaml:"prechecks,omitempty"`
	StateBefore  interface{}          `json:"state_before,omitempty" yaml:"state_before,omitempty"`
	StateAfter   interface{}          `json:"state_after,omitempty" yaml:"state_after,omitempty"`
	NextActions  []output.NextAction  `json:"next_actions,omitempty" yaml:"next_actions,omitempty"`
	NativeOutput string               `json:"native_output,omitempty" yaml:"native_output,omitempty"`
}

func ancsAnn(name, typ, volatility, parallel string, idempotent bool, risk, confirm, osUser string, cost int) map[string]string {
	return map[string]string{
		"name":       name,
		"type":       typ,
		"volatility": volatility,
		"parallel":   parallel,
		"idempotent": strconv.FormatBool(idempotent),
		"risk":       risk,
		"confirm":    confirm,
		"os_user":    osUser,
		"cost":       strconv.Itoa(cost),
	}
}

func mergeAnn(base map[string]string, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func handleAuxResult(result *output.Result) error {
	if result == nil {
		return fmt.Errorf("nil result")
	}
	if err := output.Print(result); err != nil {
		return err
	}
	if !result.Success {
		return &utils.ExitCodeError{Code: result.ExitCode(), Err: fmt.Errorf("%s", result.Message)}
	}
	return nil
}

// handlePlanOutput is the single cmd-layer entry for rendering plans (M5):
// it delegates to output.RenderPlan so there is exactly one render path.
func handlePlanOutput(plan *output.Plan) error {
	if plan == nil {
		return fmt.Errorf("nil plan")
	}
	return output.RenderPlan(plan)
}

func runLegacyStructured(module int, command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructuredWithNextActions(module, command, args, params, nil, fn)
}

func runLegacyStructuredWithNextActions(module int, command string, args []string, params map[string]interface{}, nextActions []output.NextAction, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("nil command executor for %s", command)
	}
	if !config.IsStructuredOutput() {
		return fn()
	}

	data := &legacyCommandData{
		Command: command,
		Args:    append([]string(nil), args...),
		Params:  normalizeParams(params),
	}

	captured, truncated, err := captureLegacyOutput(fn, legacyOutputCaptureLimit)
	if outputText := strings.TrimSpace(captured); outputText != "" {
		data.CapturedOutput = outputText
	}
	if truncated {
		data.OutputTruncated = true
	}

	if err != nil {
		result := output.Fail(output.GenericOpFailed(module), command+" failed").
			WithDetail(err.Error()).
			WithData(data)
		return handleAuxResult(result)
	}
	result := output.OK(command+" completed", data)
	if len(nextActions) > 0 {
		result.WithNextActions(nextActions...)
	}
	return handleAuxResult(result)
}

// highRiskTextConfirm seams utils.Confirm so tests can stub the prompt.
var highRiskTextConfirm = utils.Confirm

func requireTextHighRiskConfirmation(yes bool, warning, action string) error {
	if config.IsStructuredOutput() || yes {
		return nil
	}
	return highRiskTextConfirm(warning, action)
}

func silenceCobraOnSilentExit(cmd *cobra.Command, err error) error {
	if err != nil && utils.IsSilentExit(err) && cmd != nil {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
	}
	return err
}

// wrapSilentExitSilence wraps every RunE in the command tree so a silent
// ExitCodeError (subprocess output already streamed to the terminal) also
// silences cobra's duplicate "Error: ..." line — individual RunEs never need
// to remember this. Idempotent: it runs at registration (cmd package init)
// and again in Execute() to catch subcommands attached by later init() funcs.
var silentExitWrapped = map[*cobra.Command]bool{}

func wrapSilentExitSilence(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		wrapSilentExitSilence(sub)
	}
	if run := cmd.RunE; run != nil && !silentExitWrapped[cmd] {
		silentExitWrapped[cmd] = true
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return silenceCobraOnSilentExit(c, run(c, args))
		}
	}
}

func structuredParamError(module int, command, message, detail string, args []string, params map[string]interface{}) error {
	if !config.IsStructuredOutput() {
		return fmt.Errorf("%s", detail)
	}
	data := &legacyCommandData{
		Command: command,
		Args:    append([]string(nil), args...),
		Params:  normalizeParams(params),
	}
	return handleAuxResult(
		output.Fail(output.GenericParamError(module), message).
			WithDetail(detail).
			WithData(data),
	)
}

// requireStructuredConfirmation is the single fail-closed T2 gate for
// structured output mode (M1 forerunner: P2 lifts these params into a
// per-command OpSpec table that also derives cobra annotations).
// extraActions appends command-specific routing (e.g. "pig pt switchover").
func requireStructuredConfirmation(module string, code int, message, command, boundary, risk, executeCommand, planCommand string, extraActions ...output.NextAction) error {
	actions := []output.NextAction{
		{Command: executeCommand, Reason: "execute after explicit confirmation", Required: true},
	}
	detail := "structured output mode does not prompt interactively; rerun with --yes to execute"
	if planCommand != "" { // not every gated command offers --plan (e.g. pg init)
		actions = append(actions, output.NextAction{Command: planCommand, Reason: "preview the operation without executing", Required: false})
		detail += " or --plan to preview"
	}
	actions = append(actions, extraActions...)
	return structuredConfirmationError(
		code,
		message,
		detail,
		output.OperationMeta{
			Module:       module,
			Command:      command,
			Boundary:     boundary,
			Risk:         risk,
			Confirmation: "required",
			Executed:     false,
			DryRun:       false,
		},
		actions,
	)
}

// rejectRestoreExtraArgsBeforeDash rejects stray positionals before the "--"
// separator on pb restore / pitr: silently forwarding them to pgbackrest is
// how a -d/-D typo tears down a cluster before failing. Shared by pb and pitr.
func rejectRestoreExtraArgsBeforeDash(cmd *cobra.Command, args []string, code int) error {
	if len(args) == 0 {
		return nil
	}
	dashLen := -1
	if cmd != nil {
		dashLen = cmd.ArgsLenAtDash()
	}
	if dashLen == 0 {
		return nil // every positional sits after the -- separator
	}
	// dashLen > 0: stray positionals before the --; dashLen == -1: no -- at all.
	err := fmt.Errorf("extra pgBackRest restore arguments must be placed after --")
	return restoreInvalidParamsError(code, err)
}

func restoreInvalidParamsError(code int, err error) error {
	if err == nil {
		return nil
	}
	if config.IsStructuredOutput() {
		return handleAuxResult(
			output.Fail(code, "invalid restore parameters").
				WithDetail(err.Error()),
		)
	}
	return &utils.ExitCodeError{Code: output.ExitCode(code), Err: err}
}

// structuredConfirmationError emits the fail-closed refusal for a destructive
// command invoked in structured output mode without --yes/--force. Follow-up
// commands live in the typed envelope-level next_actions field (B39) so agents
// never have to parse prose; the operation metadata stays in data.
func structuredConfirmationError(code int, message, detail string, operation output.OperationMeta, nextActions []output.NextAction) error {
	return handleAuxResult(
		output.Fail(code, message).
			WithDetail(detail).
			WithData(&primitiveContractData{
				Operation: operation,
			}).
			WithNextActions(nextActions...),
	)
}

func isJSONLogOutput() bool {
	return config.OutputFormat == config.OUTPUT_JSON
}

func validateLogLines(lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	return nil
}

func rejectUnsupportedLogOutputFormat(command string) error {
	switch config.OutputFormat {
	case config.OUTPUT_TEXT, config.OUTPUT_JSON:
		return nil
	case config.OUTPUT_YAML, config.OUTPUT_JSON_PRETTY:
		return fmt.Errorf("%s supports structured log output only with -o json (JSONL); use -o text or -o json", command)
	default:
		return nil
	}
}

func captureLegacyOutput(fn func() error, limit int) (captured string, truncated bool, runErr error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", false, fn()
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldFormat := config.OutputFormat
	oldLogrusOut := logrus.StandardLogger().Out

	os.Stdout = writer
	os.Stderr = writer
	logrus.SetOutput(writer)
	config.OutputFormat = config.OUTPUT_TEXT

	done := make(chan captureResult, 1)
	go func() {
		done <- readLimited(reader, limit)
	}()

	defer func() {
		_ = writer.Close()
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		logrus.SetOutput(oldLogrusOut)
		config.OutputFormat = oldFormat
		result := <-done
		_ = reader.Close()
		captured = result.output
		truncated = result.truncated
		if result.readErr != nil && runErr == nil {
			runErr = result.readErr
		}
	}()

	runErr = fn()
	return captured, truncated, runErr
}

func readLimited(r io.Reader, limit int) captureResult {
	if limit <= 0 {
		limit = legacyOutputCaptureLimit
	}
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	total := 0
	truncated := false

	for {
		n, err := r.Read(tmp)
		if n > 0 {
			if total < limit {
				writeN := n
				if total+n > limit {
					writeN = limit - total
					truncated = true
				}
				_, _ = buf.Write(tmp[:writeN])
			} else {
				truncated = true
			}
			total += n
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return captureResult{
				output:    buf.String(),
				truncated: truncated,
				readErr:   err,
			}
		}
	}

	return captureResult{
		output:    buf.String(),
		truncated: truncated,
	}
}

func normalizeParams(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		if v == nil {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
