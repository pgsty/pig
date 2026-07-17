package inventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

var (
	ErrStaleFragment       = errors.New("inventory fragment is stale")
	ErrFragmentUnsupported = errors.New("inventory fragment cannot be edited losslessly")
)

// FragmentKind describes the temporary YAML view presented to an editor.
type FragmentKind string

const (
	DocumentFragment     FragmentKind = "document"
	MappingBodyFragment  FragmentKind = "mapping-body"
	SequenceBodyFragment FragmentKind = "sequence-body"
	LiteralFragment      FragmentKind = "literal"
	BlockScalarFragment  FragmentKind = "block-scalar"
)

// Fragment is a revision-bound, dedented view of one YAML value. Private span
// metadata prevents callers from forging a replacement range.
type Fragment struct {
	Path       Path         `json:"path,omitempty" yaml:"path,omitempty"`
	Kind       FragmentKind `json:"kind" yaml:"kind"`
	Bytes      []byte       `json:"bytes" yaml:"bytes"`
	BaseIndent int          `json:"base_indent" yaml:"base_indent"`
	Range      SourceRange  `json:"range" yaml:"range"`
	Revision   string       `json:"revision" yaml:"revision"`

	span     byteSpan
	original []byte
	implicit bool
	lineEnd  bool
}

// Revision returns a content hash suitable for optimistic concurrency checks.
func (d *Document) Revision() string {
	if d == nil {
		return ""
	}
	sum := sha256.Sum256(d.source)
	return hex.EncodeToString(sum[:])
}

// Extract returns a temporary editor view. Block mappings and sequences are
// exposed as dedented bodies; scalar and flow values retain their exact YAML
// literal. An empty path selects the complete document.
func (d *Document) Extract(path Path) (*Fragment, error) {
	if d == nil {
		return nil, errors.New("inventory document is nil")
	}
	revision := d.Revision()
	if len(path) == 0 {
		content := bytes.Clone(d.source)
		return &Fragment{Path: nil, Kind: DocumentFragment, Bytes: content, Range: d.sourceRange(byteSpan{0, len(d.source)}), Revision: revision, span: byteSpan{0, len(d.source)}, original: bytes.Clone(content)}, nil
	}
	if hasMixedNewlines(d.source) {
		return nil, fmt.Errorf("extract %s: mixed newline styles: %w", path.String(), ErrFragmentUnsupported)
	}
	ref, ok := d.refs[pathKey(path)]
	if !ok || ref == nil || ref.node == nil {
		return nil, fmt.Errorf("extract %s: %w", path.String(), ErrPathNotFound)
	}
	if protectedFragmentNode(ref.node) {
		return nil, fmt.Errorf("extract %s: anchor, alias, or explicit tag: %w", path.String(), ErrFragmentUnsupported)
	}
	span := ref.valueSpan
	kind := LiteralFragment
	indent := 0
	implicit := false
	lineEnd := false
	if isNullNode(ref.node) && ref.key != nil && span.start == span.end && implicitMappingPath(path) {
		lineStart, ok := d.offsetAt(ref.key.Line, 1)
		if !ok {
			return nil, fmt.Errorf("extract %s: invalid implicit mapping position: %w", path.String(), ErrFragmentUnsupported)
		}
		contentEnd := d.lineEnd(lineStart, false)
		insertAt := d.lineEnd(lineStart, true)
		span = byteSpan{start: insertAt, end: insertAt}
		kind = MappingBodyFragment
		indent = max(ref.key.Column+1, 0)
		implicit = true
		lineEnd = insertAt > contentEnd
	}
	switch ref.node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		if ref.node.Style&yaml.FlowStyle != 0 || len(ref.node.Content) == 0 {
			if bytes.IndexAny(d.source[span.start:span.end], "\r\n") >= 0 {
				return nil, fmt.Errorf("extract %s: multiline flow collection: %w", path.String(), ErrFragmentUnsupported)
			}
			break
		}
		kind = MappingBodyFragment
		if ref.node.Kind == yaml.SequenceNode {
			kind = SequenceBodyFragment
		}
		span = d.blockBodySpan(ref.node)
		line := d.positionAt(span.start).Line
		indent = span.start - d.lineStarts[line-1]
	case yaml.ScalarNode:
		if ref.node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			kind = BlockScalarFragment
		}
	}
	if !span.valid(len(d.source)) {
		return nil, fmt.Errorf("extract %s: invalid source range: %w", path.String(), ErrFragmentUnsupported)
	}
	raw := bytes.Clone(d.source[span.start:span.end])
	view := raw
	if kind == MappingBodyFragment || kind == SequenceBodyFragment {
		view = dedentBody(raw, indent)
	}
	return &Fragment{
		Path: clonePath(path), Kind: kind, Bytes: bytes.Clone(view), BaseIndent: indent,
		Range: d.sourceRange(span), Revision: revision, span: span, original: bytes.Clone(view),
		implicit: implicit, lineEnd: lineEnd,
	}, nil
}

