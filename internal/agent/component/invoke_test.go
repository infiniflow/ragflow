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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"ragflow/internal/common"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Set the environment opt-in so tests select private-host access explicitly.
func setupAllowAnyHost(t *testing.T, enabled bool) {
	t.Helper()
	t.Setenv(common.EnvAllowAnyHost, strconv.FormatBool(enabled))
}

// TestInvoke_GET exercises the happy path: a GET request to a stub
// server returns the canned body as the Python-compatible result output.
func TestInvoke_GET(t *testing.T) {
	setupAllowAnyHost(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("server: got method %q, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"method": "GET",
		"url":    srv.URL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["result"].(string); got != "hello" {
		t.Errorf("result: got %q, want hello", got)
	}
	if len(out) != 3 {
		t.Errorf("output = %#v, want result and timing fields", out)
	}
	if _, ok := out["_created_time"].(float64); !ok {
		t.Errorf("_created_time = %#v, want float64", out["_created_time"])
	}
	if elapsed, ok := out["_elapsed_time"].(float64); !ok || elapsed < 0 {
		t.Errorf("_elapsed_time = %#v, want non-negative float64", out["_elapsed_time"])
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"method": "POST",
		"url":    srv.URL,
		"body":   `{"k":"v"}`,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if seenCT != "application/json" {
		t.Errorf("server saw Content-Type %q, want application/json (default)", seenCT)
	}
	if seenBody != `{"k":"v"}` {
		t.Errorf("server saw body %q, want %q", seenBody, `{"k":"v"}`)
	}
	if got, _ := out["result"].(string); got != `echo:{"k":"v"}` {
		t.Errorf("result: got %q, want %q", got, `echo:{"k":"v"}`)
	}
}

func TestInvoke_UsesNodeParams(t *testing.T) {
	setupAllowAnyHost(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Configured"); got != "yes" {
			t.Errorf("X-Configured = %q, want yes", got)
		}
		_, _ = w.Write([]byte("configured"))
	}))
	defer srv.Close()

	c, err := NewInvokeComponent(map[string]any{
		"method":  "GET",
		"url":     srv.URL,
		"headers": `{"X-Configured":"yes"}`,
	})
	if err != nil {
		t.Fatalf("NewInvokeComponent: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["result"].(string); got != "configured" {
		t.Errorf("result = %q, want configured", got)
	}
}

func TestInvokeHeadersToleratesInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want map[string]any
	}{
		{name: "malformed JSON", raw: `{"Authorization":"Bearer secret-token"`, want: nil},
		{name: "empty", raw: "", want: nil},
		{name: "array", raw: `["secret-token"]`, want: nil},
		{name: "scalar", raw: `true`, want: nil},
		{name: "null", raw: "null", want: nil},
		{name: "unsupported", raw: 42, want: nil},
		{name: "object", raw: `{"X-Test":"yes"}`, want: map[string]any{"X-Test": "yes"}},
		{name: "direct map", raw: map[string]any{"X-Test": "yes"}, want: map[string]any{"X-Test": "yes"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := invokeHeaders(tt.raw)
			if err != nil {
				t.Fatalf("invokeHeaders() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("invokeHeaders() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestInvokeHeadersDoesNotLogRawPayload(t *testing.T) {
	previous := zap.L()
	core, logs := observer.New(zap.WarnLevel)
	zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(func() { zap.ReplaceGlobals(previous) })

	const secret = "Bearer secret-token"
	if _, err := invokeHeaders(`{"Authorization":"` + secret); err != nil {
		t.Fatalf("invokeHeaders() error = %v", err)
	}
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, secret) || strings.Contains(fmt.Sprint(entry.ContextMap()), secret) {
			t.Fatalf("log contains raw header payload: %#v", entry)
		}
	}
}

func TestInvoke_GetInputForm(t *testing.T) {
	c, err := NewInvokeComponent(map[string]any{
		"variables": []any{
			map[string]any{"key": "Search query", "ref": "sys.query"},
		},
	})
	if err != nil {
		t.Fatalf("NewInvokeComponent: %v", err)
	}
	getter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("Invoke does not expose GetInputForm")
	}
	field, ok := getter.GetInputForm()["sys.query"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[sys.query] = %#v, want field", getter.GetInputForm()["sys.query"])
	}
	if field["type"] != "line" || field["name"] != "Search query" {
		t.Errorf("field = %#v, want line Search query", field)
	}
}

// TestInvoke_BadMethod ensures invalid HTTP methods are rejected
// before any network I/O happens.
func TestInvoke_BadMethod(t *testing.T) {
	setupAllowAnyHost(t, true)

	c, _ := NewInvokeComponent(nil)
	_, err := c.Invoke(t.Context(), nil, map[string]any{
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
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error %q should mention url is required", err.Error())
	}
}

func TestInvoke_AllowAnyHost(t *testing.T) {
	setupAllowAnyHost(t, false)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, "local.test.invalid:") || r.URL.Path != "/logo.svg" {
			t.Errorf("request = %s%s, want local.test.invalid:<port>/logo.svg", r.Host, r.URL.Path)
		}
		_, _ = w.Write([]byte("local response"))
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}

	originalLookup := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		if host != "local.test.invalid" {
			return nil, fmt.Errorf("unexpected hostname %q", host)
		}
		return []string{"127.0.0.1"}, nil
	}
	t.Cleanup(func() { common.LookupHost = originalLookup })

	for _, value := range []string{"", "0", "1"} {
		t.Run("ALLOW_ANY_HOST="+value, func(t *testing.T) {
			t.Setenv(common.EnvAllowAnyHost, value)
			c, err := NewInvokeComponent(nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"method": "GET",
				"url":    "http://local.test.invalid:" + port + "/logo.svg",
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if value == "1" {
				if out["result"] != "local response" || out["_ERROR"] != nil {
					t.Fatalf("output = %#v, want local response", out)
				}
			} else if out["_ERROR"] != "URL not valid" || out["result"] != nil {
				t.Fatalf("output = %#v, want blocked URL", out)
			}
		})
	}
}

func TestInvoke_AllowAnyHostRejectsInvalidURLs(t *testing.T) {
	setupAllowAnyHost(t, false)
	t.Setenv(common.EnvAllowAnyHost, "1")
	originalLookup := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		return nil, fmt.Errorf("no such host: %s", host)
	}
	t.Cleanup(func() { common.LookupHost = originalLookup })

	for _, kind := range []string{"url", "proxy"} {
		for _, target := range []string{"file://127.0.0.1/logo.svg", "http:///logo.svg", "http://missing.test.invalid/logo.svg"} {
			t.Run(kind+"/"+target, func(t *testing.T) {
				c, err := NewInvokeComponent(nil)
				if err != nil {
					t.Fatal(err)
				}
				inputs := map[string]any{"method": "GET", "url": "http://192.168.1.5/logo.svg"}
				inputs[kind] = target
				out, err := c.Invoke(t.Context(), nil, inputs)
				if err != nil {
					t.Fatalf("Invoke: %v", err)
				}
				if out["_ERROR"] != "URL not valid" || out["result"] != nil {
					t.Fatalf("output = %#v, want blocked URL", out)
				}
			})
		}
	}
}

