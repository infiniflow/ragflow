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

// Package component — Invoke component (T3).
//
// Invoke is the canvas HTTP client node. It supports GET/POST/
// PUT/DELETE with custom headers, optional proxy, and per-request
// timeout, and wraps the underlying net/http.Transport with
// go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
// .NewTransport so outbound calls automatically propagate W3C
// traceparent headers.
//
// SSRF guard (PR #15426): every outbound URL is validated against
// the shared utility.AssertURLSafe before any network I/O. Both the
// target URL and an optional proxy URL are checked — the proxy
// vector matters because the Go transport hands the request to the
// proxy host, which would otherwise re-resolve the original host
// and re-open the rebinding window the SSRF guard just closed. To
// defeat DNS rebinding the transport dials the validated public IP
// directly (utility.PinnedHTTPClient) and we disable redirect
// following so a 30x to a private host cannot bypass the guard.
package component

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"ragflow/internal/utility"
)

const (
	componentNameInvoke = "Invoke"

	defaultInvokeTimeout   = 30 * time.Second
	defaultInvokeUserAgent = "ragflow-agent/1.0 (Invoke component)"
	defaultInvokeContentCT = "application/json"
	maxInvokeResponseBody  = 16 << 20 // 16 MiB; hard cap to avoid OOM
)

// InvokeComponent is the HTTP client node. Stateless across invocations.
type InvokeComponent struct {
	name string
}

// NewInvokeComponent constructs an Invoke component.
func NewInvokeComponent(_ map[string]any) (Component, error) {
	return &InvokeComponent{name: componentNameInvoke}, nil
}

// Name returns the registered component name.
func (i *InvokeComponent) Name() string { return i.name }

