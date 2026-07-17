package cmdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pig/inventory"
)

func TestContainerPlainDumpIsCanonicalPrivateAndExclusive(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}

	first := staticLoadFixture(t, "dump-first.yml", "all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    password: first-private-value\n    generation: 1\n")
	firstResult := applyStaticLoadFixture(t, store, first)
	second := staticLoadFixture(t, "dump-second.yml", "all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    password: second-private-value\n    generation: 2\n")
	secondResult := applyStaticLoadFixture(t, store, second)

	directory := symlinkFreeTempDir(t)
	headPath := filepath.Join(directory, "head.yml")
	head, err := store.Dump(t.Context(), DumpRequest{OutputPath: headPath})
	if err != nil {
		t.Fatal(err)
	}
	if !head.Revision.Equal(secondResult.Token) || head.RevisionCount != 2 || head.NodeCount != second.Snapshot.NodeCount() ||
		head.Merge || head.OutputPath != headPath || head.OutputSHA256 == "" || head.SourceSHA256 != "" || len(head.Conflicts) != 0 {
		t.Fatalf("plain dump metadata = %+v", head)
	}
	assertPlainDumpFile(t, headPath, secondResult.Token, second.Snapshot, head.OutputSHA256)

	encoded, err := json.Marshal(head)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		"first-private-value", "second-private-value", string(second.Snapshot.CanonicalBytes()),
		first.SourceLocator, second.SourceLocator,
	} {
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatalf("plain dump metadata leaked private data: %s", encoded)
		}
	}

	historicalPath := filepath.Join(directory, "historical.yml")
	historical, err := store.Dump(t.Context(), DumpRequest{OutputPath: historicalPath, Revision: &firstResult.Token})
	if err != nil {
		t.Fatal(err)
	}
	if !historical.Revision.Equal(firstResult.Token) || historical.RevisionCount != 2 {
		t.Fatalf("historical dump metadata = %+v", historical)
	}
	assertPlainDumpFile(t, historicalPath, firstResult.Token, first.Snapshot, historical.OutputSHA256)

	wrongRevision := newRevisionToken(firstResult.Token.Revision(), sha256.Sum256([]byte("wrong-revision")))
	wrongRevisionPath := filepath.Join(directory, "wrong-revision.yml")
	if _, err := store.Dump(t.Context(), DumpRequest{OutputPath: wrongRevisionPath, Revision: &wrongRevision}); !IsCode(err, "CMDB_REVISION_NOT_FOUND") {
		t.Fatalf("non-exact historical revision was accepted: %v", err)
	}
	assertPathAbsent(t, wrongRevisionPath)

	before, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Dump(t.Context(), DumpRequest{OutputPath: headPath}); !IsCode(err, "CMDB_OUTPUT_EXISTS") {
		t.Fatalf("existing output was accepted: %v", err)
	}
	after, _ := os.ReadFile(headPath)
	if !bytes.Equal(before, after) {
		t.Fatal("existing output changed")
	}

	target := filepath.Join(directory, "symlink-target.yml")
	if err := os.WriteFile(target, []byte("sentinel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "symlink-output.yml")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Dump(t.Context(), DumpRequest{OutputPath: symlink}); !IsCode(err, "CMDB_OUTPUT_EXISTS") {
		t.Fatalf("symlink output was accepted: %v", err)
	}
	unchanged, _ := os.ReadFile(target)
	if string(unchanged) != "sentinel\n" {
		t.Fatalf("symlink target changed: %q", unchanged)
	}
}

