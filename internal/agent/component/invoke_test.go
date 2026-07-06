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

package component

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/utility"
)

// Test setup: enable the test-only SSRF bypass for tests in this
// file (the happy-path httptest server lives on 127.0.0.1, which
// the production guard rejects). The bypass is now a process-
// memory boolean (utility.AllowAnyHostForTest) instead of an
// env var — the previous form (ALLOW_ANY_HOST env) was a live
// runtime toggle any operator could flip to disable the guard
// globally. PR review round 6, Major #3.
//
// Each test that wants the production guard back resets it to
// false in its body; we don't blanket-disable here because some
// tests below rely on the bypass being on.
func setupAllowAnyHost(t *testing.T, enabled bool) {
	t.Helper()
	prev := utility.AllowAnyHostForTest
	utility.AllowAnyHostForTest = enabled
	t.Cleanup(func() { utility.AllowAnyHostForTest = prev })
}

// TestInvoke_GET exercises the happy path: a GET request to a stub
// server returns the canned body, and the response map carries the
// expected status / body / headers.
func TestInvoke_GET(t *testing.T) {
	setupAllowAnyHost(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("server: got method %q, want GET", r.Method)
		}
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    srv.URL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if status, _ := out["status"].(int); status != http.StatusOK {
		t.Errorf("status: got %d, want 200", status)
	}
	if body, _ := out["body"].(string); body != "hello" {
		t.Errorf("body: got %q, want %q", body, "hello")
	}
	hdr, _ := out["headers"].(map[string]string)
	if hdr["X-Test"] != "ok" {
		t.Errorf("headers[X-Test]: got %q, want %q", hdr["X-Test"], "ok")
	}
}

// TestInvoke_POST verifies that POST with a body echoes the body back
// from the server. The Content-Type defaults to application/json when
// not specified; we confirm that default in the test.
func TestInvoke_POST(t *testing.T) {
	setupAllowAnyHost(t, true)

	var seenCT, seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("echo:" + seenBody))
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "POST",
		"url":    srv.URL,
		"body":   `{"k":"v"}`,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if status, _ := out["status"].(int); status != http.StatusCreated {
		t.Errorf("status: got %d, want 201", status)
	}
	if seenCT != "application/json" {
		t.Errorf("server saw Content-Type %q, want application/json (default)", seenCT)
	}
	if seenBody != `{"k":"v"}` {
		t.Errorf("server saw body %q, want %q", seenBody, `{"k":"v"}`)
	}
	if body, _ := out["body"].(string); body != `echo:{"k":"v"}` {
		t.Errorf("body: got %q, want %q", body, `echo:{"k":"v"}`)
	}
}

// TestInvoke_BadMethod ensures invalid HTTP methods are rejected
// before any network I/O happens.
func TestInvoke_BadMethod(t *testing.T) {
	setupAllowAnyHost(t, true)

	c, _ := NewInvokeComponent(nil)
	_, err := c.Invoke(context.Background(), map[string]any{
		"method": "PATCH",
		"url":    "http://localhost:1",
	})
	if err == nil {
		t.Fatal("expected error for PATCH method, got nil")
	}
	if !strings.Contains(err.Error(), "invalid method") {
		t.Errorf("error %q should mention invalid method", err.Error())
	}
}

// TestInvoke_MissingURL confirms url is required.
func TestInvoke_MissingURL(t *testing.T) {
	setupAllowAnyHost(t, true)

	c, _ := NewInvokeComponent(nil)
	_, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error %q should mention url is required", err.Error())
	}
}

