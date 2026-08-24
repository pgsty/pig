package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var (
	// ErrPathNotFound indicates that an edit target is not active in the source.
	ErrPathNotFound = errors.New("inventory path not found")
	// ErrEditConflict indicates overlapping source patches in one transaction.
	ErrEditConflict = errors.New("inventory edit conflict")
	// ErrProtectedNode indicates a v1-unsupported direct edit of a tagged,
	// anchored, aliased, or block-scalar node.
	ErrProtectedNode = errors.New("inventory node cannot be directly edited losslessly")
)

// Change contains identifiers and source paths only; old/new values are never
// retained, so ChangeSet is safe for password-bearing configure operations.
type Change struct {
	Operation  string `json:"operation" yaml:"operation"`
	Path       Path   `json:"path,omitempty" yaml:"path,omitempty"`
	Identifier string `json:"identifier,omitempty" yaml:"identifier,omitempty"`
	Count      int    `json:"count,omitempty" yaml:"count,omitempty"`
}

// ChangeSet summarizes a committed transaction without source values.
type ChangeSet struct {
	Changes []Change `json:"changes,omitempty" yaml:"changes,omitempty"`
}

// EditResult reports what a targeted source operation observed and changed.
// Paths are populated after a continuous Mutation reparses the candidate.
type EditResult struct {
	Matched int    `json:"matched" yaml:"matched"`
	Changed int    `json:"changed" yaml:"changed"`
	Paths   []Path `json:"paths,omitempty" yaml:"paths,omitempty"`
}

// ValidationError reports diagnostics from a candidate that was never made
// visible as a committed Document.
type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "inventory validation failed"
	}
	first := e.Diagnostics[0]
	for _, diagnostic := range e.Diagnostics {
		if diagnostic.Severity == SeverityError {
			first = diagnostic
			break
		}
	}
	return fmt.Sprintf("inventory validation failed: %s at %s: %s", first.Code, first.Path.String(), first.Message)
}

type sourcePatch struct {
	span        byteSpan
	replacement []byte
	order       int
}

// Editor stages a transaction against one immutable Document.
type Editor struct {
	base    *Document
	patches []sourcePatch
	changes []Change
	failed  error
}

// Edit starts a transaction. The source Document remains immutable.
func (d *Document) Edit() *Editor { return &Editor{base: d} }

// Set replaces only the target scalar lexeme. Whitespace, comments, padding,
// and all surrounding bytes remain untouched.
func (e *Editor) Set(path Path, value any) (err error) {
	defer e.captureFailure(&err)
	ref, err := e.reference(path)
	if err != nil {
		return err
	}
	if err := editableScalar(ref.node); err != nil {
		return fmt.Errorf("set %s: %w", path.String(), err)
	}
	replacement, err := renderExistingScalar(ref.node, value, ref.parent != nil && ref.parent.Style&yaml.FlowStyle != 0)
	if err != nil {
		return fmt.Errorf("set %s: %w", path.String(), err)
	}
	if err := e.stageChange("set", path, ref.valueSpan, []byte(replacement), finalIdentifier(path)); err != nil {
		return fmt.Errorf("set %s: %w", path.String(), err)
	}
	return nil
}

// Upsert sets an existing mapping entry or inserts it beneath its parent.
func (e *Editor) Upsert(path Path, value any, options ...InsertOption) (err error) {
	defer e.captureFailure(&err)
	if e == nil || e.base == nil {
		return errors.New("inventory editor is nil")
	}
	if _, ok := e.base.refs[pathKey(path)]; ok {
		return e.Set(path, value)
	}
	parent, ok := path.Parent()
	if !ok {
		return fmt.Errorf("upsert %s: %w", path.String(), ErrPathNotFound)
	}
	last, ok := path.Last()
	if !ok || last.Kind != KeySegment {
		return fmt.Errorf("upsert %s: final segment must be a mapping key", path.String())
	}
	return e.Insert(parent, last.Key, value, options...)
}

type insertOptions struct {
	after         string
	hasAfter      bool
	inlineComment string
}

// InsertOption customizes a mapping insertion.
type InsertOption func(*insertOptions)

// AfterKey inserts immediately after an existing sibling when possible.
func AfterKey(key string) InsertOption {
	return func(options *insertOptions) {
		options.after = key
		options.hasAfter = true
	}
}

