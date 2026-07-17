package cmdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pig/inventory"
)

const fixedBundleBytes = `{"kind":"pig.sty.cmdb.backup/v1","format":1}
{"record":"migration","version":1,"name":"init","checksum":"1fb054340dce1b3a719f88345a568d032d2da045f89d787be759a5009052d299","tool_version":"tool-v1","applied_at":"2026-01-02T03:04:05.123456Z"}
{"record":"state","singleton":true,"head_revision":1,"updated_at":"2026-01-02T03:04:07Z"}
{"record":"snapshot","revision":1,"parent_revision":null,"merge_base_revision":1,"semantic_sha256":"77e2b99964b4702ccc560229579cb77734fff05ad0e6ea154566ba372f69262a","payload":{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[]}},"origin":"static_load","source_kind":"static","source_locator":"/srv/pigsty/pigsty.yml","source_bytes_sha256":"b8bb034f9b63bd0254fbc7c157cae746c75853f4643d6cea844dc48ddb57f522","tool_version":"tool-v1","created_at":"2026-01-02T03:04:06.654321Z"}
{"record":"checksum","sha256":"86e0680f61b4a7a0b334d43526adef519ee0045c6d656d798de22b8323cc63cf"}
`

func TestBundleV1GoldenAndSafeInspection(t *testing.T) {
	encoded, digest := encodeFixedBundle(t)
	if string(encoded) != fixedBundleBytes {
		t.Fatalf("bundle v1 bytes changed:\n%s", encoded)
	}
	if got := hex.EncodeToString(digest[:]); got != "c9378e29f50aa27db836430fda73398a70c32a6671313c650f9f26ca00c79e00" {
		t.Fatalf("bundle v1 file digest = %s", got)
	}

	directory := symlinkFreeTempDir(t)
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "fixed-v1.ndjson")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err := InspectBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	state := inspection.State()
	if inspection.Path() != path || inspection.SHA256() != hex.EncodeToString(digest[:]) ||
		state.MigrationVersion != 1 || state.RevisionCount != 1 || state.Head == nil || state.Head.Revision() != 1 {
		t.Fatalf("unexpected safe inspection: path=%q sha256=%q state=%+v", inspection.Path(), inspection.SHA256(), state)
	}
	metadata, err := json.Marshal(inspection)
	if err != nil || string(metadata) != "{}" || bytes.Contains(metadata, []byte("pigsty.yml")) {
		t.Fatalf("bundle inspection exposed private body: %s err=%v", metadata, err)
	}
}

func TestBundleV1RejectsNonCanonicalAndAmbiguousRecords(t *testing.T) {
	footerStart := strings.LastIndex(fixedBundleBytes, `{"record":"checksum"`)
	if footerStart < 0 {
		t.Fatal("fixed bundle footer not found")
	}
	body := fixedBundleBytes[:footerStart]
	tests := map[string][]byte{
		"duplicate field": bundleWithChecksum(strings.Replace(body,
			`{"kind":"pig.sty.cmdb.backup/v1","format":1}`,
			`{"kind":"pig.sty.cmdb.backup/v1","kind":"pig.sty.cmdb.backup/v1","format":1}`, 1)),
		"unknown field": bundleWithChecksum(strings.Replace(body,
			`{"kind":"pig.sty.cmdb.backup/v1","format":1}`,
			`{"kind":"pig.sty.cmdb.backup/v1","format":1,"extra":true}`, 1)),
		"noncanonical payload": bundleWithChecksum(strings.Replace(body,
			`"payload":{"kind":"pig.inventory.snapshot/v1"`,
			`"payload":{ "kind":"pig.inventory.snapshot/v1"`, 1)),
		"uppercase digest": bundleWithChecksum(strings.Replace(body,
			"77e2b99964b4702c", "77E2b99964b4702c", 1)),
		"noncanonical timestamp": bundleWithChecksum(strings.Replace(body,
			`"2026-01-02T03:04:05.123456Z"`, `"2026-01-02T03:04:05.123456+00:00"`, 1)),
		"crlf records":        bundleWithChecksum(strings.ReplaceAll(body, "\n", "\r\n")),
		"trailing object":     append([]byte(fixedBundleBytes), []byte("{}\n")...),
		"unterminated footer": []byte(strings.TrimSuffix(fixedBundleBytes, "\n")),
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			directory := symlinkFreeTempDir(t)
			if err := os.Chmod(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "invalid.ndjson")
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectBundle(path); !IsCode(err, "CMDB_BACKUP_INVALID") {
				t.Fatalf("invalid bundle was accepted: %v", err)
			}
		})
	}
}

func encodeFixedBundle(t *testing.T) ([]byte, [sha256.Size]byte) {
	t.Helper()
	payload := []byte(`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[]}}`)
	snapshot, err := inventory.ParseSnapshot(payload)
	if err != nil {
		t.Fatal(err)
	}
	semanticDigest := snapshot.SemanticSHA256()
	sourceDigest := sha256.Sum256([]byte("source\n"))
	locator := "/srv/pigsty/pigsty.yml"
	token := newRevisionToken(1, semanticDigest)
	chain := &verifiedChain{
		revisions: []*Revision{{
			revision: 1, mergeBaseRevision: 1, token: token, snapshot: snapshot,
			origin: OriginStaticLoad, sourceKind: SourceStatic, sourceLocator: &locator,
			sourceBytesDigest: sourceDigest[:], toolVersion: "tool-v1",
			createdAt: time.Date(2026, 1, 2, 3, 4, 6, 654321000, time.UTC),
		}},
		head:      &token,
		updatedAt: time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC),
	}
	ledger := []migrationRecord{{
		Version: 1, Name: "init", Checksum: append([]byte(nil), migrations[0].Checksum[:]...),
		ToolVersion: "tool-v1", AppliedAt: time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC),
	}}
	var output bytes.Buffer
	digest, err := encodeBundle(&output, ledger, chain)
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes(), digest
}

func bundleWithChecksum(body string) []byte {
	digest := sha256.Sum256([]byte(body))
	footer := `{"record":"checksum","sha256":"` + hex.EncodeToString(digest[:]) + `"}` + "\n"
	return []byte(body + footer)
}
