package cmdb

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestContainerLoadPlanPruneConfirmationAndRollback(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}

	first := staticLoadFixture(t, "load-first.yml", "all: { vars: { keep: 1, password: first-secret, values: [one, two] } }\n")
	firstPlan, err := store.PlanStaticLoad(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	if firstPlan.Kind != LoadKind || firstPlan.ExpectedHead != nil || firstPlan.NoOp ||
		firstPlan.RequiresPrune || firstPlan.RequiresConfirmation || len(firstPlan.Changes) == 0 {
		t.Fatalf("unexpected first load plan: %+v", firstPlan)
	}
	firstResult, err := store.ApplyLoad(t.Context(), firstPlan, LoadConsent{})
	if err != nil || !firstResult.Changed || firstResult.RevisionCount != 1 {
		t.Fatalf("first load failed: result=%+v err=%v", firstResult, err)
	}

	second := staticLoadFixture(t, "load-second.yml", "all: { vars: { keep: 2, values: [one] } }\n")
	secondPlan, err := store.PlanStaticLoad(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	if !secondPlan.RequiresPrune || !secondPlan.RequiresConfirmation || len(secondPlan.Removals) != 2 || secondPlan.ExpectedHead == nil ||
		!secondPlan.ExpectedHead.Equal(firstResult.Token) {
		t.Fatalf("destructive load plan is incomplete: %+v", secondPlan)
	}
	forged := secondPlan
	forged.RequiresPrune = false
	forged.RequiresConfirmation = false
	forged.Removals = nil
	if _, err := store.ApplyLoad(t.Context(), forged, LoadConsent{}); !IsCode(err, "CMDB_LOAD_PLAN_INVALID") {
		t.Fatalf("modified load plan was accepted: %v", err)
	}
	encoded, err := json.Marshal(secondPlan)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range [][]byte{[]byte("first-secret"), []byte(first.SourceLocator), second.Snapshot.CanonicalBytes()} {
		if bytes.Contains(encoded, private) {
			t.Fatalf("load plan leaked private Inventory data: %s", encoded)
		}
	}
	if _, err := store.ApplyLoad(t.Context(), secondPlan, LoadConsent{}); !IsCode(err, "CMDB_PRUNE_REQUIRED") {
		t.Fatalf("load without prune was accepted: %v", err)
	}
	if _, err := store.ApplyLoad(t.Context(), secondPlan, LoadConsent{Prune: true}); !IsCode(err, "CMDB_CONFIRMATION_REQUIRED") {
		t.Fatalf("load without confirmation was accepted: %v", err)
	}
	unchanged, err := store.Verify(t.Context())
	if err != nil || unchanged.Head == nil || !unchanged.Head.Equal(firstResult.Token) || unchanged.RevisionCount != 1 {
		t.Fatalf("refused load changed native state: %+v err=%v", unchanged, err)
	}
	secondResult, err := store.ApplyLoad(t.Context(), secondPlan, LoadConsent{Prune: true, Confirmed: true})
	if err != nil || !secondResult.Changed || secondResult.RevisionCount != 2 {
		t.Fatalf("confirmed load failed: result=%+v err=%v", secondResult, err)
	}

	noOpPlan, err := store.PlanStaticLoad(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	if !noOpPlan.NoOp || len(noOpPlan.Changes) != 0 || noOpPlan.RequiresConfirmation {
		t.Fatalf("unchanged static candidate is not a no-op: %+v", noOpPlan)
	}
	noOp, err := store.ApplyLoad(t.Context(), noOpPlan, LoadConsent{})
	if err != nil || noOp.Changed || !noOp.Token.Equal(secondResult.Token) || noOp.RevisionCount != 2 {
		t.Fatalf("static no-op allocated history: result=%+v err=%v", noOp, err)
	}

	rollbackPlan, err := store.PlanRollback(t.Context(), firstResult.Token, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	if rollbackPlan.SourceKind != SourceRevision || rollbackPlan.Target == nil || !rollbackPlan.Target.Equal(firstResult.Token) ||
		rollbackPlan.RequiresPrune || !rollbackPlan.RequiresConfirmation {
		t.Fatalf("rollback plan is incomplete: %+v", rollbackPlan)
	}
	if _, err := store.ApplyLoad(t.Context(), rollbackPlan, LoadConsent{}); !IsCode(err, "CMDB_CONFIRMATION_REQUIRED") {
		t.Fatalf("unconfirmed rollback was accepted: %v", err)
	}
	rollbackResult, err := store.ApplyLoad(t.Context(), rollbackPlan, LoadConsent{Confirmed: true})
	if err != nil || !rollbackResult.Changed || rollbackResult.RevisionCount != 3 {
		t.Fatalf("confirmed rollback failed: result=%+v err=%v", rollbackResult, err)
	}
	rollbackRevision, err := store.ReadRevision(t.Context(), rollbackResult.Token)
	if err != nil || rollbackRevision.Origin() != OriginRollback || rollbackRevision.SourceKind() != SourceRevision {
		t.Fatalf("rollback did not create an immutable child: revision=%+v err=%v", rollbackRevision, err)
	}

	destructiveRollbackPlan, err := store.PlanRollback(t.Context(), secondResult.Token, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !destructiveRollbackPlan.RequiresPrune || !destructiveRollbackPlan.RequiresConfirmation {
		t.Fatalf("destructive rollback plan is incomplete: %+v", destructiveRollbackPlan)
	}
	if _, err := store.ApplyLoad(t.Context(), destructiveRollbackPlan, LoadConsent{Confirmed: true}); !IsCode(err, "CMDB_PRUNE_REQUIRED") {
		t.Fatalf("rollback removal without prune was accepted: %v", err)
	}
	destructiveRollback, err := store.ApplyLoad(t.Context(), destructiveRollbackPlan, LoadConsent{Prune: true, Confirmed: true})
	if err != nil || !destructiveRollback.Changed || destructiveRollback.RevisionCount != 4 {
		t.Fatalf("destructive rollback failed: result=%+v err=%v", destructiveRollback, err)
	}

	rollbackNoOpPlan, err := store.PlanRollback(t.Context(), secondResult.Token, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !rollbackNoOpPlan.NoOp || rollbackNoOpPlan.RequiresPrune || !rollbackNoOpPlan.RequiresConfirmation {
		t.Fatalf("rollback no-op consent contract changed: %+v", rollbackNoOpPlan)
	}
	if _, err := store.ApplyLoad(t.Context(), rollbackNoOpPlan, LoadConsent{}); !IsCode(err, "CMDB_CONFIRMATION_REQUIRED") {
		t.Fatalf("historical no-op skipped confirmation: %v", err)
	}
	rollbackNoOp, err := store.ApplyLoad(t.Context(), rollbackNoOpPlan, LoadConsent{Confirmed: true})
	if err != nil || rollbackNoOp.Changed || !rollbackNoOp.Token.Equal(destructiveRollback.Token) {
		t.Fatalf("confirmed historical no-op changed history: result=%+v err=%v", rollbackNoOp, err)
	}
}

func TestContainerLoadPlanExactCASRejectsStaleWriter(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	base := staticLoadFixture(t, "load-base.yml", "all: { vars: { generation: 1 } }\n")
	basePlan, err := store.PlanStaticLoad(t.Context(), base)
	if err != nil {
		t.Fatal(err)
	}
	baseResult, err := store.ApplyLoad(t.Context(), basePlan, LoadConsent{})
	if err != nil {
		t.Fatal(err)
	}

	left := staticLoadFixture(t, "load-left.yml", "all: { vars: { generation: 2 } }\n")
	right := staticLoadFixture(t, "load-right.yml", "all: { vars: { generation: 3 } }\n")
	leftPlan, err := store.PlanStaticLoad(t.Context(), left)
	if err != nil {
		t.Fatal(err)
	}
	rightPlan, err := store.PlanStaticLoad(t.Context(), right)
	if err != nil {
		t.Fatal(err)
	}
	leftResult, err := store.ApplyLoad(t.Context(), leftPlan, LoadConsent{})
	if err != nil || !leftResult.Changed {
		t.Fatalf("first exact plan failed: result=%+v err=%v", leftResult, err)
	}
	if _, err := store.ApplyLoad(t.Context(), rightPlan, LoadConsent{}); !IsCode(err, "CMDB_REVISION_CONFLICT") {
		t.Fatalf("stale exact plan was accepted: %v", err)
	}
	state, err := store.Verify(t.Context())
	if err != nil || state.Head == nil || !state.Head.Equal(leftResult.Token) || state.RevisionCount != baseResult.RevisionCount+1 {
		t.Fatalf("stale load changed native state: %+v err=%v", state, err)
	}
}