// WithInlineComment appends one inline comment to a newly inserted entry.
// Existing entries and their comments are never rewritten by this option.
func WithInlineComment(comment string) InsertOption {
	return func(options *insertOptions) {
		options.inlineComment = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment), "#"))
	}
}

// Insert adds a new mapping entry while inheriting local flow/block style,
// indentation, colon padding, inline-comment alignment, newline form, and
// delimiter spacing.
func (e *Editor) Insert(parent Path, key string, value any, options ...InsertOption) (err error) {
	defer e.captureFailure(&err)
	if e == nil || e.base == nil {
		return errors.New("inventory editor is nil")
	}
	childPath := parent.ChildKey(key)
	if _, exists := e.base.refs[pathKey(childPath)]; exists {
		return fmt.Errorf("insert %s: key already exists", childPath.String())
	}
	parentRef, ok := e.base.refs[pathKey(parent)]
	if !ok {
		return fmt.Errorf("insert %s: %w", parent.String(), ErrPathNotFound)
	}
	if err := editableContainer(parentRef.node); err != nil {
		return fmt.Errorf("insert %s: %w", childPath.String(), err)
	}
	config := insertOptions{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("insert key must not be empty")
	}
	if strings.ContainsAny(config.inlineComment, "\r\n") {
		return fmt.Errorf("insert %s: inline comment must contain exactly one line", childPath.String())
	}
	keyText := renderKey(key)
	valueText, err := renderNewValue(value, parentRef.node.Style&yaml.FlowStyle != 0)
	if err != nil {
		return fmt.Errorf("insert %s: %w", childPath.String(), err)
	}
	var span byteSpan
	var fragment string
	var commentSpan byteSpan
	var commentFragment string
	if parentRef.node.Kind == yaml.MappingNode && parentRef.node.Style&yaml.FlowStyle != 0 {
		span, fragment, err = e.flowInsertion(parent, parentRef, config.after, config.hasAfter, keyText+": "+valueText)
		if err == nil && config.inlineComment != "" {
			commentSpan, commentFragment, err = e.base.flowCommentInsertion(parentRef.node, config.inlineComment)
		}
	} else {
		span, fragment, err = e.blockInsertion(parent, parentRef, config.after, config.hasAfter, keyText, valueText, config.inlineComment)
	}
	if err != nil {
		return fmt.Errorf("insert %s: %w", childPath.String(), err)
	}
	if err := e.stageChange("insert", childPath, span, []byte(fragment), key); err != nil {
		return fmt.Errorf("insert %s: %w", childPath.String(), err)
	}
	if commentFragment != "" {
		if _, err := e.stage(commentSpan, []byte(commentFragment)); err != nil {
			return fmt.Errorf("insert %s comment: %w", childPath.String(), err)
		}
	}
	return nil
}

func (e *Editor) flowInsertion(parent Path, parentRef *nodeRef, after string, hasAfter bool, entry string) (byteSpan, string, error) {
	span := e.base.nodeSpan(parentRef.node)
	if !span.valid(len(e.base.source)) || span.end <= span.start || e.base.source[span.end-1] != '}' {
		return byteSpan{}, "", errors.New("unable to locate flow mapping delimiter")
	}
	insertAt := span.end - 1
	prefix := ""
	if len(parentRef.node.Content) > 0 {
		for insertAt > span.start+1 && (e.base.source[insertAt-1] == ' ' || e.base.source[insertAt-1] == '\t') {
			insertAt--
		}
		prefix = e.base.flowSeparator(parentRef.node)
	} else if insertAt > span.start+1 {
		prefix = string(e.base.source[span.start+1 : insertAt])
		insertAt = span.start + 1
	} else {
		prefix = " "
		entry += " "
	}
	if hasAfter {
		anchorPath := parent.ChildKey(after)
		anchor, ok := e.base.refs[pathKey(anchorPath)]
		if !ok || anchor.parent != parentRef.node {
			return byteSpan{}, "", fmt.Errorf("after key %q: %w", after, ErrPathNotFound)
		}
		insertAt = e.base.nodeSpan(anchor.node).end
		prefix = e.base.flowSeparator(parentRef.node)
	}
	return byteSpan{start: insertAt, end: insertAt}, prefix + entry, nil
}

