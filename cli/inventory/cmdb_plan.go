package inventorycli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"pig/inventory"
)

const maxCMDBSourceSize = 32 << 20

type cmdbSourceSnapshot struct {
	Path       string
	Body       []byte
	Digest     string
	Info       os.FileInfo
	LinkInfo   os.FileInfo
	LinkTarget string
}

func readCMDBSourceSnapshot(path string) (*cmdbSourceSnapshot, error) {
	return readRegularCMDBSnapshot(path, "CMDB_BASELINE_INVALID", false)
}

func readInventoryCMDBSourceSnapshot(path string) (*cmdbSourceSnapshot, error) {
	return readRegularCMDBSnapshot(path, "CMDB_SOURCE_INVALID", true)
}

func readRegularCMDBSnapshot(path, code string, allowSymlink bool) (*cmdbSourceSnapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, &cmdb.Error{Code: code, Message: "source path is empty"}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, &cmdb.Error{Code: code, Message: "source path is invalid"}
	}
	absolute = filepath.Clean(absolute)
	linkBefore, err := os.Lstat(absolute)
	if err != nil || !validCMDBSourceType(linkBefore, allowSymlink) {
		return nil, &cmdb.Error{Code: code, Message: "source is not one bounded regular file"}
	}
	linkTarget, err := cmdbSourceLinkTarget(absolute, linkBefore)
	if err != nil {
		return nil, &cmdb.Error{Code: code, Message: "source symlink could not be inspected"}
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, &cmdb.Error{Code: code, Message: "source could not be opened"}
	}
	before, statErr := file.Stat()
	body, readErr := io.ReadAll(io.LimitReader(file, maxCMDBSourceSize+1))
	after, afterErr := file.Stat()
	closeErr := file.Close()
	linkAfter, pathErr := os.Lstat(absolute)
	linkTargetAfter := ""
	if pathErr == nil && linkAfter.Mode()&os.ModeSymlink != 0 {
		linkTargetAfter, pathErr = os.Readlink(absolute)
	}
	observation := cmdbSnapshotObservation{
		allowSymlink: allowSymlink, body: body,
		linkTarget: linkTarget, linkTargetAfter: linkTargetAfter,
		before: before, after: after, linkBefore: linkBefore, linkAfter: linkAfter,
		errs: []error{readErr, statErr, afterErr, closeErr, pathErr},
	}
	if !validCMDBSnapshotRead(observation) {
		return nil, &cmdb.Error{Code: code, Message: "source changed while it was read"}
	}
	return &cmdbSourceSnapshot{
		Path: absolute, Body: body, Digest: contentDigest(body), Info: before,
		LinkInfo: linkBefore, LinkTarget: linkTarget,
	}, nil
}

func validCMDBSourceType(info os.FileInfo, allowSymlink bool) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return allowSymlink
	}
	return info.Mode().IsRegular()
}

func cmdbSourceLinkTarget(path string, info os.FileInfo) (string, error) {
	if info.Mode()&os.ModeSymlink == 0 {
		return "", nil
	}
	return os.Readlink(path)
}

type cmdbSnapshotObservation struct {
	allowSymlink                bool
	body                        []byte
	linkTarget, linkTargetAfter string
	before, after               os.FileInfo
	linkBefore, linkAfter       os.FileInfo
	errs                        []error
}

func validCMDBSnapshotRead(observation cmdbSnapshotObservation) bool {
	for _, err := range observation.errs {
		if err != nil {
			return false
		}
	}
	return observation.before.Mode().IsRegular() && (!observation.allowSymlink || observation.before.Mode().Perm()&0o111 == 0) &&
		observation.before.Size() <= maxCMDBSourceSize && len(observation.body) > 0 && len(observation.body) <= maxCMDBSourceSize &&
		sameCMDBFileRevision(observation.before, observation.after) && sameCMDBFileRevision(observation.linkBefore, observation.linkAfter) &&
		observation.linkTarget == observation.linkTargetAfter
}

func sameCMDBFileRevision(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) && left.Mode() == right.Mode() &&
		left.Size() == right.Size() && left.ModTime().Equal(right.ModTime())
}

