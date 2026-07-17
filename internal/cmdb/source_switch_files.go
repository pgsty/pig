package cmdb

import (
	"bytes"
	"crypto/rand"
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
	SwitchRecordKind    = "pig.sty.cmdb.switch/v1"
	SourceSwitchKind    = "pig.sty.cmdb.source-switch/v1"
	maxSwitchRecordSize = 64 << 10
)

type SourceSwitchOperation string
type SourceSwitchState string

const (
	SourceSwitchEnable  SourceSwitchOperation = "enable"
	SourceSwitchDisable SourceSwitchOperation = "disable"

	SourceSwitchNone    SourceSwitchState = "none"
	SourceSwitchPending SourceSwitchState = "pending"
	SourceSwitchApplied SourceSwitchState = "applied"
	SourceSwitchDrift   SourceSwitchState = "drift"
)

// SwitchRecord is a value-free, canonical intent. It contains paths, exact
// byte revisions, adapter identity, and one head token, never Inventory data.
type SwitchRecord struct {
	Kind               string                `json:"kind"`
	IntentPath         string                `json:"intent_path"`
	PreviousIntent     string                `json:"previous_intent,omitempty"`
	Operation          SourceSwitchOperation `json:"operation"`
	Home               string                `json:"home"`
	ConfigPath         string                `json:"config_path"`
	BeforeSHA256       string                `json:"before_sha256"`
	AfterSHA256        string                `json:"after_sha256"`
	BeforeLiteral      string                `json:"before_literal"`
	BeforeLocator      string                `json:"before_locator"`
	AfterLiteral       string                `json:"after_literal"`
	AfterLocator       string                `json:"after_locator"`
	PriorLiteral       string                `json:"prior_literal"`
	PriorStaticLocator string                `json:"prior_static_locator"`
	Head               string                `json:"head"`
	RuntimePath        string                `json:"runtime_path"`
	AdapterPath        string                `json:"adapter_path"`
	AdapterSHA256      string                `json:"adapter_sha256"`
	PigPath            string                `json:"pig_path"`
	PigSHA256          string                `json:"pig_sha256"`
	BackupPath         string                `json:"backup_path"`
	BackupSHA256       string                `json:"backup_sha256"`
	ConfigMode         uint32                `json:"config_mode"`
	BeforeOwnerUID     uint32                `json:"before_owner_uid"`
	BeforeOwnerGID     uint32                `json:"before_owner_gid"`
}

type SourceSwitchInspection struct {
	Kind   string            `json:"kind"`
	State  SourceSwitchState `json:"state"`
	Record *SwitchRecord     `json:"record,omitempty"`
}

type sourceSwitchIntentOptions struct {
	operation          SourceSwitchOperation
	home               string
	runtimePath        string
	head               RevisionToken
	adapter            RuntimeRecord
	priorLiteral       string
	priorStaticLocator string
	previous           *SwitchRecord
	intentID           string
}

// InspectSourceSwitch authenticates the latest record, its retained unique
// intent and exact backup, then derives state only from current config bytes.
func InspectSourceSwitch(runtimePath, configPath string) (SourceSwitchInspection, error) {
	inspection := SourceSwitchInspection{Kind: SourceSwitchKind, State: SourceSwitchNone}
	canonicalRuntime, err := filepath.Abs(runtimePath)
	if err != nil {
		return inspection, failure("CMDB_SWITCH_INVALID", "runtime record path is invalid")
	}
	canonicalRuntime = filepath.Clean(canonicalRuntime)
	directory := filepath.Dir(canonicalRuntime)
	latestPath := filepath.Join(directory, "switch.json")
	if _, err := os.Lstat(latestPath); errors.Is(err, os.ErrNotExist) {
		return inspection, nil
	}
	record, err := readSwitchRecord(latestPath, canonicalRuntime)
	if err != nil {
		return inspection, err
	}
	inspection.Record = cloneSwitchRecord(&record)
	if configPath == "" {
		configPath = record.ConfigPath
	}
	absoluteConfig, err := filepath.Abs(configPath)
	if err != nil || filepath.Clean(absoluteConfig) != record.ConfigPath {
		return inspection, failure("CMDB_SWITCH_INVALID", "source-switch config identity does not match")
	}
	document, err := readAnsibleConfig(record.ConfigPath)
	if err != nil {
		inspection.State = SourceSwitchDrift
		return inspection, failure("CMDB_SWITCH_DRIFT", "ansible.cfg no longer matches a recorded byte revision")
	}
	digest := sha256.Sum256(document.body)
	encoded := hex.EncodeToString(digest[:])
	switch encoded {
	case record.BeforeSHA256:
		inspection.State = SourceSwitchPending
	case record.AfterSHA256:
		inspection.State = SourceSwitchApplied
	default:
		inspection.State = SourceSwitchDrift
	}
	return inspection, nil
}

