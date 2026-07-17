package cmdb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"

	"pig/inventory"
)

const LoadKind = "pig.sty.cmdb.load/v1"

type LoadPlan struct {
	Kind                 string                     `json:"kind" yaml:"kind"`
	SourceKind           RevisionSourceKind         `json:"source_kind" yaml:"source_kind"`
	ExpectedHead         *RevisionToken             `json:"expected_head,omitempty" yaml:"expected_head,omitempty"`
	Target               *RevisionToken             `json:"target,omitempty" yaml:"target,omitempty"`
	CandidateSHA256      string                     `json:"candidate_sha256" yaml:"candidate_sha256"`
	RevisionCount        int64                      `json:"revision_count" yaml:"revision_count"`
	Changes              []inventory.SnapshotChange `json:"changes,omitempty" yaml:"changes,omitempty"`
	Removals             []inventory.SnapshotChange `json:"removals,omitempty" yaml:"removals,omitempty"`
	NoOp                 bool                       `json:"no_op" yaml:"no_op"`
	RequiresPrune        bool                       `json:"requires_prune" yaml:"requires_prune"`
	RequiresConfirmation bool                       `json:"requires_confirmation" yaml:"requires_confirmation"`

	store  *Store
	static StaticLoad
	target RevisionToken
	seal   [sha256.Size]byte
}

type LoadConsent struct {
	Prune     bool
	Confirmed bool
}

// PlanStaticLoad fully verifies the current native history and produces a
// value-free plan bound to this Store, exact head, candidate, and provenance.
func (s *Store) PlanStaticLoad(ctx context.Context, candidate StaticLoad) (LoadPlan, error) {
	if err := validateStaticLoad(candidate); err != nil {
		return LoadPlan{}, err
	}
	chain, err := s.readVerifiedChain(ctx, true)
	if err != nil {
		return LoadPlan{}, err
	}
	plan, err := newLoadPlan(s, chain, candidate.Snapshot)
	if err != nil {
		return LoadPlan{}, err
	}
	plan.SourceKind = SourceStatic
	plan.static = candidate
	plan.RequiresConfirmation = plan.RequiresPrune
	if err := sealLoadPlan(&plan); err != nil {
		return LoadPlan{}, err
	}
	return plan, nil
}

// PlanRollback verifies the exact historical token and produces a plan for a
// new immutable child. Historical rollback always requires confirmation.
func (s *Store) PlanRollback(ctx context.Context, target RevisionToken, toolVersion string) (LoadPlan, error) {
	if target.String() == "" {
		return LoadPlan{}, invalidRevisionToken()
	}
	if err := validateToolVersion(toolVersion); err != nil {
		return LoadPlan{}, err
	}
	chain, err := s.readVerifiedChain(ctx, true)
	if err != nil {
		return LoadPlan{}, err
	}
	if target.revision > int64(len(chain.revisions)) ||
		!chain.revisions[target.revision-1].token.Equal(target) {
		return LoadPlan{}, failure("CMDB_REVISION_NOT_FOUND", "exact rollback target was not found")
	}
	plan, err := newLoadPlan(s, chain, chain.revisions[target.revision-1].snapshot)
	if err != nil {
		return LoadPlan{}, err
	}
	copy := target
	plan.SourceKind = SourceRevision
	plan.Target = &copy
	plan.target = target
	plan.static.ToolVersion = toolVersion
	plan.RequiresConfirmation = true
	if err := sealLoadPlan(&plan); err != nil {
		return LoadPlan{}, err
	}
	return plan, nil
}

