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
	"ragflow/internal/common"
	"testing"
)

func stubLookupHost(t *testing.T, ips []string) {
	t.Helper()
	original := common.LookupHost
	common.LookupHost = func(string) ([]string, error) { return ips, nil }
	t.Cleanup(func() { common.LookupHost = original })
}

func TestAssertMCPURLSafeAllowlistedPrivateHost(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	t.Setenv(common.EnvMCPAllowPrivateHosts, "navigo-mcp")

	host, ip, err := AssertMCPURLSafe("http://navigo-mcp:8765/mcp")
	if err != nil {
		t.Fatalf("expected allowlisted private MCP host to pass, got %v", err)
	}
	if host != "navigo-mcp" || ip != "172.18.0.5" {
		t.Fatalf("expected pinning to resolved IP, got host=%q ip=%q", host, ip)
	}
}

func TestAssertMCPURLSafeRejectsUnlistedPrivateHost(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	t.Setenv(common.EnvMCPAllowPrivateHosts, "navigo-mcp")

	if _, _, err := AssertMCPURLSafe("http://other-internal:8765/mcp"); err == nil {
		t.Fatal("expected non-allowlisted private host to be rejected")
	}
}

func TestAssertMCPURLSafeAllowlistDisabledByDefault(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})

	if _, _, err := AssertMCPURLSafe("http://navigo-mcp:8765/mcp"); err == nil {
		t.Fatal("expected private host to be rejected without the allowlist env")
	}
}

func TestAssertMCPURLSafePublicHostUnaffected(t *testing.T) {
	stubLookupHost(t, []string{"93.184.216.34"})

	host, ip, err := AssertMCPURLSafe("https://mcp.example.com/mcp")
	if err != nil {
		t.Fatalf("expected public MCP host to pass, got %v", err)
	}
	if host != "mcp.example.com" || ip != "93.184.216.34" {
		t.Fatalf("unexpected host/ip: %q %q", host, ip)
	}
}
