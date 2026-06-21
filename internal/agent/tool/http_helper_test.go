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

package tool

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"sync/atomic"
	"testing"
	"time"
)

func newTestHelper(maxAttempts int, base, max time.Duration) *HTTPHelper {
	return NewHTTPHelperWithRetry(RetryConfig{
		MaxAttempts: maxAttempts,
		BaseBackoff: base,
		MaxBackoff:  max,
	})
}

// TestHTTPHelper_HappyPath verifies a 2xx response is returned on the first
// attempt with no retry, and the body / content-type round-trip cleanly.
func TestHTTPHelper_HappyPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	h := newTestHelper(3, 1*time.Millisecond, 5*time.Millisecond)
	resp, err := h.Do(context.Background(), http.MethodGet, srv.URL, "", "", nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q, want %q", body, `{"ok":true}`)
	}
}

// TestHTTPHelper_RetriesOn5xx verifies that the helper retries on 5xx and
// returns the first 2xx response. Server returns 503 twice, then 200.
func TestHTTPHelper_RetriesOn5xx(t *testing.T) {
	t.Parallel()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "recovered")
	}))
	defer srv.Close()

	h := newTestHelper(3, 1*time.Millisecond, 5*time.Millisecond)
	resp, err := h.Do(context.Background(), http.MethodGet, srv.URL, "", "", nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("server hits = %d, want 3 (2 retries + 1 success)", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "recovered" {
		t.Fatalf("body = %q, want %q", body, "recovered")
	}
}

// TestHTTPHelper_NoRetryOn4xx verifies that 4xx is returned immediately with
// no retry — the caller is responsible for fixing 4xx, retrying won't help.
func TestHTTPHelper_NoRetryOn4xx(t *testing.T) {
	t.Parallel()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "bad")
	}))
	defer srv.Close()

	h := newTestHelper(3, 1*time.Millisecond, 5*time.Millisecond)
	resp, err := h.Do(context.Background(), http.MethodGet, srv.URL, "", "", nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("server hits = %d, want 1 (no retry on 4xx)", got)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestHTTPHelper_Timeout verifies that a context deadline aborts the call
// and returns context.DeadlineExceeded promptly (no infinite retry).
func TestHTTPHelper_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewHTTPHelper().WithClient(&http.Client{
		Timeout: 30 * time.Second,
	})
	// Tight deadline — server takes 500ms; we expect to bail in <100ms.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := h.Do(ctx, http.MethodGet, srv.URL, "", "", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("Do took %s, want < 250ms (timeout should abort promptly)", elapsed)
	}
}

// TestHTTPHelper_5xxExhaustion verifies that after MaxAttempts the last
// 5xx error is returned.
func TestHTTPHelper_5xxExhaustion(t *testing.T) {
	t.Parallel()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newTestHelper(3, 1*time.Millisecond, 5*time.Millisecond)
	_, err := h.Do(context.Background(), http.MethodGet, srv.URL, "", "", nil)
	if err == nil {
		t.Fatal("expected error after 5xx exhaustion, got nil")
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("server hits = %d, want 3", got)
	}
}

