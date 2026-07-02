package postgres

import (
	"strings"
	"testing"
)

func TestPatroniLifecycleRiskWarningNamesCommandAndManagedAlternative(t *testing.T) {
	for _, action := range []string{"stop", "restart", "promote"} {
		t.Run(action, func(t *testing.T) {
			msg := patroniLifecycleRiskWarning(action, "/pg/data")
			for _, want := range []string{action, "Patroni", "/pg/data", "pig pt"} {
				if !strings.Contains(msg, want) {
					t.Fatalf("warning %q should contain %q", msg, want)
				}
			}
		})
	}
}
