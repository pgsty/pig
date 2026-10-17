package inventorycli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
	"pig/inventory"

	"gopkg.in/yaml.v3"
)

const (
	InventoryCMDBCheckKind   = "pig.inventory.cmdb.check/v1"
	InventoryCMDBInitKind    = "pig.inventory.cmdb.init/v1"
	InventoryCMDBLoadKind    = "pig.inventory.cmdb.load/v1"
	InventoryCMDBDumpKind    = "pig.inventory.cmdb.dump/v1"
	InventoryCMDBEnableKind  = "pig.inventory.cmdb.enable/v1"
	InventoryCMDBDisableKind = "pig.inventory.cmdb.disable/v1"
	CMDBFailureKind          = "pig.inventory.cmdb.failure/v1"
	CMDBValidationKind       = "pig.inventory.cmdb.validation/v1"
)

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

type CMDBValidationData struct {
	Kind        string                `json:"kind" yaml:"kind"`
	Path        string                `json:"path" yaml:"path"`
	Diagnostics []InventoryDiagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

func (d *CMDBValidationData) Text() string {
	if d == nil || len(d.Diagnostics) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(d.Diagnostics))
	for _, diagnostic := range d.Diagnostics {
		rows = append(rows, []string{diagnostic.Code, diagnostic.Path, string(diagnostic.Severity)})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"CODE", "PATH", "STATUS"}, rows), "\n")
}

// InventoryCMDBData is the shared value-free result metadata for the six
// thin operations. It never contains SQL, a DSN, or Inventory values.
type InventoryCMDBData struct {
	Kind                 string                `json:"kind" yaml:"kind"`
	Operation            string                `json:"operation" yaml:"operation"`
	CMDBCode             string                `json:"cmdb_code,omitempty" yaml:"cmdb_code,omitempty"`
	ConnectionSource     cmdb.ConnectionSource `json:"connection_source,omitempty" yaml:"connection_source,omitempty"`
	DatabaseTargetDigest string                `json:"database_target_digest,omitempty" yaml:"database_target_digest,omitempty"`
	SourcePath           string                `json:"source_path,omitempty" yaml:"source_path,omitempty"`
	OutputPath           string                `json:"output_path,omitempty" yaml:"output_path,omitempty"`
	ConfigPath           string                `json:"config_path,omitempty" yaml:"config_path,omitempty"`
	InventoryPath        string                `json:"inventory_path,omitempty" yaml:"inventory_path,omitempty"`
	Objects              int                   `json:"objects,omitempty" yaml:"objects,omitempty"`
	ExistingSchemas      int                   `json:"existing_schemas,omitempty" yaml:"existing_schemas,omitempty"`
	Statements           int                   `json:"statements,omitempty" yaml:"statements,omitempty"`
	Globals              int                   `json:"globals,omitempty" yaml:"globals,omitempty"`
	Groups               int                   `json:"groups,omitempty" yaml:"groups,omitempty"`
	Hosts                int                   `json:"hosts,omitempty" yaml:"hosts,omitempty"`
	GroupVars            int                   `json:"group_vars,omitempty" yaml:"group_vars,omitempty"`
	HostVars             int                   `json:"host_vars,omitempty" yaml:"host_vars,omitempty"`
	DefaultVars          int                   `json:"default_vars,omitempty" yaml:"default_vars,omitempty"`
	Packages             int                   `json:"packages,omitempty" yaml:"packages,omitempty"`
	Changed              bool                  `json:"changed,omitempty" yaml:"changed,omitempty"`
	Noop                 bool                  `json:"noop,omitempty" yaml:"noop,omitempty"`
	Planned              bool                  `json:"planned,omitempty" yaml:"planned,omitempty"`
	Strict               bool                  `json:"strict,omitempty" yaml:"strict,omitempty"`
	Compared             bool                  `json:"compared,omitempty" yaml:"compared,omitempty"`
	Equivalent           *bool                 `json:"equivalent,omitempty" yaml:"equivalent,omitempty"`
	ScriptChanged        bool                  `json:"script_changed,omitempty" yaml:"script_changed,omitempty"`
	ConfigChanged        bool                  `json:"config_changed,omitempty" yaml:"config_changed,omitempty"`
	SourceDigest         string                `json:"source_digest,omitempty" yaml:"source_digest,omitempty"`
	SourceRevision       string                `json:"source_revision,omitempty" yaml:"source_revision,omitempty"`
	TargetDigest         string                `json:"target_digest,omitempty" yaml:"target_digest,omitempty"`
	RequiresConfirmation bool                  `json:"requires_confirmation,omitempty" yaml:"requires_confirmation,omitempty"`
	Warnings             []string              `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Findings             []CMDBFinding         `json:"findings,omitempty" yaml:"findings,omitempty"`
	Diagnostics          []InventoryDiagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
	Changes              []InventoryDiffItem   `json:"changes,omitempty" yaml:"changes,omitempty"`
	Plan                 *output.Plan          `json:"plan,omitempty" yaml:"plan,omitempty"`
}

type CMDBFinding struct {
	Code   string `json:"code" yaml:"code"`
	Status string `json:"status" yaml:"status"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
}

