package cmdb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"pig/inventory"
)

const (
	DumpKind            = "pig.sty.cmdb.dump/v1"
	generatedMarkerKind = "pig.inventory.generated/v1"
	mergeMarkerKind     = "pig.inventory.merge/v1"
	maxMergeSourceSize  = 16 << 20
	cmdbMarkerPrefix    = "# pig-sty-cmdb:"
)

type DumpRequest struct {
	OutputPath string
	Revision   *RevisionToken
	Merge      bool
	ExpectBase *RevisionToken

	beforeSourceRecheck     func() error
	beforeDestinationCommit func() error
}

type DumpResult struct {
	Kind          string                       `json:"kind" yaml:"kind"`
	Revision      RevisionToken                `json:"revision" yaml:"revision"`
	Base          *RevisionToken               `json:"base,omitempty" yaml:"base,omitempty"`
	RevisionCount int64                        `json:"revision_count" yaml:"revision_count"`
	NodeCount     int                          `json:"node_count" yaml:"node_count"`
	Merge         bool                         `json:"merge" yaml:"merge"`
	OutputPath    string                       `json:"output_path,omitempty" yaml:"output_path,omitempty"`
	OutputSHA256  string                       `json:"output_sha256,omitempty" yaml:"output_sha256,omitempty"`
	SourceSHA256  string                       `json:"source_sha256,omitempty" yaml:"source_sha256,omitempty"`
	Changes       []inventory.SnapshotChange   `json:"changes,omitempty" yaml:"changes,omitempty"`
	Conflicts     []inventory.SnapshotConflict `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
}

// Dump writes a new owner-only canonical or lossless-merge file while keeping
// every Inventory-bearing byte outside the returned safe metadata.
func (s *Store) Dump(ctx context.Context, request DumpRequest) (DumpResult, error) {
	if s == nil || s.pool == nil {
		return DumpResult{}, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	outputPath, err := preflightNewPrivateOutput(request.OutputPath)
	if err != nil {
		return DumpResult{}, err
	}
	if request.Revision != nil && request.Revision.String() == "" {
		return DumpResult{}, invalidRevisionToken()
	}
	if request.ExpectBase != nil && request.ExpectBase.String() == "" {
		return DumpResult{}, invalidRevisionToken()
	}
	if request.Merge && request.Revision != nil {
		return DumpResult{}, failure("CMDB_PARAMETER_INVALID", "merge dump cannot select a historical revision")
	}
	if !request.Merge && request.ExpectBase != nil {
		return DumpResult{}, failure("CMDB_PARAMETER_INVALID", "expected base is valid only for merge dump")
	}

	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return DumpResult{}, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "dump transaction start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))
	if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock_shared($1)`, CMDBAdvisoryKey); err != nil {
		return DumpResult{}, sanitizeDatabaseFailure("CMDB_LOCK_FAILED", "dump shared lock", err)
	}
	if err := verifyMigrationLedger(txCtx, tx); err != nil {
		return DumpResult{}, err
	}
	if err := validateNativeSchema(txCtx, tx); err != nil {
		return DumpResult{}, err
	}
	chain, err := readVerifiedChainTx(txCtx, tx)
	if err != nil {
		return DumpResult{}, err
	}

	if request.Merge {
		return s.dumpMergeLocked(txCtx, tx, chain, outputPath, request)
	}
	return s.dumpPlainLocked(txCtx, tx, chain, outputPath, request.Revision)
}

func (s *Store) dumpPlainLocked(ctx context.Context, tx pgx.Tx, chain *verifiedChain, outputPath string, selected *RevisionToken) (DumpResult, error) {
	revision, err := selectDumpRevision(chain, selected)
	if err != nil {
		return DumpResult{}, err
	}
	yamlBody, err := revision.snapshot.CanonicalYAML()
	if err != nil {
		return DumpResult{}, failure("CMDB_DUMP_FAILED", "canonical Inventory output could not be generated")
	}
	marker := cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=" + revision.token.String()
	body := append([]byte(marker+"\n"), yamlBody...)
	path, digest, err := writeNewPrivateOutput(outputPath, body)
	if err != nil {
		return DumpResult{}, err
	}
	result := DumpResult{
		Kind: DumpKind, Revision: revision.token, RevisionCount: int64(len(chain.revisions)),
		NodeCount: revision.snapshot.NodeCount(), OutputPath: path,
		OutputSHA256: hex.EncodeToString(digest[:]),
	}
	if err := tx.Commit(ctx); err != nil {
		return result, failure("CMDB_DUMP_PARTIAL", "dump output was committed but the database read transaction did not close cleanly")
	}
	return result, nil
}

