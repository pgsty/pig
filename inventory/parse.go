package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
	"gopkg.in/yaml.v3"
)

const maxDocumentDepth = 128

type byteSpan struct {
	start int
	end   int
}

func (s byteSpan) valid(limit int) bool {
	return s.start >= 0 && s.end >= s.start && s.end <= limit
}

type nodeRef struct {
	path      Path
	node      *yaml.Node
	parent    *yaml.Node
	key       *yaml.Node
	entry     int
	keySpan   byteSpan
	valueSpan byteSpan
	entrySpan byteSpan
}

// Parse parses one non-empty UTF-8 YAML document while retaining its exact
// source representation. goccy/go-yaml identifies comment tokens while the
// private yaml.Node tree supplies semantics; neither is used for write-back
// marshaling.
func Parse(name string, source []byte) (*Document, error) {
	if !utf8.Valid(source) {
		return nil, fmt.Errorf("%s: inventory source is not valid UTF-8", sourceName(name))
	}
	if len(bytes.TrimSpace(source)) == 0 {
		return nil, fmt.Errorf("%s: inventory must contain one non-empty YAML document", sourceName(name))
	}

	tokens := lexer.Tokenize(string(source))
	root, err := decodeOneYAMLDocument(source)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid YAML: %w", sourceName(name), err)
	}

	doc := &Document{
		name:       name,
		source:     bytes.Clone(source),
		lineStarts: buildLineStarts(source),
		newline:    detectNewline(source),
		root:       root,
		scalarFlow: make(map[*yaml.Node]bool),
		refs:       make(map[string]*nodeRef),
		values:     make(map[string]*Value),
		keyPaths:   make(map[string][]Path),
	}
	doc.indexScalarContexts(root, false)
	if err := doc.indexComments(tokens); err != nil {
		return nil, fmt.Errorf("%s: invalid YAML source index: %w", sourceName(name), err)
	}
	var rootValue Value
	if err := doc.indexNode(root, nil, nil, -1, nil, &rootValue, 0); err != nil {
		return nil, fmt.Errorf("%s: %w", sourceName(name), err)
	}
	doc.inventory = doc.buildInventory()
	return doc, nil
}

// Load reads and parses a static YAML inventory.
func Load(path string) (*Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("inventory source must be one static YAML file")
	}
	if info.Mode().Perm()&0111 != 0 {
		return nil, fmt.Errorf("executable dynamic inventory sources are unsupported")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, source)
}

func sourceName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "inventory"
	}
	return name
}

func decodeOneYAMLDocument(source []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("inventory must contain one non-empty YAML document")
		}
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0] == nil {
		return nil, errors.New("inventory must contain one non-empty YAML document")
	}
	var extra yaml.Node
	err := decoder.Decode(&extra)
	if err == nil {
		return nil, errors.New("inventory must contain exactly one YAML document")
	}
	if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return document.Content[0], nil
}

