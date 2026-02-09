package utils

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestDownloadFile_HeadAndGet(t *testing.T) {
	content := []byte("hello world\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

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

func TestDownloadFile_HeadNotAllowedStillWorks(t *testing.T) {
	content := []byte("abc")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "file.bin")
	if err := DownloadFile(srv.URL+"/file.bin", dst); err != nil {
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
		switch r.Method {
		case http.MethodHead:
			// Intentionally omit Content-Length.
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			// Intentionally omit Content-Length to force chunked encoding.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

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

func TestDownloadFile_SkipWhenSameSize(t *testing.T) {
	content := []byte("same-size-skip")
	var headCalls int32
	var getCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			atomic.AddInt32(&headCalls, 1)
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

	dst := filepath.Join(t.TempDir(), "skip.txt")
	if err := DownloadFile(srv.URL+"/skip.txt", dst); err != nil {
		t.Fatalf("first DownloadFile() error = %v", err)
	}
	if err := DownloadFile(srv.URL+"/skip.txt", dst); err != nil {
		t.Fatalf("second DownloadFile() error = %v", err)
	}

	if atomic.LoadInt32(&getCalls) != 1 {
		t.Fatalf("expected 1 GET, got %d", atomic.LoadInt32(&getCalls))
	}
	if atomic.LoadInt32(&headCalls) < 2 {
		t.Fatalf("expected at least 2 HEAD calls, got %d", atomic.LoadInt32(&headCalls))
	}
}