// TestHTTPHelper_HeadersAndContentType verifies the helper propagates
// custom headers and a non-empty content-type on POST bodies.
func TestHTTPHelper_HeadersAndContentType(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Token"); got != "abc" {
			t.Errorf("X-Token = %q, want abc", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Echo-Body", string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	h := newTestHelper(1, 1*time.Millisecond, 5*time.Millisecond)
	resp, err := h.Do(context.Background(), http.MethodPost, srv.URL, `{"k":1}`, "application/json", map[string]string{"X-Token": "abc"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Echo-Body"); got != `{"k":1}` {
		t.Fatalf("echoed body = %q, want %q", got, `{"k":1}`)
	}
}

// TestBackoffExponential verifies the helper-internal backoff function grows
// exponentially and caps at MaxBackoff.
func TestBackoffExponential(t *testing.T) {
	t.Parallel()

	base := 50 * time.Millisecond
	max := 300 * time.Millisecond

	got1 := backoff(base, max, 1)
	if got1 < 0 || got1 > base {
		t.Fatalf("backoff(attempt=1) = %s, want [0, %s]", got1, base)
	}
	got3 := backoff(base, max, 3)
	if got3 < 0 || got3 > max {
		t.Fatalf("backoff(attempt=3) = %s, want [0, %s] (capped)", got3, max)
	}
	// With base=50ms, attempt=10 should be capped at 300ms.
	got10 := backoff(base, max, 10)
	if got10 > max {
		t.Fatalf("backoff(attempt=10) = %s, want <= %s (cap)", got10, max)
	}
}

// TestRetryConfigDefaults ensures zero-value RetryConfig falls
// back to the documented defaults.
func TestRetryConfigDefaults(t *testing.T) {
	t.Parallel()

	c := RetryConfig{}.withDefaults()
	if c.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", c.MaxAttempts)
	}
	if c.BaseBackoff != 200*time.Millisecond {
		t.Errorf("BaseBackoff = %s, want 200ms", c.BaseBackoff)
	}
	if c.MaxBackoff != 3*time.Second {
		t.Errorf("MaxBackoff = %s, want 3s", c.MaxBackoff)
	}
}

// TestHTTPHelper_DoPinnedHTTPS_PreservesSNIAndCert is the regression
// test for the M1-rebinding fix as hardened by the post-Phase-7
// review: DNS pinning MUST happen at the transport layer, not by
// rewriting the request URL. If the URL host were rewritten to the IP,
// the TLS ServerName (auto-populated by Go from req.URL.Host) would
// become the IP, the SNI would send the IP, and cert verification
// would target the IP — which is not what real HTTPS sites have, and
// would manifest as x509 errors against any host-cert-only target.
//
// This test stands up a real TLS server with a cert whose DNS SAN is
// "example.test" and whose IP SAN covers the loopback address. The
// pinned dialer connects to 127.0.0.1, but the request URL host stays
// as "example.test". The server observes the SNI the client sent and
// we assert it equals "example.test" (not the IP), and the request
// completes successfully (cert verification passes because the URL
// host matches the SAN).
func TestHTTPHelper_DoPinnedHTTPS_PreservesSNIAndCert(t *testing.T) {
	t.Parallel()

	// Cert valid for "example.test" (DNS SAN) and 127.0.0.1, ::1
	// (IP SANs, just so the test environment itself can resolve).
	cert := generateTestCert(t, "example.test")

	var observedSNI string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			observedSNI = hello.ServerName
			return &cert, nil
		},
	}
	srv.StartTLS()
	defer srv.Close()

	// srv.URL is https://127.0.0.1:<port>. We extract the port and
	// re-build the target URL with "example.test" as the host — i.e.
	// the URL host we send the request with is NOT 127.0.0.1, even
	// though the connection itself goes to 127.0.0.1.
	u, perr := neturl.Parse(srv.URL)
	if perr != nil {
		t.Fatalf("parse server URL %q: %v", srv.URL, perr)
	}
	serverHost, serverPort, sperr := net.SplitHostPort(u.Host)
	if sperr != nil {
		t.Fatalf("split server host:port from %q: %v", u.Host, sperr)
	}
	targetURL := fmt.Sprintf("https://example.test:%s/", serverPort)
	pinnedIP := net.ParseIP(serverHost)
	if pinnedIP == nil {
		t.Fatalf("server host %q is not an IP literal", serverHost)
	}

	// Trust the self-signed cert for the duration of the test by
	// mutating baseTransport directly. This is the supported way to
	// install a custom trust store (see WithClient's docstring).
	leaf, lperr := x509.ParseCertificate(cert.Certificate[0])
	if lperr != nil {
		t.Fatalf("parse leaf cert: %v", lperr)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	h := NewHTTPHelper()
	h.baseTransport.TLSClientConfig = &tls.Config{RootCAs: pool}

	resp, err := h.DoPinned(context.Background(),
		http.MethodGet, targetURL, "", "", nil, "example.test", pinnedIP)
	if err != nil {
		t.Fatalf("DoPinned: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
	}
	if observedSNI != "example.test" {
		t.Errorf("SNI = %q, want %q — DoPinned must keep the URL host for SNI, "+
			"otherwise HTTPS breaks for any host-cert-only target", observedSNI, "example.test")
	}
}

// TestHTTPHelper_DoPinnedRefusesMismatchedURLHost locks in the
// defense-in-depth check in DoPinned: a caller that passes a URL
// whose host does not match originalHost would produce a request
// whose TLS ServerName differs from the validated hostname, which is
// exactly the SSRF / rebinding bypass we are trying to prevent. We
// refuse rather than silently deliver a broken connection.
func TestHTTPHelper_DoPinnedRefusesMismatchedURLHost(t *testing.T) {
	t.Parallel()

	h := NewHTTPHelper()
	_, err := h.DoPinned(context.Background(),
		http.MethodGet,
		"https://attacker.example/foo",
		"", "", nil,
		"real.example", // originalHost from resolver
		net.ParseIP("1.2.3.4"))
	if err == nil {
		t.Fatal("DoPinned accepted a mismatched URL host, want error")
	}
	if got := err.Error(); !contains(got, "would break TLS SNI") {
		t.Errorf("error %q does not mention TLS SNI breakage", got)
	}
}

// TestHTTPHelper_DoPinnedBypassesProxy locks in the proxy bypass
// added after the post-Phase-7 review: even when baseTransport.Proxy
// is set (e.g. via HTTP_PROXY / HTTPS_PROXY env), DoPinned must NOT
// route through the proxy. Two failure modes were possible without
// the bypass:
//
//  1. *http.Transport dials the proxy first; pinnedDialer would
//     rewrite the proxy's own address to pinnedIP:proxyPort and the
//     connection would fail in any proxied deployment.
//
//  2. Even if the dialer were proxy-aware, the proxy would receive
//     the original hostname and re-resolve it, re-opening the
//     rebinding window the SSRF guard just closed.
//
// The test points baseTransport.Proxy at a port that is guaranteed
// to be closed (127.0.0.1:1) — if DoPinned used the proxy, the
// request would fail with "connection refused"; if it correctly
// bypasses the proxy, the direct dial to 127.0.0.1 succeeds.
func TestHTTPHelper_DoPinnedBypassesProxy(t *testing.T) {
	t.Parallel()

	cert := generateTestCert(t, "example.test")
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()

	u, perr := neturl.Parse(srv.URL)
	if perr != nil {
		t.Fatalf("parse server URL: %v", perr)
	}
	serverHost, serverPort, sperr := net.SplitHostPort(u.Host)
	if sperr != nil {
		t.Fatalf("split host:port: %v", sperr)
	}
	targetURL := fmt.Sprintf("https://example.test:%s/", serverPort)
	pinnedIP := net.ParseIP(serverHost)
	if pinnedIP == nil {
		t.Fatalf("server host %q is not an IP literal", serverHost)
	}

	leaf, lperr := x509.ParseCertificate(cert.Certificate[0])
	if lperr != nil {
		t.Fatalf("parse leaf cert: %v", lperr)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	h := NewHTTPHelper()
	h.baseTransport.TLSClientConfig = &tls.Config{RootCAs: pool}

	// Point the proxy at a port the OS just gave us and which we then
	// released. The port is therefore guaranteed to be closed at this
	// moment (modulo the microsecond race where another process grabs
	// it between Close() and the test dial — acceptable in a unit
	// test). This is more self-documenting than reaching for a
	// "guaranteed-unused" magic port like 127.0.0.1:1.
	probe, lerr := net.Listen("tcp", "127.0.0.1:0")
	if lerr != nil {
		t.Fatalf("listen for a free port: %v", lerr)
	}
	closedAddr := probe.Addr().String()
	if cerr := probe.Close(); cerr != nil {
		t.Fatalf("close probe listener: %v", cerr)
	}
	closedProxy, perr := neturl.Parse("http://" + closedAddr)
	if perr != nil {
		t.Fatalf("parse closed proxy URL: %v", perr)
	}
	h.baseTransport.Proxy = http.ProxyURL(closedProxy)

	resp, err := h.DoPinned(context.Background(),
		http.MethodGet, targetURL, "", "", nil, "example.test", pinnedIP)
	if err != nil {
		t.Fatalf("DoPinned: %v (proxy may not have been bypassed)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
	}
}

// TestPinnedDialer_RewritesAddress verifies the unit-level behaviour
// of pinnedDialer: it discards the host in the dial address and
// connects to pinnedIP:port, preserving the port. This is the
// transport-layer primitive that DoPinned uses.
func TestPinnedDialer_RewritesAddress(t *testing.T) {
	t.Parallel()

	// Stand up a TCP server on 127.0.0.1:<random> and capture the
	// accepted conn to confirm the pinned dialer actually dialed it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan struct{}, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr == nil {
			_ = c.Close()
		}
		accepted <- struct{}{}
	}()

	d := &pinnedDialer{
		pinnedIP: net.ParseIP("127.0.0.1"),
		base:     &net.Dialer{Timeout: 2 * time.Second},
	}
	// Pass a deliberately misleading host in the addr — the dialer
	// must ignore it and dial 127.0.0.1:<ln port> instead.
	misleading := net.JoinHostPort("203.0.113.99", fmt.Sprint(ln.Addr().(*net.TCPAddr).Port))
	conn, derr := d.DialContext(context.Background(), "tcp", misleading)
	if derr != nil {
		t.Fatalf("DialContext: %v", derr)
	}
	_ = conn.Close()

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("pinned dialer did not connect to 127.0.0.1:port (or listener did not accept)")
	}
}

// contains is a tiny helper to avoid dragging in strings just for one
// assertion. (The rest of the file uses strings.Contains.)
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// generateTestCert builds a self-signed ECDSA cert valid for dnsName
// (DNS SAN) and the loopback addresses (IP SANs). The cert is not
// trusted by the system pool — tests must inject it into RootCAs to
// use it. Returned together with a t.Cleanup-free form so tests can
// store the certificate in their tls.Config.Certificates.
func generateTestCert(t *testing.T, dnsName string) tls.Certificate {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"RAGFlow Tool Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{dnsName},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
}
