package cmd

import (
	"errors"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"testing"
)

func TestResolvePbInfoRawOutput(t *testing.T) {
	origRawOutput := pbInfoRawOutput
	origFormat := config.OutputFormat
	defer func() {
		pbInfoRawOutput = origRawOutput
		config.OutputFormat = origFormat
	}()

	tests := []struct {
		name      string
		rawOutput string
		format    string
		want      string
		wantErr   bool
	}{
		{name: "explicit json", rawOutput: "json", format: config.OUTPUT_TEXT, want: "json", wantErr: false},
		{name: "explicit text", rawOutput: "text", format: config.OUTPUT_JSON, want: "text", wantErr: false},
		{name: "explicit uppercase normalized", rawOutput: "JSON", format: config.OUTPUT_TEXT, want: "json", wantErr: false},
		{name: "explicit invalid", rawOutput: "yaml", format: config.OUTPUT_TEXT, want: "", wantErr: true},
		{name: "inherit json output", rawOutput: "", format: config.OUTPUT_JSON, want: "json", wantErr: false},
		{name: "inherit json-pretty output", rawOutput: "", format: config.OUTPUT_JSON_PRETTY, want: "json", wantErr: false},
		{name: "inherit yaml output unsupported", rawOutput: "", format: config.OUTPUT_YAML, want: "", wantErr: true},
		{name: "text output default", rawOutput: "", format: config.OUTPUT_TEXT, want: "", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pbInfoRawOutput = tt.rawOutput
			config.OutputFormat = tt.format
			got, err := resolvePbInfoRawOutput()
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolvePbInfoRawOutput() error=%v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolvePbInfoRawOutput()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestPbInfoRawOutputFlagDoesNotShadowGlobalOutput(t *testing.T) {
	if f := pbInfoCmd.Flags().Lookup("output"); f != nil {
		t.Fatalf("pb info local --output should not exist, found %q", f.Name)
	}
	if f := pbInfoCmd.Flags().ShorthandLookup("o"); f != nil {
		t.Fatalf("pb info local -o should not exist, found %q", f.Name)
	}
	if f := pbInfoCmd.Flags().Lookup("raw-output"); f == nil {
		t.Fatal("pb info --raw-output flag should exist")
	}
	if f := rootCmd.PersistentFlags().Lookup("output"); f == nil {
		t.Fatal("root --output flag should exist")
	}
}

func TestPbInfoRawOutputValidationStructuredMode(t *testing.T) {
	origRaw := pbInfoRaw
	origRawOutput := pbInfoRawOutput
	origSet := pbInfoSet
	origFormat := config.OutputFormat
	defer func() {
		pbInfoRaw = origRaw
		pbInfoRawOutput = origRawOutput
		pbInfoSet = origSet
		config.OutputFormat = origFormat
	}()

	pbInfoRaw = false
	pbInfoRawOutput = "json"
	pbInfoSet = ""
	config.OutputFormat = config.OUTPUT_JSON

	err := pbInfoCmd.RunE(pbInfoCmd, nil)
	if err == nil {
		t.Fatal("expected error for --raw-output without --raw in structured mode")
	}

	var exitCodeErr *utils.ExitCodeError
	if !errors.As(err, &exitCodeErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	wantCode := output.ExitCode(output.CodePbInvalidInfoParams)
	if exitCodeErr.Code != wantCode {
		t.Fatalf("unexpected exit code: got %d, want %d", exitCodeErr.Code, wantCode)
	}
}
