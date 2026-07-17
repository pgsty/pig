package cmdb

import (
	"crypto/sha256"
	"errors"
	"time"

	"pig/inventory"
)

type RevisionOrigin string
type RevisionSourceKind string

const (
	OriginStaticLoad RevisionOrigin     = "static_load"
	OriginRollback   RevisionOrigin     = "rollback"
	SourceStatic     RevisionSourceKind = "static"
	SourceRevision   RevisionSourceKind = "revision"
)

// Revision is an opaque verified record. Inventory-bearing fields are private
// and JSON marshaling is prohibited so a Result cannot accidentally contain a
// Snapshot body. Snapshot is an explicit internal-consumer boundary.
type Revision struct {
	revision          int64
	parentRevision    *int64
	mergeBaseRevision int64
	token             RevisionToken
	snapshot          *inventory.Snapshot
	origin            RevisionOrigin
	sourceKind        RevisionSourceKind
	sourceLocator     *string
	sourceBytesDigest []byte
	toolVersion       string
	createdAt         time.Time
}

func (r *Revision) MarshalJSON() ([]byte, error) {
	return nil, errors.New("CMDB revision bodies cannot be serialized")
}

func (r *Revision) Revision() int64 {
	if r == nil {
		return 0
	}
	return r.revision
}

func (r *Revision) ParentRevision() (int64, bool) {
	if r == nil || r.parentRevision == nil {
		return 0, false
	}
	return *r.parentRevision, true
}

func (r *Revision) MergeBaseRevision() int64 {
	if r == nil {
		return 0
	}
	return r.mergeBaseRevision
}

func (r *Revision) Token() RevisionToken {
	if r == nil {
		return RevisionToken{}
	}
	return r.token
}

func (r *Revision) Snapshot() *inventory.Snapshot {
	if r == nil {
		return nil
	}
	return r.snapshot
}

func (r *Revision) Origin() RevisionOrigin {
	if r == nil {
		return ""
	}
	return r.origin
}

func (r *Revision) SourceKind() RevisionSourceKind {
	if r == nil {
		return ""
	}
	return r.sourceKind
}

func (r *Revision) SourceLocator() (string, bool) {
	if r == nil || r.sourceLocator == nil {
		return "", false
	}
	return *r.sourceLocator, true
}

func (r *Revision) SourceBytesSHA256() ([sha256.Size]byte, bool) {
	var digest [sha256.Size]byte
	if r == nil || len(r.sourceBytesDigest) != sha256.Size {
		return digest, false
	}
	copy(digest[:], r.sourceBytesDigest)
	return digest, true
}

func (r *Revision) ToolVersion() string {
	if r == nil {
		return ""
	}
	return r.toolVersion
}

func (r *Revision) CreatedAt() time.Time {
	if r == nil {
		return time.Time{}
	}
	return r.createdAt
}

type VerifiedState struct {
	MigrationVersion int64          `json:"migration_version"`
	RevisionCount    int64          `json:"revision_count"`
	Head             *RevisionToken `json:"head,omitempty"`
}

type AppendResult struct {
	Changed       bool          `json:"changed"`
	Token         RevisionToken `json:"revision"`
	RevisionCount int64         `json:"revision_count"`
}

type StaticLoad struct {
	Snapshot          *inventory.Snapshot
	SourceLocator     string
	SourceBytesSHA256 [sha256.Size]byte
	ToolVersion       string
}