func (d *Document) flowSeparator(mapping *yaml.Node) string {
	for i := 0; i+3 < len(mapping.Content); i += 2 {
		leftEnd := d.nodeSpan(mapping.Content[i+1]).end
		rightStart := d.scalarSpan(mapping.Content[i+2]).start
		if leftEnd >= 0 && rightStart > leftEnd && rightStart <= len(d.source) {
			between := string(d.source[leftEnd:rightStart])
			if comma := strings.IndexByte(between, ','); comma >= 0 {
				return between[comma:]
			}
		}
	}
	return ", "
}

func (d *Document) flowCommentInsertion(mapping *yaml.Node, comment string) (byteSpan, string, error) {
	span := d.nodeSpan(mapping)
	if !span.valid(len(d.source)) {
		return byteSpan{}, "", errors.New("unable to locate flow mapping")
	}
	line := d.positionAt(span.end).Line
	lineStart, ok := d.offsetAt(line, 1)
	if !ok {
		return byteSpan{}, "", errors.New("unable to locate flow mapping line")
	}
	lineEnd := d.lineEnd(lineStart, false)
	if lineEnd > span.end && d.source[lineEnd-1] == '\r' {
		lineEnd--
	}
	if d.inlineCommentColumn(line, span.end) > 0 {
		return byteSpan{}, "", nil
	}
	if lineEnd < span.end || !horizontalWhitespace(d.source[span.end:lineEnd]) {
		return byteSpan{}, "", errors.New("flow mapping is not the final value on its line")
	}
	prefix := "# "
	if lineEnd == span.end {
		prefix = " # "
	}
	return byteSpan{start: lineEnd, end: lineEnd}, prefix + comment, nil
}

type blockEntryStyle struct {
	beforeColon   string
	afterColon    string
	colonColumn   int
	commentColumn int
}

func (e *Editor) blockInsertion(parent Path, parentRef *nodeRef, after string, hasAfter bool, key, value, inlineComment string) (byteSpan, string, error) {
	indent := 0
	insertAt := 0
	prefixNewline := false
	var reference *nodeRef
	node := parentRef.node
	if node.Kind == yaml.MappingNode {
		if len(node.Content) > 0 {
			indent = max(node.Content[0].Column-1, 0)
			lastKey := node.Content[len(node.Content)-2].Value
			lastRef := e.base.refs[pathKey(parent.ChildKey(lastKey))]
			if lastRef == nil {
				return byteSpan{}, "", errors.New("unable to locate final mapping entry")
			}
			insertAt = lastRef.entrySpan.end
			reference = lastRef
		} else {
			indent = max(node.Column-1, 0)
			span := e.base.nodeSpan(node)
			insertAt = span.end
			prefixNewline = true
		}
	} else if isNullNode(node) {
		if parentRef.key == nil {
			return byteSpan{}, "", errors.New("implicit mapping has no parent key")
		}
		indent = max(parentRef.key.Column+1, 0)
		lineStart, _ := e.base.offsetAt(parentRef.key.Line, 1)
		insertAt = e.base.lineEnd(lineStart, true)
	} else {
		return byteSpan{}, "", errors.New("target is not a mapping")
	}
	if hasAfter {
		anchorPath := parent.ChildKey(after)
		anchor, ok := e.base.refs[pathKey(anchorPath)]
		if !ok || anchor.parent != node {
			return byteSpan{}, "", fmt.Errorf("after key %q: %w", after, ErrPathNotFound)
		}
		insertAt = anchor.entrySpan.end
		reference = anchor
	}
	if insertAt > 0 && insertAt <= len(e.base.source) && e.base.source[insertAt-1] != '\n' {
		prefixNewline = true
	}
	prefix := ""
	if prefixNewline {
		prefix = e.base.newline
	}
	entry := e.base.renderBlockEntry(node, reference, indent, key, value, inlineComment)
	fragment := prefix + entry + e.base.newline
	return byteSpan{start: insertAt, end: insertAt}, fragment, nil
}

func (d *Document) renderBlockEntry(mapping *yaml.Node, reference *nodeRef, indent int, key, value, comment string) string {
	style := d.detectBlockEntryStyle(mapping, reference)
	beforeColon := style.beforeColon
	if style.colonColumn > 0 {
		keyEndColumn := indent + 1 + utf8.RuneCountInString(key)
		beforeColon = strings.Repeat(" ", max(style.colonColumn-keyEndColumn, 1))
	}
	afterColon := style.afterColon
	if value == "" {
		afterColon = ""
	}
	line := strings.Repeat(" ", indent) + key + beforeColon + ":" + afterColon + value
	if comment == "" {
		return line
	}
	nextColumn := utf8.RuneCountInString(line) + 1
	return line + strings.Repeat(" ", max(style.commentColumn-nextColumn, 1)) + "# " + comment
}

