package sty

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"pig/inventory"
	"strings"
)

// EditOptions controls one interactive, revision-checked inventory edit.
type EditOptions struct {
	Path     string
	Selector string
	Editor   string // optional test/embedding override
}

// EditData is safe structured metadata; it never contains inventory values.
type EditData struct {
	Path        string                 `json:"path" yaml:"path"`
	Selector    string                 `json:"selector,omitempty" yaml:"selector,omitempty"`
	Resolved    string                 `json:"resolved" yaml:"resolved"`
	Changed     bool                   `json:"changed" yaml:"changed"`
	Changes     inventory.ChangeSet    `json:"changes,omitempty" yaml:"changes,omitempty"`
	Diagnostics []inventory.Diagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

func (d *EditData) Text() string {
	if d == nil {
		return ""
	}
	state := "unchanged"
	if d.Changed {
		state = "updated"
	}
	return fmt.Sprintf("  %s %s (%s)", state, d.Path, d.Resolved)
}

var (
	runInventoryEditorFn = runInventoryEditor
	stdinIsTerminalFn    = stdinIsTerminal
)

// Edit opens a complete inventory or a dedented selector fragment in the
// user's editor and atomically writes a validated candidate when it changed.
func Edit(opts EditOptions) *output.Result {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return output.Fail(output.CodeStyInventoryNotFound, "inventory path is not configured")
	}
	if config.IsStructuredOutput() {
		return output.Fail(output.CodeStyEditInvalidArgs, "interactive inventory edit requires text output")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return output.Fail(output.CodeStyInventoryNotFound, fmt.Sprintf("inventory not found: %s", path))
	}
	if err != nil {
		return output.Fail(output.CodeStyEditFailed, fmt.Sprintf("failed to inspect inventory: %v", err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return output.Fail(output.CodeStyEditInvalidArgs, "refusing to edit an inventory symlink")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return output.Fail(output.CodeStyEditFailed, fmt.Sprintf("failed to read inventory: %v", err))
	}
	doc, err := inventory.Parse(path, source)
	if err != nil {
		return output.Fail(output.CodeStyInventoryInvalid, "inventory cannot be edited until its YAML parse errors are fixed").WithDetail(parseErrorSummary(err))
	}
	selected, err := inventory.ParseSelector(doc, opts.Selector)
	if err != nil {
		return output.Fail(output.CodeStyEditInvalidArgs, err.Error())
	}
	fragment, err := doc.Extract(selected)
	if err != nil {
		return output.Fail(output.CodeStyEditFailed, err.Error())
	}

	editorCommand := strings.TrimSpace(opts.Editor)
	if editorCommand == "" {
		editorCommand = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if editorCommand == "" {
		editorCommand = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editorCommand == "" {
		if !stdinIsTerminalFn() {
			return output.Fail(output.CodeStyEditInvalidArgs, "VISUAL or EDITOR is required when stdin is not a terminal")
		}
		editorCommand = "vi"
	}

	tempDir, err := os.MkdirTemp("", "pig-sty-edit-*")
	if err != nil {
		return output.Fail(output.CodeStyEditFailed, "failed to create private edit directory")
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()
	if err := os.Chmod(tempDir, 0700); err != nil {
		return output.Fail(output.CodeStyEditFailed, "failed to secure edit directory")
	}
	tempPath := filepath.Join(tempDir, "fragment.yml")
	if err := os.WriteFile(tempPath, fragment.Bytes, 0600); err != nil {
		return output.Fail(output.CodeStyEditFailed, "failed to write private edit fragment")
	}
	if err := runInventoryEditorFn(editorCommand, tempPath); err != nil {
		removeTemp = false
		return retainedEditFailure(output.CodeStyEditFailed, "inventory editor exited unsuccessfully", tempPath)
	}
	edited, err := os.ReadFile(tempPath)
	if err != nil {
		removeTemp = false
		return retainedEditFailure(output.CodeStyEditFailed, "failed to read edited fragment", tempPath)
	}
	data := &EditData{Path: path, Selector: strings.TrimSpace(opts.Selector), Resolved: selected.String()}
	if bytes.Equal(edited, fragment.Bytes) {
		return output.OK("inventory unchanged", data)
	}

	editor := doc.Edit()
	if err := editor.ReplaceFragment(fragment, edited); err != nil {
		removeTemp = false
		return retainedEditFailure(output.CodeStyInventoryInvalid, fragmentEditMessage(err), tempPath)
	}
	candidate, changes, err := editor.Commit()
	if err != nil {
		removeTemp = false
		return retainedEditFailure(output.CodeStyInventoryInvalid, editValidationMessage(err), tempPath)
	}
	diagnostics := candidate.Validate()
	if inventory.HasErrors(diagnostics) {
		removeTemp = false
		return retainedEditFailure(output.CodeStyInventoryInvalid, firstDiagnosticMessage(diagnostics), tempPath).
			WithData(&EditData{Path: path, Selector: data.Selector, Resolved: data.Resolved, Diagnostics: diagnostics})
	}
	latest, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(latest, source) {
		removeTemp = false
		return retainedEditFailure(output.CodeStyEditConflict, "inventory changed while the editor was open; no file was written", tempPath)
	}
	if err := utils.AtomicWriteFile(path, candidate.Bytes(), 0600); err != nil {
		removeTemp = false
		return retainedEditFailure(output.CodeStyEditWriteFailed, fmt.Sprintf("failed to write edited inventory: %v", err), tempPath)
	}
	data.Changed = true
	data.Changes = changes
	data.Diagnostics = diagnostics
	return output.OK("inventory updated", data)
}

func retainedEditFailure(code int, message, tempPath string) *output.Result {
	return output.Fail(code, message).WithDetail("edited fragment retained at " + tempPath)
}

func runInventoryEditor(command, path string) error {
	cmd := exec.Command("/bin/sh", "-c", `exec `+command+` "$1"`, "pig-sty-edit", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func editValidationMessage(err error) string {
	var validation *inventory.ValidationError
	if errors.As(err, &validation) {
		return firstDiagnosticMessage(validation.Diagnostics)
	}
	return parseErrorSummary(err)
}

func fragmentEditMessage(err error) string {
	switch {
	case errors.Is(err, inventory.ErrStaleFragment):
		return "inventory changed before the edited fragment could be applied"
	case errors.Is(err, inventory.ErrFragmentUnsupported):
		return "edited fragment changes an unsupported YAML shape"
	default:
		return "edited fragment is not valid YAML for the selected inventory value"
	}
}

func firstDiagnosticMessage(diagnostics []inventory.Diagnostic) string {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == inventory.SeverityError {
			return fmt.Sprintf("%s at %s: %s", diagnostic.Code, diagnostic.Path.String(), diagnostic.Message)
		}
	}
	return "inventory validation failed"
}
