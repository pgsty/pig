package cmdb

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	SchemaName      = "pigsty_cmdb"
	CMDBAdvisoryKey = int64(5644146564720209289)
	minimumPGMajor  = 14
	maximumPGMajor  = 18
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	Version        int64
	Name           string
	MinimumPGMajor int
	SQL            []byte
	Checksum       [sha256.Size]byte
}

var migrations = mustLoadMigrations()

// InitResult is value-free migration metadata suitable for a later Result.
type InitResult struct {
	MigrationVersion int64
	Applied          int
}

func mustLoadMigrations() []migration {
	entries, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		panic(err)
	}
	sort.Strings(entries)
	result := make([]migration, 0, len(entries))
	for index, name := range entries {
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			panic(err)
		}
		version := int64(index + 1)
		prefix := fmt.Sprintf("migrations/%03d_", version)
		if !strings.HasPrefix(name, prefix) || len(body) == 0 {
			panic("invalid embedded CMDB migration ordering")
		}
		result = append(result, migration{
			Version:        version,
			Name:           strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".sql"),
			MinimumPGMajor: minimumPGMajor,
			SQL:            slices.Clone(body),
			Checksum:       sha256.Sum256(body),
		})
	}
	if len(result) != 1 {
		panic("CMDB v1 requires exactly one embedded migration")
	}
	return result
}

// Init applies the embedded migration ledger exactly once under the stable
// transaction lock, or verifies an existing current native schema unchanged.
func (s *Store) Init(ctx context.Context, toolVersion string) (InitResult, error) {
	if s == nil || s.pool == nil {
		return InitResult{}, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	if toolVersion == "" || len(toolVersion) > 128 || !utf8.ValidString(toolVersion) {
		return InitResult{}, failure("CMDB_PARAMETER_INVALID", "tool version is invalid")
	}
	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	// READ COMMITTED is deliberate for bootstrap: a concurrent initializer that
	// waited on the advisory lock must observe the schema committed by its
	// predecessor rather than retain a pre-lock transaction snapshot.
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return InitResult{}, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "transaction start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))

	if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock($1)`, CMDBAdvisoryKey); err != nil {
		return InitResult{}, sanitizeDatabaseFailure("CMDB_LOCK_FAILED", "lock", err)
	}
	major, err := serverMajor(txCtx, tx)
	if err != nil {
		return InitResult{}, err
	}
	if major < minimumPGMajor || major > maximumPGMajor {
		return InitResult{}, failure("CMDB_SERVER_UNSUPPORTED", "PostgreSQL major version must be between 14 and 18")
	}

	exists, err := schemaExists(txCtx, tx)
	if err != nil {
		return InitResult{}, err
	}
	applied := 0
	if !exists {
		for _, asset := range migrations {
			if major < asset.MinimumPGMajor {
				return InitResult{}, failure("CMDB_SERVER_UNSUPPORTED", "PostgreSQL server is older than a required migration")
			}
			if _, err := tx.Exec(txCtx, string(asset.SQL)); err != nil {
				return InitResult{}, sanitizeDatabaseFailure("CMDB_MIGRATION_FAILED", "migration", err)
			}
			if _, err := tx.Exec(txCtx, `
INSERT INTO pigsty_cmdb.schema_migration
       (version, name, checksum, tool_version, applied_at)
VALUES ($1, $2, $3, $4, clock_timestamp())`,
				asset.Version, asset.Name, asset.Checksum[:], toolVersion,
			); err != nil {
				return InitResult{}, sanitizeDatabaseFailure("CMDB_MIGRATION_FAILED", "migration ledger write", err)
			}
			applied++
		}
	} else if err := verifyMigrationLedger(txCtx, tx); err != nil {
		return InitResult{}, err
	}

	if err := validateNativeSchema(txCtx, tx); err != nil {
		return InitResult{}, err
	}
	if err := tx.Commit(txCtx); err != nil {
		return InitResult{}, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "transaction commit", err)
	}
	return InitResult{MigrationVersion: migrations[len(migrations)-1].Version, Applied: applied}, nil
}

func serverMajor(ctx context.Context, tx pgx.Tx) (int, error) {
	var versionNumber int
	if err := tx.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&versionNumber); err != nil {
		return 0, sanitizeDatabaseFailure("CMDB_SERVER_INVALID", "server version query", err)
	}
	return versionNumber / 10000, nil
}

func schemaExists(ctx context.Context, tx pgx.Tx) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT to_regnamespace('pigsty_cmdb') IS NOT NULL`).Scan(&exists); err != nil {
		return false, sanitizeDatabaseFailure("CMDB_SCHEMA_INVALID", "schema lookup", err)
	}
	return exists, nil
}

func verifyMigrationLedger(ctx context.Context, tx pgx.Tx) error {
	var nativeTable bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
      FROM pg_catalog.pg_class AS c
      JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
     WHERE n.nspname = 'pigsty_cmdb'
       AND c.relname = 'schema_migration'
       AND c.relkind = 'r'
)`).Scan(&nativeTable); err != nil {
		return sanitizeDatabaseFailure("CMDB_SCHEMA_INVALID", "migration ledger lookup", err)
	}
	if !nativeTable {
		return failure("CMDB_SCHEMA_FOREIGN", "existing pigsty_cmdb schema has no native migration ledger")
	}

	rows, err := tx.Query(ctx, `
SELECT version, name, checksum
  FROM pigsty_cmdb.schema_migration
 ORDER BY version`)
	if err != nil {
		return failure("CMDB_SCHEMA_FOREIGN", "existing pigsty_cmdb migration ledger is not native")
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var version int64
		var name string
		var checksum []byte
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return failure("CMDB_SCHEMA_FOREIGN", "existing pigsty_cmdb migration ledger is unreadable")
		}
		if version > int64(len(migrations)) {
			return failure("CMDB_MIGRATION_NEWER", "database schema is newer than this Pig version")
		}
		if index >= len(migrations) || version != int64(index+1) {
			return failure("CMDB_MIGRATION_DRIFT", "migration ledger versions are not contiguous")
		}
		expected := migrations[index]
		if name != expected.Name || !slices.Equal(checksum, expected.Checksum[:]) {
			return failure("CMDB_MIGRATION_DRIFT", "migration ledger checksum does not match embedded assets")
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return failure("CMDB_SCHEMA_FOREIGN", "existing pigsty_cmdb migration ledger is unreadable")
	}
	if index != len(migrations) {
		return failure("CMDB_MIGRATION_DRIFT", "migration ledger is missing an embedded version")
	}
	return nil
}

func advisoryKeyFromContract() int64 {
	digest := sha256.Sum256([]byte("pig.sty.cmdb/v1"))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}
