package cmdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"pig/inventory"
)

type SourceSwitchRequest struct {
	Operation   SourceSwitchOperation
	PigstyHome  string
	RuntimePath string
	CurrentPig  string
	StaticPath  string

	beforeConfigCommit func() error
	intentID           string
}

// SourceSwitchResult contains only file identities, byte revisions, adapter
// identity, and the exact verified head token.
type SourceSwitchResult struct {
	Kind             string                `json:"kind"`
	Operation        SourceSwitchOperation `json:"operation"`
	State            SourceSwitchState     `json:"state"`
	Changed          bool                  `json:"changed"`
	NoOp             bool                  `json:"no_op"`
	Head             *RevisionToken        `json:"head,omitempty"`
	ConfigPath       string                `json:"config_path"`
	BeforeSHA256     string                `json:"before_sha256,omitempty"`
	AfterSHA256      string                `json:"after_sha256,omitempty"`
	StaticLocator    string                `json:"static_locator,omitempty"`
	AdapterIdentity  string                `json:"adapter_identity"`
	IntentPath       string                `json:"intent_path,omitempty"`
	BackupPath       string                `json:"backup_path,omitempty"`
	OwnershipChanged bool                  `json:"ownership_changed"`
}

type switchStaticSource struct {
	path     string
	digest   [sha256.Size]byte
	snapshot *inventory.Snapshot
}