func cmdbSourceSnapshotCurrent(snapshot *cmdbSourceSnapshot) bool {
	if snapshot == nil {
		return false
	}
	current, err := readRegularCMDBSnapshot(snapshot.Path, "CMDB_SOURCE_CHANGED", snapshot.LinkInfo.Mode()&os.ModeSymlink != 0)
	return err == nil && current.Digest == snapshot.Digest && bytes.Equal(current.Body, snapshot.Body) &&
		sameCMDBFileRevision(snapshot.Info, current.Info) && sameCMDBFileRevision(snapshot.LinkInfo, current.LinkInfo) &&
		snapshot.LinkTarget == current.LinkTarget
}

func preparedSourceCurrent(prepared *preparedPigstyInventory) bool {
	if prepared == nil || prepared.Snapshot == nil {
		return false
	}
	return cmdbSourceSnapshotCurrent(prepared.Snapshot)
}

func contentDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("sha256:%x", sum)
}

func populateCMDBInspection(data *InventoryCMDBData, inspection cmdb.PigstyInspection) {
	if data == nil {
		return
	}
	data.Objects = inspection.VerifiedObjects
	data.Globals, data.Groups, data.Hosts = inspection.Globals, inspection.Groups, inspection.Hosts
	data.GroupVars, data.HostVars = inspection.GroupVars, inspection.HostVars
}

func populateDeclarationCounts(data *InventoryCMDBData, declarations cmdb.Declarations) {
	if data == nil {
		return
	}
	data.Globals, data.Groups = len(declarations.Globals), len(declarations.Groups)
	for _, group := range declarations.Groups {
		data.GroupVars += len(group.Vars)
		data.Hosts += len(group.Hosts)
		for _, host := range group.Hosts {
			data.HostVars += len(host.Vars)
		}
	}
}

func populateCMDBComparison(data *InventoryCMDBData, source *preparedPigstyInventory, targetCanonical []byte) bool {
	if data == nil || source == nil {
		return false
	}
	equivalent := bytes.Equal(source.Canonical, targetCanonical)
	data.SourcePath = source.Path
	data.SourceRevision = source.Revision
	data.SourceDigest = contentDigest(source.Canonical)
	data.TargetDigest = contentDigest(targetCanonical)
	data.Diagnostics = inventoryDiagnostics(source.Diagnostics)
	data.Compared, data.Equivalent = true, &equivalent
	data.Changed, data.Noop = !equivalent, equivalent
	data.Changes = cmdbValueFreeChanges(source.Document, source.Canonical, targetCanonical)
	return equivalent
}

func buildCMDBInitPlan(data *InventoryCMDBData, existingSchemas int) *output.Plan {
	confirmation, impact := "none", "create"
	risks := []string(nil)
	if existingSchemas > 0 {
		confirmation, impact = "required", "replace"
		risks = []string{"existing pigsty and pglog schemas will be replaced"}
	}
	plan := &output.Plan{
		API: output.APIVersion, Command: "pig inventory cmdb init", Boundary: "database schemas", Confirmation: confirmation,
		Actions: []output.Action{
			{Step: 1, Description: "verify the selected cmdb.sql source revision"},
			{Step: 2, Description: "apply the exact version-matched baseline"},
			{Step: 3, Description: "verify required Pigsty CMDB objects"},
		},
		Affects: []output.Resource{
			{Type: "schema", Name: "pigsty", Impact: impact},
			{Type: "schema", Name: "pglog", Impact: impact},
		},
		Expected: "required Pigsty CMDB relations and projections are present", Risks: risks,
		Preconditions: []output.Check{
			{Name: "baseline", Status: "pass", Detail: data.SourceDigest},
			{Name: "database target", Status: "pass", Detail: data.DatabaseTargetDigest},
			{Name: "existing schemas", Status: fmt.Sprint(existingSchemas)},
		},
	}
	if existingSchemas > 0 {
		plan.NextActions = []output.NextAction{safeCMDBNextAction(data, "pig inventory cmdb init --yes", "execute the reviewed replacement", true)}
	} else {
		plan.NextActions = []output.NextAction{safeCMDBNextAction(data, "pig inventory cmdb init", "execute the reviewed initialization", false)}
	}
	return plan
}

