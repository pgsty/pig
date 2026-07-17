package sty

import (
	"errors"
	"fmt"
	"os"
	"pig/internal/config"
	"pig/internal/output"
	"pig/inventory"
	"strconv"
	"strings"
)

const InventoryListKind = "pig.sty.inventory.list/v1"

// InventoryOptions identifies one static Inventory and an optional selector.
type InventoryOptions struct {
	Path     string
	Selector string
}

// InventoryListItem is safe topology/metadata. It deliberately carries no
// scalar values, raw lexemes, tags, comments, or source fragments.
type InventoryListItem struct {
	Path      string              `json:"path" yaml:"path"`
	Name      string              `json:"name,omitempty" yaml:"name,omitempty"`
	Kind      string              `json:"kind" yaml:"kind"`
	ValueKind inventory.ValueKind `json:"value_kind" yaml:"value_kind"`
}

// InventoryListData is the versioned structured payload for inventory list.
type InventoryListData struct {
	Kind     string              `json:"kind" yaml:"kind"`
	Path     string              `json:"path" yaml:"path"`
	Selector string              `json:"selector,omitempty" yaml:"selector,omitempty"`
	Root     string              `json:"root" yaml:"root"`
	Items    []InventoryListItem `json:"items" yaml:"items"`
}

func (d *InventoryListData) Text() string {
	if d == nil {
		return ""
	}
	rows := make([][]string, 0, len(d.Items))
	for _, item := range d.Items {
		rows = append(rows, []string{item.Path, item.Kind, string(item.ValueKind)})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"PATH", "OBJECT", "VALUE KIND"}, rows), "\n")
}

// ListInventory emits only ordered paths, topology roles, names, and value
// kinds. It never copies arbitrary Inventory values into the Result.
func ListInventory(opts InventoryOptions) *output.Result {
	doc, failure := loadStaticInventory(opts.Path, output.CodeStyInventoryListInvalidArgs)
	if failure != nil {
		return failure
	}
	selected, err := inventory.ParseSelector(doc, opts.Selector)
	if err != nil {
		return output.Fail(output.CodeStyInventoryListInvalidArgs, err.Error())
	}
	value, ok := doc.Lookup(selected)
	if !ok {
		return output.Fail(output.CodeStyInventoryListInvalidArgs, "selected inventory path does not exist")
	}
	items := make([]InventoryListItem, 0)
	appendInventoryListItems(&items, selected, value)
	data := &InventoryListData{
		Kind: InventoryListKind, Path: strings.TrimSpace(opts.Path),
		Selector: strings.TrimSpace(opts.Selector), Root: selected.String(), Items: items,
	}
	return output.OK("inventory listed", data)
}

// ShowInventory returns a verbatim or dedented source fragment separately from
// Result so secret-bearing bytes cannot be serialized or copied into logs.
// A nil Result means success.
func ShowInventory(opts InventoryOptions) ([]byte, *output.Result) {
	if config.IsStructuredOutput() {
		return nil, output.Fail(output.CodeStyInventoryShowInvalidArgs, "inventory show supports text output only")
	}
	doc, failure := loadStaticInventory(opts.Path, output.CodeStyInventoryShowInvalidArgs)
	if failure != nil {
		return nil, failure
	}
	selected, err := inventory.ParseSelector(doc, opts.Selector)
	if err != nil {
		return nil, output.Fail(output.CodeStyInventoryShowInvalidArgs, err.Error())
	}
	fragment, err := doc.Extract(selected)
	if err != nil {
		return nil, output.Fail(output.CodeStyInventoryShowInvalidArgs, "selected inventory fragment cannot be shown losslessly").WithDetail(err.Error())
	}
	return fragment.Bytes, nil
}

func loadStaticInventory(path string, invalidCode int) (*inventory.Document, *output.Result) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, output.Fail(output.CodeStyInventoryNotFound, "inventory path is not configured")
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, output.Fail(output.CodeStyInventoryNotFound, fmt.Sprintf("inventory not found: %s", path))
	}
	if err != nil {
		return nil, output.Fail(output.CodeStyInventoryReadFailed, fmt.Sprintf("failed to inspect inventory: %v", err))
	}
	if failure := staticInventorySourceFailure(info, invalidCode); failure != nil {
		return nil, failure
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, output.Fail(output.CodeStyInventoryReadFailed, fmt.Sprintf("failed to read inventory: %v", err))
	}
	doc, err := inventory.Parse(path, source)
	if err != nil {
		return nil, output.Fail(output.CodeStyInventoryInvalid, "inventory YAML could not be parsed").WithDetail(parseErrorSummary(err))
	}
	return doc, nil
}

func staticInventorySourceFailure(info os.FileInfo, invalidCode int) *output.Result {
	if info == nil || !info.Mode().IsRegular() {
		return output.Fail(invalidCode, "inventory source must be one static YAML file; directory and dynamic sources are unsupported")
	}
	if info.Mode().Perm()&0111 != 0 {
		return output.Fail(invalidCode, "executable dynamic Inventory sources are unsupported")
	}
	return nil
}

func appendInventoryListItems(items *[]InventoryListItem, path inventory.Path, value *inventory.Value) {
	if value == nil {
		return
	}
	*items = append(*items, InventoryListItem{
		Path: path.String(), Name: inventoryPathName(path),
		Kind: inventoryObjectKind(path), ValueKind: value.Kind,
	})
	for index := range value.Mapping {
		entry := &value.Mapping[index]
		appendInventoryListItems(items, entry.Path, &entry.Value)
	}
	for index := range value.Sequence {
		item := &value.Sequence[index]
		appendInventoryListItems(items, item.Path, item)
	}
}

func inventoryPathName(path inventory.Path) string {
	last, ok := path.Last()
	if !ok {
		return ""
	}
	if last.Kind == inventory.IndexSegment {
		return strconv.Itoa(last.Index)
	}
	return last.Key
}

func inventoryObjectKind(path inventory.Path) string {
	if len(path) == 0 {
		return "document"
	}
	if inventoryGroupPath(path) {
		return "group"
	}
	parent, ok := path.Parent()
	if !ok {
		return "value"
	}
	last, _ := path.Last()
	if inventoryGroupPath(parent) && last.Kind == inventory.KeySegment {
		switch last.Key {
		case "children":
			return "groups"
		case "hosts":
			return "hosts"
		case "vars":
			return "vars"
		}
	}
	container, ok := parent.Last()
	if ok && container.Kind == inventory.KeySegment && container.Key == "hosts" {
		group, hasGroup := parent.Parent()
		if hasGroup && inventoryGroupPath(group) {
			return "host"
		}
	}
	return "value"
}

func inventoryGroupPath(path inventory.Path) bool {
	if len(path) == 1 && path[0].Kind == inventory.KeySegment && path[0].Key == "all" {
		return true
	}
	if len(path) < 3 || path[0].Kind != inventory.KeySegment || path[0].Key != "all" || (len(path)-1)%2 != 0 {
		return false
	}
	for index := 1; index < len(path); index += 2 {
		if path[index].Kind != inventory.KeySegment || path[index].Key != "children" || path[index+1].Kind != inventory.KeySegment {
			return false
		}
	}
	return true
}