func (s *Store) dumpMergeLocked(ctx context.Context, tx pgx.Tx, chain *verifiedChain, outputPath string, request DumpRequest) (DumpResult, error) {
	if chain.head == nil {
		return DumpResult{}, failure("CMDB_REVISION_NOT_FOUND", "native store has no revision to merge")
	}
	head := chain.revisions[len(chain.revisions)-1]
	base := chain.revisions[head.mergeBaseRevision-1]
	if request.ExpectBase != nil && !base.token.Equal(*request.ExpectBase) {
		return DumpResult{}, failure("CMDB_BASE_MISMATCH", "expected merge base does not match native provenance")
	}
	if base.origin != OriginStaticLoad || base.sourceKind != SourceStatic || base.sourceLocator == nil ||
		!validCanonicalStaticLocator(*base.sourceLocator) || len(base.sourceBytesDigest) != sha256.Size {
		return DumpResult{}, failure("CMDB_BASE_MISMATCH", "native merge base provenance is incomplete")
	}
	if outputPath == *base.sourceLocator {
		return DumpResult{}, failure("CMDB_PARAMETER_INVALID", "merge output must be distinct from the recorded static source")
	}
	source, err := readMergeSource(*base.sourceLocator)
	if err != nil {
		return DumpResult{}, err
	}
	working, err := parseValidatedMergeSource(*base.sourceLocator, source)
	if err != nil {
		return DumpResult{}, err
	}
	merged, err := inventory.MergeSnapshots(base.snapshot, working, head.snapshot)
	if err != nil {
		return DumpResult{}, classifyMergeFailure(err)
	}
	sourceDigest := sha256.Sum256(source)
	baseToken := base.token
	result := DumpResult{
		Kind: DumpKind, Revision: head.token, Base: &baseToken,
		RevisionCount: int64(len(chain.revisions)), Merge: true,
		SourceSHA256: hex.EncodeToString(sourceDigest[:]),
	}
	if !merged.Clean() {
		result.Conflicts = merged.Conflicts
		return result, failure("CMDB_MERGE_CONFLICT", "three-way merge has declaration conflicts")
	}
	result.Changes = merged.Changes
	candidate := merged.Document().Bytes()
	marker := cmdbMarkerPrefix + " kind=" + mergeMarkerKind + " base=" + base.token.String() +
		" head=" + head.token.String() + " source-sha256=" + result.SourceSHA256
	marked, err := replaceLeadingCMDBMarker(candidate, marker)
	if err != nil {
		return DumpResult{}, err
	}
	validated, err := parseValidatedMergeSource(outputPath, marked)
	if err != nil {
		if IsCode(err, "CMDB_SNAPSHOT_UNREPRESENTABLE") {
			return DumpResult{}, err
		}
		return DumpResult{}, failure("CMDB_MERGE_INVALID", "merged Inventory candidate failed validation")
	}
	validatedSnapshot, err := inventory.SnapshotFromDocument(validated)
	if err != nil {
		return DumpResult{}, failure("CMDB_SNAPSHOT_UNREPRESENTABLE", "merged Inventory candidate is not representable")
	}
	result.NodeCount = validatedSnapshot.NodeCount()
	if request.beforeSourceRecheck != nil {
		if err := request.beforeSourceRecheck(); err != nil {
			return DumpResult{}, failure("CMDB_DUMP_FAILED", "merge source recheck hook failed")
		}
	}
	currentSource, err := readMergeSource(*base.sourceLocator)
	if err != nil || !bytes.Equal(currentSource, source) {
		return DumpResult{}, failure("CMDB_SOURCE_CHANGED", "recorded static source changed during merge dump")
	}
	if request.beforeDestinationCommit != nil {
		if err := request.beforeDestinationCommit(); err != nil {
			return DumpResult{}, failure("CMDB_DUMP_FAILED", "merge destination hook failed")
		}
	}
	path, digest, err := writeNewPrivateOutput(outputPath, marked)
	if err != nil {
		return DumpResult{}, err
	}
	result.OutputPath = path
	result.OutputSHA256 = hex.EncodeToString(digest[:])
	if err := tx.Commit(ctx); err != nil {
		return result, failure("CMDB_DUMP_PARTIAL", "merge output was committed but the database read transaction did not close cleanly")
	}
	return result, nil
}