// implicitMappingPath identifies inventory containers whose omitted value is
// an empty mapping rather than an editable scalar null.
func implicitMappingPath(path Path) bool {
	last, ok := path.Last()
	if !ok || last.Kind != KeySegment {
		return false
	}
	if last.Key == "vars" || last.Key == "hosts" || last.Key == "children" {
		return true
	}
	parent, ok := path.Parent()
	if !ok {
		return false
	}
	container, ok := parent.Last()
	return ok && container.Kind == KeySegment && container.Key == "children"
}

func (d *Document) blockBodySpan(node *yaml.Node) byteSpan {
	span := d.nodeSpan(node)
	if span.end > span.start && span.end <= len(d.source) && d.source[span.end-1] != '\n' && d.source[span.end-1] != '\r' {
		span.end = d.lineEnd(span.end, true)
	}
	return span
}

func protectedFragmentNode(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return true
	}
	for _, child := range node.Content {
		if protectedFragmentNode(child) {
			return true
		}
	}
	return false
}

// ReplaceFragment stages one revision-checked source-range replacement.
func (e *Editor) ReplaceFragment(fragment *Fragment, edited []byte) (err error) {
	defer e.captureFailure(&err)
	if e == nil || e.base == nil {
		return errors.New("inventory editor is nil")
	}
	if fragment == nil || !fragment.span.valid(len(e.base.source)) {
		return errors.New("inventory fragment is invalid")
	}
	if fragment.Revision == "" || fragment.Revision != e.base.Revision() {
		return ErrStaleFragment
	}
	if bytes.Equal(edited, fragment.original) {
		return nil
	}
	if len(bytes.TrimSpace(edited)) == 0 {
		return fmt.Errorf("empty fragment: %w", ErrFragmentUnsupported)
	}
	replacement := bytes.Clone(edited)
	if fragment.Kind == MappingBodyFragment || fragment.Kind == SequenceBodyFragment {
		var normalizeErr error
		replacement, normalizeErr = normalizeFragmentNewlines(replacement, e.base.newline)
		if normalizeErr != nil {
			return normalizeErr
		}
		replacement = stripCommonIndent(replacement)
		if err := validateFragmentShape(fragment.Kind, replacement); err != nil {
			return err
		}
		original := e.base.source[fragment.span.start:fragment.span.end]
		if fragment.implicit && fragment.lineEnd {
			original = []byte(e.base.newline)
		}
		replacement = matchTerminalNewline(replacement, original, e.base.newline)
		replacement = reindentBody(replacement, fragment.BaseIndent)
		if fragment.implicit {
			replacement = append(bytes.Repeat([]byte(" "), fragment.BaseIndent), replacement...)
			if !fragment.lineEnd {
				replacement = append([]byte(e.base.newline), replacement...)
			}
		}
	} else if fragment.Kind == BlockScalarFragment {
		if err := validateFragmentShape(fragment.Kind, replacement); err != nil {
			return err
		}
	} else {
		var normalizeErr error
		replacement, normalizeErr = normalizeFragmentNewlines(replacement, e.base.newline)
		if normalizeErr != nil {
			return normalizeErr
		}
		if err := validateFragmentShape(fragment.Kind, replacement); err != nil {
			return err
		}
		replacement = matchTerminalNewline(replacement, e.base.source[fragment.span.start:fragment.span.end], e.base.newline)
	}
	return e.stageChange("replace-fragment", fragment.Path, fragment.span, replacement, finalIdentifier(fragment.Path))
}

