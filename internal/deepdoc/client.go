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

// Package deepdoc — Go client for the optional deepdoc vision service
// (DLA / OCR / TSR).
//
// Wire contract reconstructed from `deepdoc/vision/dla_cli.py` (fork)
// and the Phase 0 research deliverable
// `docs/agent-port/deepdoc-endpoints.md`. Only DLA has a remote HTTP
// endpoint; OCR and TSR are 100% local ONNX in Python and stubbed
// here as ErrNoRemoteEndpoint.
package deepdoc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ErrNoURL is returned by methods when the client was constructed
// without a base URL (DEEPDOC_URL / TENSORRT_DLA_SVR unset).
var ErrNoURL = errors.New("deepdoc: not configured (set DEEPDOC_URL or TENSORRT_DLA_SVR)")

// ErrNoRemoteEndpoint is returned by OCR/TSR because the Python
// deepdoc service exposes no remote endpoint for those — they're
// local ONNX only (deepdoc/vision/ocr.py:542, table_structure_recognizer.py:30).
var ErrNoRemoteEndpoint = errors.New("deepdoc: no remote endpoint exists (Python deepdoc is local-ONNX only)")

// ErrInvalidResponse is returned when the server returns a payload
// that doesn't validate (e.g. DLA response missing "bboxes" key).
// Per the Python contract, this triggers the retry loop.
var ErrInvalidResponse = errors.New("deepdoc: invalid response")

// DefaultPerAttemptTimeout matches Python's @timeout(18) decorator
// on DLAClient.predict (deepdoc/vision/dla_cli.py:23).
const DefaultPerAttemptTimeout = 18 * time.Second

// DefaultMaxAttempts matches Python's `for _ in range(3)` retry loop.
const DefaultMaxAttempts = 3

// DefaultBackoff is the initial backoff between retries; doubled
// each attempt, capped at MaxBackoff.
const DefaultBackoff = 200 * time.Millisecond

// MaxBackoff caps the exponential backoff between retries.
const MaxBackoff = 3 * time.Second

// predictPath is the DLA endpoint per
// docs/agent-port/deepdoc-endpoints.md §2.1.
const predictPath = "/predict"

// Client talks to an optional deepdoc service. When baseURL is empty,
// Enabled() reports false and HTTP methods return ErrNoURL without
// making any network call.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	maxAttempts int
	backoff     time.Duration
}

// Option mutates a Client at construction time. Used by tests to
// point at httptest servers, override timeouts, etc.
type Option func(*Client)

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithMaxAttempts overrides the per-call retry count (default 3).
func WithMaxAttempts(n int) Option {
	return func(c *Client) { c.maxAttempts = n }
}

// WithBackoff overrides the initial backoff between retries.
func WithBackoff(d time.Duration) Option {
	return func(c *Client) { c.backoff = d }
}

// NewClient returns a Client configured from the environment. The
// base URL is read from DEEPDOC_URL (preferred) or TENSORRT_DLA_SVR
// (legacy alias per deepdoc/vision/layout_recognizer.py:52). When
// both are unset, Enabled() reports false.
func NewClient(opts ...Option) *Client {
	url := os.Getenv("DEEPDOC_URL")
	if url == "" {
		url = os.Getenv("TENSORRT_DLA_SVR")
	}
	return NewClientWithURL(url, opts...)
}

// NewClientWithURL is NewClient with an explicit base URL. Primarily
// used by tests pointing at httptest servers.
func NewClientWithURL(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:     baseURL,
		maxAttempts: DefaultMaxAttempts,
		backoff:     DefaultBackoff,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		// otelhttp.NewTransport is a no-op when no OTel exporter is
		// configured (see plan §2.10.4) — safe default.
		c.httpClient = &http.Client{
			Timeout:   DefaultPerAttemptTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}
	}
	return c
}

// Enabled reports whether a remote deepdoc URL is configured. When
// false, HTTP methods return ErrNoURL immediately.
func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != ""
}

// bodyBuilder is a factory that produces a fresh request body per
// retry attempt. Returns (body, contentType). The body is consumed
// by http.Client.Do and must be readable (multipart.Writer writes
// into a buffer so this is straightforward).
type bodyBuilder func() (io.Reader, string)

// doPost issues a POST with retry + exponential backoff, matching
// the Python DLAClient semantics (3 attempts, session rebuild on
// failure, 18s per-attempt timeout, 200ms initial backoff).
//
// Retries on: network errors, 5xx, validation failure (validate
// non-nil + returns error). Does NOT retry on: 4xx, ctx done.
// Returns the validated response body bytes on success.
func (c *Client) doPost(ctx context.Context, url string, buildBody bodyBuilder, validate func([]byte) error) ([]byte, error) {
	if !c.Enabled() {
		return nil, ErrNoURL
	}
	var lastErr error
	backoff := c.backoff
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		body, contentType := buildBody()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			switch {
			case readErr != nil:
				lastErr = readErr
			case resp.StatusCode >= 500:
				lastErr = &httpError{Status: resp.Status, Body: string(data), retryable: true}
			case resp.StatusCode >= 400:
				// 4xx is a config error, not transient — surface
				// immediately without retrying.
				return nil, &httpError{Status: resp.Status, Body: string(data), retryable: false}
			case validate != nil:
				if vErr := validate(data); vErr != nil {
					lastErr = vErr
				} else {
					return data, nil
				}
			default:
				return data, nil
			}
		}
		if attempt < c.maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > MaxBackoff {
				backoff = MaxBackoff
			}
		}
	}
	if lastErr == nil {
		lastErr = ErrInvalidResponse
	}
	return nil, lastErr
}

// httpError carries the HTTP status + body so callers can inspect.
// retryable=true means the doPost loop already exhausted retries.
type httpError struct {
	Status   string
	Body     string
	retryable bool
}

func (e *httpError) Error() string {
	return "deepdoc: " + e.Status + ": " + e.Body
}