func selectDumpRevision(chain *verifiedChain, selected *RevisionToken) (*Revision, error) {
	if len(chain.revisions) == 0 || chain.head == nil {
		return nil, failure("CMDB_REVISION_NOT_FOUND", "native store has no revision to dump")
	}
	if selected == nil {
		return chain.revisions[len(chain.revisions)-1], nil
	}
	if selected.revision > int64(len(chain.revisions)) ||
		!chain.revisions[selected.revision-1].token.Equal(*selected) {
		return nil, failure("CMDB_REVISION_NOT_FOUND", "exact revision token was not found")
	}
	return chain.revisions[selected.revision-1], nil
}

func parseValidatedMergeSource(name string, source []byte) (*inventory.Document, error) {
	document, err := inventory.Parse(name, source)
	if err != nil {
		return nil, failure("CMDB_SOURCE_INVALID", "recorded static source YAML is invalid")
	}
	diagnostics := document.Validate(
		inventory.YAMLRules, inventory.AnsibleRules, inventory.PigstyRules, inventory.CompleteInventoryRules,
	)
	if inventory.HasErrors(diagnostics) {
		return nil, failure("CMDB_SOURCE_INVALID", "recorded static source validation failed")
	}
	if _, err := inventory.SnapshotFromDocument(document); err != nil {
		return nil, failure("CMDB_SNAPSHOT_UNREPRESENTABLE", "recorded static source is not representable")
	}
	return document, nil
}

func classifyMergeFailure(err error) error {
	var validation *inventory.ValidationError
	if errors.As(err, &validation) {
		return failure("CMDB_SOURCE_INVALID", "recorded static source validation failed")
	}
	var snapshot *inventory.SnapshotError
	if errors.As(err, &snapshot) {
		return failure("CMDB_SNAPSHOT_UNREPRESENTABLE", "recorded static source is not representable")
	}
	return failure("CMDB_MERGE_INVALID", "three-way merge could not be applied losslessly")
}

func readMergeSource(path string) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path || !pathIsSymlinkFree(path) {
		return nil, failure("CMDB_BASE_MISMATCH", "recorded static source path no longer matches native provenance")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o111 != 0 {
		return nil, failure("CMDB_BASE_MISMATCH", "recorded static source is unavailable or unsupported")
	}
	if before.Size() > maxMergeSourceSize {
		return nil, failure("CMDB_SNAPSHOT_UNREPRESENTABLE", "recorded static source exceeds the CMDB v1 bound")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, failure("CMDB_BASE_MISMATCH", "recorded static source cannot be opened")
	}
	defer file.Close()
	after, err := file.Stat()
	pathAfter, pathErr := os.Lstat(path)
	if err != nil || pathErr != nil || !sameSourceRevision(before, after) || !sameSourceRevision(after, pathAfter) {
		return nil, failure("CMDB_SOURCE_CHANGED", "recorded static source changed during inspection")
	}
	source, err := io.ReadAll(io.LimitReader(file, maxMergeSourceSize+1))
	if err != nil {
		return nil, failure("CMDB_BASE_MISMATCH", "recorded static source cannot be read within bounds")
	}
	if len(source) > maxMergeSourceSize {
		return nil, failure("CMDB_SOURCE_CHANGED", "recorded static source changed during inspection")
	}
	readAfter, readErr := file.Stat()
	pathFinal, pathErr := os.Lstat(path)
	if readErr != nil || pathErr != nil || !sameSourceRevision(after, readAfter) || !sameSourceRevision(readAfter, pathFinal) {
		return nil, failure("CMDB_SOURCE_CHANGED", "recorded static source changed during inspection")
	}
	return source, nil
}

func sameSourceRevision(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) && left.Mode() == right.Mode() &&
		left.Size() == right.Size() && left.ModTime().Equal(right.ModTime())
}

func preflightNewPrivateOutput(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", failure("CMDB_PARAMETER_INVALID", "dump requires a new output path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil || filepath.Clean(absolute) != absolute {
		return "", failure("CMDB_PARAMETER_INVALID", "dump output path is invalid")
	}
	parent := filepath.Dir(absolute)
	info, statErr := os.Lstat(parent)
	if statErr != nil || !info.IsDir() || !pathIsSymlinkFree(parent) {
		return "", failure("CMDB_OUTPUT_INVALID", "dump output directory is unavailable or unsafe")
	}
	if _, err := os.Lstat(absolute); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return "", failure("CMDB_OUTPUT_EXISTS", "dump output already exists")
		}
		return "", failure("CMDB_OUTPUT_INVALID", "dump output cannot be inspected")
	}
	return absolute, nil
}

