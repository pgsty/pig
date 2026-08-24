package inventorycli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	From     string
	FromSet  bool
	Stdin    io.Reader
}

const InventoryEditKind = "pig.inventory.edit/v1"

// EditData is safe structured metadata; it never contains inventory values.
type EditData struct {
	Kind          string                `json:"kind,omitempty" yaml:"kind,omitempty"`
	Path          string                `json:"path" yaml:"path"`
	Selector      string                `json:"selector,omitempty" yaml:"selector,omitempty"`
	Resolved      string                `json:"resolved" yaml:"resolved"`
	Changed       bool                  `json:"changed" yaml:"changed"`
	ModeBefore    string                `json:"mode_before,omitempty" yaml:"mode_before,omitempty"`
	ModeAfter     string                `json:"mode_after,omitempty" yaml:"mode_after,omitempty"`
	ModeTightened bool                  `json:"mode_tightened,omitempty" yaml:"mode_tightened,omitempty"`
	Changes       InventoryChangeSet    `json:"changes,omitempty" yaml:"changes,omitempty"`
	Diagnostics   []InventoryDiagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

func (d *EditData) Text() string {
	if d == nil {
		return ""
	}
	state := "unchanged"
	if d.Changed {
		state = "updated"
	}
	lines := []string{fmt.Sprintf("  %s %s (%s)", state, d.Path, d.Resolved)}
	if d.ModeTightened {
		lines = append(lines, fmt.Sprintf("  notice: permissions tightened from %s to %s", d.ModeBefore, d.ModeAfter))
	}
	return strings.Join(lines, "\n")
}

var (
	runInventoryEditorFn = runInventoryEditor
	stdinIsTerminalFn    = utils.StdinIsTerminal
)

// Edit opens a complete inventory or a dedented selector fragment in the
// user's editor and atomically writes a validated candidate when it changed.
func Edit(opts EditOptions) *output.Result {
	ctx, failure := prepareInventoryEdit(opts)
	if failure != nil {
		return failure
	}
	tempDir, err := os.MkdirTemp("", "pig-sty-edit-*")
	if err != nil {
		return output.Fail(output.CodeInventoryEditFailed, "failed to create private edit directory")
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()
	if err := os.Chmod(tempDir, 0700); err != nil {
		return output.Fail(output.CodeInventoryEditFailed, "failed to secure edit directory")
	}
	tempPath := filepath.Join(tempDir, "fragment.yml")
	if err := os.WriteFile(tempPath, ctx.fragment.Bytes, 0600); err != nil {
		return output.Fail(output.CodeInventoryEditFailed, "failed to write private edit fragment")
	}
	if failure, retain := populateEditFragment(opts, ctx, tempPath); failure != nil {
		removeTemp = !retain
		return failure
	}
	edited, err := os.ReadFile(tempPath)
	if err != nil {
		removeTemp = false
		return retainedEditFailure(output.CodeInventoryEditFailed, "failed to read edited fragment", tempPath)
	}
	result, retain := finishInventoryEdit(ctx, edited, tempPath)
	if retain {
		removeTemp = false
	}
	return result
}

type inventoryEditContext struct {
	path           string
	source         []byte
	doc            *inventory.Document
	selected       inventory.Path
	selector       string
	fragment       *inventory.Fragment
	nonInteractive bool
	editorCommand  string
}

func prepareInventoryEdit(opts EditOptions) (*inventoryEditContext, *output.Result) {
	if opts.FromSet && strings.TrimSpace(opts.From) == "" {
		return nil, output.Fail(output.CodeInventoryEditInvalidArgs, "--from requires one regular file path or -")
	}
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return nil, output.Fail(output.CodeInventoryNotFound, "inventory path is not configured")
	}
	nonInteractive := opts.FromSet || strings.TrimSpace(opts.From) != ""
	if config.IsStructuredOutput() && !nonInteractive {
		return nil, output.Fail(output.CodeInventoryEditInvalidArgs, "interactive inventory edit requires text output")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, output.Fail(output.CodeInventoryNotFound, fmt.Sprintf("inventory not found: %s", path))
	}
	if err != nil {
		return nil, output.Fail(output.CodeInventoryEditFailed, fmt.Sprintf("failed to inspect inventory: %v", err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, output.Fail(output.CodeInventoryEditInvalidArgs, "refusing to edit an inventory symlink")
	}
	if failure := staticInventorySourceFailure(info, output.CodeInventoryEditInvalidArgs); failure != nil {
		return nil, failure
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, output.Fail(output.CodeInventoryEditFailed, fmt.Sprintf("failed to read inventory: %v", err))
	}
	doc, err := inventory.Parse(path, source)
	if err != nil {
		return nil, output.Fail(output.CodeInventoryInvalid, "inventory cannot be edited until its YAML parse errors are fixed").WithDetail(parseErrorSummary(err))
	}
	selected, err := inventory.ParseSelector(doc, opts.Selector)
	if err != nil {
		return nil, output.Fail(output.CodeInventoryEditInvalidArgs, err.Error())
	}
	fragment, err := doc.Extract(selected)
	if err != nil {
		return nil, output.Fail(output.CodeInventoryEditFailed, err.Error())
	}
	editorCommand, failure := resolveInventoryEditor(opts, nonInteractive)
	if failure != nil {
		return nil, failure
	}
	return &inventoryEditContext{
		path: path, source: source, doc: doc, selected: selected, selector: strings.TrimSpace(opts.Selector), fragment: fragment,
		nonInteractive: nonInteractive, editorCommand: editorCommand,
	}, nil
}

func resolveInventoryEditor(opts EditOptions, nonInteractive bool) (string, *output.Result) {
	if nonInteractive {
		return "", nil
	}
	for _, candidate := range []string{opts.Editor, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if command := strings.TrimSpace(candidate); command != "" {
			return command, nil
		}
	}
	if !stdinIsTerminalFn() {
		return "", output.Fail(output.CodeInventoryEditInvalidArgs, "VISUAL or EDITOR is required when stdin is not a terminal")
	}
	return "vi", nil
}

func populateEditFragment(opts EditOptions, ctx *inventoryEditContext, tempPath string) (*output.Result, bool) {
	if ctx.nonInteractive {
		replacement, failure := readEditReplacement(opts)
		if failure != nil {
			return failure, false
		}
		if err := os.WriteFile(tempPath, replacement, 0600); err != nil {
			return output.Fail(output.CodeInventoryEditFailed, "failed to stage replacement fragment"), false
		}
		return nil, false
	}
	if err := runInventoryEditorFn(ctx.editorCommand, tempPath); err != nil {
		return retainedEditFailure(output.CodeInventoryEditFailed, "inventory editor exited unsuccessfully", tempPath), true
	}
	return nil, false
}

func finishInventoryEdit(ctx *inventoryEditContext, edited []byte, tempPath string) (*output.Result, bool) {
	data := &EditData{Kind: InventoryEditKind, Path: ctx.path, Selector: ctx.selector, Resolved: ctx.selected.String()}
	originalDiagnostics := editRuleDiagnostics(ctx.doc)
	if bytes.Equal(edited, ctx.fragment.Bytes) {
		if blocking := editBlockingDiagnostics(originalDiagnostics, originalDiagnostics, ctx.selected); len(blocking) > 0 {
			failure := retainedEditFailure(output.CodeInventoryInvalid, firstDiagnosticMessage(blocking), tempPath).
				WithData(editFailureData(data, originalDiagnostics))
			return failure, true
		}
		return output.OK("inventory unchanged", data), false
	}
	editor := ctx.doc.Edit()
	if err := editor.ReplaceFragment(ctx.fragment, edited); err != nil {
		return retainedEditFailure(output.CodeInventoryInvalid, fragmentEditMessage(err), tempPath), true
	}
	candidate, changes, err := editor.CommitCandidate()
	if err != nil {
		return retainedEditFailure(output.CodeInventoryInvalid, editValidationMessage(err), tempPath), true
	}
	diagnostics := editRuleDiagnostics(candidate)
	blocking := editBlockingDiagnostics(originalDiagnostics, diagnostics, ctx.selected)
	if len(blocking) > 0 {
		failure := retainedEditFailure(output.CodeInventoryInvalid, firstDiagnosticMessage(blocking), tempPath).
			WithData(editFailureData(data, diagnostics))
		return failure, true
	}
	latestInfo, err := os.Lstat(ctx.path)
	if err != nil || latestInfo.Mode()&os.ModeSymlink != 0 || !latestInfo.Mode().IsRegular() {
		return retainedEditFailure(output.CodeInventoryEditConflict, "inventory source changed shape while the editor was open; no file was written", tempPath), true
	}
	latest, err := os.ReadFile(ctx.path)
	if err != nil || !bytes.Equal(latest, ctx.source) {
		return retainedEditFailure(output.CodeInventoryEditConflict, "inventory changed while the editor was open; no file was written", tempPath), true
	}
	modeBefore := latestInfo.Mode().Perm()
	modeAfter := modeBefore & 0600
	if err := utils.AtomicWriteFile(ctx.path, candidate.Bytes(), modeAfter); err != nil {
		return retainedEditFailure(output.CodeInventoryEditWriteFailed, fmt.Sprintf("failed to write edited inventory: %v", err), tempPath), true
	}
	data.Changed = true
	data.ModeBefore = fmt.Sprintf("%04o", modeBefore)
	data.ModeAfter = fmt.Sprintf("%04o", modeAfter)
	data.ModeTightened = modeBefore != modeAfter
	data.Changes = inventoryChanges(changes)
	data.Diagnostics = inventoryDiagnostics(diagnostics)
	return output.OK("inventory updated", data), false
}

func editFailureData(data *EditData, diagnostics []inventory.Diagnostic) *EditData {
	return &EditData{
		Kind: data.Kind, Path: data.Path, Selector: data.Selector, Resolved: data.Resolved,
		Diagnostics: inventoryDiagnostics(diagnostics),
	}
}

func readEditReplacement(opts EditOptions) ([]byte, *output.Result) {
	from := strings.TrimSpace(opts.From)
	if from == "-" {
		reader := opts.Stdin
		if reader == nil {
			reader = os.Stdin
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			return nil, output.Fail(output.CodeInventoryEditFailed, "failed to read replacement fragment from stdin")
		}
		return body, nil
	}
	if from == "" {
		return nil, output.Fail(output.CodeInventoryEditInvalidArgs, "replacement source is not configured")
	}
	info, err := os.Lstat(from)
	if errors.Is(err, os.ErrNotExist) {
		return nil, output.Fail(output.CodeInventoryNotFound, "replacement fragment file was not found")
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, output.Fail(output.CodeInventoryEditInvalidArgs, "replacement source must be one regular, non-symlink file")
	}
	body, err := os.ReadFile(from)
	if err != nil {
		return nil, output.Fail(output.CodeInventoryEditFailed, "failed to read replacement fragment file")
	}
	return body, nil
}

func editRuleDiagnostics(doc *inventory.Document) []inventory.Diagnostic {
	if doc == nil {
		return nil
	}
	return doc.ValidateComplete()
}

func editBlockingDiagnostics(original, candidate []inventory.Diagnostic, selected inventory.Path) []inventory.Diagnostic {
	originalErrors := make(map[string]struct{})
	for _, diagnostic := range original {
		if diagnostic.Severity == inventory.SeverityError {
			originalErrors[diagnostic.Code+"\x00"+diagnostic.Path.String()] = struct{}{}
		}
	}
	blocking := make([]inventory.Diagnostic, 0)
	for _, diagnostic := range candidate {
		if diagnostic.Severity != inventory.SeverityError {
			continue
		}
		_, existed := originalErrors[diagnostic.Code+"\x00"+diagnostic.Path.String()]
		if !existed || inventoryPathWithin(diagnostic.Path, selected) {
			blocking = append(blocking, diagnostic)
		}
	}
	return blocking
}

func inventoryPathWithin(path, root inventory.Path) bool {
	if len(root) > len(path) {
		return false
	}
	for index := range root {
		if root[index] != path[index] {
			return false
		}
	}
	return true
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