func createSourceSwitchIntent(document *ansibleConfigDocument, patch *ansibleConfigPatch, options sourceSwitchIntentOptions) (SwitchRecord, error) {
	if document == nil || patch == nil || patch.document != document || !bytes.Equal(patch.document.body, document.body) {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch patch is incomplete")
	}
	id := options.intentID
	if id == "" {
		var random [16]byte
		if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
			return SwitchRecord{}, failure("CMDB_SWITCH_FAILED", "source-switch identifier generation failed")
		}
		id = hex.EncodeToString(random[:])
	}
	if !validSwitchID(id) {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch identifier is invalid")
	}
	directory := filepath.Dir(options.runtimePath)
	backupPath := filepath.Join(directory, "backups", "ansible-"+id+".cfg")
	intentPath := filepath.Join(directory, "switch-"+id+".json")
	beforeDigest := sha256.Sum256(document.body)
	previousIntent := ""
	if options.previous != nil {
		previousIntent = options.previous.IntentPath
	}
	record := SwitchRecord{
		Kind: SwitchRecordKind, IntentPath: intentPath, PreviousIntent: previousIntent,
		Operation: options.operation, Home: options.home, ConfigPath: document.source.ConfigPath,
		BeforeSHA256: hex.EncodeToString(beforeDigest[:]), AfterSHA256: hex.EncodeToString(patch.afterDigest[:]),
		BeforeLiteral: document.source.Literal, BeforeLocator: document.source.Locator,
		AfterLiteral: patch.nextLiteral, AfterLocator: patch.nextLocator,
		PriorLiteral: options.priorLiteral, PriorStaticLocator: options.priorStaticLocator,
		Head: options.head.String(), RuntimePath: options.runtimePath,
		AdapterPath: options.adapter.AdapterPath, AdapterSHA256: options.adapter.AdapterSHA256,
		PigPath: options.adapter.PigPath, PigSHA256: options.adapter.PigSHA256,
		BackupPath: backupPath, BackupSHA256: hex.EncodeToString(beforeDigest[:]),
		ConfigMode: uint32(document.mode), BeforeOwnerUID: document.ownerUID, BeforeOwnerGID: document.ownerGID,
	}
	if err := validateSwitchRecord(record, options.runtimePath); err != nil {
		return SwitchRecord{}, err
	}
	if err := writeNewSwitchArtifact(backupPath, document.body, document.mode); err != nil {
		return SwitchRecord{}, err
	}
	body := switchRecordBytes(record)
	if err := writeNewSwitchArtifact(intentPath, body, 0o600); err != nil {
		return SwitchRecord{}, err
	}
	if err := replacePrivateRuntimeFile(filepath.Join(directory, "switch.json"), body, 0o600); err != nil {
		return SwitchRecord{}, err
	}
	return record, nil
}

func readSwitchRecord(path, runtimePath string) (SwitchRecord, error) {
	body, err := readStablePrivateRuntimeFile(path, 0o600, maxSwitchRecordSize)
	if err != nil {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch record is unavailable or unsafe")
	}
	var record SwitchRecord
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch record is malformed")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch record has trailing data")
	}
	if err := validateSwitchRecord(record, runtimePath); err != nil {
		return SwitchRecord{}, err
	}
	canonical := switchRecordBytes(record)
	if !bytes.Equal(body, canonical) {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch record is not canonical")
	}
	unique, err := readStablePrivateRuntimeFile(record.IntentPath, 0o600, maxSwitchRecordSize)
	if err != nil || !bytes.Equal(unique, canonical) {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "retained source-switch intent is unavailable or changed")
	}
	if record.PreviousIntent != "" {
		if _, err := readStablePrivateRuntimeFile(record.PreviousIntent, 0o600, maxSwitchRecordSize); err != nil {
			return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "previous source-switch intent is not retained")
		}
	}
	backup, mode, err := readStableSwitchBackup(record.BackupPath, maxAnsibleConfigSize)
	if err != nil || uint32(mode) != record.ConfigMode {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch backup is unavailable or changed")
	}
	digest := sha256.Sum256(backup)
	if hex.EncodeToString(digest[:]) != record.BackupSHA256 || record.BackupSHA256 != record.BeforeSHA256 {
		return SwitchRecord{}, failure("CMDB_SWITCH_INVALID", "source-switch backup digest does not match the intent")
	}
	return record, nil
}

