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

package mcpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"strings"
)

// effectivePort returns the explicit port, or the scheme's default port when
// omitted, so http://h and http://h:80 are treated as the same origin.
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	}
	return ""
}

// sameOrigin reports whether two URLs share scheme, port, and host. DNS
// hostnames are case-insensitive, so the hostname is compared with
// strings.EqualFold; scheme is already lowercased by url.Parse and compared
// case-insensitively for safety; port is normalized so the default-port form
// (http://h) and the explicit form (http://h:80) match.
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		effectivePort(a) == effectivePort(b) &&
		strings.EqualFold(a.Hostname(), b.Hostname())
}

// AssertMCPURLSafe validates an MCP server URL. It delegates to
// common.AssertURLSafe by default. When allowPrivateHosts is true (a
// deployment-level operator decision, sourced from
// ALLOW_ANY_HOST at the call site), the
// public-routability requirement is relaxed so self-hosted MCP servers on
// private/docker networks work — scheme validation, DNS resolution, and
// connection pinning to the resolved IP still apply.
//
// Use this variant for operator-configured URLs (MCP server registrations and
// direct tool calls). For URLs advertised by an SSE endpoint event, use
// AssertMCPURLSafeSameOrigin so a malicious server cannot redirect POSTs to an
// unrelated internal host.
func AssertMCPURLSafe(rawURL string, allowPrivateHosts bool) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil || !allowPrivateHosts {
		return hostname, resolvedIP, err
	}
	strictErr := err
	if serr := common.AssertURLSchemeSafe(rawURL); serr != nil {
		return "", "", strictErr
	}
	parsed, perr := url.Parse(strings.TrimSpace(rawURL))
	if perr != nil {
		return "", "", strictErr
	}
	host := parsed.Hostname()
	addresses, lerr := common.LookupHost(host)
	if lerr != nil || len(addresses) == 0 {
		return "", "", strictErr
	}
	return host, addresses[0], nil
}

// AssertMCPURLSafeSameOrigin validates a server-advertised URL (e.g., the POST
// URL from an SSE endpoint event). The URL must share the origin (scheme,
// port, and host) with originURL — checked before any success path — and
// private URLs additionally require allowPrivateHosts, so the server cannot
// bounce POSTs to a cleartext downgrade or an unrelated target.
func AssertMCPURLSafeSameOrigin(rawURL, originURL string, allowPrivateHosts bool) (hostname, resolvedIP string, err error) {
	// The advertised URL must keep the full origin — scheme, host and
	// effective port — before any success path, so a server cannot bounce
	// POSTs to a cleartext downgrade or a different public/internal target.
	origin, perr := url.Parse(strings.TrimSpace(originURL))
	if perr != nil || origin.Hostname() == "" {
		return "", "", fmt.Errorf("invalid MCP origin URL %q", originURL)
	}
	parsed, perr := url.Parse(strings.TrimSpace(rawURL))
	if perr != nil {
		return "", "", perr
	}
	if !sameOrigin(parsed, origin) {
		return "", "", fmt.Errorf("MCP advertised URL %q does not match origin %q", rawURL, originURL)
	}
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil {
		return hostname, resolvedIP, nil
	}
	if !allowPrivateHosts {
		return "", "", err
	}
	return AssertMCPURLSafe(rawURL, allowPrivateHosts)
}

// restrictClientRedirects returns a copy of a caller-supplied HTTP client
// whose redirect policy keeps credential headers on the validated origin:
// it rejects HTTPS downgrades and any redirect target that is not the same
// origin as the first request. The default PinnedHTTPClient applies the same
// rules; a supplied client must not quietly bypass them.
func restrictClientRedirects(client *http.Client, allowPrivateHosts bool) *http.Client {
	restricted := *client
	inner := restricted.CheckRedirect
	restricted.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
			return fmt.Errorf("refusing HTTPS to HTTP redirect to %q", req.URL)
		}
		if _, _, err := AssertMCPURLSafeSameOrigin(req.URL.String(), via[0].URL.String(), allowPrivateHosts); err != nil {
			return err
		}
		if inner != nil {
			return inner(req, via)
		}
		return nil
	}
	return &restricted
}