func (d *Document) detectBlockEntryStyle(mapping *yaml.Node, reference *nodeRef) blockEntryStyle {
	style := blockEntryStyle{afterColon: " "}
	preferredColon := 0
	if reference != nil && reference.key != nil {
		if _, colon, ok := d.keyColon(reference.key); ok {
			preferredColon = d.positionAt(colon).Column
			before := d.source[reference.keySpan.end:colon]
			if horizontalWhitespace(before) {
				style.beforeColon = string(before)
				if len(before) > 0 && !bytes.ContainsRune(before, '\t') {
					style.colonColumn = preferredColon
				}
			}
			valueSpan := d.nodeSpan(reference.node)
			if reference.node.Line == reference.key.Line && valueSpan.start > colon {
				after := d.source[colon+1 : valueSpan.start]
				if len(after) > 0 && horizontalWhitespace(after) {
					style.afterColon = string(after)
				}
			}
			style.commentColumn = d.inlineCommentColumn(reference.key.Line, colon+1)
		}
	}
	if style.colonColumn == 0 && style.beforeColon == "" {
		if column, ok := d.dominantAlignedColonColumn(mapping, preferredColon); ok {
			style.colonColumn = column
		}
	}
	if style.commentColumn == 0 {
		style.commentColumn = d.nearestInlineCommentColumn(mapping, reference)
	}
	if style.commentColumn == 0 {
		style.commentColumn = d.dominantInlineCommentColumn(mapping)
	}
	return style
}

func (d *Document) mappingColon(keySpan byteSpan, lineEnd int) (int, bool) {
	if !keySpan.valid(len(d.source)) || keySpan.end > lineEnd {
		return 0, false
	}
	cursor := keySpan.end
	for cursor < lineEnd && (d.source[cursor] == ' ' || d.source[cursor] == '\t') {
		cursor++
	}
	return cursor, cursor < lineEnd && d.source[cursor] == ':'
}

func horizontalWhitespace(value []byte) bool {
	return len(bytes.Trim(value, " \t")) == 0
}

func (d *Document) inlineCommentColumn(line, after int) int {
	if line < 1 || line > len(d.lineStarts) {
		return 0
	}
	lineStart := d.lineStarts[line-1]
	lineEnd := d.lineEnd(lineStart, false)
	commentStart := lineEnd
	for _, comment := range d.comments {
		if comment.start >= after && comment.start >= lineStart && comment.start < commentStart {
			commentStart = comment.start
		}
	}
	if commentStart == lineEnd {
		return 0
	}
	return d.positionAt(commentStart).Column
}

func (d *Document) dominantAlignedColonColumn(mapping *yaml.Node, preferred int) (int, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return 0, false
	}
	type alignment struct {
		count  int
		padded bool
	}
	stats := make(map[int]alignment)
	var order []int
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keySpan, colon, ok := d.keyColon(mapping.Content[i])
		if !ok {
			continue
		}
		column := d.positionAt(colon).Column
		stat := stats[column]
		if stat.count == 0 {
			order = append(order, column)
		}
		stat.count++
		stat.padded = stat.padded || colon > keySpan.end && bytes.IndexByte(d.source[keySpan.end:colon], 9) < 0
		stats[column] = stat
	}
	if stat := stats[preferred]; stat.count >= 2 && stat.padded {
		return preferred, true
	}
	bestColumn, bestCount := 0, 0
	for _, column := range order {
		if stat := stats[column]; stat.count >= 2 && stat.padded && stat.count > bestCount {
			bestColumn, bestCount = column, stat.count
		}
	}
	return bestColumn, bestColumn > 0
}

func (d *Document) keyColon(key *yaml.Node) (byteSpan, int, bool) {
	lineStart, ok := d.offsetAt(key.Line, 1)
	if !ok {
		return byteSpan{}, 0, false
	}
	span := d.scalarSpan(key)
	colon, ok := d.mappingColon(span, d.lineEnd(lineStart, false))
	return span, colon, ok
}

