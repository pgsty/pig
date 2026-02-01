package output

import (
	"encoding/json"
	"fmt"
)

// JSON serializes the Result to compact JSON format.
// Empty fields (Detail, Data) are omitted via struct tags.
// Returns an error if the receiver is nil.
func (r *Result) JSON() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot render nil Result")
	}
	return json.Marshal(r)
}

// JSONPretty serializes the Result to indented JSON format for human readability.
// Empty fields (Detail, Data) are omitted via struct tags.
// Returns an error if the receiver is nil.
func (r *Result) JSONPretty() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot render nil Result")
	}
	return json.MarshalIndent(r, "", "  ")
}