func (d *InventoryCMDBData) Text() string {
	if d == nil {
		return ""
	}
	rows := [][]string{{"operation", d.Operation}}
	if d.CMDBCode != "" {
		rows = append(rows, []string{"finding", d.CMDBCode})
	}
	if d.ConnectionSource != "" {
		rows = append(rows, []string{"connection", string(d.ConnectionSource)})
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{"source", d.SourcePath}, {"output", d.OutputPath}, {"config", d.ConfigPath}, {"inventory", d.InventoryPath},
	} {
		if item.value != "" {
			rows = append(rows, []string{item.name, item.value})
		}
	}
	for _, item := range []struct {
		name  string
		value int
	}{
		{"objects", d.Objects}, {"existing schemas", d.ExistingSchemas}, {"statements", d.Statements}, {"globals", d.Globals}, {"groups", d.Groups},
		{"hosts", d.Hosts}, {"group vars", d.GroupVars}, {"host vars", d.HostVars},
		{"default vars", d.DefaultVars}, {"packages", d.Packages},
	} {
		if item.value != 0 {
			rows = append(rows, []string{item.name, fmt.Sprint(item.value)})
		}
	}
	for _, warning := range d.Warnings {
		rows = append(rows, []string{"warning", warning})
	}
	for _, finding := range d.Findings {
		rows = append(rows, []string{finding.Status, finding.Code})
	}
	if d.SourceDigest != "" {
		rows = append(rows, []string{"source digest", d.SourceDigest})
	}
	if d.TargetDigest != "" {
		rows = append(rows, []string{"target digest", d.TargetDigest})
	}
	if d.DatabaseTargetDigest != "" {
		rows = append(rows, []string{"database target", d.DatabaseTargetDigest})
	}
	text := strings.TrimSuffix(output.RenderTable([]string{"FIELD", "VALUE"}, rows), "\n")
	if d.Planned && d.Plan != nil {
		text += "\n\n" + d.Plan.Text()
	}
	return text
}

type InventoryCMDBOptions struct {
	Home         string
	Database     string
	Path         string
	ExplicitPath bool
	Yes          bool
	Plan         bool
	Force        bool
	Strict       bool
	OSCode       string
	OSArch       string
	Source       config.InventorySourceDescriptor
	// ExpectedPlanDigest pins an interactive confirmation to the plan shown
	// before the prompt. Direct --yes execution leaves it empty.
	ExpectedPlanDigest string
}

// ConfirmationDigest identifies the mutation inputs shown before an
// interactive confirmation without exposing Inventory values or database
// connection details.
func (d *InventoryCMDBData) ConfirmationDigest() string {
	if d == nil || d.SourceRevision == "" || d.DatabaseTargetDigest == "" {
		return ""
	}
	switch d.Operation {
	case "init":
	case "load":
		if d.TargetDigest == "" {
			return ""
		}
	default:
		return ""
	}
	projection := struct {
		Operation      string
		SourceRevision string
		DatabaseTarget string
		TargetDigest   string
		ExistingSchema int
	}{d.Operation, d.SourceRevision, d.DatabaseTargetDigest, d.TargetDigest, d.ExistingSchemas}
	raw, _ := json.Marshal(projection)
	return contentDigest(raw)
}

func openPigstyCMDB(ctx context.Context, database string) (*cmdb.Store, *output.Result) {
	resolved, err := cmdb.ResolvePigstyConnection(cmdb.PigstyConnectionOptions{Database: database})
	if err != nil {
		return nil, cmdbFailureResult("CMDB connection source is invalid", err)
	}
	store, err := cmdb.Open(ctx, resolved, cmdb.OpenOptions{})
	if err != nil {
		return nil, cmdbFailureResult("CMDB connection failed", err)
	}
	return store, nil
}

