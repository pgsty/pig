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

func handlePlanOutput(plan *output.Plan) error {
	if plan == nil {
		return fmt.Errorf("nil plan")
	}
	data, err := plan.Render(config.OutputFormat)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func runLegacyStructured(module int, command string, args []string, params map[string]interface{}, fn func() error) error {
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
		return handleAuxResult(
			output.Fail(module+output.CAT_OPERATION+1, command+" failed").
				WithDetail(err.Error()).
				WithData(data),
		)
	}
	return handleAuxResult(output.OK(command+" completed", data))
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
		output.Fail(module+output.CAT_PARAM+1, message).
			WithDetail(detail).
			WithData(data),
	)
}

func structuredConfirmationError(code int, message, detail string, operation output.OperationMeta, nextActions []output.NextAction) error {
	return handleAuxResult(
		output.Fail(code, message).
			WithDetail(detail).
			WithData(&primitiveContractData{
				Operation:   operation,
				NextActions: nextActions,
			}),
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

func captureLegacyOutput(fn func() error, limit int) (string, bool, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", false, fn()
	}
	defer reader.Close()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldFormat := config.OutputFormat

	os.Stdout = writer
	os.Stderr = writer
	config.OutputFormat = config.OUTPUT_TEXT

	done := make(chan captureResult, 1)
	go func() {
		done <- readLimited(reader, limit)
	}()

	runErr := fn()

	_ = writer.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	config.OutputFormat = oldFormat

	result := <-done
	if result.readErr != nil && runErr == nil {
		runErr = result.readErr
	}
	return result.output, result.truncated, runErr
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
