package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"unicode/utf8"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type testStatusError struct {
	code int
	err  error
}

func (e *testStatusError) Error() string   { return e.err.Error() }
func (e *testStatusError) Unwrap() error   { return e.err }
func (e *testStatusError) StatusCode() int { return e.code }

func TestRunLegacyStructuredCapturesCommandOutput(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()
	config.OutputFormat = config.OUTPUT_JSON

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	runErr := runLegacyStructured(990000, "test command", []string{"arg1"}, map[string]interface{}{"k": "v"}, func() error {
		fmt.Printf("%slegacy text output%s\n", utils.ColorBlue, utils.ColorReset)
		return nil
	})

	_ = w.Close()
	os.Stdout = origStdout
	raw, _ := io.ReadAll(r)
	_ = r.Close()

	if runErr != nil {
		t.Fatalf("runLegacyStructured() error = %v", runErr)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(trimJSONSpace(raw), &payload); err != nil {
		t.Fatalf("invalid json output: %v, raw=%q", err, string(raw))
	}

	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected success=true, got payload=%v", payload)
	}

	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if cmd, _ := data["command"].(string); cmd != "test command" {
		t.Fatalf("expected command 'test command', got %q", cmd)
	}
	if captured, _ := data["captured_output"].(string); !strings.Contains(captured, "legacy text output") || strings.Contains(captured, "\x1b[") {
		t.Fatalf("expected plain captured output, got %q", captured)
	}
}

