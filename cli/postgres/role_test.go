package postgres

import (
	"reflect"
	"testing"
)

func TestInspectPostgresProcessTree(t *testing.T) {
	raw := `
100 1 postgres -D /pg/data-a
101 100 postgres: checkpointer
102 100 postgres: walreceiver streaming 0/1
200 1 postgres -D /pg/data-b
201 200 postgres: checkpointer
202 201 postgres: logical replication worker
`
	tests := []struct {
		name string
		pid  int
		want postgresProcessEvidence
	}{
		{"missing", 300, postgresProcessEvidence{}},
		{"replica", 100, postgresProcessEvidence{alive: true, recovery: true, walreceiver: true}},
		{"primary", 200, postgresProcessEvidence{alive: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inspectPostgresProcessTree(raw, tt.pid); got != tt.want {
				t.Fatalf("inspectPostgresProcessTree() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRoleSQLArgsBindTargetEndpoint(t *testing.T) {
	got := roleSQLArgs("/usr/bin/psql", PostmasterPidInfo{Port: 15432, SocketDir: "/run/postgresql-alt"})
	want := []string{
		"/usr/bin/psql", "-AXtqw", "-F", "\t", "-p", "15432", "-h", "/run/postgresql-alt",
		"-d", "postgres", "-c", "SELECT current_setting('data_directory'), pg_is_in_recovery()",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roleSQLArgs() = %#v, want %#v", got, want)
	}
}

func TestParseRoleSQLResult(t *testing.T) {
	for _, tc := range []struct {
		output string
		data   string
		role   Role
		ok     bool
	}{
		{"/pg/data-a\tf\n", "/pg/data-a", RolePrimary, true},
		{"/pg/data-b\tt\n", "/pg/data-b", RoleReplica, true},
		{"f\n", "", RoleUnknown, false},
		{"/pg/data\tmaybe\n", "", RoleUnknown, false},
	} {
		data, role, ok := parseRoleSQLResult(tc.output)
		if data != tc.data || role != tc.role || ok != tc.ok {
			t.Fatalf("parseRoleSQLResult(%q) = %q, %s, %t", tc.output, data, role, ok)
		}
	}
}

func TestRoleFromDataFiles(t *testing.T) {
	role := roleFromDataFiles("postgres", "/pg/data", []string{"postmaster.pid", "standby.signal"}, false)
	if role != RoleReplica {
		t.Fatalf("standby files yielded role=%s, want replica", role)
	}
	role = roleFromDataFiles("postgres", "/pg/data", []string{"PG_VERSION", "postgresql.conf"}, false)
	if role != RolePrimary {
		t.Fatalf("primary files yielded role=%s, want primary", role)
	}
}
