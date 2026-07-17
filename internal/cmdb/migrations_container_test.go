package cmdb

import (
	"bytes"
	"context"
	"sync"
	"testing"
)

func TestContainerMigrationIdempotenceAndConcurrency(t *testing.T) {
	first := openContainerStore(t)
	resetContainerSchema(t, first)
	createOutsideSentinels(t, first)
	second := openContainerStore(t)

	start := make(chan struct{})
	results := make(chan InitResult, 2)
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, store := range []*Store{first, second} {
		wait.Add(1)
		go func(store *Store) {
			defer wait.Done()
			<-start
			result, err := store.Init(context.Background(), "test-v1")
			results <- result
			errors <- err
		}(store)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	applied := 0
	for result := range results {
		applied += result.Applied
		if result.MigrationVersion != 1 {
			t.Fatalf("unexpected migration version: %+v", result)
		}
	}
	if applied != 1 {
		t.Fatalf("concurrent init applied migration %d times", applied)
	}

	before := migrationState(t, first)
	result, err := first.Init(t.Context(), "different-tool-version")
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 0 || !bytes.Equal(before, migrationState(t, first)) {
		t.Fatal("current init was not a database-state no-op")
	}
	assertOutsideSentinels(t, first)
	t.Logf("pinned platform: %s", cmdbContainer.selectedPlatform)
}

func TestContainerMigrationFailsClosed(t *testing.T) {
	store := openContainerStore(t)

	t.Run("foreign schema", func(t *testing.T) {
		resetContainerSchema(t, store)
		if _, err := store.pool.Exec(t.Context(), `
CREATE SCHEMA pigsty_cmdb;
CREATE TABLE pigsty_cmdb.foreign_sentinel (id integer PRIMARY KEY);
INSERT INTO pigsty_cmdb.foreign_sentinel VALUES (1)`); err != nil {
			t.Fatal(err)
		}
		_, err := store.Init(t.Context(), "test-v1")
		if !IsCode(err, "CMDB_SCHEMA_FOREIGN") {
			t.Fatalf("expected foreign schema failure, got %v", err)
		}
		var count int
		if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM pigsty_cmdb.foreign_sentinel`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("foreign schema mutated: count=%d err=%v", count, err)
		}
	})

	t.Run("checksum drift", func(t *testing.T) {
		resetContainerSchema(t, store)
		if _, err := store.Init(t.Context(), "test-v1"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(t.Context(), `
DROP TRIGGER schema_migration_append_only ON pigsty_cmdb.schema_migration;
UPDATE pigsty_cmdb.schema_migration SET checksum = decode(repeat('00', 32), 'hex') WHERE version = 1`); err != nil {
			t.Fatal(err)
		}
		before := migrationState(t, store)
		_, err := store.Init(t.Context(), "test-v1")
		if !IsCode(err, "CMDB_MIGRATION_DRIFT") {
			t.Fatalf("expected checksum drift failure, got %v", err)
		}
		if !bytes.Equal(before, migrationState(t, store)) {
			t.Fatal("checksum drift path mutated schema state")
		}
	})

	t.Run("newer version", func(t *testing.T) {
		resetContainerSchema(t, store)
		if _, err := store.Init(t.Context(), "test-v1"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(t.Context(), `
INSERT INTO pigsty_cmdb.schema_migration
       (version, name, checksum, tool_version, applied_at)
VALUES (2, 'future', decode(repeat('01', 32), 'hex'), 'future', clock_timestamp())`); err != nil {
			t.Fatal(err)
		}
		before := migrationState(t, store)
		_, err := store.Init(t.Context(), "test-v1")
		if !IsCode(err, "CMDB_MIGRATION_NEWER") {
			t.Fatalf("expected newer migration failure, got %v", err)
		}
		if !bytes.Equal(before, migrationState(t, store)) {
			t.Fatal("newer migration path mutated schema state")
		}
	})

	t.Run("shape drift", func(t *testing.T) {
		resetContainerSchema(t, store)
		if _, err := store.Init(t.Context(), "test-v1"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(t.Context(), `CREATE TABLE pigsty_cmdb.unauthorized (id integer)`); err != nil {
			t.Fatal(err)
		}
		_, err := store.Init(t.Context(), "test-v1")
		if !IsCode(err, "CMDB_SCHEMA_INVALID") {
			t.Fatalf("expected shape failure, got %v", err)
		}
		var exists bool
		if err := store.pool.QueryRow(t.Context(), `SELECT to_regclass('pigsty_cmdb.unauthorized') IS NOT NULL`).Scan(&exists); err != nil || !exists {
			t.Fatalf("shape drift path mutated foreign object: exists=%v err=%v", exists, err)
		}
	})
}

func TestContainerAppendOnlyAndPublicBoundary(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE pigsty_cmdb.schema_migration SET tool_version = 'changed' WHERE version = 1`,
		`DELETE FROM pigsty_cmdb.schema_migration WHERE version = 1`,
	} {
		if _, err := store.pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("append-only ledger accepted %s", statement)
		}
	}
	tx, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := validateNativeSchema(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
}

func createOutsideSentinels(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.pool.Exec(t.Context(), `
CREATE SCHEMA IF NOT EXISTS pigsty;
CREATE TABLE IF NOT EXISTS pigsty.cmdb_outside_sentinel (id integer PRIMARY KEY);
INSERT INTO pigsty.cmdb_outside_sentinel VALUES (1) ON CONFLICT DO NOTHING;
CREATE SCHEMA IF NOT EXISTS pglog;
CREATE TABLE IF NOT EXISTS pglog.cmdb_outside_sentinel (id integer PRIMARY KEY);
INSERT INTO pglog.cmdb_outside_sentinel VALUES (1) ON CONFLICT DO NOTHING;
CREATE SCHEMA IF NOT EXISTS unrelated_cmdb_test;
CREATE TABLE IF NOT EXISTS unrelated_cmdb_test.sentinel (id integer PRIMARY KEY);
INSERT INTO unrelated_cmdb_test.sentinel VALUES (1) ON CONFLICT DO NOTHING`)
	if err != nil {
		t.Fatal(err)
	}
}

func assertOutsideSentinels(t *testing.T, store *Store) {
	t.Helper()
	for _, table := range []string{
		"pigsty.cmdb_outside_sentinel",
		"pglog.cmdb_outside_sentinel",
		"unrelated_cmdb_test.sentinel",
	} {
		var count int
		query := "SELECT count(*) FROM " + table // fixed test-only identifiers
		if err := store.pool.QueryRow(t.Context(), query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("outside sentinel %s changed: count=%d err=%v", table, count, err)
		}
	}
}

func migrationState(t *testing.T, store *Store) []byte {
	t.Helper()
	var state []byte
	err := store.pool.QueryRow(t.Context(), `
SELECT pg_catalog.convert_to(
           COALESCE(jsonb_agg(to_jsonb(m) ORDER BY version)::text, '[]') || '|' ||
           (SELECT to_jsonb(s)::text FROM pigsty_cmdb.state AS s),
           'UTF8')
  FROM pigsty_cmdb.schema_migration AS m`).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
