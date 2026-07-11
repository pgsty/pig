package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"
)

func TestDownloadFile_KnownContentLength(t *testing.T) {
	content := []byte("hello world\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	t.Cleanup(srv.Close)

	dst := filepath.Join(t.TempDir(), "file.txt")
	if err := DownloadFile(srv.URL+"/file.txt", dst); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", string(data), string(content))
	}
}

func TestDownloadFile_UnknownContentLength(t *testing.T) {
	content := []byte("chunked-body-no-content-length")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write(content)
	}))
	t.Cleanup(srv.Close)

	dst := filepath.Join(t.TempDir(), "file.dat")
	if err := DownloadFile(srv.URL+"/file.dat", dst); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", string(data), string(content))
	}
}

func TestDownloadFile_ReplacesSameLengthCorruptFile(t *testing.T) {
	content := []byte("correct")
	corrupt := []byte("corrupt")
	var getCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			atomic.AddInt32(&getCalls, 1)
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "replace.txt")
	if err := os.WriteFile(dst, corrupt, 0644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadFile(srv.URL+"/replace.txt", dst); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}

	if atomic.LoadInt32(&getCalls) != 1 {
		t.Fatalf("expected 1 GET, got %d", atomic.LoadInt32(&getCalls))
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestDownloadFile_PreservesExistingFileOnFailure(t *testing.T) {
	readErr := errors.New("injected read failure")
	tests := []struct {
		name      string
		transport http.RoundTripper
		wantErr   string
	}{
		{
			name: "transport",
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("injected transport failure")
			}),
			wantErr: "injected transport failure",
		},
		{
			name: "http status",
			transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return downloadResponse(req, http.StatusServiceUnavailable, nil, 0), nil
			}),
			wantErr: "bad http status: 503",
		},
		{
			name: "body read",
			transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := io.MultiReader(bytes.NewReader([]byte("partial")), iotest.ErrReader(readErr))
				return downloadResponse(req, http.StatusOK, body, -1), nil
			}),
			wantErr: readErr.Error(),
		},
		{
			name: "size mismatch",
			transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := []byte("short")
				return downloadResponse(req, http.StatusOK, bytes.NewReader(body), int64(len(body)+1)), nil
			}),
			wantErr: "size mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useDownloadTransport(t, tt.transport)
			dst := filepath.Join(t.TempDir(), "artifact")
			if err := os.WriteFile(dst, []byte("previous-content"), 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dst, 0750); err != nil {
				t.Fatal(err)
			}

			err := DownloadFile("https://download.invalid/artifact", dst)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("DownloadFile() error = %v, want containing %q", err, tt.wantErr)
			}
			assertFileContentAndMode(t, dst, "previous-content", 0750)
			assertNoDownloadTemps(t, dst)
		})
	}
}

func TestDownloadFile_RejectsSymlinkDestination(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	dst := filepath.Join(dir, "artifact")
	if err := os.WriteFile(target, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dst); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	content := []byte("new-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	}))
	t.Cleanup(srv.Close)

	err := DownloadFile(srv.URL+"/artifact", dst)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("DownloadFile() error = %v, want symlink rejection", err)
	}
	if got, err := os.Readlink(dst); err != nil || got != target {
		t.Fatalf("destination symlink changed: target=%q err=%v", got, err)
	}
	assertFileContentAndMode(t, target, "sentinel", 0600)
	assertNoDownloadTemps(t, dst)
}

func TestDownloadFile_DoesNotUseLegacyFixedTempPath(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact")
	legacyTemp := dst + ".tmp"
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, legacyTemp); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	content := []byte("downloaded")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	}))
	t.Cleanup(srv.Close)

	if err := DownloadFile(srv.URL+"/artifact", dst); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	assertFileContentAndMode(t, dst, string(content), 0644)
	assertFileContentAndMode(t, victim, "sentinel", 0600)
	if got, err := os.Readlink(legacyTemp); err != nil || got != victim {
		t.Fatalf("legacy temp symlink changed: target=%q err=%v", got, err)
	}
}

