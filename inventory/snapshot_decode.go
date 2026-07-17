package inventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"unicode/utf8"
)

// ParseSnapshot decodes a stored Snapshot payload, rejects non-v1 shapes, and
// re-canonicalizes it before calculating the semantic digest.
func ParseSnapshot(source []byte) (*Snapshot, error) {
	if len(source) == 0 {
		return nil, errors.New("invalid Snapshot payload: empty input")
	}
	if !utf8.Valid(source) {
		return nil, errors.New("invalid Snapshot payload: input is not UTF-8")
	}
	if len(source) > maxSnapshotOutputBytes {
		return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	fields, err := decodeSnapshotObject(source, "Snapshot")
	if err != nil {
		return nil, err
	}
	if len(fields) != 2 {
		return nil, errors.New("invalid Snapshot payload: top-level fields")
	}
	kindRaw, hasKind := fields["kind"]
	rootRaw, hasRoot := fields["root"]
	if !hasKind || !hasRoot {
		return nil, errors.New("invalid Snapshot payload: kind and root are required")
	}
	var kind string
	if err := decodeSnapshotJSON(kindRaw, &kind); err != nil || kind != SnapshotKind {
		return nil, errors.New("invalid Snapshot payload: unsupported kind")
	}
	decoder := snapshotValueDecoder{}
	root, err := decoder.value(rootRaw, nil, 0)
	if err != nil {
		return nil, err
	}
	if root.kind != SnapshotMap {
		return nil, snapshotFailure(nil, SnapshotReasonRootType)
	}
	if err := validateSnapshotHostDeclarations(root); err != nil {
		return nil, err
	}
	canonical, err := encodeCanonicalSnapshot(root)
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		root:      root,
		canonical: canonical,
		digest:    sha256Sum(canonical),
		nodeCount: decoder.nodes,
	}, nil
}

type snapshotValueDecoder struct {
	nodes int
}