func buildLineStarts(source []byte) []int {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func detectNewline(source []byte) string {
	if bytes.Contains(source, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// offsetAt computes byte offsets from the parser's one-based rune column. It
// intentionally never uses token.Position.Offset (see goccy/go-yaml #856).
func (d *Document) offsetAt(line, column int) (int, bool) {
	if d == nil || line < 1 || line > len(d.lineStarts) || column < 1 {
		return 0, false
	}
	start := d.lineStarts[line-1]
	end := len(d.source)
	if line < len(d.lineStarts) {
		end = d.lineStarts[line] - 1
		if end > start && d.source[end-1] == '\r' {
			end--
		}
	}
	offset := start
	for col := 1; col < column; col++ {
		if offset >= end {
			return 0, false
		}
		_, size := utf8.DecodeRune(d.source[offset:end])
		if size <= 0 {
			return 0, false
		}
		offset += size
	}
	return offset, true
}

func (d *Document) positionAt(offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(d.source) {
		offset = len(d.source)
	}
	lineIndex := sort.Search(len(d.lineStarts), func(i int) bool { return d.lineStarts[i] > offset }) - 1
	if lineIndex < 0 {
		lineIndex = 0
	}
	lineStart := d.lineStarts[lineIndex]
	column := utf8.RuneCount(d.source[lineStart:offset]) + 1
	return Position{Line: lineIndex + 1, Column: column, Offset: offset}
}

func (d *Document) sourceRange(span byteSpan) SourceRange {
	return SourceRange{Start: d.positionAt(span.start), End: d.positionAt(span.end)}
}

// indexComments uses goccy's lexer only to decide which source lines contain
// YAML comments. The token offset/origin is deliberately not trusted: comment
// tokens may include leading trivia, and their byte offset can drift after a
// prior comment. The comment text is instead tail-anchored on its source line.
func (d *Document) indexComments(tokens token.Tokens) error {
	for _, tok := range tokens {
		if tok == nil || tok.Type != token.CommentType {
			continue
		}
		span, err := d.locateComment(tok)
		if err != nil {
			return err
		}
		d.comments = append(d.comments, span)
	}
	return d.normalizeCommentSpans()
}

func (d *Document) locateComment(tok *token.Token) (byteSpan, error) {
	if tok.Position == nil {
		return byteSpan{}, errors.New("comment token has no source position")
	}
	line := tok.Position.Line
	if line < 1 || line > len(d.lineStarts) {
		return byteSpan{}, fmt.Errorf("comment position is outside source at line %d", line)
	}
	lineStart := d.lineStarts[line-1]
	lineEnd := d.lineEnd(lineStart, false)
	if lineEnd > lineStart && d.source[lineEnd-1] == '\r' {
		lineEnd--
	}
	value := []byte(tok.Value)
	hash := lineEnd - len(value) - 1
	if hash < lineStart || hash >= lineEnd || d.source[hash] != '#' || !bytes.Equal(d.source[hash+1:lineEnd], value) {
		var err error
		hash, err = d.searchCommentHash(lineStart, lineEnd, value, line)
		if err != nil {
			return byteSpan{}, err
		}
	}
	if hash < 0 || (hash > lineStart && d.source[hash-1] != ' ' && d.source[hash-1] != '\t') {
		return byteSpan{}, fmt.Errorf("comment cannot be located at line %d", line)
	}
	return byteSpan{start: hash, end: lineEnd}, nil
}

func (d *Document) searchCommentHash(lineStart, lineEnd int, value []byte, line int) (int, error) {
	hash := -1
	for candidate := lineStart; candidate < lineEnd; candidate++ {
		if d.source[candidate] != '#' || !bytes.Equal(d.source[candidate+1:lineEnd], value) {
			continue
		}
		if hash >= 0 {
			return -1, fmt.Errorf("comment cannot be located uniquely at line %d", line)
		}
		hash = candidate
	}
	return hash, nil
}

func (d *Document) normalizeCommentSpans() error {
	sort.Slice(d.comments, func(i, j int) bool { return d.comments[i].start < d.comments[j].start })
	unique := d.comments[:0]
	for _, comment := range d.comments {
		if !comment.valid(len(d.source)) || comment.start >= comment.end || d.source[comment.start] != '#' {
			return errors.New("invalid comment source span")
		}
		if len(unique) > 0 {
			previous := unique[len(unique)-1]
			if comment.start < previous.end {
				if comment == previous {
					continue
				}
				return errors.New("overlapping comment source spans")
			}
		}
		unique = append(unique, comment)
	}
	d.comments = unique
	return nil
}

func (d *Document) indexScalarContexts(node *yaml.Node, flow bool) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode || node.Kind == yaml.AliasNode {
		d.scalarFlow[node] = flow
	}
	childFlow := node.Style&yaml.FlowStyle != 0
	for _, child := range node.Content {
		d.indexScalarContexts(child, childFlow)
	}
}

func (d *Document) indexNode(node, parent, key *yaml.Node, entry int, path Path, value *Value, depth int) error {
	if node == nil {
		return nil
	}
	if depth > maxDocumentDepth {
		return fmt.Errorf("YAML nesting exceeds maximum depth %d", maxDocumentDepth)
	}
	span := d.nodeSpan(node)
	if node.Kind == yaml.ScalarNode {
		if err := d.validateScalarSpan(node, span); err != nil {
			return fmt.Errorf("%s: %w", path.String(), err)
		}
	}
	d.initializeIndexedValue(node, span, path, value)
	d.indexValueReference(node, parent, key, entry, path, value, span)
	switch node.Kind {
	case yaml.MappingNode:
		return d.indexMappingNode(node, path, value, depth)
	case yaml.SequenceNode:
		return d.indexSequenceNode(node, path, value, depth)
	default:
		return nil
	}
}

func (d *Document) initializeIndexedValue(node *yaml.Node, span byteSpan, path Path, value *Value) {
	*value = Value{
		Path:   clonePath(path),
		Range:  d.sourceRange(span),
		Flow:   node.Style&yaml.FlowStyle != 0,
		Tag:    node.Tag,
		Anchor: node.Anchor,
	}
	if node.Kind == yaml.AliasNode {
		value.Kind = ScalarValue
		value.Alias = node.Value
		raw := ""
		if span.valid(len(d.source)) {
			raw = string(d.source[span.start:span.end])
		}
		value.Scalar = &Scalar{Kind: UnknownScalar, Value: node.Value, Raw: raw, Tag: node.Tag, Style: "alias", Range: value.Range}
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		value.Kind = MappingValue
		value.Mapping = make([]Entry, len(node.Content)/2)
	case yaml.SequenceNode:
		value.Kind = SequenceValue
		value.Sequence = make([]Value, len(node.Content))
	default:
		scalar := d.semanticScalar(node, span)
		value.Kind = ScalarValue
		if scalar.Kind == NullScalar {
			value.Kind = NullValue
		}
		value.Scalar = &scalar
	}
}

func (d *Document) indexValueReference(node, parent, key *yaml.Node, entry int, path Path, value *Value, span byteSpan) {
	ref := &nodeRef{
		path:      clonePath(path),
		node:      node,
		parent:    parent,
		key:       key,
		entry:     entry,
		valueSpan: span,
	}
	if key != nil {
		ref.keySpan = d.scalarSpan(key)
	}
	keyPath := pathKey(path)
	d.refs[keyPath] = ref
	d.values[keyPath] = value
}

func (d *Document) indexMappingNode(node *yaml.Node, path Path, value *Value, depth int) error {
	seenKeys := make(map[string]struct{}, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valueNode := node.Content[i], node.Content[i+1]
		keyName := keyNode.Value
		childPath := appendPath(path, Key(keyName))
		keySpan := d.scalarSpan(keyNode)
		keyScalar := d.semanticScalar(keyNode, keySpan)
		identity := scalarKeyIdentity(keyScalar)
		if _, exists := seenKeys[identity]; exists {
			return fmt.Errorf("%s: duplicate mapping key %q", childPath.String(), keyName)
		}
		seenKeys[identity] = struct{}{}
		childValue := &value.Mapping[i/2]
		*childValue = Entry{
			Key: keyScalar, Path: clonePath(childPath),
			Range: d.sourceRange(byteSpan{start: keySpan.start, end: d.nodeSpan(valueNode).end}),
		}
		d.keyPaths[keyName] = append(d.keyPaths[keyName], clonePath(childPath))
		if err := d.indexNode(valueNode, node, keyNode, i/2, childPath, &childValue.Value, depth+1); err != nil {
			return err
		}
		childRef := d.refs[pathKey(childPath)]
		if node.Style&yaml.FlowStyle != 0 {
			childRef.entrySpan = d.flowEntrySpan(node, i/2)
		} else {
			start, _ := d.offsetAt(keyNode.Line, 1)
			childRef.entrySpan = byteSpan{start: start, end: d.lineEnd(childRef.valueSpan.end, true)}
		}
	}
	return nil
}

func (d *Document) indexSequenceNode(node *yaml.Node, path Path, value *Value, depth int) error {
	for i, child := range node.Content {
		if err := d.indexNode(child, node, nil, i, appendPath(path, Index(i)), &value.Sequence[i], depth+1); err != nil {
			return err
		}
	}
	return nil
}

// scalarKeyIdentity follows the equality behavior of PyYAML's Python mapping:
// booleans, integers, and floats share one numeric key space (true == 1 ==
// 1.0), while strings remain distinct. This prevents declarations that
// Ansible would silently collapse from entering the semantic model.
func scalarKeyIdentity(scalar Scalar) string {
	switch scalar.Kind {
	case BoolScalar:
		if value, _ := scalar.Value.(bool); value {
			return "number\x001"
		}
		return "number\x000"
	case IntegerScalar:
		return "number\x00" + fmt.Sprint(scalar.Value)
	case FloatScalar:
		value, ok := scalar.Value.(float64)
		switch {
		case !ok:
			return "float\x00" + fmt.Sprint(scalar.Value)
		case math.IsNaN(value):
			return "float\x00nan"
		case math.IsInf(value, 1):
			return "float\x00+inf"
		case math.IsInf(value, -1):
			return "float\x00-inf"
		default:
			return "number\x00" + new(big.Rat).SetFloat64(value).RatString()
		}
	case StringScalar:
		return "string\x00" + fmt.Sprint(scalar.Value)
	case NullScalar:
		return "null"
	default:
		return string(scalar.Kind) + "\x00" + scalar.Tag + "\x00" + scalar.Raw
	}
}

func clonePath(path Path) Path {
	return slices.Clone(path)
}

func appendPath(path Path, segment Segment) Path {
	return append(clonePath(path), segment)
}

func (d *Document) nodeSpan(node *yaml.Node) byteSpan {
	if node == nil {
		return byteSpan{}
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		start, _ := d.offsetAt(node.Line, node.Column)
		return byteSpan{start: start, end: d.scanPlainEnd(start, d.scalarFlow[node])}
	}
	if node.Kind == yaml.ScalarNode {
		return d.scalarSpan(node)
	}
	start, ok := d.offsetAt(node.Line, node.Column)
	if !ok {
		start = len(d.source)
	}
	if node.Style&yaml.FlowStyle != 0 {
		if end, ok := d.matchFlowEnd(start); ok {
			return byteSpan{start: start, end: end}
		}
	}
	end := start
	for _, child := range node.Content {
		span := d.nodeSpan(child)
		if span.end > end {
			end = span.end
		}
	}
	return byteSpan{start: start, end: end}
}

func (d *Document) scalarSpan(node *yaml.Node) byteSpan {
	start, ok := d.offsetAt(node.Line, node.Column)
	if !ok {
		return byteSpan{}
	}
	if node.Value == "" && node.Style == 0 && (node.Tag == "!!null" || node.Tag == "") {
		return byteSpan{start: start, end: start}
	}
	if node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return byteSpan{start: start, end: d.blockScalarEnd(node, start)}
	}
	return byteSpan{start: start, end: d.scanScalarEnd(start, d.scalarFlow[node])}
}

func (d *Document) scanScalarEnd(start int, flow bool) int {
	if start >= len(d.source) {
		return start
	}
	quote := d.source[start]
	if quote != '\'' && quote != '"' {
		return d.scanPlainEnd(start, flow)
	}
	escaped := false
	for i := start + 1; i < len(d.source); i++ {
		if quote == '"' && (escaped || d.source[i] == '\\') {
			escaped = !escaped
			continue
		}
		if d.source[i] == quote {
			if quote == '\'' && i+1 < len(d.source) && d.source[i+1] == quote {
				i++
				continue
			}
			return i + 1
		}
	}
	return d.scanPlainEnd(start, flow)
}

func (d *Document) scanPlainEnd(start int, flow bool) int {
	end := start
	for end < len(d.source) {
		b := d.source[end]
		if b == '\n' || b == '\r' || (flow && (b == ',' || b == ']' || b == '}')) {
			break
		}
		if b == '#' && (end == start || d.source[end-1] == ' ' || d.source[end-1] == '\t') {
			break
		}
		if b == ':' && end+1 < len(d.source) && isYAMLSeparator(d.source[end+1]) {
			break
		}
		end++
	}
	for end > start && (d.source[end-1] == ' ' || d.source[end-1] == '\t') {
		end--
	}
	return end
}

func (d *Document) validateScalarSpan(node *yaml.Node, span byteSpan) error {
	if node == nil || node.Kind != yaml.ScalarNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return nil
	}
	if !span.valid(len(d.source)) {
		return errors.New("scalar source span is invalid")
	}
	raw := d.source[span.start:span.end]
	if node.Value == "" && len(raw) == 0 {
		return nil
	}
	if node.Style == 0 {
		if bytes.ContainsAny(raw, "\r\n") || string(raw) != node.Value {
			return errors.New("plain scalar source span does not match its semantic value")
		}
		return nil
	}
	if node.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		var decoded string
		if err := yaml.Unmarshal(raw, &decoded); err != nil || decoded != node.Value {
			return errors.New("styled scalar source span does not match its semantic value")
		}
	}
	return nil
}

