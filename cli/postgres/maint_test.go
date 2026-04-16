package postgres

import (
	"strings"
	"testing"
)

func TestBuildMaintSQLForTable(t *testing.T) {
	task := &maintTask{
		command: "VACUUM",
		options: "(VERBOSE)",
		schema:  "public",
		table:   "events",
	}

	got := buildMaintSQL(task)
	want := "VACUUM (VERBOSE) public.events"
	if got != want {
		t.Fatalf("buildMaintSQL() = %q, want %q", got, want)
	}
}

func TestBuildMaintSQLForSchemaEscapesLiteral(t *testing.T) {
	task := &maintTask{
		command: "ANALYZE",
		options: "(VERBOSE)",
		schema:  "o'hara",
	}

	got := buildMaintSQL(task)
	if !strings.Contains(got, "schemaname = 'o''hara'") {
		t.Fatalf("expected escaped schema literal, got %q", got)
	}
	if strings.Contains(got, "quote_literal(") {
		t.Fatalf("schema SQL should not double-quote literal, got %q", got)
	}
	if !strings.Contains(got, "EXECUTE 'ANALYZE (VERBOSE) ' || quote_ident(r.schemaname) || '.' || quote_ident(r.tablename)") {
		t.Fatalf("expected schema loop to preserve command/options, got %q", got)
	}
}

func TestBuildMaintSQLForWholeDatabase(t *testing.T) {
	task := &maintTask{
		command: "VACUUM",
		options: "(FULL)",
	}

	got := buildMaintSQL(task)
	want := "VACUUM (FULL)"
	if got != want {
		t.Fatalf("buildMaintSQL() = %q, want %q", got, want)
	}
}