func (d *snapshotValueDecoder) value(source json.RawMessage, path Path, depth int) (SnapshotValue, error) {
	if depth > maxDocumentDepth {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonDepth)
	}
	d.nodes++
	if d.nodes > maxSnapshotNodes {
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonNodeLimit)
	}
	fields, err := decodeSnapshotObject(source, "value")
	if err != nil {
		return SnapshotValue{}, err
	}
	typeRaw, exists := fields["t"]
	if !exists {
		return SnapshotValue{}, errors.New("invalid Snapshot payload: value type is required")
	}
	var kind SnapshotValueKind
	if err := decodeSnapshotJSON(typeRaw, &kind); err != nil {
		return SnapshotValue{}, errors.New("invalid Snapshot payload: value type")
	}
	valueRaw, hasValue := fields["v"]
	if kind == SnapshotNull {
		if hasValue || len(fields) != 1 {
			return SnapshotValue{}, errors.New("invalid Snapshot payload: null shape")
		}
		return SnapshotValue{kind: SnapshotNull}, nil
	}
	if !hasValue || len(fields) != 2 {
		return SnapshotValue{}, errors.New("invalid Snapshot payload: typed value shape")
	}

	switch kind {
	case SnapshotMap:
		var rawEntries []json.RawMessage
		if err := decodeSnapshotJSON(valueRaw, &rawEntries); err != nil {
			return SnapshotValue{}, errors.New("invalid Snapshot payload: mapping entries")
		}
		result := SnapshotValue{kind: SnapshotMap, entries: make([]SnapshotEntry, 0, len(rawEntries))}
		seen := make(map[string]struct{}, len(rawEntries))
		for i := range rawEntries {
			entryFields, err := decodeSnapshotObject(rawEntries[i], "mapping entry")
			if err != nil {
				return SnapshotValue{}, err
			}
			if len(entryFields) != 2 {
				return SnapshotValue{}, errors.New("invalid Snapshot payload: mapping entry shape")
			}
			keyRaw, hasKey := entryFields["k"]
			childRaw, hasChild := entryFields["v"]
			if !hasKey || !hasChild {
				return SnapshotValue{}, errors.New("invalid Snapshot payload: mapping entry fields")
			}
			var key string
			if err := decodeSnapshotJSON(keyRaw, &key); err != nil {
				return SnapshotValue{}, snapshotFailure(path, SnapshotReasonMappingKeyType)
			}
			childPath := path.ChildKey(key)
			if _, exists := seen[key]; exists {
				return SnapshotValue{}, snapshotFailure(childPath, SnapshotReasonDuplicateKey)
			}
			seen[key] = struct{}{}
			child, err := d.value(childRaw, childPath, depth+1)
			if err != nil {
				return SnapshotValue{}, err
			}
			result.entries = append(result.entries, SnapshotEntry{key: key, value: child})
		}
		return result, nil

	case SnapshotSeq:
		var rawItems []json.RawMessage
		if err := decodeSnapshotJSON(valueRaw, &rawItems); err != nil {
			return SnapshotValue{}, errors.New("invalid Snapshot payload: sequence values")
		}
		result := SnapshotValue{kind: SnapshotSeq, items: make([]SnapshotValue, len(rawItems))}
		for i := range rawItems {
			child, err := d.value(rawItems[i], path.ChildIndex(i), depth+1)
			if err != nil {
				return SnapshotValue{}, err
			}
			result.items[i] = child
		}
		return result, nil

	case SnapshotStr:
		var value string
		if err := decodeSnapshotJSON(valueRaw, &value); err != nil {
			return SnapshotValue{}, errors.New("invalid Snapshot payload: string value")
		}
		return SnapshotValue{kind: SnapshotStr, text: value}, nil
	case SnapshotBool:
		var value bool
		if err := decodeSnapshotJSON(valueRaw, &value); err != nil {
			return SnapshotValue{}, errors.New("invalid Snapshot payload: boolean value")
		}
		return SnapshotValue{kind: SnapshotBool, boolean: value}, nil
	case SnapshotInt:
		var value string
		if err := decodeSnapshotJSON(valueRaw, &value); err != nil || !validCanonicalSnapshotInteger(value) {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonIntegerRange)
		}
		return SnapshotValue{kind: SnapshotInt, text: value}, nil
	case SnapshotFloat:
		var value string
		if err := decodeSnapshotJSON(valueRaw, &value); err != nil || !validCanonicalSnapshotFloat(value) {
			return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
		}
		return SnapshotValue{kind: SnapshotFloat, text: value}, nil
	default:
		return SnapshotValue{}, snapshotFailure(path, SnapshotReasonUnknownScalar)
	}
}

func decodeSnapshotObject(source []byte, label string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("invalid Snapshot payload: %s object", label)
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid Snapshot payload: %s field", label)
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid Snapshot payload: %s field name", label)
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, fmt.Errorf("invalid Snapshot payload: duplicate %s field", label)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("invalid Snapshot payload: %s field value", label)
		}
		fields[name] = raw
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, fmt.Errorf("invalid Snapshot payload: %s terminator", label)
	}
	if err := snapshotDecoderEOF(decoder); err != nil {
		return nil, fmt.Errorf("invalid Snapshot payload: trailing %s data", label)
	}
	return fields, nil
}

func decodeSnapshotJSON(source []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return snapshotDecoderEOF(decoder)
}

func snapshotDecoderEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}

func validCanonicalSnapshotInteger(value string) bool {
	if signed, err := strconv.ParseInt(value, 10, 64); err == nil && strconv.FormatInt(signed, 10) == value {
		return true
	}
	if unsigned, err := strconv.ParseUint(value, 10, 64); err == nil && strconv.FormatUint(unsigned, 10) == value {
		return true
	}
	return false
}

func validCanonicalSnapshotFloat(value string) bool {
	parsed, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) &&
		strconv.FormatFloat(parsed, 'g', -1, 64) == value
}

func sha256Sum(source []byte) [32]byte {
	// Kept behind a small helper so the decoder and builder cannot accidentally
	// diverge in how canonical bytes become a semantic revision.
	return sha256.Sum256(source)
}
