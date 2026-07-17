package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	// SnapshotKind identifies the only semantic Snapshot encoding accepted by
	// the native CMDB v1 contract.
	SnapshotKind = "pig.inventory.snapshot/v1"

	maxSnapshotSourceBytes = 16 << 20
	maxSnapshotNodes       = 250_000
	maxSnapshotOutputBytes = 64 << 20
)

// SnapshotValueKind is the closed set of typed values in Snapshot v1.
type SnapshotValueKind string

const (
	SnapshotMap   SnapshotValueKind = "map"
	SnapshotSeq   SnapshotValueKind = "seq"
	SnapshotStr   SnapshotValueKind = "str"
	SnapshotBool  SnapshotValueKind = "bool"
	SnapshotInt   SnapshotValueKind = "int"
	SnapshotFloat SnapshotValueKind = "float"
	SnapshotNull  SnapshotValueKind = "null"
)

// SnapshotReason is a stable, value-free representability reason.
type SnapshotReason string

const (
	SnapshotReasonNilDocument       SnapshotReason = "nil-document"
	SnapshotReasonSourceTooLarge    SnapshotReason = "source-too-large"
	SnapshotReasonRootType          SnapshotReason = "root-not-mapping"
	SnapshotReasonDepth             SnapshotReason = "depth-limit"
	SnapshotReasonNodeLimit         SnapshotReason = "node-limit"
	SnapshotReasonOutputTooLarge    SnapshotReason = "output-too-large"
	SnapshotReasonExplicitTag       SnapshotReason = "explicit-tag"
	SnapshotReasonCustomTag         SnapshotReason = "custom-tag"
	SnapshotReasonAnchor            SnapshotReason = "anchor"
	SnapshotReasonAlias             SnapshotReason = "alias"
	SnapshotReasonMergeKey          SnapshotReason = "merge-key"
	SnapshotReasonMappingKeyType    SnapshotReason = "mapping-key-not-string"
	SnapshotReasonDuplicateKey      SnapshotReason = "duplicate-mapping-key"
	SnapshotReasonUnknownScalar     SnapshotReason = "unknown-scalar"
	SnapshotReasonIntegerRange      SnapshotReason = "integer-out-of-range"
	SnapshotReasonFloatNonFinite    SnapshotReason = "non-finite-float"
	SnapshotReasonDuplicateHostVars SnapshotReason = "conflicting-duplicate-host"
	SnapshotReasonInvalidProjection SnapshotReason = "invalid-inventory-projection"
	SnapshotReasonRoundTripMismatch SnapshotReason = "round-trip-mismatch"
)

// SnapshotError is safe for public diagnostics: it contains a declaration
// path and a fixed reason, never the rejected Inventory value.
type SnapshotError struct {
	Code   string         `json:"code" yaml:"code"`
	Path   Path           `json:"path,omitempty" yaml:"path,omitempty"`
	Reason SnapshotReason `json:"reason" yaml:"reason"`
}

func (e *SnapshotError) Error() string {
	if e == nil {
		return "CMDB_SNAPSHOT_UNREPRESENTABLE"
	}
	return fmt.Sprintf("%s at %s: %s", e.Code, e.Path.String(), e.Reason)
}

func snapshotFailure(path Path, reason SnapshotReason) *SnapshotError {
	return &SnapshotError{
		Code:   "CMDB_SNAPSHOT_UNREPRESENTABLE",
		Path:   clonePath(path),
		Reason: reason,
	}
}

// Snapshot is an immutable, declaration-ordered semantic Inventory. Its
// cached bytes are the sole canonical representation used for revision
// digests; source presentation never enters the payload.
type Snapshot struct {
	root      SnapshotValue
	canonical []byte
	digest    [sha256.Size]byte
	nodeCount int
}

// SnapshotEntry is one declaration-ordered mapping member. Accessors return
// defensive copies so a Snapshot remains immutable outside this package.
type SnapshotEntry struct {
	key   string
	value SnapshotValue
}

// SnapshotValue is one typed Snapshot node.
type SnapshotValue struct {
	kind    SnapshotValueKind
	entries []SnapshotEntry
	items   []SnapshotValue
	text    string
	boolean bool
}

// SnapshotFromDocument builds the ordered CMDB v1 semantic representation of
// an already parsed static Document and enforces the full representability
// gate. Callers remain responsible for selecting any additional Pigsty
// completeness rules required by their use case before a database plan.
func SnapshotFromDocument(doc *Document) (*Snapshot, error) {
	return snapshotFromDocument(doc, true)
}

