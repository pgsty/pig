package cmdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type schemaPresence string

const (
	schemaAbsent schemaPresence = "absent"
	schemaNative schemaPresence = "native"
)

// ResetFingerprint binds a destructive plan to either exact absence or the
// full migration/state/revision byte identity observed by InspectReset.
type ResetFingerprint struct {
	presence schemaPresence
	digest   [sha256.Size]byte
}

func (f ResetFingerprint) String() string {
	if f.presence != schemaAbsent && f.presence != schemaNative {
		return ""
	}
	return "cmdb-reset-v1:" + string(f.presence) + ":sha256:" + hex.EncodeToString(f.digest[:])
}

func (f ResetFingerprint) Equal(other ResetFingerprint) bool {
	return f.presence == other.presence && f.digest == other.digest && f.String() != ""
}

func (f ResetFingerprint) MarshalJSON() ([]byte, error) {
	if f.String() == "" {
		return nil, failure("CMDB_RESET_INSPECTION_INVALID", "reset fingerprint is invalid")
	}
	return json.Marshal(f.String())
}

type ResetInspection struct {
	Absent           bool             `json:"absent"`
	MigrationVersion int64            `json:"migration_version"`
	MigrationCount   int              `json:"migration_count"`
	RevisionCount    int64            `json:"revision_count"`
	Head             *RevisionToken   `json:"head,omitempty"`
	Fingerprint      ResetFingerprint `json:"fingerprint"`
}

type resetData struct {
	presence    schemaPresence
	ledger      []migrationRecord
	chain       *verifiedChain
	fingerprint ResetFingerprint
}