func (d *Document) keyCommentColumn(key *yaml.Node) int {
	_, colon, ok := d.keyColon(key)
	if !ok {
		return 0
	}
	return d.inlineCommentColumn(key.Line, colon+1)
}

func (d *Document) nearestInlineCommentColumn(mapping *yaml.Node, reference *nodeRef) int {
	if mapping == nil || mapping.Kind != yaml.MappingNode || reference == nil || reference.entry < 0 {
		return 0
	}
	count := len(mapping.Content) / 2
	for distance := 1; distance < count; distance++ {
		for _, entry := range []int{reference.entry - distance, reference.entry + distance} {
			if entry < 0 || entry >= count {
				continue
			}
			if column := d.keyCommentColumn(mapping.Content[entry*2]); column > 0 {
				return column
			}
		}
	}
	return 0
}

func (d *Document) dominantInlineCommentColumn(mapping *yaml.Node) int {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return 0
	}
	counts := make(map[int]int)
	bestColumn, bestCount := 0, 0
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		column := d.keyCommentColumn(mapping.Content[i])
		if column == 0 {
			continue
		}
		counts[column]++
		if counts[column] > bestCount {
			bestColumn, bestCount = column, counts[column]
		}
	}
	return bestColumn
}

// Delete removes one active mapping entry. Its inline comment is part of the
// entry span; independent comment lines are intentionally left in place.
func (e *Editor) Delete(path Path) (err error) {
	defer e.captureFailure(&err)
	ref, err := e.reference(path)
	if err != nil {
		return err
	}
	if ref.key == nil || ref.parent == nil || ref.parent.Kind != yaml.MappingNode {
		return fmt.Errorf("delete %s: target is not a mapping entry", path.String())
	}
	if err := editableContainer(ref.node); err != nil {
		return fmt.Errorf("delete %s: %w", path.String(), err)
	}
	if err := e.stageChange("delete", path, ref.entrySpan, nil, finalIdentifier(path)); err != nil {
		return fmt.Errorf("delete %s: %w", path.String(), err)
	}
	return nil
}

// RenameKey replaces only the mapping-key lexeme.
func (e *Editor) RenameKey(path Path, newName string) (err error) {
	defer e.captureFailure(&err)
	if strings.TrimSpace(newName) == "" {
		return errors.New("new key name must not be empty")
	}
	ref, err := e.reference(path)
	if err != nil {
		return err
	}
	if ref.key == nil {
		return fmt.Errorf("rename %s: target has no mapping key", path.String())
	}
	if err := editableScalar(ref.key); err != nil {
		return fmt.Errorf("rename %s: %w", path.String(), err)
	}
	replacement := renderStringLike(ref.key, newName, ref.parent != nil && ref.parent.Style&yaml.FlowStyle != 0)
	if err := e.stageChange("rename-key", path, ref.keySpan, []byte(replacement), newName); err != nil {
		return fmt.Errorf("rename %s: %w", path.String(), err)
	}
	return nil
}

// LiteralTarget selects the lexical regions searched by ReplaceAll.
type LiteralTarget uint8

const (
	LiteralScalars LiteralTarget = 1 << iota
	LiteralKeys
	LiteralComments
	LiteralAll = LiteralScalars | LiteralKeys | LiteralComments
)

type replaceOptions struct {
	targets    LiteralTarget
	identifier string
}

// ReplaceOption customizes exact token-local literal replacement.
type ReplaceOption func(*replaceOptions)

// WithLiteralTargets restricts replacement to active scalar values, active
// keys, comments, or any combination of those regions.
func WithLiteralTargets(targets LiteralTarget) ReplaceOption {
	return func(options *replaceOptions) { options.targets = targets }
}

// WithIdentifier overrides the non-sensitive ChangeSet label.
func WithIdentifier(identifier string) ReplaceOption {
	return func(options *replaceOptions) { options.identifier = identifier }
}