func snapshotFromDocument(doc *Document, enforceSourceLimit bool) (*Snapshot, error) {
	if doc == nil || doc.root == nil {
		return nil, snapshotFailure(nil, SnapshotReasonNilDocument)
	}
	if enforceSourceLimit && len(doc.source) > maxSnapshotSourceBytes {
		return nil, snapshotFailure(nil, SnapshotReasonSourceTooLarge)
	}
	if doc.root.Kind != yaml.MappingNode {
		return nil, snapshotFailure(nil, SnapshotReasonRootType)
	}
	builder := snapshotBuilder{}
	root, err := builder.value(doc.root, nil, 0)
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
	snapshot := &Snapshot{
		root:      root,
		canonical: canonical,
		digest:    sha256.Sum256(canonical),
		nodeCount: builder.nodes,
	}
	return snapshot, nil
}

// Kind returns the fixed Snapshot payload kind.
func (s *Snapshot) Kind() string {
	if s == nil {
		return ""
	}
	return SnapshotKind
}

// Root returns a defensive copy of the ordered root value.
func (s *Snapshot) Root() SnapshotValue {
	if s == nil {
		return SnapshotValue{}
	}
	return cloneSnapshotValue(s.root)
}

// CanonicalBytes returns the exact typed JSON bytes used for the digest.
func (s *Snapshot) CanonicalBytes() []byte {
	if s == nil {
		return nil
	}
	return slices.Clone(s.canonical)
}

// SemanticSHA256 returns the binary digest of CanonicalBytes.
func (s *Snapshot) SemanticSHA256() [sha256.Size]byte {
	if s == nil {
		return [sha256.Size]byte{}
	}
	return s.digest
}

// Digest returns the lowercase hexadecimal semantic SHA-256.
func (s *Snapshot) Digest() string {
	if s == nil {
		return ""
	}
	return hex.EncodeToString(s.digest[:])
}

// NodeCount returns the number of typed value nodes, excluding mapping keys.
func (s *Snapshot) NodeCount() int {
	if s == nil {
		return 0
	}
	return s.nodeCount
}

// Equal reports exact typed-value and declaration-order equality.
func (s *Snapshot) Equal(other *Snapshot) bool {
	return s != nil && other != nil && s.digest == other.digest &&
		string(s.canonical) == string(other.canonical)
}

// MarshalJSON always returns the canonical Snapshot v1 JSON representation.
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	return s.CanonicalBytes(), nil
}

// Kind returns this value's closed Snapshot type.
func (v SnapshotValue) Kind() SnapshotValueKind { return v.kind }

// Entries returns declaration-ordered defensive mapping entries.
func (v SnapshotValue) Entries() []SnapshotEntry {
	if v.kind != SnapshotMap {
		return nil
	}
	entries := make([]SnapshotEntry, len(v.entries))
	for i := range v.entries {
		entries[i] = SnapshotEntry{key: v.entries[i].key, value: cloneSnapshotValue(v.entries[i].value)}
	}
	return entries
}

// Items returns declaration-ordered defensive sequence values.
func (v SnapshotValue) Items() []SnapshotValue {
	if v.kind != SnapshotSeq {
		return nil
	}
	items := make([]SnapshotValue, len(v.items))
	for i := range v.items {
		items[i] = cloneSnapshotValue(v.items[i])
	}
	return items
}

// String returns the string scalar and whether this is a string node.
func (v SnapshotValue) String() (string, bool) { return v.text, v.kind == SnapshotStr }

// Bool returns the boolean scalar and whether this is a boolean node.
func (v SnapshotValue) Bool() (bool, bool) { return v.boolean, v.kind == SnapshotBool }

// Number returns the canonical integer or float spelling and whether this is a
// numeric node.
func (v SnapshotValue) Number() (string, bool) {
	return v.text, v.kind == SnapshotInt || v.kind == SnapshotFloat
}

// Lookup returns a defensive copy of one mapping value.
func (v SnapshotValue) Lookup(key string) (SnapshotValue, bool) {
	if v.kind != SnapshotMap {
		return SnapshotValue{}, false
	}
	for i := range v.entries {
		if v.entries[i].key == key {
			return cloneSnapshotValue(v.entries[i].value), true
		}
	}
	return SnapshotValue{}, false
}

// Key returns this mapping entry's string key.
func (e SnapshotEntry) Key() string { return e.key }