func InventoryCMDBCheck(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	data := &InventoryCMDBData{
		Kind: InventoryCMDBCheckKind, Operation: "check", ConnectionSource: store.Source(), DatabaseTargetDigest: store.TargetFingerprint(),
	}
	inspection, err := store.InspectPigstyCMDB(ctx)
	if err != nil {
		return inventoryCMDBFailure("Pigsty CMDB health check failed", err, data)
	}
	populateCMDBInspection(data, inspection)
	data.TargetDigest = contentDigest(inspection.CanonicalRaw)
	data.Findings = append(data.Findings,
		CMDBFinding{Code: "CMDB_REQUIRED_OBJECTS_OK", Status: "pass"},
		CMDBFinding{Code: "CMDB_RAW_CONFIG_OK", Status: "pass"},
		CMDBFinding{Code: "CMDB_DYNAMIC_INVENTORY_OK", Status: "pass"},
	)
	if options.Source.Type == config.InventorySourceManagedCMDB {
		finding, err := checkManagedLocalSource(options.Home, options.Source)
		if err != nil {
			data.Findings = append(data.Findings, finding)
			return inventoryCMDBFailure("managed CMDB Inventory source is inconsistent", err, data)
		}
		data.Findings = append(data.Findings, finding)
	}
	if strings.TrimSpace(options.Path) != "" {
		data.SourcePath = strings.TrimSpace(options.Path)
		prepared, validationFailure := preparePigstyInventory(options.Path, false)
		if validationFailure != nil {
			copyCMDBValidation(data, validationFailure)
			return output.Fail(validationFailure.Code, validationFailure.Message).WithData(data)
		}
		equivalent := populateCMDBComparison(data, prepared, inspection.CanonicalRaw)
		if equivalent {
			data.Findings = append(data.Findings, CMDBFinding{Code: "CMDB_DECLARATIONS_EQUIVALENT", Status: "pass", Path: prepared.Path})
		} else {
			data.Findings = append(data.Findings, CMDBFinding{Code: "CMDB_DECLARATIONS_DIFFER", Status: "warn", Path: prepared.Path})
		}
	}
	return output.OK("Pigsty CMDB check passed", data)
}

func InventoryCMDBInit(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	if options.Plan && options.Yes {
		return cmdbFailureResult("--plan and --yes are mutually exclusive", &cmdb.Error{Code: "CMDB_PARAMETER_INVALID"})
	}
	home := strings.TrimSpace(options.Home)
	if home == "" {
		return cmdbFailureResult("Pigsty home is required for CMDB init", &cmdb.Error{Code: "CMDB_SOURCE_INVALID"})
	}
	snapshot, err := readCMDBSourceSnapshot(filepath.Join(home, "files", "cmdb.sql"))
	if err != nil {
		return cmdbFailureResult("Pigsty cmdb.sql baseline is unavailable", err)
	}
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	data := &InventoryCMDBData{
		Kind: InventoryCMDBInitKind, Operation: "init", ConnectionSource: store.Source(),
		DatabaseTargetDigest: store.TargetFingerprint(), SourcePath: snapshot.Path,
		SourceDigest: snapshot.Digest, SourceRevision: snapshot.Digest, Changed: true,
	}
	inspection, err := store.InspectPigstyBaseline(ctx)
	if err != nil {
		return inventoryCMDBFailure("CMDB target could not be inspected", err, data)
	}
	data.ExistingSchemas = inspection.ExistingSchemas
	data.RequiresConfirmation = inspection.ExistingSchemas > 0
	data.Plan = buildCMDBInitPlan(data, inspection.ExistingSchemas)
	if options.ExpectedPlanDigest != "" && options.ExpectedPlanDigest != data.ConfirmationDigest() {
		return inventoryCMDBFailure("CMDB init plan changed after confirmation", &cmdb.Error{Code: "CMDB_SOURCE_CHANGED"}, data)
	}
	if options.Plan {
		data.Planned = true
		return output.OK("Pigsty CMDB init plan ready", data)
	}
	if data.RequiresConfirmation && !options.Yes {
		next := safeCMDBNextAction(data, "pig inventory cmdb init --yes", "confirm replacement of existing Pigsty schemas", true)
		if data.Plan != nil && len(data.Plan.NextActions) > 0 {
			next = data.Plan.NextActions[0]
		}
		return output.Fail(output.CodeInventoryCMDBConfirmationRequired, "CMDB init will replace existing pigsty/pglog schemas").
			WithData(data).
			WithNextActions(next)
	}
	if !cmdbSourceSnapshotCurrent(snapshot) {
		return inventoryCMDBFailure("Pigsty cmdb.sql changed after planning", &cmdb.Error{Code: "CMDB_SOURCE_CHANGED"}, data)
	}
	result, err := store.ApplyPigstyBaseline(ctx, snapshot.Body)
	if err != nil {
		return inventoryCMDBFailure("Pigsty cmdb.sql baseline could not be applied", err, data)
	}
	data.Objects, data.Statements = result.VerifiedObjects, result.StatementResults
	return output.OK("Pigsty CMDB initialized", data)
}

