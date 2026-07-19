package inventorycli

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

func TestInventoryStatusIsValueFreeAndObservable(t *testing.T) {
	static := config.InventorySourceDescriptor{
		Home: "/srv/pigsty", Tier: config.InventorySourceAnsibleConfig,
		Type: config.InventorySourceStaticYAML, Path: "/srv/pigsty/pigsty.yml",
		Exists: true, Readable: true, StaticCommands: []string{"list", "show", "validate", "diff", "edit"},
	}
	result := InventoryStatus(static)
	if result == nil || !result.Success || len(result.NextActions) != 0 {
		t.Fatalf("static status = %+v", result)
	}
	data, ok := result.Data.(*InventoryStatusData)
	if !ok || data.Kind != InventoryStatusKind || data.Source.Type != config.InventorySourceStaticYAML {
		t.Fatalf("status data = %#v", result.Data)
	}
	if text := data.Text(); !strings.Contains(text, "static_yaml") || strings.Contains(text, "secret") {
		t.Fatalf("status text = %q", text)
	}

	missing := config.InventorySourceDescriptor{
		Tier: config.InventorySourceFlag, Type: config.InventorySourceMissing,
		Path: "/tmp/missing.yml", Findings: []string{"SOURCE_NOT_FOUND"},
	}
	result = InventoryStatus(missing)
	if result == nil || !result.Success || len(result.NextActions) != 1 || !result.NextActions[0].Required {
		t.Fatalf("missing status = %+v", result)
	}
	if got := result.Data.(*InventoryStatusData).Source.Findings; !reflect.DeepEqual(got, []string{"SOURCE_NOT_FOUND"}) {
		t.Fatalf("status findings = %v", got)
	}
}

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
			code: output.CodeInventoryInvalid,
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
			code: output.CodeInventoryEditConflict,
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
	if result.Success || result.Code != output.CodeInventoryEditInvalidArgs {
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

func TestEditAllowsTargetedRepairButBlocksSelectedErrors(t *testing.T) {
	const invalid = `all:
  children:
    pgsql:
      hosts:
        10.0.0.1: { pg_seq: 1, pg_role: leader }
        10.0.0.2: { pg_seq: 2, pg_role: leader }
      vars: { pg_cluster: pgsql }
  vars:
    version: v4.4.0
    admin_ip: 10.0.0.1
    region: default
`
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
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
		return os.WriteFile(fragment, bytes.Replace(content, []byte("leader"), []byte("primary"), 1), 0600)
	}
	result := Edit(EditOptions{
		Path: path, Selector: "/all/children/pgsql/hosts/10.0.0.1/pg_role", Editor: "test-editor",
	})
	if result == nil || !result.Success {
		t.Fatalf("targeted repair failed: %+v", result)
	}
	repaired, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(repaired), "pg_role: leader") != 1 || strings.Count(string(repaired), "pg_role: primary") != 1 {
		t.Fatalf("targeted repair bytes = %s", repaired)
	}

	// Editing the whole first host while leaving its pre-existing bad role must
	// fail even though the same error is allowed to remain on the other host.
	runInventoryEditorFn = func(_ string, fragment string) error {
		content, err := os.ReadFile(fragment)
		if err != nil {
			return err
		}
		return os.WriteFile(fragment, bytes.Replace(content, []byte("pg_seq: 2"), []byte("pg_seq: 3"), 1), 0600)
	}
	result = Edit(EditOptions{
		Path: path, Selector: "/all/children/pgsql/hosts/10.0.0.2", Editor: "test-editor",
	})
	if result == nil || result.Success || result.Code != output.CodeInventoryInvalid {
		t.Fatalf("selected error was not blocked: %+v", result)
	}
	retained := strings.TrimPrefix(result.Detail, "edited fragment retained at ")
	if retained != result.Detail {
		defer os.RemoveAll(filepath.Dir(retained))
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, repaired) {
		t.Fatal("blocked targeted edit changed the Inventory")
	}
}

func TestEditFromStdinIsNonInteractiveAndStructuredSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	if err := os.WriteFile(path, []byte(validEditInventory), 0644); err != nil {
		t.Fatal(err)
	}
	originalRunner, originalFormat := runInventoryEditorFn, config.OutputFormat
	defer func() { runInventoryEditorFn, config.OutputFormat = originalRunner, originalFormat }()
	config.OutputFormat = config.OUTPUT_JSON
	runInventoryEditorFn = func(string, string) error {
		return errors.New("editor must not run for --from")
	}
	result := Edit(EditOptions{
		Path: path, Selector: "vars.region", From: "-", Stdin: strings.NewReader("china\n"),
	})
	if result == nil || !result.Success {
		t.Fatalf("Edit --from - failed: %+v", result)
	}
	data, ok := result.Data.(*EditData)
	if !ok || data.Kind != InventoryEditKind || !data.Changed || len(data.Changes.Changes) == 0 || data.Changes.Changes[0].Path != "/all/vars/region" {
		t.Fatalf("Edit --from data = %#v", result.Data)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("region: china")) {
		t.Fatalf("Edit --from bytes = %s", body)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("Edit --from mode info=%v err=%v", info, err)
	}
}

func TestEditFromRejectsReplacementSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pigsty.yml")
	replacement := filepath.Join(dir, "replacement.yml")
	link := filepath.Join(dir, "replacement-link.yml")
	if err := os.WriteFile(path, []byte(validEditInventory), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("china\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, link); err != nil {
		t.Fatal(err)
	}
	result := Edit(EditOptions{Path: path, Selector: "vars.region", From: link})
	if result == nil || result.Success || result.Code != output.CodeInventoryEditInvalidArgs {
		t.Fatalf("replacement symlink result = %+v", result)
	}
}

