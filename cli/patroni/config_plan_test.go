package patroni

import (
	"strings"
	"testing"
)

func TestBuildSetPlanShowsSelectedConfigAndExactNativeCommand(t *testing.T) {
	preservePatroniCommandOptions(t)
	SetCommandOptions(CommandOptions{ConfigFile: "/tmp/patroni config.yml"})
	if err := ResolveCommandOptions("postgres"); err != nil {
		t.Fatal(err)
	}
	pairs, err := ParseSetPairs([]string{"ttl=60", "work_mem=a=b", "empty="})
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildSetPlan(pairs)
	for _, want := range []string{
		"patronictl -c '/tmp/patroni config.yml' edit-config --set ttl=60 --pg work_mem=a=b --pg empty=",
		"caller-supplied configuration values",
	} {
		if !strings.Contains(plan.Text(), want) {
			t.Fatalf("plan does not contain %q:\n%s", want, plan.Text())
		}
	}
	if strings.Contains(plan.Command, "--force") {
		t.Fatalf("plan command unexpectedly contains --force: %s", plan.Command)
	}
}
