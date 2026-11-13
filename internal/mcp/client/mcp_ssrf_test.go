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
	"ragflow/internal/common"
	"testing"
)

// stubLookupHost pins DNS resolution for the duration of a test without
// touching the server config singleton — the allowPrivateHosts decision is
// now an explicit parameter instead of a global read.
func stubLookupHost(t *testing.T, ips []string) {
	t.Helper()
	original := common.LookupHost
	common.LookupHost = func(string) ([]string, error) { return ips, nil }
	t.Cleanup(func() { common.LookupHost = original })
}

func TestAssertMCPURLSafePrivateHostAllowed(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})

	host, ip, err := AssertMCPURLSafe("http://navigo-mcp:8765/mcp", true)
	if err != nil {
		t.Fatalf("expected allowed private MCP host to pass, got %v", err)
	}
	if host != "navigo-mcp" || ip != "172.18.0.5" {
		t.Fatalf("expected pinning to resolved IP, got host=%q ip=%q", host, ip)
	}
}

func TestAssertMCPURLSafeRejectsPrivateHostWhenDisabled(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})

	if _, _, err := AssertMCPURLSafe("http://navigo-mcp:8765/mcp", false); err == nil {
		t.Fatal("expected private host to be rejected when allowPrivateHosts is false")
	}
}

func TestAssertMCPURLSafePublicHostUnaffected(t *testing.T) {
	stubLookupHost(t, []string{"93.184.216.34"})

	host, ip, err := AssertMCPURLSafe("https://mcp.example.com/mcp", false)
	if err != nil {
		t.Fatalf("expected public MCP host to pass, got %v", err)
	}
	if host != "mcp.example.com" || ip != "93.184.216.34" {
		t.Fatalf("unexpected host/ip: %q %q", host, ip)
	}
}

func TestAssertMCPURLSafeSameOriginAllowsPrivateAdvertisedURL(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})

	host, ip, err := AssertMCPURLSafeSameOrigin("http://navigo-mcp:8765/mcp/messages", "http://navigo-mcp:8765/mcp/sse", true)
	if err != nil {
		t.Fatalf("expected same-origin private advertised URL to pass, got %v", err)
	}
	if host != "navigo-mcp" || ip != "172.18.0.5" {
		t.Fatalf("unexpected host/ip: %q %q", host, ip)
	}
}

func TestAssertMCPURLSafeSameOriginRejectsCrossOriginPrivateAdvertisedURL(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.6"})

	if _, _, err := AssertMCPURLSafeSameOrigin("http://other-internal:8765/mcp/messages", "http://navigo-mcp:8765/mcp/sse", true); err == nil {
		t.Fatal("expected cross-origin private advertised URL to be rejected")
	}
}

// ---- corner cases for the parameterized guards ----

func TestAssertMCPURLSafeEmptyURL(t *testing.T) {
	stubLookupHost(t, []string{"8.8.8.8"})
	if _, _, err := AssertMCPURLSafe("", true); err == nil {
		t.Fatal("expected empty URL to be rejected")
	}
}

func TestAssertMCPURLSafeMalformedURL(t *testing.T) {
	stubLookupHost(t, []string{"8.8.8.8"})
	// Unparseable / scheme-less input must error, never panic.
	if _, _, err := AssertMCPURLSafe("://nonsense", true); err == nil {
		t.Fatal("expected malformed URL to be rejected")
	}
}

func TestAssertMCPURLSafeUnsupportedSchemeRejectedEvenWhenPrivateAllowed(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	// allowPrivateHosts relaxes routability, NOT the scheme check; a non
	// http(s) scheme must still be rejected via the strict path.
	if _, _, err := AssertMCPURLSafe("ftp://navigo-mcp:8765/mcp", true); err == nil {
		t.Fatal("expected unsupported scheme to be rejected even with allowPrivateHosts")
	}
}

func TestAssertMCPURLSafeSameOriginRejectsPrivateWhenDisabled(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	// Same-origin relaxation is gated on allowPrivateHosts; when false a
	// private advertised URL must be rejected even on the same origin.
	if _, _, err := AssertMCPURLSafeSameOrigin("http://navigo-mcp:8765/mcp/messages", "http://navigo-mcp:8765/mcp/sse", false); err == nil {
		t.Fatal("expected same-origin private advertised URL to be rejected when disabled")
	}
}

func TestAssertMCPURLSafePrivateHostWithEmptyLookupRejected(t *testing.T) {
	stubLookupHost(t, nil) // resolver returns no addresses
	if _, _, err := AssertMCPURLSafe("http://navigo-mcp:8765/mcp", true); err == nil {
		t.Fatal("expected empty DNS resolution to be rejected")
	}
}

// Same host, different port must be rejected: an SSE server must not be able to
// bounce an admitted POST to another service listening on the same host but a
// different port.
func TestAssertMCPURLSafeSameOriginRejectsDifferentPort(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	if _, _, err := AssertMCPURLSafeSameOrigin(
		"http://navigo-mcp:9000/mcp/messages",
		"http://navigo-mcp:8080/mcp/sse", true); err == nil {
		t.Fatal("expected same-host-different-port advertised URL to be rejected")
	}
}

// Same host and port, different scheme must be rejected: http vs https is a
// different origin and must not be accepted as same-origin.
func TestAssertMCPURLSafeSameOriginRejectsDifferentScheme(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	if _, _, err := AssertMCPURLSafeSameOrigin(
		"https://navigo-mcp:8765/mcp/messages",
		"http://navigo-mcp:8765/mcp/sse", true); err == nil {
		t.Fatal("expected same-host-different-scheme advertised URL to be rejected")
	}
}

// DNS hostnames are case-insensitive, so a server-advertised URL on the same
// host spelled with a different case must still be accepted as same-origin.
func TestAssertMCPURLSafeSameOriginAcceptsMixedCaseHost(t *testing.T) {
	stubLookupHost(t, []string{"172.18.0.5"})
	if _, _, err := AssertMCPURLSafeSameOrigin(
		"http://Example.COM:8765/mcp/messages",
		"http://example.com:8765/mcp/sse", true); err != nil {
		t.Fatalf("expected mixed-case same-host advertised URL to be accepted, got %v", err)
	}
}
