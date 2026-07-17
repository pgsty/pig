package sty

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
)

type fakeCMDBResetter struct {
	inspection cmdb.ResetInspection
	result     cmdb.ResetResult
	inspectErr error
	resetErr   error
	request    cmdb.ResetRequest
	inspects   int
	resets     int
	source     cmdb.ConnectionSource
}

func (f *fakeCMDBResetter) InspectReset(context.Context) (cmdb.ResetInspection, error) {
	f.inspects++
	return f.inspection, f.inspectErr
}

func (f *fakeCMDBResetter) Reset(_ context.Context, request cmdb.ResetRequest) (cmdb.ResetResult, error) {
	f.resets++
	f.request = request
	return f.result, f.resetErr
}

func (f *fakeCMDBResetter) Source() cmdb.ConnectionSource { return f.source }

func TestCMDBResetPlanRequiresConfirmationWithoutDDL(t *testing.T) {
	home, runtimeRoot := t.TempDir(), filepath.Join(t.TempDir(), "cmdb")
	head, err := cmdb.ParseRevisionToken("cmdb-v1:2:sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeCMDBResetter{
		inspection: cmdb.ResetInspection{
			MigrationVersion: 1, MigrationCount: 1, RevisionCount: 2, Head: &head,
		},
		source: cmdb.ConnectionEnvSource,
	}
	prepared := 0
	operation, failure := planCMDBReset(
		t.Context(), store,
		CMDBInitOptions{PigstyHome: home, RuntimeRoot: runtimeRoot, Reset: true, ToolVersion: "test-v1"},
		cmdb.InspectBundle,
		func(options cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error) {
			prepared++
			runtimePath, pathErr := cmdb.RuntimeRecordPath(options.PigstyHome, options.RuntimeRoot)
			if pathErr != nil {
				return cmdb.RuntimeRecord{}, pathErr
			}
			return cmdb.RuntimeRecord{
				Kind: cmdb.RuntimeRecordKind, AdapterPath: filepath.Join(filepath.Dir(runtimePath), "inventory"),
				AdapterSHA256: strings.Repeat("b", 64),
			}, nil
		},
		func() (string, error) { return strings.Repeat("c", 32), nil },
	)
	if failure != nil || operation == nil || store.inspects != 1 || store.resets != 0 || prepared != 0 {
		t.Fatalf("plan operation=%+v failure=%+v inspect=%d reset=%d prepared=%d", operation, failure, store.inspects, store.resets, prepared)
	}
	data := operation.Data()
	if !data.Reset || data.Absent || data.Head == nil || !data.Head.Equal(head) ||
		!strings.HasSuffix(data.BackupPath, "backups/reset-"+strings.Repeat("c", 32)+".ndjson") || !data.RequiresConfirm {
		t.Fatalf("reset plan data = %+v", data)
	}

	confirmation := ApplyCMDBReset(t.Context(), operation, false)
	if confirmation.Success || confirmation.Code != output.CodeStyCMDBConfirmationRequired || store.resets != 0 || prepared != 0 {
		t.Fatalf("unconfirmed reset = %+v reset=%d prepared=%d", confirmation, store.resets, prepared)
	}
}

func TestCMDBResetApplyUsesSealedPlanAndPreparesRuntimeBeforeReset(t *testing.T) {
	home, runtimeRoot := t.TempDir(), filepath.Join(t.TempDir(), "cmdb")
	head, err := cmdb.ParseRevisionToken("cmdb-v1:1:sha256:" + strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeCMDBResetter{
		inspection: cmdb.ResetInspection{MigrationVersion: 1, MigrationCount: 1, RevisionCount: 1, Head: &head},
		result: cmdb.ResetResult{
			State:      cmdb.VerifiedState{MigrationVersion: 1},
			BackupPath: filepath.Join(runtimeRoot, "future.ndjson"), BackupSHA256: strings.Repeat("e", 64),
		},
		source: cmdb.ConnectionFileSource,
	}
	prepared := 0
	operation, failure := planCMDBReset(
		t.Context(), store,
		CMDBInitOptions{
			PigstyHome: home, RuntimeRoot: runtimeRoot, Reset: true,
			ConnectionFile: "/private/connection", ToolVersion: "test-v1",
		},
		cmdb.InspectBundle,
		func(options cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error) {
			prepared++
			if options.ConnectionSource != cmdb.ConnectionFileSource || options.ConnectionFile != "/private/connection" {
				t.Fatalf("runtime options = %+v", options)
			}
			runtimePath, pathErr := cmdb.RuntimeRecordPath(options.PigstyHome, options.RuntimeRoot)
			if pathErr != nil {
				return cmdb.RuntimeRecord{}, pathErr
			}
			return cmdb.RuntimeRecord{
				Kind: cmdb.RuntimeRecordKind, AdapterPath: filepath.Join(filepath.Dir(runtimePath), "inventory"),
				AdapterSHA256: strings.Repeat("f", 64),
			}, nil
		},
		func() (string, error) { return strings.Repeat("1", 32), nil },
	)
	if failure != nil {
		t.Fatal(failure.Message)
	}
	result := ApplyCMDBReset(t.Context(), operation, true)
	if !result.Success || store.resets != 1 || prepared != 1 || store.request.BackupPath != operation.Data().BackupPath || store.request.ToolVersion != "test-v1" {
		t.Fatalf("apply result=%+v request=%+v resets=%d prepared=%d", result, store.request, store.resets, prepared)
	}
	data, ok := result.Data.(*CMDBInitData)
	if !ok || !data.ResetApplied || data.AdapterIdentity == "" || data.BackupSHA256 != strings.Repeat("e", 64) {
		t.Fatalf("apply data = %+v", result.Data)
	}
}

func TestCMDBResetValidationAndFailuresStayRedacted(t *testing.T) {
	secret := "reset-dsn-password"
	if failure := ValidateCMDBInitOptions(CMDBInitOptions{RestorePath: "/private/restore"}); failure == nil || failure.Code != output.CodeStyCMDBInvalidArgs {
		t.Fatalf("restore without reset = %+v", failure)
	}
	store := &fakeCMDBResetter{
		inspectErr: &cmdb.Error{Code: "CMDB_SCHEMA_FOREIGN", Message: "foreign schema"},
		source:     cmdb.ConnectionEnvSource,
	}
	operation, failure := planCMDBReset(
		t.Context(), store, CMDBInitOptions{PigstyHome: t.TempDir(), Reset: true},
		cmdb.InspectBundle, cmdb.PrepareRuntime,
		func() (string, error) { return "", errors.New(secret) },
	)
	if operation != nil || failure == nil || failure.Code != output.CodeStyCMDBSchemaInvalid {
		t.Fatalf("foreign reset plan operation=%+v failure=%+v", operation, failure)
	}
	body, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) {
		t.Fatalf("reset failure leaked secret: %s", body)
	}
}