func isYAMLSeparator(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == ',' || b == ']' || b == '}' || b == '{' || b == '['
}

func (d *Document) blockScalarEnd(node *yaml.Node, start int) int {
	headerLine := node.Line
	headerStart := d.lineStarts[max(headerLine-1, 0)]
	headerEnd := d.lineEnd(headerStart, false)
	keyIndent := 0
	for headerStart+keyIndent < headerEnd && d.source[headerStart+keyIndent] == ' ' {
		keyIndent++
	}
	last := d.lineEnd(start, true)
	for line := headerLine + 1; line <= len(d.lineStarts); line++ {
		lineStart := d.lineStarts[line-1]
		lineEnd := d.lineEnd(lineStart, false)
		trimmed := bytes.TrimSpace(d.source[lineStart:lineEnd])
		if len(trimmed) == 0 {
			last = d.lineEnd(lineStart, true)
			continue
		}
		indent := 0
		for lineStart+indent < lineEnd && d.source[lineStart+indent] == ' ' {
			indent++
		}
		if indent <= keyIndent {
			break
		}
		last = d.lineEnd(lineStart, true)
	}
	return last
}

func (d *Document) lineEnd(start int, includeNewline bool) int {
	if start < 0 {
		start = 0
	}
	if start > len(d.source) {
		return len(d.source)
	}
	if idx := bytes.IndexByte(d.source[start:], '\n'); idx >= 0 {
		end := start + idx
		if includeNewline {
			end++
		}
		return end
	}
	return len(d.source)
}

