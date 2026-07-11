package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type timeoutTestError struct{}

func (timeoutTestError) Error() string { return "timed out" }
func (timeoutTestError) Timeout() bool { return true }

func TestFetchFirstValid_SucceedsWhenOneMirrorFails(t *testing.T) {
	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srvBad.Close)

	okBody := []byte("ok")
	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(okBody)
	}))
	t.Cleanup(srvOK.Close)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	body, url, err := FetchFirstValid(ctx, []string{srvBad.URL, srvOK.URL}, nil)
	if err != nil {
		t.Fatalf("FetchFirstValid() error = %v", err)
	}
	if string(body) != string(okBody) {
		t.Fatalf("unexpected body: got %q want %q", string(body), string(okBody))
	}
	if url != srvOK.URL {
		t.Fatalf("unexpected url: got %q want %q", url, srvOK.URL)
	}
}

func TestFetchFirstValid_ValidationFallback(t *testing.T) {
	srvInvalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad"))
	}))
	t.Cleanup(srvInvalid.Close)

	srvValid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("good"))
	}))
	t.Cleanup(srvValid.Close)

	validate := func(b []byte) error {
		if string(b) != "good" {
			return errors.New("invalid content")
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	body, url, err := FetchFirstValid(ctx, []string{srvInvalid.URL, srvValid.URL}, validate)
	if err != nil {
		t.Fatalf("FetchFirstValid() error = %v", err)
	}
	if string(body) != "good" {
		t.Fatalf("unexpected body: got %q want %q", string(body), "good")
	}
	if url != srvValid.URL {
		t.Fatalf("unexpected url: got %q want %q", url, srvValid.URL)
	}
}

func TestFetchFirstValid_AllFail(t *testing.T) {
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srvA.Close)
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srvB.Close)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	body, url, err := FetchFirstValid(ctx, []string{srvA.URL, srvB.URL}, nil)
	if err == nil {
		t.Fatalf("expected error, got nil (body=%q url=%q)", string(body), url)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body on error, got %q", string(body))
	}
	if url != "" {
		t.Fatalf("expected empty url on error, got %q", url)
	}
}

func TestFetchFirstValidWithTimeout_DeadlineExceeded(t *testing.T) {
	srvSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow"))
	}))
	t.Cleanup(srvSlow.Close)

	body, url, err := FetchFirstValidWithTimeout(20*time.Millisecond, []string{srvSlow.URL}, nil)
	if err == nil {
		t.Fatalf("expected timeout error, got nil (body=%q url=%q)", string(body), url)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "wrapped deadline exceeded", err: fmt.Errorf("fetch failed: %w", context.DeadlineExceeded), want: true},
		{name: "timeout interface", err: timeoutTestError{}, want: true},
		{name: "ordinary error", err: errors.New("connection refused"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTimeoutError(tt.err); got != tt.want {
				t.Fatalf("IsTimeoutError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSyncFirstValidFileValidatesAndPreservesExistingMode(t *testing.T) {
	srvInvalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid"))
	}))
	t.Cleanup(srvInvalid.Close)
	srvValid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("valid"))
	}))
	t.Cleanup(srvValid.Close)

	dst := filepath.Join(t.TempDir(), "catalog")
	if err := os.WriteFile(dst, []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0644); err != nil {
		t.Fatal(err)
	}
	validate := func(content []byte) error {
		if string(content) != "valid" {
			return errors.New("invalid catalog")
		}
		return nil
	}
	result, err := SyncFirstValidFile(time.Second, []string{srvInvalid.URL, srvValid.URL}, dst, 0600, validate)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceURL != srvValid.URL {
		t.Fatalf("source url = %q, want %q", result.SourceURL, srvValid.URL)
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "valid" {
		t.Fatalf("content = %q", content)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("mode = %04o, want preserved 0644", info.Mode().Perm())
	}
}

func TestSyncFirstValidFileUsesDefaultModeForNewFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("valid"))
	}))
	t.Cleanup(srv.Close)

	dst := filepath.Join(t.TempDir(), "new-catalog")
	if _, err := SyncFirstValidFile(time.Second, []string{srv.URL}, dst, 0600, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %04o, want default 0600", info.Mode().Perm())
	}
}

func TestSyncFirstValidFileReportsWriteStage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("valid"))
	}))
	t.Cleanup(srv.Close)

	dst := t.TempDir() // Renaming a file over a directory must fail.
	result, err := SyncFirstValidFile(time.Second, []string{srv.URL}, dst, 0644, nil)
	if err == nil {
		t.Fatal("expected write error")
	}
	if result.SourceURL != srv.URL {
		t.Fatalf("source url = %q, want %q", result.SourceURL, srv.URL)
	}
	var syncErr *MirrorSyncError
	if !errors.As(err, &syncErr) || syncErr.Stage != MirrorSyncWrite {
		t.Fatalf("error = %v, want MirrorSyncWrite", err)
	}
}
