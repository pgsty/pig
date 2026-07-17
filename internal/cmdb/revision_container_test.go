package cmdb

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/inventory"
)

func TestContainerImmutableRevisionLifecycle(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	first := staticLoadFixture(t, "first.yml", `
all:
  children:
    db:
      hosts:
        db-a: { ansible_host: 10.0.0.1 }
`)
	second := staticLoadFixture(t, "second.yml", `
all:
  children:
    db:
      hosts:
        db-a: { ansible_host: 10.0.0.1 }
        db-b: { ansible_host: 10.0.0.2 }
`)

	created, err := store.AppendStatic(t.Context(), nil, first)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Changed || created.Token.Revision() != 1 || created.RevisionCount != 1 {
		t.Fatalf("unexpected first append: %+v", created)
	}
	expected := created.Token
	noOp, err := store.AppendStatic(t.Context(), &expected, first)
	if err != nil {
		t.Fatal(err)
	}
	if noOp.Changed || !noOp.Token.Equal(created.Token) || noOp.RevisionCount != 1 {
		t.Fatalf("unchanged candidate allocated a revision: %+v", noOp)
	}

	secondResult, err := store.AppendStatic(t.Context(), &expected, second)
	if err != nil {
		t.Fatal(err)
	}
	if !secondResult.Changed || secondResult.Token.Revision() != 2 || secondResult.RevisionCount != 2 {
		t.Fatalf("unexpected second append: %+v", secondResult)
	}
	rollback, err := store.AppendRollback(t.Context(), secondResult.Token, created.Token, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !rollback.Changed || rollback.Token.Revision() != 3 || rollback.RevisionCount != 3 {
		t.Fatalf("rollback did not append a child: %+v", rollback)
	}

	head, err := store.ReadHead(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if parent, ok := head.ParentRevision(); !ok || parent != 2 || head.MergeBaseRevision() != 2 ||
		head.Origin() != OriginRollback || head.SourceKind() != SourceRevision || !head.Snapshot().Equal(first.Snapshot) {
		t.Fatalf("rollback lineage is wrong: revision=%d parent=%d base=%d origin=%s source=%s",
			head.Revision(), parent, head.MergeBaseRevision(), head.Origin(), head.SourceKind())
	}
	locator, ok := head.SourceLocator()
	if !ok || locator != created.Token.String() {
		t.Fatalf("rollback did not retain exact target token: %q", locator)
	}
	if _, ok := head.SourceBytesSHA256(); ok {
		t.Fatal("rollback unexpectedly retained static source bytes digest")
	}
	state, err := store.Verify(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.RevisionCount != 3 || state.Head == nil || !state.Head.Equal(rollback.Token) {
		t.Fatalf("unexpected verified state: %+v", state)
	}
	if _, err := json.Marshal(head); err == nil {
		t.Fatal("opaque revision body was JSON serializable")
	}

	for _, statement := range []string{
		`UPDATE pigsty_cmdb.snapshot SET payload = payload WHERE revision = 1`,
		`DELETE FROM pigsty_cmdb.snapshot WHERE revision = 1`,
	} {
		if _, err := store.pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("append-only Snapshot accepted %s", statement)
		}
	}
}

func TestContainerExactCASAndRollbackTarget(t *testing.T) {
	firstClient := openContainerStore(t)
	resetContainerSchema(t, firstClient)
	if _, err := firstClient.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	secondClient := openContainerStore(t)
	base := staticLoadFixture(t, "base.yml", "all: { vars: { generation: 1 } }\n")
	left := staticLoadFixture(t, "left.yml", "all: { vars: { generation: 2 } }\n")
	right := staticLoadFixture(t, "right.yml", "all: { vars: { generation: 3 } }\n")

	baseResult, err := firstClient.AppendStatic(t.Context(), nil, base)
	if err != nil {
		t.Fatal(err)
	}
	planned := baseResult.Token
	leftResult, err := firstClient.AppendStatic(t.Context(), &planned, left)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondClient.AppendStatic(t.Context(), &planned, right); !IsCode(err, "CMDB_REVISION_CONFLICT") {
		t.Fatalf("stale exact CAS was accepted: %v", err)
	}
	state, err := secondClient.Verify(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.RevisionCount != 2 || state.Head == nil || !state.Head.Equal(leftResult.Token) {
		t.Fatalf("conflicting writer partially mutated history: %+v", state)
	}

	wrongText := "cmdb-v1:1:sha256:" + strings.Repeat("00", 32)
	wrongTarget, err := ParseRevisionToken(wrongText)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstClient.AppendRollback(t.Context(), leftResult.Token, wrongTarget, "test-v1"); !IsCode(err, "CMDB_REVISION_NOT_FOUND") {
		t.Fatalf("rollback accepted a number-only target: %v", err)
	}
	if _, err := firstClient.ReadRevision(t.Context(), wrongTarget); !IsCode(err, "CMDB_REVISION_NOT_FOUND") {
		t.Fatalf("historical read accepted a number-only token: %v", err)
	}
}

func TestContainerAppendTransactionRollsBackAfterInsert(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	candidate := staticLoadFixture(t, "rollback.yml", "all: { vars: { secret_value: hidden } }\n")
	_, err := store.appendRevision(t.Context(), appendRequest{
		kind:   appendStatic,
		static: candidate,
		afterInsert: func() error {
			return errors.New("test injection")
		},
	})
	if !IsCode(err, "CMDB_TRANSACTION_FAILED") || strings.Contains(err.Error(), "hidden") {
		t.Fatalf("unexpected injected failure: %v", err)
	}
	state, err := store.Verify(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.Head != nil || state.RevisionCount != 0 {
		t.Fatalf("failed transaction left partial history: %+v", state)
	}
}

func TestContainerRevisionCorruptionFailsClosed(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	first := staticLoadFixture(t, "corrupt-base.yml", "all: { vars: { generation: 1 } }\n")
	second := staticLoadFixture(t, "corrupt-next.yml", "all: { vars: { generation: 2 } }\n")
	base, err := store.AppendStatic(t.Context(), nil, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendStatic(t.Context(), &base.Token, second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(t.Context(), `ALTER TABLE pigsty_cmdb.snapshot DISABLE TRIGGER snapshot_append_only`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(t.Context(), `UPDATE pigsty_cmdb.snapshot SET payload = $1::jsonb WHERE revision = 1`,
		string(second.Snapshot.CanonicalBytes())); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(t.Context(), `ALTER TABLE pigsty_cmdb.snapshot ENABLE TRIGGER snapshot_append_only`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(t.Context()); !IsCode(err, "CMDB_HISTORY_INVALID") {
		t.Fatalf("payload/digest corruption was accepted: %v", err)
	}

	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	base, err = store.AppendStatic(t.Context(), nil, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendStatic(t.Context(), &base.Token, second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(t.Context(), `UPDATE pigsty_cmdb.state SET head_revision = 1 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(t.Context()); !IsCode(err, "CMDB_HISTORY_INVALID") {
		t.Fatalf("non-maximal head was accepted: %v", err)
	}
}

func staticLoadFixture(t *testing.T, name, source string) StaticLoad {
	t.Helper()
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, name)
	bytes := []byte(source)
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := inventory.Parse(path, bytes)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	return StaticLoad{
		Snapshot:          snapshot,
		SourceLocator:     path,
		SourceBytesSHA256: sha256.Sum256(bytes),
		ToolVersion:       "test-v1",
	}
}
