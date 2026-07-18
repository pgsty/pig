package cmdb

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"
)

const maxAnsibleConfigSize = 1 << 20

type ansibleConfigDocument struct {
	path       string
	literal    string
	locator    string
	body       []byte
	valueStart int
	valueEnd   int
	mode       os.FileMode
	ownerUID   uint32
	ownerGID   uint32
}

type ansibleConfigPatch struct {
	document  *ansibleConfigDocument
	candidate []byte
}

func ansibleConfigFailure(path string) error {
	return failure("CMDB_ANSIBLE_CONFIG_INVALID", "edit "+path+" manually so [defaults] contains exactly one literal inventory = <path> directive")
}

func readAnsibleConfig(name string) (*ansibleConfigDocument, error) {
	absolute, err := filepath.Abs(name)
	if err != nil || !validLocalPath(absolute) {
		return nil, ansibleConfigFailure(name)
	}
	absolute = filepath.Clean(absolute)
	if !pathIsSymlinkFree(absolute) {
		return nil, ansibleConfigFailure(absolute)
	}
	before, err := os.Lstat(absolute)
	if err != nil || !before.Mode().IsRegular() || before.Size() > maxAnsibleConfigSize {
		return nil, ansibleConfigFailure(absolute)
	}
	ownerUID, ownerGID, ok := fileOwner(before)
	if !ok {
		return nil, ansibleConfigFailure(absolute)
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, ansibleConfigFailure(absolute)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxAnsibleConfigSize+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	pathAfter, pathErr := os.Lstat(absolute)
	if readErr != nil || statErr != nil || closeErr != nil || pathErr != nil || len(body) > maxAnsibleConfigSize ||
		!sameSourceRevision(before, after) || !sameSourceRevision(after, pathAfter) {
		return nil, ansibleConfigFailure(absolute)
	}
	document, err := parseAnsibleConfig(absolute, body)
	if err != nil {
		return nil, ansibleConfigFailure(absolute)
	}
	document.mode = before.Mode()
	document.ownerUID = ownerUID
	document.ownerGID = ownerGID
	return document, nil
}

func parseAnsibleConfig(path string, body []byte) (*ansibleConfigDocument, error) {
	if len(body) > maxAnsibleConfigSize || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return nil, errors.New("invalid ansible config encoding")
	}
	document := &ansibleConfigDocument{path: path, body: bytes.Clone(body)}
	defaultsSections := 0
	inDefaults := false
	inventoryAssignments := 0
	for offset := 0; offset < len(body); {
		lineStart := offset
		newline := bytes.IndexByte(body[offset:], '\n')
		lineEnd := len(body)
		if newline >= 0 {
			lineEnd = offset + newline
			offset = lineEnd + 1
		} else {
			offset = len(body)
		}
		contentEnd := lineEnd
		if contentEnd > lineStart && body[contentEnd-1] == '\r' {
			contentEnd--
		}
		line := body[lineStart:contentEnd]
		if bytes.IndexByte(line, '\r') >= 0 {
			return nil, errors.New("unsupported line ending")
		}
		trimmed, leading := trimOrdinarySpace(line)
		if len(trimmed) == 0 || trimmed[0] == '#' || trimmed[0] == ';' {
			continue
		}
		if trimmed[0] == '[' {
			section, ok := parseAnsibleSection(trimmed)
			if !ok {
				return nil, errors.New("malformed section")
			}
			inDefaults = strings.EqualFold(section, "defaults")
			if inDefaults {
				defaultsSections++
			}
			continue
		}
		equal := bytes.IndexByte(trimmed, '=')
		if equal < 0 {
			continue
		}
		key := strings.TrimSpace(string(trimmed[:equal]))
		if !inDefaults || !strings.EqualFold(key, "inventory") {
			continue
		}
		literal, locator, relativeStart, relativeEnd, err := parseAnsibleInventoryValue(path, trimmed, equal+1)
		if err != nil {
			return nil, err
		}
		inventoryAssignments++
		document.valueStart = lineStart + leading + relativeStart
		document.valueEnd = lineStart + leading + relativeEnd
		document.literal, document.locator = literal, locator
	}
	if defaultsSections != 1 || inventoryAssignments != 1 || document.valueStart >= document.valueEnd {
		return nil, errors.New("ambiguous inventory directive")
	}
	return document, nil
}

