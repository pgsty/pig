package sty

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/inventory"
	"reflect"
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
	data, ok := result.Data.(*EditData)
	if !ok || !data.ModeTightened || data.ModeBefore != "0644" || data.ModeAfter != "0600" {
		t.Fatalf("edit mode metadata = %+v", result.Data)
	}
	if got := strings.Count(data.Text(), "permissions tightened"); got != 1 {
		t.Fatalf("mode tighten notices = %d in %q", got, data.Text())
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
			if info, err := os.Stat(retained); err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("recovery fragment mode: info=%v err=%v", info, err)
			}
			if info, err := os.Stat(filepath.Dir(retained)); err != nil || info.Mode().Perm() != 0700 {
				t.Fatalf("recovery directory mode: info=%v err=%v", info, err)
			}
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

func TestEditPreservesStricterOwnerOnlyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	if err := os.WriteFile(path, []byte(validEditInventory), 0400); err != nil {
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
		return os.WriteFile(fragment, bytes.Replace(content, []byte("default"), []byte("china"), 1), 0600)
	}
	result := Edit(EditOptions{Path: path, Selector: "vars.region", Editor: "test-editor"})
	if result == nil || !result.Success {
		t.Fatalf("Edit failed: %+v", result)
	}
	data := result.Data.(*EditData)
	if data.ModeTightened || data.ModeBefore != "0400" || data.ModeAfter != "0400" {
		t.Fatalf("stricter mode metadata = %+v", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0400 {
		t.Fatalf("edited inventory mode = %04o, want 0400", info.Mode().Perm())
	}
}

func TestEditRechecksDestinationModeBeforeCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	if err := os.WriteFile(path, []byte(validEditInventory), 0644); err != nil {
		t.Fatal(err)
	}
	originalRunner, originalFormat := runInventoryEditorFn, config.OutputFormat
	defer func() { runInventoryEditorFn, config.OutputFormat = originalRunner, originalFormat }()
	config.OutputFormat = config.OUTPUT_TEXT
	runInventoryEditorFn = func(_ string, fragment string) error {
		if err := os.Chmod(path, 0400); err != nil {
			return err
		}
		content, err := os.ReadFile(fragment)
		if err != nil {
			return err
		}
		return os.WriteFile(fragment, bytes.Replace(content, []byte("default"), []byte("china"), 1), 0600)
	}
	result := Edit(EditOptions{Path: path, Selector: "vars.region", Editor: "test-editor"})
	if result == nil || !result.Success {
		t.Fatalf("Edit failed: %+v", result)
	}
	data := result.Data.(*EditData)
	if data.ModeTightened || data.ModeBefore != "0400" || data.ModeAfter != "0400" {
		t.Fatalf("rechecked mode metadata = %+v", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0400 {
		t.Fatalf("committed mode = %04o, want 0400", info.Mode().Perm())
	}
}

func TestEditRefusesSymlinkInventory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yml")
	link := filepath.Join(dir, "pigsty.yml")
	if err := os.WriteFile(target, []byte(validEditInventory), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	result := Edit(EditOptions{Path: link, Editor: "test-editor"})
	if result.Success || result.Code != output.CodeStyEditInvalidArgs {
		t.Fatalf("symlink edit result = %+v", result)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != validEditInventory {
		t.Fatal("symlink refusal changed its target")
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
	if data.Kind != InventoryValidateKind {
		t.Fatalf("validation kind = %q, want %q", data.Kind, InventoryValidateKind)
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
	defaultResult := ValidateInventory(ValidateOptions{Path: valid})
	if !defaultResult.Success {
		t.Fatalf("warning-only validation failed by default: %+v", defaultResult)
	}
	strictResult := ValidateInventory(ValidateOptions{Path: valid, Strict: true})
	if strictResult.Success {
		t.Fatal("strict validation accepted warning")
	}
	defaultData := defaultResult.Data.(*ValidateData)
	strictData := strictResult.Data.(*ValidateData)
	if !reflect.DeepEqual(defaultData.Diagnostics, strictData.Diagnostics) {
		t.Fatalf("strict rewrote diagnostics:\ndefault=%+v\nstrict=%+v", defaultData.Diagnostics, strictData.Diagnostics)
	}
	for _, diagnostic := range strictData.Diagnostics {
		if diagnostic.Code == "INV_GROUP_NAME" && diagnostic.Severity != inventory.SeverityWarning {
			t.Fatalf("strict rewrote warning severity: %+v", diagnostic)
		}
	}
}

func TestValidateMalformedYAMLReturnsStableDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yml")
	if err := os.WriteFile(path, []byte("all:\n  vars: [secret: value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := ValidateInventory(ValidateOptions{Path: path})
	data, ok := result.Data.(*ValidateData)
	if result.Success || !ok || data.Kind != InventoryValidateKind || len(data.Diagnostics) != 1 || data.Diagnostics[0].Code != "YAML_PARSE" {
		t.Fatalf("malformed validation result = %+v", result)
	}
	if strings.Contains(strings.ToLower(data.Diagnostics[0].Message), "secret") {
		t.Fatal("parse diagnostic leaked source content")
	}
}

func TestValidateRequiresCompleteStaticInventorySource(t *testing.T) {
	dir := t.TempDir()
	if result := ValidateInventory(ValidateOptions{Path: dir}); result.Success || result.Code != output.CodeStyValidateInvalidArgs {
		t.Fatalf("directory validation result = %+v", result)
	}

	dynamic := filepath.Join(t.TempDir(), "dynamic.yml")
	if err := os.WriteFile(dynamic, []byte("#!/bin/sh\necho all:\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if result := ValidateInventory(ValidateOptions{Path: dynamic}); result.Success || result.Code != output.CodeStyValidateInvalidArgs {
		t.Fatalf("dynamic validation result = %+v", result)
	}

	partial := filepath.Join(t.TempDir(), "partial.yml")
	if err := os.WriteFile(partial, []byte("all:\n  vars:\n    version: v4.4.0\n    admin_ip: 10.0.0.1\n    region: default\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := ValidateInventory(ValidateOptions{Path: partial})
	data, ok := result.Data.(*ValidateData)
	if result.Success || !ok {
		t.Fatalf("partial validation result = %+v", result)
	}
	found := false
	for _, diagnostic := range data.Diagnostics {
		if diagnostic.Code == "INV_CHILDREN_REQUIRED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing complete-file diagnostic: %+v", data.Diagnostics)
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

func TestListInventoryOrderedMetadataNeverContainsValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	source := `all:
  children:
    pg-meta:
      hosts:
        10.10.10.10: { pg_role: primary }
      vars:
        pg_cluster: pg-meta
  vars:
    password: super-secret-value
    ports: [5432, 6432]
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	result := ListInventory(InventoryOptions{Path: path})
	if result == nil || !result.Success {
		t.Fatalf("ListInventory failed: %+v", result)
	}
	data, ok := result.Data.(*InventoryListData)
	if !ok {
		t.Fatalf("list data type = %T", result.Data)
	}
	if data.Kind != InventoryListKind || data.Root != "/" {
		t.Fatalf("list metadata = %+v", data)
	}
	wantPrefix := []struct{ path, kind string }{
		{"/", "document"},
		{"/all", "group"},
		{"/all/children", "groups"},
		{"/all/children/pg-meta", "group"},
		{"/all/children/pg-meta/hosts", "hosts"},
		{"/all/children/pg-meta/hosts/10.10.10.10", "host"},
	}
	if len(data.Items) < len(wantPrefix) {
		t.Fatalf("list items = %+v", data.Items)
	}
	for index, want := range wantPrefix {
		if data.Items[index].Path != want.path || data.Items[index].Kind != want.kind {
			t.Fatalf("item %d = %+v, want path=%s kind=%s", index, data.Items[index], want.path, want.kind)
		}
	}
	for _, format := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON, config.OUTPUT_YAML} {
		rendered, err := result.Render(format)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(rendered, []byte("super-secret-value")) || bytes.Contains(rendered, []byte("primary")) || bytes.Contains(rendered, []byte("5432")) {
			t.Fatalf("%s inventory list leaked a value: %s", format, rendered)
		}
	}

	selected := ListInventory(InventoryOptions{Path: path, Selector: "vars.ports"})
	selectedData := selected.Data.(*InventoryListData)
	if selectedData.Root != "/all/vars/ports" || len(selectedData.Items) != 3 {
		t.Fatalf("selected list = %+v", selectedData)
	}
}

func TestShowInventoryIsVerbatimAndBodyStaysOutsideResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	source := []byte("all:\r\n  vars:\r\n    password: super-secret-value # keep\r\n")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()
	config.OutputFormat = config.OUTPUT_TEXT

	body, failure := ShowInventory(InventoryOptions{Path: path})
	if failure != nil || !bytes.Equal(body, source) {
		t.Fatalf("whole show body=%q failure=%+v", body, failure)
	}
	body, failure = ShowInventory(InventoryOptions{Path: path, Selector: "vars"})
	if failure != nil || string(body) != "password: super-secret-value # keep\r\n" {
		t.Fatalf("fragment show body=%q failure=%+v", body, failure)
	}

	config.OutputFormat = config.OUTPUT_JSON
	body, failure = ShowInventory(InventoryOptions{Path: path})
	if len(body) != 0 || failure == nil || failure.Code != output.CodeStyInventoryShowInvalidArgs || failure.ExitCode() != 2 {
		t.Fatalf("structured show body=%q failure=%+v", body, failure)
	}
	rendered, err := failure.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rendered, []byte("super-secret-value")) {
		t.Fatalf("show failure copied body into Result: %s", rendered)
	}
}

func TestInventoryQueriesRejectNonStaticSource(t *testing.T) {
	directory := t.TempDir()
	if result := ListInventory(InventoryOptions{Path: directory}); result.Success || result.Code != output.CodeStyInventoryListInvalidArgs {
		t.Fatalf("directory list result = %+v", result)
	}
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()
	config.OutputFormat = config.OUTPUT_TEXT
	if body, result := ShowInventory(InventoryOptions{Path: directory}); len(body) != 0 || result == nil || result.Code != output.CodeStyInventoryShowInvalidArgs {
		t.Fatalf("directory show body=%q result=%+v", body, result)
	}

	executable := filepath.Join(t.TempDir(), "inventory.py")
	if err := os.WriteFile(executable, []byte("#!/usr/bin/env python3\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if result := ListInventory(InventoryOptions{Path: executable}); result.Success || result.Code != output.CodeStyInventoryListInvalidArgs || !strings.Contains(result.Message, "dynamic") {
		t.Fatalf("dynamic list result = %+v", result)
	}
	if result := Edit(EditOptions{Path: directory, Editor: "test-editor"}); result.Success || result.Code != output.CodeStyEditInvalidArgs {
		t.Fatalf("directory edit result = %+v", result)
	}
}