func validateSwitchRecord(record SwitchRecord, runtimePath string) error {
	canonicalRuntime, err := filepath.Abs(runtimePath)
	if err != nil {
		return failure("CMDB_SWITCH_INVALID", "source-switch runtime identity is invalid")
	}
	canonicalRuntime = filepath.Clean(canonicalRuntime)
	directory := filepath.Dir(canonicalRuntime)
	if record.Kind != SwitchRecordKind || record.RuntimePath != canonicalRuntime ||
		!filepath.IsAbs(record.Home) || filepath.Clean(record.Home) != record.Home ||
		record.ConfigPath != filepath.Join(record.Home, "ansible.cfg") {
		return failure("CMDB_SWITCH_INVALID", "source-switch identity is invalid")
	}
	if record.Operation != SourceSwitchEnable && record.Operation != SourceSwitchDisable {
		return failure("CMDB_SWITCH_INVALID", "source-switch operation is invalid")
	}
	if !switchPathWithin(record.IntentPath, directory, "switch-", ".json") ||
		!switchPathWithin(record.BackupPath, filepath.Join(directory, "backups"), "ansible-", ".cfg") {
		return failure("CMDB_SWITCH_INVALID", "source-switch artifact path is invalid")
	}
	if record.PreviousIntent != "" && (!switchPathWithin(record.PreviousIntent, directory, "switch-", ".json") || record.PreviousIntent == record.IntentPath) {
		return failure("CMDB_SWITCH_INVALID", "previous source-switch intent path is invalid")
	}
	for _, digest := range []string{
		record.BeforeSHA256, record.AfterSHA256, record.AdapterSHA256,
		record.PigSHA256, record.BackupSHA256,
	} {
		if _, err := decodeLowerSHA256(digest); err != nil {
			return failure("CMDB_SWITCH_INVALID", "source-switch digest is invalid")
		}
	}
	if record.BeforeSHA256 == record.AfterSHA256 || record.BackupSHA256 != record.BeforeSHA256 {
		return failure("CMDB_SWITCH_INVALID", "source-switch byte revisions are invalid")
	}
	if _, err := ParseRevisionToken(record.Head); err != nil {
		return failure("CMDB_SWITCH_INVALID", "source-switch head token is invalid")
	}
	if !validSwitchLiteral(record.BeforeLiteral) || !validSwitchLiteral(record.AfterLiteral) || !validSwitchLiteral(record.PriorLiteral) ||
		!validAbsoluteSwitchPath(record.BeforeLocator) || !validAbsoluteSwitchPath(record.AfterLocator) ||
		!validAbsoluteSwitchPath(record.PriorStaticLocator) || !validAbsoluteSwitchPath(record.AdapterPath) ||
		!validAbsoluteSwitchPath(record.PigPath) || record.AdapterPath != filepath.Join(directory, "inventory") {
		return failure("CMDB_SWITCH_INVALID", "source-switch locator metadata is invalid")
	}
	if !switchLiteralResolves(record.ConfigPath, record.BeforeLiteral, record.BeforeLocator) ||
		!switchLiteralResolves(record.ConfigPath, record.AfterLiteral, record.AfterLocator) ||
		!switchLiteralResolves(record.ConfigPath, record.PriorLiteral, record.PriorStaticLocator) {
		return failure("CMDB_SWITCH_INVALID", "source-switch literal metadata is inconsistent")
	}
	if record.ConfigMode == 0 || record.ConfigMode&uint32(os.ModeType) != 0 {
		return failure("CMDB_SWITCH_INVALID", "source-switch config mode is invalid")
	}
	return nil
}

func switchRecordBytes(record SwitchRecord) []byte {
	body, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return append(body, '\n')
}

func writeNewSwitchArtifact(path string, body []byte, mode os.FileMode) error {
	parent := filepath.Dir(path)
	if !validPrivateBundleDirectory(parent) {
		return failure("CMDB_SWITCH_PERMISSION", "source-switch artifact directory is not private")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return failure("CMDB_SWITCH_CONFLICT", "source-switch artifact already exists")
	}
	temporary, err := os.CreateTemp(parent, ".pig-cmdb-switch-*")
	if err != nil {
		return failure("CMDB_SWITCH_FAILED", "source-switch artifact staging failed")
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return failure("CMDB_SWITCH_PERMISSION", "source-switch artifact mode cannot be preserved")
	}
	if _, err := io.Copy(temporary, bytes.NewReader(body)); err != nil {
		temporary.Close()
		return failure("CMDB_SWITCH_FAILED", "source-switch artifact write failed")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return failure("CMDB_SWITCH_FAILED", "source-switch artifact fsync failed")
	}
	if err := temporary.Close(); err != nil {
		return failure("CMDB_SWITCH_FAILED", "source-switch artifact close failed")
	}
	if err := os.Link(temporaryName, path); err != nil {
		return failure("CMDB_SWITCH_CONFLICT", "source-switch artifact commit failed")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return failure("CMDB_SWITCH_PARTIAL", "source-switch artifact exists but directory fsync failed")
	}
	if err := os.Remove(temporaryName); err != nil {
		return failure("CMDB_SWITCH_PARTIAL", "source-switch staging cleanup failed")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return failure("CMDB_SWITCH_PARTIAL", "source-switch artifact cleanup fsync failed")
	}
	return nil
}

