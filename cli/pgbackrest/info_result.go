/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pb info structured output result and DTO.
*/
package pgbackrest

import (
	"encoding/json"
	"strings"

	"pig/internal/output"
)

// InfoResult creates a structured result for pb info command.
// It collects pgBackRest backup information and returns it in a Result structure.
// Returns nil-safe Result on all paths.
func InfoResult(cfg *Config, opts *InfoOptions) *output.Result {
	// Get effective config (validates config file exists, auto-detects stanza)
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return pbConfigErrorResult(err, output.CodePbInfoFailed, "Failed to get pgBackRest configuration")
	}

	// Build arguments for pgbackrest info --output=json
	// Suppress console logs to keep JSON output clean.
	args := []string{"--output=json", "--log-level-console=error"}
	if opts != nil && opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}

	// Execute pgbackrest info and capture JSON output
	jsonOutput, err := RunPgBackRestOutput(effCfg, "info", args)
	if err != nil {
		errMsg := combineCommandError(jsonOutput, err)
		return output.Fail(output.CodePbInfoFailed, "Failed to execute pgbackrest info").
			WithDetail(errMsg)
	}

	// Parse JSON output
	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(jsonOutput), &infos); err != nil {
		return output.Fail(output.CodePbInfoFailed, "Failed to parse pgbackrest info output").
			WithDetail(err.Error())
	}

	// Handle empty result (no stanzas)
	if len(infos) == 0 {
		return output.Fail(output.CodePbStanzaNotFound, "No stanza information found").
			WithDetail("pgbackrest info returned empty result")
	}

	// Structured output should embed pgBackRest native JSON (wrapped by Result),
	// so agents can consume the upstream schema directly.
	data := output.NewEmbeddedJSON([]byte(jsonOutput))

	// Preserve existing semantics: if a single stanza reports non-zero status,
	// treat it as a failure (but still include the upstream info payload).
	if len(infos) == 1 && infos[0].Status.Code != 0 {
		code := output.CodePbInfoFailed
		if isStanzaNotFoundMessage(infos[0].Status.Message) {
			code = output.CodePbStanzaNotFound
		}
		return output.Fail(code, infos[0].Status.Message).
			WithData(data)
	}

	return output.OK("pgBackRest backup info retrieved", data)
}

// containsAny checks if s contains any of the substrings. This is reserved
// for classifying native pgbackrest output; pig's own configuration errors
// are classified via sentinel errors (pbConfigErrorResult).
func containsAny(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// combineCommandError merges command output and error message for better diagnostics.
func combineCommandError(output string, err error) string {
	outMsg := strings.TrimSpace(output)
	if err == nil {
		return outMsg
	}
	errMsg := strings.TrimSpace(err.Error())
	if outMsg == "" {
		return errMsg
	}
	if errMsg == "" {
		return outMsg
	}
	if strings.Contains(errMsg, outMsg) {
		return errMsg
	}
	return outMsg + "\n" + errMsg
}

// isStanzaNotFoundMessage checks if a status message indicates stanza is missing.
func isStanzaNotFoundMessage(message string) bool {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "stanza") &&
		(strings.Contains(lower, "not found") || strings.Contains(lower, "missing") || strings.Contains(lower, "does not exist")) {
		return true
	}
	return false
}