func buildCMDBLoadPlan(data *InventoryCMDBData) *output.Plan {
	impact, confirmation := "none", "none"
	risks := []string(nil)
	if data != nil && data.Changed {
		impact, confirmation = "replace", "required"
		risks = []string{"all five declaration tables will be replaced in one transaction"}
	}
	plan := &output.Plan{
		API: output.APIVersion, Command: "pig inventory cmdb load", Boundary: "CMDB declaration tables", Confirmation: confirmation,
		Actions: []output.Action{
			{Step: 1, Description: "recheck the validated static Inventory revision"},
			{Step: 2, Description: "lock and compare the current canonical declaration revision"},
			{Step: 3, Description: "replace declarations transactionally when changed"},
			{Step: 4, Description: "apply OS defaults separately and report any warning"},
		},
		Affects:  []output.Resource{{Type: "tables", Name: "pigsty declarations", Impact: impact, Detail: "global, group, group_var, host, host_var"}},
		Expected: "CMDB raw_config is declaration-equivalent to the selected static Inventory", Risks: risks,
	}
	if data != nil {
		plan.Preconditions = []output.Check{
			{Name: "database target", Status: "pass", Detail: data.DatabaseTargetDigest},
			{Name: "source revision", Status: "pass", Detail: data.SourceRevision},
			{Name: "source declarations", Status: "pass", Detail: data.SourceDigest},
			{Name: "target declarations", Status: "pass", Detail: data.TargetDigest},
		}
		command := "pig inventory cmdb load " + shellCommandQuote(data.SourcePath)
		if data.Strict {
			command += " --strict"
		}
		if data.Changed {
			command += " --yes"
		}
		plan.NextActions = []output.NextAction{safeCMDBNextAction(data, command, "execute the reviewed declaration plan", data.Changed)}
	}
	return plan
}

func populateCMDBSourcePlan(data *InventoryCMDBData, plan *cmdb.PigstySourcePlan) {
	if data == nil || plan == nil {
		return
	}
	data.ConfigPath, data.InventoryPath = plan.ConfigPath, plan.InventoryPath
	data.Changed, data.Noop = plan.Changed, !plan.Changed
	data.ScriptChanged, data.ConfigChanged = plan.ScriptChanged, plan.ConfigChanged
}

func buildCMDBSourcePlan(data *InventoryCMDBData, operation string) *output.Plan {
	plan := &output.Plan{
		API: output.APIVersion, Command: "pig inventory cmdb " + operation,
		Boundary: "local Inventory source files", Confirmation: "none",
		Actions:  []output.Action{{Step: 1, Description: "recheck supported file revisions and ownership guards"}},
		Expected: "the supported ansible.cfg Inventory directive selects the requested source",
	}
	if operation == "enable" {
		plan.Actions = append(plan.Actions,
			output.Action{Step: 2, Description: "write the password-free managed query adapter when needed"},
			output.Action{Step: 3, Description: "switch the guarded Inventory directive to inventory.sh when needed"},
		)
	} else {
		plan.Actions = append(plan.Actions, output.Action{Step: 2, Description: "switch the guarded Inventory directive to pigsty.yml when needed"})
	}
	if data != nil {
		if data.DatabaseTargetDigest != "" {
			plan.Preconditions = append(plan.Preconditions, output.Check{Name: "database target", Status: "pass", Detail: data.DatabaseTargetDigest})
		}
		if data.ScriptChanged {
			plan.Affects = append(plan.Affects, output.Resource{Type: "file", Name: data.InventoryPath, Impact: "replace"})
		}
		if data.ConfigChanged {
			plan.Affects = append(plan.Affects, output.Resource{Type: "file", Name: data.ConfigPath, Impact: "narrow patch"})
		}
		plan.NextActions = []output.NextAction{safeCMDBNextAction(data, plan.Command, "execute the reviewed source switch", false)}
	}
	return plan
}

func safeCMDBNextAction(data *InventoryCMDBData, command, reason string, required bool) output.NextAction {
	action := output.NextAction{Command: command, Reason: reason, Required: required}
	if data != nil && data.ConnectionSource == cmdb.ConnectionArgSource {
		action.Command = ""
		action.Reason = reason + "; rerun the original command with the same explicit -d target"
	}
	return action
}

func cmdbValueFreeChanges(source *inventory.Document, sourceCanonical, currentCanonical []byte) []InventoryDiffItem {
	if source == nil || bytes.Equal(sourceCanonical, currentCanonical) {
		return nil
	}
	body, err := canonicalCMDBYAML(currentCanonical)
	if err != nil {
		return nil
	}
	current, err := inventory.Parse("CMDB raw_config", body)
	if err != nil {
		return nil
	}
	differences, err := inventory.DiffDocuments(current, source)
	if err != nil {
		return nil
	}
	return inventoryDiffItems(differences)
}