func InventoryCMDBLoad(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	if options.Plan && options.Yes {
		return cmdbFailureResult("--plan and --yes are mutually exclusive", &cmdb.Error{Code: "CMDB_PARAMETER_INVALID"})
	}
	if !options.ExplicitPath && options.Source.Type != "" && options.Source.Type != config.InventorySourceStaticYAML {
		return output.Fail(output.CodeInventoryCMDBConfigInvalid, "active Inventory source is not one static YAML file").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_SOURCE_INVALID"}).
			WithNextActions(output.NextAction{Command: "pig inventory cmdb load <file>", Reason: "provide an explicit static Inventory file", Required: true})
	}
	prepared, failure := preparePigstyInventory(options.Path, options.Strict)
	if failure != nil {
		return failure
	}
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	inspection, err := store.InspectPigstyCMDB(ctx)
	if err != nil {
		return inventoryCMDBFailure("CMDB target could not be inspected", err, nil)
	}
	data := &InventoryCMDBData{
		Kind: InventoryCMDBLoadKind, Operation: "load", ConnectionSource: store.Source(),
		DatabaseTargetDigest: store.TargetFingerprint(), Strict: options.Strict,
	}
	populateCMDBComparison(data, prepared, inspection.CanonicalRaw)
	populateDeclarationCounts(data, prepared.Declarations)
	data.RequiresConfirmation = data.Changed
	preflightPigstyOSDefaults(data, options.Home, options.OSCode, options.OSArch)
	data.Plan = buildCMDBLoadPlan(data)
	if options.ExpectedPlanDigest != "" && options.ExpectedPlanDigest != data.ConfirmationDigest() {
		return inventoryCMDBFailure("CMDB load plan changed after confirmation", &cmdb.Error{Code: "CMDB_SOURCE_CHANGED"}, data)
	}
	if options.Plan {
		data.Planned = true
		return output.OK("Pigsty CMDB load plan ready", data)
	}
	if data.Noop {
		return output.OK("Pigsty CMDB already matches the Inventory", data)
	}
	if !options.Yes {
		next := output.NextAction{Command: "pig inventory cmdb load --yes", Reason: "confirm replacement of CMDB declaration rows", Required: true}
		if data.Plan != nil && len(data.Plan.NextActions) > 0 {
			next = data.Plan.NextActions[0]
		}
		return output.Fail(output.CodeInventoryCMDBConfirmationRequired, "CMDB load will replace all declaration rows").
			WithData(data).
			WithNextActions(next)
	}
	if !preparedSourceCurrent(prepared) {
		return inventoryCMDBFailure("Inventory source changed after planning", &cmdb.Error{Code: "CMDB_SOURCE_CHANGED"}, data)
	}
	loaded, err := store.ReplaceDeclarationsIfCurrent(ctx, prepared.Declarations, inspection.CanonicalRaw)
	if err != nil {
		return inventoryCMDBFailure("Pigsty Inventory could not be loaded", err, data)
	}
	data.Globals, data.Groups, data.Hosts = loaded.Globals, loaded.Groups, loaded.Hosts
	data.GroupVars, data.HostVars = loaded.GroupVar, loaded.HostVar

	defaults, warning := readPigstyOSDefaults(options.Home, options.OSCode, options.OSArch)
	if warning != "" {
		appendCMDBWarning(data, warning)
	} else if len(defaults.Values) > 0 || len(defaults.Packages) > 0 {
		updated, err := store.ApplyOSDefaults(ctx, defaults)
		if err != nil {
			appendCMDBWarning(data, "CMDB_DEFAULTS_FAILED")
		} else {
			data.DefaultVars, data.Packages = updated.Values, updated.Packages
		}
	}
	return output.OK("Pigsty Inventory loaded into CMDB", data)
}