func TestDownloadFile_PreservesModeAndCleansTemporaryFiles(t *testing.T) {
	content := []byte("replacement")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	}))
	t.Cleanup(srv.Close)

	t.Run("existing mode", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "artifact")
		if err := os.WriteFile(dst, []byte("old"), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dst, 0750); err != nil {
			t.Fatal(err)
		}
		if err := DownloadFile(srv.URL+"/artifact", dst); err != nil {
			t.Fatalf("DownloadFile() error = %v", err)
		}
		assertFileContentAndMode(t, dst, string(content), 0750)
		assertNoDownloadTemps(t, dst)
	})

	t.Run("new file mode", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "artifact")
		if err := DownloadFile(srv.URL+"/artifact", dst); err != nil {
			t.Fatalf("DownloadFile() error = %v", err)
		}
		assertFileContentAndMode(t, dst, string(content), 0644)
		assertNoDownloadTemps(t, dst)
	})
}

func TestDownloadFile_ConcurrentDownloadsUseUniqueTemps(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact")
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	payloads := [][]byte{[]byte("payload-a"), []byte("payload-b")}
	var getCalls int32

	useDownloadTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead {
			return downloadResponse(req, http.StatusMethodNotAllowed, nil, 0), nil
		}
		idx := int(atomic.AddInt32(&getCalls, 1)) - 1
		if idx >= len(payloads) {
			return nil, fmt.Errorf("unexpected GET %d", idx+1)
		}
		body := &gatedDownloadBody{
			data:    append([]byte(nil), payloads[idx]...),
			started: started,
			release: release,
		}
		return downloadResponse(req, http.StatusOK, body, int64(len(payloads[idx]))), nil
	}))

	for i := 0; i < 2; i++ {
		go func() {
			results <- DownloadFile("https://download.invalid/artifact", dst)
		}()
	}

	startupTimer := time.NewTimer(5 * time.Second)
	defer startupTimer.Stop()
	startedCount := 0
	var startupErr error
	earlyResults := make([]error, 0, 2)
waitForBodies:
	for startedCount < 2 {
		select {
		case <-started:
			startedCount++
		case err := <-results:
			earlyResults = append(earlyResults, err)
			startupErr = fmt.Errorf("download returned before both bodies started: %v", err)
			break waitForBodies
		case <-startupTimer.C:
			startupErr = fmt.Errorf("timed out waiting for concurrent body reads")
			break waitForBodies
		}
	}

	var tempMatches []string
	var globErr error
	if startupErr == nil {
		tempMatches, globErr = filepath.Glob(filepath.Join(dir, ".artifact.tmp-*"))
	}
	close(release)

	drainTimer := time.NewTimer(5 * time.Second)
	defer drainTimer.Stop()
	for len(earlyResults) < 2 {
		select {
		case err := <-results:
			earlyResults = append(earlyResults, err)
		case <-drainTimer.C:
			t.Fatal("timed out waiting for concurrent downloads to finish")
		}
	}
	if startupErr != nil {
		t.Fatal(startupErr)
	}
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(tempMatches) != 2 {
		t.Errorf("concurrent downloads exposed %d unique temps, want 2: %v", len(tempMatches), tempMatches)
	}
	for i, err := range earlyResults {
		if err != nil {
			t.Errorf("DownloadFile() call %d error = %v", i+1, err)
		}
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payloads[0]) && !bytes.Equal(got, payloads[1]) {
		t.Fatalf("final content is not a complete payload: %q", got)
	}
	assertNoDownloadTemps(t, dst)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func useDownloadTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	original := defaultHTTPClient
	defaultHTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() { defaultHTTPClient = original })
}

func downloadResponse(req *http.Request, status int, body io.Reader, contentLength int64) *http.Response {
	if body == nil {
		body = http.NoBody
	}
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		Body:          io.NopCloser(body),
		ContentLength: contentLength,
		Request:       req,
	}
}

type gatedDownloadBody struct {
	data        []byte
	started     chan<- struct{}
	release     <-chan struct{}
	readStarted bool
}

func (b *gatedDownloadBody) Read(p []byte) (int, error) {
	if !b.readStarted {
		b.readStarted = true
		b.started <- struct{}{}
		<-b.release
	}
	if len(b.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, nil
}

func (b *gatedDownloadBody) Close() error { return nil }

func assertFileContentAndMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := info.Mode().Perm(); gotMode != mode.Perm() {
		t.Fatalf("mode = %04o, want %04o", gotMode, mode.Perm())
	}
}

func assertNoDownloadTemps(t *testing.T, dst string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
	if _, err := os.Lstat(dst + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy temporary path remains: %v", err)
	}
}