// SwitchSource applies one enable or disable request while a shared advisory
// lock pins the exact verified CMDB head through the filesystem commit.
func (s *Store) SwitchSource(ctx context.Context, request SourceSwitchRequest) (SourceSwitchResult, error) {
	result := SourceSwitchResult{Kind: SourceSwitchKind, Operation: request.Operation}
	if s == nil || s.pool == nil {
		return result, failure("CMDB_CONNECTION_INVALID", "database store is not open")
	}
	if request.Operation != SourceSwitchEnable && request.Operation != SourceSwitchDisable {
		return result, failure("CMDB_PARAMETER_INVALID", "source switch requires enable or disable")
	}
	if request.Operation == SourceSwitchEnable && request.StaticPath != "" {
		return result, failure("CMDB_PARAMETER_INVALID", "enable does not accept a fallback override")
	}
	home, err := canonicalRuntimeHome(request.PigstyHome)
	if err != nil {
		return result, err
	}
	runtimePath, err := canonicalExistingRuntimePath(request.RuntimePath)
	if err != nil {
		return result, err
	}
	currentPig := request.CurrentPig
	if currentPig == "" {
		currentPig, err = os.Executable()
		if err != nil {
			return result, failure("CMDB_RUNTIME_PIG_INVALID", "current Pig binary is unavailable")
		}
	}
	request.CurrentPig = currentPig
	adapter, err := VerifyRuntimeFile(runtimePath, request.CurrentPig)
	if err != nil {
		return result, err
	}
	if adapter.Home != home {
		return result, failure("CMDB_RUNTIME_COLLISION", "runtime adapter belongs to another Pigsty home")
	}
	configPath := filepath.Join(home, "ansible.cfg")
	document, err := readAnsibleConfig(configPath)
	if err != nil {
		return result, err
	}
	inspection, err := InspectSourceSwitch(runtimePath, configPath)
	if err != nil {
		return result, err
	}
	if inspection.State == SourceSwitchDrift {
		return result, failure("CMDB_SWITCH_DRIFT", "ansible.cfg matches neither recorded source-switch revision")
	}
	if inspection.State == SourceSwitchPending && inspection.Record != nil && inspection.Record.Operation != request.Operation {
		return result, failure("CMDB_SWITCH_CONFLICT", "a different source-switch operation is pending")
	}

	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "source-switch transaction start", err)
	}
	defer tx.Rollback(context.WithoutCancel(txCtx))
	if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock_shared($1)`, CMDBAdvisoryKey); err != nil {
		return result, sanitizeDatabaseFailure("CMDB_LOCK_FAILED", "source-switch shared lock", err)
	}
	if err := verifyMigrationLedger(txCtx, tx); err != nil {
		return result, err
	}
	if err := validateNativeSchema(txCtx, tx); err != nil {
		return result, err
	}
	chain, err := readVerifiedChainTx(txCtx, tx)
	if err != nil {
		return result, err
	}
	if chain.head == nil || len(chain.revisions) == 0 {
		return result, failure("CMDB_REVISION_NOT_FOUND", "native store has no active revision")
	}
	head := chain.revisions[len(chain.revisions)-1]
	projection, err := head.snapshot.AnsibleInventoryJSON()
	if err != nil || !json.Valid(projection) {
		return result, failure("CMDB_SNAPSHOT_INVALID", "active Snapshot cannot be projected by the native adapter")
	}
	projection = nil

	headToken := head.token
	result.Head = &headToken
	result.ConfigPath = configPath
	result.AdapterIdentity = RuntimeAdapterIdentity(adapter)
	var transition *ansibleConfigPatch
	var static switchStaticSource
	var priorLiteral, priorLocator string
	noOp := false
	switch request.Operation {
	case SourceSwitchEnable:
		transition, static, priorLiteral, priorLocator, noOp, err = planEnableSourceSwitch(document, head, adapter)
	case SourceSwitchDisable:
		transition, static, priorLiteral, priorLocator, noOp, err = planDisableSourceSwitch(
			document, head, adapter, home, request.StaticPath, inspection,
		)
	}
	if err != nil {
		return result, err
	}
	result.StaticLocator = priorLocator
	if noOp {
		result.NoOp = true
		result.State = SourceSwitchApplied
		before := sha256.Sum256(document.body)
		result.BeforeSHA256 = hex.EncodeToString(before[:])
		result.AfterSHA256 = result.BeforeSHA256
		if err := tx.Commit(txCtx); err != nil {
			return result, sanitizeDatabaseFailure("CMDB_TRANSACTION_FAILED", "source-switch no-op transaction commit", err)
		}
		return result, nil
	}

	if err := verifySwitchPrecommit(static, adapter, runtimePath, request.CurrentPig); err != nil {
		return result, err
	}
	var record SwitchRecord
	reused := false
	if inspection.State == SourceSwitchPending && inspection.Record != nil &&
		pendingSwitchMatches(*inspection.Record, request.Operation, home, runtimePath, head.token, adapter, document, transition, priorLiteral, priorLocator) {
		record = *inspection.Record
		reused = true
	} else if inspection.State == SourceSwitchPending {
		return result, failure("CMDB_SWITCH_CONFLICT", "pending source-switch intent does not match the requested operation")
	} else {
		record, err = createSourceSwitchIntent(document, transition, sourceSwitchIntentOptions{
			operation: request.Operation, home: home, runtimePath: runtimePath,
			head: head.token, adapter: adapter,
			priorLiteral: priorLiteral, priorStaticLocator: priorLocator,
			previous: inspection.Record, intentID: request.intentID,
		})
		if err != nil {
			return result, err
		}
	}
	if reused && (record.BeforeSHA256 != document.source.RevisionSHA256 || record.AfterSHA256 != hex.EncodeToString(transition.afterDigest[:])) {
		return result, failure("CMDB_SWITCH_CONFLICT", "pending source-switch byte revisions no longer match")
	}

	ownershipChanged, err := commitAnsibleConfigPatch(transition, func() error {
		if request.beforeConfigCommit != nil {
			if err := request.beforeConfigCommit(); err != nil {
				return err
			}
		}
		return verifySwitchPrecommit(static, adapter, runtimePath, request.CurrentPig)
	})
	result.BeforeSHA256 = record.BeforeSHA256
	result.AfterSHA256 = record.AfterSHA256
	result.IntentPath = record.IntentPath
	result.BackupPath = record.BackupPath
	if err != nil {
		return result, err
	}
	result.Changed = true
	result.State = SourceSwitchApplied
	result.OwnershipChanged = ownershipChanged
	if err := tx.Commit(txCtx); err != nil {
		return result, failure("CMDB_SWITCH_PARTIAL", "ansible.cfg was switched but the database read transaction did not close cleanly")
	}
	return result, nil
}

func planEnableSourceSwitch(document *ansibleConfigDocument, head *Revision, adapter RuntimeRecord) (*ansibleConfigPatch, switchStaticSource, string, string, bool, error) {
	if document.source.Locator == adapter.AdapterPath {
		return nil, switchStaticSource{}, "", "", true, nil
	}
	static, err := readSwitchStaticSource(document.source.Locator)
	if err != nil {
		return nil, switchStaticSource{}, "", "", false, err
	}
	if !static.snapshot.Equal(head.snapshot) {
		return nil, switchStaticSource{}, "", "", false, failure("CMDB_SOURCE_HEAD_MISMATCH", "static Inventory declarations do not match the current CMDB head")
	}
	patch, err := planAnsibleConfigPatch(document, adapter.AdapterPath)
	if err != nil {
		return nil, switchStaticSource{}, "", "", false, err
	}
	return patch, static, document.source.Literal, document.source.Locator, false, nil
}

func planDisableSourceSwitch(document *ansibleConfigDocument, head *Revision, adapter RuntimeRecord, home, explicitStatic string, inspection SourceSwitchInspection) (*ansibleConfigPatch, switchStaticSource, string, string, bool, error) {
	priorLiteral := ""
	priorLocator := ""
	if explicitStatic != "" {
		absolute, err := filepath.Abs(explicitStatic)
		if err != nil || filepath.Clean(absolute) != absolute {
			return nil, switchStaticSource{}, "", "", false, failure("CMDB_PARAMETER_INVALID", "explicit static fallback path is invalid")
		}
		priorLocator = absolute
		priorLiteral, err = renderAnsibleInventoryLiteral(document.source.ConfigPath, priorLocator)
		if err != nil {
			return nil, switchStaticSource{}, "", "", false, ansibleConfigFailure(document.source.ConfigPath)
		}
	} else {
		if inspection.Record == nil {
			return nil, switchStaticSource{}, "", "", false, failure("CMDB_STATIC_REQUIRED", "disable requires an explicit static fallback without a valid switch record")
		}
		priorLiteral = inspection.Record.PriorLiteral
		priorLocator = inspection.Record.PriorStaticLocator
	}
	static, err := readSwitchStaticSource(priorLocator)
	if err != nil {
		return nil, switchStaticSource{}, "", "", false, err
	}
	if !static.snapshot.Equal(head.snapshot) {
		return nil, switchStaticSource{}, "", "", false, failure("CMDB_SOURCE_HEAD_MISMATCH", "static fallback declarations do not match the current CMDB head")
	}

	legacy := filepath.Join(home, "inventory.sh")
	switch document.source.Locator {
	case adapter.AdapterPath:
		patch, err := planAnsibleConfigLiteralPatch(document, priorLiteral, priorLocator)
		return patch, static, priorLiteral, priorLocator, false, err
	case legacy:
		if explicitStatic == "" {
			return nil, switchStaticSource{}, "", "", false, failure("CMDB_STATIC_REQUIRED", "legacy Inventory migration requires an explicit static fallback")
		}
		patch, err := planAnsibleConfigLiteralPatch(document, priorLiteral, priorLocator)
		return patch, static, priorLiteral, priorLocator, false, err
	default:
		current, currentErr := readSwitchStaticSource(document.source.Locator)
		if currentErr != nil {
			return nil, switchStaticSource{}, "", "", false, failure("CMDB_SWITCH_STATE_CONFLICT", "active Inventory source is neither the verified adapter nor a supported static source")
		}
		if !current.snapshot.Equal(head.snapshot) {
			return nil, switchStaticSource{}, "", "", false, failure("CMDB_SOURCE_HEAD_MISMATCH", "active static Inventory declarations do not match the current CMDB head")
		}
		if document.source.Locator == priorLocator {
			return nil, static, priorLiteral, priorLocator, true, nil
		}
		return nil, switchStaticSource{}, "", "", false, failure("CMDB_SWITCH_STATE_CONFLICT", "disable cannot replace one unrelated static source with another")
	}
}

func readSwitchStaticSource(path string) (switchStaticSource, error) {
	var result switchStaticSource
	if !validAbsoluteSwitchPath(path) || !pathIsSymlinkFree(path) {
		return result, failure("CMDB_SOURCE_INVALID", "static Inventory must be one canonical symlink-free file")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return result, failure("CMDB_SOURCE_INVALID", "static Inventory must be one canonical symlink-free file")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o111 != 0 || before.Size() < 0 || before.Size() > maxMergeSourceSize {
		return result, failure("CMDB_SOURCE_INVALID", "static Inventory must be one bounded non-executable regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return result, failure("CMDB_SOURCE_INVALID", "static Inventory cannot be opened")
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxMergeSourceSize+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	pathAfter, pathErr := os.Lstat(path)
	if readErr != nil || statErr != nil || closeErr != nil || pathErr != nil || len(body) > maxMergeSourceSize ||
		!sameSourceRevision(before, after) || !sameSourceRevision(after, pathAfter) {
		return result, failure("CMDB_SOURCE_CHANGED", "static Inventory changed during inspection")
	}
	document, err := inventory.Parse(path, body)
	if err != nil {
		return result, failure("CMDB_SOURCE_INVALID", "static Inventory YAML is invalid")
	}
	diagnostics := document.Validate(
		inventory.YAMLRules, inventory.AnsibleRules, inventory.PigstyRules, inventory.CompleteInventoryRules,
	)
	if inventory.HasErrors(diagnostics) {
		return result, failure("CMDB_SOURCE_INVALID", "static Inventory validation failed")
	}
	snapshot, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		var snapshotError *inventory.SnapshotError
		if errors.As(err, &snapshotError) {
			return result, failure("CMDB_SNAPSHOT_UNREPRESENTABLE", "static Inventory is not representable")
		}
		return result, failure("CMDB_SNAPSHOT_INVALID", "static Inventory Snapshot could not be created")
	}
	result.path = path
	result.digest = sha256.Sum256(body)
	result.snapshot = snapshot
	return result, nil
}

func verifySwitchPrecommit(static switchStaticSource, adapter RuntimeRecord, runtimePath, currentPig string) error {
	verified, err := VerifyRuntimeFile(runtimePath, currentPig)
	if err != nil || verified.Home != adapter.Home || verified.AdapterPath != adapter.AdapterPath ||
		verified.AdapterSHA256 != adapter.AdapterSHA256 || verified.PigPath != adapter.PigPath || verified.PigSHA256 != adapter.PigSHA256 {
		return failure("CMDB_RUNTIME_ADAPTER_INVALID", "runtime adapter changed during source-switch planning")
	}
	if static.path != "" {
		current, err := readSwitchStaticSource(static.path)
		if err != nil || current.digest != static.digest || !current.snapshot.Equal(static.snapshot) {
			return failure("CMDB_SOURCE_CHANGED", "static Inventory changed during source-switch planning")
		}
	}
	return nil
}

func pendingSwitchMatches(record SwitchRecord, operation SourceSwitchOperation, home, runtimePath string, head RevisionToken, adapter RuntimeRecord, document *ansibleConfigDocument, patch *ansibleConfigPatch, priorLiteral, priorLocator string) bool {
	if document == nil || patch == nil {
		return false
	}
	before := sha256.Sum256(document.body)
	return record.Operation == operation && record.Home == home && record.RuntimePath == runtimePath &&
		record.ConfigPath == document.source.ConfigPath && record.BeforeSHA256 == hex.EncodeToString(before[:]) &&
		record.AfterSHA256 == hex.EncodeToString(patch.afterDigest[:]) && record.BeforeLiteral == document.source.Literal &&
		record.BeforeLocator == document.source.Locator && record.AfterLiteral == patch.nextLiteral && record.AfterLocator == patch.nextLocator &&
		record.PriorLiteral == priorLiteral && record.PriorStaticLocator == priorLocator && record.Head == head.String() &&
		record.AdapterPath == adapter.AdapterPath && record.AdapterSHA256 == adapter.AdapterSHA256 &&
		record.PigPath == adapter.PigPath && record.PigSHA256 == adapter.PigSHA256
}
