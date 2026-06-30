package do

import (
	"reflect"
	"testing"
)

func TestBuildPgsqlAddCommandsAppendsReplicaAndRefreshesService(t *testing.T) {
	commands, err := BuildPgsqlAddCommands("pg-test", []string{"10.10.10.12", "10.10.10.13"})
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
	commands, err := BuildPgsqlRmCommands("pg-test", []string{"10.10.10.13"}, true)
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
	addCommand, err := BuildNodeAddCommand([]string{"10.10.10.10", "10.10.10.11"})
	if err != nil {
		t.Fatalf("BuildNodeAddCommand() error = %v", err)
	}
	if want := []string{"node.yml", "-l", "10.10.10.10,10.10.10.11"}; !reflect.DeepEqual(addCommand, want) {
		t.Fatalf("BuildNodeAddCommand() = %#v, want %#v", addCommand, want)
	}

	rmCommand, err := BuildNodeRmCommand([]string{"pg-test", "10.10.10.11"})
	if err != nil {
		t.Fatalf("BuildNodeRmCommand() error = %v", err)
	}
	if want := []string{"node-rm.yml", "-l", "pg-test,10.10.10.11"}; !reflect.DeepEqual(rmCommand, want) {
		t.Fatalf("BuildNodeRmCommand() = %#v, want %#v", rmCommand, want)
	}
}

func TestBuildNodeRepoCommandRejectsTooManyArgs(t *testing.T) {
	if _, err := BuildNodeRepoCommand([]string{"pg-test", "node", "extra"}); err == nil {
		t.Fatal("BuildNodeRepoCommand() should reject more than two arguments")
	}
}

func TestBuildRedisCommandsValidatePortsAndUninstall(t *testing.T) {
	if _, err := BuildRedisAddCommands("redis-test", []string{"6379"}); err == nil {
		t.Fatal("BuildRedisAddCommands() should require an IP selector when ports are specified")
	}
	if _, err := BuildRedisAddCommands("10.10.10.10", []string{"80"}); err == nil {
		t.Fatal("BuildRedisAddCommands() should reject invalid redis port")
	}

	commands, err := BuildRedisRmCommands("10.10.10.10", nil, true)
	if err != nil {
		t.Fatalf("BuildRedisRmCommands() error = %v", err)
	}
	want := [][]string{{"redis-rm.yml", "-l", "10.10.10.10", "-e", "redis_rm_pkg=true"}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildRedisRmCommands() = %#v, want %#v", commands, want)
	}

	if _, err := BuildRedisRmCommands("10.10.10.10", []string{"6379"}, true); err == nil {
		t.Fatal("BuildRedisRmCommands() should reject --uninstall for per-port removal")
	}
}
