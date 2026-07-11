package do

import (
	"reflect"
	"testing"
)

func TestBuildPgsqlAddCommandsAppendsReplicaAndRefreshesService(t *testing.T) {
	commands, err := BuildPgsqlAddCommands([]string{"pg-test", "10.10.10.12", "10.10.10.13"})
	if err != nil {
		t.Fatalf("BuildPgsqlAddCommands() error = %v", err)
	}

	want := [][]string{
		{"pgsql.yml", "-l", "10.10.10.13,10.10.10.12,&pg-test"},
		{"pgsql.yml", "-l", "pg-test,!10.10.10.12,!10.10.10.13", "-t", "pg_service"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildPgsqlAddCommands() = %#v, want %#v", commands, want)
	}
}

func TestBuildPgsqlRmCommandsRemovesReplicaInsteadOfCluster(t *testing.T) {
	commands, err := BuildPgsqlRmCommands([]string{"pg-test", "10.10.10.13"}, true)
	if err != nil {
		t.Fatalf("BuildPgsqlRmCommands() error = %v", err)
	}

	want := [][]string{
		{"pgsql-rm.yml", "-l", "10.10.10.13,&pg-test", "-e", "pg_rm_pkg=true"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildPgsqlRmCommands() = %#v, want %#v", commands, want)
	}
}

func TestBuildNodeCommandsAcceptMultipleSelectors(t *testing.T) {
	addCommands, err := BuildNodeAddCommands([]string{"10.10.10.10", "10.10.10.11"})
	if err != nil {
		t.Fatalf("BuildNodeAddCommands() error = %v", err)
	}
	if want := [][]string{{"node.yml", "-l", "10.10.10.10,10.10.10.11"}}; !reflect.DeepEqual(addCommands, want) {
		t.Fatalf("BuildNodeAddCommands() = %#v, want %#v", addCommands, want)
	}

	rmCommands, err := BuildNodeRmCommands([]string{"pg-test", "10.10.10.11"})
	if err != nil {
		t.Fatalf("BuildNodeRmCommands() error = %v", err)
	}
	if want := [][]string{{"node-rm.yml", "-l", "pg-test,10.10.10.11"}}; !reflect.DeepEqual(rmCommands, want) {
		t.Fatalf("BuildNodeRmCommands() = %#v, want %#v", rmCommands, want)
	}
}

func TestBuildNodeRepoCommandsRejectsTooManyArgs(t *testing.T) {
	if _, err := BuildNodeRepoCommands([]string{"pg-test", "node", "extra"}); err == nil {
		t.Fatal("BuildNodeRepoCommands() should reject more than two arguments")
	}
}

func TestBuildRedisCommandsValidatePortsAndUninstall(t *testing.T) {
	if _, err := BuildRedisAddCommands([]string{"redis-test", "6379"}); err == nil {
		t.Fatal("BuildRedisAddCommands() should require an IP selector when ports are specified")
	}
	if _, err := BuildRedisAddCommands([]string{"10.10.10.10", "80"}); err == nil {
		t.Fatal("BuildRedisAddCommands() should reject invalid redis port")
	}

	commands, err := BuildRedisRmCommands([]string{"10.10.10.10"}, true)
	if err != nil {
		t.Fatalf("BuildRedisRmCommands() error = %v", err)
	}
	want := [][]string{{"redis-rm.yml", "-l", "10.10.10.10", "-e", "redis_rm_pkg=true"}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildRedisRmCommands() = %#v, want %#v", commands, want)
	}

	if _, err := BuildRedisRmCommands([]string{"10.10.10.10", "6379"}, true); err == nil {
		t.Fatal("BuildRedisRmCommands() should reject --uninstall for per-port removal")
	}
}

func TestSimplePlaybookBuilders(t *testing.T) {
	tests := []struct {
		name  string
		build PlaybookBuilder
		args  []string
		want  [][]string
	}{
		{"pgsql-user", BuildPgsqlUserCommands, []string{"pg-meta", "dbuser_meta", "--check"}, [][]string{{"pgsql-user.yml", "-l", "pg-meta", "-e", "username=dbuser_meta", "--check"}}},
		{"pgsql-db", BuildPgsqlDBCommands, []string{"pg-meta", "meta"}, [][]string{{"pgsql-db.yml", "-l", "pg-meta", "-e", "dbname=meta"}}},
		{"pgsql-ext", BuildPgsqlExtCommands, []string{"pg-meta", "postgis", "vector"}, [][]string{{"pgsql.yml", "-l", "pg-meta", "-t", "pg_extension", "-e", `{"pg_extensions":["postgis,vector"]}`}}},
		{"pgsql-hba", BuildPgsqlHBACommands, []string{"pg-meta"}, [][]string{{"pgsql.yml", "-l", "pg-meta", "-t", "pg_hba,pg_reload,pgbouncer_hba,pgbouncer_reload"}}},
		{"pgsql-svc", BuildPgsqlServiceCommands, []string{"pg-meta"}, [][]string{{"pgsql.yml", "-l", "pg-meta", "-t", "pg_service"}}},
		{"pgmon-add", BuildPgmonAddCommands, []string{"pg-meta"}, [][]string{{"pgsql-monitor.yml", "-e", "clsname=pg-meta"}}},
		{"node-pkg", BuildNodePackageCommands, []string{"all", "duckdb", "restic"}, [][]string{{"node.yml", "-l", "all", "-t", "node_pkg_extra", "-e", `{"node_packages":["duckdb,restic"]}`}}},
		{"repo-build", BuildRepoBuildCommands, nil, [][]string{{"infra.yml", "-l", "infra", "-t", "repo_build"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build(tt.args)
			if err != nil {
				t.Fatalf("builder returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("builder result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSingleSelectorBuildersRejectExtraArgs(t *testing.T) {
	for name, build := range map[string]PlaybookBuilder{
		"pgsql-hba": BuildPgsqlHBACommands,
		"pgsql-svc": BuildPgsqlServiceCommands,
		"pgmon-add": BuildPgmonAddCommands,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := build([]string{"pg-meta", "ignored"}); err == nil {
				t.Fatal("builder should reject an ignored positional argument")
			}
		})
	}
	if err := RemovePgmon("", []string{"pg-meta", "ignored"}); err == nil {
		t.Fatal("RemovePgmon should reject an ignored positional argument")
	}
}