func TestContainerMergeDumpPreservesSourceAndFailsClosed(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}

	baseOne := staticLoadFixture(t, "merge-base-one.yml", "all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    a: base-one-private\n    b: base-private\n")
	baseOneResult := applyStaticLoadFixture(t, store, baseOne)
	baseTwo := staticLoadFixture(t, "merge-base-two.yml", "all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    a: cmdb-two-private\n    b: base-private\n")
	baseTwoResult := applyStaticLoadFixture(t, store, baseTwo)
	rollbackPlan, err := store.PlanRollback(t.Context(), baseOneResult.Token, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := store.ApplyLoad(t.Context(), rollbackPlan, LoadConsent{Confirmed: true})
	if err != nil || !rollback.Changed || rollback.RevisionCount != 3 {
		t.Fatalf("rollback setup failed: result=%+v err=%v", rollback, err)
	}

	working := mergeWorkingSource(baseTwoResult.Token, "operator-private")
	if err := os.WriteFile(baseTwo.SourceLocator, working, 0o600); err != nil {
		t.Fatal(err)
	}
	directory := symlinkFreeTempDir(t)
	outputPath := filepath.Join(directory, "merged.yml")
	merged, err := store.Dump(t.Context(), DumpRequest{
		OutputPath: outputPath,
		Merge:      true,
		ExpectBase: &baseTwoResult.Token,
	})
	if err != nil {
		t.Fatal(err)
	}
	workingDigest := sha256.Sum256(working)
	if !merged.Merge || !merged.Revision.Equal(rollback.Token) || merged.Base == nil || !merged.Base.Equal(baseTwoResult.Token) ||
		merged.SourceSHA256 != hex.EncodeToString(workingDigest[:]) || merged.OutputPath != outputPath || len(merged.Changes) != 1 || len(merged.Conflicts) != 0 {
		t.Fatalf("merge dump metadata = %+v", merged)
	}
	if merged.Changes[0].Operation != inventory.SnapshotReplace || merged.Changes[0].Path.String() != "/all/vars/a" {
		t.Fatalf("merge change metadata = %+v", merged.Changes)
	}
	assertMergedDumpFile(t, outputPath, baseTwoResult.Token, rollback.Token, workingDigest)

	encoded, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, encoded, "base-one-private", "cmdb-two-private", "base-private", "operator-private", string(working))

	wrongBase := newRevisionToken(baseTwoResult.Token.Revision(), sha256.Sum256([]byte("wrong-base")))
	wrongBasePath := filepath.Join(directory, "wrong-base.yml")
	if _, err := store.Dump(t.Context(), DumpRequest{
		OutputPath: wrongBasePath, Merge: true, ExpectBase: &wrongBase,
	}); !IsCode(err, "CMDB_BASE_MISMATCH") {
		t.Fatalf("wrong expected base was accepted: %v", err)
	}
	assertPathAbsent(t, wrongBasePath)

	invalidSource := mergeWorkingSource(baseTwoResult.Token, "base-private")
	invalidSource = bytes.Replace(invalidSource, []byte("10.0.0.1"), []byte("merge-validation-private"), 1)
	if err := os.WriteFile(baseTwo.SourceLocator, invalidSource, 0o600); err != nil {
		t.Fatal(err)
	}
	invalidSourcePath := filepath.Join(directory, "invalid-source.yml")
	if _, err := store.Dump(t.Context(), DumpRequest{OutputPath: invalidSourcePath, Merge: true}); !IsCode(err, "CMDB_SOURCE_INVALID") {
		t.Fatalf("invalid current source was accepted: %v", err)
	} else {
		assertAbsent(t, []byte(err.Error()), "merge-validation-private", string(invalidSource))
	}
	assertPathAbsent(t, invalidSourcePath)

	unrepresentableSource := mergeWorkingSource(baseTwoResult.Token, "base-private")
	unrepresentableSource = bytes.Replace(unrepresentableSource, []byte("a: cmdb-two-private"), []byte("a: !vault merge-vault-private"), 1)
	if err := os.WriteFile(baseTwo.SourceLocator, unrepresentableSource, 0o600); err != nil {
		t.Fatal(err)
	}
	unrepresentablePath := filepath.Join(directory, "unrepresentable-source.yml")
	if _, err := store.Dump(t.Context(), DumpRequest{OutputPath: unrepresentablePath, Merge: true}); !IsCode(err, "CMDB_SNAPSHOT_UNREPRESENTABLE") {
		t.Fatalf("unrepresentable current source was accepted: %v", err)
	} else {
		assertAbsent(t, []byte(err.Error()), "merge-vault-private", string(unrepresentableSource))
	}
	assertPathAbsent(t, unrepresentablePath)

	conflictSource := mergeWorkingSource(baseTwoResult.Token, "base-private")
	conflictSource = bytes.Replace(conflictSource, []byte("cmdb-two-private"), []byte("operator-conflict-private"), 1)
	if err := os.WriteFile(baseTwo.SourceLocator, conflictSource, 0o600); err != nil {
		t.Fatal(err)
	}
	conflictPath := filepath.Join(directory, "conflict.yml")
	conflict, err := store.Dump(t.Context(), DumpRequest{OutputPath: conflictPath, Merge: true})
	if !IsCode(err, "CMDB_MERGE_CONFLICT") || len(conflict.Conflicts) == 0 || conflict.OutputPath != "" {
		t.Fatalf("merge conflict did not fail closed: result=%+v err=%v", conflict, err)
	}
	conflictJSON, marshalErr := json.Marshal(conflict)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	assertAbsent(t, conflictJSON, "base-one-private", "cmdb-two-private", "base-private", "operator-conflict-private", string(conflictSource))
	assertAbsent(t, []byte(err.Error()), "base-one-private", "cmdb-two-private", "base-private", "operator-conflict-private")
	assertPathAbsent(t, conflictPath)

	if err := os.WriteFile(baseTwo.SourceLocator, working, 0o600); err != nil {
		t.Fatal(err)
	}
	changedSource := mergeWorkingSource(baseTwoResult.Token, "changed-during-dump-private")
	recheckPath := filepath.Join(directory, "source-race.yml")
	_, err = store.Dump(t.Context(), DumpRequest{
		OutputPath: recheckPath,
		Merge:      true,
		beforeSourceRecheck: func() error {
			return os.WriteFile(baseTwo.SourceLocator, changedSource, 0o600)
		},
	})
	if !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("source exact-byte race was accepted: %v", err)
	}
	assertPathAbsent(t, recheckPath)

	if err := os.WriteFile(baseTwo.SourceLocator, working, 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(directory, "lock-held.yml")
	lockChecked := false
	locked, err := store.Dump(t.Context(), DumpRequest{
		OutputPath: lockPath,
		Merge:      true,
		beforeDestinationCommit: func() error {
			var acquired bool
			if err := store.pool.QueryRow(t.Context(), `SELECT pg_try_advisory_xact_lock($1)`, CMDBAdvisoryKey).Scan(&acquired); err != nil {
				return err
			}
			lockChecked = true
			if acquired {
				_, _ = store.pool.Exec(t.Context(), `SELECT pg_advisory_unlock($1)`, CMDBAdvisoryKey)
				return errors.New("exclusive native lock was available during merge commit")
			}
			return nil
		},
	})
	if err != nil || !lockChecked || locked.OutputPath != lockPath {
		t.Fatalf("shared lock was not held through destination commit: checked=%v result=%+v err=%v", lockChecked, locked, err)
	}

	movedSource := baseTwo.SourceLocator + ".moved"
	if err := os.Rename(baseTwo.SourceLocator, movedSource); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(movedSource, baseTwo.SourceLocator); err != nil {
		t.Fatal(err)
	}
	symlinkSourcePath := filepath.Join(directory, "symlink-source.yml")
	if _, err := store.Dump(t.Context(), DumpRequest{OutputPath: symlinkSourcePath, Merge: true}); !IsCode(err, "CMDB_BASE_MISMATCH") {
		t.Fatalf("symlinked recorded source was accepted: %v", err)
	}
	assertPathAbsent(t, symlinkSourcePath)
}