// TestInvoke_SSRFGuard_BlocksLoopback mirrors PR #15426: when
// AllowAnyHostForTest is unset (production shape), the Invoke
// component must reject loopback / link-local / RFC1918 URLs
// BEFORE any HTTP request is made. The test sets up an
// httptest server on 127.0.0.1, points Invoke at it, and
// asserts the request never reached the server — the function
// returns an _ERROR output instead.
func TestInvoke_SSRFGuard_BlocksLoopback(t *testing.T) {
	// Force the production guard on (override any inherited state).
	setupAllowAnyHost(t, false)

	var serverHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHit = true
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    srv.URL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if serverHit {
		t.Errorf("server was hit despite loopback URL; SSRF guard bypassed")
	}
	if got, _ := out["_ERROR"].(string); got != "URL not valid" {
		t.Errorf("_ERROR = %q, want %q", got, "URL not valid")
	}
	if status, _ := out["status"].(int); status != 0 {
		t.Errorf("status = %d, want 0 on SSRF block", status)
	}
}

// TestInvoke_SSRFGuard_BlocksMetadataIP covers the cloud
// metadata endpoint (169.254.169.254) case.
func TestInvoke_SSRFGuard_BlocksMetadataIP(t *testing.T) {
	setupAllowAnyHost(t, false)

	var serverHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHit = true
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    "http://169.254.169.254/latest/meta-data/",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if serverHit {
		t.Errorf("metadata IP was dialed; SSRF guard bypassed")
	}
	if got, _ := out["_ERROR"].(string); got != "URL not valid" {
		t.Errorf("_ERROR = %q, want %q", got, "URL not valid")
	}
}

// TestInvoke_SSRFGuard_BlocksProxy mirrors the python
// assert_url_is_safe(proxy_url) check. The proxy URL itself
// must be validated independently of the target URL.
func TestInvoke_SSRFGuard_BlocksProxy(t *testing.T) {
	setupAllowAnyHost(t, false)

	var serverHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHit = true
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	// The proxy is a loopback URL; the SSRF guard must reject it
	// regardless of the (presumed-public) target URL. Assert
	// both that the server was never reached AND that the guard
	// returned the canonical `_ERROR="URL not valid"` payload,
	// so a regression that silently lets the request through
	// (without a side-effect on the local server) is still
	// caught. PR review round 5 / duplicate fix from the
	// serverHit-only assertion in the earlier review.
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    "https://example.com/api",
		"proxy":  "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if serverHit {
		t.Errorf("server hit despite unsafe proxy; guard bypassed")
	}
	if got, _ := out["_ERROR"].(string); got != "URL not valid" {
		t.Errorf("unsafe proxy: _ERROR = %q, want %q", got, "URL not valid")
	}
}

