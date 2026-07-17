package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"gopkg.in/yaml.v3"
)

var (
	// ErrSnapshotMergeUnsafe means a semantic merge was clean but the current
	// source shape could not be patched without guessing comment attachment or
	// normalizing an ordered container.
	ErrSnapshotMergeUnsafe = errors.New("Snapshot merge source patch is unsafe")
)

// snapshotFragmentValue is a package-private, prevalidated one-line YAML/JSON
// value. Editor accepts it only for a merge insertion; arbitrary callers
// cannot use it to bypass normal scalar rendering.
type snapshotFragmentValue string

func patchDocumentToSnapshot(working *Document, workingSnapshot, target *Snapshot) (*Document, []SnapshotChange, error) {
	deltas, err := diffSnapshotValues(workingSnapshot, target)
	if err != nil {
		return nil, nil, err
	}
	mutation := working.Mutate()
	for i := range deltas {
		delta := deltas[i]
		if err := mutation.Apply(func(editor *Editor) error {
			return applySnapshotSourceDelta(editor, target, delta)
		}); err != nil {
			return nil, nil, err
		}
	}
	candidate, _, err := mutation.Commit()
	if err != nil {
		return nil, nil, err
	}
	if diagnostics := candidate.Validate(YAMLRules, AnsibleRules); HasErrors(diagnostics) {
		return nil, nil, &ValidationError{Diagnostics: diagnostics}
	}
	candidateSnapshot, err := SnapshotFromDocument(candidate)
	if err != nil {
		return nil, nil, err
	}
	if !candidateSnapshot.Equal(target) {
		return nil, nil, fmt.Errorf("%w: patched candidate digest mismatch", ErrSnapshotMergeUnsafe)
	}
	changes := make([]SnapshotChange, len(deltas))
	for i := range deltas {
		changes[i] = SnapshotChange{Operation: deltas[i].operation, Path: clonePath(deltas[i].path)}
	}
	return candidate, changes, nil
}

func applySnapshotSourceDelta(editor *Editor, target *Snapshot, delta snapshotDelta) error {
	if editor == nil || editor.base == nil {
		return errors.New("Snapshot merge editor is nil")
	}
	switch delta.operation {
	case SnapshotRemove:
		return editor.Delete(delta.path)
	case SnapshotAdd:
		return insertSnapshotSourceValue(editor, target, delta.path, delta.after)
	case SnapshotReplace:
		return replaceSnapshotSourceValue(editor, delta.path, delta.after)
	case SnapshotReorder:
		return reorderSnapshotSourceMapping(editor, delta.path, delta.order)
	default:
		return fmt.Errorf("%w: unknown operation at %s", ErrSnapshotMergeUnsafe, delta.path.String())
	}
}

func insertSnapshotSourceValue(editor *Editor, target *Snapshot, path Path, value SnapshotValue) error {
	parentPath, hasParent := path.Parent()
	last, hasLast := path.Last()
	if !hasParent || !hasLast || last.Kind != KeySegment {
		return fmt.Errorf("%w: add path %s is not a mapping entry", ErrSnapshotMergeUnsafe, path.String())
	}
	targetParent, exists := snapshotValueAt(target.root, parentPath)
	if !exists || targetParent.kind != SnapshotMap {
		return fmt.Errorf("%w: add parent %s is not a mapping", ErrSnapshotMergeUnsafe, parentPath.String())
	}
	index := snapshotEntryIndex(targetParent, last.Key)
	if index < 0 {
		return fmt.Errorf("%w: add path %s is absent from target", ErrSnapshotMergeUnsafe, path.String())
	}
	fragment, err := renderSnapshotFlowFragment(value)
	if err != nil {
		return err
	}
	options := []InsertOption{permitSnapshotKey()}
	anchored := false
	for previousIndex := index - 1; previousIndex >= 0 && !anchored; previousIndex-- {
		previous := targetParent.entries[previousIndex].key
		if _, exists := editor.base.Lookup(parentPath.ChildKey(previous)); exists {
			options = append(options, AfterKey(previous))
			anchored = true
		}
	}
	for nextIndex := index + 1; nextIndex < len(targetParent.entries) && !anchored; nextIndex++ {
		next := targetParent.entries[nextIndex].key
		if _, exists := editor.base.Lookup(parentPath.ChildKey(next)); exists {
			options = append(options, beforeKey(next))
			anchored = true
		}
	}
	if !anchored {
		parent, exists := editor.base.Lookup(parentPath)
		if !exists || parent.Kind != MappingValue || len(parent.Mapping) != 0 {
			return fmt.Errorf("%w: add path %s has no exact source anchor", ErrSnapshotMergeUnsafe, path.String())
		}
	}
	return editor.Insert(parentPath, last.Key, snapshotFragmentValue(fragment), options...)
}