func parseAnsibleSection(line []byte) (string, bool) {
	close := bytes.IndexByte(line, ']')
	if close <= 1 {
		return "", false
	}
	name := strings.TrimSpace(string(line[1:close]))
	if name == "" || strings.ContainsAny(name, "[]\r\n\x00") {
		return "", false
	}
	rest := trimOrdinarySpaceOnly(line[close+1:])
	if len(rest) > 0 && rest[0] != '#' && rest[0] != ';' {
		return "", false
	}
	return name, true
}

func parseAnsibleInventoryValue(configPath string, line []byte, valueOffset int) (string, string, int, int, error) {
	if valueOffset < 0 || valueOffset > len(line) {
		return "", "", 0, 0, errors.New("invalid value offset")
	}
	leading := 0
	for valueOffset+leading < len(line) && ordinarySpace(line[valueOffset+leading]) {
		leading++
	}
	start := valueOffset + leading
	if start >= len(line) || line[start] == '#' || line[start] == ';' {
		return "", "", 0, 0, errors.New("missing inventory literal")
	}
	end := start
	valueStart := start
	valueEnd := start
	var value []byte
	if line[start] == '\'' || line[start] == '"' {
		quote := line[start]
		end = start + 1
		for end < len(line) && line[end] != quote {
			end++
		}
		if end >= len(line) {
			return "", "", 0, 0, errors.New("unterminated inventory quote")
		}
		value = line[start+1 : end]
		valueEnd = end + 1
		end++
	} else {
		for end < len(line) && !ordinarySpace(line[end]) {
			end++
		}
		value = line[start:end]
		valueEnd = end
		if bytes.IndexAny(value, "'\"") >= 0 {
			return "", "", 0, 0, errors.New("unsupported inventory quote")
		}
	}
	rest := trimOrdinarySpaceOnly(line[end:])
	if len(rest) > 0 && rest[0] != '#' && rest[0] != ';' {
		return "", "", 0, 0, errors.New("unsupported inventory suffix")
	}
	if !validAnsibleLiteral(value) {
		return "", "", 0, 0, errors.New("unsupported inventory literal")
	}
	locator, err := resolveAnsibleLiteral(configPath, string(value))
	if err != nil {
		return "", "", 0, 0, err
	}
	return string(line[valueStart:valueEnd]), locator, valueStart, valueEnd, nil
}

func validAnsibleLiteral(value []byte) bool {
	if len(value) == 0 || !utf8.Valid(value) || bytes.IndexAny(value, "\x00\r\n\\,$") >= 0 {
		return false
	}
	text := string(value)
	return !strings.Contains(text, "{{") && !strings.Contains(text, "}}") &&
		!strings.Contains(text, "{%") && !strings.Contains(text, "%}") &&
		!strings.Contains(text, "{#") && !strings.Contains(text, "#}") &&
		!strings.Contains(text, "%(")
}

func resolveAnsibleLiteral(configPath, literal string) (string, error) {
	path := literal
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(configPath), path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil || !validLocalPath(absolute) {
		return "", errors.New("invalid inventory path")
	}
	return filepath.Clean(absolute), nil
}

func planAnsibleConfigLiteralPatch(document *ansibleConfigDocument, literal, target string) (*ansibleConfigPatch, error) {
	if document == nil || document.valueStart < 0 || document.valueEnd > len(document.body) || document.valueStart >= document.valueEnd ||
		!filepath.IsAbs(target) || filepath.Clean(target) != target || literal == "" || strings.ContainsAny(literal, "\r\n\x00") {
		return nil, ansibleConfigFailure("ansible.cfg")
	}
	candidate := make([]byte, 0, len(document.body)-(document.valueEnd-document.valueStart)+len(literal))
	candidate = append(candidate, document.body[:document.valueStart]...)
	candidate = append(candidate, literal...)
	candidate = append(candidate, document.body[document.valueEnd:]...)
	reparsed, err := parseAnsibleConfig(document.path, candidate)
	if err != nil || reparsed.locator != target || reparsed.literal != literal {
		return nil, ansibleConfigFailure(document.path)
	}
	return &ansibleConfigPatch{document: document, candidate: candidate}, nil
}