// Value returns a defensive copy of this mapping entry's value.
func (e SnapshotEntry) Value() SnapshotValue { return cloneSnapshotValue(e.value) }

type snapshotBuilder struct {
	nodes int
}

func (b *snapshotBuilder) value(node *yaml.Node, path Path, depth int) (SnapshotValue, error) {
	if node == nil {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
	}
	if depth > maxDocumentDepth {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonDepth)
	}
	b.nodes++
	if b.nodes > maxSnapshotNodes {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonNodeLimit)
	}
	if node.Kind == yaml.AliasNode {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonAlias)
	}
	if node.Anchor != "" {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonAnchor)
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonExplicitTag)
	}

	switch node.Kind {
	case yaml.MappingNode:
		if !coreYAMLTag(node.Tag, "!!map") {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonCustomTag)
		}
		result := SnapshotValue{kind: SnapshotMap, entries: make([]SnapshotEntry, 0, len(node.Content)/2)}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			if keyNode == nil || keyNode.Kind != yaml.ScalarNode || keyNode.Anchor != "" {
				return SnapshotValue{}, snapshotFailure(path, SnapshotReasonMappingKeyType)
			}
			if keyNode.Style&yaml.TaggedStyle != 0 {
				return SnapshotValue{}, snapshotFailure(path, SnapshotReasonExplicitTag)
			}
			if keyNode.Tag == "!!merge" {
				return SnapshotValue{}, snapshotFailure(path, SnapshotReasonMergeKey)
			}
			if !coreYAMLTag(keyNode.Tag, "!!str") {
				return SnapshotValue{}, snapshotFailure(path, SnapshotReasonMappingKeyType)
			}
			key := keyNode.Value
			childPath := path.ChildKey(key)
			if _, exists := seen[key]; exists {
				return SnapshotValue{}, snapshotFailure(childPath, SnapshotReasonDuplicateKey)
			}
			seen[key] = struct{}{}
			child, err := b.value(valueNode, childPath, depth+1)
			if err != nil {
				return SnapshotValue{}, err
			}
			result.entries = append(result.entries, SnapshotEntry{key: key, value: child})
		}
		return result, nil

	case yaml.SequenceNode:
		if !coreYAMLTag(node.Tag, "!!seq") {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonCustomTag)
		}
		result := SnapshotValue{kind: SnapshotSeq, items: make([]SnapshotValue, len(node.Content))}
		for i := range node.Content {
			child, err := b.value(node.Content[i], path.ChildIndex(i), depth+1)
			if err != nil {
				return SnapshotValue{}, err
			}
			result.items[i] = child
		}
		return result, nil

	case yaml.ScalarNode:
		return snapshotScalar(node, path)
	default:
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
	}
}

func coreYAMLTag(actual, expected string) bool {
	return actual == "" || actual == expected || actual == "tag:yaml.org,2002:"+strings.TrimPrefix(expected, "!!")
}

func snapshotScalar(node *yaml.Node, path Path) (SnapshotValue, error) {
	switch node.Tag {
	case "!!str", "tag:yaml.org,2002:str":
		return SnapshotValue{kind: SnapshotStr, text: node.Value}, nil
	case "!!bool", "tag:yaml.org,2002:bool":
		var value bool
		if err := node.Decode(&value); err != nil {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
		}
		return SnapshotValue{kind: SnapshotBool, boolean: value}, nil
	case "!!int", "tag:yaml.org,2002:int":
		if value, ok := canonicalSnapshotInteger(node); ok {
			return SnapshotValue{kind: SnapshotInt, text: value}, nil
		}
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonIntegerRange)
	case "!!float", "tag:yaml.org,2002:float":
		if looksLikeDecimalInteger(node.Value) {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonIntegerRange)
		}
		var value float64
		if err := node.Decode(&value); err != nil {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonFloatNonFinite)
		}
		return SnapshotValue{kind: SnapshotFloat, text: strconv.FormatFloat(value, 'g', -1, 64)}, nil
	case "!!null", "tag:yaml.org,2002:null", "":
		if node.Tag == "" && node.Value != "" && !strings.EqualFold(node.Value, "null") && node.Value != "~" {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
		}
		return SnapshotValue{kind: SnapshotNull}, nil
	default:
		if isCustomYAMLTag(node.Tag) {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonCustomTag)
		}
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
	}
}

