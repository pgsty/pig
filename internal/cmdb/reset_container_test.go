package cmdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestContainerResetBackupRestoreRoundTrip(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	createOutsideSentinels(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	first := staticLoadFixture(t, "reset-first.yml", "all: { vars: { generation: 1, password: bundle-secret } }\n")
	second := staticLoadFixture(t, "reset-second.yml", "all: { vars: { generation: 2, password: bundle-secret } }\n")
	firstResult, err := store.AppendStatic(t.Context(), nil, first)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := store.AppendStatic(t.Context(), &firstResult.Token, second)
	if err != nil {
		t.Fatal(err)
	}
	rollbackResult, err := store.AppendRollback(t.Context(), secondResult.Token, firstResult.Token, "test-v1")
	if err != nil {
		t.Fatal(err)
	}

	planned, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if planned.Absent || planned.RevisionCount != 3 || planned.Head == nil || !planned.Head.Equal(rollbackResult.Token) {
		t.Fatalf("unexpected native reset inspection: %+v", planned)
	}
	directory := privateBundleDirectory(t)
	backupPath := filepath.Join(directory, "native-reset.ndjson")
	resetResult, err := store.Reset(t.Context(), ResetRequest{
		Expected: planned.Fingerprint, BackupPath: backupPath, ToolVersion: "test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resetResult.Restored || resetResult.BackupPath != backupPath || len(resetResult.BackupSHA256) != 64 ||
		resetResult.State.Head != nil || resetResult.State.RevisionCount != 0 {
		t.Fatalf("unexpected reset result: %+v", resetResult)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode is not 0600: mode=%v", info.Mode())
	}
	bundle, err := InspectBundle(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	bundleState := bundle.State()
	if bundleState.RevisionCount != 3 || bundleState.Head == nil || !bundleState.Head.Equal(rollbackResult.Token) ||
		bundle.SHA256() != resetResult.BackupSHA256 {
		t.Fatalf("backup does not describe the exact old state: %+v", bundleState)
	}
	assertOutsideSentinels(t, store)

	// Restore the verified bundle into an absent schema: there is no old native
	// state and therefore no pre-reset backup is permitted.
	resetContainerSchema(t, store)
	absent, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !absent.Absent {
		t.Fatalf("schema absence was not proven: %+v", absent)
	}
	restored, err := store.Reset(t.Context(), ResetRequest{
		Expected: absent.Fingerprint, Restore: &bundle, ToolVersion: "test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Restored || restored.BackupPath != "" || restored.State.Head == nil ||
		!restored.State.Head.Equal(rollbackResult.Token) || restored.State.RevisionCount != 3 {
		t.Fatalf("fresh restore did not reinstate exact history: %+v", restored)
	}
	restoredInspection, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !restoredInspection.Fingerprint.Equal(bundle.fingerprint) {
		t.Fatal("fresh restore did not preserve the exact state fingerprint")
	}

	// Restoring over verified native state requires another completed backup.
	nativeBackupPath := filepath.Join(directory, "before-native-restore.ndjson")
	nativeRestore, err := store.Reset(t.Context(), ResetRequest{
		Expected:    restoredInspection.Fingerprint,
		BackupPath:  nativeBackupPath,
		Restore:     &bundle,
		ToolVersion: "test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !nativeRestore.Restored || nativeRestore.BackupPath != nativeBackupPath {
		t.Fatalf("native restore did not create its mandatory backup: %+v", nativeRestore)
	}
	nativeBackup, err := InspectBundle(nativeBackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if !nativeBackup.fingerprint.Equal(bundle.fingerprint) {
		t.Fatal("native pre-restore backup did not preserve exact history")
	}
	assertOutsideSentinels(t, store)

	// Existing destinations are never overwritten and the database stays put.
	current, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: current.Fingerprint, BackupPath: nativeBackupPath, ToolVersion: "test-v1",
	}); !IsCode(err, "CMDB_BACKUP_INVALID") {
		t.Fatalf("reset accepted an existing backup destination: %v", err)
	}
	afterRefusal, err := store.InspectReset(t.Context())
	if err != nil || !afterRefusal.Fingerprint.Equal(current.Fingerprint) {
		t.Fatalf("backup destination refusal changed database state: %+v err=%v", afterRefusal, err)
	}
}

func TestContainerResetDistinguishesEmptyNativeFromAbsence(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	emptyNative, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if emptyNative.Absent || emptyNative.Head != nil || emptyNative.RevisionCount != 0 {
		t.Fatalf("empty native schema was confused with absence: %+v", emptyNative)
	}
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: emptyNative.Fingerprint, ToolVersion: "test-v1",
	}); !IsCode(err, "CMDB_BACKUP_REQUIRED") {
		t.Fatalf("empty native reset omitted its mandatory backup: %v", err)
	}
	unchanged, err := store.InspectReset(t.Context())
	if err != nil || !unchanged.Fingerprint.Equal(emptyNative.Fingerprint) {
		t.Fatalf("missing-backup refusal changed empty native state: %+v err=%v", unchanged, err)
	}

	path := filepath.Join(privateBundleDirectory(t), "empty-native.ndjson")
	result, err := store.Reset(t.Context(), ResetRequest{
		Expected: emptyNative.Fingerprint, BackupPath: path, ToolVersion: "test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := InspectBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath != path || backup.State().Head != nil || backup.State().RevisionCount != 0 {
		t.Fatalf("empty native backup is not exact: result=%+v state=%+v", result, backup.State())
	}

	resetContainerSchema(t, store)
	absent, err := store.InspectReset(t.Context())
	if err != nil || !absent.Absent {
		t.Fatalf("schema absence was not proven: %+v err=%v", absent, err)
	}
	unused := filepath.Join(privateBundleDirectory(t), "absence-must-not-back-up.ndjson")
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: absent.Fingerprint, BackupPath: unused, ToolVersion: "test-v1",
	}); !IsCode(err, "CMDB_PARAMETER_INVALID") {
		t.Fatalf("absent schema claimed a pre-reset backup: %v", err)
	}
	after, err := store.InspectReset(t.Context())
	if err != nil || !after.Absent || !after.Fingerprint.Equal(absent.Fingerprint) {
		t.Fatalf("absent backup refusal performed DDL: %+v err=%v", after, err)
	}
}

func TestContainerResetBackupSurvivesDatabaseRollback(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	candidate := staticLoadFixture(t, "reset-failure.yml", "all: { vars: { generation: 1 } }\n")
	created, err := store.AppendStatic(t.Context(), nil, candidate)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(privateBundleDirectory(t), "survives-rollback.ndjson")
	_, err = store.Reset(t.Context(), ResetRequest{
		Expected: planned.Fingerprint, BackupPath: backupPath, ToolVersion: "test-v1",
		afterBackup: func() error { return errors.New("test failure") },
	})
	if !IsCode(err, "CMDB_TRANSACTION_FAILED") {
		t.Fatalf("unexpected injected reset failure: %v", err)
	}
	bundle, err := InspectBundle(backupPath)
	if err != nil {
		t.Fatalf("completed backup did not survive rollback: %v", err)
	}
	if state := bundle.State(); state.Head == nil || !state.Head.Equal(created.Token) {
		t.Fatalf("surviving backup has wrong head: %+v", state)
	}
	current, err := store.InspectReset(t.Context())
	if err != nil || !current.Fingerprint.Equal(planned.Fingerprint) {
		t.Fatalf("database transaction did not roll back: %+v err=%v", current, err)
	}
}

func TestContainerResetConflictsAndForeignSchemaStayUntouched(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	createOutsideSentinels(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	first := staticLoadFixture(t, "reset-conflict-one.yml", "all: { vars: { generation: 1 } }\n")
	second := staticLoadFixture(t, "reset-conflict-two.yml", "all: { vars: { generation: 2 } }\n")
	firstResult, err := store.AppendStatic(t.Context(), nil, first)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := store.AppendStatic(t.Context(), &firstResult.Token, second)
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(privateBundleDirectory(t), "must-not-exist.ndjson")
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: stale.Fingerprint, BackupPath: backupPath, ToolVersion: "test-v1",
	}); !IsCode(err, "CMDB_RESET_CONFLICT") {
		t.Fatalf("stale reset inspection was accepted: %v", err)
	}
	if _, err := os.Lstat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting reset created a backup: %v", err)
	}
	state, err := store.Verify(t.Context())
	if err != nil || state.Head == nil || !state.Head.Equal(secondResult.Token) || state.RevisionCount != 2 {
		t.Fatalf("conflicting reset changed history: %+v err=%v", state, err)
	}
	assertOutsideSentinels(t, store)

	resetContainerSchema(t, store)
	if _, err := store.pool.Exec(t.Context(), `
CREATE SCHEMA pigsty_cmdb;
CREATE TABLE pigsty_cmdb.foreign_reset_sentinel (id integer PRIMARY KEY);
INSERT INTO pigsty_cmdb.foreign_reset_sentinel VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectReset(t.Context()); !IsCode(err, "CMDB_SCHEMA_FOREIGN") {
		t.Fatalf("foreign schema produced a reset inspection: %v", err)
	}
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: absentResetFingerprint(), ToolVersion: "test-v1",
	}); !IsCode(err, "CMDB_SCHEMA_FOREIGN") {
		t.Fatalf("foreign schema reset was not refused: %v", err)
	}
	var count int
	if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM pigsty_cmdb.foreign_reset_sentinel`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("foreign reset sentinel changed: count=%d err=%v", count, err)
	}
	assertOutsideSentinels(t, store)

	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	native, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(t.Context(), `
CREATE TABLE pigsty_cmdb.malformed_reset_sentinel (id integer PRIMARY KEY);
INSERT INTO pigsty_cmdb.malformed_reset_sentinel VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectReset(t.Context()); !IsCode(err, "CMDB_SCHEMA_INVALID") {
		t.Fatalf("malformed native schema produced a reset inspection: %v", err)
	}
	malformedBackup := filepath.Join(privateBundleDirectory(t), "malformed-must-not-exist.ndjson")
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: native.Fingerprint, BackupPath: malformedBackup, ToolVersion: "test-v1",
	}); !IsCode(err, "CMDB_SCHEMA_INVALID") {
		t.Fatalf("malformed native schema reset was not refused: %v", err)
	}
	if _, err := os.Lstat(malformedBackup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed reset created a backup: %v", err)
	}
	if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM pigsty_cmdb.malformed_reset_sentinel`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("malformed reset sentinel changed: count=%d err=%v", count, err)
	}
	assertOutsideSentinels(t, store)
}

func TestContainerRestoreBundleRecheckedBeforeDDL(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	secret := "never-print-this-bundle-value"
	candidate := staticLoadFixture(t, "restore-recheck.yml", "all: { vars: { password: "+secret+" } }\n")
	created, err := store.AppendStatic(t.Context(), nil, candidate)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateBundleDirectory(t), "restore-recheck.ndjson")
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: planned.Fingerprint, BackupPath: path, ToolVersion: "test-v1",
	}); err != nil {
		t.Fatal(err)
	}
	bundle, err := InspectBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if state := bundle.State(); state.Head == nil || !state.Head.Equal(created.Token) {
		t.Fatalf("unexpected source bundle: %+v", state)
	}
	resetContainerSchema(t, store)
	absent, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Reset(t.Context(), ResetRequest{
		Expected: absent.Fingerprint, Restore: &bundle, ToolVersion: "test-v1",
		afterRestorePreflight: func() error {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content[len(content)/2] ^= 1
			return os.WriteFile(path, content, 0o600)
		},
	})
	if !IsCode(err, "CMDB_BACKUP_INVALID") && !IsCode(err, "CMDB_BACKUP_CHANGED") {
		t.Fatalf("changed restore bundle was accepted: %v", err)
	}
	if err != nil && bytes.Contains([]byte(err.Error()), []byte(secret)) {
		t.Fatalf("restore error leaked bundle body: %v", err)
	}
	after, err := store.InspectReset(t.Context())
	if err != nil || !after.Absent {
		t.Fatalf("bundle recheck failure performed DDL: %+v err=%v", after, err)
	}
}