// InventoryCMDBDump returns secret-bearing YAML separately from safe Result
// metadata. With a destination, the returned body is nil.
func InventoryCMDBDump(ctx context.Context, options InventoryCMDBOptions) ([]byte, *output.Result) {
	if strings.TrimSpace(options.Path) == "" && options.Force {
		return nil, output.Fail(output.CodeInventoryCMDBInvalidArgs, "--force requires a CMDB dump destination file")
	}
	if strings.TrimSpace(options.Path) == "" && config.IsStructuredOutput() {
		return nil, output.Fail(output.CodeInventoryCMDBInvalidArgs, "structured CMDB dump requires a destination file")
	}
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return nil, failure
	}
	defer store.Close()
	raw, err := store.RawConfig(ctx)
	data := &InventoryCMDBData{
		Kind: InventoryCMDBDumpKind, Operation: "dump", ConnectionSource: store.Source(), DatabaseTargetDigest: store.TargetFingerprint(),
	}
	if err != nil {
		return nil, inventoryCMDBFailure("Pigsty raw_config could not be exported", err, data)
	}
	body, canonical, err := prepareCMDBDump(raw)
	if err != nil {
		return nil, inventoryCMDBFailure("CMDB dump declaration equivalence could not be verified", err, data)
	}
	data.SourceDigest = contentDigest(canonical)
	if strings.TrimSpace(options.Path) == "" {
		return body, output.OK("Pigsty CMDB dumped", data)
	}
	committed, err := commitCMDBDumpDestination(options.Path, body, options.Force)
	if err != nil {
		if cmdb.IsCode(err, "CMDB_OUTPUT_EXISTS") {
			path, _ := filepath.Abs(strings.TrimSpace(options.Path))
			data.OutputPath = filepath.Clean(path)
			return nil, output.Fail(output.CodeInventoryCMDBOutputFailed, "CMDB dump destination already contains different bytes").
				WithData(data).
				WithNextActions(safeCMDBNextAction(data, "pig inventory cmdb dump "+shellCommandQuote(data.OutputPath)+" --force", "replace the differing destination", true))
		}
		return nil, inventoryCMDBFailure("CMDB dump destination could not be committed", err, data)
	}
	data.OutputPath, data.Changed, data.Noop = committed.Path, committed.Changed, committed.Noop
	if committed.Warning != "" {
		data.Warnings = append(data.Warnings, committed.Warning)
	}
	if data.Noop {
		return nil, output.OK("Pigsty CMDB dump is unchanged", data)
	}
	return nil, output.OK("Pigsty CMDB dumped", data)
}

func InventoryCMDBEnable(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	data := &InventoryCMDBData{
		Kind: InventoryCMDBEnableKind, Operation: "enable", ConnectionSource: store.Source(), DatabaseTargetDigest: store.TargetFingerprint(),
	}
	plan, err := store.PlanEnablePigstyInventory(ctx, options.Home)
	if err != nil {
		return cmdbSourceGuardFailure("dynamic Pigsty Inventory could not be enabled", err, data, options.Home, "enable")
	}
	populateCMDBSourcePlan(data, plan)
	data.Plan = buildCMDBSourcePlan(data, "enable")
	if options.Plan {
		data.Planned = true
		return output.OK("dynamic Pigsty Inventory enable plan ready", data)
	}
	result, err := plan.Apply()
	if err != nil {
		return cmdbSourceGuardFailure("dynamic Pigsty Inventory could not be enabled", err, data, options.Home, "enable")
	}
	data.ConfigPath, data.InventoryPath, data.Changed = result.ConfigPath, result.InventoryPath, result.Changed
	data.ScriptChanged, data.ConfigChanged = result.ScriptChanged, result.ConfigChanged
	data.Noop = !data.Changed
	return output.OK("dynamic Pigsty Inventory enabled", data)
}

func InventoryCMDBDisable(options InventoryCMDBOptions) *output.Result {
	data := &InventoryCMDBData{Kind: InventoryCMDBDisableKind, Operation: "disable"}
	plan, err := cmdb.PlanDisablePigstyInventory(options.Home)
	if err != nil {
		return cmdbSourceGuardFailure("static Pigsty Inventory could not be restored", err, data, options.Home, "disable")
	}
	populateCMDBSourcePlan(data, plan)
	data.Plan = buildCMDBSourcePlan(data, "disable")
	if options.Plan {
		data.Planned = true
		return output.OK("static Pigsty Inventory restore plan ready", data)
	}
	result, err := plan.Apply()
	if err != nil {
		return cmdbSourceGuardFailure("static Pigsty Inventory could not be restored", err, data, options.Home, "disable")
	}
	data.ConfigPath, data.InventoryPath, data.Changed = result.ConfigPath, result.InventoryPath, result.Changed
	data.ScriptChanged, data.ConfigChanged = result.ScriptChanged, result.ConfigChanged
	data.Noop = !data.Changed
	return output.OK("static Pigsty Inventory restored", data)
}

