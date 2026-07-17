package sty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

const (
	CMDBCheckKind = "pig.sty.cmdb.check/v1"

	CMDBStaticShadowsPersistentNotice = "NOTICE: invocation static Inventory shadows persistent CMDB source from ansible.cfg; using the invocation source"
	CMDBNativeShadowsPersistentNotice = "NOTICE: invocation CMDB source shadows persistent static Inventory from ansible.cfg; using the invocation source"
)

type CMDBFinding struct {
	Code   string `json:"code" yaml:"code"`
	Status string `json:"status" yaml:"status"`
}

type CMDBCheckData struct {
	Kind             string                 `json:"kind" yaml:"kind"`
	ConnectionSource cmdb.ConnectionSource  `json:"connection_source,omitempty" yaml:"connection_source,omitempty"`
	MigrationVersion int64                  `json:"migration_version" yaml:"migration_version"`
	RevisionCount    int64                  `json:"revision_count" yaml:"revision_count"`
	Head             *cmdb.RevisionToken    `json:"head,omitempty" yaml:"head,omitempty"`
	RuntimePath      string                 `json:"runtime_path,omitempty" yaml:"runtime_path,omitempty"`
	AdapterIdentity  string                 `json:"adapter_identity,omitempty" yaml:"adapter_identity,omitempty"`
	EffectiveSource  CMDBSourceDescriptor   `json:"effective_source" yaml:"effective_source"`
	PersistentSource CMDBSourceDescriptor   `json:"persistent_source" yaml:"persistent_source"`
	SwitchState      cmdb.SourceSwitchState `json:"switch_state,omitempty" yaml:"switch_state,omitempty"`
	Healthy          bool                   `json:"healthy" yaml:"healthy"`
	Findings         []CMDBFinding          `json:"findings" yaml:"findings"`
}

func (d *CMDBCheckData) Text() string {
	if d == nil {
		return ""
	}
	lines := []string{
		"  connection source: " + string(d.ConnectionSource),
		"  migration: " + fmt.Sprint(d.MigrationVersion),
		"  revisions: " + fmt.Sprint(d.RevisionCount),
		"  head: " + formatCMDBToken(d.Head),
	}
	if d.RuntimePath != "" {
		lines = append(lines, "  runtime: "+d.RuntimePath)
	}
	if d.EffectiveSource.Kind != "" {
		lines = append(lines, "  effective source: "+string(d.EffectiveSource.Kind)+" ("+string(d.EffectiveSource.Origin)+")")
	}
	if d.SwitchState != "" {
		lines = append(lines, "  switch state: "+string(d.SwitchState))
	}
	rows := make([][]string, 0, len(d.Findings))
	for _, finding := range d.Findings {
		rows = append(rows, []string{finding.Code, finding.Status})
	}
	if len(rows) > 0 {
		lines = append(lines, strings.TrimSuffix(output.RenderTable([]string{"FINDING", "STATUS"}, rows), "\n"))
	}
	return strings.Join(lines, "\n")
}

type CMDBCheckOptions struct {
	PigstyHome          string
	InvocationInventory string
	WorkingDirectory    string
	UserHome            string
	UserConfigPath      string
	RuntimeRoot         string
	RuntimePath         string
	PigBinary           string
	LookupEnv           func(string) (string, bool)
}

// CMDBCheck performs the complete P2.4 read-only check. It never prepares a
// runtime, writes an intent, repairs a config, or executes an adapter.
func CMDBCheck(ctx context.Context, store *cmdb.Store, options CMDBCheckOptions) *output.Result {
	if store == nil {
		return cmdbCheckFull(ctx, nil, options)
	}
	return cmdbCheckFull(ctx, nativeCMDBCheckReader{store: store}, options)
}

type cmdbFullCheckReader interface {
	Verify(context.Context) (cmdb.VerifiedState, error)
	Source() cmdb.ConnectionSource
	ReadHeadSnapshot(context.Context) (*inventory.Snapshot, cmdb.RevisionToken, error)
}

type nativeCMDBCheckReader struct {
	store *cmdb.Store
}

func (r nativeCMDBCheckReader) Verify(ctx context.Context) (cmdb.VerifiedState, error) {
	return r.store.Verify(ctx)
}

func (r nativeCMDBCheckReader) Source() cmdb.ConnectionSource { return r.store.Source() }

