package postgres

import "testing"

func TestInspectPostgresProcesses(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want postgresProcessEvidence
	}{
		{"none", "unrelated process\n", postgresProcessEvidence{}},
		{"primary", "postgres -D /pg/data\npostgres: checkpointer\n", postgresProcessEvidence{alive: true}},
		{"replica", "postgres: walreceiver streaming 0/1\n", postgresProcessEvidence{alive: false, recovery: true, walreceiver: true}},
		{"recovering", "postgres: startup recovering 0/1\n", postgresProcessEvidence{recovery: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inspectPostgresProcesses(tt.raw); got != tt.want {
				t.Fatalf("inspectPostgresProcesses() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRoleFromDataFiles(t *testing.T) {
	role, alive := roleFromDataFiles("postgres", "/pg/data", []string{"postmaster.pid", "standby.signal"}, false)
	if role != RoleReplica || !alive {
		t.Fatalf("standby files yielded role=%s alive=%v, want replica/true", role, alive)
	}
	role, alive = roleFromDataFiles("postgres", "/pg/data", []string{"PG_VERSION", "postgresql.conf"}, false)
	if role != RolePrimary || alive {
		t.Fatalf("primary files yielded role=%s alive=%v, want primary/false", role, alive)
	}
}
