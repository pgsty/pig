package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultMirrorFetchTimeout is the default wall-clock timeout used by
// mirror fetchers when callers don't provide an explicit deadline.
const DefaultMirrorFetchTimeout = 30 * time.Second

type mirrorFetchResult struct {
	url     string
	content []byte
	err     error
}

type MirrorSyncStage string

const (
	MirrorSyncFetch MirrorSyncStage = "fetch"
	MirrorSyncWrite MirrorSyncStage = "write"
)

type MirrorSyncResult struct {
	SourceURL string
}

// MirrorSyncError identifies whether mirror synchronization failed while
// fetching/validating content or while atomically replacing the destination.
type MirrorSyncError struct {
	Stage MirrorSyncStage
	Err   error
}

func (e *MirrorSyncError) Error() string { return e.Err.Error() }
func (e *MirrorSyncError) Unwrap() error { return e.Err }

func shouldFallbackToGetOnHeadStatus(code int) bool {
	// Some servers may not implement HEAD correctly. In these cases, still try GET.
	// 405: Method Not Allowed, 403: Forbidden (rare, but sometimes HEAD is blocked while GET works).
	return code == http.StatusMethodNotAllowed || code == http.StatusForbidden
}

// FetchFirstValid concurrently fetches content from all given URLs and returns the
// first response that:
//  1. returns HTTP 200
//  2. can be fully read
//  3. passes validate (when validate is not nil)
//
// It is intentionally quiet: it does not log per-mirror failures. Callers should
// only surface an error when all mirrors fail (or the context times out).
func FetchFirstValid(ctx context.Context, urls []string, validate func([]byte) error) ([]byte, string, error) {
	if len(urls) == 0 {
		return nil, "", fmt.Errorf("no urls provided")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan mirrorFetchResult, len(urls))
	client := DefaultClient()
	for _, u := range urls {
		u := u
		go func() {
			// 1) HEAD probe: check existence / fast-fail without downloading full body.
			headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
			if err != nil {
				results <- mirrorFetchResult{url: u, err: err}
				return
			}
			headResp, err := client.Do(headReq)
			if headResp != nil && headResp.Body != nil {
				_ = headResp.Body.Close()
			}
			if err != nil {
				results <- mirrorFetchResult{url: u, err: err}
				return
			}

			// Fail fast on HEAD non-2xx unless it's a known case where GET may still work.
			if headResp != nil && headResp.StatusCode/100 != 2 && !shouldFallbackToGetOnHeadStatus(headResp.StatusCode) {
				results <- mirrorFetchResult{url: u, err: fmt.Errorf("bad status: %s", headResp.Status)}
				return
			}

			// 2) GET content
			getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				results <- mirrorFetchResult{url: u, err: err}
				return
			}
			getResp, err := client.Do(getReq)
			if err != nil {
				results <- mirrorFetchResult{url: u, err: err}
				return
			}
			defer getResp.Body.Close()

			if getResp.StatusCode != http.StatusOK {
				results <- mirrorFetchResult{url: u, err: fmt.Errorf("bad status: %s", getResp.Status)}
				return
			}

			content, err := io.ReadAll(getResp.Body)
			if err != nil {
				results <- mirrorFetchResult{url: u, err: err}
				return
			}
			results <- mirrorFetchResult{url: u, content: content}
		}()
	}

	var errs []error
	for i := 0; i < len(urls); i++ {
		select {
		case res := <-results:
			if res.err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", res.url, res.err))
				continue
			}
			if validate != nil {
				if err := validate(res.content); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", res.url, err))
					continue
				}
			}
			// Stop other in-flight requests as soon as we have a valid response.
			cancel()
			return res.content, res.url, nil
		case <-ctx.Done():
			// Deadline/cancellation: still considered a failure of all mirrors in this call.
			if len(errs) == 0 {
				return nil, "", ctx.Err()
			}
			return nil, "", errors.Join(append(errs, ctx.Err())...)
		}
	}

	if len(errs) == 0 {
		return nil, "", fmt.Errorf("all mirrors failed")
	}
	return nil, "", errors.Join(errs...)
}

// FetchFirstValidWithTimeout is a convenience wrapper around FetchFirstValid.
// It creates a context with timeout and guarantees that mirror probing cannot
// block forever when callers do not already have a deadline.
func FetchFirstValidWithTimeout(timeout time.Duration, urls []string, validate func([]byte) error) ([]byte, string, error) {
	if timeout <= 0 {
		timeout = DefaultMirrorFetchTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return FetchFirstValid(ctx, urls, validate)
}

// SyncFirstValidFile fetches the first valid mirror response and atomically
// replaces dstPath. A failed fetch or write leaves the previous file untouched.
// Existing files retain their permission mode; mode is the default for new files.
func SyncFirstValidFile(timeout time.Duration, urls []string, dstPath string, mode os.FileMode, validate func([]byte) error) (MirrorSyncResult, error) {
	var result MirrorSyncResult
	content, sourceURL, err := FetchFirstValidWithTimeout(timeout, urls, validate)
	if err != nil {
		return result, &MirrorSyncError{Stage: MirrorSyncFetch, Err: err}
	}
	result.SourceURL = sourceURL
	writeMode, err := existingFileMode(dstPath, mode)
	if err != nil {
		return result, &MirrorSyncError{Stage: MirrorSyncWrite, Err: err}
	}
	if err := AtomicWriteFile(dstPath, content, writeMode); err != nil {
		return result, &MirrorSyncError{Stage: MirrorSyncWrite, Err: err}
	}
	return result, nil
}

// IsTimeoutError reports timeout errors through wrapped network and context errors.
func IsTimeoutError(err error) bool {
	var netErr interface{ Timeout() bool }
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}

// MirrorAttemptDetail formats the common diagnostics used by catalog reloads.
func MirrorAttemptDetail(urls []string, err error) string {
	attempted := strings.Join(urls, "\n- ")
	if attempted != "" {
		attempted = "- " + attempted
	}
	errText := ""
	if err != nil {
		errText = strings.TrimSpace(err.Error())
	}
	return fmt.Sprintf("attempted:\n%s\nerrors:\n%s", attempted, errText)
}
