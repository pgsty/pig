package inventorycli

import "pig/inventory"

// InventoryDiagnostic is the public, value-free diagnostic DTO. The path is a
// canonical JSON-pointer-like string; internal numeric segment kinds never
// become the only wire representation.
type InventoryDiagnostic struct {
	Code     string                `json:"code" yaml:"code"`
	Severity inventory.Severity    `json:"severity" yaml:"severity"`
	Path     string                `json:"path" yaml:"path"`
	Range    inventory.SourceRange `json:"range" yaml:"range"`
	Message  string                `json:"message" yaml:"message"`
}

// InventoryChange is the public, value-free change DTO.
type InventoryChange struct {
	Operation  string `json:"operation" yaml:"operation"`
	Path       string `json:"path" yaml:"path"`
	Identifier string `json:"identifier,omitempty" yaml:"identifier,omitempty"`
	Count      int    `json:"count,omitempty" yaml:"count,omitempty"`
}

type InventoryChangeSet struct {
	Changes []InventoryChange `json:"changes,omitempty" yaml:"changes,omitempty"`
}

func inventoryDiagnostics(diagnostics []inventory.Diagnostic) []InventoryDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	result := make([]InventoryDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, InventoryDiagnostic{
			Code: diagnostic.Code, Severity: diagnostic.Severity,
			Path: diagnostic.Path.String(), Range: diagnostic.Range, Message: diagnostic.Message,
		})
	}
	return result
}

func inventoryChanges(changes inventory.ChangeSet) InventoryChangeSet {
	result := InventoryChangeSet{Changes: make([]InventoryChange, 0, len(changes.Changes))}
	for _, change := range changes.Changes {
		result.Changes = append(result.Changes, InventoryChange{
			Operation: change.Operation, Path: change.Path.String(), Identifier: change.Identifier, Count: change.Count,
		})
	}
	return result
}