// TestInvoke_SSRFGuard_BlocksLoopback mirrors PR #15426: when
// ALLOW_ANY_HOST is unset, the Invoke
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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
	if result, exists := out["result"]; !exists || result != nil {
		t.Errorf("result = %#v, want nil result on SSRF block", result)
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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
	setupAllowAnyHost(t, false)
	t.Setenv(common.EnvAllowAnyHost, "1")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:1/secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"method": "GET",
		"url":    srv.URL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["result"].(string); got != "" {
		t.Errorf("result = %q, want empty 302 response body", got)
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
// proves the pinning path is active. ALLOW_ANY_HOST permits
// the local proxy while the dial stays pinned.
//
// Target host is a literal IP (8.8.8.8) — proxy mode
// fail-closes for hostname targets because the proxy
// performs its own DNS resolution at connect time, which
// would re-open the rebinding window. PR review round 5,
// Major #3.
func TestInvoke_ProxyDNSPin(t *testing.T) {
	setupAllowAnyHost(t, false)
	t.Setenv(common.EnvAllowAnyHost, "1")

	proxyHit := make(chan struct{}, 1)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit <- struct{}{}
		if r.RequestURI != "http://8.8.8.8/api" {
			t.Errorf("proxy RequestURI = %q, want absolute-form target", r.RequestURI)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxySrv.Close()

	proxyURL, err := url.Parse(proxySrv.URL)
	if err != nil {
		t.Fatalf("parse proxy server URL: %v", err)
	}
	pinnedProxyIP, proxyPort, err := net.SplitHostPort(proxyURL.Host)
	if err != nil {
		t.Fatalf("split proxy server host: %v", err)
	}

	// Build a small Invoke call with a proxy URL whose
	// hostname will resolve via SSRF (we override the
	// resolver below). The SSRF guard validates against
	// the validated public IP, then the dialer is
	// expected to use that IP — even if the hostname
	// "rebinds" to a different answer afterward.
	// We achieve "rebinding" by stubbing the DNS lookup
	// to return a different IP on a second call.
	originalLookup := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		// Always return the already-running fake proxy. If the
		// Invoke transport re-resolves proxy.test.invalid instead
		// of using the pinned IP, the request will never hit it.
		return []string{pinnedProxyIP}, nil
	}
	t.Cleanup(func() { common.LookupHost = originalLookup })

	for _, tc := range []struct {
		name   string
		prefix string
		suffix string
	}{
		{name: "plain"},
		{name: "spaces", prefix: " ", suffix: " "},
		{name: "tabs and newlines", prefix: "\t", suffix: "\r\n"},
		{name: "unicode whitespace", prefix: "\u3000", suffix: "\u00a0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewInvokeComponent(map[string]any{
				"method":  "GET",
				"url":     "http://8.8.8.8/api",
				"proxy":   tc.prefix + "http://proxy.test.invalid:" + proxyPort + tc.suffix,
				"timeout": 2,
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			out, err := c.Invoke(ctx, nil, nil)
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if out["_ERROR"] != nil || out["result"] != "" {
				t.Fatalf("output = %#v, want empty proxy response", out)
			}
			select {
			case <-proxyHit:
			case <-time.After(time.Second):
				t.Fatal("fake proxy was not hit; proxy dial was not pinned to validated IP")
			}
		})
	}
}

func TestInvokeBlankProxy(t *testing.T) {
	setupAllowAnyHost(t, true)
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "direct response")
	}))
	defer srv.Close()

	for _, proxy := range []string{"", " ", "\t\r\n", "\u3000\u00a0"} {
		t.Run(fmt.Sprintf("%q", proxy), func(t *testing.T) {
			before := requests.Load()
			c, err := NewInvokeComponent(map[string]any{
				"method": "GET",
				"url":    srv.URL,
				"proxy":  proxy,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, nil)
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if proxy == "" {
				if out["result"] != "direct response" || out["_ERROR"] != nil {
					t.Fatalf("output = %#v, want direct response", out)
				}
				return
			}
			if out["_ERROR"] != "URL not valid" || out["result"] != nil {
				t.Errorf("output = %#v, want rejected proxy", out)
			}
			if requests.Load() != before {
				t.Error("whitespace-only proxy sent a direct request")
			}
		})
	}
}

