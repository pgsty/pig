package postgres

import (
	"fmt"
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

func TestRunMaintAllDatabasesReturnsPartialFailureAfterAllAttempts(t *testing.T) {
	origList := maintGetAllDatabases
	origRun := maintRunPsqlMaintenance
	defer func() {
		maintGetAllDatabases = origList
		maintRunPsqlMaintenance = origRun
	}()

	maintGetAllDatabases = func(*Config) ([]string, error) {
		return []string{"app", "broken", "report"}, nil
	}
	var calls []string
	maintRunPsqlMaintenance = func(_ *Config, dbname, sql string) error {
		calls = append(calls, dbname+":"+sql)
		if dbname == "broken" {
			return fmt.Errorf("permission denied")
		}
		return nil
	}

	err := runMaintAllDatabases(nil, &maintTask{
		command:  "VACUUM",
		options:  "(FULL)",
		taskName: "Vacuuming",
	})
	if err == nil {
		t.Fatal("runMaintAllDatabases should report partial failure")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "1/3") {
		t.Fatalf("partial failure error should include failing database and count, got %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("maintenance should continue after a database failure, calls=%v", calls)
	}
	for _, call := range calls {
		if !strings.Contains(call, "VACUUM (FULL)") {
			t.Fatalf("maintenance call should use full SQL, got calls=%v", calls)
		}
	}
}