func TestContainerBundleRejectsTamperCompatibilityAndPermissions(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	candidate := staticLoadFixture(t, "bundle-validation.yml", "all: { vars: { generation: 1 } }\n")
	if _, err := store.AppendStatic(t.Context(), nil, candidate); err != nil {
		t.Fatal(err)
	}
	planned, err := store.InspectReset(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	directory := privateBundleDirectory(t)
	validPath := filepath.Join(directory, "valid.ndjson")
	if _, err := store.Reset(t.Context(), ResetRequest{
		Expected: planned.Fingerprint, BackupPath: validPath, ToolVersion: "test-v1",
	}); err != nil {
		t.Fatal(err)
	}
	valid, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}

	tamperedPath := filepath.Join(directory, "tampered.ndjson")
	tampered := bytes.Clone(valid)
	tampered[len(tampered)/2] ^= 1
	if err := os.WriteFile(tamperedPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(tamperedPath); !IsCode(err, "CMDB_BACKUP_INVALID") {
		t.Fatalf("tampered bundle was accepted: %v", err)
	}

	incompatiblePath := filepath.Join(directory, "incompatible.ndjson")
	incompatible := bundleWithFutureMigration(t, valid)
	if err := os.WriteFile(incompatiblePath, incompatible, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(incompatiblePath); !IsCode(err, "CMDB_BACKUP_INVALID") {
		t.Fatalf("migration-incompatible bundle was accepted: %v", err)
	}

	if err := os.Chmod(validPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(validPath); !IsCode(err, "CMDB_BACKUP_INVALID") {
		t.Fatalf("world-readable bundle was accepted: %v", err)
	}
	if err := os.Chmod(validPath, 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(directory, "bundle-link.ndjson")
	if err := os.Symlink(validPath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(symlinkPath); !IsCode(err, "CMDB_BACKUP_INVALID") {
		t.Fatalf("symlink bundle was accepted: %v", err)
	}
}

func privateBundleDirectory(t *testing.T) string {
	t.Helper()
	directory := symlinkFreeTempDir(t)
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func bundleWithFutureMigration(t *testing.T, valid []byte) []byte {
	t.Helper()
	lines := bytes.SplitAfter(valid, []byte("\n"))
	if len(lines) < 3 || len(lines[len(lines)-1]) != 0 {
		t.Fatal("unexpected bundle fixture line shape")
	}
	footerIndex := len(lines) - 2
	body := bytes.Join(lines[:footerIndex], nil)
	changed := bytes.Replace(body, []byte(`"version":1`), []byte(`"version":2`), 1)
	if bytes.Equal(changed, body) {
		t.Fatal("migration record was not found")
	}
	digest := sha256.Sum256(changed)
	footer := []byte(`{"record":"checksum","sha256":"` + hex.EncodeToString(digest[:]) + `"}` + "\n")
	return append(changed, footer...)
}