func looksLikeDecimalInteger(value string) bool {
	if value == "" {
		return false
	}
	start := 0
	if value[0] == '+' || value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return false
	}
	digits := 0
	for _, character := range value[start:] {
		if character == '_' {
			continue
		}
		if character < '0' || character > '9' {
			return false
		}
		digits++
	}
	return digits > 0
}

func canonicalSnapshotInteger(node *yaml.Node) (string, bool) {
	var signed int64
	if err := node.Decode(&signed); err == nil {
		return strconv.FormatInt(signed, 10), true
	}
	var unsigned uint64
	if err := node.Decode(&unsigned); err == nil {
		return strconv.FormatUint(unsigned, 10), true
	}
	return "", false
}

func validateSnapshotHostDeclarations(root SnapshotValue) error {
	type declaration struct {
		value SnapshotValue
	}
	seen := make(map[string]declaration)
	var walkGroup func(SnapshotValue, Path) error
	walkGroup = func(group SnapshotValue, path Path) error {
		if group.kind != SnapshotMap {
			return nil
		}
		if hosts, ok := snapshotLookupRaw(group, "hosts"); ok && hosts.kind == SnapshotMap {
			for i := range hosts.entries {
				host := hosts.entries[i]
				hostPath := path.ChildKey("hosts").ChildKey(host.key)
				if previous, exists := seen[host.key]; exists {
					if !snapshotValueEqual(previous.value, host.value) {
						return snapshotFailure(hostPath, SnapshotReasonDuplicateHostVars)
					}
					continue
				}
				seen[host.key] = declaration{value: host.value}
			}
		}
		if children, ok := snapshotLookupRaw(group, "children"); ok && children.kind == SnapshotMap {
			for i := range children.entries {
				child := children.entries[i]
				if err := walkGroup(child.value, path.ChildKey("children").ChildKey(child.key)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if root.kind != SnapshotMap {
		return snapshotFailure(nil, SnapshotReasonRootType)
	}
	for i := range root.entries {
		entry := root.entries[i]
		if err := walkGroup(entry.value, Keys(entry.key)); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalSnapshot(root SnapshotValue) ([]byte, error) {
	writer := newSnapshotByteWriter(maxSnapshotOutputBytes)
	if err := writer.literal(`{"kind":"pig.inventory.snapshot/v1","root":`); err != nil {
		return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	if err := writeCanonicalSnapshotValue(writer, root); err != nil {
		return nil, err
	}
	if err := writer.byte('}'); err != nil {
		return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return writer.bytes(), nil
}

func writeCanonicalSnapshotValue(writer *snapshotByteWriter, value SnapshotValue) error {
	var err error
	switch value.kind {
	case SnapshotMap:
		err = writer.literal(`{"t":"map","v":[`)
		for i := range value.entries {
			if err != nil {
				break
			}
			if i > 0 {
				err = writer.byte(',')
			}
			if err == nil {
				err = writer.literal(`{"k":`)
			}
			if err == nil {
				err = writer.jsonString(value.entries[i].key)
			}
			if err == nil {
				err = writer.literal(`,"v":`)
			}
			if err == nil {
				err = writeCanonicalSnapshotValue(writer, value.entries[i].value)
			}
			if err == nil {
				err = writer.byte('}')
			}
		}
		if err == nil {
			err = writer.literal(`]}`)
		}
	case SnapshotSeq:
		err = writer.literal(`{"t":"seq","v":[`)
		for i := range value.items {
			if err != nil {
				break
			}
			if i > 0 {
				err = writer.byte(',')
			}
			if err == nil {
				err = writeCanonicalSnapshotValue(writer, value.items[i])
			}
		}
		if err == nil {
			err = writer.literal(`]}`)
		}
	case SnapshotStr:
		err = writer.literal(`{"t":"str","v":`)
		if err == nil {
			err = writer.jsonString(value.text)
		}
		if err == nil {
			err = writer.byte('}')
		}
	case SnapshotBool:
		err = writer.literal(`{"t":"bool","v":`)
		if err == nil {
			err = writer.literal(strconv.FormatBool(value.boolean))
		}
		if err == nil {
			err = writer.byte('}')
		}
	case SnapshotInt:
		err = writer.literal(`{"t":"int","v":`)
		if err == nil {
			err = writer.jsonString(value.text)
		}
		if err == nil {
			err = writer.byte('}')
		}
	case SnapshotFloat:
		err = writer.literal(`{"t":"float","v":`)
		if err == nil {
			err = writer.jsonString(value.text)
		}
		if err == nil {
			err = writer.byte('}')
		}
	case SnapshotNull:
		err = writer.literal(`{"t":"null"}`)
	default:
		return snapshotFailure(nil, SnapshotReasonUnknownScalar)
	}
	if err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return nil
}

type snapshotByteWriter struct {
	buf   []byte
	limit int
}

func newSnapshotByteWriter(limit int) *snapshotByteWriter {
	return &snapshotByteWriter{buf: make([]byte, 0, min(limit, 4096)), limit: limit}
}

func (w *snapshotByteWriter) bytes() []byte { return slices.Clone(w.buf) }

func (w *snapshotByteWriter) literal(value string) error {
	if len(value) > w.limit-len(w.buf) {
		return errors.New("snapshot output limit exceeded")
	}
	w.buf = append(w.buf, value...)
	return nil
}

func (w *snapshotByteWriter) byte(value byte) error {
	if len(w.buf) >= w.limit {
		return errors.New("snapshot output limit exceeded")
	}
	w.buf = append(w.buf, value)
	return nil
}

// jsonString matches encoding/json string escaping with HTML escaping
// disabled. U+2028/U+2029 remain escaped so the canonical spelling is fixed
// across JavaScript-capable consumers.
func (w *snapshotByteWriter) jsonString(value string) error {
	if err := w.byte('"'); err != nil {
		return err
	}
	start := 0
	for i := 0; i < len(value); {
		if value[i] < utf8.RuneSelf {
			byteValue := value[i]
			if byteValue >= 0x20 && byteValue != '\\' && byteValue != '"' {
				i++
				continue
			}
			if start < i {
				if err := w.literal(value[start:i]); err != nil {
					return err
				}
			}
			escape := ""
			switch byteValue {
			case '\\', '"':
				escape = "\\" + string(byteValue)
			case '\b':
				escape = `\b`
			case '\f':
				escape = `\f`
			case '\n':
				escape = `\n`
			case '\r':
				escape = `\r`
			case '\t':
				escape = `\t`
			default:
				escape = `\u00` + string("0123456789abcdef"[byteValue>>4]) + string("0123456789abcdef"[byteValue&0x0f])
			}
			if err := w.literal(escape); err != nil {
				return err
			}
			i++
			start = i
			continue
		}
		runeValue, size := utf8.DecodeRuneInString(value[i:])
		if runeValue == utf8.RuneError && size == 1 {
			return errors.New("invalid UTF-8 string")
		}
		if runeValue == '\u2028' || runeValue == '\u2029' {
			if start < i {
				if err := w.literal(value[start:i]); err != nil {
					return err
				}
			}
			if err := w.literal(fmt.Sprintf(`\u%04x`, runeValue)); err != nil {
				return err
			}
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(value) {
		if err := w.literal(value[start:]); err != nil {
			return err
		}
	}
	return w.byte('"')
}

func snapshotLookupRaw(value SnapshotValue, key string) (SnapshotValue, bool) {
	if value.kind != SnapshotMap {
		return SnapshotValue{}, false
	}
	for i := range value.entries {
		if value.entries[i].key == key {
			return value.entries[i].value, true
		}
	}
	return SnapshotValue{}, false
}

func snapshotValueEqual(left, right SnapshotValue) bool {
	if left.kind != right.kind || left.text != right.text || left.boolean != right.boolean ||
		len(left.entries) != len(right.entries) || len(left.items) != len(right.items) {
		return false
	}
	for i := range left.entries {
		if left.entries[i].key != right.entries[i].key || !snapshotValueEqual(left.entries[i].value, right.entries[i].value) {
			return false
		}
	}
	for i := range left.items {
		if !snapshotValueEqual(left.items[i], right.items[i]) {
			return false
		}
	}
	return true
}

func cloneSnapshotValue(value SnapshotValue) SnapshotValue {
	clone := SnapshotValue{kind: value.kind, text: value.text, boolean: value.boolean}
	if len(value.entries) > 0 {
		clone.entries = make([]SnapshotEntry, len(value.entries))
		for i := range value.entries {
			clone.entries[i] = SnapshotEntry{key: value.entries[i].key, value: cloneSnapshotValue(value.entries[i].value)}
		}
	}
	if len(value.items) > 0 {
		clone.items = make([]SnapshotValue, len(value.items))
		for i := range value.items {
			clone.items[i] = cloneSnapshotValue(value.items[i])
		}
	}
	return clone
}
