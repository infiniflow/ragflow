//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ragflow/internal/utility"
)

// connectorAssertURLSafe is the SSRF guard shared by all connectors. It is an
// indirection over utility.AssertURLSafe so unit tests can substitute a stub
// without touching the shared utility guard (same pattern as sitemap.go and
// azure_blob.go).
var connectorAssertURLSafe = utility.AssertURLSafe

// connectorAllowLoopbackForTest lets unit tests exercise the real HTTP path
// against httptest servers bound to loopback. Production code keeps it false so
// loopback/private endpoints stay blocked.
var connectorAllowLoopbackForTest bool

// assertConnectorURLSafe validates rawURL with the shared strict SSRF guard and
// returns the hostname plus the first validated IP so callers can pin DNS for
// the actual dial, preventing DNS rebinding between validation and the
// connection.
//
// When connectorAllowLoopbackForTest is set, URLs whose hostname resolves only
// to loopback addresses are allowed; anything else falls through to the strict
// guard, so a mixed private address is still rejected.
func assertConnectorURLSafe(rawURL string) (string, net.IP, error) {
	if connectorAllowLoopbackForTest {
		parsed, err := url.Parse(strings.TrimSpace(rawURL))
		if err != nil || parsed.Hostname() == "" {
			return "", nil, fmt.Errorf("URL is missing a host.")
		}
		if scheme := strings.ToLower(parsed.Scheme); scheme == "http" || scheme == "https" {
			if host, ip, ok := loopbackTestAllow(parsed.Hostname()); ok {
				return host, ip, nil
			}
		}
		// Not allowed by the test hook — fall through to the strict guard.
	}
	hostname, ipStr, err := connectorAssertURLSafe(rawURL)
	if err != nil {
		return "", nil, err
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", nil, fmt.Errorf("Could not parse validated address %q for hostname %q", ipStr, hostname)
	}
	return hostname, ip, nil
}

// loopbackTestAllow reports whether hostname is a loopback-only target that the
// test hook may allow. Literal loopback IPs are allowed directly; localhost
// names must resolve (via the shared resolver seam) to loopback addresses only.
func loopbackTestAllow(hostname string) (string, net.IP, bool) {
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() {
			return hostname, ip, true
		}
		return "", nil, false
	}
	lower := strings.ToLower(hostname)
	if lower != "localhost" && !strings.HasSuffix(lower, ".localhost") {
		return "", nil, false
	}
	addrs, err := utility.LookupHost(hostname)
	if err != nil {
		return "", nil, false
	}
	allLoopback := true
	var first net.IP
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil || !ip.IsLoopback() {
			allLoopback = false
			break
		}
		if first == nil {
			first = ip
		}
	}
	if allLoopback && first != nil {
		return hostname, first, true
	}
	return "", nil, false
}

// assertConnectorURLSafeHTTPS validates rawURL with the strict SSRF guard and
// additionally requires HTTPS. Plain-HTTP loopback targets are still permitted
// under the test hook (httptest servers are plain HTTP); anything else must use
// TLS so credentials in request headers are never sent in the clear.
func assertConnectorURLSafeHTTPS(rawURL string) (string, net.IP, error) {
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		if scheme := strings.ToLower(parsed.Scheme); scheme != "https" {
			if !connectorAllowLoopbackForTest {
				return "", nil, fmt.Errorf("URL must use HTTPS (got %q)", scheme)
			}
			if _, _, ok := loopbackTestAllow(parsed.Hostname()); !ok {
				return "", nil, fmt.Errorf("URL must use HTTPS (got %q)", scheme)
			}
		}
	}
	return assertConnectorURLSafe(rawURL)
}

// validateConnectorURL is the config-time SSRF check used by connector
// Validate/New paths.
func validateConnectorURL(rawURL string) error {
	// localhost is never a legitimate connector base URL, even under the test
	// hook (which only relaxes loopback IPs such as httptest's 127.0.0.1).
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		host := strings.ToLower(parsed.Hostname())
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return &ConnectorValidationError{Message: fmt.Sprintf("URL hostname %q is not allowed (localhost is blocked).", parsed.Hostname())}
		}
	}
	if _, _, err := assertConnectorURLSafe(rawURL); err != nil {
		return &ConnectorValidationError{Message: err.Error()}
	}
	return nil
}

