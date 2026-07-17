package inventory

import (
	"errors"
	"slices"
)

// SnapshotOperation is a value-free semantic change kind.
type SnapshotOperation string

const (
	SnapshotAdd     SnapshotOperation = "add"
	SnapshotRemove  SnapshotOperation = "remove"
	SnapshotReplace SnapshotOperation = "replace"
	SnapshotReorder SnapshotOperation = "reorder"
)

// SnapshotChange is safe for public plans and conflict output. It carries
// only a declaration path and operation kind, never an Inventory value.
type SnapshotChange struct {
	Operation SnapshotOperation `json:"operation" yaml:"operation"`
	Path      Path              `json:"path,omitempty" yaml:"path,omitempty"`
}

// DiffSnapshots returns declaration-oriented semantic changes. Mapping keys
// are compared by name, order among surviving base keys is an atomic reorder,
// and sequences are atomic at their containing path.
func DiffSnapshots(base, current *Snapshot) ([]SnapshotChange, error) {
	deltas, err := diffSnapshotValues(base, current)
	if err != nil {
		return nil, err
	}
	changes := make([]SnapshotChange, len(deltas))
	for i := range deltas {
		changes[i] = SnapshotChange{Operation: deltas[i].operation, Path: clonePath(deltas[i].path)}
	}
	return changes, nil
}

// SnapshotRemovals returns the value-free subset of a diff that requires the
// CMDB prune scope. Mapping declarations removed from the candidate are
// explicit removals. Sequence replacement is destructive only when an exact
// typed element is lost; replacement of a mapping or sequence by another kind
// is conservatively destructive because it can discard nested declarations.
func SnapshotRemovals(base, current *Snapshot) ([]SnapshotChange, error) {
	deltas, err := diffSnapshotValues(base, current)
	if err != nil {
		return nil, err
	}
	removals := make([]SnapshotChange, 0)
	for i := range deltas {
		delta := deltas[i]
		destructive := delta.operation == SnapshotRemove
		if delta.operation == SnapshotReplace && delta.hasBefore {
			switch delta.before.kind {
			case SnapshotMap:
				destructive = true
			case SnapshotSeq:
				if !delta.hasAfter || delta.after.kind != SnapshotSeq {
					destructive = true
				} else {
					var err error
					destructive, err = sequenceRemovesSnapshotElements(delta.before, delta.after)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		if destructive {
			removals = append(removals, SnapshotChange{
				Operation: delta.operation,
				Path:      clonePath(delta.path),
			})
		}
	}
	return removals, nil
}

func sequenceRemovesSnapshotElements(before, after SnapshotValue) (bool, error) {
	counts := make(map[string]int, len(after.items))
	for i := range after.items {
		key, err := snapshotValueIdentity(after.items[i])
		if err != nil {
			return false, err
		}
		counts[key]++
	}
	for i := range before.items {
		key, err := snapshotValueIdentity(before.items[i])
		if err != nil {
			return false, err
		}
		if counts[key] == 0 {
			return true, nil
		}
		counts[key]--
	}
	return false, nil
}

func snapshotValueIdentity(value SnapshotValue) (string, error) {
	writer := newSnapshotByteWriter(maxSnapshotOutputBytes)
	if err := writeCanonicalSnapshotValue(writer, value); err != nil {
		return "", err
	}
	return string(writer.bytes()), nil
}

type snapshotGap struct {
	previous    string
	hasPrevious bool
	next        string
	hasNext     bool
	ordinal     int
}

type snapshotDelta struct {
	operation SnapshotOperation
	path      Path
	before    SnapshotValue
	after     SnapshotValue
	hasBefore bool
	hasAfter  bool
	gap       snapshotGap
	order     []string
}

func diffSnapshotValues(base, current *Snapshot) ([]snapshotDelta, error) {
	if base == nil || current == nil {
		return nil, errors.New("Snapshot diff requires base and current revisions")
	}
	var deltas []snapshotDelta
	diffSnapshotValue(base.root, current.root, nil, &deltas)
	return deltas, nil
}

func diffSnapshotValue(base, current SnapshotValue, path Path, deltas *[]snapshotDelta) {
	if snapshotValueEqual(base, current) {
		return
	}
	if base.kind != current.kind {
		*deltas = append(*deltas, snapshotDelta{
			operation: SnapshotReplace, path: clonePath(path), before: base, after: current,
			hasBefore: true, hasAfter: true,
		})
		return
	}
	if base.kind != SnapshotMap {
		// Sequences are intentionally atomic in v1; scalar kinds have already
		// been proven equal or unequal above.
		*deltas = append(*deltas, snapshotDelta{
			operation: SnapshotReplace, path: clonePath(path), before: base, after: current,
			hasBefore: true, hasAfter: true,
		})
		return
	}

	baseIndexes := make(map[string]int, len(base.entries))
	currentIndexes := make(map[string]int, len(current.entries))
	for i := range base.entries {
		baseIndexes[base.entries[i].key] = i
	}
	for i := range current.entries {
		currentIndexes[current.entries[i].key] = i
	}
	for i := range base.entries {
		entry := base.entries[i]
		if _, survives := currentIndexes[entry.key]; survives {
			continue
		}
		*deltas = append(*deltas, snapshotDelta{
			operation: SnapshotRemove, path: path.ChildKey(entry.key), before: entry.value, hasBefore: true,
		})
	}

	baseSurvivors := make([]string, 0, min(len(base.entries), len(current.entries)))
	currentSurvivors := make([]string, 0, cap(baseSurvivors))
	for i := range base.entries {
		if _, survives := currentIndexes[base.entries[i].key]; survives {
			baseSurvivors = append(baseSurvivors, base.entries[i].key)
		}
	}
	for i := range current.entries {
		if _, existed := baseIndexes[current.entries[i].key]; existed {
			currentSurvivors = append(currentSurvivors, current.entries[i].key)
		}
	}
	if !slices.Equal(baseSurvivors, currentSurvivors) {
		*deltas = append(*deltas, snapshotDelta{
			operation: SnapshotReorder, path: clonePath(path), before: base, after: current,
			hasBefore: true, hasAfter: true, order: slices.Clone(currentSurvivors),
		})
	}

	gapOrdinals := make(map[snapshotGap]int)
	for i := range current.entries {
		entry := current.entries[i]
		baseIndex, existed := baseIndexes[entry.key]
		if !existed {
			gap := addedSnapshotGap(current.entries, i, baseIndexes)
			identity := gap
			identity.ordinal = 0
			gap.ordinal = gapOrdinals[identity]
			gapOrdinals[identity]++
			*deltas = append(*deltas, snapshotDelta{
				operation: SnapshotAdd, path: path.ChildKey(entry.key), after: entry.value,
				hasAfter: true, gap: gap,
			})
			continue
		}
		diffSnapshotValue(base.entries[baseIndex].value, entry.value, path.ChildKey(entry.key), deltas)
	}
}

func addedSnapshotGap(entries []SnapshotEntry, index int, baseIndexes map[string]int) snapshotGap {
	gap := snapshotGap{}
	for previous := index - 1; previous >= 0; previous-- {
		if _, existed := baseIndexes[entries[previous].key]; existed {
			gap.previous = entries[previous].key
			gap.hasPrevious = true
			break
		}
	}
	for next := index + 1; next < len(entries); next++ {
		if _, existed := baseIndexes[entries[next].key]; existed {
			gap.next = entries[next].key
			gap.hasNext = true
			break
		}
	}
	return gap
}
