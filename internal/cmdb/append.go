package cmdb

import (
	"context"
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pig/inventory"
)

type appendKind int

const (
	appendStatic appendKind = iota + 1
	appendRollback
)

type appendRequest struct {
	kind         appendKind
	expectedHead *RevisionToken
	static       StaticLoad
	target       RevisionToken
	afterInsert  func() error
}

// AppendStatic creates one immutable static merge base under exact CAS, or
// returns a no-op when the verified candidate already equals the head.
func (s *Store) AppendStatic(ctx context.Context, expectedHead *RevisionToken, candidate StaticLoad) (AppendResult, error) {
	return s.appendRevision(ctx, appendRequest{kind: appendStatic, expectedHead: expectedHead, static: candidate})
}

// AppendRollback copies an exact historical Snapshot into a new child. It
// never moves the head backward and inherits the current head's merge base.
func (s *Store) AppendRollback(ctx context.Context, expectedHead RevisionToken, target RevisionToken, toolVersion string) (AppendResult, error) {
	return s.appendRevision(ctx, appendRequest{
		kind:         appendRollback,
		expectedHead: &expectedHead,
		target:       target,
		static:       StaticLoad{ToolVersion: toolVersion},
	})
}

func (s *Store) appendRevision(ctx context.Context, request appendRequest) (AppendResult, error) {
	if s == nil || s.pool == nil {
		return AppendResult{}, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	if request.expectedHead != nil && request.expectedHead.String() == "" {
		return AppendResult{}, invalidRevisionToken()
	}
	if err := validateToolVersion(request.static.ToolVersion); err != nil {
		return AppendResult{}, err
	}
	if request.kind == appendStatic {
		if request.static.Snapshot == nil || !validCanonicalStaticLocator(request.static.SourceLocator) {
			return AppendResult{}, failure("CMDB_PARAMETER_INVALID", "static Snapshot provenance is invalid")
		}
		parsed, err := inventory.ParseSnapshot(request.static.Snapshot.CanonicalBytes())
		if err != nil || !parsed.Equal(request.static.Snapshot) {
			return AppendResult{}, failure("CMDB_SNAPSHOT_INVALID", "static Snapshot is not canonical CMDB v1 data")
		}
	} else if request.kind == appendRollback {
		if request.target.String() == "" {
			return AppendResult{}, invalidRevisionToken()
		}
	} else {
		return AppendResult{}, failure("CMDB_PARAMETER_INVALID", "revision append kind is invalid")
	}

	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return AppendResult{}, classifyWriteFailure("transaction start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))
	if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock($1)`, CMDBAdvisoryKey); err != nil {
		return AppendResult{}, classifyWriteFailure("lock", err)
	}
	if err := verifyMigrationLedger(txCtx, tx); err != nil {
		return AppendResult{}, err
	}
	if err := validateNativeSchema(txCtx, tx); err != nil {
		return AppendResult{}, err
	}
	var lockedHead *int64
	if err := tx.QueryRow(txCtx, `
SELECT head_revision
  FROM pigsty_cmdb.state
 WHERE singleton
 FOR UPDATE`).Scan(&lockedHead); err != nil {
		return AppendResult{}, classifyWriteFailure("state lock", err)
	}
	chain, err := readVerifiedChainTx(txCtx, tx)
	if err != nil {
		return AppendResult{}, err
	}
	if !expectedHeadMatches(chain.head, request.expectedHead) {
		return AppendResult{}, failure("CMDB_REVISION_CONFLICT", "planned head no longer matches the exact current token")
	}
	if (lockedHead == nil) != (chain.head == nil) || (lockedHead != nil && *lockedHead != chain.head.revision) {
		return AppendResult{}, historyInvalid("locked head")
	}

	var candidate *inventory.Snapshot
	var origin RevisionOrigin
	var sourceKind RevisionSourceKind
	var sourceLocator string
	var sourceBytes []byte
	var mergeBase int64
	if request.kind == appendStatic {
		candidate = request.static.Snapshot
		origin = OriginStaticLoad
		sourceKind = SourceStatic
		sourceLocator = request.static.SourceLocator
		sourceBytes = slices.Clone(request.static.SourceBytesSHA256[:])
		mergeBase = int64(len(chain.revisions) + 1)
	} else {
		if request.target.revision > int64(len(chain.revisions)) ||
			!chain.revisions[request.target.revision-1].token.Equal(request.target) {
			return AppendResult{}, failure("CMDB_REVISION_NOT_FOUND", "exact rollback target was not found")
		}
		candidate = chain.revisions[request.target.revision-1].snapshot
		origin = OriginRollback
		sourceKind = SourceRevision
		sourceLocator = request.target.String()
		mergeBase = chain.revisions[len(chain.revisions)-1].mergeBaseRevision
	}
	if chain.head != nil && chain.revisions[len(chain.revisions)-1].snapshot.Equal(candidate) {
		if err := tx.Commit(txCtx); err != nil {
			return AppendResult{}, classifyWriteFailure("no-op commit", err)
		}
		return AppendResult{Changed: false, Token: *chain.head, RevisionCount: int64(len(chain.revisions))}, nil
	}

	next := int64(len(chain.revisions) + 1)
	var parent *int64
	if chain.head != nil {
		value := chain.head.revision
		parent = &value
	}
	semanticDigest := candidate.SemanticSHA256()
	if _, err := tx.Exec(txCtx, `
INSERT INTO pigsty_cmdb.snapshot
       (revision, parent_revision, merge_base_revision, semantic_sha256,
        payload, origin, source_kind, source_locator, source_bytes_sha256,
        tool_version, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, clock_timestamp())`,
		next, parent, mergeBase, semanticDigest[:], string(candidate.CanonicalBytes()),
		string(origin), string(sourceKind), sourceLocator, sourceBytes,
		request.static.ToolVersion,
	); err != nil {
		return AppendResult{}, classifyWriteFailure("revision insert", err)
	}
	if request.afterInsert != nil {
		if err := request.afterInsert(); err != nil {
			return AppendResult{}, failure("CMDB_TRANSACTION_FAILED", "injected post-insert failure")
		}
	}
	command, err := tx.Exec(txCtx, `
UPDATE pigsty_cmdb.state
   SET head_revision = $1, updated_at = clock_timestamp()
 WHERE singleton
   AND head_revision IS NOT DISTINCT FROM $2`, next, parent)
	if err != nil {
		return AppendResult{}, classifyWriteFailure("head update", err)
	}
	if command.RowsAffected() != 1 {
		return AppendResult{}, failure("CMDB_REVISION_CONFLICT", "head changed before revision commit")
	}
	updated, err := readVerifiedChainTx(txCtx, tx)
	if err != nil {
		return AppendResult{}, err
	}
	token := newRevisionToken(next, semanticDigest)
	if updated.head == nil || !updated.head.Equal(token) {
		return AppendResult{}, historyInvalid("committed head")
	}
	if err := tx.Commit(txCtx); err != nil {
		return AppendResult{}, classifyWriteFailure("transaction commit", err)
	}
	committed, err := s.ReadRevision(ctx, token)
	if err != nil || committed == nil || !committed.token.Equal(token) {
		if err != nil {
			return AppendResult{}, err
		}
		return AppendResult{}, historyInvalid("committed revision")
	}
	return AppendResult{Changed: true, Token: token, RevisionCount: int64(len(updated.revisions))}, nil
}

func expectedHeadMatches(actual, expected *RevisionToken) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return actual.Equal(*expected)
}

func validateToolVersion(value string) error {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) {
		return failure("CMDB_PARAMETER_INVALID", "tool version is invalid")
	}
	return nil
}

func classifyWriteFailure(action string, err error) error {
	if err == nil {
		return nil
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "40001" {
		return failure("CMDB_REVISION_CONFLICT", "concurrent serializable write changed the planned head")
	}
	return sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", action, err)
}
