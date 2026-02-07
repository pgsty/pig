/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pt config show structured output result and DTO.
*/
package patroni

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"

	"gopkg.in/yaml.v3"
)

// PtConfigResultData contains Patroni cluster dynamic configuration in an agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
// Known top-level keys are typed fields; all keys are preserved in Raw for arbitrary access.
type PtConfigResultData struct {
	LoopWait             *int                   `json:"loop_wait,omitempty" yaml:"loop_wait,omitempty"`
	TTL                  *int                   `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	RetryTimeout         *int                   `json:"retry_timeout,omitempty" yaml:"retry_timeout,omitempty"`
	MaximumLagOnFailover *int                   `json:"maximum_lag_on_failover,omitempty" yaml:"maximum_lag_on_failover,omitempty"`
	MaximumLagOnSyncnode *int                   `json:"maximum_lag_on_syncnode,omitempty" yaml:"maximum_lag_on_syncnode,omitempty"`
	PostgreSQL           map[string]interface{} `json:"postgresql,omitempty" yaml:"postgresql,omitempty"`
	Standby              map[string]interface{} `json:"standby_cluster,omitempty" yaml:"standby_cluster,omitempty"`
	Slots                map[string]interface{} `json:"slots,omitempty" yaml:"slots,omitempty"`
	IgnoreSlots          []interface{}          `json:"ignore_slots,omitempty" yaml:"ignore_slots,omitempty"`
	Raw                  map[string]interface{} `json:"raw,omitempty" yaml:"raw,omitempty"`
}

// Text returns a human-readable YAML representation of the config data.
func (d *PtConfigResultData) Text() string {
	if d == nil {
		return ""
	}
	if len(d.Raw) == 0 {
		return ""
	}
	b, err := yaml.Marshal(d.Raw)
	if err != nil {
		return fmt.Sprintf("error rendering config: %v", err)
	}
	return string(b)
}

// ConfigShowResult creates a structured result for pt config show command.
// It executes patronictl show-config and returns parsed DCS configuration.
func ConfigShowResult(dbsu string) *output.Result {
	// 1. Check patronictl existence
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return output.Fail(output.CodePtNotFound, "patronictl not found in PATH")
	}

	// 2. Check config file existence
	if _, err := os.Stat(DefaultConfigPath); os.IsNotExist(err) {
		return output.Fail(output.CodePtConfigNotFound,
			fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath))
	}

	// 3. Execute show-config
	args := []string{binPath, "-c", DefaultConfigPath, "show-config"}
	yamlOutput, err := utils.DBSUCommandOutput(dbsu, args)
	if err != nil {
		if isPermissionDenied(err, yamlOutput) {
			return output.Fail(output.CodePtPermDenied,
				"Permission denied executing patronictl show-config").WithDetail(commandErrorDetail(yamlOutput, err))
		}
		return output.Fail(output.CodePtConfigShowFailed,
			"Failed to execute patronictl show-config").WithDetail(commandErrorDetail(yamlOutput, err))
	}

	// 4. Parse YAML output
	data, err := parseShowConfigOutput(yamlOutput)
	if err != nil {
		return output.Fail(output.CodePtParseFailed,
			"Failed to parse show-config output").WithDetail(err.Error())
	}

	return output.OK("Patroni cluster config retrieved", data)
}

// parseShowConfigOutput parses the YAML output from patronictl show-config.
// It extracts known top-level keys into typed fields and preserves all keys in Raw.
func parseShowConfigOutput(yamlStr string) (*PtConfigResultData, error) {
	if strings.TrimSpace(yamlStr) == "" {
		return nil, fmt.Errorf("empty show-config output")
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	data := &PtConfigResultData{Raw: raw}

	// Extract known integer fields
	if v, ok := raw["loop_wait"]; ok {
		if n, ok := toInt(v); ok {
			data.LoopWait = &n
		}
	}
	if v, ok := raw["ttl"]; ok {
		if n, ok := toInt(v); ok {
			data.TTL = &n
		}
	}
	if v, ok := raw["retry_timeout"]; ok {
		if n, ok := toInt(v); ok {
			data.RetryTimeout = &n
		}
	}
	if v, ok := raw["maximum_lag_on_failover"]; ok {
		if n, ok := toInt(v); ok {
			data.MaximumLagOnFailover = &n
		}
	}
	if v, ok := raw["maximum_lag_on_syncnode"]; ok {
		if n, ok := toInt(v); ok {
			data.MaximumLagOnSyncnode = &n
		}
	}

	// Extract map-type fields
	if v, ok := raw["postgresql"].(map[string]interface{}); ok {
		data.PostgreSQL = v
	}
	if v, ok := raw["standby_cluster"].(map[string]interface{}); ok {
		data.Standby = v
	}
	if v, ok := raw["slots"].(map[string]interface{}); ok {
		data.Slots = v
	}

	// Extract ignore_slots (array type)
	if v, ok := raw["ignore_slots"].([]interface{}); ok {
		data.IgnoreSlots = v
	}

	return data, nil
}

// toInt converts an interface{} value to int.
// Handles int, float64, and int64 types from YAML/JSON parsing.
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}
