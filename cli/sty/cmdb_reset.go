package sty

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
)

type cmdbResetter interface {
	InspectReset(context.Context) (cmdb.ResetInspection, error)
	Reset(context.Context, cmdb.ResetRequest) (cmdb.ResetResult, error)
	Source() cmdb.ConnectionSource
}

// CMDBResetOperation seals the exact native state fingerprint, verified
// restore bundle identity, future private backup path, and runtime identity
// across one interactive confirmation. Only its value-free Data is exposed.
type CMDBResetOperation struct {
	data        CMDBInitData
	store       cmdbResetter
	inspection  cmdb.ResetInspection
	restore     *cmdb.BundleInspection
	options     CMDBInitOptions
	runtimePath string
	prepare     func(cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error)
}

func (o *CMDBResetOperation) Data() *CMDBInitData {
	if o == nil {
		return nil
	}
	copy := o.data
	copy.Head = cloneRevisionToken(o.data.Head)
	copy.RestoreHead = cloneRevisionToken(o.data.RestoreHead)
	return &copy
}

// ValidateCMDBInitOptions rejects reset/restore consent ambiguity without a
// database connection. --yes is meaningful only for the destructive scope.
func ValidateCMDBInitOptions(options CMDBInitOptions) *output.Result {
	if strings.TrimSpace(options.RestorePath) != "" && !options.Reset {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB restore requires reset scope").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	if options.Yes && !options.Reset {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB init --yes requires reset scope").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	return nil
}

func PlanCMDBReset(ctx context.Context, store *cmdb.Store, options CMDBInitOptions) (*CMDBResetOperation, *output.Result) {
	return planCMDBReset(ctx, store, options, cmdb.InspectBundle, cmdb.PrepareRuntime, newCMDBResetID)
}

func planCMDBReset(
	ctx context.Context,
	store cmdbResetter,
	options CMDBInitOptions,
	inspectBundle func(string) (cmdb.BundleInspection, error),
	prepare func(cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error),
	newID func() (string, error),
) (*CMDBResetOperation, *output.Result) {
	if failure := ValidateCMDBInitOptions(options); failure != nil {
		return nil, failure
	}
	if !options.Reset {
		return nil, output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB reset plan requires reset scope").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	if store == nil {
		return nil, cmdbFailureResult("CMDB reset requires an open native store", &cmdb.Error{
			Code: "CMDB_CONNECTION_INVALID", Message: "database store is not open",
		})
	}
	inspection, err := store.InspectReset(ctx)
	if err != nil {
		return nil, cmdbFailureResult("CMDB reset target is not a verified native schema or exact absence", err)
	}

	toolVersion := strings.TrimSpace(options.ToolVersion)
	if toolVersion == "" {
		toolVersion = config.PigVersion
	}
	options.ToolVersion = toolVersion
	runtimePath, err := cmdb.RuntimeRecordPath(options.PigstyHome, options.RuntimeRoot)
	if err != nil {
		return nil, cmdbFailureResult("CMDB reset runtime identity is invalid", err)
	}
	data := CMDBInitData{
		Kind: "pig.sty.cmdb.init/v1", ConnectionSource: store.Source(),
		MigrationVersion: inspection.MigrationVersion, MigrationCount: inspection.MigrationCount,
		RevisionCount: inspection.RevisionCount, Head: cloneRevisionToken(inspection.Head),
		RuntimePath: runtimePath, Reset: true, Absent: inspection.Absent,
		RequiresConfirm: true,
	}

	var restore *cmdb.BundleInspection
	if strings.TrimSpace(options.RestorePath) != "" {
		bundle, inspectErr := inspectBundle(options.RestorePath)
		if inspectErr != nil {
			return nil, cmdbFailureResult("CMDB restore bundle could not be verified", inspectErr)
		}
		state := bundle.State()
		data.RestorePath = bundle.Path()
		data.RestoreSHA256 = bundle.SHA256()
		data.RestoreRevisions = state.RevisionCount
		data.RestoreHead = cloneRevisionToken(state.Head)
		restore = &bundle
	}
	if !inspection.Absent {
		identifier, idErr := newID()
		if idErr != nil || len(identifier) != 32 {
			return nil, cmdbFailureResult("CMDB reset backup identity could not be created", &cmdb.Error{
				Code: "CMDB_OPERATION_FAILED", Message: "reset backup identifier generation failed",
			})
		}
		for _, char := range identifier {
			if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
				return nil, cmdbFailureResult("CMDB reset backup identity could not be created", &cmdb.Error{
					Code: "CMDB_OPERATION_FAILED", Message: "reset backup identifier is invalid",
				})
			}
		}
		data.BackupPath = filepath.Join(filepath.Dir(runtimePath), "backups", "reset-"+identifier+".ndjson")
	}
	return &CMDBResetOperation{
		data: data, store: store, inspection: inspection, restore: restore,
		options: options, runtimePath: runtimePath, prepare: prepare,
	}, nil
}

func ApplyCMDBReset(ctx context.Context, operation *CMDBResetOperation, yes bool) *output.Result {
	if operation == nil || operation.store == nil || operation.prepare == nil {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB reset requires an exact prepared plan").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_RESET_INSPECTION_INVALID"})
	}
	data := operation.Data()
	if !yes {
		return output.Fail(output.CodeStyCMDBConfirmationRequired, "CMDB reset requires explicit confirmation").
			WithData(data).
			WithNextActions(output.NextAction{
				Command: "pig sty cmdb init --reset --yes", Reason: "apply the exact displayed reset plan", Required: true,
			})
	}

	record, err := operation.prepare(cmdb.RuntimeOptions{
		PigstyHome: operation.options.PigstyHome, RuntimeRoot: operation.options.RuntimeRoot,
		PigBinary: operation.options.PigBinary, ConnectionSource: operation.store.Source(),
		ConnectionFile: operation.options.ConnectionFile,
	})
	if err != nil {
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB reset runtime adapter preparation failed before DDL", err)
		result.Data = data
		return result
	}
	preparedRuntimePath := filepath.Join(filepath.Dir(record.AdapterPath), "runtime.json")
	if preparedRuntimePath != operation.runtimePath {
		err = &cmdb.Error{Code: "CMDB_RUNTIME_COLLISION", Message: "prepared runtime identity differs from reset plan"}
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB reset runtime identity changed after planning", err)
		result.Data = data
		return result
	}
	data.AdapterIdentity = cmdb.RuntimeAdapterIdentity(record)
	native, err := operation.store.Reset(ctx, cmdb.ResetRequest{
		Expected: operation.inspection.Fingerprint, BackupPath: data.BackupPath,
		Restore: operation.restore, ToolVersion: operation.options.ToolVersion,
	})
	if err != nil {
		data.FindingCode, _ = cmdbFailureCode(err)
		result := cmdbFailureResult("CMDB reset did not commit", err)
		result.Data = data
		return result
	}
	data.MigrationVersion = native.State.MigrationVersion
	data.RevisionCount = native.State.RevisionCount
	data.Head = cloneRevisionToken(native.State.Head)
	data.BackupPath = native.BackupPath
	data.BackupSHA256 = native.BackupSHA256
	data.ResetApplied = true
	data.Restored = native.Restored
	return output.OK("CMDB reset committed and runtime adapter verified", data)
}

func CMDBReset(ctx context.Context, store *cmdb.Store, options CMDBInitOptions) *output.Result {
	operation, failure := PlanCMDBReset(ctx, store, options)
	if failure != nil {
		return failure
	}
	return ApplyCMDBReset(ctx, operation, options.Yes)
}

func newCMDBResetID() (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
