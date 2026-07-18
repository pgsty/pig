package inventory

import (
	"errors"
	"fmt"
	"reflect"
)

// Difference is a value-free semantic declaration change. It deliberately
// retains no scalar, source fragment, comment, or formatting data.
type Difference struct {
	Operation string
	Path      Path
	ValueKind ValueKind
}

// DiffDocuments compares semantic declarations. Mapping order and YAML
// presentation are ignored; sequence order is significant and a changed
// sequence is reported as one atomic replacement.
func DiffDocuments(left, right *Document) ([]Difference, error) {
	if left == nil || right == nil {
		return nil, errors.New("inventory diff requires two documents")
	}
	leftRoot, leftOK := left.Lookup(nil)
	rightRoot, rightOK := right.Lookup(nil)
	if !leftOK || !rightOK {
		return nil, errors.New("inventory diff root is unavailable")
	}
	changes := make([]Difference, 0)
	diffValues(&changes, nil, leftRoot, rightRoot)
	return changes, nil
}

func diffValues(changes *[]Difference, path Path, left, right *Value) {
	switch {
	case left == nil && right == nil:
		return
	case left == nil:
		*changes = append(*changes, Difference{Operation: "add", Path: clonePath(path), ValueKind: right.Kind})
		return
	case right == nil:
		*changes = append(*changes, Difference{Operation: "remove", Path: clonePath(path), ValueKind: left.Kind})
		return
	case left.Kind != right.Kind || left.Tag != right.Tag || left.Alias != right.Alias:
		*changes = append(*changes, Difference{Operation: "replace", Path: clonePath(path), ValueKind: right.Kind})
		return
	}

	switch left.Kind {
	case MappingValue:
		rightEntries := mappingEntriesByName(right)
		leftNames := make(map[string]struct{}, len(left.Mapping))
		for index := range left.Mapping {
			entry := &left.Mapping[index]
			name := diffEntryName(entry)
			leftNames[name] = struct{}{}
			other, exists := rightEntries[name]
			if !exists {
				diffValues(changes, entry.Path, &entry.Value, nil)
				continue
			}
			diffValues(changes, entry.Path, &entry.Value, &other.Value)
		}
		for index := range right.Mapping {
			entry := &right.Mapping[index]
			if _, exists := leftNames[diffEntryName(entry)]; !exists {
				diffValues(changes, entry.Path, nil, &entry.Value)
			}
		}
	case SequenceValue:
		if !semanticValueEqual(left, right) {
			*changes = append(*changes, Difference{Operation: "replace", Path: clonePath(path), ValueKind: SequenceValue})
		}
	case ScalarValue:
		if !semanticScalarEqual(left.Scalar, right.Scalar) {
			*changes = append(*changes, Difference{Operation: "replace", Path: clonePath(path), ValueKind: ScalarValue})
		}
	case NullValue:
		return
	default:
		if !semanticValueEqual(left, right) {
			*changes = append(*changes, Difference{Operation: "replace", Path: clonePath(path), ValueKind: right.Kind})
		}
	}
}

func mappingEntriesByName(value *Value) map[string]*Entry {
	result := make(map[string]*Entry, len(value.Mapping))
	for index := range value.Mapping {
		entry := &value.Mapping[index]
		result[diffEntryName(entry)] = entry
	}
	return result
}

func diffEntryName(entry *Entry) string {
	if entry == nil {
		return ""
	}
	if last, ok := entry.Path.Last(); ok && last.Kind == KeySegment {
		return last.Key
	}
	return fmt.Sprint(entry.Key.Value)
}

func semanticValueEqual(left, right *Value) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Kind != right.Kind || left.Tag != right.Tag || left.Alias != right.Alias {
		return false
	}
	switch left.Kind {
	case ScalarValue:
		return semanticScalarEqual(left.Scalar, right.Scalar)
	case NullValue:
		return true
	case SequenceValue:
		if len(left.Sequence) != len(right.Sequence) {
			return false
		}
		for index := range left.Sequence {
			if !semanticValueEqual(&left.Sequence[index], &right.Sequence[index]) {
				return false
			}
		}
		return true
	case MappingValue:
		if len(left.Mapping) != len(right.Mapping) {
			return false
		}
		rightEntries := mappingEntriesByName(right)
		for index := range left.Mapping {
			entry := &left.Mapping[index]
			other, exists := rightEntries[diffEntryName(entry)]
			if !exists || !semanticValueEqual(&entry.Value, &other.Value) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func semanticScalarEqual(left, right *Scalar) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Kind == right.Kind && left.Tag == right.Tag && reflect.DeepEqual(left.Value, right.Value)
}
