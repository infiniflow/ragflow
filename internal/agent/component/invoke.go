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
	// url.Parse is a sanity check; we trust the orchestrator to have
	// already resolved any {{...}} refs, but a bad string here is a
	// programmer error worth surfacing.
	if _, err := url.Parse(rawURL); err != nil {
		return nil, fmt.Errorf("Invoke: parse url: %w", err)
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
	if s, ok := inputs["body"].(string); ok && s != "" {
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

	// Wrap the stdlib Transport with otelhttp so the request gets a
	// child span + W3C traceparent injected automatically.
	transport := otelhttp.NewTransport(http.DefaultTransport)
	// Optional proxy support: if inputs["proxy"] is set, build a
	// dedicated Transport that uses it. This avoids mutating the
	// global http.DefaultTransport (which would also affect unrelated
	// components in the same process).
	if proxyStr, ok := inputs["proxy"].(string); ok && proxyStr != "" {
		transport = otelhttp.NewTransport(&http.Transport{
			Proxy: http.ProxyURL(mustParseProxy(proxyStr)),
		})
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
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

	return map[string]any{
		"status":  resp.StatusCode,
		"body":    string(bodyBytes),
		"headers": hdr,
	}, nil
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
	}
}

// Outputs returns the response surface.
func (i *InvokeComponent) Outputs() map[string]string {
	return map[string]string{
		"status":  "HTTP status code (int).",
		"body":    "Response body (string, truncated at 16 MiB).",
		"headers": "Response headers (first value per key).",
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

// netHTTPImports is a no-op reference to keep `net` in the import set
// for go vet's unused-import check while the production code path
// doesn't otherwise need the net package (only used by the optional
// proxy path via http.ProxyURL).
var _ = net.IPv4len

func init() {
	Register(componentNameInvoke, NewInvokeComponent)
}
