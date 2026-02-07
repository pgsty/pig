package postgres

import "testing"

func TestBuildKillWhereClause_Default(t *testing.T) {
	got := buildKillWhereClause(nil)
	want := "pid <> pg_backend_pid() AND backend_type = 'client backend'"
	if got != want {
		t.Fatalf("buildKillWhereClause(nil)=%q, want %q", got, want)
	}
}

func TestBuildKillSQL_DefaultDryRun(t *testing.T) {
	got := buildKillSQL("pg_terminate_backend", nil)
	want := "SELECT pid, usename, datname, client_addr, state, LEFT(query, 40) AS query FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND backend_type = 'client backend'"
	if got != want {
		t.Fatalf("buildKillSQL(dry-run)=%q, want %q", got, want)
	}
}

func TestBuildKillSQL_Execute(t *testing.T) {
	opts := &KillOptions{Execute: true, User: "alice", Db: "postgres"}
	got := buildKillSQL("pg_terminate_backend", opts)
	want := "SELECT pg_terminate_backend(pid), pid, usename, datname, state FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND backend_type = 'client backend' AND usename = 'alice' AND datname = 'postgres'"
	if got != want {
		t.Fatalf("buildKillSQL(execute)=%q, want %q", got, want)
	}
}

func TestBuildKillSQL_Pid(t *testing.T) {
	opts := &KillOptions{Pid: 1234}
	got := buildKillSQL("pg_cancel_backend", opts)
	want := "SELECT pg_cancel_backend(1234)"
	if got != want {
		t.Fatalf("buildKillSQL(pid)=%q, want %q", got, want)
	}
}

func TestBuildKillWhereClause_QueryEscaping(t *testing.T) {
	opts := &KillOptions{Query: "foo%bar_baz"}
	got := buildKillWhereClause(opts)
	want := "pid <> pg_backend_pid() AND backend_type = 'client backend' AND query ILIKE '%foo\\%bar\\_baz%' ESCAPE '\\\\'"
	if got != want {
		t.Fatalf("buildKillWhereClause(query)=%q, want %q", got, want)
	}
}