func replaceSnapshotSourceValue(editor *Editor, path Path, target SnapshotValue) error {
	ref, err := editor.reference(path)
	if err != nil {
		return err
	}
	if snapshotYAMLInline(target) && target.kind != SnapshotMap && target.kind != SnapshotSeq && target.kind != SnapshotFloat {
		if value, ok := snapshotScalarGoValue(target); ok && editableScalar(ref.node) == nil {
			return editor.Set(path, value)
		}
	}
	fragment, err := renderSnapshotFlowFragment(target)
	if err != nil {
		return err
	}
	span := ref.valueSpan
	replacement := []byte(fragment)
	if (ref.node.Kind == yaml.MappingNode || ref.node.Kind == yaml.SequenceNode) &&
		ref.node.Style&yaml.FlowStyle == 0 && len(ref.node.Content) > 0 {
		span = editor.base.blockBodySpan(ref.node)
		original := editor.base.source[span.start:span.end]
		replacement = matchTerminalNewline(replacement, original, editor.base.newline)
		indent := max(ref.node.Column-1, 0)
		replacement = reindentBody(replacement, indent)
	} else {
		original := editor.base.source[span.start:span.end]
		replacement = matchTerminalNewline(replacement, original, editor.base.newline)
		if span.start == span.end && ref.key != nil {
			if _, colon, ok := editor.base.keyColon(ref.key); ok && colon+1 == span.start {
				replacement = append([]byte(" "), replacement...)
			}
		}
	}
	if err := editor.stageChange("merge-replace", path, span, replacement, finalIdentifier(path)); err != nil {
		return fmt.Errorf("%w: replace at %s: %v", ErrSnapshotMergeUnsafe, path.String(), err)
	}
	return nil
}

func snapshotScalarGoValue(value SnapshotValue) (any, bool) {
	switch value.kind {
	case SnapshotStr:
		return value.text, true
	case SnapshotBool:
		return value.boolean, true
	case SnapshotInt:
		if signed, err := strconv.ParseInt(value.text, 10, 64); err == nil {
			return signed, true
		}
		if unsigned, err := strconv.ParseUint(value.text, 10, 64); err == nil {
			return unsigned, true
		}
	case SnapshotFloat:
		if number, err := strconv.ParseFloat(value.text, 64); err == nil {
			return number, true
		}
	case SnapshotNull:
		return nil, true
	}
	return nil, false
}

func renderSnapshotFlowFragment(value SnapshotValue) (string, error) {
	writer := newSnapshotByteWriter(maxSnapshotOutputBytes)
	if err := writeProjectionValue(writer, value); err != nil {
		return "", err
	}
	return string(writer.bytes()), nil
}

func reorderSnapshotSourceMapping(editor *Editor, path Path, order []string) error {
	ref, err := editor.reference(path)
	if err != nil {
		return err
	}
	if ref.node == nil || ref.node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: reorder target %s is not a mapping", ErrSnapshotMergeUnsafe, path.String())
	}
	current := make([]string, 0, len(ref.node.Content)/2)
	for i := 0; i+1 < len(ref.node.Content); i += 2 {
		current = append(current, ref.node.Content[i].Value)
	}
	if slices.Equal(current, order) {
		return nil
	}
	if !sameSnapshotKeySet(current, order) {
		return fmt.Errorf("%w: reorder key set changed at %s", ErrSnapshotMergeUnsafe, path.String())
	}
	if ref.node.Style&yaml.FlowStyle != 0 {
		return reorderSnapshotFlowMapping(editor, path, ref, current, order)
	}
	return reorderSnapshotBlockMapping(editor, path, ref, current, order)
}

func sameSnapshotKeySet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, key := range left {
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	for _, key := range right {
		if _, exists := seen[key]; !exists {
			return false
		}
		delete(seen, key)
	}
	return len(seen) == 0
}

func reorderSnapshotBlockMapping(editor *Editor, path Path, ref *nodeRef, current, order []string) error {
	if len(current) < 2 {
		return nil
	}
	entrySpans := make([]byteSpan, len(current))
	for i, key := range current {
		child := editor.base.refs[pathKey(path.ChildKey(key))]
		if child == nil || child.parent != ref.node || !child.entrySpan.valid(len(editor.base.source)) {
			return fmt.Errorf("%w: invalid block entry span at %s", ErrSnapshotMergeUnsafe, path.String())
		}
		entrySpans[i] = child.entrySpan
		if i > 0 && entrySpans[i-1].end > entrySpans[i].start {
			return fmt.Errorf("%w: overlapping block entry spans at %s", ErrSnapshotMergeUnsafe, path.String())
		}
	}
	scope, err := editor.base.subtreeSpan(path)
	if err != nil {
		return fmt.Errorf("%w: cannot resolve mapping scope at %s", ErrSnapshotMergeUnsafe, path.String())
	}
	for _, comment := range editor.base.comments {
		if comment.start < scope.start || comment.end > scope.end || !editor.base.standaloneComment(comment) {
			continue
		}
		contained := false
		for _, span := range entrySpans {
			if comment.start >= span.start && comment.end <= span.end {
				contained = true
				break
			}
		}
		if !contained {
			return fmt.Errorf("%w: ambiguous standalone comment at %s", ErrSnapshotMergeUnsafe, path.String())
		}
	}

	moveSpans := make([]byteSpan, len(entrySpans))
	for i := range entrySpans {
		end := entrySpans[i].end
		if i+1 < len(entrySpans) {
			end = entrySpans[i+1].start
			if len(bytes.TrimSpace(editor.base.source[entrySpans[i].end:end])) != 0 {
				return fmt.Errorf("%w: non-whitespace block entry gap at %s", ErrSnapshotMergeUnsafe, path.String())
			}
		}
		moveSpans[i] = byteSpan{start: entrySpans[i].start, end: end}
	}
	chunks := make(map[string][]byte, len(current))
	for i, key := range current {
		chunks[key] = editor.base.source[moveSpans[i].start:moveSpans[i].end]
	}
	var replacement []byte
	for _, key := range order {
		replacement = append(replacement, chunks[key]...)
	}
	region := byteSpan{start: moveSpans[0].start, end: moveSpans[len(moveSpans)-1].end}
	if err := editor.stageChange("merge-reorder", path, region, replacement, finalIdentifier(path)); err != nil {
		return fmt.Errorf("%w: block reorder at %s: %v", ErrSnapshotMergeUnsafe, path.String(), err)
	}
	return nil
}

