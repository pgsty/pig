package sty

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
)

type CMDBInitOptions struct {
	PigstyHome     string
	RuntimeRoot    string
	PigBinary      string
	ConnectionFile string
	ToolVersion    string
}

type CMDBInitData struct {
	Kind             string                `json:"kind" yaml:"kind"`
	MigrationVersion int64                 `json:"migration_version" yaml:"migration_version"`
	Applied          int                   `json:"applied" yaml:"applied"`
	RevisionCount    int64                 `json:"revision_count" yaml:"revision_count"`
	Head             *cmdb.RevisionToken   `json:"head,omitempty" yaml:"head,omitempty"`
	ConnectionSource cmdb.ConnectionSource `json:"connection_source" yaml:"connection_source"`
	RuntimePath      string                `json:"runtime_path,omitempty" yaml:"runtime_path,omitempty"`
	AdapterIdentity  string                `json:"adapter_identity,omitempty" yaml:"adapter_identity,omitempty"`
	FindingCode      string                `json:"finding_code,omitempty" yaml:"finding_code,omitempty"`
}

func (d *CMDBInitData) Text() string {
	if d == nil {
		return ""
	}
	lines := []string{
		"  migration: " + fmt.Sprint(d.MigrationVersion),
		"  migrations applied: " + fmt.Sprint(d.Applied),
		"  revisions: " + fmt.Sprint(d.RevisionCount),
		"  head: " + formatCMDBToken(d.Head),
		"  connection source: " + string(d.ConnectionSource),
	}
	if d.RuntimePath != "" {
		lines = append(lines, "  runtime: "+d.RuntimePath)
	}
	if d.FindingCode != "" {
		lines = append(lines, "  finding: "+d.FindingCode)
	}
	return strings.Join(lines, "\n")
}

type cmdbInitializer interface {
	Init(context.Context, string) (cmdb.InitResult, error)
	Verify(context.Context) (cmdb.VerifiedState, error)
	Source() cmdb.ConnectionSource
}

func CMDBInit(ctx context.Context, store *cmdb.Store, options CMDBInitOptions) *output.Result {
	return cmdbInit(ctx, store, options, cmdb.PrepareRuntime)
}

func cmdbInit(ctx context.Context, store cmdbInitializer, options CMDBInitOptions, prepare func(cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error)) *output.Result {
	data := &CMDBInitData{Kind: "pig.sty.cmdb.init/v1"}
	if store == nil {
		data.FindingCode = "CMDB_CONNECTION_INVALID"
		return output.Fail(output.CodeStyCMDBConfigInvalid, "CMDB init requires an open native store").WithData(data)
	}
	data.ConnectionSource = store.Source()
	toolVersion := strings.TrimSpace(options.ToolVersion)
	if toolVersion == "" {
		toolVersion = config.PigVersion
	}
	migration, err := store.Init(ctx, toolVersion)
	if err != nil {
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB migrations could not be applied", err)
		result.Data = data
		return result
	}
	data.MigrationVersion = migration.MigrationVersion
	data.Applied = migration.Applied
	state, err := store.Verify(ctx)
	if err != nil {
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB state could not be verified after init", err)
		result.Data = data
		return result
	}
	data.RevisionCount = state.RevisionCount
	data.Head = cloneRevisionToken(state.Head)
	record, err := prepare(cmdb.RuntimeOptions{
		PigstyHome: options.PigstyHome, RuntimeRoot: options.RuntimeRoot,
		PigBinary: options.PigBinary, ConnectionSource: store.Source(),
		ConnectionFile: options.ConnectionFile,
	})
	if err != nil {
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB migrations are current but runtime adapter preparation failed", err)
		result.Data = data
		return result
	}
	data.RuntimePath = filepath.Join(filepath.Dir(record.AdapterPath), "runtime.json")
	data.AdapterIdentity = cmdb.RuntimeAdapterIdentity(record)
	return output.OK("CMDB initialized and runtime adapter verified", data)
}
