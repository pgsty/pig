package sty

import (
	"context"
	"errors"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

const (
	CMDBFailureKind    = "pig.sty.cmdb.failure/v1"
	CMDBValidationKind = "pig.sty.cmdb.validation/v1"
)

// CMDBConnectionOptions identifies only the protected connection source. A
// password-bearing value is resolved and retained inside internal/cmdb.
type CMDBConnectionOptions struct {
	ConnectionFile string
}

// CMDBFailureData exposes one stable redacted finding code, never a driver
// error, DSN, SQL statement, Inventory value, or body.
type CMDBFailureData struct {
	Kind     string `json:"kind" yaml:"kind"`
	CMDBCode string `json:"cmdb_code" yaml:"cmdb_code"`
}

func (d *CMDBFailureData) Text() string {
	if d == nil || d.CMDBCode == "" {
		return ""
	}
	return "  finding: " + d.CMDBCode
}

// CMDBValidationData is safe validation metadata for one static source. It
// intentionally excludes the Document, Snapshot, source bytes, and values.
type CMDBValidationData struct {
	Kind             string                   `json:"kind" yaml:"kind"`
	Path             string                   `json:"path" yaml:"path"`
	Diagnostics      []inventory.Diagnostic   `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
	Representability *inventory.SnapshotError `json:"representability,omitempty" yaml:"representability,omitempty"`
}

func (d *CMDBValidationData) Text() string {
	if d == nil {
		return ""
	}
	rows := make([][]string, 0, len(d.Diagnostics)+1)
	for _, diagnostic := range d.Diagnostics {
		rows = append(rows, []string{diagnostic.Code, diagnostic.Path.String(), string(diagnostic.Severity)})
	}
	if d.Representability != nil {
		rows = append(rows, []string{d.Representability.Code, d.Representability.Path.String(), string(d.Representability.Reason)})
	}
	if len(rows) == 0 {
		return ""
	}
	return strings.TrimSuffix(output.RenderTable([]string{"CODE", "PATH", "STATUS"}, rows), "\n")
}

func OpenCMDBStore(ctx context.Context, options CMDBConnectionOptions) (*cmdb.Store, *output.Result) {
	resolved, err := cmdb.ResolveConnection(cmdb.ResolveOptions{ConnectionFile: options.ConnectionFile})
	if err != nil {
		return nil, cmdbFailureResult("CMDB connection source is invalid", err)
	}
	store, err := cmdb.Open(ctx, resolved, cmdb.OpenOptions{})
	if err != nil {
		return nil, cmdbFailureResult("CMDB connection failed", err)
	}
	return store, nil
}

func cmdbFailureResult(message string, err error) *output.Result {
	code, status := cmdbFailureCode(err)
	return output.Fail(status, message).WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: code})
}

func cmdbFailureCode(err error) (string, int) {
	var adapterError *cmdb.Error
	if !errors.As(err, &adapterError) || adapterError == nil || adapterError.Code == "" {
		return "CMDB_INTERNAL", output.CodeStyCMDBOperationFailed
	}
	code := adapterError.Code
	switch code {
	case "CMDB_PARAMETER_INVALID", "CMDB_REVISION_TOKEN_INVALID", "CMDB_LOAD_PLAN_INVALID":
		return code, output.CodeStyCMDBInvalidArgs
	case "CMDB_CONNECTION_FILE_PERMISSION", "CMDB_RUNTIME_PERMISSION", "CMDB_SWITCH_PERMISSION":
		return code, output.CodeStyCMDBPermissionDenied
	case "CMDB_CONNECTION_FAILED":
		return code, output.CodeStyCMDBConnectionFailed
	case "CMDB_REVISION_NOT_FOUND", "CMDB_OUTPUT_EXISTS", "CMDB_BACKUP_REQUIRED", "CMDB_STATIC_REQUIRED":
		return code, output.CodeStyCMDBResourceMissing
	case "CMDB_REVISION_CONFLICT", "CMDB_RESET_CONFLICT", "CMDB_TARGET_CHANGED", "CMDB_SWITCH_CONFLICT":
		return code, output.CodeStyCMDBRevisionConflict
	case "CMDB_BASE_MISMATCH":
		return code, output.CodeStyCMDBBaseMismatch
	case "CMDB_MERGE_CONFLICT":
		return code, output.CodeStyCMDBMergeConflict
	case "CMDB_SOURCE_CHANGED", "CMDB_BACKUP_CHANGED":
		return code, output.CodeStyCMDBSourceChanged
	case "CMDB_SWITCH_DRIFT", "CMDB_SWITCH_STATE_CONFLICT", "CMDB_SOURCE_HEAD_MISMATCH":
		return code, output.CodeStyCMDBDrift
	case "CMDB_CONNECTION_INVALID", "CMDB_CONNECTION_FILE_INVALID", "CMDB_SOURCE_INVALID", "CMDB_SOURCE_UNSUPPORTED",
		"CMDB_MERGE_INVALID", "CMDB_BACKUP_INVALID", "CMDB_RUNTIME_INVALID", "CMDB_RUNTIME_COLLISION",
		"CMDB_RUNTIME_HOME_INVALID", "CMDB_RUNTIME_PIG_INVALID", "CMDB_RUNTIME_ADAPTER_INVALID",
		"CMDB_RUNTIME_CONNECTION_INVALID", "CMDB_ANSIBLE_CONFIG_INVALID", "CMDB_ANSIBLE_CONFIG_MISSING",
		"CMDB_SWITCH_INVALID":
		return code, output.CodeStyCMDBConfigInvalid
	case "CMDB_SNAPSHOT_UNREPRESENTABLE", "CMDB_SNAPSHOT_INVALID":
		return code, output.CodeStyCMDBSnapshotUnrepresentable
	case "CMDB_MIGRATION_DRIFT", "CMDB_MIGRATION_NEWER", "CMDB_SCHEMA_FOREIGN", "CMDB_SCHEMA_INVALID", "CMDB_HISTORY_INVALID", "CMDB_RESET_INSPECTION_INVALID", "CMDB_SERVER_INVALID", "CMDB_SERVER_UNSUPPORTED":
		return code, output.CodeStyCMDBSchemaInvalid
	case "CMDB_OUTPUT_INVALID", "CMDB_OUTPUT_FAILED", "CMDB_OUTPUT_PARTIAL", "CMDB_DUMP_PARTIAL":
		return code, output.CodeStyCMDBOutputFailed
	case "CMDB_CONFIRMATION_REQUIRED":
		return code, output.CodeStyCMDBConfirmationRequired
	case "CMDB_PRUNE_REQUIRED":
		return code, output.CodeStyCMDBPruneRequired
	default:
		return code, output.CodeStyCMDBOperationFailed
	}
}

func formatCMDBToken(token *cmdb.RevisionToken) string {
	if token == nil {
		return "none"
	}
	return token.String()
}
