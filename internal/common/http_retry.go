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

package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultLLMMaxRetries is how many times a failed model-provider call is
	// retried when no setting overrides it.
	DefaultLLMMaxRetries = 3
	// MaxLLMMaxRetries caps the configurable retry count. Retries share the
	// caller's request deadline, so a larger budget only delays the failure.
	MaxLLMMaxRetries = 10

	llmRetryBaseDelay = 500 * time.Millisecond
	llmRetryMaxDelay  = 30 * time.Second
	// retryDrainBytes bounds how much of a discarded failed response is read
	// so its connection can be reused.
	retryDrainBytes = 4 << 10
)

var llmMaxRetries atomic.Int64

func init() { llmMaxRetries.Store(DefaultLLMMaxRetries) }

// ValidateLLMMaxRetries reports whether n is a usable retry count: 0 (retry
// disabled) through MaxLLMMaxRetries.
func ValidateLLMMaxRetries(n int) error {
	if n < 0 || n > MaxLLMMaxRetries {
		return fmt.Errorf("llm max retries %d out of range [0, %d]", n, MaxLLMMaxRetries)
	}
	return nil
}

// SetLLMMaxRetries sets the process-wide retry count for model-provider calls.
// It takes effect for requests started afterwards.
func SetLLMMaxRetries(n int) error {
	if err := ValidateLLMMaxRetries(n); err != nil {
		return err
	}
	llmMaxRetries.Store(int64(n))
	return nil
}

// LLMMaxRetries returns the process-wide retry count for model-provider calls.
func LLMMaxRetries() int {
	return int(llmMaxRetries.Load())
}

// retryTransport retries a request when the provider is temporarily unable to
// answer: a transport failure or an HTTP 408, 429 or 5xx status (except 501).
// Only the wait for response headers is retried, so a streaming response that
// has started is never replayed; once the caller can see output it is final.
// A request whose body cannot be replayed, or whose context is done, is sent
// once.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries func() int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// NewRetryTransport wraps base with retries bounded by LLMMaxRetries.
func NewRetryTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &retryTransport{base: base, maxRetries: LLMMaxRetries, baseDelay: llmRetryBaseDelay, maxDelay: llmRetryMaxDelay}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	retries := t.maxRetries()
	if retries <= 0 || (req.Body != nil && req.Body != http.NoBody && req.GetBody == nil) {
		return t.base.RoundTrip(req)
	}

	ctx := req.Context()
	for attempt := 0; ; attempt++ {
		attemptReq := req
		if attempt > 0 {
			var err error
			if attemptReq, err = cloneForRetry(req); err != nil {
				return nil, err
			}
		}

		resp, err := t.base.RoundTrip(attemptReq)
		switch {
		case err != nil:
			if ctx.Err() != nil || !retryableError(err) {
				return nil, err
			}
		case !retryableStatus(resp.StatusCode):
			return resp, nil
		}
		if attempt >= retries {
			return resp, err
		}

		wait := t.backoff(attempt + 1)
		if resp != nil {
			if after := retryAfter(resp); after > wait {
				wait = min(after, t.maxDelay)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, retryDrainBytes))
			_ = resp.Body.Close()
		}
		Warn("Retrying provider call",
			zap.String("url", redactProviderURL(req.URL)),
			zap.Int("attempt", attempt+1),
			zap.Int("maxRetries", retries),
			zap.Duration("wait", wait),
			zap.NamedError("error", err))
		if err := sleepContext(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// backoff returns the exponential delay before the given 1-indexed retry,
// spread over its upper half so concurrent callers do not retry in lockstep.
func (t *retryTransport) backoff(retry int) time.Duration {
	d := ExponentialBackoff(t.baseDelay, t.maxDelay, retry)
	if d <= 1 {
		return d
	}
	half := d / 2
	return half + rand.N(d-half)
}

func cloneForRetry(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("failed to replay request body: %w", err)
		}
		clone.Body = body
	}
	return clone, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryableStatus reports whether a provider answered with a transient failure.
// Other 4xx statuses (authentication, validation, unknown model) and 501 will
// not change on a second attempt.
func retryableStatus(code int) bool {
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests ||
		(code >= 500 && code != http.StatusNotImplemented)
}

// retryableError reports whether a transport failure is a network condition
// (reset, refused, timeout, truncated response, temporary DNS failure) rather
// than a rejected request such as a failed SSRF check or certificate error.
func retryableError(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTimeout || dnsErr.IsTemporary
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// retryAfter returns the delay a provider asks for through Retry-After, given
// either as seconds or as an HTTP date, or 0 when absent or unparsable.
func retryAfter(resp *http.Response) time.Duration {
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return max(time.Duration(seconds)*time.Second, 0)
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}
