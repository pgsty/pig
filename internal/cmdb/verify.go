package cmdb

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pig/inventory"
)

type verifiedChain struct {
	revisions []*Revision
	head      *RevisionToken
	updatedAt time.Time
}

// Verify checks the current migration, catalog, singleton, complete history,
// source lineage, and every re-canonicalized Snapshot digest.
func (s *Store) Verify(ctx context.Context) (VerifiedState, error) {
	chain, err := s.readVerifiedChain(ctx, true)
	if err != nil {
		return VerifiedState{}, err
	}
	return stateFromChain(chain), nil
}

// ReadHead returns the verified current opaque revision, or nil for an empty
// native store.
func (s *Store) ReadHead(ctx context.Context) (*Revision, error) {
	chain, err := s.readVerifiedChain(ctx, true)
	if err != nil || chain.head == nil {
		return nil, err
	}
	return cloneRevision(chain.revisions[len(chain.revisions)-1]), nil
}

// ReadRevision verifies the full chain and then selects an exact token. A
// matching revision number with a different digest is not accepted.
func (s *Store) ReadRevision(ctx context.Context, token RevisionToken) (*Revision, error) {
	if token.String() == "" {
		return nil, invalidRevisionToken()
	}
	chain, err := s.readVerifiedChain(ctx, true)
	if err != nil {
		return nil, err
	}
	if token.revision > int64(len(chain.revisions)) {
		return nil, failure("CMDB_REVISION_NOT_FOUND", "exact revision token was not found")
	}
	revision := chain.revisions[token.revision-1]
	if !revision.token.Equal(token) {
		return nil, failure("CMDB_REVISION_NOT_FOUND", "exact revision token was not found")
	}
	return cloneRevision(revision), nil
}

