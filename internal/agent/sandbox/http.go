//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// http.go is the sandbox package's small HTTP client. It is
// intentionally separate from `internal/agent/tool/http_helper.go`
// to keep the sandbox package's import graph independent of the
// tool package — the tool package depends on the sandbox package
// (for ManagerClient) and not the other way around.
//
// The retry semantics mirror the tool/http_helper defaults (3
// attempts, 200ms base backoff, 3s cap, 5xx + network errors only).
// 4xx is the caller's error and is not retried.

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

// HTTPClient is a minimal context-aware HTTP client for the sandbox
// providers. It is safe for concurrent use.
type HTTPClient struct {
	client      *http.Client
	timeout     time.Duration
	maxAttempts int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

// HTTPConfig configures an HTTPClient.
type HTTPConfig struct {
	// Timeout is the per-request timeout. Default 30s.
	Timeout time.Duration
	// MaxAttempts is the total number of attempts (including the
	// first). Default 3.
	MaxAttempts int
	// BaseBackoff is the initial retry backoff. Default 200ms.
	BaseBackoff time.Duration
	// MaxBackoff caps the exponential backoff. Default 3s.
	MaxBackoff time.Duration
}

func (c HTTPConfig) withDefaults() HTTPConfig {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 3
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 200 * time.Millisecond
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 3 * time.Second
	}
	return c
}

// NewHTTPClient returns an HTTPClient with the given config (or
// defaults when zero-valued).
func NewHTTPClient(cfg HTTPConfig) *HTTPClient {
	c := cfg.withDefaults()
	return &HTTPClient{
		client:      &http.Client{Timeout: c.Timeout},
		timeout:     c.Timeout,
		maxAttempts: c.MaxAttempts,
		baseBackoff: c.BaseBackoff,
		maxBackoff:  c.MaxBackoff,
	}
}

// Do issues a request and returns the response. body and contentType
// may be empty. body non-empty with empty contentType is sent as
// application/octet-stream.
//
// Retry policy:
//   - 5xx: retried
//   - network errors: retried
//   - 4xx: NOT retried
//   - 2xx/3xx: returned as-is
//
// The context is honored on every attempt; cancellation aborts the
// loop.
func (h *HTTPClient) Do(
	ctx context.Context,
	method, url, body, contentType string,
	headers map[string]string,
) (*http.Response, error) {
	if method == "" {
		method = http.MethodGet
	}
	if body != "" && contentType == "" {
		contentType = "application/octet-stream"
	}

	var lastErr error
	for attempt := 1; attempt <= h.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader([]byte(body)))
		if err != nil {
			return nil, fmt.Errorf("sandbox http: build request: %w", err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableNetError(err) {
				return nil, err
			}
			if attempt == h.maxAttempts {
				return nil, fmt.Errorf("sandbox http: %s %s failed after %d attempts: %w", method, url, attempt, err)
			}
			sleepCtx(ctx, backoff(h.baseBackoff, h.maxBackoff, attempt))
			continue
		}

		// 5xx is retryable, 4xx is not.
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("sandbox http: %s %s returned %d", method, url, resp.StatusCode)
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if attempt == h.maxAttempts {
				return nil, lastErr
			}
			sleepCtx(ctx, backoff(h.baseBackoff, h.maxBackoff, attempt))
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("sandbox http: exhausted retries with no recorded error")
}

// backoff returns an exponentially increasing duration with full jitter,
// capped at max. attempt is 1-indexed; the first retry uses base.
func backoff(base, max time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > max {
			d = max
			break
		}
	}
	return time.Duration(rand.Int64N(int64(d) + 1))
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func isRetryableNetError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}
