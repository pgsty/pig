package inventory

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Mutation is a continuous, in-memory editing session. Each operation is
// reparsed before the next one, so newly inserted mappings and keys are
// immediately addressable. The original Document remains immutable.
type Mutation struct {
	base    *Document
	current *Document
	changes []Change
	failed  error
}

// Mutate starts a continuous editing session.
func (d *Document) Mutate() *Mutation {
	return &Mutation{base: d, current: d}
}

// Document returns the current immutable snapshot of the session.
func (m *Mutation) Document() *Document {
	if m == nil {
		return nil
	}
	if m.failed != nil {
		return m.base
	}
	return m.current
}

// apply runs one low-level edit transaction and makes its result visible to
// subsequent mutation operations. Any failure aborts the entire session.
func (m *Mutation) apply(edit func(*Editor) error) error {
	if m == nil || m.base == nil || m.current == nil {
		return errors.New("inventory mutation is nil")
	}
	if m.failed != nil {
		return fmt.Errorf("inventory mutation aborted: %w", m.failed)
	}
	editor := m.current.Edit()
	if edit == nil {
		return nil
	}
	if err := edit(editor); err != nil {
		return m.fail(err)
	}
	next, changes, err := editor.Commit()
	if err != nil {
		return m.fail(err)
	}
	m.current = next
	m.changes = append(m.changes, changes.Changes...)
	return nil
}

// Set replaces an existing scalar at path.
func (m *Mutation) Set(path Path, value any) error {
	return m.apply(func(editor *Editor) error { return editor.Set(path, value) })
}

// Upsert sets or inserts a mapping entry at path.
func (m *Mutation) Upsert(path Path, value any, options ...InsertOption) error {
	return m.apply(func(editor *Editor) error { return editor.Upsert(path, value, options...) })
}

// Delete removes an active mapping entry.
func (m *Mutation) Delete(path Path) error {
	return m.apply(func(editor *Editor) error { return editor.Delete(path) })
}

// Rename changes only the final mapping-key lexeme at path.
func (m *Mutation) Rename(path Path, newName string) error {
	return m.apply(func(editor *Editor) error { return editor.RenameKey(path, newName) })
}

// ReplaceAll replaces a placeholder in selected YAML key, scalar, and comment
// token regions. It never scans YAML whitespace or delimiters.
func (m *Mutation) ReplaceAll(old, replacement string, options ...ReplaceOption) (int, error) {
	count := 0
	err := m.apply(func(editor *Editor) (err error) {
		count, err = editor.ReplaceAll(old, replacement, options...)
		return err
	})
	return count, err
}

// UncommentKey activates exact commented mapping keys and returns their new
// semantic paths after the candidate is reparsed.
func (m *Mutation) UncommentKey(key string, options ...UncommentOption) (EditResult, error) {
	result := EditResult{}
	before := make(map[string]struct{})
	if m != nil && m.Document() != nil {
		for _, path := range m.Document().PathsByKey(key) {
			before[pathKey(path)] = struct{}{}
		}
	}
	err := m.apply(func(editor *Editor) (err error) {
		result, err = editor.UncommentKey(key, options...)
		return err
	})
	if err != nil || m == nil || m.Document() == nil {
		return result, err
	}
	for _, path := range m.Document().PathsByKey(key) {
		if _, existed := before[pathKey(path)]; !existed {
			result.Paths = append(result.Paths, clonePath(path))
		}
	}
	return result, nil
}

// Commit validates and returns the final snapshot and aggregated, redacted
// ChangeSet. Failed sessions never expose a partially edited Document.
func (m *Mutation) Commit() (*Document, ChangeSet, error) {
	if m == nil || m.base == nil || m.current == nil {
		return nil, ChangeSet{}, errors.New("inventory mutation is nil")
	}
	if m.failed != nil {
		return nil, ChangeSet{}, fmt.Errorf("inventory mutation aborted: %w", m.failed)
	}
	if diagnostics := m.current.Validate(YAMLRules, AnsibleRules); HasErrors(diagnostics) {
		return nil, ChangeSet{}, &ValidationError{Diagnostics: diagnostics}
	}
	return m.current, ChangeSet{Changes: slices.Clone(m.changes)}, nil
}

// MappingEditor edits one mapping in a Mutation. Existing comments are
// preserved; comment options apply only when a key is inserted.
type MappingEditor struct {
	mutation      *Mutation
	path          Path
	ensureOptions []InsertOption
	err           error
}

// Mapping returns a generic mapping editor rooted at path.
func (m *Mutation) Mapping(path Path, options ...InsertOption) *MappingEditor {
	return &MappingEditor{mutation: m, path: clonePath(path), ensureOptions: slices.Clone(options)}
}

// AllVars returns the all.vars mapping editor.
func (m *Mutation) AllVars() *MappingEditor {
	return m.Mapping(AllVars())
}

