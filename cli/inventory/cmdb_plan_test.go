package inventorycli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
	"pig/inventory"
)

func TestPreparePigstyInventoryUsesFullValidationAndStrictWarnings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warning.yml")
	source := []byte(`all:
  vars:
    version: v4.4.0
    region: default
    admin_ip: 10.10.10.10
  children:
    demo.bad:
      hosts:
        10.10.10.10: {}
      vars: {}
`)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, failure := preparePigstyInventory(path, false)
	if failure != nil || prepared == nil || len(prepared.Diagnostics) == 0 {
		t.Fatalf("non-strict preparation = %+v, %v", prepared, failure)
	}
	if _, failure := preparePigstyInventory(path, true); failure == nil {
		t.Fatal("strict CMDB preparation accepted validation warnings")
	}
}

func TestCMDBPlanConfirmationConflictAndDynamicDefaultFailBeforeIO(t *testing.T) {
	for name, response := range map[string]*output.Result{
		"init": InventoryCMDBInit(t.Context(), InventoryCMDBOptions{Plan: true, Yes: true}),
		"load": InventoryCMDBLoad(t.Context(), InventoryCMDBOptions{Plan: true, Yes: true}),
	} {
		failure, ok := response.Data.(*CMDBFailureData)
		if response.Success || !ok || failure.CMDBCode != "CMDB_PARAMETER_INVALID" {
			t.Fatalf("%s conflict = %+v", name, response)
		}
	}
	result := InventoryCMDBLoad(t.Context(), InventoryCMDBOptions{
		Path: "/must/not/be/read", Source: config.InventorySourceDescriptor{Type: config.InventorySourceManagedCMDB},
	})
	failure, ok := result.Data.(*CMDBFailureData)
	if result.Success || !ok || failure.CMDBCode != "CMDB_SOURCE_INVALID" || len(result.NextActions) != 1 {
		t.Fatalf("dynamic default load = %+v", result)
	}
	if _, result := InventoryCMDBDump(t.Context(), InventoryCMDBOptions{Force: true}); result.Success || result.Code != output.CodeInventoryCMDBInvalidArgs {
		t.Fatalf("stdout dump with --force = %+v", result)
	}
}

func TestCMDBConfirmationDigestPinsMutationInputs(t *testing.T) {
	base := InventoryCMDBData{
		Operation: "load", SourceRevision: "source", DatabaseTargetDigest: "database", TargetDigest: "target",
	}
	digest := base.ConfirmationDigest()
	if digest == "" {
		t.Fatal("complete load plan has no confirmation digest")
	}
	for name, mutate := range map[string]func(*InventoryCMDBData){
		"operation": func(data *InventoryCMDBData) { data.Operation = "init" },
		"source":    func(data *InventoryCMDBData) { data.SourceRevision = "changed" },
		"database":  func(data *InventoryCMDBData) { data.DatabaseTargetDigest = "changed" },
		"target":    func(data *InventoryCMDBData) { data.TargetDigest = "changed" },
	} {
		changed := base
		mutate(&changed)
		if changed.ConfirmationDigest() == digest {
			t.Fatalf("%s change retained confirmation digest", name)
		}
	}
	base.TargetDigest = ""
	if base.ConfirmationDigest() != "" {
		t.Fatal("incomplete load plan is confirmable")
	}
	base.Operation = "init"
	initDigest := base.ConfirmationDigest()
	base.ExistingSchemas++
	if initDigest == "" || base.ConfirmationDigest() == initDigest {
		t.Fatal("init schema count is not pinned")
	}
}