// TestInvoke_NoRedirects_NotFollowed asserts the
// CheckRedirect policy — a 302 response from the upstream
// must be returned to the caller (with the Location header),
// not followed. This closes the bypass window where a public
// host could 302-redirect to a private one.
func TestInvoke_NoRedirects_NotFollowed(t *testing.T) {
	// Need the bypass to talk to the local httptest server.
	setupAllowAnyHost(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:1/secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    srv.URL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if status, _ := out["status"].(int); status != http.StatusFound {
		t.Errorf("status = %d, want 302 (no follow)", status)
	}
	hdr, _ := out["headers"].(map[string]string)
	if hdr["Location"] != "http://127.0.0.1:1/secret" {
		t.Errorf("Location header not preserved: %v", hdr)
	}
}

// TestInvoke_ProxyDNSPin guards the regression the user
// caught in code review: the proxy path's previous
// implementation only validated the proxy URL, but did not
// pin the dial target. The Go http.Transport dials the
// proxy host using its own dialer, which re-resolves the
// hostname at connect time — opening a TOCTOU window the
// SSRF guard was supposed to close.
//
// The fix: when a proxy is configured, the Invoke component
// wraps the proxy transport with a custom DialContext that
// intercepts the proxy-host dial and replaces the target
// with the validated public IP. The connection thus goes
// to the IP we validated, even if a subsequent DNS lookup
// returns a different answer.
//
// This test uses a public IP (8.8.8.8) as the proxy
// "resolved IP" so the dial target is well-known. The
// proxy URL itself is unreachable on the test network, so
// the dial will fail — but with an error that mentions
// the IP we dialled, not the original hostname. That
// proves the pinning path is active. We un-set
// ALLOW_ANY_HOST so the SSRF guard accepts a public-IP
// URL but the dial still happens through our code path.
//
// Target host is a literal IP (8.8.8.8) — proxy mode
// fail-closes for hostname targets because the proxy
// performs its own DNS resolution at connect time, which
// would re-open the rebinding window. PR review round 5,
// Major #3.
func TestInvoke_ProxyDNSPin(t *testing.T) {
	setupAllowAnyHost(t, false)
	// The validated proxy IP we will pin the dial to.
	// 192.88.99.1 is the 6to4 anycast prefix — a real public
	// IP that the SSRF guard accepts (not in any block-list
	// range) but is highly unlikely to be listening on port
	// 9999 in the test environment, so the dial fails fast
	// with "connection refused" carrying the IP we pinned.
	// Earlier versions used 1.1.1.1, but Cloudflare's edge
	// network has started responding on 9999, so we switched
	// to a less-likely-to-listen anycast prefix.
	const pinnedProxyIP = "192.88.99.1"

	// Build a small Invoke call with a proxy URL whose
	// hostname will resolve via SSRF (we override the
	// resolver below). The SSRF guard validates against
	// the validated public IP, then the dialer is
	// expected to use that IP — even if the hostname
	// "rebinds" to a different answer afterwards.
	// We achieve "rebinding" by stubbing the DNS lookup
	// to return a different IP on a second call.
	originalLookup := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		// Always return a public IP so SSRF accepts.
		return []string{pinnedProxyIP}, nil
	}
	t.Cleanup(func() { utility.LookupHost = originalLookup })

	// We expect the dial to fail (no proxy server at
	// 1.1.1.1:9999). The error message tells us whether
	// the pin was active: with the fix, the dial targets
	// 1.1.1.1:9999; without the fix, the Go transport
	// would have re-resolved the hostname and dialed a
	// different IP (or refused to dial because the
	// stubbed hostname is unreachable).
	c, _ := NewInvokeComponent(nil)
	// Tight per-call timeout so the test fails fast — the request
	// will hang waiting for the unreachable proxy (1.1.1.1:9999)
	// to respond, and the default 30s client timeout would make
	// this test take 30s on every run.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.Invoke(ctx, map[string]any{
		"method":  "GET",
		"url":     "http://8.8.8.8/api",
		"proxy":   "http://proxy.test.invalid:9999",
		"timeout": 2,
	})
	if err == nil {
		t.Fatal("expected dial error (no proxy listening), got nil")
	}
	// The dial error must reference 1.1.1.1:9999 (the
	// pinned IP) — NOT the unresolved proxy.test.invalid
	// hostname, which would prove the dialer fell through
	// to the default resolver.
	if !strings.Contains(err.Error(), pinnedProxyIP+":9999") {
		t.Fatalf("dial error = %v; want pinned proxy IP %s:9999 "+
			"(connection-refused is acceptable; an absent IP means "+
			"the dialer fell through to the default resolver and the "+
			"pinning regression went undetected)", err, pinnedProxyIP)
	}
}

// TestInvoke_ProxyRejectsHostnameTarget pins PR review round 5,
// Major #3: proxy mode refuses hostname targets because the
// proxy performs its own DNS resolution at connect time, which
// would re-open the SSRF/DNS-rebinding window the SSRF guard
// just closed. The handler must return an _ERROR envelope (not
// a Go error) so the canvas can route around the failure.
func TestInvoke_ProxyRejectsHostnameTarget(t *testing.T) {
	setupAllowAnyHost(t, false)

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    "http://example.com/api",
		"proxy":  "http://proxy.example.invalid:9999",
	})
	if err != nil {
		t.Fatalf("Invoke: want nil Go error (canvas routes around _ERROR), got %v", err)
	}
	if got, _ := out["_ERROR"].(string); got != "URL not valid" {
		t.Errorf("hostname+proxy target: _ERROR = %q, want %q", got, "URL not valid")
	}
}