func (r nativeCMDBCheckReader) ReadHeadSnapshot(ctx context.Context) (*inventory.Snapshot, cmdb.RevisionToken, error) {
	revision, err := r.store.ReadHead(ctx)
	if err != nil || revision == nil {
		return nil, cmdb.RevisionToken{}, err
	}
	return revision.Snapshot(), revision.Token(), nil
}

func cmdbCheckFull(ctx context.Context, store cmdbFullCheckReader, options CMDBCheckOptions) *output.Result {
	data := &CMDBCheckData{Kind: CMDBCheckKind, Findings: []CMDBFinding{}}
	status := 0
	findingIndexes := make(map[string]int)
	appendError := func(code string, codeStatus int) {
		if index, exists := findingIndexes[code]; exists {
			data.Findings[index].Status = "error"
			if status == 0 {
				status = codeStatus
			}
			return
		}
		findingIndexes[code] = len(data.Findings)
		data.Findings = append(data.Findings, CMDBFinding{Code: code, Status: "error"})
		if status == 0 {
			status = codeStatus
		}
	}
	appendPass := func(code string) {
		if _, exists := findingIndexes[code]; exists {
			return
		}
		findingIndexes[code] = len(data.Findings)
		data.Findings = append(data.Findings, CMDBFinding{Code: code, Status: "pass"})
	}

	var headSnapshot *inventory.Snapshot
	if store == nil {
		appendError("CMDB_CONNECTION_INVALID", output.CodeStyCMDBConfigInvalid)
	} else {
		data.ConnectionSource = store.Source()
		state, err := store.Verify(ctx)
		if err != nil {
			code, mapped := cmdbFailureCode(err)
			appendError(code, mapped)
		} else {
			data.MigrationVersion = state.MigrationVersion
			data.RevisionCount = state.RevisionCount
			data.Head = cloneRevisionToken(state.Head)
			appendPass("CMDB_CONNECTION_OK")
			appendPass("CMDB_MIGRATION_OK")
			appendPass("CMDB_HISTORY_OK")
			if state.Head == nil {
				appendError("CMDB_HEAD_EMPTY", output.CodeStyCMDBResourceMissing)
			} else {
				readSnapshot, headToken, readErr := store.ReadHeadSnapshot(ctx)
				if readErr != nil || readSnapshot == nil || !headToken.Equal(*state.Head) {
					appendError("CMDB_REVISION_CONFLICT", output.CodeStyCMDBRevisionConflict)
				} else {
					headSnapshot = readSnapshot
					appendPass("CMDB_HEAD_OK")
				}
			}
		}
	}

	runtimePath := options.RuntimePath
	if runtimePath == "" && options.PigstyHome != "" {
		if resolved, err := cmdb.RuntimeRecordPath(options.PigstyHome, options.RuntimeRoot); err == nil {
			runtimePath = resolved
		}
	}
	data.RuntimePath = runtimePath
	currentPig := options.PigBinary
	if currentPig == "" {
		currentPig, _ = os.Executable()
	}
	var runtimeRecord cmdb.RuntimeRecord
	if runtimePath == "" {
		appendError("CMDB_RUNTIME_INVALID", output.CodeStyCMDBConfigInvalid)
	} else {
		verified, err := cmdb.VerifyRuntimeFile(runtimePath, currentPig)
		if err != nil {
			code, mapped := cmdbFailureCode(err)
			appendError(code, mapped)
		} else {
			runtimeRecord = verified
			data.AdapterIdentity = cmdb.RuntimeAdapterIdentity(verified)
			appendPass("CMDB_RUNTIME_OK")
		}
	}

	resolution, sourceFailure := ResolveCMDBSource(CMDBSourceResolveOptions{
		PigstyHome: options.PigstyHome, InvocationInventory: options.InvocationInventory,
		WorkingDirectory: options.WorkingDirectory, UserHome: options.UserHome,
		UserConfigPath: options.UserConfigPath, RuntimeRoot: options.RuntimeRoot,
		RuntimePath: runtimePath, PigBinary: currentPig, LookupEnv: options.LookupEnv,
	})
	data.EffectiveSource = resolution.Effective
	data.PersistentSource = resolution.Persistent
	if resolution.PersistentFinding != "" {
		appendError(resolution.PersistentFinding, output.CodeStyCMDBConfigInvalid)
	} else {
		appendPass("CMDB_ANSIBLE_CONFIG_OK")
	}
	if sourceFailure != nil {
		code := cmdbResultFindingCode(sourceFailure)
		appendError(code, sourceFailure.Code)
	} else {
		appendPass("CMDB_SOURCE_OK")
	}

	if runtimePath != "" && options.PigstyHome != "" {
		checkHome := options.PigstyHome
		if runtimeRecord.Home != "" {
			checkHome = runtimeRecord.Home
		}
		configPath := filepath.Join(checkHome, "ansible.cfg")
		inspection, err := cmdb.InspectSourceSwitch(runtimePath, configPath)
		if err != nil {
			code, mapped := cmdbFailureCode(err)
			appendError(code, mapped)
		} else {
			data.SwitchState = inspection.State
			switch inspection.State {
			case cmdb.SourceSwitchPending:
				appendError("CMDB_SWITCH_PENDING", output.CodeStyCMDBDrift)
			case cmdb.SourceSwitchDrift:
				appendError("CMDB_SWITCH_DRIFT", output.CodeStyCMDBDrift)
			case cmdb.SourceSwitchApplied:
				appendPass("CMDB_SWITCH_APPLIED")
			case cmdb.SourceSwitchNone:
				if resolution.Persistent.Kind == CMDBSourceNative {
					appendError("CMDB_SWITCH_RECORD_MISSING", output.CodeStyCMDBDrift)
				} else {
					appendPass("CMDB_SWITCH_NONE")
				}
			}
		}
	}

	if sourceFailure == nil && headSnapshot != nil {
		switch resolution.Effective.Kind {
		case CMDBSourceStatic:
			load, failure := prepareCMDBStaticLoad(resolution.Effective.Locator, "", "cmdb-check")
			if failure != nil {
				appendError(cmdbResultFindingCode(failure), failure.Code)
			} else if !load.Snapshot.Equal(headSnapshot) {
				appendError("CMDB_SOURCE_HEAD_MISMATCH", output.CodeStyCMDBDrift)
			} else {
				appendPass("CMDB_STATIC_PARITY_OK")
			}
		case CMDBSourceNative:
			body, err := headSnapshot.AnsibleInventoryJSON()
			if err != nil || !json.Valid(body) || runtimeRecord.AdapterPath == "" ||
				resolution.Effective.Locator != cmdb.RuntimeAdapterIdentity(runtimeRecord) {
				appendError("CMDB_ADAPTER_PARITY_INVALID", output.CodeStyCMDBDrift)
			} else {
				appendPass("CMDB_ADAPTER_PARITY_OK")
			}
			body = nil
		default:
			appendError("CMDB_SOURCE_UNSUPPORTED", output.CodeStyCMDBConfigInvalid)
		}
	}

	data.Healthy = status == 0
	if !data.Healthy {
		if status == 0 {
			status = output.CodeStyCMDBDrift
		}
		return output.Fail(status, "CMDB check found one or more redacted failures").WithData(data)
	}
	return output.OK("CMDB source and native state are healthy", data)
}

