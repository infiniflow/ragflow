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

// Package tool implements RAGFlow agent canvas tool adapters in Go.
//
// All tools implement eino's tool.InvokableTool interface and are
// intended to be registered with the agent canvas via a factory function
// (see .claude/plans/agent-go-port.md §2.11.4 "Tool 关键统一模式").
package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	neturl "net/url"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// RetryConfig controls the HTTPHelper retry policy. The defaults
// (3 attempts, 200ms base, 3s max backoff) apply when the
// corresponding field is zero. HTTPHelper{} and NewHTTPHelper()
// are interchangeable.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (including the first one).
	// Values < 1 are clamped to 1 (no retry). Default 3.
	MaxAttempts int
	// BaseBackoff is the initial backoff between attempts. Default 200ms.
	BaseBackoff time.Duration
	// MaxBackoff caps the exponential backoff. Default 3s.
	MaxBackoff time.Duration
}

func (c RetryConfig) withDefaults() RetryConfig {
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

// HTTPHelper is a context-aware HTTP client shared by the HTTP
// tools. It wraps http.Client with otelhttp.NewTransport for
// OTel span propagation, enforces a 30s default timeout, and
// retries idempotent failures (5xx + network errors) per the
// RetryConfig.
//
// HTTPHelper is safe for concurrent use by multiple goroutines.
type HTTPHelper struct {
	// baseTransport is the source-of-truth *http.Transport. Do wraps it
	// with otelhttp and stores the result in h.client. DoPinned clones it
	// and installs a pinned dialer (see pinnedDialer) so the connect
	// goes to a known IP regardless of what the request's URL says.
	// Tests may mutate baseTransport directly to inject a custom
	// TLSClientConfig (e.g. an in-memory RootCAs pool) — the change
	// applies to both Do and DoPinned.
	baseTransport *http.Transport

	// client is the RoundTripper used by Do. It is an OTel-wrapped view
	// of baseTransport. WithClient replaces this field; DoPinned always
	// derives its client from baseTransport regardless, so a custom
	// client installed via WithClient does NOT participate in
	// DNS-rebinding pinning. Pinning needs the dialer to be replaced at
	// the transport layer and that is only possible if we own the
	// transport — hence the baseTransport/client split.
	client *http.Client

	retry RetryConfig
}

// NewHTTPHelper returns a HTTPHelper with default retry/timeout settings.
func NewHTTPHelper() *HTTPHelper {
	base := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &HTTPHelper{
		baseTransport: base,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(base),
		},
		retry: RetryConfig{}.withDefaults(),
	}
}

// NewHTTPHelperWithRetry returns a HTTPHelper with a custom retry policy.
// Pass RetryConfig{} to use defaults.
func NewHTTPHelperWithRetry(rc RetryConfig) *HTTPHelper {
	h := NewHTTPHelper()
	h.retry = rc.withDefaults()
	return h
}

// WithClient replaces the underlying http.Client used by Do. Useful for
// tests that supply a pre-configured transport (e.g. for OTel recording).
//
// Note: WithClient affects Do calls only. DoPinned always derives its
// client from the helper's internal baseTransport, so a custom client
// installed here does NOT participate in DNS-rebinding pinning. To
// customise pinning behaviour (RootCAs, etc.) mutate baseTransport
// directly — typically only tests do this.
func (h *HTTPHelper) WithClient(c *http.Client) *HTTPHelper {
	if c != nil {
		h.client = c
	}
	return h
}

// Do issues an HTTP request and returns the response. The caller is
// responsible for closing the returned response body.
//
// body and contentType may be empty (e.g. for GET). When body is non-empty
// and contentType is empty, "application/octet-stream" is assumed so servers
// that sniff the body still behave sensibly.
//
// Retry policy:
//   - 5xx responses: retried
//   - network errors (connection refused, DNS, etc.): retried
//   - 4xx responses: NOT retried (caller error, won't help to retry)
//   - 2xx / 3xx: returned as-is
//
// The context is honored on every attempt; cancellation aborts the loop.
func (h *HTTPHelper) Do(
	ctx context.Context,
	method, url, body, contentType string,
	headers map[string]string,
) (*http.Response, error) {
	return h.doRawWithClient(ctx, h.client, method, url, body, contentType, headers)
}

// DoPinned is identical to Do but dials pinnedIP for the host in url,
// while leaving the request URL host untouched. Use immediately after a
// successful ResolveAndValidate (ssrf.go) to defeat DNS-rebinding: an
// attacker cannot swap a public A record for a private one between the
// validation lookup and the connect, because the connect goes to the IP
// we resolved here and the URL host is never re-resolved by the
// transport (Go's *http.Transport only re-resolves when the URL's host
// is used in the dial, and even then the pinned dialer intercepts).
//
// The TLS handshake ServerName is derived from the URL host
// (Go's http.Transport populates tls.Config.ServerName from
// req.URL.Host before calling DialContext), so SNI sends originalHost
// and the cert is verified against originalHost — not against the IP.
// This is critical: rewriting u.Host to the IP (the previous version of
// this fix) sent the IP as SNI and required an IP SAN in the cert,
// which is not what real HTTPS sites have. Pinning must happen at the
// transport layer, not by mutating the request URL.
//
// originalHost and pinnedIP are required; an empty value for either
// returns an error so misuse is loud rather than silently falling back
// to a non-pinned connection. The URL host must also equal originalHost
// (defense in depth — a caller that passes a mismatched URL would
// produce broken TLS and we refuse rather than silently deliver it).
//
// Note on proxies: the pinned transport has Proxy disabled, regardless
// of baseTransport.Proxy (e.g. HTTP_PROXY/HTTPS_PROXY env). A proxy
// would re-resolve the hostname, re-opening the rebinding window the
// SSRF guard just closed — and the pinned dialer would otherwise
// rewrite the proxy's own dial target to pinnedIP:proxyPort, breaking
// the connection. Operators who want proxied outbound should use Do.
func (h *HTTPHelper) DoPinned(
	ctx context.Context,
	method, url, body, contentType string,
	headers map[string]string,
	originalHost string,
	pinnedIP net.IP,
) (*http.Response, error) {
	if originalHost == "" || pinnedIP == nil {
		return nil, errors.New("http_helper: DoPinned requires originalHost and pinnedIP")
	}
	u, err := neturl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("http_helper: parse url: %w", err)
	}
	if u.Hostname() != originalHost {
		return nil, fmt.Errorf(
			"http_helper: DoPinned url host %q != originalHost %q "+
				"(refusing to pin — would break TLS SNI / cert check)",
			u.Hostname(), originalHost)
	}
	return h.doRawWithClient(ctx, h.pinnedClient(pinnedIP), method, url, body, contentType, headers)
}

