package utils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

