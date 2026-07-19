package inventorycli

import (
	"pig/internal/output"
	"pig/inventory"
	"strings"
)

const InventoryDiffKind = "pig.inventory.diff/v1"

type DiffOptions struct {
	Path      string
	OtherPath string
}

type InventoryDiffItem struct {
	Operation string              `json:"operation" yaml:"operation"`
	Path      string              `json:"path" yaml:"path"`
	Object    string              `json:"object" yaml:"object"`
	ValueKind inventory.ValueKind `json:"value_kind" yaml:"value_kind"`
}

type InventoryDiffData struct {
	Kind      string              `json:"kind" yaml:"kind"`
	Path      string              `json:"path" yaml:"path"`
	OtherPath string              `json:"other_path" yaml:"other_path"`
	Changes   []InventoryDiffItem `json:"changes" yaml:"changes"`
}

func (d *InventoryDiffData) Text() string {
	if d == nil || len(d.Changes) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(d.Changes))
	for _, change := range d.Changes {
		rows = append(rows, []string{change.Operation, change.Path, change.Object, string(change.ValueKind)})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"OPERATION", "PATH", "OBJECT", "VALUE KIND"}, rows), "\n")
}

func DiffInventory(opts DiffOptions) *output.Result {
	left, failure := loadStaticInventory(opts.Path, output.CodeInventoryListInvalidArgs)
	if failure != nil {
		return failure
	}
	right, failure := loadStaticInventory(opts.OtherPath, output.CodeInventoryListInvalidArgs)
	if failure != nil {
		return failure
	}
	differences, err := inventory.DiffDocuments(left, right)
	if err != nil {
		return output.Fail(output.CodeInventoryInvalid, "inventory declarations could not be compared")
	}
	changes := make([]InventoryDiffItem, 0, len(differences))
	for _, difference := range differences {
		changes = append(changes, InventoryDiffItem{
			Operation: difference.Operation, Path: difference.Path.String(),
			Object: inventoryObjectKind(difference.Path), ValueKind: difference.ValueKind,
		})
	}
	data := &InventoryDiffData{
		Kind: InventoryDiffKind, Path: strings.TrimSpace(opts.Path), OtherPath: strings.TrimSpace(opts.OtherPath), Changes: changes,
	}
	return output.OK("inventory declarations compared", data)
}