func inventoryCMDBFailure(message string, err error, data *InventoryCMDBData) *output.Result {
	result := cmdbFailureResult(message, err)
	if data != nil {
		code, _ := cmdbFailureCode(err)
		data.CMDBCode = code
		result.Data = data
	}
	return result
}

func cmdbFailureResult(message string, err error) *output.Result {
	code, status := cmdbFailureCode(err)
	return output.Fail(status, message).WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: code})
}

func cmdbFailureCode(err error) (string, int) {
	var failure *cmdb.Error
	if !errors.As(err, &failure) || failure == nil || failure.Code == "" {
		return "CMDB_INTERNAL", output.CodeInventoryCMDBOperationFailed
	}
	switch failure.Code {
	case "CMDB_PARAMETER_INVALID", "CMDB_DECLARATION_INVALID":
		return failure.Code, output.CodeInventoryCMDBInvalidArgs
	case "CMDB_SWITCH_PERMISSION":
		return failure.Code, output.CodeInventoryCMDBPermissionDenied
	case "CMDB_CONNECTION_FAILED":
		return failure.Code, output.CodeInventoryCMDBConnectionFailed
	case "CMDB_CONNECTION_INVALID", "CMDB_SOURCE_INVALID", "CMDB_CONNECTION_SECRET", "CMDB_ANSIBLE_CONFIG_INVALID", "CMDB_SWITCH_INVALID",
		"CMDB_INVENTORY_SCRIPT_UNMANAGED", "CMDB_INVENTORY_DIRECTIVE_UNMANAGED", "CMDB_MANAGED_SOURCE_INVALID", "CMDB_RAW_CONFIG_INVALID":
		return failure.Code, output.CodeInventoryCMDBConfigInvalid
	case "CMDB_NESTED_GROUP_UNSUPPORTED", "CMDB_HOSTS_REQUIRED", "CMDB_HOST_VARS_REQUIRED":
		return failure.Code, output.CodeInventoryCMDBUnrepresentable
	case "CMDB_BASELINE_INVALID", "CMDB_BASELINE_VERIFY_FAILED":
		return failure.Code, output.CodeInventoryCMDBSchemaInvalid
	case "CMDB_OUTPUT_INVALID", "CMDB_OUTPUT_FAILED", "CMDB_SWITCH_PARTIAL":
		return failure.Code, output.CodeInventoryCMDBOutputFailed
	case "CMDB_SOURCE_CHANGED":
		return failure.Code, output.CodeInventoryCMDBSourceChanged
	case "CMDB_CONFIRMATION_REQUIRED":
		return failure.Code, output.CodeInventoryCMDBConfirmationRequired
	default:
		return failure.Code, output.CodeInventoryCMDBOperationFailed
	}
}

type preparedPigstyInventory struct {
	Path         string
	Document     *inventory.Document
	Declarations cmdb.Declarations
	Diagnostics  []inventory.Diagnostic
	Canonical    []byte
	Revision     string
	Snapshot     *cmdbSourceSnapshot
}

func preparePigstyInventory(path string, strict bool) (*preparedPigstyInventory, *output.Result) {
	snapshot, err := readInventoryCMDBSourceSnapshot(path)
	if err != nil {
		return nil, cmdbFailureResult("Inventory source must be one bounded regular file", err)
	}
	doc, err := inventory.Parse(snapshot.Path, snapshot.Body)
	if err != nil {
		diagnostic := parseDiagnostic(err)
		return nil, output.Fail(output.CodeInventoryValidationFailed, "Inventory validation failed before CMDB operation").
			WithData(&CMDBValidationData{Kind: CMDBValidationKind, Path: snapshot.Path, Diagnostics: inventoryDiagnostics([]inventory.Diagnostic{diagnostic})})
	}
	diagnostics := doc.ValidateComplete()
	inventory.SortDiagnostics(diagnostics)
	if inventory.HasErrors(diagnostics) || strict && hasWarnings(diagnostics) {
		return nil, output.Fail(output.CodeInventoryValidationFailed, "Inventory validation failed before CMDB operation").
			WithData(&CMDBValidationData{Kind: CMDBValidationKind, Path: snapshot.Path, Diagnostics: inventoryDiagnostics(diagnostics)})
	}
	declarations, representabilityFailure := pigstyDeclarationsFromDocument(doc, snapshot.Path)
	if representabilityFailure != nil {
		return nil, representabilityFailure
	}
	canonical, err := cmdb.CanonicalDeclarationsJSON(declarations)
	if err != nil {
		return nil, cmdbRepresentabilityFailure(snapshot.Path, "CMDB_DECLARATION_INVALID")
	}
	return &preparedPigstyInventory{
		Path: snapshot.Path, Document: doc, Declarations: declarations, Diagnostics: diagnostics,
		Canonical: canonical, Revision: snapshot.Digest, Snapshot: snapshot,
	}, nil
}

