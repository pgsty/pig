package cmdb

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pig/inventory"
)

const (
	bundleKind          = "pig.sty.cmdb.backup/v1"
	bundleFormat        = 1
	maxBundleRecordSize = 70 << 20
)

type migrationRecord struct {
	Version     int64
	Name        string
	Checksum    []byte
	ToolVersion string
	AppliedAt   time.Time
}

type bundleData struct {
	path        string
	fileDigest  [sha256.Size]byte
	bodyDigest  [sha256.Size]byte
	ledger      []migrationRecord
	chain       *verifiedChain
	fingerprint ResetFingerprint
}

// BundleInspection is safe metadata for a fully parsed and verified private
// bundle. The path and exact file identity remain bound in private fields so a
// caller cannot forge a restore inspection.
type BundleInspection struct {
	path        string
	fileDigest  [sha256.Size]byte
	fingerprint ResetFingerprint
	state       VerifiedState
}

func (b BundleInspection) Path() string { return b.path }

func (b BundleInspection) SHA256() string { return hex.EncodeToString(b.fileDigest[:]) }

func (b BundleInspection) State() VerifiedState {
	state := b.state
	if state.Head != nil {
		copy := *state.Head
		state.Head = &copy
	}
	return state
}

// InspectBundle performs secure-file, full checksum, migration compatibility,
// complete lineage, and every Snapshot digest check without returning bodies.
func InspectBundle(path string) (BundleInspection, error) {
	data, err := readBundle(path)
	if err != nil {
		return BundleInspection{}, err
	}
	return inspectionFromBundle(data), nil
}

func inspectionFromBundle(data *bundleData) BundleInspection {
	return BundleInspection{
		path:        data.path,
		fileDigest:  data.fileDigest,
		fingerprint: data.fingerprint,
		state:       stateFromChain(data.chain),
	}
}

type bundleHeader struct {
	Kind   string `json:"kind"`
	Format int    `json:"format"`
}

type bundleMigration struct {
	Record      string `json:"record"`
	Version     int64  `json:"version"`
	Name        string `json:"name"`
	Checksum    string `json:"checksum"`
	ToolVersion string `json:"tool_version"`
	AppliedAt   string `json:"applied_at"`
}

type bundleState struct {
	Record       string `json:"record"`
	Singleton    bool   `json:"singleton"`
	HeadRevision *int64 `json:"head_revision"`
	UpdatedAt    string `json:"updated_at"`
}

type bundleSnapshot struct {
	Record            string          `json:"record"`
	Revision          int64           `json:"revision"`
	ParentRevision    *int64          `json:"parent_revision"`
	MergeBaseRevision int64           `json:"merge_base_revision"`
	SemanticSHA256    string          `json:"semantic_sha256"`
	Payload           json.RawMessage `json:"payload"`
	Origin            string          `json:"origin"`
	SourceKind        string          `json:"source_kind"`
	SourceLocator     *string         `json:"source_locator"`
	SourceBytesSHA256 *string         `json:"source_bytes_sha256"`
	ToolVersion       string          `json:"tool_version"`
	CreatedAt         string          `json:"created_at"`
}

type bundleFooter struct {
	Record string `json:"record"`
	SHA256 string `json:"sha256"`
}

func encodeBundle(writer io.Writer, ledger []migrationRecord, chain *verifiedChain) ([sha256.Size]byte, error) {
	var fileDigest [sha256.Size]byte
	bodyHasher := sha256.New()
	fileHasher := sha256.New()
	bodyWriter := io.MultiWriter(writer, bodyHasher, fileHasher)
	if err := encodeJSONLine(bodyWriter, bundleHeader{Kind: bundleKind, Format: bundleFormat}); err != nil {
		return fileDigest, err
	}
	if err := writeBundleStateRecords(bodyWriter, ledger, chain); err != nil {
		return fileDigest, err
	}
	bodyDigest := bodyHasher.Sum(nil)
	if err := encodeJSONLine(io.MultiWriter(writer, fileHasher), bundleFooter{
		Record: "checksum",
		SHA256: hex.EncodeToString(bodyDigest),
	}); err != nil {
		return fileDigest, err
	}
	copy(fileDigest[:], fileHasher.Sum(nil))
	return fileDigest, nil
}