func newLoadPlan(store *Store, chain *verifiedChain, candidate *inventory.Snapshot) (LoadPlan, error) {
	base := emptySnapshot
	noOp := false
	if chain.head != nil {
		base = chain.revisions[len(chain.revisions)-1].snapshot
		noOp = base.Equal(candidate)
	}
	changes, err := inventory.DiffSnapshots(base, candidate)
	if err != nil {
		return LoadPlan{}, failure("CMDB_SNAPSHOT_INVALID", "candidate Snapshot diff could not be computed")
	}
	removals, err := inventory.SnapshotRemovals(base, candidate)
	if err != nil {
		return LoadPlan{}, failure("CMDB_SNAPSHOT_INVALID", "candidate Snapshot prune scope could not be computed")
	}
	plan := LoadPlan{
		Kind: LoadKind, CandidateSHA256: candidate.Digest(), RevisionCount: int64(len(chain.revisions)),
		Changes: changes, Removals: removals, NoOp: noOp, RequiresPrune: len(removals) > 0,
		store: store,
	}
	if chain.head != nil {
		copy := *chain.head
		plan.ExpectedHead = &copy
	}
	return plan, nil
}

// ApplyLoad enforces prune selection and confirmation before delegating to the
// serializable exact-CAS append primitive. A plan from another Store or a zero
// plan is never accepted.
func (s *Store) ApplyLoad(ctx context.Context, plan LoadPlan, consent LoadConsent) (AppendResult, error) {
	if s == nil || s.pool == nil {
		return AppendResult{}, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	if plan.store != s || plan.Kind != LoadKind || plan.CandidateSHA256 == "" || !validLoadPlanSeal(plan) {
		return AppendResult{}, failure("CMDB_LOAD_PLAN_INVALID", "load requires an exact native plan")
	}
	if plan.RequiresPrune && !consent.Prune {
		return AppendResult{}, failure("CMDB_PRUNE_REQUIRED", "load removals require explicit prune scope")
	}
	if plan.RequiresConfirmation && !consent.Confirmed {
		return AppendResult{}, failure("CMDB_CONFIRMATION_REQUIRED", "load requires explicit confirmation")
	}
	switch plan.SourceKind {
	case SourceStatic:
		return s.AppendStatic(ctx, plan.ExpectedHead, plan.static)
	case SourceRevision:
		if plan.ExpectedHead == nil || plan.target.String() == "" {
			return AppendResult{}, failure("CMDB_LOAD_PLAN_INVALID", "rollback plan is incomplete")
		}
		return s.AppendRollback(ctx, *plan.ExpectedHead, plan.target, plan.static.ToolVersion)
	default:
		return AppendResult{}, failure("CMDB_LOAD_PLAN_INVALID", "load source kind is invalid")
	}
}

func sealLoadPlan(plan *LoadPlan) error {
	if plan == nil {
		return failure("CMDB_LOAD_PLAN_INVALID", "load plan could not be bound")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return failure("CMDB_LOAD_PLAN_INVALID", "load plan could not be bound")
	}
	plan.seal = sha256.Sum256(encoded)
	return nil
}

func validLoadPlanSeal(plan LoadPlan) bool {
	expected := plan.seal
	plan.seal = [sha256.Size]byte{}
	encoded, err := json.Marshal(plan)
	return err == nil && expected != [sha256.Size]byte{} && expected == sha256.Sum256(encoded)
}

func validateStaticLoad(candidate StaticLoad) error {
	if err := validateToolVersion(candidate.ToolVersion); err != nil {
		return err
	}
	if candidate.Snapshot == nil || !validCanonicalStaticLocator(candidate.SourceLocator) {
		return failure("CMDB_PARAMETER_INVALID", "static Snapshot provenance is invalid")
	}
	parsed, err := inventory.ParseSnapshot(candidate.Snapshot.CanonicalBytes())
	if err != nil || !parsed.Equal(candidate.Snapshot) ||
		!slices.Equal(parsed.CanonicalBytes(), candidate.Snapshot.CanonicalBytes()) {
		return failure("CMDB_SNAPSHOT_INVALID", "static Snapshot is not canonical CMDB v1 data")
	}
	return nil
}

var emptySnapshot = func() *inventory.Snapshot {
	snapshot, err := inventory.ParseSnapshot([]byte(`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[]}}`))
	if err != nil {
		panic(err)
	}
	return snapshot
}()