func pigstyDeclarationsFromDocument(doc *inventory.Document, path string) (cmdb.Declarations, *output.Result) {
	semantic := doc.Inventory()
	if semantic == nil || semantic.All == nil || len(semantic.All.Hosts) > 0 {
		return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
	}
	for _, group := range semantic.Groups {
		if group != nil && !cmdbAllChildGroupPath(group.Path) {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_ROOT_GROUP_UNSUPPORTED")
		}
	}
	globals, err := valueFields(semantic.All.Vars)
	if err != nil {
		return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
	}
	declarations := cmdb.Declarations{Globals: globals, Groups: make([]cmdb.GroupDeclaration, 0, len(semantic.All.Children))}
	for _, group := range semantic.All.Children {
		if group == nil || len(group.Children) > 0 {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_NESTED_GROUP_UNSUPPORTED")
		}
		groupValue, ok := doc.Lookup(group.Path)
		if !ok || groupValue.Kind != inventory.MappingValue {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
		}
		hostsValue, ok := groupValue.LookupKey("hosts")
		if !ok || hostsValue.Kind != inventory.MappingValue {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_HOSTS_REQUIRED")
		}
		vars, err := valueFields(group.Vars)
		if err != nil {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
		}
		declaration := cmdb.GroupDeclaration{Name: group.Name, Vars: vars, Hosts: make([]cmdb.HostDeclaration, 0, len(group.Hosts))}
		for _, host := range group.Hosts {
			if host == nil || host.Vars.Kind != inventory.MappingValue {
				return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_HOST_VARS_REQUIRED")
			}
			vars, err := valueFields(&host.Vars)
			if err != nil {
				return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
			}
			declaration.Hosts = append(declaration.Hosts, cmdb.HostDeclaration{IP: host.Name, Vars: vars})
		}
		declarations.Groups = append(declarations.Groups, declaration)
	}
	return declarations, nil
}

func cmdbAllChildGroupPath(path inventory.Path) bool {
	return len(path) >= 3 &&
		path[0].Kind == inventory.KeySegment && path[0].Key == "all" &&
		path[1].Kind == inventory.KeySegment && path[1].Key == "children"
}

func cmdbRepresentabilityFailure(path, code string) *output.Result {
	return output.Fail(output.CodeInventoryCMDBUnrepresentable, "Inventory cannot be represented by the existing Pigsty CMDB").
		WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: code}).
		WithDetail("source: " + path)
}

func valueFields(value *inventory.Value) ([]cmdb.JSONField, error) {
	if value == nil {
		return nil, nil
	}
	if value.Kind != inventory.MappingValue {
		return nil, errors.New("declaration vars must be a mapping")
	}
	fields := make([]cmdb.JSONField, 0, len(value.Mapping))
	for _, entry := range value.Mapping {
		key, ok := entry.Key.Value.(string)
		if !ok || key == "" {
			return nil, errors.New("declaration key must be a string")
		}
		plain, err := plainInventoryValue(&entry.Value)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(plain)
		if err != nil {
			return nil, err
		}
		fields = append(fields, cmdb.JSONField{Key: key, Value: encoded})
	}
	return fields, nil
}