func reorderSnapshotFlowMapping(editor *Editor, path Path, ref *nodeRef, current, order []string) error {
	span := editor.base.nodeSpan(ref.node)
	if !span.valid(len(editor.base.source)) || span.end-span.start < 2 ||
		editor.base.source[span.start] != '{' || editor.base.source[span.end-1] != '}' ||
		bytes.IndexAny(editor.base.source[span.start:span.end], "\r\n") >= 0 {
		return fmt.Errorf("%w: unsupported flow mapping at %s", ErrSnapshotMergeUnsafe, path.String())
	}
	for _, comment := range editor.base.comments {
		if comment.start >= span.start && comment.end <= span.end {
			return fmt.Errorf("%w: flow mapping comment at %s", ErrSnapshotMergeUnsafe, path.String())
		}
	}
	entrySpans := make([]byteSpan, len(current))
	for i, key := range current {
		child := editor.base.refs[pathKey(path.ChildKey(key))]
		if child == nil || child.parent != ref.node || !child.keySpan.valid(len(editor.base.source)) ||
			!child.valueSpan.valid(len(editor.base.source)) || child.keySpan.start >= child.valueSpan.end {
			return fmt.Errorf("%w: invalid flow entry span at %s", ErrSnapshotMergeUnsafe, path.String())
		}
		entrySpans[i] = byteSpan{start: child.keySpan.start, end: child.valueSpan.end}
		if i > 0 && entrySpans[i-1].end > entrySpans[i].start {
			return fmt.Errorf("%w: overlapping flow entry spans at %s", ErrSnapshotMergeUnsafe, path.String())
		}
	}
	prefix := editor.base.source[span.start+1 : entrySpans[0].start]
	suffix := editor.base.source[entrySpans[len(entrySpans)-1].end : span.end-1]
	if !horizontalWhitespace(prefix) || !horizontalWhitespace(suffix) {
		return fmt.Errorf("%w: unsupported flow mapping trivia at %s", ErrSnapshotMergeUnsafe, path.String())
	}
	separators := make([][]byte, len(entrySpans)-1)
	for i := 0; i+1 < len(entrySpans); i++ {
		separator := editor.base.source[entrySpans[i].end:entrySpans[i+1].start]
		if !validSnapshotFlowSeparator(separator) {
			return fmt.Errorf("%w: unsupported flow separator at %s", ErrSnapshotMergeUnsafe, path.String())
		}
		separators[i] = separator
	}
	chunks := make(map[string][]byte, len(current))
	for i, key := range current {
		chunks[key] = editor.base.source[entrySpans[i].start:entrySpans[i].end]
	}
	replacement := slices.Clone(prefix)
	for i, key := range order {
		replacement = append(replacement, chunks[key]...)
		if i < len(separators) {
			replacement = append(replacement, separators[i]...)
		}
	}
	replacement = append(replacement, suffix...)
	region := byteSpan{start: span.start + 1, end: span.end - 1}
	if err := editor.stageChange("merge-reorder", path, region, replacement, finalIdentifier(path)); err != nil {
		return fmt.Errorf("%w: flow reorder at %s: %v", ErrSnapshotMergeUnsafe, path.String(), err)
	}
	return nil
}

func validSnapshotFlowSeparator(source []byte) bool {
	trimmed := bytes.TrimSpace(source)
	return len(trimmed) == 1 && trimmed[0] == ','
}

func (d *Document) standaloneComment(comment byteSpan) bool {
	if d == nil || !comment.valid(len(d.source)) {
		return false
	}
	position := d.positionAt(comment.start)
	if position.Line < 1 || position.Line > len(d.lineStarts) {
		return false
	}
	lineStart := d.lineStarts[position.Line-1]
	return horizontalWhitespace(d.source[lineStart:comment.start])
}
