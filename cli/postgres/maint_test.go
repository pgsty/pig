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
			t.Fatalf("maintenance call should use full SQL, got calls=%v", call)
		}
	}
}

func TestVacuumAndFreezeRejectSchemaWithoutTable(t *testing.T) {
	origRun := maintRunPsqlMaintenance
	defer func() {
		maintRunPsqlMaintenance = origRun
	}()

	var calls []string
	maintRunPsqlMaintenance = func(_ *Config, dbname, sql string) error {
		calls = append(calls, dbname+":"+sql)
		return nil
	}

	err := Vacuum(nil, "app", &VacuumOptions{
		MaintOptions: MaintOptions{Schema: "public"},
	})
	if err == nil || !strings.Contains(err.Error(), "--schema requires --table") {
		t.Fatalf("Vacuum(--schema without --table) error = %v, want --schema requires --table", err)
	}

	err = Freeze(nil, "app", &FreezeOptions{Schema: "public"})
	if err == nil || !strings.Contains(err.Error(), "--schema requires --table") {
		t.Fatalf("Freeze(--schema without --table) error = %v, want --schema requires --table", err)
	}
	if len(calls) != 0 {
		t.Fatalf("Vacuum/Freeze with invalid target should not invoke psql, calls=%v", calls)
	}

	if err := Vacuum(nil, "app", &VacuumOptions{
		MaintOptions: MaintOptions{Schema: "public", Table: "events"},
	}); err != nil {
		t.Fatalf("Vacuum(--schema public --table events) error = %v", err)
	}
	if len(calls) != 1 || calls[0] != "app:VACUUM public.events" {
		t.Fatalf("Vacuum(--schema public --table events) calls = %v", calls)
	}

	calls = nil
	if err := Analyze(nil, "app", &MaintOptions{Schema: "public"}); err != nil {
		t.Fatalf("Analyze(--schema public) should remain supported, got %v", err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "EXECUTE 'ANALYZE") {
		t.Fatalf("Analyze(--schema public) calls = %v", calls)
	}
}
