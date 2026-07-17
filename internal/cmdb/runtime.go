package cmdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	RuntimeRecordKind = "pig.sty.cmdb.runtime/v1"
	RuntimeMarker     = "pig-sty-cmdb-adapter/v1"
	RuntimeEnv        = "PIGSTY_CMDB_RUNTIME"

	runtimeRecordLimit  = 64 << 10
	runtimeAdapterLimit = 64 << 10
	runtimeBinaryLimit  = 1 << 30
)

// RuntimeRecord contains only non-secret identity and connection-source
// metadata. In particular, it never carries a DSN or Inventory value.
type RuntimeRecord struct {
	Kind             string           `json:"kind"`
	Home             string           `json:"home"`
	HomeKey          string           `json:"home_key"`
	AdapterPath      string           `json:"adapter_path"`
	AdapterSHA256    string           `json:"adapter_sha256"`
	PigPath          string           `json:"pig_path"`
	PigSHA256        string           `json:"pig_sha256"`
	ConnectionSource ConnectionSource `json:"connection_source"`
	ConnectionFile   string           `json:"connection_file,omitempty"`
}

// RuntimeOptions identifies one per-home adapter. RuntimeRoot and PigBinary
// are injectable for tests; their empty values use ~/.pig/cmdb and the current
// executable respectively.
type RuntimeOptions struct {
	PigstyHome       string
	RuntimeRoot      string
	PigBinary        string
	ConnectionSource ConnectionSource
	ConnectionFile   string
}

// RuntimeRecordPath resolves the deterministic, read-only record location for
// one canonical Pigsty home. It never creates runtime directories.
func RuntimeRecordPath(pigstyHome, configuredRoot string) (string, error) {
	home, err := canonicalRuntimeHome(pigstyHome)
	if err != nil {
		return "", err
	}
	root, err := runtimeRoot(configuredRoot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(home))
	key := hex.EncodeToString(digest[:])[:16]
	return filepath.Join(root, key, "runtime.json"), nil
}

// RuntimeAdapterIdentity is the stable non-secret locator used by source
// descriptors for a verified native adapter.
func RuntimeAdapterIdentity(record RuntimeRecord) string {
	if record.Kind != RuntimeRecordKind || record.AdapterPath == "" || record.AdapterSHA256 == "" {
		return ""
	}
	return RuntimeMarker + ":sha256:" + record.AdapterSHA256 + ":" + record.AdapterPath
}

// PrepareRuntime creates or refreshes one private native adapter. An existing
// keyed directory must contain a valid record for the same complete home
// identity; otherwise it fails closed rather than adopting the directory.
func PrepareRuntime(options RuntimeOptions) (RuntimeRecord, error) {
	home, err := canonicalRuntimeHome(options.PigstyHome)
	if err != nil {
		return RuntimeRecord{}, err
	}
	root, err := runtimeRoot(options.RuntimeRoot)
	if err != nil {
		return RuntimeRecord{}, err
	}
	if err := ensurePrivateRuntimeRoot(root); err != nil {
		return RuntimeRecord{}, err
	}

	homeDigest := sha256.Sum256([]byte(home))
	homeKey := hex.EncodeToString(homeDigest[:])[:16]
	directory := filepath.Join(root, homeKey)
	created, err := ensureKeyedRuntimeDirectory(directory)
	if err != nil {
		return RuntimeRecord{}, err
	}
	runtimePath := filepath.Join(directory, "runtime.json")
	adapterPath := filepath.Join(directory, "inventory")
	if !created {
		previous, readErr := readRuntimeRecord(runtimePath)
		if readErr != nil {
			return RuntimeRecord{}, failure("CMDB_RUNTIME_COLLISION", "existing runtime directory has no valid home identity")
		}
		if previous.Home != home || previous.HomeKey != homeKey || previous.AdapterPath != adapterPath {
			return RuntimeRecord{}, failure("CMDB_RUNTIME_COLLISION", "runtime home identity does not match its directory key")
		}
	}
	if err := ensureRuntimeBackups(filepath.Join(directory, "backups")); err != nil {
		return RuntimeRecord{}, err
	}

	pigPath, pigDigest, err := inspectRuntimeBinary(options.PigBinary)
	if err != nil {
		return RuntimeRecord{}, err
	}
	source, connectionFile, err := prepareRuntimeConnection(options.ConnectionSource, options.ConnectionFile)
	if err != nil {
		return RuntimeRecord{}, err
	}
	wrapper, err := renderRuntimeAdapter(runtimePath, pigPath)
	if err != nil {
		return RuntimeRecord{}, err
	}
	wrapperDigest := sha256.Sum256(wrapper)
	record := RuntimeRecord{
		Kind: RuntimeRecordKind, Home: home, HomeKey: homeKey,
		AdapterPath: adapterPath, AdapterSHA256: hex.EncodeToString(wrapperDigest[:]),
		PigPath: pigPath, PigSHA256: hex.EncodeToString(pigDigest[:]),
		ConnectionSource: source, ConnectionFile: connectionFile,
	}
	if err := validateRuntimeRecord(record, runtimePath); err != nil {
		return RuntimeRecord{}, err
	}

	// Write the complete identity first. If adapter replacement is interrupted,
	// a later init can authenticate the directory and safely finish the refresh.
	if err := replacePrivateRuntimeFile(runtimePath, runtimeRecordBytes(record), 0o600); err != nil {
		return RuntimeRecord{}, err
	}
	if err := replacePrivateRuntimeFile(adapterPath, wrapper, 0o700); err != nil {
		return RuntimeRecord{}, err
	}
	return VerifyRuntimeFile(runtimePath, pigPath)
}