func TestRunLegacyStructuredSensitiveRedactsCapturedOutput(t *testing.T) {
	origFormat := config.OutputFormat
	config.OutputFormat = config.OUTPUT_JSON
	t.Cleanup(func() { config.OutputFormat = origFormat })

	const secret = "password=do-not-emit"
	raw := captureStdout(t, func() {
		err := runLegacyStructuredSensitiveWithNextActions(
			990000,
			"test sensitive",
			[]string{"password=<redacted>"},
			nil,
			nil,
			func() error {
				fmt.Println(secret)
				return nil
			},
		)
		if err != nil {
			t.Fatalf("sensitive wrapper returned error: %v", err)
		}
	})
	if strings.Contains(raw, secret) {
		t.Fatalf("sensitive output leaked: %s", raw)
	}
	var payload struct {
		Data legacyCommandData `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode structured output: %v; raw=%q", err, raw)
	}
	if !payload.Data.OutputRedacted || payload.Data.CapturedOutput != "" || payload.Data.OutputTruncated {
		t.Fatalf("unexpected sensitive data envelope: %+v", payload.Data)
	}
	if len(payload.Data.Args) != 1 || payload.Data.Args[0] != "password=<redacted>" {
		t.Fatalf("redacted args = %#v", payload.Data.Args)
	}
}

func TestRunLegacyStructuredUsesExecutorStatusCode(t *testing.T) {
	original := config.OutputFormat
	config.OutputFormat = config.OUTPUT_JSON
	defer func() { config.OutputFormat = original }()

	raw := captureStdout(t, func() {
		err := runLegacyStructured(legacyModuleSty, "pig sty test", nil, nil, func() error {
			return &testStatusError{code: output.CodeStyDeployDependencyMissing, err: errors.New("missing dependency")}
		})
		if err == nil || utils.ExitCode(err) != 4 {
			t.Fatalf("runLegacyStructured error = %v, exit = %d", err, utils.ExitCode(err))
		}
	})
	var payload output.Result
	if err := json.Unmarshal(trimJSONSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("decode result: %v; raw=%q", err, raw)
	}
	if payload.Code != output.CodeStyDeployDependencyMissing || payload.Detail != "missing dependency" {
		t.Fatalf("structured result = %+v", payload)
	}
}

func TestRuntimeCommandErrorSuppressesOnlyDuplicateOutput(t *testing.T) {
	origFormat := config.OutputFormat
	config.OutputFormat = config.OUTPUT_TEXT
	t.Cleanup(func() { config.OutputFormat = origFormat })

	command := &cobra.Command{}
	plain := errors.New("patronictl not found")
	var got error
	stderr := captureStderr(t, func() { got = runtimeCommandError(command, plain) })
	if !errors.Is(got, plain) || !utils.IsSilentExit(got) {
		t.Fatalf("runtimeCommandError() = %v, want wrapped silent error", got)
	}
	if !strings.Contains(stderr, "Error: patronictl not found") {
		t.Fatalf("plain runtime stderr = %q, want one error line", stderr)
	}
	if !command.SilenceUsage || !command.SilenceErrors {
		t.Fatalf("plain runtime error flags = usage:%v errors:%v, want both",
			command.SilenceUsage, command.SilenceErrors)
	}

	command = &cobra.Command{}
	silent := &utils.ExitCodeError{Code: 1, Err: errors.New("already printed"), Silent: true}
	stderr = captureStderr(t, func() { got = runtimeCommandError(command, silent) })
	if got != silent {
		t.Fatalf("runtimeCommandError() = %v, want silent error", got)
	}
	if stderr != "" {
		t.Fatalf("already-printed runtime stderr = %q, want no duplicate", stderr)
	}
	if !command.SilenceUsage || !command.SilenceErrors {
		t.Fatalf("silent runtime error flags = usage:%v errors:%v, want both",
			command.SilenceUsage, command.SilenceErrors)
	}
}

func TestRunLogSnapshotDispatchesByOutputPolicy(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()

	jsonCalls, textCalls := 0, 0
	jsonl := func() error { jsonCalls++; return nil }
	text := func() error { textCalls++; return nil }

	config.OutputFormat = config.OUTPUT_TEXT
	if err := runLogSnapshot("test log", 10, jsonl, text); err != nil {
		t.Fatalf("text snapshot returned error: %v", err)
	}
	config.OutputFormat = config.OUTPUT_JSON
	if err := runLogSnapshot("test log", 10, jsonl, text); err != nil {
		t.Fatalf("json snapshot returned error: %v", err)
	}
	if jsonCalls != 1 || textCalls != 1 {
		t.Fatalf("dispatch calls json=%d text=%d, want 1 each", jsonCalls, textCalls)
	}

	config.OutputFormat = config.OUTPUT_YAML
	if err := runLogSnapshot("test log", 10, jsonl, text); err == nil {
		t.Fatal("yaml snapshot should be rejected")
	}
	if err := runLogSnapshot("test log", 0, jsonl, text); err == nil {
		t.Fatal("non-positive line count should be rejected")
	}
	if jsonCalls != 1 || textCalls != 1 {
		t.Fatalf("rejected snapshots executed a backend: json=%d text=%d", jsonCalls, textCalls)
	}
}

func TestRunLogStreamDispatchesByOutputPolicy(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()

	t.Run("text invokes stream", func(t *testing.T) {
		config.OutputFormat = config.OUTPUT_TEXT
		wantErr := errors.New("stream stopped")
		streamCalls := 0
		err := runLogStream(output.MODULE_PT, "pig pt log tail", "pig pt log show", 10, nil, nil, func() error {
			streamCalls++
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("runLogStream() error = %v, want %v", err, wantErr)
		}
		if streamCalls != 1 {
			t.Fatalf("stream calls = %d, want 1", streamCalls)
		}
	})

	t.Run("invalid lines reject before stream", func(t *testing.T) {
		config.OutputFormat = config.OUTPUT_TEXT
		streamCalls := 0
		err := runLogStream(output.MODULE_PT, "pig pt log tail", "pig pt log show", 0, nil, nil, func() error {
			streamCalls++
			return nil
		})
		if err == nil || err.Error() != "lines must be positive" {
			t.Fatalf("runLogStream() error = %v, want lines validation error", err)
		}
		if streamCalls != 0 {
			t.Fatalf("invalid lines executed stream %d time(s)", streamCalls)
		}
	})

	t.Run("structured returns typed error without stream", func(t *testing.T) {
		config.OutputFormat = config.OUTPUT_JSON
		streamCalls := 0
		var runErr error
		raw := captureStdout(t, func() {
			runErr = runLogStream(
				output.MODULE_PT,
				"pig pt log tail",
				"pig pt log show",
				10,
				[]string{"patroni.log"},
				map[string]interface{}{"lines": 10},
				func() error {
					streamCalls++
					return nil
				},
			)
		})

		var exitErr *utils.ExitCodeError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("runLogStream() error = %T %v, want *utils.ExitCodeError", runErr, runErr)
		}
		wantCode := output.GenericParamError(output.MODULE_PT)
		if exitErr.Code != output.ExitCode(wantCode) {
			t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitCode(wantCode))
		}
		if streamCalls != 0 {
			t.Fatalf("structured output executed stream %d time(s)", streamCalls)
		}

		var payload struct {
			Success bool              `json:"success"`
			Code    int               `json:"code"`
			Message string            `json:"message"`
			Detail  string            `json:"detail"`
			Data    legacyCommandData `json:"data"`
		}
		if err := json.Unmarshal(trimJSONSpace([]byte(raw)), &payload); err != nil {
			t.Fatalf("invalid JSON output: %v, raw=%q", err, raw)
		}
		if payload.Success || payload.Code != wantCode {
			t.Fatalf("payload success=%v code=%d, want false/%d", payload.Success, payload.Code, wantCode)
		}
		if payload.Message != "streaming log output is not supported in structured output" {
			t.Fatalf("message = %q", payload.Message)
		}
		if !strings.Contains(payload.Detail, "pig pt log show -n N -o json") {
			t.Fatalf("detail = %q, want snapshot guidance", payload.Detail)
		}
		if payload.Data.Command != "pig pt log tail" || len(payload.Data.Args) != 1 || payload.Data.Args[0] != "patroni.log" {
			t.Fatalf("unexpected command data: %+v", payload.Data)
		}
	})
}

func TestRunLegacyStructuredCapturesLogrusAndRestoresOutput(t *testing.T) {
	origFormat := config.OutputFormat
	origLogOut := logrus.StandardLogger().Out
	origLevel := logrus.GetLevel()
	defer func() {
		config.OutputFormat = origFormat
		logrus.SetOutput(origLogOut)
		logrus.SetLevel(origLevel)
	}()
	config.OutputFormat = config.OUTPUT_JSON
	logrus.SetLevel(logrus.InfoLevel)

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	runErr := runLegacyStructured(990000, "test command", nil, nil, func() error {
		logrus.Warn("legacy logrus output")
		return nil
	})

	_ = w.Close()
	os.Stdout = origStdout
	raw, _ := io.ReadAll(r)
	_ = r.Close()

	if runErr != nil {
		t.Fatalf("runLegacyStructured() error = %v", runErr)
	}
	if logrus.StandardLogger().Out != origLogOut {
		t.Fatal("runLegacyStructured should restore logrus output")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(trimJSONSpace(raw), &payload); err != nil {
		t.Fatalf("invalid json output: %v, raw=%q", err, string(raw))
	}
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if captured, _ := data["captured_output"].(string); !strings.Contains(captured, "legacy logrus output") {
		t.Fatalf("expected captured logrus output, got %q", captured)
	}
}

func TestRunLegacyStructuredSerializesGlobalCapture(t *testing.T) {
	original := config.OutputFormat
	config.OutputFormat = config.OUTPUT_JSON
	t.Cleanup(func() { config.OutputFormat = original })

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	raw := captureStdout(t, func() {
		start := make(chan struct{})
		for _, marker := range []string{"alpha", "beta"} {
			marker := marker
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- runLegacyStructured(990000, marker, nil, nil, func() error {
					fmt.Println(marker)
					return nil
				})
			}()
		}
		close(start)
		wg.Wait()
	})
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("runLegacyStructured() error = %v", err)
		}
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	seen := make(map[string]string)
	for len(seen) < 2 {
		var payload struct {
			Data legacyCommandData `json:"data"`
		}
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("decode concurrent structured output: %v; raw=%q", err, raw)
		}
		seen[payload.Data.Command] = payload.Data.CapturedOutput
	}
	for _, marker := range []string{"alpha", "beta"} {
		if seen[marker] != marker {
			t.Fatalf("captured output for %s = %q, want isolated marker", marker, seen[marker])
		}
	}
}

func TestCaptureLegacyOutputRestoresStateAfterPanic(t *testing.T) {
	origStdout := os.Stdout
	origStderr := os.Stderr
	origFormat := config.OutputFormat
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
		config.OutputFormat = origFormat
	}()

	config.OutputFormat = config.OUTPUT_JSON
	var recovered interface{}
	func() {
		defer func() {
			recovered = recover()
		}()
		_, _, _ = captureLegacyOutputUnlocked(func() error {
			panic("legacy panic")
		}, 1024)
	}()

	if recovered != "legacy panic" {
		t.Fatalf("captureLegacyOutput should re-panic, got %v", recovered)
	}
	if os.Stdout != origStdout {
		t.Fatal("captureLegacyOutput should restore stdout after panic")
	}
	if os.Stderr != origStderr {
		t.Fatal("captureLegacyOutput should restore stderr after panic")
	}
	if config.OutputFormat != config.OUTPUT_JSON {
		t.Fatalf("captureLegacyOutput should restore output format, got %q", config.OutputFormat)
	}
}

func TestReadLimitedPreservesHeadAndTail(t *testing.T) {
	for _, limit := range []int{1, 15, 16, legacyOutputCaptureLimit, 0, -1} {
		effectiveLimit := limit
		if effectiveLimit <= 0 {
			effectiveLimit = legacyOutputCaptureLimit
		}
		for _, size := range []int{0, effectiveLimit - 1, effectiveLimit, effectiveLimit + 1, 3 * effectiveLimit} {
			input := strings.Repeat("0123456789", size/10+1)[:size]
			for _, fragmented := range []bool{false, true} {
				reader := io.Reader(strings.NewReader(input))
				if fragmented {
					reader = iotest.OneByteReader(reader)
				}
				result := readLimited(reader, limit)
				want := input
				if size > effectiveLimit {
					want = input[:effectiveLimit/2] + legacyOutputTruncationMarker + input[size-(effectiveLimit-effectiveLimit/2):]
				}
				if result.output != want || result.truncated != (size > effectiveLimit) || result.readErr != nil {
					t.Fatalf("limit=%d size=%d fragmented=%t: got length=%d truncated=%t err=%v, want length=%d",
						limit, size, fragmented, len(result.output), result.truncated, result.readErr, len(want))
				}
			}
		}
	}
}

func TestReadLimitedPreservesUTF8AndReadErrors(t *testing.T) {
	input := "开始🌱" + strings.Repeat("进度", 100) + "末尾错误"
	readErr := errors.New("reader failed")
	for _, limit := range []int{1, 16, 17, 19, len(input), len(input) + 1} {
		reader := io.MultiReader(strings.NewReader(input), iotest.ErrReader(readErr))
		result := readLimited(reader, limit)
		if !errors.Is(result.readErr, readErr) || !utf8.ValidString(result.output) {
			t.Fatalf("limit=%d: result=%+v", limit, result)
		}
		if limit >= len(input) {
			if result.output != input || result.truncated {
				t.Fatalf("untruncated UTF-8 input changed: %+v", result)
			}
			continue
		}
		parts := strings.Split(result.output, legacyOutputTruncationMarker)
		if !result.truncated || len(parts) != 2 || !strings.HasPrefix(input, parts[0]) || !strings.HasSuffix(input, parts[1]) {
			t.Fatalf("UTF-8 head/tail lost: %+v", result)
		}
		if limit > 1 && (!strings.Contains(parts[0], "开始") || !strings.Contains(parts[1], "错误")) {
			t.Fatalf("UTF-8 boundary removed complete runes: %+v", result)
		}
	}
}

func TestRunLegacyStructuredTruncatedFailureIsPureJSON(t *testing.T) {
	origFormat, origStdout := config.OutputFormat, os.Stdout
	t.Cleanup(func() { config.OutputFormat, os.Stdout = origFormat, origStdout })
	t.Setenv("PIG_NON_INTERACTIVE", "")
	config.OutputFormat = config.OUTPUT_JSON
	for _, redact := range []bool{false, true} {
		// A file avoids blocking on an outer pipe while rendering a 64 KiB result.
		file, err := os.CreateTemp(t.TempDir(), "legacy-output-*.json")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		os.Stdout = file
		var runErr error
		stderr := captureStderr(t, func() {
			runErr = runLegacyStructuredContractWithPolicy(legacyModuleSty, "test download", "", nil, nil, nil,
				legacyStructuredPolicy{RedactCapturedOutput: redact}, func() error {
					fmt.Fprintln(os.Stdout, "download context")
					fmt.Fprint(os.Stdout, strings.Repeat("progress\n", legacyOutputCaptureLimit))
					fmt.Fprintln(os.Stderr, "final connection failure")
					return errors.New("download failed")
				})
		})
		os.Stdout = origStdout
		if runErr == nil || stderr != "" {
			t.Fatalf("failure error=%v stderr=%q", runErr, stderr)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Success bool              `json:"success"`
			Data    legacyCommandData `json:"data"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.Success {
			t.Fatalf("failure stdout is not one JSON envelope: %v", err)
		}
		if redact {
			if !result.Data.OutputRedacted || result.Data.CapturedOutput != "" || result.Data.OutputTruncated || strings.Contains(string(raw), "connection failure") {
				t.Fatalf("sensitive truncated output leaked: %+v", result.Data)
			}
			continue
		}
		captured := result.Data.CapturedOutput
		if !result.Data.OutputTruncated || !strings.HasPrefix(captured, "download context") || !strings.HasSuffix(captured, "final connection failure") || !strings.Contains(captured, legacyOutputTruncationMarker) {
			t.Fatal("truncated JSON must retain context, truncation marker, and final failure")
		}
		if len(captured) > legacyOutputCaptureLimit+len(legacyOutputTruncationMarker) {
			t.Fatalf("captured content exceeded limit: %d", len(captured))
		}
	}
}

func trimJSONSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