// ReplaceAll performs exact replacement only inside selected YAML token
// regions. It never scans arbitrary whitespace or document delimiters.
func (e *Editor) ReplaceAll(old, replacement string, options ...ReplaceOption) (count int, err error) {
	defer e.captureFailure(&err)
	if e == nil || e.base == nil {
		return 0, errors.New("inventory editor is nil")
	}
	if old == "" {
		return 0, errors.New("replacement literal must not be empty")
	}
	if old == replacement {
		return 0, nil
	}
	config := replaceOptions{targets: LiteralAll, identifier: "literal"}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if config.targets == 0 {
		return 0, nil
	}
	if protectedPath, protected := e.base.protectedLiteralOccurrence([]byte(old), config.targets); protected {
		return 0, fmt.Errorf("replace literal %q at %s: %w", config.identifier, protectedPath.String(), ErrProtectedNode)
	}
	containers := e.base.literalContainers(config.targets)
	var occurrences []byteSpan
	needle := []byte(old)
	for _, container := range containers {
		if !container.valid(len(e.base.source)) || container.end <= container.start {
			continue
		}
		search := e.base.source[container.start:container.end]
		for offset := 0; offset <= len(search)-len(needle); {
			index := bytes.Index(search[offset:], needle)
			if index < 0 {
				break
			}
			start := container.start + offset + index
			item := byteSpan{start: start, end: start + len(needle)}
			occurrences = append(occurrences, item)
			offset += index + len(needle)
		}
	}
	sort.Slice(occurrences, func(i, j int) bool { return occurrences[i].start < occurrences[j].start })
	for _, item := range occurrences {
		if _, err := e.stage(item, []byte(replacement)); err != nil {
			return 0, fmt.Errorf("replace literal %q: %w", config.identifier, err)
		}
	}
	if len(occurrences) > 0 {
		e.changes = append(e.changes, Change{Operation: "replace-literal", Identifier: config.identifier, Count: len(occurrences)})
	}
	return len(occurrences), nil
}

func (d *Document) protectedLiteralOccurrence(needle []byte, targets LiteralTarget) (Path, bool) {
	for _, ref := range d.refs {
		if ref == nil {
			continue
		}
		if targets&LiteralKeys != 0 && ref.key != nil && !isEditableLiteralNode(ref.key) && ref.keySpan.valid(len(d.source)) && bytes.Contains(d.source[ref.keySpan.start:ref.keySpan.end], needle) {
			return ref.path, true
		}
		if targets&LiteralScalars != 0 && ref.node != nil && (ref.node.Kind == yaml.ScalarNode || ref.node.Kind == yaml.AliasNode) && !isEditableLiteralNode(ref.node) && ref.valueSpan.valid(len(d.source)) && bytes.Contains(d.source[ref.valueSpan.start:ref.valueSpan.end], needle) {
			return ref.path, true
		}
	}
	return nil, false
}

func (d *Document) literalContainers(targets LiteralTarget) []byteSpan {
	containers := make([]byteSpan, 0, len(d.refs))
	appendSpan := func(span byteSpan) {
		if span.valid(len(d.source)) {
			containers = append(containers, span)
		}
	}
	for _, ref := range d.refs {
		if ref == nil {
			continue
		}
		if targets&LiteralKeys != 0 && ref.key != nil && isEditableLiteralNode(ref.key) {
			appendSpan(ref.keySpan)
		}
		if targets&LiteralScalars != 0 && ref.node != nil && (ref.node.Kind == yaml.ScalarNode || ref.node.Kind == yaml.AliasNode) && isEditableLiteralNode(ref.node) {
			appendSpan(ref.valueSpan)
		}
	}
	if targets&LiteralComments != 0 {
		for _, comment := range d.comments {
			appendSpan(comment)
		}
	}
	return containers
}

// UncommentKey activates exact commented mapping keys with this name. Only
// the comment marker and its separator spaces are removed.
func (e *Editor) UncommentKey(key string) (result EditResult, err error) {
	defer e.captureFailure(&err)
	if e == nil || e.base == nil {
		return result, errors.New("inventory editor is nil")
	}
	if strings.TrimSpace(key) == "" {
		return result, errors.New("commented key must not be empty")
	}
	for _, marker := range e.base.commentedKeyMarkers(key) {
		result.Matched++
		changed, stageErr := e.stage(marker, nil)
		if stageErr != nil {
			return result, fmt.Errorf("uncomment key %q: %w", key, stageErr)
		}
		if changed {
			result.Changed++
		}
	}
	if result.Changed > 0 {
		e.changes = append(e.changes, Change{Operation: "uncomment-key", Identifier: key, Count: result.Changed})
	}
	return result, nil
}

// CommentedKeyCount reports exact, inactive mapping-like key comments.
func (d *Document) CommentedKeyCount(key string) int {
	if d == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	return len(d.commentedKeyMarkers(key))
}