func writeNewPrivateOutput(path string, body []byte) (string, [sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	absolute, err := preflightNewPrivateOutput(path)
	if err != nil {
		return "", empty, err
	}
	parent := filepath.Dir(absolute)
	temporary, err := os.CreateTemp(parent, ".pig-cmdb-output-*")
	if err != nil {
		return "", empty, failure("CMDB_OUTPUT_FAILED", "dump output staging failed")
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return "", empty, failure("CMDB_OUTPUT_FAILED", "dump output mode could not be secured")
	}
	if _, err := io.Copy(temporary, bytes.NewReader(body)); err != nil {
		temporary.Close()
		return "", empty, failure("CMDB_OUTPUT_FAILED", "dump output write failed")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", empty, failure("CMDB_OUTPUT_FAILED", "dump output fsync failed")
	}
	if err := temporary.Close(); err != nil {
		return "", empty, failure("CMDB_OUTPUT_FAILED", "dump output close failed")
	}
	if err := os.Link(temporaryName, absolute); err != nil {
		if _, existsErr := os.Lstat(absolute); existsErr == nil {
			return "", empty, failure("CMDB_OUTPUT_EXISTS", "dump output already exists")
		}
		return "", empty, failure("CMDB_OUTPUT_FAILED", "dump output commit failed")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return "", empty, failure("CMDB_OUTPUT_PARTIAL", "dump output exists but directory fsync failed")
	}
	if err := os.Remove(temporaryName); err != nil {
		return "", empty, failure("CMDB_OUTPUT_PARTIAL", "dump output exists but staging cleanup failed")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return "", empty, failure("CMDB_OUTPUT_PARTIAL", "dump output exists but directory cleanup fsync failed")
	}
	digest := sha256.Sum256(body)
	return absolute, digest, nil
}

func replaceLeadingCMDBMarker(source []byte, marker string) ([]byte, error) {
	newline := []byte("\n")
	if index := bytes.IndexByte(source, '\n'); index > 0 && source[index-1] == '\r' {
		newline = []byte("\r\n")
	}
	markerStart, markerEnd := -1, -1
	for start := 0; start < len(source); {
		end := bytes.IndexByte(source[start:], '\n')
		if end < 0 {
			end = len(source)
		} else {
			end += start + 1
		}
		contentEnd := end
		if contentEnd > start && source[contentEnd-1] == '\n' {
			contentEnd--
		}
		if contentEnd > start && source[contentEnd-1] == '\r' {
			contentEnd--
		}
		line := string(source[start:contentEnd])
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			break
		}
		if strings.HasPrefix(trimmed, cmdbMarkerPrefix) {
			if line != trimmed || !validCMDBMarkerLine(line) || markerStart >= 0 {
				return nil, failure("CMDB_MERGE_INVALID", "leading Pig CMDB marker is ambiguous or invalid")
			}
			markerStart, markerEnd = start, end
		}
		if end == len(source) {
			break
		}
		start = end
	}
	remainder := source
	if markerStart >= 0 {
		remainder = make([]byte, 0, len(source)-(markerEnd-markerStart))
		remainder = append(remainder, source[:markerStart]...)
		remainder = append(remainder, source[markerEnd:]...)
	}
	output := make([]byte, 0, len(marker)+len(newline)+len(remainder))
	output = append(output, marker...)
	output = append(output, newline...)
	output = append(output, remainder...)
	return output, nil
}

func validCMDBMarkerLine(line string) bool {
	fields := strings.Fields(line)
	if strings.Join(fields, " ") != line || len(fields) < 4 || fields[0] != "#" || fields[1] != "pig-sty-cmdb:" {
		return false
	}
	kind, ok := strings.CutPrefix(fields[2], "kind=")
	if !ok {
		return false
	}
	switch kind {
	case generatedMarkerKind:
		if len(fields) != 4 {
			return false
		}
		revision, ok := strings.CutPrefix(fields[3], "revision=")
		_, err := ParseRevisionToken(revision)
		return ok && err == nil
	case mergeMarkerKind:
		if len(fields) != 6 {
			return false
		}
		base, baseOK := strings.CutPrefix(fields[3], "base=")
		head, headOK := strings.CutPrefix(fields[4], "head=")
		digest, digestOK := strings.CutPrefix(fields[5], "source-sha256=")
		_, baseErr := ParseRevisionToken(base)
		_, headErr := ParseRevisionToken(head)
		_, digestErr := decodeLowerSHA256(digest)
		return baseOK && headOK && digestOK && baseErr == nil && headErr == nil && digestErr == nil
	default:
		return false
	}
}
