package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"pig/internal/config"
	"pig/internal/output"
	"strings"
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
