package cmd

import (
	"strings"
	"testing"
)

func TestDoCommandUseStringsDescribeArgs(t *testing.T) {
	tests := []struct {
		name string
		use  string
		want string
	}{
		{name: "pgsql-add", use: doPgsqlAddCmd.Use, want: "<cluster> [ip...]"},
		{name: "pgsql-rm", use: doPgsqlRmCmd.Use, want: "<cluster> [ip...]"},
		{name: "node-add", use: doNodeAddCmd.Use, want: "<sel> [sel...]"},
		{name: "node-rm", use: doNodeRmCmd.Use, want: "<sel> [sel...]"},
		{name: "node-repo", use: doNodeRepoCmd.Use, want: "[sel] [module]"},
		{name: "redis-add", use: doRedisAddCmd.Use, want: "<sel> [port...]"},
		{name: "redis-rm", use: doRedisRmCmd.Use, want: "<sel> [port...]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.use, tt.want) {
				t.Fatalf("%s Use = %q, want it to contain %q", tt.name, tt.use, tt.want)
			}
		})
	}
}

func TestDoRemoveUninstallFlagsAreIndependent(t *testing.T) {
	origPgsql, origRedis := doPgsqlRmUninstall, doRedisRmUninstall
	defer func() {
		doPgsqlRmUninstall = origPgsql
		doRedisRmUninstall = origRedis
	}()
	doPgsqlRmUninstall = false
	doRedisRmUninstall = false

	pgsqlFlag := doPgsqlRmCmd.Flags().Lookup("uninstall")
	redisFlag := doRedisRmCmd.Flags().Lookup("uninstall")
	if pgsqlFlag == nil || redisFlag == nil {
		t.Fatal("both removal commands must declare --uninstall")
	}
	if err := pgsqlFlag.Value.Set("true"); err != nil {
		t.Fatal(err)
	}
	if !doPgsqlRmUninstall || doRedisRmUninstall {
		t.Fatalf("pgsql flag leaked: pgsql=%v redis=%v", doPgsqlRmUninstall, doRedisRmUninstall)
	}

	doPgsqlRmUninstall = false
	if err := redisFlag.Value.Set("true"); err != nil {
		t.Fatal(err)
	}
	if doPgsqlRmUninstall || !doRedisRmUninstall {
		t.Fatalf("redis flag leaked: pgsql=%v redis=%v", doPgsqlRmUninstall, doRedisRmUninstall)
	}
}
