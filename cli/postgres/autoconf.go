/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Reusable postgresql.auto.conf read/write utilities.
Can be used by pig pg tune and future pig config commands.
*/
package postgres

import (
	"fmt"
	"pig/internal/utils"
	"os"
	"sort"
	"strings"
)

// ReadAutoConf reads postgresql.auto.conf and returns parsed parameters and raw lines.
// Parameters are returned as a lowercase-key map (without surrounding quotes).
// Lines preserve the original file order including comments and blanks.
func ReadAutoConf(path, dbsu string) (params map[string]string, lines []string, err error) {
	content, err := utils.ReadFileAsDBSU(path, dbsu)
	if err != nil {
		return nil, nil, err
	}

	params = make(map[string]string)
	lines = strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "'")
		params[key] = val
	}
	return params, lines, nil
}

// WriteAutoConf merges parameters into postgresql.auto.conf:
//   - Replaces existing parameter lines in-place
//   - Appends new parameters (sorted for deterministic output)
//   - Preserves comments and unrelated parameter lines
//   - Replaces any old "# pig pg tune:" header with the new one
//
// If the file does not exist yet, it is created from scratch.
func WriteAutoConf(path, dbsu string, params map[string]string, headerComment string) error {
	if len(params) == 0 {
		return nil
	}

	// Read existing content. If file is missing, initialize from scratch.
	// If existing file exists but read fails, abort instead of clobbering it.
	content, readErr := utils.ReadFileAsDBSU(path, dbsu)
	existingLines := []string{}
	if readErr != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			if os.IsNotExist(statErr) {
				existingLines = []string{}
			} else {
				return fmt.Errorf("cannot access existing file %s: %w", path, statErr)
			}
		} else {
			return fmt.Errorf("cannot read existing file %s: %w", path, readErr)
		}
	} else {
		existingLines = strings.Split(content, "\n")
	}

	replaced := make(map[string]bool)
	var outputLines []string

	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)

		// Remove old pig tune header
		if strings.HasPrefix(trimmed, "# pig pg tune:") {
			continue
		}

		// Replace matching parameter lines
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				if newVal, ok := params[key]; ok {
					outputLines = append(outputLines, fmt.Sprintf("%s = '%s'", key, newVal))
					replaced[key] = true
					continue
				}
			}
		}

		outputLines = append(outputLines, line)
	}

	// Append new parameters in sorted order
	var newKeys []string
	for k := range params {
		if !replaced[k] {
			newKeys = append(newKeys, k)
		}
	}
	sort.Strings(newKeys)
	for _, k := range newKeys {
		outputLines = append(outputLines, fmt.Sprintf("%s = '%s'", k, params[k]))
	}

	// Prepend header
	var finalLines []string
	if headerComment != "" {
		finalLines = append(finalLines, fmt.Sprintf("# %s", headerComment))
	}
	finalLines = append(finalLines, outputLines...)

	result := strings.Join(finalLines, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return utils.WriteFileAsDBSU(path, result, dbsu)
}
