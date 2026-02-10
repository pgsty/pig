package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type mirrorFetchResult struct {
	url     string
	content []byte
	err     error
}

func shouldFallbackToGetOnHeadStatus(code int) bool {
	// Some servers may not implement HEAD correctly. In these cases, still try GET.
	// 405: Method Not Allowed, 403: Forbidden (rare, but sometimes HEAD is blocked while GET works).
	return code == http.StatusMethodNotAllowed || code == http.StatusForbidden
}

// FetchFirstValid concurrently fetches content from all given URLs and returns the
// first response that:
//   1) returns HTTP 200
//   2) can be fully read
//   3) passes validate (when validate is not nil)
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
	client := defaultClient()
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