// VerifyRuntimeFile authenticates a complete runtime directory and optionally
// proves that currentPig resolves to the exact binary recorded by init.
func VerifyRuntimeFile(name, currentPig string) (RuntimeRecord, error) {
	runtimePath, err := canonicalExistingRuntimePath(name)
	if err != nil {
		return RuntimeRecord{}, err
	}
	record, err := readRuntimeRecord(runtimePath)
	if err != nil {
		return RuntimeRecord{}, err
	}
	directory := filepath.Dir(runtimePath)
	if !validPrivateBundleDirectory(filepath.Dir(directory)) ||
		!validPrivateBundleDirectory(directory) ||
		!validPrivateBundleDirectory(filepath.Join(directory, "backups")) {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_PERMISSION", "runtime directories must be private and owner-controlled")
	}
	canonicalHome, err := canonicalRuntimeHome(record.Home)
	if err != nil || canonicalHome != record.Home {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_HOME_INVALID", "recorded Pigsty home is unavailable or no longer canonical")
	}

	pigPath, pigDigest, err := inspectRuntimeBinary(record.PigPath)
	if err != nil || pigPath != record.PigPath || hex.EncodeToString(pigDigest[:]) != record.PigSHA256 {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_PIG_INVALID", "recorded Pig binary moved or changed after init")
	}
	if currentPig != "" {
		currentPath, _, currentErr := inspectRuntimeBinary(currentPig)
		if currentErr != nil || currentPath != record.PigPath {
			return RuntimeRecord{}, failure("CMDB_RUNTIME_PIG_INVALID", "current Pig binary does not match the runtime identity")
		}
	}

	wrapper, err := readStablePrivateRuntimeFile(record.AdapterPath, 0o700, runtimeAdapterLimit)
	if err != nil {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_ADAPTER_INVALID", "runtime adapter is unavailable or unsafe")
	}
	expected, err := renderRuntimeAdapter(runtimePath, record.PigPath)
	if err != nil || !bytes.Equal(wrapper, expected) {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_ADAPTER_INVALID", "runtime adapter content does not match its identity")
	}
	digest := sha256.Sum256(wrapper)
	if hex.EncodeToString(digest[:]) != record.AdapterSHA256 {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_ADAPTER_INVALID", "runtime adapter digest does not match its identity")
	}
	if record.ConnectionSource == ConnectionFileSource {
		if _, err := readConnectionFile(record.ConnectionFile); err != nil {
			return RuntimeRecord{}, failure("CMDB_RUNTIME_CONNECTION_INVALID", "recorded connection file is unavailable or unsafe")
		}
	}
	return record, nil
}

// ResolveRuntimeConnection resolves exactly the source recorded by init. It
// deliberately does not fall through to a lower-precedence connection source.
func ResolveRuntimeConnection(record RuntimeRecord) (ResolvedConnection, error) {
	switch record.ConnectionSource {
	case ConnectionFileSource:
		if record.ConnectionFile == "" {
			return ResolvedConnection{}, failure("CMDB_RUNTIME_CONNECTION_INVALID", "runtime connection-file metadata is incomplete")
		}
		value, err := readConnectionFile(record.ConnectionFile)
		if err != nil {
			return ResolvedConnection{}, failure("CMDB_RUNTIME_CONNECTION_INVALID", "recorded connection file is unavailable or unsafe")
		}
		return ResolvedConnection{source: ConnectionFileSource, value: value}, nil
	case ConnectionEnvSource:
		value, ok := os.LookupEnv(connectionEnv)
		if !ok || strings.TrimSpace(value) == "" {
			return ResolvedConnection{}, failure("CMDB_RUNTIME_CONNECTION_INVALID", "recorded connection environment is unavailable")
		}
		return ResolvedConnection{source: ConnectionEnvSource, value: value}, nil
	case ConnectionMetaSource:
		return ResolvedConnection{source: ConnectionMetaSource, value: "service=meta"}, nil
	default:
		return ResolvedConnection{}, failure("CMDB_RUNTIME_CONNECTION_INVALID", "runtime connection source is unsupported")
	}
}

