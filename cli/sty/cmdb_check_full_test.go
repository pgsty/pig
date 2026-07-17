package sty

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

type fakeCMDBFullCheckReader struct {
	state    cmdb.VerifiedState
	snapshot *inventory.Snapshot
	token    cmdb.RevisionToken
	source   cmdb.ConnectionSource
	err      error
}

func (f fakeCMDBFullCheckReader) Verify(context.Context) (cmdb.VerifiedState, error) {
	return f.state, f.err
}

func (f fakeCMDBFullCheckReader) Source() cmdb.ConnectionSource { return f.source }

func (f fakeCMDBFullCheckReader) ReadHeadSnapshot(context.Context) (*inventory.Snapshot, cmdb.RevisionToken, error) {
	return f.snapshot, f.token, f.err
}

func TestCMDBCheckFullStaticParityAndSecretHygiene(t *testing.T) {
	fixture := newCMDBFullCheckFixture(t)
	reader := fakeCMDBFullCheckReader{
		state:    cmdb.VerifiedState{MigrationVersion: 1, RevisionCount: 1, Head: &fixture.token},
		snapshot: fixture.snapshot, token: fixture.token, source: cmdb.ConnectionMetaSource,
	}
	result := cmdbCheckFull(t.Context(), reader, fixture.options())
	if result == nil || !result.Success {
		t.Fatalf("healthy full check = %+v", result)
	}
	data, ok := result.Data.(*CMDBCheckData)
	if !ok || !data.Healthy || data.EffectiveSource.Kind != CMDBSourceStatic ||
		data.PersistentSource.Kind != CMDBSourceStatic || data.SwitchState != cmdb.SourceSwitchNone ||
		!hasCMDBFinding(data.Findings, "CMDB_STATIC_PARITY_OK", "pass") {
		t.Fatalf("healthy check data = %+v", result.Data)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fixture.secret) {
		t.Fatalf("CMDB check leaked Inventory value: %s", encoded)
	}

	drifted := strings.Replace(string(fixture.staticBody), "generation: 1", "generation: 2", 1)
	if err := os.WriteFile(fixture.staticPath, []byte(drifted), 0o600); err != nil {
		t.Fatal(err)
	}
	result = cmdbCheckFull(t.Context(), reader, fixture.options())
	data = result.Data.(*CMDBCheckData)
	if result.Success || result.Code != output.CodeStyCMDBDrift || data.Healthy ||
		!hasCMDBFinding(data.Findings, "CMDB_SOURCE_HEAD_MISMATCH", "error") {
		t.Fatalf("drifted full check = %+v", result)
	}
}

func TestCMDBCheckFullNativeRequiresSwitchRecordAndCurrentAdapter(t *testing.T) {
	fixture := newCMDBFullCheckFixture(t)
	if err := os.WriteFile(fixture.configPath, []byte("[defaults]\ninventory = "+fixture.runtime.AdapterPath+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := fakeCMDBFullCheckReader{
		state:    cmdb.VerifiedState{MigrationVersion: 1, RevisionCount: 1, Head: &fixture.token},
		snapshot: fixture.snapshot, token: fixture.token, source: cmdb.ConnectionMetaSource,
	}
	result := cmdbCheckFull(t.Context(), reader, fixture.options())
	data := result.Data.(*CMDBCheckData)
	if result.Success || !hasCMDBFinding(data.Findings, "CMDB_ADAPTER_PARITY_OK", "pass") ||
		!hasCMDBFinding(data.Findings, "CMDB_SWITCH_RECORD_MISSING", "error") {
		t.Fatalf("recordless native check = %+v", result)
	}

	if err := os.WriteFile(fixture.runtime.AdapterPath, []byte("#!/bin/sh\n# altered\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result = cmdbCheckFull(t.Context(), reader, fixture.options())
	data = result.Data.(*CMDBCheckData)
	if result.Success || !hasCMDBFinding(data.Findings, "CMDB_RUNTIME_ADAPTER_INVALID", "error") ||
		data.AdapterIdentity != "" {
		t.Fatalf("altered adapter check = %+v", result)
	}
}

type cmdbFullCheckFixture struct {
	root        string
	home        string
	configPath  string
	staticPath  string
	staticBody  []byte
	secret      string
	pig         string
	runtimeRoot string
	runtimePath string
	runtime     cmdb.RuntimeRecord
	snapshot    *inventory.Snapshot
	token       cmdb.RevisionToken
}

func newCMDBFullCheckFixture(t *testing.T) cmdbFullCheckFixture {
	t.Helper()
	root := symlinkFreeStyTempDir(t)
	home := filepath.Join(root, "pigsty")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	pig := filepath.Join(root, "pig")
	writeCMDBSourceFile(t, pig, "#!/bin/sh\nexit 0\n", 0o700)
	runtimeRoot := filepath.Join(root, "runtime")
	runtime, err := cmdb.PrepareRuntime(cmdb.RuntimeOptions{
		PigstyHome: home, RuntimeRoot: runtimeRoot, PigBinary: pig,
		ConnectionSource: cmdb.ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	secret := "check-private-password"
	staticBody := []byte("all:\n  children: {}\n  vars:\n    admin_ip: 127.0.0.1\n    version: v4.4.0\n    region: default\n    generation: 1\n    password: " + secret + "\n")
	staticPath := filepath.Join(root, "static.yml")
	writeCMDBSourceFile(t, staticPath, string(staticBody), 0o600)
	configPath := filepath.Join(home, "ansible.cfg")
	writeCMDBSourceFile(t, configPath, "[defaults]\ninventory = ../static.yml\n", 0o600)
	document, err := inventory.Parse(staticPath, staticBody)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	token, err := cmdb.ParseRevisionToken("cmdb-v1:1:sha256:" + snapshot.Digest())
	if err != nil {
		t.Fatal(err)
	}
	return cmdbFullCheckFixture{
		root: root, home: home, configPath: configPath, staticPath: staticPath,
		staticBody: staticBody, secret: secret, pig: pig, runtimeRoot: runtimeRoot,
		runtimePath: filepath.Join(filepath.Dir(runtime.AdapterPath), "runtime.json"),
		runtime:     runtime, snapshot: snapshot, token: token,
	}
}

func (f cmdbFullCheckFixture) options() CMDBCheckOptions {
	return CMDBCheckOptions{
		PigstyHome: f.home, WorkingDirectory: f.root, UserHome: f.root,
		UserConfigPath: filepath.Join(f.root, "missing-config.yml"),
		RuntimeRoot:    f.runtimeRoot, RuntimePath: f.runtimePath, PigBinary: f.pig,
		LookupEnv: func(string) (string, bool) { return "", false },
	}
}

func hasCMDBFinding(findings []CMDBFinding, code, status string) bool {
	for _, finding := range findings {
		if finding.Code == code && finding.Status == status {
			return true
		}
	}
	return false
}
