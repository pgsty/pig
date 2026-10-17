package postgres

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"pig/internal/output"
)

func TestUnknownRuntimePlansAndStatus(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	for _, state := range []string{"invalid-pid", "identity-failure", "permission-denied"} {
		t.Run(state, func(t *testing.T) {
			cfg := runtimeFixture(t, dbsu, os.Getpid(), "")
			if state == "invalid-pid" {
				if err := os.WriteFile(filepath.Join(cfg.PgData, "postmaster.pid"), []byte("invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				previous := postgresDBSUCommandOutput
				t.Cleanup(func() { postgresDBSUCommandOutput = previous })
				postgresDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
					if args[0] == "ps" {
						if state == "permission-denied" {
							return "", syscall.EPERM
						}
						return "", errors.New("identity unavailable")
					}
					return previous(dbsu, args)
				}
			}
			status := StatusResult(cfg)
			want := output.CodePgStatusProbeFailed
			if state == "permission-denied" {
				want = output.CodePgStatusPermissionDenied
			}
			if status.Success || status.Code != want {
				t.Errorf("status misclassified %s: %+v", state, status)
			}
			for name, plan := range map[string]*output.Plan{
				"restart": BuildRestartPlan(cfg, nil),
				"stop":    BuildStopPlan(cfg, nil),
				"promote": BuildPromotePlan(cfg, nil),
			} {
				found := false
				for _, check := range plan.Preconditions {
					if check.Name == "postgres running" {
						found = true
						if check.Status != "unknown" || strings.Contains(check.Detail, "pid=0") {
							t.Errorf("%s: unknown probe passed precondition: %+v", name, check)
						}
					}
				}
				if !found {
					t.Errorf("%s omitted runtime precondition", name)
				}
				if name != "promote" && !strings.Contains(plan.Expected, "unknown") {
					t.Errorf("%s claims known state: %s", name, plan.Expected)
				}
				for _, action := range plan.NextActions {
					if strings.HasPrefix(action.Command, "pig pg ") && action.Required {
						t.Errorf("%s recommends execution with unknown state: %+v", name, action)
					}
				}
			}
		})
	}
}
