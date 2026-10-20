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
// RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS at the call site), the
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
// URL from an SSE endpoint event). Public URLs are accepted. Private URLs are
// accepted only when allowPrivateHosts is true AND the advertised URL shares the
// same origin (scheme, port, and host) as originURL, so the server cannot bounce
// POSTs to another service on the same host but a different port or scheme.
func AssertMCPURLSafeSameOrigin(rawURL, originURL string, allowPrivateHosts bool) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil {
		return hostname, resolvedIP, nil
	}
	if !allowPrivateHosts {
		return "", "", err
	}
	origin, perr := url.Parse(strings.TrimSpace(originURL))
	if perr != nil || origin.Hostname() == "" {
		return "", "", err
	}
	parsed, perr := url.Parse(strings.TrimSpace(rawURL))
	if perr != nil {
		return "", "", err
	}
	if !sameOrigin(parsed, origin) {
		return "", "", err
	}
	return AssertMCPURLSafe(rawURL, allowPrivateHosts)
}