// InspectReset proves exact absence or a fully native readable store under a
// shared lock. Foreign and malformed schemas never produce a fingerprint.
func (s *Store) InspectReset(ctx context.Context) (ResetInspection, error) {
	if s == nil || s.pool == nil {
		return ResetInspection{}, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ResetInspection{}, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "reset inspection start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))
	if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock_shared($1)`, CMDBAdvisoryKey); err != nil {
		return ResetInspection{}, sanitizeDatabaseFailure("CMDB_LOCK_FAILED", "reset inspection lock", err)
	}
	data, err := inspectResetTx(txCtx, tx)
	if err != nil {
		return ResetInspection{}, err
	}
	if err := tx.Commit(txCtx); err != nil {
		return ResetInspection{}, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "reset inspection commit", err)
	}
	return inspectionFromResetData(data), nil
}

func inspectionFromResetData(data *resetData) ResetInspection {
	inspection := ResetInspection{
		Absent:      data.presence == schemaAbsent,
		Fingerprint: data.fingerprint,
	}
	if data.presence == schemaNative {
		state := stateFromChain(data.chain)
		inspection.MigrationVersion = state.MigrationVersion
		inspection.MigrationCount = len(data.ledger)
		inspection.RevisionCount = state.RevisionCount
		inspection.Head = state.Head
	}
	return inspection
}

func inspectResetTx(ctx context.Context, tx pgx.Tx) (*resetData, error) {
	exists, err := schemaExists(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return &resetData{presence: schemaAbsent, fingerprint: absentResetFingerprint()}, nil
	}
	if err := verifyMigrationLedger(ctx, tx); err != nil {
		return nil, err
	}
	if err := validateNativeSchema(ctx, tx); err != nil {
		return nil, err
	}
	chain, err := readVerifiedChainTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	ledger, err := readMigrationRecordsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	fingerprint, err := nativeStateFingerprint(ledger, chain)
	if err != nil {
		return nil, err
	}
	return &resetData{presence: schemaNative, ledger: ledger, chain: chain, fingerprint: fingerprint}, nil
}

func absentResetFingerprint() ResetFingerprint {
	digest := sha256.Sum256([]byte("pig.sty.cmdb.state-fingerprint/v1\nabsent\n"))
	return ResetFingerprint{presence: schemaAbsent, digest: digest}
}

type ResetRequest struct {
	Expected    ResetFingerprint
	BackupPath  string
	Restore     *BundleInspection
	ToolVersion string

	afterRestorePreflight func() error
	afterBackup           func() error
}

type ResetResult struct {
	State        VerifiedState `json:"state"`
	BackupPath   string        `json:"backup_path,omitempty"`
	BackupSHA256 string        `json:"backup_sha256,omitempty"`
	Restored     bool          `json:"restored"`
}

// Reset rechecks an exact inspection under the exclusive advisory lock,
// creates and verifies a mandatory private backup for native state, drops only
// pigsty_cmdb, reapplies v1, and optionally restores one twice-verified bundle.
func (s *Store) Reset(ctx context.Context, request ResetRequest) (ResetResult, error) {
	if s == nil || s.pool == nil {
		return ResetResult{}, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	if request.Expected.String() == "" {
		return ResetResult{}, failure("CMDB_RESET_INSPECTION_INVALID", "reset requires an exact prior inspection")
	}
	if err := validateToolVersion(request.ToolVersion); err != nil {
		return ResetResult{}, err
	}
	var restorePreflight *bundleData
	if request.Restore != nil {
		var err error
		restorePreflight, err = readBundle(request.Restore.path)
		if err != nil {
			return ResetResult{}, err
		}
		if !bundleMatchesInspection(restorePreflight, *request.Restore) {
			return ResetResult{}, failure("CMDB_BACKUP_CHANGED", "restore bundle no longer matches its inspection")
		}
	}
	if request.afterRestorePreflight != nil {
		if err := request.afterRestorePreflight(); err != nil {
			return ResetResult{}, failure("CMDB_TRANSACTION_FAILED", "restore preflight hook failed")
		}
	}

	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ResetResult{}, classifyResetFailure("transaction start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))
	if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock($1)`, CMDBAdvisoryKey); err != nil {
		return ResetResult{}, classifyResetFailure("lock", err)
	}
	major, err := serverMajor(txCtx, tx)
	if err != nil {
		return ResetResult{}, err
	}
	if major < minimumPGMajor || major > maximumPGMajor {
		return ResetResult{}, failure("CMDB_SERVER_UNSUPPORTED", "PostgreSQL major version must be between 14 and 18")
	}
	current, err := inspectResetTx(txCtx, tx)
	if err != nil {
		return ResetResult{}, err
	}
	if !current.fingerprint.Equal(request.Expected) {
		return ResetResult{}, failure("CMDB_RESET_CONFLICT", "database state changed after reset inspection")
	}

	var restoreLocked *bundleData
	if request.Restore != nil {
		restoreLocked, err = readBundle(request.Restore.path)
		if err != nil {
			return ResetResult{}, err
		}
		if !bundleMatchesInspection(restoreLocked, *request.Restore) ||
			restorePreflight.fileDigest != restoreLocked.fileDigest {
			return ResetResult{}, failure("CMDB_BACKUP_CHANGED", "restore bundle changed before reset lock")
		}
	}

	result := ResetResult{Restored: restoreLocked != nil}
	if current.presence == schemaNative {
		if strings.TrimSpace(request.BackupPath) == "" {
			return ResetResult{}, failure("CMDB_BACKUP_REQUIRED", "reset of native state requires a new private backup path")
		}
		backup, err := writePrivateBundle(request.BackupPath, current.ledger, current.chain)
		if err != nil {
			return ResetResult{}, err
		}
		if !backup.fingerprint.Equal(current.fingerprint) {
			return ResetResult{}, bundleInvalid("state verification")
		}
		result.BackupPath = backup.path
		result.BackupSHA256 = hex.EncodeToString(backup.fileDigest[:])
		if request.afterBackup != nil {
			if err := request.afterBackup(); err != nil {
				return ResetResult{}, failure("CMDB_TRANSACTION_FAILED", "injected post-backup failure")
			}
		}
	} else if strings.TrimSpace(request.BackupPath) != "" {
		return ResetResult{}, failure("CMDB_PARAMETER_INVALID", "absent schema reset must not claim a pre-reset backup")
	}

	if current.presence == schemaNative {
		if _, err := tx.Exec(txCtx, `DROP SCHEMA pigsty_cmdb CASCADE`); err != nil {
			return ResetResult{}, classifyResetFailure("schema drop", err)
		}
	}
	if err := applyFreshSchemaTx(txCtx, tx, major, request.ToolVersion, restoreLocked); err != nil {
		return ResetResult{}, err
	}
	if err := verifyMigrationLedger(txCtx, tx); err != nil {
		return ResetResult{}, err
	}
	if err := validateNativeSchema(txCtx, tx); err != nil {
		return ResetResult{}, err
	}
	updatedChain, err := readVerifiedChainTx(txCtx, tx)
	if err != nil {
		return ResetResult{}, err
	}
	updatedLedger, err := readMigrationRecordsTx(txCtx, tx)
	if err != nil {
		return ResetResult{}, err
	}
	updatedFingerprint, err := nativeStateFingerprint(updatedLedger, updatedChain)
	if err != nil {
		return ResetResult{}, err
	}
	if restoreLocked != nil && !updatedFingerprint.Equal(restoreLocked.fingerprint) {
		return ResetResult{}, historyInvalid("restored state fingerprint")
	}
	result.State = stateFromChain(updatedChain)
	if err := tx.Commit(txCtx); err != nil {
		return ResetResult{}, classifyResetFailure("transaction commit", err)
	}
	committed, err := s.InspectReset(ctx)
	if err != nil {
		return ResetResult{}, err
	}
	if committed.Absent || !committed.Fingerprint.Equal(updatedFingerprint) {
		return ResetResult{}, failure("CMDB_RESET_CONFLICT", "database state changed immediately after reset commit")
	}
	result.State = VerifiedState{
		MigrationVersion: committed.MigrationVersion,
		RevisionCount:    committed.RevisionCount,
		Head:             committed.Head,
	}
	return result, nil
}

