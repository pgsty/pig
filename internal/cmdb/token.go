package cmdb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
)

const revisionTokenPrefix = "cmdb-v1:"

// RevisionToken is the exact, value-free optimistic-concurrency identity.
// Its fields remain private so callers cannot accidentally construct a token
// with a revision that is not bound to its semantic digest.
type RevisionToken struct {
	revision int64
	digest   [sha256.Size]byte
}

func newRevisionToken(revision int64, digest [sha256.Size]byte) RevisionToken {
	return RevisionToken{revision: revision, digest: digest}
}

// ParseRevisionToken accepts only the exact lowercase CMDB v1 grammar.
func ParseRevisionToken(value string) (RevisionToken, error) {
	if !strings.HasPrefix(value, revisionTokenPrefix) {
		return RevisionToken{}, invalidRevisionToken()
	}
	rest := strings.TrimPrefix(value, revisionTokenPrefix)
	revisionText, digestText, ok := strings.Cut(rest, ":sha256:")
	if !ok || revisionText == "" || digestText == "" || strings.Contains(digestText, ":") ||
		(revisionText[0] == '0' && len(revisionText) > 1) {
		return RevisionToken{}, invalidRevisionToken()
	}
	revision, err := strconv.ParseInt(revisionText, 10, 64)
	if err != nil || revision <= 0 || strconv.FormatInt(revision, 10) != revisionText || len(digestText) != 64 {
		return RevisionToken{}, invalidRevisionToken()
	}
	digestBytes, err := hex.DecodeString(digestText)
	if err != nil || hex.EncodeToString(digestBytes) != digestText {
		return RevisionToken{}, invalidRevisionToken()
	}
	var digest [sha256.Size]byte
	copy(digest[:], digestBytes)
	return newRevisionToken(revision, digest), nil
}

func invalidRevisionToken() error {
	return failure("CMDB_REVISION_TOKEN_INVALID", "revision token must match the exact CMDB v1 grammar")
}

func (t RevisionToken) Revision() int64 { return t.revision }

func (t RevisionToken) Digest() string { return hex.EncodeToString(t.digest[:]) }

func (t RevisionToken) String() string {
	if t.revision <= 0 {
		return ""
	}
	return revisionTokenPrefix + strconv.FormatInt(t.revision, 10) + ":sha256:" + t.Digest()
}

func (t RevisionToken) Equal(other RevisionToken) bool {
	return t.revision == other.revision && t.digest == other.digest && t.revision > 0
}

func (t RevisionToken) MarshalText() ([]byte, error) {
	if t.String() == "" {
		return nil, invalidRevisionToken()
	}
	return []byte(t.String()), nil
}

func (t RevisionToken) MarshalJSON() ([]byte, error) {
	if t.String() == "" {
		return nil, invalidRevisionToken()
	}
	return json.Marshal(t.String())
}