func cmdbResultFindingCode(result *output.Result) string {
	if result == nil {
		return "CMDB_INTERNAL"
	}
	if data, ok := result.Data.(*CMDBFailureData); ok && data != nil && data.CMDBCode != "" {
		return data.CMDBCode
	}
	switch result.Code {
	case output.CodeStyCMDBSnapshotUnrepresentable:
		return "CMDB_SNAPSHOT_UNREPRESENTABLE"
	case output.CodeStyCMDBResourceMissing:
		return "CMDB_SOURCE_MISSING"
	case output.CodeStyCMDBSourceChanged:
		return "CMDB_SOURCE_CHANGED"
	case output.CodeStyCMDBConfigInvalid, output.CodeStyCMDBInvalidArgs:
		return "CMDB_SOURCE_INVALID"
	}
	return "CMDB_INTERNAL"
}

// CMDBCheckNative performs the P2.3 read-only native-store portion of check.
// P2.4 extends the same DTO with runtime-adapter, source-shape, switch-record,
// and active parity findings after those contracts are implemented.
func CMDBCheckNative(ctx context.Context, store *cmdb.Store) *output.Result {
	return cmdbCheckNative(ctx, store)
}

type cmdbNativeVerifier interface {
	Verify(context.Context) (cmdb.VerifiedState, error)
	Source() cmdb.ConnectionSource
}

