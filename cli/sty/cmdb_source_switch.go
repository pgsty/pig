package sty

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"pig/internal/cmdb"
	"pig/internal/output"
)

type CMDBSourceSwitchOptions struct {
	PigstyHome       string
	RuntimeRoot      string
	RuntimePath      string
	PigBinary        string
	StaticPath       string
	WorkingDirectory string
}

type CMDBSourceSwitchData struct {
	Kind             string                     `json:"kind" yaml:"kind"`
	Operation        cmdb.SourceSwitchOperation `json:"operation" yaml:"operation"`
	State            cmdb.SourceSwitchState     `json:"state" yaml:"state"`
	Changed          bool                       `json:"changed" yaml:"changed"`
	NoOp             bool                       `json:"no_op" yaml:"no_op"`
	Head             *cmdb.RevisionToken        `json:"head,omitempty" yaml:"head,omitempty"`
	ConfigPath       string                     `json:"config_path" yaml:"config_path"`
	BeforeSHA256     string                     `json:"before_sha256,omitempty" yaml:"before_sha256,omitempty"`
	AfterSHA256      string                     `json:"after_sha256,omitempty" yaml:"after_sha256,omitempty"`
	StaticLocator    string                     `json:"static_locator,omitempty" yaml:"static_locator,omitempty"`
	AdapterIdentity  string                     `json:"adapter_identity" yaml:"adapter_identity"`
	IntentPath       string                     `json:"intent_path,omitempty" yaml:"intent_path,omitempty"`
	BackupPath       string                     `json:"backup_path,omitempty" yaml:"backup_path,omitempty"`
	OwnershipChanged bool                       `json:"ownership_changed" yaml:"ownership_changed"`
	FindingCode      string                     `json:"finding_code,omitempty" yaml:"finding_code,omitempty"`
}

func (d *CMDBSourceSwitchData) Text() string {
	if d == nil {
		return ""
	}
	lines := []string{
		"  operation: " + string(d.Operation),
		"  state: " + string(d.State),
		"  head: " + formatCMDBToken(d.Head),
		"  config: " + d.ConfigPath,
		"  changed: " + fmt.Sprint(d.Changed),
	}
	if d.StaticLocator != "" {
		lines = append(lines, "  static source: "+d.StaticLocator)
	}
	if d.BackupPath != "" {
		lines = append(lines, "  backup: "+d.BackupPath)
	}
	if d.OwnershipChanged {
		lines = append(lines, "  ownership changed: true")
	}
	if d.FindingCode != "" {
		lines = append(lines, "  finding: "+d.FindingCode)
	}
	return strings.Join(lines, "\n")
}

func CMDBEnable(ctx context.Context, store *cmdb.Store, options CMDBSourceSwitchOptions) *output.Result {
	return cmdbSourceSwitch(ctx, store, cmdb.SourceSwitchEnable, options)
}

func CMDBDisable(ctx context.Context, store *cmdb.Store, options CMDBSourceSwitchOptions) *output.Result {
	return cmdbSourceSwitch(ctx, store, cmdb.SourceSwitchDisable, options)
}

type cmdbSourceSwitcher interface {
	SwitchSource(context.Context, cmdb.SourceSwitchRequest) (cmdb.SourceSwitchResult, error)
}

func cmdbSourceSwitch(ctx context.Context, store cmdbSourceSwitcher, operation cmdb.SourceSwitchOperation, options CMDBSourceSwitchOptions) *output.Result {
	if store == nil {
		return cmdbFailureResult("CMDB source switch requires an open native store", &cmdb.Error{
			Code: "CMDB_CONNECTION_INVALID", Message: "database store is not open",
		})
	}
	runtimePath := options.RuntimePath
	if runtimePath == "" {
		var err error
		runtimePath, err = cmdb.RuntimeRecordPath(options.PigstyHome, options.RuntimeRoot)
		if err != nil {
			return cmdbFailureResult("CMDB runtime identity is invalid", err)
		}
	}
	staticPath := options.StaticPath
	if operation == cmdb.SourceSwitchDisable && staticPath != "" {
		var ok bool
		staticPath, ok = resolveCMDBSwitchStaticPath(staticPath, options.WorkingDirectory)
		if !ok {
			return cmdbFailureResult("CMDB static fallback path is invalid", &cmdb.Error{
				Code: "CMDB_PARAMETER_INVALID", Message: "static fallback path is invalid",
			})
		}
	}
	native, err := store.SwitchSource(ctx, cmdb.SourceSwitchRequest{
		Operation: operation, PigstyHome: options.PigstyHome,
		RuntimePath: runtimePath, CurrentPig: options.PigBinary, StaticPath: staticPath,
	})
	data := newCMDBSourceSwitchData(native)
	if err != nil {
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB source switch did not complete cleanly", err)
		result.Data = data
		return result
	}
	message := "CMDB source switch applied"
	if native.NoOp {
		message = "CMDB source is already in the requested state"
	}
	return output.OK(message, data)
}

func newCMDBSourceSwitchData(native cmdb.SourceSwitchResult) *CMDBSourceSwitchData {
	return &CMDBSourceSwitchData{
		Kind: native.Kind, Operation: native.Operation, State: native.State,
		Changed: native.Changed, NoOp: native.NoOp, Head: cloneRevisionToken(native.Head),
		ConfigPath: native.ConfigPath, BeforeSHA256: native.BeforeSHA256, AfterSHA256: native.AfterSHA256,
		StaticLocator: native.StaticLocator, AdapterIdentity: native.AdapterIdentity,
		IntentPath: native.IntentPath, BackupPath: native.BackupPath,
		OwnershipChanged: native.OwnershipChanged,
	}
}

func resolveCMDBSwitchStaticPath(name, workingDirectory string) (string, bool) {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n") {
		return "", false
	}
	path := name
	if !filepath.IsAbs(path) {
		base := workingDirectory
		if base == "" {
			var err error
			base, err = filepath.Abs(".")
			if err != nil {
				return "", false
			}
		}
		path = filepath.Join(base, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(absolute), true
}
