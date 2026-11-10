package output

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// YAML serializes the Result to YAML format.
// Empty fields (Detail, Data) are omitted via struct tags.
// Returns an error if the receiver is nil.
func (r *Result) YAML() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot render nil Result")
	}
	return yaml.Marshal(r)
}
