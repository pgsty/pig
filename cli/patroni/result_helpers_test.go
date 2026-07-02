package patroni

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"pig/internal/output"
)

// patroniTestDepsMu serializes package-level hook replacement in tests.
// Do not call t.Parallel in tests that mutate these hooks; t.Cleanup releases
// the lock only when the owning test or subtest completes.
var patroniTestDepsMu sync.Mutex

func TestClusterNameErrorResultCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "permission", err: newClusterConfigReadError(errors.New("permission denied")), code: output.CodePtPermDenied},
		{name: "config not found", err: newClusterConfigReadError(errors.New("patroni.yml: no such file or directory")), code: output.CodePtConfigNotFound},
		{name: "read failed", err: newClusterConfigReadError(errors.New("cannot read patroni.yml: file too large")), code: output.CodePtConfigReadFailed},
		{name: "scope missing", err: errClusterScopeMissing, code: output.CodePtScopeMissing},
		{name: "scope invalid", err: errClusterScopeInvalid, code: output.CodePtConfigResolveFailed},
		{name: "unknown", err: errors.New("context canceled"), code: output.CodePtConfigResolveFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clusterNameErrorResult(tt.err)
			if result.Code != tt.code {
				t.Fatalf("code = %d, want %d; detail=%q", result.Code, tt.code, result.Detail)
			}
		})
	}
}

func TestNeedYesResultCodes(t *testing.T) {
	tests := []struct {
		name   string
		result *output.Result
		code   int
	}{
		{name: "restart", result: RestartNeedYesResult(), code: output.CodePtConfirmationRequired},
		{name: "reinit", result: ReinitNeedYesResult(), code: output.CodePtConfirmationRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Success {
				t.Fatal("need-yes result should fail")
			}
			if tt.result.Code != tt.code {
				t.Fatalf("code = %d, want %d", tt.result.Code, tt.code)
			}
			if !strings.Contains(tt.result.Message, "--yes (-y)") {
				t.Fatalf("message should reference --yes (-y), got %q", tt.result.Message)
			}
			for _, action := range tt.result.NextActions {
				if strings.Contains(action.Command, "--force") {
					t.Fatalf("next_actions must route to --yes, not --force: %q", action.Command)
				}
			}
		})
	}
}

func stubPatroniResultDeps(t *testing.T, cluster string, clusterErr error, captured *[]string) {
	t.Helper()
	patroniTestDepsMu.Lock()

	oldLookPath := patroniLookPath
	oldStat := patroniStat
	oldGetClusterName := patroniGetClusterName
	oldDBSUCommandOutput := patroniDBSUCommandOutput

	patroniLookPath = func(file string) (string, error) {
		return "/usr/bin/patronictl", nil
	}
	patroniStat = func(name string) (os.FileInfo, error) {
		return os.Stat(".")
	}
	patroniGetClusterName = func(dbsu string) (string, error) {
		return cluster, clusterErr
	}
	patroniDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		*captured = append([]string(nil), args...)
		return "ok", nil
	}

	t.Cleanup(func() {
		patroniLookPath = oldLookPath
		patroniStat = oldStat
		patroniGetClusterName = oldGetClusterName
		patroniDBSUCommandOutput = oldDBSUCommandOutput
		patroniTestDepsMu.Unlock()
	})
}
