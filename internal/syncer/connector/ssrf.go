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
	"context"
	"fmt"
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

// pinnedConnectorClient builds an HTTP client whose transport pins the first
// validated hop to its resolved IP and whose CheckRedirect re-validates every
// redirect target against the SSRF guard before following it.
//
// base, when non-nil, provides the starting transport (preserving custom TLS /
// CA settings); otherwise utility.PinnedHTTPClient is used.
func pinnedConnectorClient(rawURL string, base *http.Client, timeout time.Duration, redirectCheck func(*http.Request, []*http.Request) error) (*http.Client, error) {
	hostname, ip, err := assertConnectorURLSafe(rawURL)
	if err != nil {
		return nil, err
	}
	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if _, _, err := assertConnectorURLSafe(req.URL.String()); err != nil {
			return err
		}
		if redirectCheck != nil {
			return redirectCheck(req, via)
		}
		return nil
	}
	if base != nil {
		if t, ok := base.Transport.(*http.Transport); ok {
			clone := t.Clone()
			dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
			clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					port = "443"
				}
				if host == hostname {
					addr = net.JoinHostPort(ip.String(), port)
				}
				return dialer.DialContext(ctx, network, addr)
			}
			client := *base
			client.Transport = clone
			client.Timeout = timeout
			client.CheckRedirect = checkRedirect
			return &client, nil
		}
	}
	client := utility.PinnedHTTPClient(hostname, ip.String(), timeout)
	client.CheckRedirect = checkRedirect
	return client, nil
}
