/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

SQL safety utilities for PostgreSQL identifier validation and string escaping.
*/
package utils

import (
	"regexp"
	"strings"
)

// IdentifierRegex validates PostgreSQL identifiers (usernames, database names, schema names, table names).
// Allows alphanumeric, underscore, and dollar sign (PostgreSQL naming rules).
// First character must be letter or underscore.
var IdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]*$`)

// SQLLikePatternRegex validates LIKE pattern input.
// Allows alphanumeric, spaces, and common punctuation characters.
var SQLLikePatternRegex = regexp.MustCompile(`^[a-zA-Z0-9_\s%*.,;:!?@#$^&()\[\]{}<>+=/-]+$`)

// ValidStateValues contains valid PostgreSQL connection states for pg_stat_activity.
var ValidStateValues = map[string]bool{
	"active":                          true,
	"idle":                            true,
	"idle in transaction":             true,
	"idle in transaction (aborted)":   true,
	"fastpath function call":          true,
	"disabled":                        true,
}

// ValidateIdentifier checks if a string is a valid PostgreSQL identifier.
// Returns true for empty string (means no filter) or valid identifier.
func ValidateIdentifier(s string) bool {
	if s == "" {
		return true // empty is allowed (means no filter)
	}
	return IdentifierRegex.MatchString(s)
}

// ValidateSQLLikePattern checks if a string is a valid LIKE pattern input.
// Returns true for empty string or valid pattern.
func ValidateSQLLikePattern(s string) bool {
	if s == "" {
		return true
	}
	return SQLLikePatternRegex.MatchString(s)
}

// ValidateConnectionState checks if a state is a valid PostgreSQL connection state.
func ValidateConnectionState(s string) bool {
	if s == "" {
		return true
	}
	return ValidStateValues[strings.ToLower(s)]
}

// EscapeSQLString escapes single quotes in a string for SQL.
// Use this when embedding user input in SQL string literals.
// Example: O'Brien -> O''Brien
func EscapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// EscapeSQLLikePattern escapes special characters in a LIKE pattern.
// This handles: backslash (escape char), % and _ (LIKE wildcards), and single quotes (SQL string).
// Use with ESCAPE '\\' clause in LIKE expressions.
func EscapeSQLLikePattern(s string) string {
	// Order matters: escape backslash first since it's the escape character
	s = strings.ReplaceAll(s, "\\", "\\\\")
	// Escape LIKE wildcards
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	// Escape single quotes for SQL string
	s = strings.ReplaceAll(s, "'", "''")
	return s
}