func (d *Document) matchFlowEnd(start int) (int, bool) {
	if start < 0 || start >= len(d.source) || (d.source[start] != '{' && d.source[start] != '[') {
		return 0, false
	}
	open := d.source[start]
	close := byte('}')
	if open == '[' {
		close = ']'
	}
	depth := 0
	var quote byte
	escaped := false
	for i := start; i < len(d.source); i++ {
		b := d.source[i]
		if quote != 0 {
			consumeFlowQuotedByte(d.source, &i, &quote, &escaped)
			continue
		}
		if b == '\'' || b == '"' {
			quote = b
			continue
		}
		if b == open {
			depth++
			continue
		}
		if b == close {
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func consumeFlowQuotedByte(source []byte, index *int, quote *byte, escaped *bool) {
	b := source[*index]
	if *quote == '"' && *escaped {
		*escaped = false
		return
	}
	if *quote == '"' && b == '\\' {
		*escaped = true
		return
	}
	if b != *quote {
		return
	}
	if *quote == '\'' && *index+1 < len(source) && source[*index+1] == '\'' {
		*index++
		return
	}
	*quote = 0
}

func (d *Document) semanticScalar(node *yaml.Node, span byteSpan) Scalar {
	raw := ""
	if span.valid(len(d.source)) {
		raw = string(d.source[span.start:span.end])
	}
	scalar := Scalar{Kind: UnknownScalar, Value: node.Value, Raw: raw, Tag: node.Tag, Style: scalarStyle(node), Range: d.sourceRange(span)}
	if implicitPlainScalar(node) {
		scalar.Kind, scalar.Value = resolveAnsiblePlainScalar(node.Value)
		return scalar
	}
	switch node.Tag {
	case "!!str":
		scalar.Kind = StringScalar
	case "!!bool", "!!int", "!!float":
		switch node.Tag {
		case "!!bool":
			scalar.Kind = BoolScalar
		case "!!int":
			scalar.Kind = IntegerScalar
		default:
			scalar.Kind = FloatScalar
		}
		var value any
		if err := node.Decode(&value); err == nil {
			scalar.Value = value
		}
	case "!!null", "":
		if node.Value == "" || strings.EqualFold(node.Value, "null") || node.Value == "~" {
			scalar.Kind = NullScalar
			scalar.Value = nil
		} else {
			scalar.Kind = StringScalar
		}
	default:
		if strings.HasPrefix(node.Tag, "!!") {
			scalar.Kind = StringScalar
		}
	}
	return scalar
}

var (
	ansiblePlainBool  = regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)
	ansiblePlainInt   = regexp.MustCompile(`^[-+]?(?:0b[0-1_]+|0[0-7_]+|(?:0|[1-9][0-9_]*)|0x[0-9a-fA-F_]+|[1-9][0-9_]*(?::[0-5]?[0-9])+)$`)
	ansiblePlainFloat = regexp.MustCompile(
		`^[-+]?(?:(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?|[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`,
	)
)

func implicitPlainScalar(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false
	}
	return node.Style&(yaml.TaggedStyle|yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) == 0
}

// resolveAnsiblePlainScalar follows the scalar resolver used by PyYAML's
// SafeLoader, which is also the semantic baseline of Ansible's legacy YAML
// inventory plugin and Pigsty's bin/inventory_load. The original lexeme remains
// in Scalar.Raw, so this affects semantics without normalizing presentation.
func resolveAnsiblePlainScalar(text string) (ScalarKind, any) {
	if text == "" || text == "~" || text == "null" || text == "Null" || text == "NULL" {
		return NullScalar, nil
	}
	if ansiblePlainBool.MatchString(text) {
		switch strings.ToLower(text) {
		case "yes", "true", "on":
			return BoolScalar, true
		default:
			return BoolScalar, false
		}
	}
	if ansiblePlainInt.MatchString(text) {
		value, ok := parseAnsibleInteger(text)
		if !ok {
			return UnknownScalar, text
		}
		return IntegerScalar, value
	}
	if ansiblePlainFloat.MatchString(text) {
		value, ok := parseAnsibleFloat(text)
		if !ok {
			return UnknownScalar, text
		}
		return FloatScalar, value
	}
	return StringScalar, text
}

// IsAmbiguousPlainString reports whether text would resolve as a non-string
// scalar under YAML 1.1 / PyYAML SafeLoader plain-scalar rules.
func IsAmbiguousPlainString(text string) bool {
	trimmed := strings.TrimSpace(text)
	switch strings.ToLower(trimmed) {
	case "y", "n":
		return true
	}
	kind, _ := resolveAnsiblePlainScalar(trimmed)
	return kind != StringScalar
}

func parseAnsibleInteger(text string) (any, bool) {
	cleaned := strings.ReplaceAll(text, "_", "")
	if strings.Contains(cleaned, ":") {
		sign := int64(1)
		if cleaned[0] == '-' || cleaned[0] == '+' {
			if cleaned[0] == '-' {
				sign = -1
			}
			cleaned = cleaned[1:]
		}
		parts := strings.Split(cleaned, ":")
		value := int64(0)
		for _, part := range parts {
			digit, err := strconv.ParseInt(part, 10, 64)
			if err != nil || value > (math.MaxInt64-digit)/60 {
				return nil, false
			}
			value = value*60 + digit
		}
		return sign * value, true
	}
	if value, err := strconv.ParseInt(cleaned, 0, 64); err == nil {
		return value, true
	}
	if !strings.HasPrefix(cleaned, "-") {
		unsigned := strings.TrimPrefix(cleaned, "+")
		if value, err := strconv.ParseUint(unsigned, 0, 64); err == nil {
			return value, true
		}
	}
	return nil, false
}

func parseAnsibleFloat(text string) (float64, bool) {
	cleaned := strings.ReplaceAll(text, "_", "")
	lower := strings.ToLower(cleaned)
	switch lower {
	case ".inf", "+.inf":
		return math.Inf(1), true
	case "-.inf":
		return math.Inf(-1), true
	case ".nan", "+.nan", "-.nan":
		return math.NaN(), true
	}
	if strings.Contains(cleaned, ":") {
		sign := 1.0
		if cleaned[0] == '-' || cleaned[0] == '+' {
			if cleaned[0] == '-' {
				sign = -1
			}
			cleaned = cleaned[1:]
		}
		parts := strings.Split(cleaned, ":")
		value := 0.0
		for _, part := range parts {
			digit, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return 0, false
			}
			value = value*60 + digit
		}
		return sign * value, true
	}
	value, err := strconv.ParseFloat(cleaned, 64)
	return value, err == nil
}

