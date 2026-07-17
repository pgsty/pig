package sty

import (
	"context"
	"fmt"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

type CMDBDumpOptions struct {
	OutputPath       string
	Revision         string
	Merge            bool
	ExpectBase       string
	InvocationSource string
}

// CMDBDumpData promotes only the native safe metadata and one optional
// redacted finding code. The Inventory body is written directly to the new
// private destination and can never enter this DTO.
type CMDBDumpData struct {
	Kind          string                       `json:"kind" yaml:"kind"`
	Revision      *cmdb.RevisionToken          `json:"revision,omitempty" yaml:"revision,omitempty"`
	Base          *cmdb.RevisionToken          `json:"base,omitempty" yaml:"base,omitempty"`
	RevisionCount int64                        `json:"revision_count" yaml:"revision_count"`
	NodeCount     int                          `json:"node_count" yaml:"node_count"`
	Merge         bool                         `json:"merge" yaml:"merge"`
	OutputPath    string                       `json:"output_path,omitempty" yaml:"output_path,omitempty"`
	OutputSHA256  string                       `json:"output_sha256,omitempty" yaml:"output_sha256,omitempty"`
	SourceSHA256  string                       `json:"source_sha256,omitempty" yaml:"source_sha256,omitempty"`
	Changes       []inventory.SnapshotChange   `json:"changes,omitempty" yaml:"changes,omitempty"`
	Conflicts     []inventory.SnapshotConflict `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
	FindingCode   string                       `json:"finding_code,omitempty" yaml:"finding_code,omitempty"`
}

func (d *CMDBDumpData) Text() string {
	if d == nil {
		return ""
	}
	lines := []string{
		"  revision: " + formatCMDBToken(d.Revision),
		"  revisions: " + fmt.Sprint(d.RevisionCount),
		"  nodes: " + fmt.Sprint(d.NodeCount),
	}
	if d.Base != nil {
		lines = append(lines, "  merge base: "+d.Base.String())
	}
	if d.OutputPath != "" {
		lines = append(lines, "  output: "+d.OutputPath, "  output sha256: "+d.OutputSHA256)
	}
	if d.FindingCode != "" {
		lines = append(lines, "  finding: "+d.FindingCode)
	}
	if len(d.Conflicts) > 0 {
		rows := make([][]string, 0, len(d.Conflicts))
		for _, conflict := range d.Conflicts {
			rows = append(rows, []string{
				string(conflict.Working.Operation), conflict.Working.Path.String(),
				string(conflict.Current.Operation), conflict.Current.Path.String(),
			})
		}
		lines = append(lines, strings.TrimSuffix(output.RenderTable(
			[]string{"WORKING OP", "WORKING PATH", "CMDB OP", "CMDB PATH"}, rows,
		), "\n"))
	}
	return strings.Join(lines, "\n")
}

func CMDBDump(ctx context.Context, store *cmdb.Store, options CMDBDumpOptions) *output.Result {
	outputPath := options.OutputPath
	if strings.TrimSpace(outputPath) == "" {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB dump requires a new output path").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	revisionText := options.Revision
	expectBaseText := options.ExpectBase
	if options.Merge && revisionText != "" {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB merge dump cannot select a historical revision").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	if options.Merge && options.InvocationSource != "" {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB merge dump is mutually exclusive with an invocation Inventory source").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	if !options.Merge && expectBaseText != "" {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB expected base is valid only for merge dump").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	var revision, expectBase *cmdb.RevisionToken
	if revisionText != "" {
		parsed, err := cmdb.ParseRevisionToken(revisionText)
		if err != nil {
			return cmdbFailureResult("CMDB dump revision token is invalid", err)
		}
		revision = &parsed
	}
	if expectBaseText != "" {
		parsed, err := cmdb.ParseRevisionToken(expectBaseText)
		if err != nil {
			return cmdbFailureResult("CMDB merge base token is invalid", err)
		}
		expectBase = &parsed
	}
	native, err := store.Dump(ctx, cmdb.DumpRequest{
		OutputPath: outputPath, Revision: revision, Merge: options.Merge, ExpectBase: expectBase,
	})
	data := newCMDBDumpData(native)
	if err != nil {
		finding, _ := cmdbFailureCode(err)
		data.FindingCode = finding
		result := cmdbFailureResult("CMDB dump failed without exposing Inventory content", err)
		result.Data = data
		return result
	}
	return output.OK("CMDB dump committed", data)
}

func newCMDBDumpData(native cmdb.DumpResult) *CMDBDumpData {
	data := &CMDBDumpData{
		Kind: native.Kind, Revision: nil, Base: cloneRevisionToken(native.Base),
		RevisionCount: native.RevisionCount, NodeCount: native.NodeCount, Merge: native.Merge,
		OutputPath: native.OutputPath, OutputSHA256: native.OutputSHA256, SourceSHA256: native.SourceSHA256,
		Changes: cloneSnapshotChanges(native.Changes), Conflicts: cloneSnapshotConflicts(native.Conflicts),
	}
	if data.Kind == "" {
		data.Kind = cmdb.DumpKind
	}
	if native.Revision.String() != "" {
		token := native.Revision
		data.Revision = &token
	}
	return data
}

func cloneSnapshotConflicts(conflicts []inventory.SnapshotConflict) []inventory.SnapshotConflict {
	if len(conflicts) == 0 {
		return nil
	}
	copy := make([]inventory.SnapshotConflict, len(conflicts))
	for index := range conflicts {
		copy[index] = inventory.SnapshotConflict{
			Working: cloneSnapshotChanges([]inventory.SnapshotChange{conflicts[index].Working})[0],
			Current: cloneSnapshotChanges([]inventory.SnapshotChange{conflicts[index].Current})[0],
		}
	}
	return copy
}