// GroupVars returns the vars mapping of an exact source group declaration.
func (m *Mutation) GroupVars(group *Group) *MappingEditor {
	if group == nil {
		return &MappingEditor{mutation: m, err: errors.New("inventory group is nil")}
	}
	return m.Mapping(group.Path.ChildKey("vars"))
}

// HostVars returns the variable mapping of an exact source host declaration.
func (m *Mutation) HostVars(host *Host) *MappingEditor {
	if host == nil {
		return &MappingEditor{mutation: m, err: errors.New("inventory host is nil")}
	}
	return m.Mapping(host.Path)
}

// Mapping returns a nested mapping editor. Options control creation of the
// mapping key when it is absent.
func (m *MappingEditor) Mapping(key string, options ...InsertOption) *MappingEditor {
	if m == nil {
		return &MappingEditor{err: errors.New("inventory mapping editor is nil")}
	}
	if m.err != nil {
		return &MappingEditor{mutation: m.mutation, err: m.err}
	}
	return &MappingEditor{mutation: m.mutation, path: m.path.ChildKey(key), ensureOptions: slices.Clone(options)}
}

// Path returns the structured path of this mapping.
func (m *MappingEditor) Path() Path {
	if m == nil {
		return nil
	}
	return clonePath(m.path)
}

// Lookup returns a value in the current session snapshot.
func (m *MappingEditor) Lookup(key string) (*Value, bool) {
	if m == nil || m.err != nil || m.mutation == nil || m.mutation.Document() == nil {
		return nil, false
	}
	return m.mutation.Document().Lookup(m.path.ChildKey(key))
}

// Ensure creates this mapping when it is absent. Block mappings are created
// as an implicit key so later entries inherit normal multiline YAML style.
func (m *MappingEditor) Ensure() error {
	if err := m.ensure(); err != nil {
		return m.fail(err)
	}
	return nil
}

// Set upserts one mapping value, preserving an existing entry's presentation
// or inheriting local style and an optional comment for a new entry.
func (m *MappingEditor) Set(key string, value any, options ...InsertOption) error {
	if err := m.ensure(); err != nil {
		return m.fail(err)
	}
	return m.mutation.Upsert(m.path.ChildKey(key), value, options...)
}

// Delete removes one mapping value.
func (m *MappingEditor) Delete(key string) error {
	if m == nil {
		return errors.New("inventory mapping editor is nil")
	}
	if m.err != nil {
		return m.fail(m.err)
	}
	return m.mutation.Delete(m.path.ChildKey(key))
}

// Rename replaces only one existing mapping-key lexeme and leaves its value,
// whitespace, and surrounding comments untouched.
func (m *MappingEditor) Rename(key, newName string) error {
	if m == nil {
		return errors.New("inventory mapping editor is nil")
	}
	if m.err != nil {
		return m.fail(m.err)
	}
	return m.mutation.Rename(m.path.ChildKey(key), newName)
}

func (m *Mutation) fail(err error) error {
	if err != nil && m != nil && m.failed == nil {
		m.failed = err
	}
	return err
}

func (m *MappingEditor) fail(err error) error {
	if m != nil && m.mutation != nil {
		return m.mutation.fail(err)
	}
	return err
}

func (m *MappingEditor) ensure() error {
	if m == nil {
		return errors.New("inventory mapping editor is nil")
	}
	if m.err != nil {
		return m.err
	}
	if m.mutation == nil || m.mutation.Document() == nil {
		return errors.New("inventory mutation is nil")
	}
	if value, ok := m.mutation.Document().Lookup(m.path); ok {
		switch value.Kind {
		case MappingValue:
			return nil
		case NullValue:
			if value.Scalar == nil || strings.TrimSpace(value.Scalar.Raw) == "" {
				return nil
			}
			return m.mutation.apply(func(editor *Editor) error {
				ref, err := editor.reference(m.path)
				if err != nil {
					return err
				}
				span := ref.valueSpan
				if _, colon, ok := editor.base.keyColon(ref.key); ok && colon+1 <= span.start && horizontalWhitespace(editor.base.source[colon+1:span.start]) {
					span.start = colon + 1
				}
				return editor.stageChange("ensure-mapping", m.path, span, nil, finalIdentifier(m.path))
			})
		default:
			return fmt.Errorf("%s is not a mapping", m.path.String())
		}
	}
	parent, ok := m.path.Parent()
	if !ok {
		return fmt.Errorf("cannot create root mapping %s", m.path.String())
	}
	key, ok := m.path.Last()
	if !ok || key.Kind != KeySegment {
		return fmt.Errorf("mapping path %s must end in a key", m.path.String())
	}
	return m.mutation.apply(func(editor *Editor) error {
		return editor.Insert(parent, key.Key, implicitMapping{}, m.ensureOptions...)
	})
}

// implicitMapping renders as an implicit block mapping key (key:) or an empty
// flow mapping (key: {}). A subsequent Mutation operation can populate it.
type implicitMapping struct{}