func validateFragmentShape(kind FragmentKind, source []byte) error {
	if kind == DocumentFragment {
		return nil
	}
	root, err := decodeOneYAMLDocument(source)
	if err != nil {
		return fmt.Errorf("edited fragment is invalid YAML: %w", err)
	}
	switch kind {
	case MappingBodyFragment:
		if root.Kind != yaml.MappingNode {
			return fmt.Errorf("edited fragment must remain a mapping: %w", ErrFragmentUnsupported)
		}
	case SequenceBodyFragment:
		if root.Kind != yaml.SequenceNode {
			return fmt.Errorf("edited fragment must remain a sequence: %w", ErrFragmentUnsupported)
		}
	}
	return nil
}

func dedentBody(source []byte, indent int) []byte {
	if indent <= 0 || len(source) == 0 {
		return bytes.Clone(source)
	}
	lines := bytes.SplitAfter(source, []byte("\n"))
	var output bytes.Buffer
	for index, line := range lines {
		if index > 0 {
			remove := min(indent, leadingSpaces(line))
			line = line[remove:]
		}
		output.Write(line)
	}
	return output.Bytes()
}

func reindentBody(source []byte, indent int) []byte {
	if indent <= 0 || len(source) == 0 {
		return source
	}
	padding := bytes.Repeat([]byte(" "), indent)
	lines := bytes.SplitAfter(source, []byte("\n"))
	var output bytes.Buffer
	for index, line := range lines {
		if index > 0 && len(line) > 0 {
			output.Write(padding)
		}
		output.Write(line)
	}
	return output.Bytes()
}

func stripCommonIndent(source []byte) []byte {
	lines := bytes.SplitAfter(source, []byte("\n"))
	common := -1
	for _, line := range lines {
		content := bytes.TrimSuffix(line, []byte("\n"))
		content = bytes.TrimSuffix(content, []byte("\r"))
		if len(bytes.TrimSpace(content)) == 0 {
			continue
		}
		spaces := leadingSpaces(content)
		if common < 0 || spaces < common {
			common = spaces
		}
	}
	if common <= 0 {
		return source
	}
	var output bytes.Buffer
	for _, line := range lines {
		line = line[min(common, leadingSpaces(line)):]
		output.Write(line)
	}
	return output.Bytes()
}

func leadingSpaces(line []byte) int {
	count := 0
	for count < len(line) && line[count] == ' ' {
		count++
	}
	return count
}

func normalizeFragmentNewlines(source []byte, newline string) ([]byte, error) {
	if bytes.IndexByte(bytes.ReplaceAll(source, []byte("\r\n"), nil), '\r') >= 0 {
		return nil, fmt.Errorf("unsupported carriage return in edited fragment: %w", ErrFragmentUnsupported)
	}
	normalized := bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
	if newline == "\r\n" {
		normalized = bytes.ReplaceAll(normalized, []byte("\n"), []byte("\r\n"))
	}
	return normalized, nil
}

func matchTerminalNewline(source, original []byte, newline string) []byte {
	want := bytes.HasSuffix(original, []byte("\n"))
	has := bytes.HasSuffix(source, []byte("\n"))
	if want && !has {
		return append(source, []byte(newline)...)
	}
	if !want && has {
		source = bytes.TrimSuffix(source, []byte("\n"))
		source = bytes.TrimSuffix(source, []byte("\r"))
	}
	return source
}

func hasMixedNewlines(source []byte) bool {
	hasCRLF := bytes.Contains(source, []byte("\r\n"))
	hasLF := false
	for index, value := range source {
		if value == '\n' && (index == 0 || source[index-1] != '\r') {
			hasLF = true
			break
		}
	}
	return hasCRLF && hasLF || bytes.Contains(bytes.ReplaceAll(source, []byte("\r\n"), nil), []byte{'\r'})
}