func commitAnsibleConfigPatch(patch *ansibleConfigPatch, beforeCommit func() error) (bool, error) {
	if patch == nil || patch.document == nil {
		return false, failure("CMDB_SWITCH_INVALID", "source-switch config patch is incomplete")
	}
	document := patch.document
	path := document.source.ConfigPath
	parent := filepath.Dir(path)
	if !pathIsSymlinkFree(parent) {
		return false, ansibleConfigFailure(path)
	}
	temporary, err := os.CreateTemp(parent, ".pig-ansible-cfg-*")
	if err != nil {
		return false, failure("CMDB_SWITCH_FAILED", "ansible.cfg staging failed")
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(document.mode); err != nil {
		temporary.Close()
		return false, failure("CMDB_SWITCH_PERMISSION", "ansible.cfg mode cannot be preserved")
	}
	if _, err := io.Copy(temporary, bytes.NewReader(patch.candidate)); err != nil {
		temporary.Close()
		return false, failure("CMDB_SWITCH_FAILED", "ansible.cfg staging write failed")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return false, failure("CMDB_SWITCH_FAILED", "ansible.cfg staging fsync failed")
	}
	if err := temporary.Close(); err != nil {
		return false, failure("CMDB_SWITCH_FAILED", "ansible.cfg staging close failed")
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return false, failure("CMDB_SOURCE_CHANGED", "ansible.cfg pre-commit check failed")
		}
	}
	current, err := readAnsibleConfig(path)
	if err != nil || !bytes.Equal(current.body, document.body) || current.mode != document.mode ||
		current.ownerUID != document.ownerUID || current.ownerGID != document.ownerGID {
		return false, failure("CMDB_SOURCE_CHANGED", "ansible.cfg changed after source-switch planning")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return false, failure("CMDB_SWITCH_FAILED", "ansible.cfg atomic replacement failed")
	}
	if err := syncBundleDirectory(parent); err != nil {
		return false, failure("CMDB_SWITCH_PARTIAL", "ansible.cfg was replaced but parent fsync failed")
	}
	committed, err := readAnsibleConfig(path)
	if err != nil || !bytes.Equal(committed.body, patch.candidate) || committed.mode != document.mode {
		return false, failure("CMDB_SWITCH_PARTIAL", "ansible.cfg replacement could not be verified")
	}
	return committed.ownerUID != document.ownerUID || committed.ownerGID != document.ownerGID, nil
}

func readStableSwitchBackup(path string, limit int64) ([]byte, os.FileMode, error) {
	if !pathIsSymlinkFree(path) {
		return nil, 0, errors.New("unsafe backup path")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || !ownedByEffectiveUser(before) || before.Size() > limit {
		return nil, 0, errors.New("unsafe backup")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	body, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	pathAfter, pathErr := os.Lstat(path)
	if readErr != nil || statErr != nil || closeErr != nil || pathErr != nil || len(body) > int(limit) ||
		!sameSourceRevision(before, after) || !sameSourceRevision(after, pathAfter) {
		return nil, 0, errors.New("backup changed")
	}
	return body, before.Mode(), nil
}

func validSwitchID(value string) bool {
	if len(value) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && value == strings.ToLower(value)
}

func switchPathWithin(path, directory, prefix, suffix string) bool {
	if !filepath.IsAbs(path) || filepath.Dir(path) != directory {
		return false
	}
	base := filepath.Base(path)
	id := strings.TrimSuffix(strings.TrimPrefix(base, prefix), suffix)
	return strings.HasPrefix(base, prefix) && strings.HasSuffix(base, suffix) && validSwitchID(id)
}

func validSwitchLiteral(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func validAbsoluteSwitchPath(value string) bool {
	return safeRuntimePath(value) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func switchLiteralResolves(configPath, literal, locator string) bool {
	body := []byte("[defaults]\ninventory = " + literal + "\n")
	document, err := parseAnsibleConfig(configPath, body)
	return err == nil && document.source.Literal == literal && document.source.Locator == locator
}

func cloneSwitchRecord(record *SwitchRecord) *SwitchRecord {
	if record == nil {
		return nil
	}
	copy := *record
	return &copy
}