func trimOrdinarySpace(line []byte) ([]byte, int) {
	start := 0
	for start < len(line) && ordinarySpace(line[start]) {
		start++
	}
	end := len(line)
	for end > start && ordinarySpace(line[end-1]) {
		end--
	}
	return line[start:end], start
}

func trimOrdinarySpaceOnly(value []byte) []byte {
	start := 0
	for start < len(value) && ordinarySpace(value[start]) {
		start++
	}
	end := len(value)
	for end > start && ordinarySpace(value[end-1]) {
		end--
	}
	return value[start:end]
}

func ordinarySpace(value byte) bool {
	return value == ' ' || value == '\t'
}

func fileOwner(info os.FileInfo) (uint32, uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return stat.Uid, stat.Gid, true
}

// commitAnsibleConfigPatch atomically replaces only the planned literal span.
// It fails closed if the file changed between planning and commit.
func commitAnsibleConfigPatch(patch *ansibleConfigPatch) error {
	if patch == nil || patch.document == nil {
		return failure("CMDB_SWITCH_INVALID", "ansible.cfg patch is incomplete")
	}
	document := patch.document
	path := document.path
	parent := filepath.Dir(path)
	if document.ownerUID != uint32(os.Geteuid()) || document.ownerGID != uint32(os.Getegid()) || !pathIsSymlinkFree(parent) {
		return failure("CMDB_SWITCH_PERMISSION", "ansible.cfg is not owner-controlled")
	}
	temporary, err := os.CreateTemp(parent, ".pig-ansible-cfg-*")
	if err != nil {
		return failure("CMDB_SWITCH_FAILED", "ansible.cfg staging failed")
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(document.mode); err != nil {
		_ = temporary.Close()
		return failure("CMDB_SWITCH_PERMISSION", "ansible.cfg mode cannot be preserved")
	}
	if _, err := temporary.Write(patch.candidate); err != nil {
		_ = temporary.Close()
		return failure("CMDB_SWITCH_FAILED", "ansible.cfg staging write failed")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return failure("CMDB_SWITCH_FAILED", "ansible.cfg staging fsync failed")
	}
	if err := temporary.Close(); err != nil {
		return failure("CMDB_SWITCH_FAILED", "ansible.cfg staging close failed")
	}
	current, err := readAnsibleConfig(path)
	if err != nil || !bytes.Equal(current.body, document.body) || current.mode != document.mode ||
		current.ownerUID != document.ownerUID || current.ownerGID != document.ownerGID {
		return failure("CMDB_SOURCE_CHANGED", "ansible.cfg changed after planning")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return failure("CMDB_SWITCH_FAILED", "ansible.cfg atomic replacement failed")
	}
	if err := syncDirectory(parent); err != nil {
		return failure("CMDB_SWITCH_PARTIAL", "ansible.cfg was replaced but parent fsync failed")
	}
	committed, err := readAnsibleConfig(path)
	if err != nil || !bytes.Equal(committed.body, patch.candidate) || committed.mode != document.mode ||
		committed.ownerUID != document.ownerUID || committed.ownerGID != document.ownerGID {
		return failure("CMDB_SWITCH_PARTIAL", "ansible.cfg replacement could not be verified")
	}
	return nil
}

func validLocalPath(path string) bool {
	return path != "" && utf8.ValidString(path) && !strings.ContainsAny(path, "\x00\r\n")
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

func sameSourceRevision(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) && left.Mode() == right.Mode() &&
		left.Size() == right.Size() && left.ModTime().Equal(right.ModTime())
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