// Invoke executes a single HTTP request and returns the status code,
// body, and response headers. See Inputs() for the param contract.
//
// SSRF flow (PR #15426):
//  1. Validate the target URL via utility.AssertURLSafe (loopback /
//     link-local / RFC1918 / metadata / unresolvable are rejected).
//  2. Validate the optional proxy URL the same way (the proxy
//     re-resolves the target host; an unsafe proxy would defeat
//     step 1).
//  3. Use utility.PinnedHTTPClient to dial the validated public IP
//     for the target host — closing the TOCTOU window between
//     validation and connect.
//  4. Disable redirect following so a 30x to a private host cannot
//     silently bypass the guard.
//
// On any of those checks failing the function returns an `_ERROR`
// output (no Go error) so the canvas can route around the failure
// the same way the Python fix does, instead of crashing the node.
func (i *InvokeComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	method, _ := inputs["method"].(string)
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return nil, fmt.Errorf("Invoke: invalid method %q (want GET/POST/PUT/DELETE)", method)
	}
	rawURL, _ := inputs["url"].(string)
	if rawURL == "" {
		return nil, errors.New("Invoke: url is required")
	}
	// Bare hostnames (no scheme) are rejected — the Python fix
	// prefixes "http://" before validating, but the Go side treats
	// the absence of a scheme as a programmer error so a canvas
	// author must be explicit. url.Parse is a sanity check; we
	// trust the orchestrator to have already resolved any
	// {{...}} refs.
	if _, err := url.Parse(rawURL); err != nil {
		return nil, fmt.Errorf("Invoke: parse url: %w", err)
	}

	// Step 1: SSRF guard for the target URL. The validated
	// hostname + resolved public IP are reused for DNS pinning.
	host, pinnedIP, err := utility.AssertURLSafe(rawURL)
	if err != nil {
		return invokeSSRFError("url", rawURL, err), nil
	}

	// Step 2: SSRF guard for the proxy URL (if configured).
	// Mirrors the Python assert_url_is_safe(proxy_url) check.
	var (
		proxyHost string
		proxyIP   string
	)
	proxyStr, _ := inputs["proxy"].(string)
	if proxyStr != "" {
		// Fail-closed target check (PR #15426 round-2 review).
		//
		// When a proxy is configured, Go dials the proxy host and then
		// forwards the request URL — including the target hostname —
		// through the proxy. Go does NOT dial the target itself, so
		// our pinned-IP DialContext only protects the proxy→us hop.
		// The proxy performs its own DNS resolution for the target
		// hostname at connect time, which re-opens the
		// SSRF/DNS-rebinding window the SSRF guard just closed.
		//
		// The safe fix is to refuse hostname targets in proxy mode:
		// a literal-IP target cannot rebind (there is nothing to
		// resolve), so the proxy either relays the IP as-is or
		// refuses — either way we have not given it a window to
		// mis-resolve. Hostname targets must be sent direct (no
		// proxy) so our PinnedHTTPClient can pin the dial.
		//
		// The Python reference accepted this trade-off for proxy mode
		// in PR #15426 (it also has no way to constrain the
		// proxy's resolution); we make it explicit at the Invoke
		// layer so a caller cannot accidentally rely on the guard
		// for a hostname+proxy combination.
		if net.ParseIP(host) == nil {
			return invokeSSRFError("url", rawURL,
				fmt.Errorf("Invoke: proxy mode requires a literal-IP target URL (hostnames are unsafe because the proxy re-resolves them)")), nil
		}
		ph, pip, perr := utility.AssertURLSafe(proxyStr)
		if perr != nil {
			return invokeSSRFError("proxy", proxyStr, perr), nil
		}
		proxyHost, proxyIP = ph, pip
	}

	timeout := defaultInvokeTimeout
	if v, ok := inputs["timeout"].(int); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	} else if v, ok := inputs["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	}

	contentType, _ := inputs["content_type"].(string)
	if contentType == "" && (method == http.MethodPost || method == http.MethodPut) {
		contentType = defaultInvokeContentCT
	}

	var body io.Reader
	if s, ok := inputs["body"].(string); ok != false && s != "" {
		body = bytes.NewReader([]byte(s))
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("Invoke: build request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", defaultInvokeUserAgent)
	if h, ok := inputs["headers"].(map[string]any); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	// Step 3: build the client. When a proxy is configured, the
	// Go transport dials the proxy host using its own dialer,
	// which would re-resolve the proxy hostname at connect time
	// and re-open the rebinding window the SSRF guard just
	// closed. We pin the proxy dial by wrapping a custom
	// DialContext that intercepts the proxy-host dial and
	// replaces the target with the validated proxy IP. The
	// underlying TCP connection thus goes to the IP we
	// validated, even if a subsequent DNS lookup returns a
	// different answer (TOCTOU). The validated IP is captured
	// from the proxy SSRF check above (proxyIP).
	var client *http.Client
	if proxyStr != "" {
		proxyURL := mustParseProxy(proxyStr)
		pinnedProxyDialer := &net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}
		client = &http.Client{
			Timeout: timeout,
			Transport: otelhttp.NewTransport(&http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					// When the transport dials the proxy, addr is
					// "<proxy_host>:<proxy_port>". Replace the
					// host with the validated public IP while
					// keeping the original port. Any other dial
					// (e.g. a redirect hop the no-redirect policy
					// would have blocked) falls through to the
					// default dialer.
					host, port, splitErr := net.SplitHostPort(addr)
					if splitErr != nil || host != proxyURL.Hostname() || proxyIP == "" {
						return pinnedProxyDialer.DialContext(ctx, network, addr)
					}
					return pinnedProxyDialer.DialContext(ctx, network, net.JoinHostPort(proxyIP, port))
				},
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
				ExpectContinueTimeout: 1 * time.Second,
				ForceAttemptHTTP2:     false,
			}),
			CheckRedirect: noRedirects,
		}
	} else {
		// Direct path: pin to the validated public IP, disable
		// redirects, and apply the OTel transport. PinnedHTTPClient
		// sets its own timeout; we re-wrap with otelhttp so the
		// request gets a child span + W3C traceparent injected.
		pinned := utility.PinnedHTTPClient(host, pinnedIP, timeout)
		pinned.Transport = otelhttp.NewTransport(pinned.Transport)
		pinned.CheckRedirect = noRedirects
		client = pinned
	}
	_ = proxyHost
	_ = proxyIP

	// a generic HTTP client node in the canvas DSL — operators wire it
	// to arbitrary endpoints. SSRF surface is limited to operators
	// (not end users), and outbound traffic is rate-limited by the
	// client timeout + maxInvokeResponseBody cap above.
	// codeql[go/request-forgery] Intentional: the Invoke component is
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Invoke: do: %w", err)
	}
	defer resp.Body.Close()

	// Cap the response body to keep a hostile server from streaming
	// infinite bytes into memory.
	limited := io.LimitReader(resp.Body, maxInvokeResponseBody)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("Invoke: read body: %w", err)
	}

	hdr := make(map[string]string, len(resp.Header))
	for k, vs := range resp.Header {
		// First value only — multi-value headers are uncommon in
		// canvas-DSL HTTP responses, and the param contract specifies
		// a string map.
		if len(vs) > 0 {
			hdr[k] = vs[0]
		}
	}

	bodyStr := string(bodyBytes)

	// Clean HTML from response body when clean_html input is set.
	if cleanHTML, _ := inputs["clean_html"].(bool); cleanHTML {
		bodyStr = stripHTMLTags(bodyStr)
	}

	// Parse body according to the requested datatype.
	datatype, _ := inputs["datatype"].(string)
	if datatype == "" {
		// Infer from Content-Type header.
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			datatype = "json"
		} else {
			datatype = "text"
		}
	}

	return map[string]any{
		"status":   resp.StatusCode,
		"body":     bodyStr,
		"headers":  hdr,
		"datatype": datatype,
	}, nil
}