func checkManagedLocalSource(home string, source config.InventorySourceDescriptor) (CMDBFinding, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return CMDBFinding{Code: "CMDB_MANAGED_SOURCE_INVALID", Status: "fail"}, &cmdb.Error{Code: "CMDB_SOURCE_INVALID"}
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return CMDBFinding{Code: "CMDB_MANAGED_SOURCE_INVALID", Status: "fail"}, &cmdb.Error{Code: "CMDB_SOURCE_INVALID"}
	}
	home = filepath.Clean(home)
	scriptPath := filepath.Join(home, "inventory.sh")
	finding := CMDBFinding{Code: "CMDB_MANAGED_SOURCE_OK", Status: "pass", Path: scriptPath}
	if filepath.Clean(source.Path) != scriptPath || !source.Managed {
		finding.Code, finding.Status = "CMDB_MANAGED_SOURCE_INVALID", "fail"
		return finding, &cmdb.Error{Code: "CMDB_SOURCE_INVALID"}
	}
	info, err := os.Lstat(scriptPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Mode().Perm() != 0o755 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		finding.Code, finding.Status = "CMDB_INVENTORY_SCRIPT_UNMANAGED", "fail"
		return finding, &cmdb.Error{Code: "CMDB_INVENTORY_SCRIPT_UNMANAGED"}
	}
	body, err := os.ReadFile(scriptPath)
	if err != nil {
		finding.Code, finding.Status = "CMDB_INVENTORY_SCRIPT_UNMANAGED", "fail"
		return finding, &cmdb.Error{Code: "CMDB_INVENTORY_SCRIPT_UNMANAGED"}
	}
	if _, ok := config.ManagedInventoryScriptShape(body); !ok {
		finding.Code, finding.Status = "CMDB_INVENTORY_SCRIPT_UNMANAGED", "fail"
		return finding, &cmdb.Error{Code: "CMDB_INVENTORY_SCRIPT_UNMANAGED"}
	}
	if config.ManagedInventoryScriptHasPassword(body) {
		finding.Code, finding.Status = "CMDB_CONNECTION_SECRET", "fail"
		return finding, &cmdb.Error{Code: "CMDB_CONNECTION_SECRET"}
	}
	configPath := filepath.Join(home, "ansible.cfg")
	configInfo, err := os.Lstat(configPath)
	if err != nil || configInfo.Mode()&os.ModeSymlink != 0 || !configInfo.Mode().IsRegular() {
		finding.Code, finding.Status, finding.Path = "CMDB_ANSIBLE_CONFIG_INVALID", "fail", configPath
		return finding, &cmdb.Error{Code: "CMDB_ANSIBLE_CONFIG_INVALID"}
	}
	directive, err := config.ReadAnsibleInventoryDirective(configPath)
	if err != nil || directive.Value != "inventory.sh" || directive.Path != scriptPath {
		finding.Code, finding.Status, finding.Path = "CMDB_INVENTORY_DIRECTIVE_UNMANAGED", "fail", configPath
		return finding, &cmdb.Error{Code: "CMDB_INVENTORY_DIRECTIVE_UNMANAGED"}
	}
	return finding, nil
}

func copyCMDBValidation(data *InventoryCMDBData, result *output.Result) {
	if data == nil || result == nil {
		return
	}
	if validation, ok := result.Data.(*CMDBValidationData); ok && validation != nil {
		data.SourcePath = validation.Path
		data.Diagnostics = append(data.Diagnostics, validation.Diagnostics...)
	}
	if failure, ok := result.Data.(*CMDBFailureData); ok && failure != nil && failure.CMDBCode != "" {
		data.Findings = append(data.Findings, CMDBFinding{Code: failure.CMDBCode, Status: "fail", Path: data.SourcePath})
	}
}

func prepareCMDBDump(raw []byte) ([]byte, []byte, error) {
	canonical, err := cmdb.CanonicalRawConfigJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	body, err := canonicalCMDBYAML(canonical)
	if err != nil {
		return nil, nil, &cmdb.Error{Code: "CMDB_DUMP_FAILED"}
	}
	doc, err := inventory.Parse("CMDB dump", body)
	if err != nil {
		return nil, nil, &cmdb.Error{Code: "CMDB_DUMP_FAILED"}
	}
	declarations, failure := pigstyDeclarationsFromDocument(doc, "CMDB dump")
	if failure != nil {
		return nil, nil, &cmdb.Error{Code: "CMDB_DUMP_FAILED"}
	}
	got, err := cmdb.CanonicalDeclarationsJSON(declarations)
	if err != nil || !bytes.Equal(got, canonical) {
		return nil, nil, &cmdb.Error{Code: "CMDB_DUMP_FAILED"}
	}
	return body, canonical, nil
}