func (s *Store) readVerifiedChain(ctx context.Context, sharedLock bool) (*verifiedChain, error) {
	if s == nil || s.pool == nil {
		return nil, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "read transaction start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))
	if sharedLock {
		if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock_shared($1)`, CMDBAdvisoryKey); err != nil {
			return nil, sanitizeDatabaseFailure("CMDB_LOCK_FAILED", "shared lock", err)
		}
	}
	if err := verifyMigrationLedger(txCtx, tx); err != nil {
		return nil, err
	}
	if err := validateNativeSchema(txCtx, tx); err != nil {
		return nil, err
	}
	chain, err := readVerifiedChainTx(txCtx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(txCtx); err != nil {
		return nil, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "read transaction commit", err)
	}
	return chain, nil
}

func readVerifiedChainTx(ctx context.Context, tx pgx.Tx) (*verifiedChain, error) {
	stateRows, err := tx.Query(ctx, `
SELECT singleton, head_revision, updated_at
  FROM pigsty_cmdb.state
 ORDER BY singleton`)
	if err != nil {
		return nil, historyInvalid("state row")
	}
	var stateCount int
	var head pgtype.Int8
	var updatedAt time.Time
	for stateRows.Next() {
		stateCount++
		var singleton bool
		if err := stateRows.Scan(&singleton, &head, &updatedAt); err != nil || !singleton {
			stateRows.Close()
			return nil, historyInvalid("state row")
		}
	}
	stateErr := stateRows.Err()
	stateRows.Close()
	if stateErr != nil || stateCount != 1 {
		return nil, historyInvalid("state cardinality")
	}

	rows, err := tx.Query(ctx, `
SELECT revision, parent_revision, merge_base_revision,
       semantic_sha256, payload::text, origin, source_kind,
       source_locator, source_bytes_sha256, tool_version, created_at
  FROM pigsty_cmdb.snapshot
 ORDER BY revision`)
	if err != nil {
		return nil, historyInvalid("snapshot rows")
	}
	defer rows.Close()
	revisions := make([]*Revision, 0)
	for rows.Next() {
		var (
			revisionNumber int64
			parent         pgtype.Int8
			mergeBase      int64
			semanticDigest []byte
			payload        string
			origin         string
			sourceKind     string
			locator        pgtype.Text
			sourceDigest   []byte
			toolVersion    string
			createdAt      time.Time
		)
		if err := rows.Scan(
			&revisionNumber, &parent, &mergeBase, &semanticDigest, &payload,
			&origin, &sourceKind, &locator, &sourceDigest, &toolVersion, &createdAt,
		); err != nil {
			return nil, historyInvalid("snapshot row")
		}
		snapshot, err := inventory.ParseSnapshot([]byte(payload))
		if err != nil {
			return nil, historyInvalid("snapshot digest")
		}
		canonicalDigest := snapshot.SemanticSHA256()
		if len(semanticDigest) != sha256.Size || !slices.Equal(semanticDigest, canonicalDigest[:]) {
			return nil, historyInvalid("snapshot digest")
		}
		revision := &Revision{
			revision:          revisionNumber,
			mergeBaseRevision: mergeBase,
			token:             newRevisionToken(revisionNumber, snapshot.SemanticSHA256()),
			snapshot:          snapshot,
			origin:            RevisionOrigin(origin),
			sourceKind:        RevisionSourceKind(sourceKind),
			sourceBytesDigest: slices.Clone(sourceDigest),
			toolVersion:       toolVersion,
			createdAt:         createdAt,
		}
		if parent.Valid {
			value := parent.Int64
			revision.parentRevision = &value
		}
		if locator.Valid {
			value := locator.String
			revision.sourceLocator = &value
		}
		revisions = append(revisions, revision)
	}
	if rows.Err() != nil {
		return nil, historyInvalid("snapshot rows")
	}

	chain := &verifiedChain{revisions: revisions, updatedAt: updatedAt}
	if err := validateRevisionLineage(chain, head); err != nil {
		return nil, err
	}
	return chain, nil
}

func validateRevisionLineage(chain *verifiedChain, storedHead pgtype.Int8) error {
	if len(chain.revisions) == 0 {
		if storedHead.Valid {
			return historyInvalid("empty head")
		}
		return nil
	}
	if !storedHead.Valid || storedHead.Int64 != int64(len(chain.revisions)) {
		return historyInvalid("head maximum")
	}
	for index, revision := range chain.revisions {
		expectedNumber := int64(index + 1)
		if revision.revision != expectedNumber {
			return historyInvalid("contiguous revisions")
		}
		if index == 0 {
			if revision.parentRevision != nil {
				return historyInvalid("first parent")
			}
		} else if revision.parentRevision == nil || *revision.parentRevision != expectedNumber-1 {
			return historyInvalid("parent chain")
		}
		if revision.mergeBaseRevision < 1 || revision.mergeBaseRevision > revision.revision {
			return historyInvalid("merge base ancestry")
		}
		base := chain.revisions[revision.mergeBaseRevision-1]
		if base.origin != OriginStaticLoad || base.sourceKind != SourceStatic ||
			base.sourceLocator == nil || !validCanonicalStaticLocator(*base.sourceLocator) ||
			len(base.sourceBytesDigest) != sha256.Size {
			return historyInvalid("merge base provenance")
		}
		switch revision.origin {
		case OriginStaticLoad:
			if revision.sourceKind != SourceStatic || revision.sourceLocator == nil ||
				!validCanonicalStaticLocator(*revision.sourceLocator) ||
				len(revision.sourceBytesDigest) != sha256.Size || revision.mergeBaseRevision != revision.revision {
				return historyInvalid("static provenance")
			}
		case OriginRollback:
			if index == 0 || revision.sourceKind != SourceRevision || revision.sourceLocator == nil ||
				revision.sourceBytesDigest != nil ||
				revision.mergeBaseRevision != chain.revisions[index-1].mergeBaseRevision {
				return historyInvalid("rollback provenance")
			}
			target, err := ParseRevisionToken(*revision.sourceLocator)
			if err != nil || target.revision >= revision.revision || target.revision > int64(len(chain.revisions)) ||
				!chain.revisions[target.revision-1].token.Equal(target) {
				return historyInvalid("rollback target")
			}
		default:
			return historyInvalid("revision origin")
		}
	}
	head := chain.revisions[len(chain.revisions)-1].token
	chain.head = &head
	return nil
}

func validCanonicalStaticLocator(locator string) bool {
	return locator != "" && utf8.ValidString(locator) && !strings.ContainsRune(locator, 0) &&
		filepath.IsAbs(locator) && filepath.Clean(locator) == locator
}

func historyInvalid(part string) error {
	return failure("CMDB_HISTORY_INVALID", "native revision "+part+" is invalid")
}

func stateFromChain(chain *verifiedChain) VerifiedState {
	state := VerifiedState{MigrationVersion: migrations[len(migrations)-1].Version, RevisionCount: int64(len(chain.revisions))}
	if chain.head != nil {
		copy := *chain.head
		state.Head = &copy
	}
	return state
}

func cloneRevision(source *Revision) *Revision {
	if source == nil {
		return nil
	}
	clone := *source
	if source.parentRevision != nil {
		value := *source.parentRevision
		clone.parentRevision = &value
	}
	if source.sourceLocator != nil {
		value := *source.sourceLocator
		clone.sourceLocator = &value
	}
	clone.sourceBytesDigest = slices.Clone(source.sourceBytesDigest)
	return &clone
}
