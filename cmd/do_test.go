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
