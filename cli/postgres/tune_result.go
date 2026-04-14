/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Structured output (JSON/YAML) support for pig pg tune.
*/
package postgres

import (
	"fmt"
	"pig/internal/output"
	"sort"
	"strings"
)

// ============================================================================
// DTOs
// ============================================================================

// TuneResultData is the structured output DTO for pig pg tune.
type TuneResultData struct {
	Profile   string           `json:"profile" yaml:"profile"`
	PgVersion int              `json:"pg_version" yaml:"pg_version"`
	Hardware  TuneHardwareInfo `json:"hardware" yaml:"hardware"`
	Params    map[string]string `json:"parameters" yaml:"parameters"`
}

// TuneHardwareInfo holds detected hardware specs for structured output.
type TuneHardwareInfo struct {
	CPU    int `json:"cpu" yaml:"cpu"`
	MemMB  int `json:"mem_mb" yaml:"mem_mb"`
	DiskGB int `json:"disk_gb" yaml:"disk_gb"`
}

// Text renders the tuned parameters in human-readable format.
func (t *TuneResultData) Text() string {
	if t == nil {
		return ""
	}
	var sb strings.Builder
	header := fmt.Sprintf("pig pg tune: %s | %dC / %dMB / %dGB / SSD | PG %d",
		t.Profile, t.Hardware.CPU, t.Hardware.MemMB, t.Hardware.DiskGB, t.PgVersion)
	sb.WriteString(fmt.Sprintf("# %s\n", header))
	keys := make([]string, 0, len(t.Params))
	for k := range t.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%s = '%s'\n", k, t.Params[k]))
	}
	return sb.String()
}

// ============================================================================
// Result Constructor
// ============================================================================

// TuneResult is the structured output entry point for pig pg tune.
func TuneResult(cfg *Config, opts *TuneOptions) *output.Result {
	if opts == nil {
		return output.Fail(output.CodePgTuneInvalidProfile, "tune options required")
	}

	prof, ok := tuneProfiles[strings.ToLower(opts.Profile)]
	if !ok {
		return output.Fail(output.CodePgTuneInvalidProfile,
			fmt.Sprintf("unknown profile: %s (valid: oltp, olap, tiny, crit)", opts.Profile))
	}

	ratio := opts.ShmemRatio
	if ratio < 0.1 || ratio > 0.4 {
		return output.Fail(output.CodePgTuneInvalidRatio,
			fmt.Sprintf("shmem-ratio must be between 0.1 and 0.4, got %.2f", ratio))
	}

	pgVersion := resolvePgVersion(cfg)
	spec := DetectHardware(cfg, opts)
	params := CalculateTuneParams(spec, prof, opts.MaxConn, ratio, pgVersion)

	data := &TuneResultData{
		Profile:   prof.Name,
		PgVersion: pgVersion,
		Hardware:  TuneHardwareInfo(spec),
		Params:    make(map[string]string, len(params)),
	}
	for _, p := range params {
		data.Params[p.Name] = p.Value
	}

	msg := fmt.Sprintf("Generated %d parameters for %s profile (%dC/%dMB/%dGB)",
		len(params), prof.Name, spec.CPU, spec.MemMB, spec.DiskGB)
	return output.OK(msg, data)
}