func scalarStyle(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	switch {
	case node.Style&yaml.DoubleQuotedStyle != 0:
		return "double-quoted"
	case node.Style&yaml.SingleQuotedStyle != 0:
		return "single-quoted"
	case node.Style&yaml.LiteralStyle != 0:
		return "literal"
	case node.Style&yaml.FoldedStyle != 0:
		return "folded"
	case node.Style&yaml.TaggedStyle != 0:
		return "tagged"
	default:
		return "plain"
	}
}

func (d *Document) flowEntrySpan(mapping *yaml.Node, entry int) byteSpan {
	count := len(mapping.Content) / 2
	keyNode := mapping.Content[entry*2]
	valueNode := mapping.Content[entry*2+1]
	keyStart := d.scalarSpan(keyNode).start
	valueEnd := d.nodeSpan(valueNode).end
	if entry+1 < count {
		nextStart := d.scalarSpan(mapping.Content[(entry+1)*2]).start
		return byteSpan{start: keyStart, end: nextStart}
	}
	if entry > 0 {
		prevEnd := d.nodeSpan(mapping.Content[(entry-1)*2+1]).end
		if comma := bytes.IndexByte(d.source[prevEnd:keyStart], ','); comma >= 0 {
			return byteSpan{start: prevEnd + comma, end: valueEnd}
		}
	}
	return byteSpan{start: keyStart, end: valueEnd}
}

