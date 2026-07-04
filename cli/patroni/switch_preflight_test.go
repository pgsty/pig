package patroni

import "testing"

func TestBuildSwitchPreflightDerivesLeaderCandidatesAndPause(t *testing.T) {
	paused := true
	state := BuildSwitchPreflight(
		&PtListResultData{
			Cluster: "pg-test",
			Members: []PtMemberSummary{
				{Member: "pg-test-1", Role: "leader", State: "running"},
				{Member: "pg-test-2", Role: "replica", State: "streaming"},
				{Member: "pg-test-3", Role: "sync_standby", State: "streaming"},
				{Member: "pg-test-4", Role: "replica", State: "stopped"},
			},
		},
		&PtConfigResultData{
			Raw: map[string]interface{}{
				"pause": paused,
			},
		},
	)

	if state == nil {
		t.Fatal("BuildSwitchPreflight returned nil")
	}
	if state.Cluster != "pg-test" {
		t.Fatalf("cluster = %q, want pg-test", state.Cluster)
	}
	if state.Leader != "pg-test-1" {
		t.Fatalf("leader = %q, want pg-test-1", state.Leader)
	}
	if !state.Paused {
		t.Fatal("paused config should set state.Paused")
	}
	wantCandidates := []string{"pg-test-2", "pg-test-3"}
	if len(state.Candidates) != len(wantCandidates) {
		t.Fatalf("candidates = %v, want %v", state.Candidates, wantCandidates)
	}
	for i, want := range wantCandidates {
		if state.Candidates[i] != want {
			t.Fatalf("candidates = %v, want %v", state.Candidates, wantCandidates)
		}
	}
}
