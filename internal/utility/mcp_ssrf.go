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

// AssertMCPURLSafe validates an operator-configured MCP server URL. It
// delegates to common.AssertURLSafe by default. When the URL's host is listed
// in the RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS allowlist (comma-separated hostnames
// or IPs, a deployment-level operator decision), the public-routability
// requirement is relaxed so self-hosted MCP servers on private/docker
// networks work — scheme validation, DNS resolution, and connection pinning
// to the resolved IP still apply. The allowlist does not weaken the guard
// for any non-MCP caller and does not apply to hosts not on the list.
func AssertMCPURLSafe(rawURL string) (hostname, resolvedIP string, err error) {
	hostname, resolvedIP, err = common.AssertURLSafe(rawURL)
	if err == nil || !mcpPrivateHostAllowed(rawURL) {
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

// mcpPrivateHostAllowed reports whether rawURL's host is on the
// RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS allowlist (exact, case-insensitive
// hostname or literal-IP match).
func mcpPrivateHostAllowed(rawURL string) bool {
	list := strings.TrimSpace(common.GetEnv(common.EnvMCPAllowPrivateHosts))
	if list == "" {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	for _, entry := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(entry), host) {
			return true
		}
	}
	return false
}