func (d *Document) commentedKeyMarkers(key string) []byteSpan {
	markers := make([]byteSpan, 0)
	for _, comment := range d.comments {
		position := d.positionAt(comment.start)
		lineStart := d.lineStarts[position.Line-1]
		if !horizontalWhitespace(d.source[lineStart:comment.start]) {
			continue
		}
		cursor := comment.start + 1
		for cursor < comment.end && (d.source[cursor] == ' ' || d.source[cursor] == '\t') {
			cursor++
		}
		if cursor+len(key) > comment.end || string(d.source[cursor:cursor+len(key)]) != key {
			continue
		}
		after := cursor + len(key)
		for after < comment.end && (d.source[after] == ' ' || d.source[after] == '\t') {
			after++
		}
		if after >= comment.end || d.source[after] != ':' {
			continue
		}
		markers = append(markers, byteSpan{start: comment.start, end: cursor})
	}
	return markers
}

// Commit applies all non-overlapping patches in descending byte order,
// reparses the complete candidate, and runs YAML plus Ansible structural rules.
// Any error leaves the base Document untouched.
func (e *Editor) Commit() (*Document, ChangeSet, error) {
	return e.commit(true)
}

// CommitCandidate applies and reparses staged patches without enforcing the
// default semantic rule set. It exists for targeted-repair workflows that
// compare original and candidate diagnostics before any write. The base
// Document remains immutable and no bytes are persisted by this method.
func (e *Editor) CommitCandidate() (*Document, ChangeSet, error) {
	return e.commit(false)
}

func (e *Editor) commit(validate bool) (*Document, ChangeSet, error) {
	if e == nil || e.base == nil {
		return nil, ChangeSet{}, errors.New("inventory editor is nil")
	}
	if e.failed != nil {
		return nil, ChangeSet{}, fmt.Errorf("inventory transaction aborted: %w", e.failed)
	}
	patches := slices.Clone(e.patches)
	sort.SliceStable(patches, func(i, j int) bool {
		if patches[i].span.start != patches[j].span.start {
			return patches[i].span.start > patches[j].span.start
		}
		iReplaces := patches[i].span.end > patches[i].span.start
		jReplaces := patches[j].span.end > patches[j].span.start
		if iReplaces != jReplaces {
			return iReplaces
		}
		return patches[i].order > patches[j].order
	})
	candidate := bytes.Clone(e.base.source)
	for _, patch := range patches {
		updated := make([]byte, 0, len(candidate)-(patch.span.end-patch.span.start)+len(patch.replacement))
		updated = append(updated, candidate[:patch.span.start]...)
		updated = append(updated, patch.replacement...)
		updated = append(updated, candidate[patch.span.end:]...)
		candidate = updated
	}
	doc, err := Parse(e.base.name, candidate)
	if err != nil {
		return nil, ChangeSet{}, err
	}
	if validate {
		diagnostics := doc.Validate(YAMLRules, AnsibleRules)
		if HasErrors(diagnostics) {
			return nil, ChangeSet{}, &ValidationError{Diagnostics: diagnostics}
		}
	}
	return doc, ChangeSet{Changes: slices.Clone(e.changes)}, nil
}

func (e *Editor) captureFailure(result *error) {
	if e == nil || result == nil || *result == nil || e.failed != nil {
		return
	}
	e.failed = *result
}

func (e *Editor) reference(path Path) (*nodeRef, error) {
	if e == nil || e.base == nil {
		return nil, errors.New("inventory editor is nil")
	}
	ref, ok := e.base.refs[pathKey(path)]
	if !ok {
		return nil, fmt.Errorf("%s: %w", path.String(), ErrPathNotFound)
	}
	return ref, nil
}

func (e *Editor) stage(span byteSpan, replacement []byte) (bool, error) {
	if !span.valid(len(e.base.source)) {
		return false, fmt.Errorf("invalid source span [%d,%d)", span.start, span.end)
	}
	if span.start != span.end && bytes.Equal(e.base.source[span.start:span.end], replacement) {
		return false, nil
	}
	if span.start == span.end && len(replacement) == 0 {
		return false, nil
	}
	for _, existing := range e.patches {
		if patchesOverlap(existing.span, span) {
			return false, fmt.Errorf("%w: source spans [%d,%d) and [%d,%d) overlap", ErrEditConflict, existing.span.start, existing.span.end, span.start, span.end)
		}
	}
	e.patches = append(e.patches, sourcePatch{span: span, replacement: bytes.Clone(replacement), order: len(e.patches)})
	return true, nil
}

