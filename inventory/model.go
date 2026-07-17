// Package inventory provides a lossless document model and a semantic model
// for Pigsty's compact, static Ansible YAML inventories.
//
// Document is the write-back authority: it retains the original source and
// applies local byte patches. Inventory and Value are read-only semantic views
// and must never be marshaled to update the source document.
package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Position is a one-based source position plus its zero-based byte offset.
type Position struct {
	Line   int `json:"line" yaml:"line"`
	Column int `json:"column" yaml:"column"`
	Offset int `json:"offset" yaml:"offset"`
}

// SourceRange is a half-open source range [Start, End).
type SourceRange struct {
	Start Position `json:"start" yaml:"start"`
	End   Position `json:"end" yaml:"end"`
}

// SegmentKind identifies a structured path segment.
type SegmentKind uint8

const (
	KeySegment SegmentKind = iota
	IndexSegment
)

// Segment is either a mapping key or a sequence index. Structured segments
// avoid the ambiguity of dotted JSONPath strings for IP addresses and keys
// containing punctuation.
type Segment struct {
	Kind  SegmentKind `json:"kind" yaml:"kind"`
	Key   string      `json:"key,omitempty" yaml:"key,omitempty"`
	Index int         `json:"index,omitempty" yaml:"index,omitempty"`
}

// Key returns a mapping-key path segment.
func Key(name string) Segment { return Segment{Kind: KeySegment, Key: name} }

// Index returns a sequence-index path segment.
func Index(index int) Segment { return Segment{Kind: IndexSegment, Index: index} }

// Path is a structured path into the source YAML document.
type Path []Segment

// Keys constructs a path containing mapping-key segments.
func Keys(names ...string) Path {
	p := make(Path, 0, len(names))
	for _, name := range names {
		p = append(p, Key(name))
	}
	return p
}

// AllVars returns the path to all.vars or one of its descendants.
func AllVars(names ...string) Path {
	return Keys(append([]string{"all", "vars"}, names...)...)
}

// HostVars returns the declaration path for a host in a group beneath all.
func HostVars(group, host string) Path {
	return Keys("all", "children", group, "hosts", host)
}

// ChildKey appends a mapping key without modifying the receiver.
func (p Path) ChildKey(name string) Path {
	return append(slices.Clone(p), Key(name))
}

// ChildIndex appends a sequence index without modifying the receiver.
func (p Path) ChildIndex(index int) Path {
	return append(slices.Clone(p), Index(index))
}

// Parent returns the parent path and whether a parent exists.
func (p Path) Parent() (Path, bool) {
	if len(p) == 0 {
		return nil, false
	}
	return slices.Clone(p[:len(p)-1]), true
}

// Last returns the final path segment and whether it exists.
func (p Path) Last() (Segment, bool) {
	if len(p) == 0 {
		return Segment{}, false
	}
	return p[len(p)-1], true
}

// String renders a diagnostic-only pointer representation. It is not parsed
// by the editing API.
func (p Path) String() string {
	if len(p) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, segment := range p {
		b.WriteByte('/')
		if segment.Kind == IndexSegment {
			b.WriteString(strconv.Itoa(segment.Index))
			continue
		}
		key := strings.ReplaceAll(segment.Key, "~", "~0")
		key = strings.ReplaceAll(key, "/", "~1")
		b.WriteString(key)
	}
	return b.String()
}

func pathKey(p Path) string {
	var b strings.Builder
	for _, segment := range p {
		if segment.Kind == IndexSegment {
			fmt.Fprintf(&b, "i%d;", segment.Index)
			continue
		}
		fmt.Fprintf(&b, "k%d:%s;", len(segment.Key), segment.Key)
	}
	return b.String()
}

// ValueKind identifies the semantic shape of a YAML value.
type ValueKind string

const (
	ScalarValue   ValueKind = "scalar"
	MappingValue  ValueKind = "mapping"
	SequenceValue ValueKind = "sequence"
	NullValue     ValueKind = "null"
)

// ScalarKind is the resolved scalar type while Raw retains the source lexeme.
type ScalarKind string

const (
	StringScalar  ScalarKind = "string"
	BoolScalar    ScalarKind = "bool"
	IntegerScalar ScalarKind = "integer"
	FloatScalar   ScalarKind = "float"
	NullScalar    ScalarKind = "null"
	UnknownScalar ScalarKind = "unknown"
)

// Scalar retains both the semantic value and its exact source lexeme.
type Scalar struct {
	Kind  ScalarKind  `json:"kind" yaml:"kind"`
	Value any         `json:"value,omitempty" yaml:"value,omitempty"`
	Raw   string      `json:"raw" yaml:"raw"`
	Tag   string      `json:"tag,omitempty" yaml:"tag,omitempty"`
	Style string      `json:"style,omitempty" yaml:"style,omitempty"`
	Range SourceRange `json:"range" yaml:"range"`
}