func applyFreshSchemaTx(ctx context.Context, tx pgx.Tx, major int, toolVersion string, restore *bundleData) error {
	for _, asset := range migrations {
		if major < asset.MinimumPGMajor {
			return failure("CMDB_SERVER_UNSUPPORTED", "PostgreSQL server is older than a required migration")
		}
		if _, err := tx.Exec(ctx, string(asset.SQL)); err != nil {
			return classifyResetFailure("migration DDL", err)
		}
	}
	if restore == nil {
		for _, asset := range migrations {
			if _, err := tx.Exec(ctx, `
INSERT INTO pigsty_cmdb.schema_migration
       (version, name, checksum, tool_version, applied_at)
VALUES ($1, $2, $3, $4, clock_timestamp())`,
				asset.Version, asset.Name, asset.Checksum[:], toolVersion,
			); err != nil {
				return classifyResetFailure("migration ledger", err)
			}
		}
		return nil
	}
	for _, record := range restore.ledger {
		if _, err := tx.Exec(ctx, `
INSERT INTO pigsty_cmdb.schema_migration
       (version, name, checksum, tool_version, applied_at)
VALUES ($1, $2, $3, $4, $5)`,
			record.Version, record.Name, record.Checksum, record.ToolVersion, record.AppliedAt,
		); err != nil {
			return classifyResetFailure("restored migration ledger", err)
		}
	}
	if err := restoreRevisionChainTx(ctx, tx, restore.chain); err != nil {
		return err
	}
	return nil
}

func restoreRevisionChainTx(ctx context.Context, tx pgx.Tx, chain *verifiedChain) error {
	for _, revision := range chain.revisions {
		semanticDigest := revision.snapshot.SemanticSHA256()
		if _, err := tx.Exec(ctx, `
INSERT INTO pigsty_cmdb.snapshot
       (revision, parent_revision, merge_base_revision, semantic_sha256,
        payload, origin, source_kind, source_locator, source_bytes_sha256,
        tool_version, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11)`,
			revision.revision, revision.parentRevision, revision.mergeBaseRevision,
			semanticDigest[:], string(revision.snapshot.CanonicalBytes()),
			string(revision.origin), string(revision.sourceKind), revision.sourceLocator,
			revision.sourceBytesDigest, revision.toolVersion, revision.createdAt,
		); err != nil {
			return classifyResetFailure("restored revision", err)
		}
	}
	var head *int64
	if chain.head != nil {
		value := chain.head.revision
		head = &value
	}
	command, err := tx.Exec(ctx, `
UPDATE pigsty_cmdb.state
   SET head_revision = $1, updated_at = $2
 WHERE singleton`, head, chain.updatedAt)
	if err != nil || command.RowsAffected() != 1 {
		if err == nil {
			err = errors.New("state cardinality")
		}
		return classifyResetFailure("restored state", err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		return classifyResetFailure("restored constraints", err)
	}
	return nil
}

func bundleMatchesInspection(data *bundleData, inspection BundleInspection) bool {
	return data != nil && inspection.path != "" && data.path == inspection.path &&
		data.fileDigest == inspection.fileDigest && data.fingerprint.Equal(inspection.fingerprint)
}

func classifyResetFailure(action string, err error) error {
	if err == nil {
		return nil
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "40001" {
		return failure("CMDB_RESET_CONFLICT", "concurrent serializable change invalidated reset")
	}
	return sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", action, err)
}
