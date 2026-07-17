package inventory

import (
	"errors"
	"fmt"
	"slices"
)

// SnapshotConflict contains only the two declaration paths and operation
// kinds that conflict. It is safe for public plans and diagnostics.
type SnapshotConflict struct {
	Working SnapshotChange `json:"working" yaml:"working"`
	Current SnapshotChange `json:"current" yaml:"current"`
}

// SnapshotMergeResult is the value-free result of a three-way merge plan. A
// conflicted result never retains or exposes a candidate Document.
type SnapshotMergeResult struct {
	Changes   []SnapshotChange   `json:"changes,omitempty" yaml:"changes,omitempty"`
	Conflicts []SnapshotConflict `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
	document  *Document
}

// Document returns the lossless patched candidate only for a clean merge.
func (r *SnapshotMergeResult) Document() *Document {
	if r == nil || len(r.Conflicts) > 0 {
		return nil
	}
	return r.document
}

// Clean reports whether the merge completed without semantic conflicts.
func (r *SnapshotMergeResult) Clean() bool {
	return r != nil && len(r.Conflicts) == 0 && r.document != nil
}

// MergeSnapshots performs the P2 v1 three-way merge:
//
//   - base is immutable B;
//   - working is the current lossless static Document W;
//   - current is the CMDB head C.
//
// Only non-conflicting B-to-C deltas are source-patched into W. Conflicts
// produce no candidate body.
func MergeSnapshots(base *Snapshot, working *Document, current *Snapshot) (*SnapshotMergeResult, error) {
	if base == nil || working == nil || current == nil {
		return nil, errors.New("Snapshot merge requires base, working Document, and current revisions")
	}
	if diagnostics := working.Validate(YAMLRules, AnsibleRules); HasErrors(diagnostics) {
		return nil, &ValidationError{Diagnostics: diagnostics}
	}
	workingSnapshot, err := SnapshotFromDocument(working)
	if err != nil {
		return nil, err
	}
	workingDeltas, err := diffSnapshotValues(base, workingSnapshot)
	if err != nil {
		return nil, err
	}
	currentDeltas, err := diffSnapshotValues(base, current)
	if err != nil {
		return nil, err
	}
	conflicts := snapshotMergeConflicts(workingSnapshot, current, workingDeltas, currentDeltas)
	if len(conflicts) > 0 {
		return &SnapshotMergeResult{Conflicts: conflicts}, nil
	}

	mergedRoot := cloneSnapshotValue(workingSnapshot.root)
	for i := range currentDeltas {
		if err := applySnapshotDelta(&mergedRoot, currentDeltas[i]); err != nil {
			return nil, err
		}
	}
	mergedSnapshot, err := snapshotFromMergedRoot(mergedRoot)
	if err != nil {
		return nil, err
	}
	candidate, changes, err := patchDocumentToSnapshot(working, workingSnapshot, mergedSnapshot)
	if err != nil {
		return nil, err
	}
	return &SnapshotMergeResult{Changes: changes, document: candidate}, nil
}

func snapshotMergeConflicts(working, current *Snapshot, workingDeltas, currentDeltas []snapshotDelta) []SnapshotConflict {
	conflicts := make([]SnapshotConflict, 0)
	seen := make(map[string]struct{})
	for i := range workingDeltas {
		for j := range currentDeltas {
			if !snapshotDeltasConflict(working, current, workingDeltas[i], currentDeltas[j]) {
				continue
			}
			conflict := SnapshotConflict{
				Working: SnapshotChange{Operation: workingDeltas[i].operation, Path: clonePath(workingDeltas[i].path)},
				Current: SnapshotChange{Operation: currentDeltas[j].operation, Path: clonePath(currentDeltas[j].path)},
			}
			key := string(conflict.Working.Operation) + "\x00" + pathKey(conflict.Working.Path) + "\x00" +
				string(conflict.Current.Operation) + "\x00" + pathKey(conflict.Current.Path)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			conflicts = append(conflicts, conflict)
		}
	}
	return conflicts
}

func snapshotDeltasConflict(working, current *Snapshot, left, right snapshotDelta) bool {
	if pathsEqual(left.path, right.path) {
		return !sameSnapshotDeltaResult(left, right)
	}
	if pathsRelated(left.path, right.path) {
		if left.operation == SnapshotRemove || right.operation == SnapshotRemove {
			return true
		}
		common := left.path
		if len(right.path) < len(common) {
			common = right.path
		}
		leftValue, leftExists := snapshotValueAt(working.root, common)
		rightValue, rightExists := snapshotValueAt(current.root, common)
		return leftExists != rightExists || leftExists && !snapshotValueEqual(leftValue, rightValue)
	}
	if left.operation == SnapshotAdd && right.operation == SnapshotAdd &&
		pathsEqual(parentPath(left.path), parentPath(right.path)) && sameSnapshotGap(left.gap, right.gap, false) {
		parent := parentPath(left.path)
		leftValue, leftExists := snapshotValueAt(working.root, parent)
		rightValue, rightExists := snapshotValueAt(current.root, parent)
		return leftExists != rightExists || leftExists && !snapshotValueEqual(leftValue, rightValue)
	}
	if deltaRemovesSnapshotAnchor(left, right) || deltaRemovesSnapshotAnchor(right, left) {
		return true
	}
	return false
}

func sameSnapshotDeltaResult(left, right snapshotDelta) bool {
	if left.operation != right.operation {
		return false
	}
	switch left.operation {
	case SnapshotRemove:
		return true
	case SnapshotAdd:
		return left.hasAfter && right.hasAfter && snapshotValueEqual(left.after, right.after) &&
			sameSnapshotGap(left.gap, right.gap, true)
	case SnapshotReplace:
		return left.hasAfter && right.hasAfter && snapshotValueEqual(left.after, right.after)
	case SnapshotReorder:
		return slices.Equal(left.order, right.order)
	default:
		return false
	}
}

func sameSnapshotGap(left, right snapshotGap, includeOrdinal bool) bool {
	if left.hasPrevious != right.hasPrevious || left.hasNext != right.hasNext ||
		left.hasPrevious && left.previous != right.previous ||
		left.hasNext && left.next != right.next {
		return false
	}
	return !includeOrdinal || left.ordinal == right.ordinal
}

func deltaRemovesSnapshotAnchor(removal, addition snapshotDelta) bool {
	if removal.operation != SnapshotRemove || addition.operation != SnapshotAdd ||
		!pathsEqual(parentPath(removal.path), parentPath(addition.path)) {
		return false
	}
	last, exists := removal.path.Last()
	if !exists || last.Kind != KeySegment {
		return false
	}
	return addition.gap.hasPrevious && addition.gap.previous == last.Key ||
		addition.gap.hasNext && addition.gap.next == last.Key
}

func pathsEqual(left, right Path) bool { return slices.Equal(left, right) }

func pathsRelated(left, right Path) bool {
	return pathIsPrefix(left, right) || pathIsPrefix(right, left)
}

func pathIsPrefix(prefix, path Path) bool {
	return len(prefix) <= len(path) && slices.Equal(prefix, path[:len(prefix)])
}

func parentPath(path Path) Path {
	parent, _ := path.Parent()
	return parent
}

func snapshotValueAt(root SnapshotValue, path Path) (SnapshotValue, bool) {
	value := root
	for _, segment := range path {
		switch segment.Kind {
		case KeySegment:
			child, exists := snapshotLookupRaw(value, segment.Key)
			if !exists {
				return SnapshotValue{}, false
			}
			value = child
		case IndexSegment:
			if value.kind != SnapshotSeq || segment.Index < 0 || segment.Index >= len(value.items) {
				return SnapshotValue{}, false
			}
			value = value.items[segment.Index]
		default:
			return SnapshotValue{}, false
		}
	}
	return value, true
}

func applySnapshotDelta(root *SnapshotValue, delta snapshotDelta) error {
	if root == nil {
		return errors.New("cannot apply Snapshot delta to nil root")
	}
	switch delta.operation {
	case SnapshotRemove:
		parent, key, err := snapshotDeltaParent(root, delta.path)
		if err != nil {
			return err
		}
		index := snapshotEntryIndex(*parent, key)
		if index < 0 {
			return nil
		}
		parent.entries = append(parent.entries[:index], parent.entries[index+1:]...)
		return nil

	case SnapshotAdd:
		parent, key, err := snapshotDeltaParent(root, delta.path)
		if err != nil {
			return err
		}
		if index := snapshotEntryIndex(*parent, key); index >= 0 {
			if snapshotValueEqual(parent.entries[index].value, delta.after) {
				return nil
			}
			return fmt.Errorf("cannot apply add at %s: key already exists", delta.path.String())
		}
		index, err := snapshotGapInsertionIndex(*parent, delta.gap)
		if err != nil {
			return fmt.Errorf("cannot apply add at %s: %w", delta.path.String(), err)
		}
		parent.entries = slices.Insert(parent.entries, index, SnapshotEntry{key: key, value: cloneSnapshotValue(delta.after)})
		return nil

	case SnapshotReplace:
		if len(delta.path) == 0 {
			*root = cloneSnapshotValue(delta.after)
			return nil
		}
		parent, key, err := snapshotDeltaParent(root, delta.path)
		if err != nil {
			return err
		}
		index := snapshotEntryIndex(*parent, key)
		if index < 0 {
			return fmt.Errorf("cannot apply replace at %s: path is absent", delta.path.String())
		}
		parent.entries[index].value = cloneSnapshotValue(delta.after)
		return nil

	case SnapshotReorder:
		mapping, exists := snapshotValuePointer(root, delta.path)
		if !exists || mapping.kind != SnapshotMap {
			return fmt.Errorf("cannot apply reorder at %s: mapping is absent", delta.path.String())
		}
		currentOrder := make([]string, 0, len(delta.order))
		orderSet := make(map[string]struct{}, len(delta.order))
		for _, key := range delta.order {
			orderSet[key] = struct{}{}
		}
		for i := range mapping.entries {
			if _, selected := orderSet[mapping.entries[i].key]; selected {
				currentOrder = append(currentOrder, mapping.entries[i].key)
			}
		}
		if slices.Equal(currentOrder, delta.order) {
			return nil
		}
		if len(mapping.entries) != len(delta.order) {
			return fmt.Errorf("cannot apply reorder at %s: mapping key set changed", delta.path.String())
		}
		entries := make(map[string]SnapshotEntry, len(mapping.entries))
		for i := range mapping.entries {
			entries[mapping.entries[i].key] = mapping.entries[i]
		}
		reordered := make([]SnapshotEntry, len(delta.order))
		for i, key := range delta.order {
			entry, exists := entries[key]
			if !exists {
				return fmt.Errorf("cannot apply reorder at %s: mapping key set changed", delta.path.String())
			}
			reordered[i] = entry
			delete(entries, key)
		}
		if len(entries) != 0 {
			return fmt.Errorf("cannot apply reorder at %s: mapping key set changed", delta.path.String())
		}
		mapping.entries = reordered
		return nil
	default:
		return fmt.Errorf("cannot apply unknown Snapshot delta at %s", delta.path.String())
	}
}

func snapshotDeltaParent(root *SnapshotValue, path Path) (*SnapshotValue, string, error) {
	parentPath, exists := path.Parent()
	last, hasLast := path.Last()
	if !exists || !hasLast || last.Kind != KeySegment {
		return nil, "", fmt.Errorf("Snapshot delta path %s is not a mapping entry", path.String())
	}
	parent, found := snapshotValuePointer(root, parentPath)
	if !found || parent.kind != SnapshotMap {
		return nil, "", fmt.Errorf("Snapshot delta parent %s is not a mapping", parentPath.String())
	}
	return parent, last.Key, nil
}

func snapshotValuePointer(root *SnapshotValue, path Path) (*SnapshotValue, bool) {
	value := root
	for _, segment := range path {
		switch segment.Kind {
		case KeySegment:
			if value.kind != SnapshotMap {
				return nil, false
			}
			index := snapshotEntryIndex(*value, segment.Key)
			if index < 0 {
				return nil, false
			}
			value = &value.entries[index].value
		case IndexSegment:
			if value.kind != SnapshotSeq || segment.Index < 0 || segment.Index >= len(value.items) {
				return nil, false
			}
			value = &value.items[segment.Index]
		default:
			return nil, false
		}
	}
	return value, true
}

func snapshotEntryIndex(mapping SnapshotValue, key string) int {
	if mapping.kind != SnapshotMap {
		return -1
	}
	for i := range mapping.entries {
		if mapping.entries[i].key == key {
			return i
		}
	}
	return -1
}

func snapshotGapInsertionIndex(mapping SnapshotValue, gap snapshotGap) (int, error) {
	index := gap.ordinal
	if gap.hasPrevious {
		previous := snapshotEntryIndex(mapping, gap.previous)
		if previous < 0 {
			return 0, errors.New("previous gap anchor is absent")
		}
		index = previous + 1 + gap.ordinal
	}
	if gap.hasNext {
		next := snapshotEntryIndex(mapping, gap.next)
		if next < 0 {
			return 0, errors.New("next gap anchor is absent")
		}
		if index > next {
			return 0, errors.New("gap anchors are out of order")
		}
	}
	if index < 0 || index > len(mapping.entries) {
		return 0, errors.New("gap ordinal is outside mapping")
	}
	return index, nil
}

func snapshotFromMergedRoot(root SnapshotValue) (*Snapshot, error) {
	if root.kind != SnapshotMap {
		return nil, snapshotFailure(nil, SnapshotReasonRootType)
	}
	nodes, err := countSnapshotNodes(root, 0)
	if err != nil {
		return nil, err
	}
	if err := validateSnapshotHostDeclarations(root); err != nil {
		return nil, err
	}
	canonical, err := encodeCanonicalSnapshot(root)
	if err != nil {
		return nil, err
	}
	return &Snapshot{root: root, canonical: canonical, digest: sha256Sum(canonical), nodeCount: nodes}, nil
}

func countSnapshotNodes(value SnapshotValue, depth int) (int, error) {
	if depth > maxDocumentDepth {
		return 0, snapshotFailure(nil, SnapshotReasonDepth)
	}
	count := 1
	for i := range value.entries {
		child, err := countSnapshotNodes(value.entries[i].value, depth+1)
		if err != nil {
			return 0, err
		}
		count += child
		if count > maxSnapshotNodes {
			return 0, snapshotFailure(nil, SnapshotReasonNodeLimit)
		}
	}
	for i := range value.items {
		child, err := countSnapshotNodes(value.items[i], depth+1)
		if err != nil {
			return 0, err
		}
		count += child
		if count > maxSnapshotNodes {
			return 0, snapshotFailure(nil, SnapshotReasonNodeLimit)
		}
	}
	return count, nil
}