// Entry is an ordered mapping entry.
type Entry struct {
	Key   Scalar      `json:"key" yaml:"key"`
	Value Value       `json:"value" yaml:"value"`
	Path  Path        `json:"path" yaml:"path"`
	Range SourceRange `json:"range" yaml:"range"`
}

// Value is an ordered, generic YAML value used by vars, host declarations,
// and future consumers. Mapping and Sequence preserve declaration order.
type Value struct {
	Kind     ValueKind   `json:"kind" yaml:"kind"`
	Scalar   *Scalar     `json:"scalar,omitempty" yaml:"scalar,omitempty"`
	Mapping  []Entry     `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	Sequence []Value     `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	Flow     bool        `json:"flow,omitempty" yaml:"flow,omitempty"`
	Tag      string      `json:"tag,omitempty" yaml:"tag,omitempty"`
	Anchor   string      `json:"anchor,omitempty" yaml:"anchor,omitempty"`
	Alias    string      `json:"alias,omitempty" yaml:"alias,omitempty"`
	Path     Path        `json:"path" yaml:"path"`
	Range    SourceRange `json:"range" yaml:"range"`
}

// LookupKey returns the first ordered mapping entry with the given key.
func (v *Value) LookupKey(key string) (*Value, bool) {
	if v == nil || v.Kind != MappingValue {
		return nil, false
	}
	for i := range v.Mapping {
		if fmt.Sprint(v.Mapping[i].Key.Value) == key {
			return &v.Mapping[i].Value, true
		}
	}
	return nil, false
}

// Host is one host declaration. Duplicate host names in different groups are
// intentionally retained as independent declarations.
type Host struct {
	Name  string      `json:"name" yaml:"name"`
	Vars  Value       `json:"vars" yaml:"vars"`
	Path  Path        `json:"path" yaml:"path"`
	Range SourceRange `json:"range" yaml:"range"`
}

// Group models an ordered source group declaration.
type Group struct {
	Name     string      `json:"name" yaml:"name"`
	Vars     *Value      `json:"vars,omitempty" yaml:"vars,omitempty"`
	Hosts    []*Host     `json:"hosts,omitempty" yaml:"hosts,omitempty"`
	Children []*Group    `json:"children,omitempty" yaml:"children,omitempty"`
	Path     Path        `json:"path" yaml:"path"`
	Range    SourceRange `json:"range" yaml:"range"`
}

// Inventory is the declaration-oriented semantic view of one static file.
// It does not resolve Ansible variable precedence or effective variables.
type Inventory struct {
	All    *Group   `json:"all,omitempty" yaml:"all,omitempty"`
	Groups []*Group `json:"groups,omitempty" yaml:"groups,omitempty"`
}

// Document retains the original bytes plus private source and semantic indexes.
// goccy CST/token types are transient; the private yaml.Node is never exposed.
type Document struct {
	name       string
	source     []byte
	lineStarts []int
	newline    string
	root       *yaml.Node
	scalarFlow map[*yaml.Node]bool
	comments   []byteSpan
	refs       map[string]*nodeRef
	values     map[string]*Value
	keyPaths   map[string][]Path
	inventory  *Inventory
}

// Name returns the logical source name passed to Parse or Load.
func (d *Document) Name() string {
	if d == nil {
		return ""
	}
	return d.name
}

// Bytes returns a defensive copy of the exact source bytes.
func (d *Document) Bytes() []byte {
	if d == nil {
		return nil
	}
	return slices.Clone(d.source)
}

// Inventory returns the immutable declaration-oriented semantic view. Values,
// groups, and hosts are bound to this Document snapshot; retain their Path,
// rather than their pointer, across an Editor or Mutation commit.
func (d *Document) Inventory() *Inventory {
	if d == nil {
		return nil
	}
	return d.inventory
}

// Lookup returns the semantic value at path.
func (d *Document) Lookup(path Path) (*Value, bool) {
	if d == nil {
		return nil, false
	}
	value, ok := d.values[pathKey(path)]
	return value, ok
}

// PathsByKey returns active mapping-value paths whose final key matches key,
// in source declaration order.
func (d *Document) PathsByKey(key string) []Path {
	if d == nil {
		return nil
	}
	paths := d.keyPaths[key]
	out := make([]Path, len(paths))
	for i := range paths {
		out[i] = slices.Clone(paths[i])
	}
	return out
}

// WriteTo writes the current lossless source without normalizing YAML.
func (d *Document) WriteTo(writer io.Writer) (int64, error) {
	if d == nil {
		return 0, errors.New("inventory document is nil")
	}
	if writer == nil {
		return 0, errors.New("inventory writer is nil")
	}
	return io.Copy(writer, bytes.NewReader(d.source))
}