// basicAuthHeader returns the value for an HTTP Basic Authorization header.
func basicAuthHeader(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// ---------------------------------------------------------------------------
// Shared request path
// ---------------------------------------------------------------------------

// connectorRequestHop is the per-hop request state carried by connectorRequest.
type connectorRequestHop struct {
	Method  string
	URL     string
	Body    []byte
	Headers map[string]string
}

// connectorRequestOptions configures connectorRequest. Zero values use safe
// defaults: Validate falls back to assertConnectorURLSafe and MaxRedirects to 10.
type connectorRequestOptions struct {
	Method       string
	RawURL       string
	Body         []byte
	Headers      map[string]string
	Timeout      time.Duration
	MaxRedirects int
	// Base, when non-nil, provides the starting transport so custom TLS/CA
	// settings are preserved; otherwise a default pinned transport is used.
	Base *http.Client
	// Validate checks a hop URL for SSRF and returns the hostname plus the IP
	// to pin for that hop.
	Validate func(rawURL string) (string, net.IP, error)
	// Prepare customizes each hop's request before it is sent (basic auth,
	// query parameters, ...).
	Prepare func(req *http.Request, hop connectorRequestHop) error
	// CheckRedirect, when non-nil, is invoked before following to the next hop
	// (after the next hop's SSRF validation) and may enforce a same-origin
	// policy or re-apply credentials.
	CheckRedirect func(req *http.Request, via []*http.Request) error
	// RetryStatus, when non-nil, decides whether a non-redirect response should
	// be retried on the same hop URL; it returns the wait duration and whether
	// to retry. Used for REST API 429 handling.
	RetryStatus func(resp *http.Response, attempt int) (time.Duration, bool)
	// NextHop adapts the hop state (method/body/headers/URL) for the hop after
	// a redirect; it runs after connectorRequest's default GET downgrade and
	// cross-origin credential stripping.
	NextHop func(nextURL string, status int, hop connectorRequestHop) connectorRequestHop
}

// connectorUnsafeErr returns the underlying error when err was produced by the
// SSRF guard (connectorUnsafeURLError), so callers can surface the original
// message; otherwise it returns err unchanged.
func connectorUnsafeErr(err error) error {
	var unsafe *connectorUnsafeURLError
	if errors.As(err, &unsafe) {
		return unsafe.Err
	}
	return err
}

// connectorUnsafeURLError marks a hop URL rejected by the SSRF guard so
// callers can tell validation failures apart from transport errors.
type connectorUnsafeURLError struct{ Err error }

func (e *connectorUnsafeURLError) Error() string { return e.Err.Error() }
func (e *connectorUnsafeURLError) Unwrap() error { return e.Err }

// connectorRequest is the single HTTP send path shared by every connector. It
// follows redirects manually so that each hop is SSRF-validated and DNS-pinned
// before a connection is made, closing the TOCTOU window that an automatic
// redirect policy would leave open (the redirect target would otherwise be
// validated once and resolved again at dial time).
//
// Per-hop behavior:
//   - Validate checks the hop URL and returns the hostname + IP to pin.
//   - The hop is sent through a transport that dials exactly the validated IP,
//     so a DNS change after validation cannot rebind the connection.
//   - Sensitive headers (Authorization etc.) are dropped when a redirect
//     crosses to a different netloc, mirroring http.Client's default.
//   - 301/302/303 downgrade the next hop to GET with an empty body.
//
// Connector-specific behavior is expressed through the Prepare, CheckRedirect,
// RetryStatus, and NextHop hooks.
func connectorRequest(ctx context.Context, opts connectorRequestOptions) (*http.Response, error) {
	if opts.Validate == nil {
		opts.Validate = assertConnectorURLSafe
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = 10
	}
	hop := connectorRequestHop{Method: opts.Method, URL: opts.RawURL, Body: opts.Body, Headers: opts.Headers}
	previousNetloc := connectorNetloc(hop.URL)
	var via []*http.Request
	for redirects := 0; ; redirects++ {
		hostname, pinIP, err := opts.Validate(hop.URL)
		if err != nil {
			return nil, &connectorUnsafeURLError{Err: err}
		}
		var reader io.Reader
		if hop.Body != nil {
			reader = bytes.NewReader(hop.Body)
		}
		req, err := http.NewRequestWithContext(ctx, hop.Method, hop.URL, reader)
		if err != nil {
			return nil, err
		}
		for k, v := range hop.Headers {
			req.Header.Set(k, v)
		}
		if opts.Prepare != nil {
			if err := opts.Prepare(req, hop); err != nil {
				return nil, err
			}
		}
		if redirects > 0 && opts.CheckRedirect != nil {
			if err := opts.CheckRedirect(req, via); err != nil {
				return nil, err
			}
		}
		transport := newConnectorPinnedTransport(opts.Base, hostname, pinIP, opts.Timeout)
		client := &http.Client{
			Transport: transport,
			Timeout:   opts.Timeout,
			// Redirects are followed manually by this loop; each hop gets its
			// own validated, pinned transport.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		var resp *http.Response
		for attempt := 1; ; attempt++ {
			resp, err = client.Do(req)
			if err != nil {
				transport.CloseIdleConnections()
				return nil, err
			}
			if !connectorIsRedirect(resp.StatusCode) && opts.RetryStatus != nil {
				if wait, retry := opts.RetryStatus(resp, attempt); retry {
					resp.Body.Close()
					select {
					case <-ctx.Done():
						transport.CloseIdleConnections()
						return nil, ctx.Err()
					case <-time.After(wait):
					}
					continue
				}
			}
			break
		}
		if !connectorIsRedirect(resp.StatusCode) {
			resp.Body = &connectorCloseIdleBody{body: resp.Body, transport: transport}
			return resp, nil
		}
		location := resp.Header.Get("Location")
		resp.Body.Close()
		transport.CloseIdleConnections()
		if location == "" {
			return nil, fmt.Errorf("redirect with empty Location header")
		}
		if redirects >= opts.MaxRedirects {
			return nil, fmt.Errorf("request stopped after %d redirects", opts.MaxRedirects)
		}
		nextURL, err := connectorResolveURL(hop.URL, location)
		if err != nil {
			return nil, err
		}
		nextNetloc := connectorNetloc(nextURL)
		if nextNetloc != "" && nextNetloc != previousNetloc {
			hop.Headers = connectorStripAuthHeaders(hop.Headers)
			// 307/308 preserve the request method and body; never forward a
			// request body (which may carry credentials, e.g. a Seafile or
			// Moodle token exchange) to a different origin.
			if len(hop.Body) > 0 && (resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusPermanentRedirect) {
				return nil, fmt.Errorf("redirect to a different origin is not allowed for a request with a body")
			}
		}
		previousNetloc = nextNetloc
		if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
			hop.Method = http.MethodGet
			hop.Body = nil
		}
		if opts.NextHop != nil {
			hop = opts.NextHop(nextURL, resp.StatusCode, hop)
		} else {
			hop.URL = nextURL
		}
		via = append(via, req)
	}
}

// newConnectorPinnedTransport builds a transport that dials exactly pinIP for
// every connection, so the hop is connected to the address validated by the
// SSRF guard regardless of later DNS changes. When base provides an
// *http.Transport (custom TLS/CA), it is cloned and reused; otherwise a default
// transport is used. Environment proxies are deliberately ignored: HTTP_PROXY /
// HTTPS_PROXY would route the connection through a proxy host instead of pinIP.
func newConnectorPinnedTransport(base *http.Client, hostname string, pinIP net.IP, timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			port = "443"
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(pinIP.String(), port))
	}
	if base != nil {
		if t, ok := base.Transport.(*http.Transport); ok {
			clone := t.Clone()
			clone.DialContext = dial
			if clone.TLSClientConfig == nil {
				clone.TLSClientConfig = &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}
			} else {
				clone.TLSClientConfig = clone.TLSClientConfig.Clone()
				if clone.TLSClientConfig.ServerName == "" {
					clone.TLSClientConfig.ServerName = hostname
				}
			}
			return clone
		}
	}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dial,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     false,
		TLSClientConfig:       &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12},
	}
}

// connectorCloseIdleBody closes the underlying response body and then releases
// the per-hop pinned transport's idle connections, so per-hop transports do not
// leak keep-alive sockets during long syncs.
type connectorCloseIdleBody struct {
	body      io.ReadCloser
	transport *http.Transport
}

func (b *connectorCloseIdleBody) Read(p []byte) (int, error) { return b.body.Read(p) }
func (b *connectorCloseIdleBody) Close() error {
	err := b.body.Close()
	b.transport.CloseIdleConnections()
	return err
}

// connectorAuthSensitiveHeaders lists headers that must not be forwarded to a
// different origin on redirect.
var connectorAuthSensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"apikey":              {},
	"api-key":             {},
	"x-api-key":           {},
	"x-auth-token":        {},
	"private-token":       {}, // GitLab credential header
}

func connectorStripAuthHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if _, sensitive := connectorAuthSensitiveHeaders[strings.ToLower(k)]; sensitive {
			continue
		}
		out[k] = v
	}
	return out
}

func connectorNetloc(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func connectorResolveURL(base, location string) (string, error) {
	baseParsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return baseParsed.ResolveReference(ref).String(), nil
}

func connectorIsRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}