func applyStaticLoadFixture(t *testing.T, store *Store, load StaticLoad) AppendResult {
	t.Helper()
	plan, err := store.PlanStaticLoad(t.Context(), load)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ApplyLoad(t.Context(), plan, LoadConsent{Prune: true, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertPlainDumpFile(t *testing.T, path string, token RevisionToken, snapshot *inventory.Snapshot, digest string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=" + token.String() + "\n"
	if !bytes.HasPrefix(body, []byte(marker)) {
		t.Fatalf("plain marker mismatch: %q", bytes.SplitN(body, []byte("\n"), 2)[0])
	}
	wantBody, err := snapshot.CanonicalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body[len(marker):], wantBody) {
		t.Fatalf("canonical body mismatch: %q", body[len(marker):])
	}
	computed := sha256.Sum256(body)
	if digest != hex.EncodeToString(computed[:]) {
		t.Fatalf("output digest = %q, want %x", digest, computed)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("plain output mode = %v, err=%v", info.Mode(), err)
	}
}

func mergeWorkingSource(base RevisionToken, bValue string) []byte {
	return []byte(cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=" + base.String() + "\n" +
		"# operator-owned comment\n" +
		"all:\n" +
		"  children: {} # preserve child style\n" +
		"  vars:\n" +
		"    admin_ip: 10.0.0.1\n" +
		"    a: cmdb-two-private # preserve a comment\n" +
		"    b: " + bValue + " # preserve b comment\n")
}

func assertMergedDumpFile(t *testing.T, path string, base, head RevisionToken, sourceDigest [sha256.Size]byte) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := cmdbMarkerPrefix + " kind=" + mergeMarkerKind + " base=" + base.String() +
		" head=" + head.String() + " source-sha256=" + hex.EncodeToString(sourceDigest[:]) + "\n"
	if !bytes.HasPrefix(body, []byte(marker)) || bytes.Count(body, []byte(cmdbMarkerPrefix)) != 1 {
		t.Fatalf("merge marker mismatch: %q", bytes.SplitN(body, []byte("\n"), 2)[0])
	}
	if !bytes.Contains(body, []byte("# operator-owned comment\n")) ||
		!bytes.Contains(body, []byte("children: {} # preserve child style\n")) ||
		!bytes.Contains(body, []byte("b: operator-private # preserve b comment\n")) {
		t.Fatalf("lossless source bytes were not preserved: %q", body)
	}
	document, err := inventory.Parse(path, body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	wantDocument, err := inventory.Parse("expected.yml", []byte("all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    a: base-one-private\n    b: operator-private\n"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := inventory.SnapshotFromDocument(wantDocument)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("merged Snapshot digest = %s, want %s", got.Digest(), want.Digest())
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("merge output mode = %v, err=%v", info.Mode(), err)
	}
}

func assertAbsent(t *testing.T, body []byte, private ...string) {
	t.Helper()
	for _, value := range private {
		if value != "" && bytes.Contains(body, []byte(value)) {
			t.Fatalf("private value %q leaked in %s", value, body)
		}
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination exists after refused dump: %q err=%v", path, err)
	}
}
