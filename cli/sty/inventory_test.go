package sty

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/inventory"
	"strings"
	"testing"
)

const validEditInventory = `all:
  children:
    pg-meta:
      hosts: { 10.10.10.10: { pg_seq: 1, pg_role: primary } }
      vars: { pg_cluster: pg-meta }
  vars:
    admin_ip: 10.10.10.10
    region: default               # keep
`

func TestEditScopedFragmentPreservesOutsideBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	if err := os.WriteFile(path, []byte(validEditInventory), 0644); err != nil {
		t.Fatal(err)
	}
	originalRunner, originalFormat := runInventoryEditorFn, config.OutputFormat
	defer func() { runInventoryEditorFn, config.OutputFormat = originalRunner, originalFormat }()
	config.OutputFormat = config.OUTPUT_TEXT
	runInventoryEditorFn = func(_ string, fragment string) error {
		content, err := os.ReadFile(fragment)
		if err != nil {
			return err
		}
		if bytes.Contains(content, []byte("    region")) {
			return errors.New("fragment was not dedented")
		}
		return os.WriteFile(fragment, bytes.Replace(content, []byte("region: default"), []byte("region: china"), 1), 0600)
	}
	result := Edit(EditOptions{Path: path, Selector: "vars", Editor: "test-editor"})
	if result == nil || !result.Success {
		t.Fatalf("Edit failed: %+v", result)
	}
	want := strings.Replace(validEditInventory, "region: default", "region: china", 1)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("scoped edit changed surrounding bytes:\n%s", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("edited inventory mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestEditInvalidCandidateAndCASNeverOverwrite(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(string, string) error
		code int
	}{
		{
			name: "invalid",
			edit: func(_ string, fragment string) error {
				return os.WriteFile(fragment, []byte("- not-a-mapping\n"), 0600)
			},
			code: output.CodeStyInventoryInvalid,
		},
		{
			name: "cas",
			edit: func(target string, fragment string) error {
				if err := os.WriteFile(target, []byte(strings.Replace(validEditInventory, "region: default", "region: europe", 1)), 0644); err != nil {
					return err
				}
				content, err := os.ReadFile(fragment)
				if err != nil {
					return err
				}
				return os.WriteFile(fragment, bytes.Replace(content, []byte("default"), []byte("china"), 1), 0600)
			},
			code: output.CodeStyEditConflict,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pigsty.yml")
			if err := os.WriteFile(path, []byte(validEditInventory), 0644); err != nil {
				t.Fatal(err)
			}
			originalRunner, originalFormat := runInventoryEditorFn, config.OutputFormat
			defer func() { runInventoryEditorFn, config.OutputFormat = originalRunner, originalFormat }()
			config.OutputFormat = config.OUTPUT_TEXT
			runInventoryEditorFn = func(_ string, fragment string) error { return test.edit(path, fragment) }
			result := Edit(EditOptions{Path: path, Selector: "vars", Editor: "test-editor"})
			if result == nil || result.Success || result.Code != test.code {
				t.Fatalf("Edit result = %+v, want code %d", result, test.code)
			}
			retained := strings.TrimPrefix(result.Detail, "edited fragment retained at ")
			if retained == result.Detail {
				t.Fatalf("missing retained fragment detail: %q", result.Detail)
			}
			defer os.RemoveAll(filepath.Dir(retained))
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "invalid" && string(got) != validEditInventory {
				t.Fatal("invalid fragment overwrote inventory")
			}
			if test.name == "cas" && !bytes.Contains(got, []byte("region: europe")) {
				t.Fatal("CAS conflict overwrote concurrent inventory")
			}
		})
	}
}

func TestEditRejectsStructuredOutput(t *testing.T) {
	original := config.OutputFormat
	defer func() { config.OutputFormat = original }()
	config.OutputFormat = config.OUTPUT_JSON
	result := Edit(EditOptions{Path: "unused.yml"})
	if result.Success || result.Code != output.CodeStyEditInvalidArgs {
		t.Fatalf("structured Edit result = %+v", result)
	}
}

func TestValidateInventoryDiagnosticsAndStrictWarnings(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "invalid.yml")
	source := `all:
  children:
    bad-group:
      hosts: { db1: { pg_seq: -1, pg_role: leader, pg_upstream: nope } }
      vars: { pg_cluster: "bad_name" }
  vars: { admin_ip: 999.1.1.1 }
`
	if err := os.WriteFile(invalid, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	result := ValidateInventory(ValidateOptions{Path: invalid})
	if result.Success || result.Code != output.CodeStyValidationFailed {
		t.Fatalf("ValidateInventory result = %+v", result)
	}
	data, ok := result.Data.(*ValidateData)
	if !ok {
		t.Fatalf("validation data type = %T", result.Data)
	}
	want := map[string]bool{"PIG_ADMIN_IP": false, "PIG_HOST_IP": false, "PIG_PG_CLUSTER": false, "PIG_SEQ_TYPE": false, "PIG_PG_ROLE": false, "PIG_PG_UPSTREAM": false}
	for _, diagnostic := range data.Diagnostics {
		if _, exists := want[diagnostic.Code]; exists {
			want[diagnostic.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing %s in %+v", code, data.Diagnostics)
		}
	}

	valid := filepath.Join(t.TempDir(), "warning.yml")
	warningSource := strings.Replace(validEditInventory, "pg-meta:", "pg-meta-bad:", 1)
	if err := os.WriteFile(valid, []byte(warningSource), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ValidateInventory(ValidateOptions{Path: valid}); !got.Success {
		t.Fatalf("warning-only validation failed by default: %+v", got)
	}
	if got := ValidateInventory(ValidateOptions{Path: valid, Strict: true}); got.Success {
		t.Fatal("strict validation accepted warning")
	}
}

func TestValidateMalformedYAMLReturnsStableDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yml")
	if err := os.WriteFile(path, []byte("all:\n  vars: [secret: value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := ValidateInventory(ValidateOptions{Path: path})
	data, ok := result.Data.(*ValidateData)
	if result.Success || !ok || len(data.Diagnostics) != 1 || data.Diagnostics[0].Code != "YAML_PARSE" {
		t.Fatalf("malformed validation result = %+v", result)
	}
	if strings.Contains(strings.ToLower(data.Diagnostics[0].Message), "secret") {
		t.Fatal("parse diagnostic leaked source content")
	}
}

func TestValidatePigstyConfCorpus(t *testing.T) {
	root := pigstyConfTestRoot(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".yml" {
			return nil
		}
		result := ValidateInventory(ValidateOptions{Path: path})
		if !result.Success {
			return errors.New(path + ": " + result.Message)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEditChangeSetDoesNotContainValues(t *testing.T) {
	data := (&EditData{Changes: inventory.ChangeSet{Changes: []inventory.Change{{Operation: "set", Identifier: "region"}}}}).Text()
	if strings.Contains(data, "secret") {
		t.Fatal("EditData text unexpectedly contains values")
	}
}