func TestCMDBSourceSnapshotTracksRegularSymlinkRevision(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.yml")
	link := filepath.Join(directory, "inventory.yml")
	if err := os.WriteFile(target, []byte("all: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.yml", link); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readInventoryCMDBSourceSnapshot(link)
	if err != nil || !cmdbSourceSnapshotCurrent(snapshot) {
		t.Fatalf("symlink snapshot = %+v, %v", snapshot, err)
	}
	if _, err := readCMDBSourceSnapshot(link); err == nil {
		t.Fatal("cmdb.sql baseline accepted a symlink")
	}
	if err := os.Chmod(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readInventoryCMDBSourceSnapshot(link); !cmdb.IsCode(err, "CMDB_SOURCE_INVALID") {
		t.Fatalf("executable Inventory source error = %v", err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("all: {vars: {}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cmdbSourceSnapshotCurrent(snapshot) {
		t.Fatal("changed symlink target retained the old source revision")
	}
}

func TestManagedLocalSourceCheckRejectsPasswordAndUnmanagedShape(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "ansible.cfg")
	scriptPath := filepath.Join(home, "inventory.sh")
	if err := os.WriteFile(configPath, []byte("[defaults]\ninventory = inventory.sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	descriptor := config.InventorySourceDescriptor{
		Type: config.InventorySourceManagedCMDB, Path: scriptPath, Managed: true,
	}
	safe := []byte("#!/bin/bash\nexec psql 'service=meta' -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := os.WriteFile(scriptPath, safe, 0o755); err != nil {
		t.Fatal(err)
	}
	if finding, err := checkManagedLocalSource(home, descriptor); err != nil || finding.Status != "pass" {
		t.Fatalf("safe source finding = %+v, %v", finding, err)
	}
	password := []byte("#!/bin/bash\npsql postgresql://user:secret@db/meta -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := os.WriteFile(scriptPath, password, 0o755); err != nil {
		t.Fatal(err)
	}
	if finding, err := checkManagedLocalSource(home, descriptor); !cmdb.IsCode(err, "CMDB_CONNECTION_SECRET") || finding.Code != "CMDB_CONNECTION_SECRET" {
		t.Fatalf("password source finding = %+v, %v", finding, err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho user-owned\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if finding, err := checkManagedLocalSource(home, descriptor); !cmdb.IsCode(err, "CMDB_INVENTORY_SCRIPT_UNMANAGED") || finding.Status != "fail" {
		t.Fatalf("unmanaged source finding = %+v, %v", finding, err)
	}
}

func TestCMDBDumpDestinationRefusalAndIdempotence(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "dump.yml")
	candidate := []byte("all: {}\n")
	if snapshot, state, err := inspectCMDBDumpDestination(path, candidate); err != nil || snapshot != nil || state != cmdbDumpMissing {
		t.Fatalf("missing destination = %+v, %v, %v", snapshot, state, err)
	}
	createdPath := filepath.Join(directory, "created.yml")
	commit, err := commitCMDBDumpDestination(createdPath, candidate, false)
	if err != nil || !commit.Changed {
		t.Fatalf("missing destination commit = %+v, %v", commit, err)
	}
	createdInfo, err := os.Stat(createdPath)
	if err != nil || createdInfo.Mode().Perm() != 0o600 {
		t.Fatalf("created destination = %v, %v", createdInfo, err)
	}
	if err := os.WriteFile(path, candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, state, err := inspectCMDBDumpDestination(path, candidate)
	if err != nil || state != cmdbDumpIdentical || !cmdbDumpDestinationCurrent(path, snapshot, state) {
		t.Fatalf("identical destination = %+v, %v, %v", snapshot, state, err)
	}
	if err := os.WriteFile(path, []byte("different\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, state, err = inspectCMDBDumpDestination(path, candidate)
	if err != nil || state != cmdbDumpDifferent || !cmdbDumpDestinationCurrent(path, snapshot, state) {
		t.Fatalf("different destination = %+v, %v, %v", snapshot, state, err)
	}
	if _, err := commitCMDBDumpDestination(path, candidate, false); !cmdb.IsCode(err, "CMDB_OUTPUT_EXISTS") {
		t.Fatalf("unforced differing destination error = %v", err)
	}
	if body, _ := os.ReadFile(path); string(body) != "different\n" {
		t.Fatal("unforced dump changed the destination")
	}
	commit, err = commitCMDBDumpDestination(path, candidate, true)
	if err != nil || !commit.Changed || commit.Noop {
		t.Fatalf("forced dump commit = %+v, %v", commit, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("forced dump mode = %v", info.Mode().Perm())
	}
	commit, err = commitCMDBDumpDestination(path, candidate, false)
	if err != nil || !commit.Noop || commit.Changed {
		t.Fatalf("idempotent dump commit = %+v, %v", commit, err)
	}
	link := filepath.Join(directory, "link.yml")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectCMDBDumpDestination(link, candidate); !cmdb.IsCode(err, "CMDB_OUTPUT_INVALID") {
		t.Fatalf("symlink destination error = %v", err)
	}
}

func TestCMDBDumpEquivalenceAndPlansRemainValueFree(t *testing.T) {
	raw := []byte(`{"all":{"children":{"demo":{"hosts":{"10.10.10.10":{"password":"secret"}},"vars":{}}},"vars":{}}}`)
	_, _, err := prepareCMDBDump(raw)
	if err != nil {
		t.Fatalf("dump equivalence = %v", err)
	}
	data := &InventoryCMDBData{
		Kind: InventoryCMDBLoadKind, Operation: "load", SourcePath: "/tmp/inventory.yml",
		ConnectionSource: cmdb.ConnectionArgSource,
		SourceDigest:     "sha256:source", SourceRevision: "sha256:revision", TargetDigest: "sha256:target", Changed: true,
	}
	data.Plan = buildCMDBLoadPlan(data)
	if len(data.Plan.NextActions) != 1 || data.Plan.NextActions[0].Command != "" || !strings.Contains(data.Plan.NextActions[0].Reason, "same explicit -d target") {
		t.Fatalf("explicit database next action = %+v", data.Plan.NextActions)
	}
	data.ConnectionSource = cmdb.ConnectionMetaSource
	if action := buildCMDBLoadPlan(data).NextActions[0]; action.Command == "" {
		t.Fatalf("safe default database next action = %+v", action)
	}
	data.Planned = true
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret", "10.10.10.10", "password", "postgres://"} {
		if bytes.Contains(encoded, []byte(forbidden)) || strings.Contains(data.Text(), forbidden) {
			t.Fatalf("safe plan contains %q: %s", forbidden, encoded)
		}
	}
}

func TestCMDBValueFreeChangesRespectCanonicalNumericEquivalence(t *testing.T) {
	doc, err := inventory.Parse("numeric.yml", []byte("all:\n  vars: { ratio: 1.0 }\n  children: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := cmdb.CanonicalRawConfigJSON([]byte(`{"all":{"children":{},"vars":{"ratio":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	declarations, failure := pigstyDeclarationsFromDocument(doc, doc.Name())
	if failure != nil {
		t.Fatal(failure)
	}
	source, err := cmdb.CanonicalDeclarationsJSON(declarations)
	if err != nil {
		t.Fatal(err)
	}
	if changes := cmdbValueFreeChanges(doc, source, current); len(changes) != 0 {
		t.Fatalf("canonical no-op changes = %+v", changes)
	}
}