func TestInvokeInvalidProxy(t *testing.T) {
	setupAllowAnyHost(t, false)
	for _, proxy := range []string{
		"http://%",
		"http://8.8.4.4:invalid",
		"http://8.8.4.4:\t3128",
		"http:///proxy",
		"file://8.8.4.4/proxy",
		" http://127.0.0.1:3128 ",
	} {
		t.Run(fmt.Sprintf("%q", proxy), func(t *testing.T) {
			c, err := NewInvokeComponent(map[string]any{
				"method": "GET",
				"url":    "http://8.8.8.8/api",
				"proxy":  proxy,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, nil)
			if err != nil {
				t.Fatalf("Invoke: want _ERROR output, got Go error: %v", err)
			}
			if out["_ERROR"] != "URL not valid" || out["result"] != nil {
				t.Fatalf("output = %#v, want rejected proxy", out)
			}
		})
	}
}

func TestInvokeInvalidProxyDoesNotLogCredentials(t *testing.T) {
	setupAllowAnyHost(t, false)
	for _, tc := range []struct {
		name  string
		proxy string
	}{
		{
			name:  "invalid port",
			proxy: "http://proxy-user:proxy-password@8.8.4.4:invalid/private-path?token=proxy-token",
		},
		{
			name:  "control character",
			proxy: "http://proxy-user:proxy-password@8.8.4.4/private-path\t?token=proxy-token",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := zap.L()
			core, logs := observer.New(zap.WarnLevel)
			zap.ReplaceGlobals(zap.New(core))
			t.Cleanup(func() { zap.ReplaceGlobals(previous) })

			c, err := NewInvokeComponent(map[string]any{
				"method": "GET",
				"url":    "http://8.8.8.8/api",
				"proxy":  tc.proxy,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, nil)
			if err != nil || out["_ERROR"] != "URL not valid" || out["result"] != nil {
				t.Fatalf("Invoke = %#v, %v; want rejected proxy", out, err)
			}
			if logs.Len() != 1 {
				t.Fatalf("log count = %d, want one proxy validation warning", logs.Len())
			}
			for _, entry := range logs.All() {
				for _, secret := range []string{"proxy-user", "proxy-password", "private-path", "proxy-token"} {
					if strings.Contains(entry.Message, secret) || strings.Contains(fmt.Sprint(entry.ContextMap()), secret) {
						t.Errorf("log contains proxy credentials or path/query: %#v", entry)
					}
				}
			}
		})
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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

func TestInvoke_ParameterTypes(t *testing.T) {
	setupAllowAnyHost(t, true)
	for _, tc := range []struct {
		name, kind string
		value      any
		invalid    bool
	}{
		{"legacy", "", `{"nested":{"items":[1,true]}}`, false},
		{"string", "string", `{"nested":{"items":[1,true]}}`, false},
		{"object", "object", `{"nested":{"items":[1,true]}}`, false},
		{"native", "object", map[string]any{"nested": map[string]any{"items": []any{float64(1), true}}}, false},
		{"invalid", "object", `{`, true},
		{"array", "object", `[]`, true},
		{"null", "object", `null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ref := range []string{"", "source@result"} {
				var captured any
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					captured = body["input"]
					w.Write([]byte(`ok`))
				}))
				c, err := NewInvokeComponent(map[string]any{"url": srv.URL, "method": "POST", "variables": []any{map[string]any{"key": "input", "type": tc.kind, "value": tc.value, "ref": ref}}})
				if err != nil {
					t.Fatal(err)
				}
				_, err = c.Invoke(t.Context(), nil, map[string]any{"source@result": tc.value})
				srv.Close()
				if tc.invalid {
					if err == nil || !strings.Contains(err.Error(), `parameter "input" must be a valid JSON object`) {
						t.Fatalf("error = %v", err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				expected := tc.value
				if tc.kind == "object" {
					expected = map[string]any{"nested": map[string]any{"items": []any{float64(1), true}}}
				}
				if !reflect.DeepEqual(captured, expected) {
					t.Fatalf("wire value = %#v, want %#v", captured, expected)
				}
			}
		})
	}
}

func TestInvoke_ExplicitBodyIgnoresParameterTypes(t *testing.T) {
	setupAllowAnyHost(t, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"explicit":true}` {
			t.Errorf("body = %s", body)
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c, err := NewInvokeComponent(map[string]any{
		"url": srv.URL, "method": "PUT", "body": `{"explicit":true}`,
		"variables": []any{map[string]any{"key": "ignored", "type": "object", "value": "invalid JSON"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Invoke(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
}
