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

package utility

import (
	"fmt"
	"net/url"
	"os"
	"ragflow/internal/common"
	"strings"
)

// privateMCPHosts returns the explicit hostname allowlist configured by the
// operator. RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS may be "true"/"1"/"yes" (allow
// all private hosts) or a comma-separated list of hostnames. The separate
// RAGFLOW_ALLOWED_PRIVATE_HOSTS variable is also accepted as a hostname list.
func privateMCPHosts() (all bool, hosts map[string]struct{}) {
	raw := strings.TrimSpace(os.Getenv(common.EnvMCPAllowPrivateHosts))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("RAGFLOW_ALLOWED_PRIVATE_HOSTS"))
	}
	if raw == "" {
		return false, nil
	}
	lower := strings.ToLower(raw)
	if lower == "true" || lower == "1" || lower == "yes" {
		return true, nil
	}
	if lower == "false" || lower == "0" || lower == "no" {
		return false, nil
	}
	parts := strings.Split(raw, ",")
	out := make(map[string]struct{}, len(parts))
	for _, h := range parts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			out[h] = struct{}{}
		}
	}
	return false, out
}

func isPrivateMCPHostAllowed(hostname string) bool {
	all, hosts := privateMCPHosts()
	if all {
		return true
	}
	if hosts == nil {
		return false
	}
	_, ok := hosts[strings.ToLower(strings.TrimSpace(hostname))]
	return ok
}

// AssertMCPURLSafe validates an MCP server URL. It delegates to
// common.AssertURLSafe by default. When the deployment enables private MCP
// hosts (a deployment-level operator decision), the public-routability
// requirement is relaxed for allowlisted self-hosted MCP servers on private/
// docker networks — scheme validation, DNS resolution, and connection pinning
// to the resolved IP still apply.
//
// Use this variant for operator-configured URLs (MCP server registrations and
// direct tool calls). For URLs advertised by an SSE endpoint event, use
// AssertMCPURLSafeSameOrigin so a malicious server cannot redirect POSTs to an
// unrelated internal host.
func AssertMCPURLSafe(rawURL string) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, strictErr := common.AssertURLSafe(rawURL)
	if strictErr == nil {
		return hostname, resolvedIP, nil
	}
	if serr := common.AssertURLSchemeSafe(rawURL); serr != nil {
		return "", "", strictErr
	}
	parsed, perr := url.Parse(strings.TrimSpace(rawURL))
	if perr != nil {
		return "", "", strictErr
	}
	host := parsed.Hostname()
	if !isPrivateMCPHostAllowed(host) {
		return "", "", strictErr
	}
	addresses, lerr := common.LookupHost(host)
	if lerr != nil || len(addresses) == 0 {
		return "", "", strictErr
	}
	return host, addresses[0], nil
}

// AssertMCPURLSafeSameOrigin validates a server-advertised URL (e.g., the POST
// URL from an SSE endpoint event). Public URLs are accepted. Private URLs are
// accepted only when RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS is enabled AND the
// advertised host matches originURL's host, so the server cannot bounce POSTs
// to an unrelated internal target.
func AssertMCPURLSafeSameOrigin(rawURL, originURL string) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil {
		return hostname, resolvedIP, nil
	}
	origin, perr := url.Parse(strings.TrimSpace(originURL))
	if perr != nil || origin.Hostname() == "" {
		return "", "", err
	}
	parsed, perr := url.Parse(strings.TrimSpace(rawURL))
	if perr != nil {
		return "", "", err
	}
	if parsed.Hostname() != origin.Hostname() {
		return "", "", err
	}
	// Refuse an HTTPS -> HTTP downgrade advertised by the server. The
	// scheme must match so auth headers and cookies are not sent in the
	// clear after the client started on a secure origin.
	if parsed.Scheme != origin.Scheme {
		return "", "", fmt.Errorf("MCP advertised URL scheme %q does not match origin scheme %q", parsed.Scheme, origin.Scheme)
	}
	if !isPrivateMCPHostAllowed(parsed.Hostname()) {
		return "", "", err
	}
	return AssertMCPURLSafe(rawURL)
}