func writeBundleStateRecords(writer io.Writer, ledger []migrationRecord, chain *verifiedChain) error {
	for _, record := range ledger {
		if err := encodeJSONLine(writer, bundleMigration{
			Record:      "migration",
			Version:     record.Version,
			Name:        record.Name,
			Checksum:    hex.EncodeToString(record.Checksum),
			ToolVersion: record.ToolVersion,
			AppliedAt:   canonicalBundleTime(record.AppliedAt),
		}); err != nil {
			return err
		}
	}
	var head *int64
	if chain.head != nil {
		value := chain.head.revision
		head = &value
	}
	if err := encodeJSONLine(writer, bundleState{
		Record:       "state",
		Singleton:    true,
		HeadRevision: head,
		UpdatedAt:    canonicalBundleTime(chain.updatedAt),
	}); err != nil {
		return err
	}
	for _, revision := range chain.revisions {
		var sourceDigest *string
		if revision.sourceBytesDigest != nil {
			value := hex.EncodeToString(revision.sourceBytesDigest)
			sourceDigest = &value
		}
		semanticDigest := revision.snapshot.SemanticSHA256()
		if err := encodeJSONLine(writer, bundleSnapshot{
			Record:            "snapshot",
			Revision:          revision.revision,
			ParentRevision:    revision.parentRevision,
			MergeBaseRevision: revision.mergeBaseRevision,
			SemanticSHA256:    hex.EncodeToString(semanticDigest[:]),
			Payload:           json.RawMessage(revision.snapshot.CanonicalBytes()),
			Origin:            string(revision.origin),
			SourceKind:        string(revision.sourceKind),
			SourceLocator:     revision.sourceLocator,
			SourceBytesSHA256: sourceDigest,
			ToolVersion:       revision.toolVersion,
			CreatedAt:         canonicalBundleTime(revision.createdAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func encodeJSONLine(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func canonicalBundleTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func readBundle(path string) (*bundleData, error) {
	file, absolute, err := openPrivateBundle(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fullHasher := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(file, fullHasher), 64<<10)
	bodyHasher := sha256.New()

	headerLine, err := readBundleLine(reader)
	if err != nil {
		return nil, bundleInvalid("header")
	}
	headerFields, err := decodeJSONObjectLine(headerLine)
	if err != nil || !exactJSONFields(headerFields, "format", "kind") {
		return nil, bundleInvalid("header")
	}
	var kind string
	var format int
	if decodeJSONValue(headerFields["kind"], &kind) != nil || decodeJSONValue(headerFields["format"], &format) != nil ||
		kind != bundleKind || format != bundleFormat {
		return nil, bundleInvalid("header")
	}
	bodyHasher.Write(headerLine)

	data := &bundleData{path: absolute}
	var stateSeen, footerSeen bool
	var storedHead pgtype.Int8
	var updatedAt time.Time
	for {
		line, lineErr := readBundleLine(reader)
		if lineErr != nil {
			return nil, bundleInvalid("terminator")
		}
		fields, err := decodeJSONObjectLine(line)
		if err != nil {
			return nil, bundleInvalid("record")
		}
		var recordKind string
		if raw, ok := fields["record"]; !ok || decodeJSONValue(raw, &recordKind) != nil {
			return nil, bundleInvalid("record kind")
		}
		if recordKind == "checksum" {
			if !stateSeen || footerSeen || !exactJSONFields(fields, "record", "sha256") {
				return nil, bundleInvalid("checksum record")
			}
			var checksum string
			if decodeJSONValue(fields["sha256"], &checksum) != nil || checksum != hex.EncodeToString(bodyHasher.Sum(nil)) {
				return nil, bundleInvalid("checksum")
			}
			copy(data.bodyDigest[:], bodyHasher.Sum(nil))
			footerSeen = true
			if trailing, trailingErr := readBundleLine(reader); trailingErr != io.EOF || len(trailing) != 0 {
				return nil, bundleInvalid("trailing data")
			}
			break
		}
		bodyHasher.Write(line)
		switch recordKind {
		case "migration":
			if stateSeen {
				return nil, bundleInvalid("migration order")
			}
			record, err := parseBundleMigration(fields)
			if err != nil {
				return nil, err
			}
			data.ledger = append(data.ledger, record)
		case "state":
			if stateSeen {
				return nil, bundleInvalid("state order")
			}
			head, timestamp, err := parseBundleState(fields)
			if err != nil {
				return nil, err
			}
			storedHead = head
			updatedAt = timestamp
			stateSeen = true
		case "snapshot":
			if !stateSeen {
				return nil, bundleInvalid("snapshot order")
			}
			revision, err := parseBundleSnapshot(fields)
			if err != nil {
				return nil, err
			}
			if data.chain == nil {
				data.chain = &verifiedChain{}
			}
			data.chain.revisions = append(data.chain.revisions, revision)
		default:
			return nil, bundleInvalid("record kind")
		}
	}
	if !footerSeen || !stateSeen {
		return nil, bundleInvalid("record sequence")
	}
	if data.chain == nil {
		data.chain = &verifiedChain{}
	}
	data.chain.updatedAt = updatedAt
	if err := validateRevisionLineage(data.chain, storedHead); err != nil {
		return nil, bundleInvalid("revision chain")
	}
	if err := verifyBundleMigrations(data.ledger); err != nil {
		return nil, err
	}
	data.fingerprint, err = nativeStateFingerprint(data.ledger, data.chain)
	if err != nil {
		return nil, err
	}
	copy(data.fileDigest[:], fullHasher.Sum(nil))
	return data, nil
}

func parseBundleMigration(fields map[string]json.RawMessage) (migrationRecord, error) {
	if !exactJSONFields(fields, "applied_at", "checksum", "name", "record", "tool_version", "version") {
		return migrationRecord{}, bundleInvalid("migration shape")
	}
	var encoded bundleMigration
	if decodeJSONValue(fields["version"], &encoded.Version) != nil ||
		decodeJSONValue(fields["name"], &encoded.Name) != nil ||
		decodeJSONValue(fields["checksum"], &encoded.Checksum) != nil ||
		decodeJSONValue(fields["tool_version"], &encoded.ToolVersion) != nil ||
		decodeJSONValue(fields["applied_at"], &encoded.AppliedAt) != nil {
		return migrationRecord{}, bundleInvalid("migration values")
	}
	checksum, err := decodeLowerSHA256(encoded.Checksum)
	if err != nil || encoded.Version <= 0 || encoded.Name == "" {
		return migrationRecord{}, bundleInvalid("migration values")
	}
	appliedAt, err := parseCanonicalBundleTime(encoded.AppliedAt)
	if err != nil {
		return migrationRecord{}, err
	}
	return migrationRecord{
		Version: encoded.Version, Name: encoded.Name, Checksum: checksum,
		ToolVersion: encoded.ToolVersion, AppliedAt: appliedAt,
	}, nil
}

func parseBundleState(fields map[string]json.RawMessage) (pgtype.Int8, time.Time, error) {
	if !exactJSONFields(fields, "head_revision", "record", "singleton", "updated_at") {
		return pgtype.Int8{}, time.Time{}, bundleInvalid("state shape")
	}
	var singleton bool
	var updatedText string
	if decodeJSONValue(fields["singleton"], &singleton) != nil || !singleton ||
		decodeJSONValue(fields["updated_at"], &updatedText) != nil {
		return pgtype.Int8{}, time.Time{}, bundleInvalid("state values")
	}
	var head pgtype.Int8
	if !bytes.Equal(fields["head_revision"], []byte("null")) {
		if decodeJSONValue(fields["head_revision"], &head.Int64) != nil || head.Int64 <= 0 {
			return pgtype.Int8{}, time.Time{}, bundleInvalid("state head")
		}
		head.Valid = true
	}
	updatedAt, err := parseCanonicalBundleTime(updatedText)
	return head, updatedAt, err
}

func parseBundleSnapshot(fields map[string]json.RawMessage) (*Revision, error) {
	expected := []string{
		"created_at", "merge_base_revision", "origin", "parent_revision", "payload", "record",
		"revision", "semantic_sha256", "source_bytes_sha256", "source_kind", "source_locator", "tool_version",
	}
	if !exactJSONFields(fields, expected...) {
		return nil, bundleInvalid("snapshot shape")
	}
	var encoded bundleSnapshot
	if decodeJSONValue(fields["revision"], &encoded.Revision) != nil ||
		decodeJSONValue(fields["merge_base_revision"], &encoded.MergeBaseRevision) != nil ||
		decodeJSONValue(fields["semantic_sha256"], &encoded.SemanticSHA256) != nil ||
		decodeJSONValue(fields["origin"], &encoded.Origin) != nil ||
		decodeJSONValue(fields["source_kind"], &encoded.SourceKind) != nil ||
		decodeJSONValue(fields["tool_version"], &encoded.ToolVersion) != nil ||
		decodeJSONValue(fields["created_at"], &encoded.CreatedAt) != nil {
		return nil, bundleInvalid("snapshot values")
	}
	parent, err := decodeNullableInt64(fields["parent_revision"])
	if err != nil {
		return nil, bundleInvalid("snapshot parent")
	}
	locator, err := decodeNullableString(fields["source_locator"])
	if err != nil {
		return nil, bundleInvalid("snapshot locator")
	}
	sourceDigestText, err := decodeNullableString(fields["source_bytes_sha256"])
	if err != nil {
		return nil, bundleInvalid("snapshot source digest")
	}
	semanticDigest, err := decodeLowerSHA256(encoded.SemanticSHA256)
	if err != nil {
		return nil, bundleInvalid("snapshot semantic digest")
	}
	var sourceDigest []byte
	if sourceDigestText != nil {
		sourceDigest, err = decodeLowerSHA256(*sourceDigestText)
		if err != nil {
			return nil, bundleInvalid("snapshot source digest")
		}
	}
	snapshot, err := inventory.ParseSnapshot(fields["payload"])
	if err != nil || !bytes.Equal(fields["payload"], snapshot.CanonicalBytes()) {
		return nil, bundleInvalid("snapshot payload")
	}
	actualDigest := snapshot.SemanticSHA256()
	if !slices.Equal(semanticDigest, actualDigest[:]) {
		return nil, bundleInvalid("snapshot semantic digest")
	}
	createdAt, err := parseCanonicalBundleTime(encoded.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &Revision{
		revision: encoded.Revision, parentRevision: parent, mergeBaseRevision: encoded.MergeBaseRevision,
		token: newRevisionToken(encoded.Revision, actualDigest), snapshot: snapshot,
		origin: RevisionOrigin(encoded.Origin), sourceKind: RevisionSourceKind(encoded.SourceKind),
		sourceLocator: locator, sourceBytesDigest: sourceDigest,
		toolVersion: encoded.ToolVersion, createdAt: createdAt,
	}, nil
}

func verifyBundleMigrations(records []migrationRecord) error {
	if len(records) != len(migrations) {
		return bundleInvalid("migration compatibility")
	}
	for index, record := range records {
		expected := migrations[index]
		if record.Version != expected.Version || record.Name != expected.Name ||
			!slices.Equal(record.Checksum, expected.Checksum[:]) {
			return bundleInvalid("migration compatibility")
		}
	}
	return nil
}

func parseCanonicalBundleTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || canonicalBundleTime(parsed) != value {
		return time.Time{}, bundleInvalid("timestamp")
	}
	return parsed, nil
}

func decodeLowerSHA256(value string) ([]byte, error) {
	if len(value) != 64 {
		return nil, errors.New("invalid digest")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return nil, errors.New("invalid digest")
	}
	return decoded, nil
}

func decodeNullableInt64(raw json.RawMessage) (*int64, error) {
	if bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var value int64
	if err := decodeJSONValue(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func decodeNullableString(raw json.RawMessage) (*string, error) {
	if bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var value string
	if err := decodeJSONValue(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func decodeJSONObjectLine(line []byte) (map[string]json.RawMessage, error) {
	if len(line) < 3 || line[len(line)-1] != '\n' || (len(line) > 1 && line[len(line)-2] == '\r') {
		return nil, errors.New("invalid line")
	}
	decoder := json.NewDecoder(bytes.NewReader(line[:len(line)-1]))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("invalid object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid field")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, errors.New("duplicate field")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid terminator")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return fields, nil
}

func decodeJSONValue(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}

func exactJSONFields(fields map[string]json.RawMessage, expected ...string) bool {
	if len(fields) != len(expected) {
		return false
	}
	sort.Strings(expected)
	for _, name := range expected {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func readBundleLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxBundleRecordSize {
			return nil, errors.New("bundle record too large")
		}
		line = append(line, fragment...)
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) == 0:
			return nil, io.EOF
		default:
			return nil, errors.New("unterminated bundle record")
		}
	}
}

func openPrivateBundle(path string) (*os.File, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || !pathIsSymlinkFree(absolute) {
		return nil, "", bundleInvalid("file path")
	}
	before, err := os.Lstat(absolute)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 || !ownedByEffectiveUser(before) {
		return nil, "", bundleInvalid("file permissions")
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, "", bundleInvalid("file open")
	}
	after, err := file.Stat()
	pathAfter, pathErr := os.Lstat(absolute)
	if err != nil || pathErr != nil || pathAfter.Mode()&os.ModeSymlink != 0 ||
		!after.Mode().IsRegular() || !os.SameFile(before, after) || !os.SameFile(after, pathAfter) {
		file.Close()
		return nil, "", bundleInvalid("file changed")
	}
	return file, absolute, nil
}

func writePrivateBundle(path string, ledger []migrationRecord, chain *verifiedChain) (*bundleData, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, bundleInvalid("destination path")
	}
	parent := filepath.Dir(absolute)
	if !validPrivateBundleDirectory(parent) {
		return nil, bundleInvalid("destination directory")
	}
	if _, err := os.Lstat(absolute); !errors.Is(err, os.ErrNotExist) {
		return nil, bundleInvalid("destination exists")
	}
	temporary, err := os.CreateTemp(parent, ".pig-cmdb-backup-*")
	if err != nil {
		return nil, bundleInvalid("destination staging")
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return nil, bundleInvalid("destination mode")
	}
	fileDigest, encodeErr := encodeBundle(temporary, ledger, chain)
	if encodeErr != nil {
		temporary.Close()
		return nil, bundleInvalid("write")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return nil, bundleInvalid("fsync")
	}
	if err := temporary.Close(); err != nil {
		return nil, bundleInvalid("close")
	}
	if err := os.Link(temporaryName, absolute); err != nil {
		return nil, bundleInvalid("destination commit")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return nil, bundleInvalid("directory fsync")
	}
	if err := os.Remove(temporaryName); err != nil {
		return nil, bundleInvalid("staging cleanup")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return nil, bundleInvalid("directory fsync")
	}
	verified, err := readBundle(absolute)
	if err != nil || verified.fileDigest != fileDigest {
		return nil, bundleInvalid("post-write verification")
	}
	return verified, nil
}

func validPrivateBundleDirectory(path string) bool {
	if !pathIsSymlinkFree(path) {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode().Perm() == 0o700 && ownedByEffectiveUser(info)
}

func pathIsSymlinkFree(absolute string) bool {
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	current := volume + string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(remainder, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func ownedByEffectiveUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func syncBundleDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func readMigrationRecordsTx(ctx context.Context, tx pgx.Tx) ([]migrationRecord, error) {
	rows, err := tx.Query(ctx, `
SELECT version, name, checksum, tool_version, applied_at
  FROM pigsty_cmdb.schema_migration
 ORDER BY version`)
	if err != nil {
		return nil, schemaInvalid("migration records")
	}
	defer rows.Close()
	var records []migrationRecord
	for rows.Next() {
		var record migrationRecord
		if err := rows.Scan(&record.Version, &record.Name, &record.Checksum, &record.ToolVersion, &record.AppliedAt); err != nil {
			return nil, schemaInvalid("migration records")
		}
		record.Checksum = slices.Clone(record.Checksum)
		records = append(records, record)
	}
	if rows.Err() != nil {
		return nil, schemaInvalid("migration records")
	}
	return records, nil
}

func nativeStateFingerprint(ledger []migrationRecord, chain *verifiedChain) (ResetFingerprint, error) {
	hasher := sha256.New()
	hasher.Write([]byte("pig.sty.cmdb.state-fingerprint/v1\n"))
	if err := writeBundleStateRecords(hasher, ledger, chain); err != nil {
		return ResetFingerprint{}, bundleInvalid("state fingerprint")
	}
	return newResetFingerprint(schemaNative, hasher), nil
}

func newResetFingerprint(presence schemaPresence, hasher hash.Hash) ResetFingerprint {
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return ResetFingerprint{presence: presence, digest: digest}
}

func bundleInvalid(part string) error {
	return failure("CMDB_BACKUP_INVALID", "private CMDB bundle "+part+" is invalid")
}
