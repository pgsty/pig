package sty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"pig/internal/cmdb"
	"pig/internal/output"
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
	Kind             string                `json:"kind" yaml:"kind"`
	ConnectionSource cmdb.ConnectionSource `json:"connection_source,omitempty" yaml:"connection_source,omitempty"`
	MigrationVersion int64                 `json:"migration_version" yaml:"migration_version"`
	RevisionCount    int64                 `json:"revision_count" yaml:"revision_count"`
	Head             *cmdb.RevisionToken   `json:"head,omitempty" yaml:"head,omitempty"`
	Healthy          bool                  `json:"healthy" yaml:"healthy"`
	Findings         []CMDBFinding         `json:"findings" yaml:"findings"`
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
	rows := make([][]string, 0, len(d.Findings))
	for _, finding := range d.Findings {
		rows = append(rows, []string{finding.Code, finding.Status})
	}
	if len(rows) > 0 {
		lines = append(lines, strings.TrimSuffix(output.RenderTable([]string{"FINDING", "STATUS"}, rows), "\n"))
	}
	return strings.Join(lines, "\n")
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
