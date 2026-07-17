package cmdb

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceLeadingCMDBMarkerPreservesUnrelatedBytes(t *testing.T) {
	oldToken := newRevisionToken(1, sha256.Sum256([]byte("old")))
	newToken := newRevisionToken(2, sha256.Sum256([]byte("new")))
	oldMarker := cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=" + oldToken.String()
	newMarker := cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=" + newToken.String()

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "prepend before comments",
			source: "# owner comment\nall:\n  children: {}\n",
			want:   newMarker + "\n# owner comment\nall:\n  children: {}\n",
		},
		{
			name:   "replace recognized marker only",
			source: "# owner comment\n" + oldMarker + "\n# second comment\nall:\n  children: {}\n",
			want:   newMarker + "\n# owner comment\n# second comment\nall:\n  children: {}\n",
		},
		{
			name:   "preserve CRLF",
			source: "# owner comment\r\n" + oldMarker + "\r\nall:\r\n  children: {}\r\n",
			want:   newMarker + "\r\n# owner comment\r\nall:\r\n  children: {}\r\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := replaceLeadingCMDBMarker([]byte(test.source), newMarker)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("marker patch mismatch\n got: %q\nwant: %q", got, test.want)
			}
			if bytes.Count(got, []byte(cmdbMarkerPrefix)) != 1 || !bytes.HasPrefix(got, []byte(newMarker)) {
				t.Fatalf("output does not have exactly one leading marker: %q", got)
			}
		})
	}
}

func TestReplaceLeadingCMDBMarkerRejectsAmbiguousMarker(t *testing.T) {
	token := newRevisionToken(1, sha256.Sum256([]byte("revision")))
	valid := cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=" + token.String()
	tests := map[string]string{
		"duplicate": valid + "\n" + valid + "\nall: {}\n",
		"indented":  "  " + valid + "\nall: {}\n",
		"malformed": cmdbMarkerPrefix + " kind=" + generatedMarkerKind + " revision=unsafe\nall: {}\n",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := replaceLeadingCMDBMarker([]byte(source), valid); !IsCode(err, "CMDB_MERGE_INVALID") {
				t.Fatalf("ambiguous marker was accepted: %v", err)
			}
		})
	}
}

func TestWriteNewPrivateOutputIsAtomicAndExclusive(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, "dump.yml")
	body := []byte("private: inventory-secret\n")

	written, digest, err := writeNewPrivateOutput(path, body)
	if err != nil {
		t.Fatal(err)
	}
	if written != path || digest != sha256.Sum256(body) {
		t.Fatalf("write metadata mismatch: path=%q digest=%x", written, digest)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("output mode = %v, want regular 0600", info.Mode())
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("output bytes mismatch: %q err=%v", got, err)
	}
	if _, _, err := writeNewPrivateOutput(path, []byte("replacement")); !IsCode(err, "CMDB_OUTPUT_EXISTS") {
		t.Fatalf("existing output was overwritten: %v", err)
	}
	got, _ = os.ReadFile(path)
	if !bytes.Equal(got, body) {
		t.Fatalf("existing output changed: %q", got)
	}

	symlink := filepath.Join(directory, "symlink.yml")
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeNewPrivateOutput(symlink, []byte("replacement")); !IsCode(err, "CMDB_OUTPUT_EXISTS") {
		t.Fatalf("symlink output was accepted: %v", err)
	}
}

func TestPreflightNewPrivateOutputRejectsUnsafeDirectory(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	realDirectory := filepath.Join(directory, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedDirectory := filepath.Join(directory, "linked")
	if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightNewPrivateOutput(filepath.Join(linkedDirectory, "dump.yml")); !IsCode(err, "CMDB_OUTPUT_INVALID") {
		t.Fatalf("symlinked output directory was accepted: %v", err)
	}
	if _, err := preflightNewPrivateOutput(strings.Repeat(" ", 4)); !IsCode(err, "CMDB_PARAMETER_INVALID") {
		t.Fatalf("empty output path was accepted: %v", err)
	}
}

func TestReadMergeSourceClassifiesSizeAsRepresentability(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	path := filepath.Join(directory, "oversized.yml")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxMergeSourceSize + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readMergeSource(path); !IsCode(err, "CMDB_SNAPSHOT_UNREPRESENTABLE") {
		t.Fatalf("oversized merge source classification = %v", err)
	}
}