func (e *Editor) stageChange(operation string, path Path, span byteSpan, replacement []byte, identifier string) error {
	changed, err := e.stage(span, replacement)
	if changed {
		e.changes = append(e.changes, Change{Operation: operation, Path: clonePath(path), Identifier: identifier, Count: 1})
	}
	return err
}

func patchesOverlap(a, b byteSpan) bool {
	if a.start == a.end {
		return a.start > b.start && a.start < b.end
	}
	if b.start == b.end {
		return b.start > a.start && b.start < a.end
	}
	return a.start < b.end && b.start < a.end
}

func editableScalar(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.ScalarNode || !isEditableLiteralNode(node) || node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return ErrProtectedNode
	}
	return nil
}

func editableContainer(node *yaml.Node) error {
	if !isEditableLiteralNode(node) {
		return ErrProtectedNode
	}
	return nil
}

func isEditableLiteralNode(node *yaml.Node) bool {
	return node != nil && node.Kind != yaml.AliasNode && node.Anchor == "" && (node.Tag == "" || strings.HasPrefix(node.Tag, "!!"))
}

func renderExistingScalar(node *yaml.Node, value any, flow bool) (string, error) {
	if node == nil {
		return "", ErrProtectedNode
	}
	if text, ok := value.(string); ok {
		return renderStringLike(node, text, flow), nil
	}
	return renderNewValue(value, flow)
}

func renderStringLike(node *yaml.Node, value string, flow bool) string {
	if node.Style&yaml.SingleQuotedStyle != 0 {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	if node.Style&yaml.DoubleQuotedStyle != 0 {
		return strconv.Quote(value)
	}
	if plainStringSafe(value, flow) {
		return value
	}
	return strconv.Quote(value)
}

func renderKey(value string) string {
	if plainStringSafe(value, false) && !strings.Contains(value, ":") {
		return value
	}
	return strconv.Quote(value)
}

func renderNewValue(value any, flow bool) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "null", nil
	case implicitMapping:
		if flow {
			return "{}", nil
		}
		return "", nil
	case string:
		if conservativePlainStringSafe(typed, flow) {
			return typed, nil
		}
		return strconv.Quote(typed), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(typed), nil
	case []string:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			rendered, _ := renderNewValue(item, true)
			parts = append(parts, rendered)
		}
		return "[ " + strings.Join(parts, ", ") + " ]", nil
	default:
		return "", fmt.Errorf("unsupported inserted value type %T", value)
	}
}

func conservativePlainStringSafe(value string, flow bool) bool {
	if !plainStringSafe(value, flow) {
		return false
	}
	first, _ := utf8.DecodeRuneInString(value)
	if unicode.IsDigit(first) || first == '+' || first == '.' {
		return false
	}
	return !strings.ContainsAny(value, ":,#[]{}&*!|>'\"%@`")
}

func plainStringSafe(value string, flow bool) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	first, _ := utf8.DecodeRuneInString(value)
	if strings.ContainsRune("-?:,[]{}#&*!|>'\"%@`", first) {
		return false
	}
	lower := strings.ToLower(value)
	switch lower {
	case "null", "~", "true", "false", "yes", "no", "y", "n", "on", "off", ".nan", ".inf", "-.inf", "+.inf":
		return false
	}
	if _, err := strconv.ParseInt(value, 0, 64); err == nil {
		return false
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return false
	}
	for i, r := range value {
		if unicode.IsControl(r) || r == '\n' || r == '\r' {
			return false
		}
		if r == '#' && i > 0 {
			previous := value[i-1]
			if previous == ' ' || previous == '\t' {
				return false
			}
		}
	}
	if strings.Contains(value, ": ") || strings.HasSuffix(value, ":") {
		return false
	}
	if flow && strings.ContainsAny(value, ",[]{}") {
		return false
	}
	return true
}

func finalIdentifier(path Path) string {
	segment, ok := path.Last()
	if !ok {
		return ""
	}
	if segment.Kind == IndexSegment {
		return strconv.Itoa(segment.Index)
	}
	return segment.Key
}