func runtimeRoot(configured string) (string, error) {
	root := configured
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", failure("CMDB_RUNTIME_INVALID", "user home is unavailable")
		}
		root = filepath.Join(home, ".pig", "cmdb")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || !safeRuntimePath(absolute) {
		return "", failure("CMDB_RUNTIME_INVALID", "runtime root path is invalid")
	}
	return filepath.Clean(absolute), nil
}

func canonicalRuntimeHome(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", failure("CMDB_RUNTIME_HOME_INVALID", "Pigsty home is required")
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", failure("CMDB_RUNTIME_HOME_INVALID", "Pigsty home path is invalid")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil || !safeRuntimePath(resolved) || !pathIsSymlinkFree(resolved) {
		return "", failure("CMDB_RUNTIME_HOME_INVALID", "Pigsty home is unavailable or unsafe")
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() {
		return "", failure("CMDB_RUNTIME_HOME_INVALID", "Pigsty home must be a directory")
	}
	return filepath.Clean(resolved), nil
}

func ensurePrivateRuntimeRoot(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if !safeRuntimeDirectoryAncestor(path) {
			return failure("CMDB_RUNTIME_INVALID", "runtime root parent is unavailable or unsafe")
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return failure("CMDB_RUNTIME_INVALID", "runtime root cannot be created")
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return failure("CMDB_RUNTIME_PERMISSION", "runtime root permissions cannot be secured")
		}
	}
	if !validPrivateBundleDirectory(path) {
		return failure("CMDB_RUNTIME_PERMISSION", "runtime root must be a private owner-controlled directory")
	}
	return nil
}

func safeRuntimeDirectoryAncestor(path string) bool {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			return info.IsDir() && pathIsSymlinkFree(current)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func ensureKeyedRuntimeDirectory(path string) (bool, error) {
	err := os.Mkdir(path, 0o700)
	created := err == nil
	if err != nil && !errors.Is(err, os.ErrExist) {
		return false, failure("CMDB_RUNTIME_INVALID", "runtime directory cannot be created")
	}
	if !validPrivateBundleDirectory(path) {
		return false, failure("CMDB_RUNTIME_PERMISSION", "runtime directory must be private and owner-controlled")
	}
	return created, nil
}

func ensureRuntimeBackups(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return failure("CMDB_RUNTIME_INVALID", "runtime backup directory cannot be created")
		}
	}
	if !validPrivateBundleDirectory(path) {
		return failure("CMDB_RUNTIME_PERMISSION", "runtime backup directory must be private and owner-controlled")
	}
	return nil
}

func prepareRuntimeConnection(source ConnectionSource, file string) (ConnectionSource, string, error) {
	if source == "" {
		resolved, err := ResolveConnection(ResolveOptions{ConnectionFile: file})
		if err != nil {
			return "", "", err
		}
		source = resolved.Source()
	}
	switch source {
	case ConnectionFileSource:
		if file == "" {
			return "", "", failure("CMDB_RUNTIME_CONNECTION_INVALID", "connection-file source requires a protected file")
		}
		absolute, err := filepath.Abs(file)
		if err != nil || !safeRuntimePath(absolute) {
			return "", "", failure("CMDB_RUNTIME_CONNECTION_INVALID", "connection file path is invalid")
		}
		absolute = filepath.Clean(absolute)
		if _, err := readConnectionFile(absolute); err != nil {
			return "", "", err
		}
		return source, absolute, nil
	case ConnectionEnvSource:
		if file != "" {
			return "", "", failure("CMDB_RUNTIME_CONNECTION_INVALID", "environment source must not record a connection file")
		}
		value, ok := os.LookupEnv(connectionEnv)
		if !ok || strings.TrimSpace(value) == "" {
			return "", "", failure("CMDB_RUNTIME_CONNECTION_INVALID", "selected connection environment is unavailable")
		}
		return source, "", nil
	case ConnectionMetaSource:
		if file != "" {
			return "", "", failure("CMDB_RUNTIME_CONNECTION_INVALID", "service source must not record a connection file")
		}
		return source, "", nil
	default:
		return "", "", failure("CMDB_RUNTIME_CONNECTION_INVALID", "connection source is unsupported")
	}
}