type cmdbDumpState uint8

const (
	cmdbDumpMissing cmdbDumpState = iota
	cmdbDumpIdentical
	cmdbDumpDifferent
)

type cmdbDumpSnapshot struct {
	Info os.FileInfo
	Body []byte
}

type cmdbDumpCommit struct {
	Path    string
	Changed bool
	Noop    bool
	Warning string
}

func commitCMDBDumpDestination(path string, candidate []byte, force bool) (*cmdbDumpCommit, error) {
	path = strings.TrimSpace(path)
	if path == "" || len(candidate) == 0 {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_INVALID"}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_INVALID"}
	}
	absolute = filepath.Clean(absolute)
	snapshot, state, err := inspectCMDBDumpDestination(absolute, candidate)
	if err != nil {
		return nil, err
	}
	if state == cmdbDumpIdentical {
		return &cmdbDumpCommit{Path: absolute, Noop: true}, nil
	}
	if state == cmdbDumpDifferent && !force {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_EXISTS"}
	}
	parent := filepath.Dir(absolute)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	temporary, err := os.CreateTemp(parent, "."+filepath.Base(absolute)+".tmp-*")
	if err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	if _, err := temporary.Write(candidate); err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	if err := temporary.Sync(); err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	if err := temporary.Close(); err != nil {
		closed = true
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	closed = true
	if !cmdbDumpDestinationCurrent(absolute, snapshot, state) {
		return nil, &cmdb.Error{Code: "CMDB_SOURCE_CHANGED"}
	}
	if err := os.Rename(temporaryPath, absolute); err != nil {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	warning := ""
	if err := utils.SyncDirectory(parent); err != nil {
		warning = "CMDB_OUTPUT_DURABILITY_UNVERIFIED"
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	body, err := os.ReadFile(absolute)
	if err != nil || !bytes.Equal(body, candidate) {
		return nil, &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}
	}
	return &cmdbDumpCommit{Path: absolute, Changed: true, Warning: warning}, nil
}

func inspectCMDBDumpDestination(path string, candidate []byte) (*cmdbDumpSnapshot, cmdbDumpState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, cmdbDumpMissing, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, cmdbDumpMissing, &cmdb.Error{Code: "CMDB_OUTPUT_INVALID"}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, cmdbDumpMissing, &cmdb.Error{Code: "CMDB_OUTPUT_INVALID"}
	}
	snapshot := &cmdbDumpSnapshot{Info: info, Body: body}
	if bytes.Equal(body, candidate) {
		return snapshot, cmdbDumpIdentical, nil
	}
	return snapshot, cmdbDumpDifferent, nil
}

func cmdbDumpDestinationCurrent(path string, snapshot *cmdbDumpSnapshot, state cmdbDumpState) bool {
	if state == cmdbDumpMissing {
		_, err := os.Lstat(path)
		return errors.Is(err, os.ErrNotExist)
	}
	if snapshot == nil || snapshot.Info == nil {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !sameCMDBFileRevision(snapshot.Info, info) {
		return false
	}
	body, err := os.ReadFile(path)
	return err == nil && bytes.Equal(body, snapshot.Body)
}

func cmdbSourceGuardFailure(message string, err error, data *InventoryCMDBData, home, operation string) *output.Result {
	result := inventoryCMDBFailure(message, err, data)
	code, _ := cmdbFailureCode(err)
	switch code {
	case "CMDB_INVENTORY_SCRIPT_UNMANAGED":
		path := filepath.Join(strings.TrimSpace(home), "inventory.sh")
		result.WithNextActions(output.NextAction{
			Command: "mv -n -- " + shellCommandQuote(path) + " " + shellCommandQuote(path+".pig-backup"),
			Reason:  "move the user-owned script aside before retrying " + operation, Required: true,
		})
	case "CMDB_INVENTORY_DIRECTIVE_UNMANAGED":
		result.WithNextActions(output.NextAction{
			Command: "pig inventory status", Reason: "inspect and manually reconcile the user-owned ansible.cfg Inventory directive", Required: true,
		})
	}
	return result
}

func shellCommandQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