func TestEditFromExplicitEmptyValueIsInvalid(t *testing.T) {
	result := Edit(EditOptions{Path: "unused.yml", FromSet: true})
	if result == nil || result.Success || result.Code != output.CodeInventoryEditInvalidArgs {
		t.Fatalf("explicit empty --from result = %+v", result)
	}
}

func TestEditRejectsStructuredOutput(t *testing.T) {
	original := config.OutputFormat
	defer func() { config.OutputFormat = original }()
	config.OutputFormat = config.OUTPUT_JSON
	result := Edit(EditOptions{Path: "unused.yml"})
	if result.Success || result.Code != output.CodeInventoryEditInvalidArgs {
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
	if result.Success || result.Code != output.CodeInventoryValidationFailed {
		t.Fatalf("ValidateInventory result = %+v", result)
	}
	data, ok := result.Data.(*ValidateData)
	if !ok {
		t.Fatalf("validation data type = %T", result.Data)
	}
	if data.Kind != "pig.inventory.validate/v1" {
		t.Fatalf("validation kind = %q, want canonical root kind", data.Kind)
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
	if result := ValidateInventory(ValidateOptions{Path: dir}); result.Success || result.Code != output.CodeInventoryValidateInvalidArgs {
		t.Fatalf("directory validation result = %+v", result)
	}

	dynamic := filepath.Join(t.TempDir(), "dynamic.yml")
	if err := os.WriteFile(dynamic, []byte("#!/bin/sh\necho all:\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if result := ValidateInventory(ValidateOptions{Path: dynamic}); result.Success || result.Code != output.CodeInventoryValidateInvalidArgs {
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
	root := inventoryConfTestRoot(t)
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

func inventoryConfTestRoot(t *testing.T) string {
	t.Helper()
	if configured := os.Getenv("PIGSTY_CONF_DIR"); configured != "" {
		info, err := os.Stat(configured)
		if err != nil || !info.IsDir() {
			t.Fatalf("PIGSTY_CONF_DIR %q is not a directory: %v", configured, err)
		}
		return filepath.Clean(configured)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "pigsty", "conf")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate)
		}
	}
	candidate := filepath.Join("..", "..", "..", "pigsty", "conf")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return filepath.Clean(candidate)
	}
	t.Skip("Pigsty conf corpus is unavailable; set PIGSTY_CONF_DIR to enable it")
	return ""
}

func TestEditChangeSetDoesNotContainValues(t *testing.T) {
	data := (&EditData{Changes: InventoryChangeSet{Changes: []InventoryChange{{Operation: "set", Identifier: "region"}}}}).Text()
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
	depth := 2
	bounded := ListInventory(InventoryOptions{Path: path, Depth: &depth})
	boundedData := bounded.Data.(*InventoryListData)
	for _, item := range boundedData.Items {
		if strings.HasPrefix(item.Path, "/all/children/") || strings.HasPrefix(item.Path, "/all/vars/") {
			t.Fatalf("depth=2 traversed too deeply: %+v", boundedData.Items)
		}
	}
	negative := -1
	if result := ListInventory(InventoryOptions{Path: path, Depth: &negative}); result.Success || result.Code != output.CodeInventoryListInvalidArgs {
		t.Fatalf("negative depth result = %+v", result)
	}
}

func TestDiffInventoryEmitsPathsAndKindsWithoutValues(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.yml")
	right := filepath.Join(dir, "right.yml")
	if err := os.WriteFile(left, []byte("all:\n  children: {}\n  vars: { password: old-secret, ports: [1, 2] }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("all:\n  vars: { ports: [2, 1], password: new-secret }\n  children: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := DiffInventory(DiffOptions{Path: left, OtherPath: right})
	if result == nil || !result.Success {
		t.Fatalf("DiffInventory failed: %+v", result)
	}
	data, ok := result.Data.(*InventoryDiffData)
	if !ok || data.Kind != InventoryDiffKind || len(data.Changes) != 2 {
		t.Fatalf("diff data = %#v", result.Data)
	}
	for _, change := range data.Changes {
		if change.Path != "/all/vars/password" && change.Path != "/all/vars/ports" {
			t.Fatalf("unexpected diff item = %+v", change)
		}
	}
	for _, format := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON, config.OUTPUT_YAML} {
		rendered, err := result.Render(format)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(rendered, []byte("old-secret")) || bytes.Contains(rendered, []byte("new-secret")) || bytes.Contains(rendered, []byte("[1, 2]")) {
			t.Fatalf("%s diff leaked values: %s", format, rendered)
		}
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
	if len(body) != 0 || failure == nil || failure.Code != output.CodeInventoryShowInvalidArgs || failure.ExitCode() != 2 {
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
	if result := ListInventory(InventoryOptions{Path: directory}); result.Success || result.Code != output.CodeInventoryListInvalidArgs {
		t.Fatalf("directory list result = %+v", result)
	}
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()
	config.OutputFormat = config.OUTPUT_TEXT
	if body, result := ShowInventory(InventoryOptions{Path: directory}); len(body) != 0 || result == nil || result.Code != output.CodeInventoryShowInvalidArgs {
		t.Fatalf("directory show body=%q result=%+v", body, result)
	}

	executable := filepath.Join(t.TempDir(), "inventory.py")
	if err := os.WriteFile(executable, []byte("#!/usr/bin/env python3\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if result := ListInventory(InventoryOptions{Path: executable}); result.Success || result.Code != output.CodeInventoryListInvalidArgs || !strings.Contains(result.Message, "dynamic") {
		t.Fatalf("dynamic list result = %+v", result)
	}
	if result := Edit(EditOptions{Path: directory, Editor: "test-editor"}); result.Success || result.Code != output.CodeInventoryEditInvalidArgs {
		t.Fatalf("directory edit result = %+v", result)
	}
}