func cmdbCheckNative(ctx context.Context, store cmdbNativeVerifier) *output.Result {
	data := &CMDBCheckData{
		Kind: CMDBCheckKind, Healthy: false, Findings: []CMDBFinding{},
	}
	if store == nil {
		data.Findings = append(data.Findings, CMDBFinding{Code: "CMDB_CONNECTION_INVALID", Status: "error"})
		return output.Fail(output.CodeStyCMDBConfigInvalid, "CMDB native check failed").WithData(data)
	}
	if store != nil {
		data.ConnectionSource = store.Source()
	}
	state, err := store.Verify(ctx)
	if err != nil {
		finding, status := cmdbFailureCode(err)
		data.Findings = append(data.Findings, CMDBFinding{Code: finding, Status: "error"})
		result := cmdbFailureResult("CMDB native check failed", err)
		result.Code = status
		result.Data = data
		return result
	}
	data.MigrationVersion = state.MigrationVersion
	data.RevisionCount = state.RevisionCount
	data.Head = cloneRevisionToken(state.Head)
	data.Findings = append(data.Findings,
		CMDBFinding{Code: "CMDB_CONNECTION_OK", Status: "pass"},
		CMDBFinding{Code: "CMDB_MIGRATION_OK", Status: "pass"},
		CMDBFinding{Code: "CMDB_HISTORY_OK", Status: "pass"},
	)
	if state.Head == nil {
		data.Findings = append(data.Findings, CMDBFinding{Code: "CMDB_HEAD_EMPTY", Status: "error"})
		return output.Fail(output.CodeStyCMDBResourceMissing, "CMDB native store has no active revision").WithData(data)
	}
	data.Healthy = true
	return output.OK("CMDB native store is healthy", data)
}

type CMDBSourceKind string
type CMDBSourceOrigin string

const (
	CMDBSourceStatic      CMDBSourceKind = "static"
	CMDBSourceNative      CMDBSourceKind = "cmdb"
	CMDBSourceUnsupported CMDBSourceKind = "unsupported"

	CMDBSourceCLI           CMDBSourceOrigin = "cli"
	CMDBSourceEnvironment   CMDBSourceOrigin = "environment"
	CMDBSourceUserConfig    CMDBSourceOrigin = "user-config"
	CMDBSourceAnsibleConfig CMDBSourceOrigin = "ansible-config"
	CMDBSourceHomeDefault   CMDBSourceOrigin = "home-default"
)

type CMDBSourceDescriptor struct {
	Kind       CMDBSourceKind   `json:"kind" yaml:"kind"`
	Origin     CMDBSourceOrigin `json:"origin" yaml:"origin"`
	Locator    string           `json:"locator" yaml:"locator"`
	Persistent bool             `json:"persistent" yaml:"persistent"`
}

// CMDBSplitBrainNotice returns only one of the RFC's exact notice lines. The
// persistent ansible.cfg descriptor remains authoritative; a notice exists
// only for a higher-precedence invocation descriptor of the opposite kind.
func CMDBSplitBrainNotice(invocation, persistent CMDBSourceDescriptor) string {
	if invocation.Persistent || !persistent.Persistent {
		return ""
	}
	switch {
	case invocation.Kind == CMDBSourceStatic && persistent.Kind == CMDBSourceNative:
		return CMDBStaticShadowsPersistentNotice
	case invocation.Kind == CMDBSourceNative && persistent.Kind == CMDBSourceStatic:
		return CMDBNativeShadowsPersistentNotice
	default:
		return ""
	}
}

// CMDBNoticeEmitter keeps the split-brain notice on its caller-provided stderr
// surface and deduplicates repeated source reads during one command.
type CMDBNoticeEmitter struct {
	mu      sync.Mutex
	writer  io.Writer
	emitted bool
}

func NewCMDBNoticeEmitter(stderr io.Writer) *CMDBNoticeEmitter {
	return &CMDBNoticeEmitter{writer: stderr}
}

func (e *CMDBNoticeEmitter) Emit(invocation, persistent CMDBSourceDescriptor) error {
	notice := CMDBSplitBrainNotice(invocation, persistent)
	if notice == "" {
		return nil
	}
	if e == nil || e.writer == nil {
		return errors.New("CMDB notice stderr is unavailable")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.emitted {
		return nil
	}
	written, err := io.WriteString(e.writer, notice+"\n")
	if err != nil {
		return err
	}
	if written != len(notice)+1 {
		return io.ErrShortWrite
	}
	e.emitted = true
	return nil
}