func (d *Document) buildInventory() *Inventory {
	root, ok := d.Lookup(nil)
	if !ok || root.Kind != MappingValue {
		return &Inventory{}
	}
	inv := &Inventory{}
	var flatten func(*Group)
	flatten = func(group *Group) {
		if group == nil {
			return
		}
		for _, child := range group.Children {
			inv.Groups = append(inv.Groups, child)
			flatten(child)
		}
	}
	for i := range root.Mapping {
		entry := &root.Mapping[i]
		name := declarationKey(entry)
		if name == "all" {
			inv.All = d.buildGroup("all", entry.Path, &entry.Value)
			flatten(inv.All)
			continue
		}
		group := d.buildGroup(name, entry.Path, &entry.Value)
		inv.Groups = append(inv.Groups, group)
		flatten(group)
	}
	return inv
}

func declarationKey(entry *Entry) string {
	if entry != nil {
		if segment, ok := entry.Path.Last(); ok && segment.Kind == KeySegment {
			return segment.Key
		}
		return fmt.Sprint(entry.Key.Value)
	}
	return ""
}

func (d *Document) buildGroup(name string, path Path, source *Value) *Group {
	group := &Group{Name: name, Path: clonePath(path)}
	if source == nil {
		return group
	}
	group.Range = source.Range
	if source.Kind != MappingValue {
		return group
	}
	if vars, ok := source.LookupKey("vars"); ok {
		group.Vars = vars
	}
	if hosts, ok := source.LookupKey("hosts"); ok && hosts.Kind == MappingValue {
		for i := range hosts.Mapping {
			entry := &hosts.Mapping[i]
			group.Hosts = append(group.Hosts, &Host{
				Name: declarationKey(entry), Vars: entry.Value,
				Path: clonePath(entry.Path), Range: entry.Range,
			})
		}
	}
	if children, ok := source.LookupKey("children"); ok && children.Kind == MappingValue {
		for i := range children.Mapping {
			entry := &children.Mapping[i]
			childName := declarationKey(entry)
			group.Children = append(group.Children, d.buildGroup(childName, entry.Path, &entry.Value))
		}
	}
	return group
}
