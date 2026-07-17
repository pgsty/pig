package sty

import (
	"context"
	"os"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

// CMDBInventoryAdapterOptions is the private input surface for the hidden
// Ansible inventory protocol. RuntimePath is normally read only from the
// wrapper's PIGSTY_CMDB_RUNTIME environment assignment.
type CMDBInventoryAdapterOptions struct {
	Arguments   []string
	RuntimePath string
	PigBinary   string
}

type cmdbInventoryRequest struct {
	host *string
}

// CMDBInventoryAdapter returns secret-bearing protocol bytes separately from
// its sanitized Result. A nil Result means success. The body must be written
// directly to stdout and must never be logged or serialized as Result data.
func CMDBInventoryAdapter(ctx context.Context, options CMDBInventoryAdapterOptions) ([]byte, *output.Result) {
	request, failure := parseCMDBInventoryRequest(options.Arguments)
	if failure != nil {
		return nil, failure
	}
	runtimePath := options.RuntimePath
	if runtimePath == "" {
		runtimePath = os.Getenv(cmdb.RuntimeEnv)
	}
	if runtimePath == "" {
		return nil, cmdbFailureResult("CMDB runtime identity is unavailable", &cmdb.Error{
			Code: "CMDB_RUNTIME_INVALID", Message: "runtime record path is required",
		})
	}
	pigBinary := options.PigBinary
	if pigBinary == "" {
		var err error
		pigBinary, err = os.Executable()
		if err != nil {
			return nil, cmdbFailureResult("CMDB runtime identity is invalid", &cmdb.Error{
				Code: "CMDB_RUNTIME_PIG_INVALID", Message: "current Pig binary is unavailable",
			})
		}
	}
	record, err := cmdb.VerifyRuntimeFile(runtimePath, pigBinary)
	if err != nil {
		return nil, cmdbFailureResult("CMDB runtime identity is invalid", err)
	}
	resolved, err := cmdb.ResolveRuntimeConnection(record)
	if err != nil {
		return nil, cmdbFailureResult("CMDB runtime connection source is invalid", err)
	}
	store, err := cmdb.Open(ctx, resolved, cmdb.OpenOptions{})
	if err != nil {
		return nil, cmdbFailureResult("CMDB runtime connection failed", err)
	}
	defer store.Close()
	revision, err := store.ReadHead(ctx)
	if err != nil {
		return nil, cmdbFailureResult("CMDB runtime head could not be verified", err)
	}
	if revision == nil || revision.Snapshot() == nil {
		return nil, cmdbFailureResult("CMDB runtime head is empty", &cmdb.Error{
			Code: "CMDB_REVISION_NOT_FOUND", Message: "native store has no active revision",
		})
	}
	return renderCMDBInventoryRequest(revision.Snapshot(), request)
}

func parseCMDBInventoryRequest(arguments []string) (cmdbInventoryRequest, *output.Result) {
	switch {
	case len(arguments) == 1 && arguments[0] == "--list":
		return cmdbInventoryRequest{}, nil
	case len(arguments) == 2 && arguments[0] == "--host" && arguments[1] != "":
		host := arguments[1]
		return cmdbInventoryRequest{host: &host}, nil
	default:
		return cmdbInventoryRequest{}, cmdbFailureResult("CMDB inventory adapter arguments are invalid", &cmdb.Error{
			Code: "CMDB_PARAMETER_INVALID", Message: "adapter accepts only --list or --host with one non-empty name",
		})
	}
}

func renderCMDBInventoryRequest(snapshot *inventory.Snapshot, request cmdbInventoryRequest) ([]byte, *output.Result) {
	if snapshot == nil {
		return nil, cmdbFailureResult("CMDB inventory projection failed", &cmdb.Error{
			Code: "CMDB_SNAPSHOT_INVALID", Message: "active Snapshot is unavailable",
		})
	}
	var (
		body []byte
		err  error
	)
	if request.host == nil {
		body, err = snapshot.AnsibleInventoryJSON()
	} else {
		body, err = snapshot.AnsibleHostJSON(*request.host)
	}
	if err != nil {
		return nil, cmdbFailureResult("CMDB inventory projection failed", &cmdb.Error{
			Code: "CMDB_SNAPSHOT_INVALID", Message: "active Snapshot cannot be projected",
		})
	}
	return body, nil
}