// pinnedClient builds a one-shot *http.Client whose transport dials
// pinnedIP:port instead of originalHost:port. The transport is cloned
// from baseTransport so the rest of the connection behaviour (TLS
// config, idle pool) is identical to a non-pinned call.
//
// The proxy setting is explicitly disabled. Two reasons:
//
//  1. *http.Transport dials the proxy first, and pinnedDialer would
//     rewrite the proxy's own address to pinnedIP:proxyPort — that
//     would redirect the proxy dial to a port on the pinned IP and
//     fail connection establishment in any proxied deployment.
//
//  2. Even if the dialer were proxy-aware, the proxy itself receives
//     the request URL (with the original hostname) and re-resolves
//     it, which re-opens the DNS-rebinding window the SSRF guard
//     just closed. The pinned dialer must be the only thing that
//     decides where the TCP connection goes.
//
// Operators who want proxied outbound for pinned traffic should use Do
// (without pinning) and validate upstream of the proxy themselves.
//
// Design note: "pinned + proxy" is intentionally not supported today.
// A proxy that re-resolves the original hostname would defeat the
// pinning; only a transparent proxy (one that forwards by IP and does
// not re-resolve) could coexist with SSRF-pinned traffic, and the
// Go stdlib's http.Transport.Proxy contract does not give us a
// reliable way to express that. If a future deployment needs
// pinned traffic through a specific egress proxy, add a dedicated
// DoPinnedWithProxy (or an HTTPHelper config option that takes an
// already-pinned IP) — do not silently re-enable Proxy here without
// also re-deriving the SSRF guarantee for that path.
func (h *HTTPHelper) pinnedClient(pinnedIP net.IP) *http.Client {
	base := h.baseTransport.Clone()
	base.Proxy = nil
	base.DialContext = (&pinnedDialer{
		pinnedIP: pinnedIP,
		base: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}).DialContext
	return &http.Client{
		Timeout:   h.client.Timeout,
		Transport: otelhttp.NewTransport(base),
	}
}

// pinnedDialer is a net.Dialer-compatible DialContext that rewrites the
// destination address to pinnedIP:port. The host part of the dial
// address is discarded; only the port is preserved. The host of the
// HTTP request is not in scope here — that lives on the request, where
// it is used by the TLS layer for SNI / cert verification.
type pinnedDialer struct {
	pinnedIP net.IP
	base     *net.Dialer
}

func (d *pinnedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("http_helper: pinned dial: split %q: %w", addr, err)
	}
	return d.base.DialContext(ctx, network, net.JoinHostPort(d.pinnedIP.String(), port))
}

// doRawWithClient is the shared retry loop behind Do and DoPinned. The
// client decides the transport (and therefore the dialer); Do uses the
// helper's default OTel-wrapped client, DoPinned uses a per-call client
// with a transport-level pinned dialer.
func (h *HTTPHelper) doRawWithClient(
	ctx context.Context,
	client *http.Client,
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
	for attempt := 1; attempt <= h.retry.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader([]byte(body)))
		if err != nil {
			return nil, fmt.Errorf("http_helper: build request: %w", err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableNetError(err) {
				return nil, err
			}
			if attempt == h.retry.MaxAttempts {
				return nil, fmt.Errorf("http_helper: %s %s failed after %d attempts: %w", method, SanitizeURL(url), attempt, err)
			}
			sleepCtx(ctx, backoff(h.retry.BaseBackoff, h.retry.MaxBackoff, attempt))
			continue
		}

		// 5xx is retryable, 4xx is not.
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("http_helper: %s %s returned %d", method, SanitizeURL(url), resp.StatusCode)
			// drain body so the connection can be reused
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if attempt == h.retry.MaxAttempts {
				return nil, lastErr
			}
			sleepCtx(ctx, backoff(h.retry.BaseBackoff, h.retry.MaxBackoff, attempt))
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("http_helper: exhausted retries with no recorded error")
}

// backoff returns an exponentially increasing duration with full jitter,
// capped at max. attempt is 1-indexed; the first retry uses BaseBackoff.
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
	// full jitter: randomize in [0, d] to avoid thundering herd
	return time.Duration(rand.Int64N(int64(d) + 1))
}

// sleepCtx waits for d, returning early if ctx is canceled.
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

// isRetryableNetError reports whether a transport-level error should trigger
// a retry. Context cancellation / deadline-exceeded are NOT retryable — the
// caller explicitly asked for that.
func isRetryableNetError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}
