package sty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
)

func TestCMDBSplitBrainNoticeExactDirections(t *testing.T) {
	staticInvocation := CMDBSourceDescriptor{Kind: CMDBSourceStatic, Origin: CMDBSourceCLI}
	nativeInvocation := CMDBSourceDescriptor{Kind: CMDBSourceNative, Origin: CMDBSourceEnvironment}
	staticPersistent := CMDBSourceDescriptor{Kind: CMDBSourceStatic, Origin: CMDBSourceAnsibleConfig, Persistent: true}
	nativePersistent := CMDBSourceDescriptor{Kind: CMDBSourceNative, Origin: CMDBSourceAnsibleConfig, Persistent: true}

	if got := CMDBSplitBrainNotice(staticInvocation, nativePersistent); got != CMDBStaticShadowsPersistentNotice {
		t.Fatalf("static split-brain notice = %q", got)
	}
	if got := CMDBSplitBrainNotice(nativeInvocation, staticPersistent); got != CMDBNativeShadowsPersistentNotice {
		t.Fatalf("native split-brain notice = %q", got)
	}
	for _, pair := range [][2]CMDBSourceDescriptor{
		{staticInvocation, staticPersistent},
		{nativeInvocation, nativePersistent},
		{staticPersistent, nativePersistent},
		{staticInvocation, CMDBSourceDescriptor{Kind: CMDBSourceNative}},
	} {
		if got := CMDBSplitBrainNotice(pair[0], pair[1]); got != "" {
			t.Fatalf("unexpected split-brain notice = %q for %+v", got, pair)
		}
	}
}

func TestCMDBNoticeEmitterWritesStderrOnceAndKeepsStructuredOutputValid(t *testing.T) {
	invocation := CMDBSourceDescriptor{Kind: CMDBSourceStatic, Origin: CMDBSourceCLI}
	persistent := CMDBSourceDescriptor{Kind: CMDBSourceNative, Origin: CMDBSourceAnsibleConfig, Persistent: true}
	var stderr bytes.Buffer
	emitter := NewCMDBNoticeEmitter(&stderr)
	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := emitter.Emit(invocation, persistent); err != nil {
				t.Errorf("emit notice: %v", err)
			}
		}()
	}
	wait.Wait()
	want := CMDBStaticShadowsPersistentNotice + "\n"
	if stderr.String() != want {
		t.Fatalf("stderr notice = %q, want %q", stderr.String(), want)
	}

	stdout, err := output.OK("checked", &CMDBCheckData{Kind: CMDBCheckKind, Healthy: true, Findings: []CMDBFinding{}}).JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(stdout) || bytes.Contains(stdout, []byte("NOTICE:")) {
		t.Fatalf("structured stdout was contaminated: %s", stdout)
	}
}

func TestCMDBCheckNativeFailureIsRedacted(t *testing.T) {
	result := CMDBCheckNative(t.Context(), nil)
	if result.Success || result.Code != output.CodeStyCMDBConfigInvalid {
		t.Fatalf("nil native store check = %v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("DSN")) || bytes.Contains(encoded, []byte("SELECT")) ||
		!bytes.Contains(encoded, []byte(CMDBCheckKind)) || !bytes.Contains(encoded, []byte("CMDB_CONNECTION_INVALID")) {
		t.Fatalf("native check failure contract = %s", encoded)
	}
}

func TestCMDBCheckNativeReportsExactHeadAndEmptyState(t *testing.T) {
	token, err := cmdb.ParseRevisionToken("cmdb-v1:2:sha256:" + string(bytes.Repeat([]byte("a"), 64)))
	if err != nil {
		t.Fatal(err)
	}
	healthy := cmdbCheckNative(t.Context(), fakeCMDBVerifier{
		source: cmdb.ConnectionEnvSource,
		state:  cmdb.VerifiedState{MigrationVersion: 1, RevisionCount: 2, Head: &token},
	})
	data, ok := healthy.Data.(*CMDBCheckData)
	if !healthy.Success || !ok || !data.Healthy || data.Head == nil || !data.Head.Equal(token) ||
		data.ConnectionSource != cmdb.ConnectionEnvSource || len(data.Findings) != 3 {
		t.Fatalf("healthy native check = %v", healthy)
	}

	empty := cmdbCheckNative(t.Context(), fakeCMDBVerifier{
		source: cmdb.ConnectionMetaSource,
		state:  cmdb.VerifiedState{MigrationVersion: 1},
	})
	emptyData, ok := empty.Data.(*CMDBCheckData)
	if empty.Success || empty.Code != output.CodeStyCMDBResourceMissing || !ok || emptyData.Healthy ||
		len(emptyData.Findings) != 4 || emptyData.Findings[3].Code != "CMDB_HEAD_EMPTY" {
		t.Fatalf("empty native check = %v", empty)
	}

	failure := cmdbCheckNative(t.Context(), fakeCMDBVerifier{err: errors.New("private driver detail")})
	encoded, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private driver detail")) || !bytes.Contains(encoded, []byte("CMDB_INTERNAL")) {
		t.Fatalf("unknown verifier failure was not redacted: %s", encoded)
	}
}

type fakeCMDBVerifier struct {
	source cmdb.ConnectionSource
	state  cmdb.VerifiedState
	err    error
}

func (f fakeCMDBVerifier) Verify(context.Context) (cmdb.VerifiedState, error) {
	return f.state, f.err
}

func (f fakeCMDBVerifier) Source() cmdb.ConnectionSource { return f.source }
