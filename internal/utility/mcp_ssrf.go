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
	"net/url"
	"ragflow/internal/common"
	"strings"
)

// AssertMCPURLSafe validates an MCP server URL. It delegates to
// common.AssertURLSafe by default. When the deployment sets
// RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS=true (a deployment-level operator decision),
// the public-routability requirement is relaxed so self-hosted MCP servers on
// private/docker networks work — scheme validation, DNS resolution, and
// connection pinning to the resolved IP still apply.
//
// Use this variant for operator-configured URLs (MCP server registrations and
// direct tool calls). For URLs advertised by an SSE endpoint event, use
// AssertMCPURLSafeSameOrigin so a malicious server cannot redirect POSTs to an
// unrelated internal host.
func AssertMCPURLSafe(rawURL string) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil || !mcpPrivateHostsAllowed() {
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
// accepted only when RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS is enabled AND the
// advertised host matches originURL's host, so the server cannot bounce POSTs
// to an unrelated internal target.
func AssertMCPURLSafeSameOrigin(rawURL, originURL string) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil {
		return hostname, resolvedIP, nil
	}
	if !mcpPrivateHostsAllowed() {
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
	if parsed.Hostname() != origin.Hostname() {
		return "", "", err
	}
	return AssertMCPURLSafe(rawURL)
}

// mcpPrivateHostsAllowed reports whether RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS is
// enabled (true/1/yes, case-insensitive).
func mcpPrivateHostsAllowed() bool {
	v := strings.ToLower(strings.TrimSpace(common.GetEnv(common.EnvMCPAllowPrivateHosts)))
	return v == "true" || v == "1" || v == "yes"
}