// invokeSSRFError builds the _ERROR output the canvas uses to route
// around a refused URL. We mirror the Python message verbatim
// ("URL not valid") so downstream consumers that key on the string
// keep working after the port.
func invokeSSRFError(kind, raw string, err error) map[string]any {
	zap.L().Warn("Invoke SSRF guard blocked request",
		zap.String("kind", kind),
		zap.String("url", sanitizeLogURL(raw)),
		zap.Error(err),
	)
	return map[string]any{
		"_ERROR":  "URL not valid",
		"status":  0,
		"body":    "",
		"headers": map[string]string{},
	}
}

// noRedirects is the http.Client.CheckRedirect value that matches
// the python requests `allow_redirects=False` semantics — a 30x is
// returned to the caller as a normal response (with the Location
// header) instead of being followed. Without this, a 302 to a
// private host would silently bypass the SSRF guard above.
func noRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// sanitizeLogURL redacts the path / query from a URL so error logs
// don't echo operator-configured tokens (e.g. an API key passed as
// a path component).
func sanitizeLogURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<invalid-url>"
	}
	return u.Scheme + "://" + u.Host
}

// Stream is a synchronous facade over Invoke. Real streaming
// (chunked transfer as it arrives) is a future enhancement.
func (i *InvokeComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := i.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the public parameter surface.
func (i *InvokeComponent) Inputs() map[string]string {
	return map[string]string{
		"method":       "HTTP method: GET, POST, PUT, or DELETE (case-insensitive).",
		"url":          "Target URL; can be a {{...}} reference resolved upstream.",
		"headers":      "Optional map of string headers.",
		"body":         "Optional request body (string).",
		"timeout":      "Per-request timeout in seconds; default 30.",
		"proxy":        "Optional proxy URL (e.g. http://host:3128).",
		"content_type": "Optional Content-Type; default 'application/json' for POST/PUT.",
		"clean_html":   "When true, strip HTML tags from the response body.",
		"datatype":     "Expected response datatype: 'json', 'text', or 'html'. Default 'json'.",
		"variables":    "Optional template variables for URL/body interpolation.",
	}
}

// Outputs returns the response surface.
func (i *InvokeComponent) Outputs() map[string]string {
	return map[string]string{
		"status":   "HTTP status code (int).",
		"body":     "Response body (string, truncated at 16 MiB).",
		"headers":  "Response headers (first value per key).",
		"datatype": "Inferred response datatype: 'json' | 'text' | 'html'.",
	}
}

// mustParseProxy parses a proxy URL string. We keep this helper here
// (rather than calling url.Parse inline) so the panic-on-bad-input
// behavior is uniform across the package — proxy strings are operator-
// configured, a malformed one is a deployment error worth crashing
// loud on.
func mustParseProxy(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("Invoke: invalid proxy URL %q: %v", raw, err))
	}
	// Defensive check: net/http.ProxyURL will silently no-op on a
	// URL with no Host. Surface a clear panic instead.
	if u.Host == "" {
		panic(fmt.Sprintf("Invoke: proxy URL %q has no host", raw))
	}
	return u
}

// stripHTMLTags removes HTML tags from the input string. This is a
// best-effort implementation — it uses a simple regexp to remove
// everything between < and >. It is NOT a full HTML sanitizer and
// should only be used for cleaning up response text for consumption
// by downstream LLM nodes.
// Mirrors Python's `strip_html_tags` helper (invoke.py).
func stripHTMLTags(s string) string {
	// Simple regexp-based approach: remove everything between < and >
	re := strings.NewReplacer(
		"<script", "\n<script",
		"</script>", "</script>\n",
		"<style", "\n<style",
		"</style>", "</style>\n",
	)
	s = re.Replace(s)
	for {
		start := strings.Index(s, "<")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], ">")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	// Collapse multiple newlines
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// netHTTPImports is a no-op reference to keep `net` in the import set
// for go vet's unused-import check while the production code path
// doesn't otherwise need the net package (only used by the optional
// proxy path via http.ProxyURL).
var _ = net.IPv4len

func init() {
	Register(componentNameInvoke, NewInvokeComponent)
}
