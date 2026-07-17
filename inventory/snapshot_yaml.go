package inventory

import "strings"

// CanonicalYAML traverses the ordered Snapshot directly and emits the
// generated v1 body. It never marshals a Document or a generic YAML tree.
// The output is reparsed and required to reproduce the exact semantic digest.
func (s *Snapshot) CanonicalYAML() ([]byte, error) {
	if s == nil || s.root.kind != SnapshotMap {
		return nil, snapshotFailure(nil, SnapshotReasonRootType)
	}
	writer := newSnapshotByteWriter(maxSnapshotOutputBytes)
	if err := writeCanonicalYAMLBlock(writer, s.root, 0); err != nil {
		return nil, err
	}
	output := writer.bytes()
	if len(output) == 0 || output[len(output)-1] != '\n' {
		return nil, snapshotFailure(nil, SnapshotReasonRoundTripMismatch)
	}
	doc, err := Parse("CMDB canonical YAML", output)
	if err != nil {
		return nil, snapshotFailure(nil, SnapshotReasonRoundTripMismatch)
	}
	roundTrip, err := snapshotFromDocument(doc, false)
	if err != nil || !s.Equal(roundTrip) {
		return nil, snapshotFailure(nil, SnapshotReasonRoundTripMismatch)
	}
	return output, nil
}

func writeCanonicalYAMLBlock(writer *snapshotByteWriter, value SnapshotValue, indent int) error {
	switch value.kind {
	case SnapshotMap:
		if len(value.entries) == 0 {
			if err := writer.literal("{}\n"); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			return nil
		}
		for i := range value.entries {
			entry := value.entries[i]
			if err := writeCanonicalYAMLIndent(writer, indent); err != nil {
				return err
			}
			if err := writer.jsonString(entry.key); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			if err := writer.byte(':'); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			if snapshotYAMLInline(entry.value) {
				if err := writer.byte(' '); err != nil {
					return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
				}
				if err := writeCanonicalYAMLInline(writer, entry.value); err != nil {
					return err
				}
				if err := writer.byte('\n'); err != nil {
					return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
				}
				continue
			}
			if err := writer.byte('\n'); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			if err := writeCanonicalYAMLBlock(writer, entry.value, indent+2); err != nil {
				return err
			}
		}
		return nil

	case SnapshotSeq:
		if len(value.items) == 0 {
			if err := writer.literal("[]\n"); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			return nil
		}
		for i := range value.items {
			if err := writeCanonicalYAMLIndent(writer, indent); err != nil {
				return err
			}
			if err := writer.byte('-'); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			if snapshotYAMLInline(value.items[i]) {
				if err := writer.byte(' '); err != nil {
					return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
				}
				if err := writeCanonicalYAMLInline(writer, value.items[i]); err != nil {
					return err
				}
				if err := writer.byte('\n'); err != nil {
					return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
				}
				continue
			}
			if err := writer.byte('\n'); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
			if err := writeCanonicalYAMLBlock(writer, value.items[i], indent+2); err != nil {
				return err
			}
		}
		return nil
	default:
		return snapshotFailure(nil, SnapshotReasonRoundTripMismatch)
	}
}

func snapshotYAMLInline(value SnapshotValue) bool {
	return value.kind != SnapshotMap && value.kind != SnapshotSeq ||
		value.kind == SnapshotMap && len(value.entries) == 0 ||
		value.kind == SnapshotSeq && len(value.items) == 0
}

func writeCanonicalYAMLInline(writer *snapshotByteWriter, value SnapshotValue) error {
	var err error
	switch value.kind {
	case SnapshotMap:
		err = writer.literal("{}")
	case SnapshotSeq:
		err = writer.literal("[]")
	case SnapshotStr:
		err = writer.jsonString(value.text)
	case SnapshotBool:
		if value.boolean {
			err = writer.literal("true")
		} else {
			err = writer.literal("false")
		}
	case SnapshotInt:
		err = writer.literal(value.text)
	case SnapshotFloat:
		err = writer.literal(snapshotFloatLiteral(value.text))
	case SnapshotNull:
		err = writer.literal("null")
	default:
		return snapshotFailure(nil, SnapshotReasonRoundTripMismatch)
	}
	if err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return nil
}

func writeCanonicalYAMLIndent(writer *snapshotByteWriter, indent int) error {
	if err := writer.literal(strings.Repeat(" ", indent)); err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return nil
}
