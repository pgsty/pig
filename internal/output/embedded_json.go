package output

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// EmbeddedJSON wraps a JSON value (object/array/etc.) so it can be embedded as a
// structured value in Result.Data for both JSON and YAML outputs.
//
// - JSON output: the raw JSON is emitted as-is (not as a quoted string).
// - YAML output: the raw JSON is decoded and then re-emitted as YAML.
//
// Note: JSON pretty-printing will not re-indent the embedded JSON; it is emitted
// as returned by MarshalJSON.
type EmbeddedJSON struct {
	Raw json.RawMessage
}

// NewEmbeddedJSON trims surrounding whitespace and returns an EmbeddedJSON.
// It does not validate JSON at construction time; validation happens at marshal time.
func NewEmbeddedJSON(raw []byte) EmbeddedJSON {
	return EmbeddedJSON{Raw: json.RawMessage(bytes.TrimSpace(raw))}
}

// MarshalJSON implements json.Marshaler to embed the raw JSON value.
func (e EmbeddedJSON) MarshalJSON() ([]byte, error) {
	raw := bytes.TrimSpace([]byte(e.Raw))
	if len(raw) == 0 {
		return []byte("null"), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid embedded json")
	}
	return raw, nil
}

// MarshalYAML implements yaml.Marshaler to convert the embedded JSON into
// native YAML structures (maps/slices/scalars).
func (e EmbeddedJSON) MarshalYAML() (interface{}, error) {
	raw := bytes.TrimSpace([]byte(e.Raw))
	if len(raw) == 0 {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid embedded json")
	}

	// JSON is a subset of YAML, so yaml.Unmarshal can parse it while preserving
	// null values (vs empty slices/maps) and large integers.
	var v any
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}