func inspectRuntimeBinary(name string) (string, [sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	path := name
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return "", empty, failure("CMDB_RUNTIME_PIG_INVALID", "current Pig binary is unavailable")
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", empty, failure("CMDB_RUNTIME_PIG_INVALID", "Pig binary path is invalid")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil || !safeRuntimePath(resolved) || !pathIsSymlinkFree(resolved) {
		return "", empty, failure("CMDB_RUNTIME_PIG_INVALID", "Pig binary is unavailable or unsafe")
	}
	resolved = filepath.Clean(resolved)
	digest, err := digestStableRuntimeFile(resolved, true, runtimeBinaryLimit)
	if err != nil {
		return "", empty, err
	}
	return resolved, digest, nil
}

func digestStableRuntimeFile(path string, executable bool, limit int64) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	if !pathIsSymlinkFree(path) {
		return empty, failure("CMDB_RUNTIME_PIG_INVALID", "Pig binary path must be symlink-free")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit || executable && before.Mode().Perm()&0o111 == 0 {
		return empty, failure("CMDB_RUNTIME_PIG_INVALID", "Pig binary must be a bounded executable regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, failure("CMDB_RUNTIME_PIG_INVALID", "Pig binary cannot be opened")
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(hasher, io.LimitReader(file, limit+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	pathAfter, pathErr := os.Lstat(path)
	if copyErr != nil || statErr != nil || closeErr != nil || pathErr != nil || written > limit ||
		!sameSourceRevision(before, after) || !sameSourceRevision(after, pathAfter) {
		return empty, failure("CMDB_RUNTIME_PIG_INVALID", "Pig binary changed during inspection")
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func renderRuntimeAdapter(runtimePath, pigPath string) ([]byte, error) {
	if !safeRuntimePath(runtimePath) || !safeRuntimePath(pigPath) || !filepath.IsAbs(runtimePath) || !filepath.IsAbs(pigPath) {
		return nil, failure("CMDB_RUNTIME_ADAPTER_INVALID", "runtime adapter paths are not representable")
	}
	body := "#!/bin/sh\n" +
		"# " + RuntimeMarker + "\n" +
		"set -eu\n" +
		"case \"$#\" in\n" +
		"  1) [ \"$1\" = \"--list\" ] || exit 2 ;;\n" +
		"  2) [ \"$1\" = \"--host\" ] && [ -n \"$2\" ] || exit 2 ;;\n" +
		"  *) exit 2 ;;\n" +
		"esac\n" +
		RuntimeEnv + "=" + shellSingleQuote(runtimePath) + " exec " + shellSingleQuote(pigPath) + " sty cmdb _inventory \"$@\"\n"
	if len(body) > runtimeAdapterLimit {
		return nil, failure("CMDB_RUNTIME_ADAPTER_INVALID", "runtime adapter exceeds its size bound")
	}
	return []byte(body), nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func safeRuntimePath(path string) bool {
	return path != "" && utf8.ValidString(path) && !strings.ContainsAny(path, "\x00\r\n")
}

func replacePrivateRuntimeFile(path string, body []byte, mode os.FileMode) error {
	parent := filepath.Dir(path)
	if !validPrivateBundleDirectory(parent) {
		return failure("CMDB_RUNTIME_PERMISSION", "runtime file parent must be private and owner-controlled")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !ownedByEffectiveUser(info) || !pathIsSymlinkFree(path) {
			return failure("CMDB_RUNTIME_PERMISSION", "existing runtime file is not owner-controlled")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return failure("CMDB_RUNTIME_INVALID", "runtime file cannot be inspected")
	}
	temporary, err := os.CreateTemp(parent, ".pig-cmdb-runtime-*")
	if err != nil {
		return failure("CMDB_RUNTIME_INVALID", "runtime file staging failed")
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return failure("CMDB_RUNTIME_PERMISSION", "runtime file mode cannot be secured")
	}
	if _, err := io.Copy(temporary, bytes.NewReader(body)); err != nil {
		temporary.Close()
		return failure("CMDB_RUNTIME_INVALID", "runtime file write failed")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return failure("CMDB_RUNTIME_INVALID", "runtime file fsync failed")
	}
	if err := temporary.Close(); err != nil {
		return failure("CMDB_RUNTIME_INVALID", "runtime file close failed")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return failure("CMDB_RUNTIME_INVALID", "runtime file commit failed")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return failure("CMDB_RUNTIME_PARTIAL", "runtime file exists but directory fsync failed")
	}
	return nil
}

func canonicalExistingRuntimePath(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil || !safeRuntimePath(absolute) {
		return "", failure("CMDB_RUNTIME_INVALID", "runtime record path is invalid")
	}
	absolute = filepath.Clean(absolute)
	if !pathIsSymlinkFree(absolute) {
		return "", failure("CMDB_RUNTIME_INVALID", "runtime record path must be symlink-free")
	}
	return absolute, nil
}

func readRuntimeRecord(path string) (RuntimeRecord, error) {
	body, err := readStablePrivateRuntimeFile(path, 0o600, runtimeRecordLimit)
	if err != nil {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_INVALID", "runtime record is unavailable or unsafe")
	}
	var record RuntimeRecord
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_INVALID", "runtime record is malformed")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_INVALID", "runtime record has trailing data")
	}
	if err := validateRuntimeRecord(record, path); err != nil {
		return RuntimeRecord{}, err
	}
	if !bytes.Equal(body, runtimeRecordBytes(record)) {
		return RuntimeRecord{}, failure("CMDB_RUNTIME_INVALID", "runtime record is not canonical")
	}
	return record, nil
}

func runtimeRecordBytes(record RuntimeRecord) []byte {
	body, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return append(body, '\n')
}

func validateRuntimeRecord(record RuntimeRecord, runtimePath string) error {
	if record.Kind != RuntimeRecordKind || !safeRuntimePath(record.Home) || !filepath.IsAbs(record.Home) || filepath.Clean(record.Home) != record.Home {
		return failure("CMDB_RUNTIME_INVALID", "runtime home identity is invalid")
	}
	homeDigest := sha256.Sum256([]byte(record.Home))
	wantKey := hex.EncodeToString(homeDigest[:])[:16]
	if record.HomeKey != wantKey || filepath.Base(filepath.Dir(runtimePath)) != record.HomeKey {
		return failure("CMDB_RUNTIME_COLLISION", "runtime home identity does not match its directory key")
	}
	directory := filepath.Dir(runtimePath)
	if filepath.Base(runtimePath) != "runtime.json" || record.AdapterPath != filepath.Join(directory, "inventory") {
		return failure("CMDB_RUNTIME_INVALID", "runtime adapter identity is invalid")
	}
	if !safeRuntimePath(record.AdapterPath) || !safeRuntimePath(record.PigPath) || !filepath.IsAbs(record.PigPath) || filepath.Clean(record.PigPath) != record.PigPath {
		return failure("CMDB_RUNTIME_INVALID", "runtime executable identity is invalid")
	}
	if _, err := decodeLowerSHA256(record.AdapterSHA256); err != nil {
		return failure("CMDB_RUNTIME_INVALID", "runtime adapter digest is invalid")
	}
	if _, err := decodeLowerSHA256(record.PigSHA256); err != nil {
		return failure("CMDB_RUNTIME_INVALID", "runtime Pig digest is invalid")
	}
	switch record.ConnectionSource {
	case ConnectionFileSource:
		if !safeRuntimePath(record.ConnectionFile) || !filepath.IsAbs(record.ConnectionFile) || filepath.Clean(record.ConnectionFile) != record.ConnectionFile {
			return failure("CMDB_RUNTIME_CONNECTION_INVALID", "runtime connection file identity is invalid")
		}
	case ConnectionEnvSource, ConnectionMetaSource:
		if record.ConnectionFile != "" {
			return failure("CMDB_RUNTIME_CONNECTION_INVALID", "runtime connection source metadata is inconsistent")
		}
	default:
		return failure("CMDB_RUNTIME_CONNECTION_INVALID", "runtime connection source is unsupported")
	}
	return nil
}

func readStablePrivateRuntimeFile(path string, mode os.FileMode, limit int64) ([]byte, error) {
	if !pathIsSymlinkFree(path) {
		return nil, errors.New("unsafe path")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != mode || !ownedByEffectiveUser(before) || before.Size() > limit {
		return nil, errors.New("unsafe file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open failed")
	}
	body, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	pathAfter, pathErr := os.Lstat(path)
	if readErr != nil || statErr != nil || closeErr != nil || pathErr != nil || len(body) > int(limit) ||
		!sameSourceRevision(before, after) || !sameSourceRevision(after, pathAfter) {
		return nil, errors.New("file changed")
	}
	return body, nil
}