func plainInventoryValue(value *inventory.Value) (any, error) {
	if value == nil || value.Kind == inventory.NullValue {
		return nil, nil
	}
	if value.Anchor != "" || value.Alias != "" || strings.HasPrefix(value.Tag, "!") && !strings.HasPrefix(value.Tag, "!!") {
		return nil, errors.New("tagged or aliased value")
	}
	switch value.Kind {
	case inventory.MappingValue:
		mapping := make(map[string]any, len(value.Mapping))
		for _, entry := range value.Mapping {
			key, ok := jsonMappingKey(entry.Key)
			if !ok {
				return nil, errors.New("mapping key is not a string")
			}
			if _, exists := mapping[key]; exists {
				return nil, errors.New("mapping keys collide after JSON string coercion")
			}
			child, err := plainInventoryValue(&entry.Value)
			if err != nil {
				return nil, err
			}
			mapping[key] = child
		}
		return mapping, nil
	case inventory.SequenceValue:
		sequence := make([]any, len(value.Sequence))
		for index := range value.Sequence {
			child, err := plainInventoryValue(&value.Sequence[index])
			if err != nil {
				return nil, err
			}
			sequence[index] = child
		}
		return sequence, nil
	case inventory.ScalarValue:
		if value.Scalar == nil || value.Scalar.Style == "tagged" {
			return nil, errors.New("unsupported scalar")
		}
		switch value.Scalar.Kind {
		case inventory.StringScalar, inventory.BoolScalar, inventory.IntegerScalar, inventory.FloatScalar:
			return value.Scalar.Value, nil
		case inventory.NullScalar:
			return nil, nil
		default:
			return nil, errors.New("unknown scalar")
		}
	default:
		return nil, errors.New("unsupported value")
	}
}

func jsonMappingKey(key inventory.Scalar) (string, bool) {
	switch key.Kind {
	case inventory.StringScalar:
		value, ok := key.Value.(string)
		return value, ok
	case inventory.BoolScalar, inventory.IntegerScalar, inventory.FloatScalar:
		return fmt.Sprint(key.Value), true
	case inventory.NullScalar:
		return "null", true
	default:
		return "", false
	}
}

var osDefaultKeys = map[string]string{
	"repo_packages_default":       "repo_packages",
	"repo_extra_packages_default": "repo_extra_packages",
	"node_packages_default":       "node_packages",
	"infra_packages_default":      "infra_packages",
	"repo_upstream_default":       "repo_upstream",
}

func readPigstyOSDefaults(home, osCode, arch string) (cmdb.OSDefaults, string) {
	osCode, arch = strings.TrimSpace(strings.ToLower(osCode)), normalizeNodeVarsArch(arch)
	fallback := ""
	switch {
	case strings.HasPrefix(osCode, "el"):
		fallback = "el9"
	case strings.HasPrefix(osCode, "d"):
		fallback = "d12"
	case strings.HasPrefix(osCode, "u"):
		fallback = "u22"
	default:
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_UNAVAILABLE"
	}
	directory := filepath.Join(home, "roles", "node_id", "vars")
	path := filepath.Join(directory, osCode+"."+arch+".yml")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(directory, fallback+"."+arch+".yml")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_UNAVAILABLE"
	}
	doc, err := inventory.Parse(path, source)
	if err != nil {
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
	}
	root, ok := doc.Lookup(nil)
	if !ok {
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
	}
	plain, err := plainInventoryValue(root)
	if err != nil {
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
	}
	values, ok := plain.(map[string]any)
	if !ok {
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
	}
	defaults := cmdb.OSDefaults{}
	for sourceKey, targetKey := range osDefaultKeys {
		value, exists := values[sourceKey]
		if !exists {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
		}
		defaults.Values = append(defaults.Values, cmdb.JSONField{Key: targetKey, Value: encoded})
	}
	if packages, ok := values["package_map"].(map[string]any); ok {
		for key, value := range packages {
			defaults.Packages = append(defaults.Packages, cmdb.TextField{Key: key, Value: fmt.Sprint(value)})
		}
	}
	return defaults, ""
}

func preflightPigstyOSDefaults(data *InventoryCMDBData, home, osCode, arch string) {
	if data == nil || !data.Changed {
		return
	}
	_, warning := readPigstyOSDefaults(home, osCode, arch)
	if warning != "" {
		appendCMDBWarning(data, warning)
	}
}

func appendCMDBWarning(data *InventoryCMDBData, warning string) {
	if data == nil || warning == "" {
		return
	}
	for _, existing := range data.Warnings {
		if existing == warning {
			return
		}
	}
	data.Warnings = append(data.Warnings, warning)
}

func normalizeNodeVarsArch(arch string) string {
	switch strings.TrimSpace(strings.ToLower(arch)) {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	default:
		return strings.TrimSpace(strings.ToLower(arch))
	}
}

func canonicalCMDBYAML(source []byte) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(source, &document); err != nil {
		return nil, err
	}
	clearGeneratedYAMLStyle(&document)
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func clearGeneratedYAMLStyle(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style = 0
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && yaml11AmbiguousString(node.Value) {
		node.Style = yaml.DoubleQuotedStyle
	}
	for _, child := range node.Content {
		clearGeneratedYAMLStyle(child)
	}
}

func yaml11AmbiguousString(value string) bool {
	return inventory.IsAmbiguousPlainString(value)
}
