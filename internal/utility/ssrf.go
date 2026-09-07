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
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AllowedURLSchemes are the schemes accepted by AssertURLSafe.
var AllowedURLSchemes = []string{"http", "https"}

// LookupHost is the indirection used to resolve hostnames. Tests override it.
var LookupHost = net.LookupHost

// AssertURLSafe parses rawURL and rejects it if the scheme is disallowed,
// the host is missing, or any resolved IP is not globally routable
// (private, loopback, link-local, multicast, reserved). Returns the hostname
// and the first validated public IP so callers can DNS-pin the address and
// prevent rebinding between validation and the actual TCP connection.
//
// Mirrors common/ssrf_guard.py:assert_url_is_safe.
func AssertURLSafe(rawURL string) (hostname, resolvedIP string, err error) {
	parsed, perr := url.Parse(strings.TrimSpace(rawURL))
	if perr != nil {
		return "", "", fmt.Errorf("Invalid url.")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !schemeAllowed(scheme) {
		sorted := append([]string(nil), AllowedURLSchemes...)
		sort.Strings(sorted)
		return "", "", fmt.Errorf("Disallowed URL scheme: '%s'. Only %v are allowed.", scheme, sorted)
	}

	hostname = parsed.Hostname()
	if hostname == "" {
		return "", "", fmt.Errorf("URL is missing a host.")
	}

	addrs, err := LookupHost(hostname)
	if err != nil {
		return "", "", fmt.Errorf("Could not resolve hostname '%s': %v", hostname, err)
	}
	if len(addrs) == 0 {
		return "", "", fmt.Errorf("Hostname '%s' resolved to no addresses.", hostname)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return "", "", fmt.Errorf("Could not parse resolved address '%s' for hostname '%s'.", addr, hostname)
		}
		if !isGlobalIP(effectiveIP(ip)) {
			return "", "", fmt.Errorf("URL resolves to a non-public address (%s), which is not allowed.", ip.String())
		}
		if resolvedIP == "" {
			resolvedIP = ip.String()
		}
	}
	return hostname, resolvedIP, nil
}

func schemeAllowed(scheme string) bool {
	for _, s := range AllowedURLSchemes {
		if s == scheme {
			return true
		}
	}
	return false
}

// effectiveIP unwraps IPv4-mapped IPv6 addresses (e.g. ::ffff:127.0.0.1) so
// the routability check sees the IPv4 form. Without this, an attacker could
// bypass the guard with an IPv4-mapped IPv6 representation of a private host.
func effectiveIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// isGlobalIP mirrors Python's ipaddress.IPv*Address.is_global: an address is
// global if it is none of {unspecified, loopback, multicast, link-local,
// private (including CGNAT and IPv6 ULA), benchmarking, documentation,
// reserved}.
func isGlobalIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsPrivate() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// CGNAT 100.64.0.0/10 — not flagged by IsPrivate in older Go versions.
		if v4[0] == 100 && v4[1]&0xC0 == 64 {
			return false
		}
		// 192.0.0.0/24 reserved for IETF protocol assignments.
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return false
		}
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 documentation (TEST-NET-1/2/3).
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 2 {
			return false
		}
		if v4[0] == 198 && v4[1] == 51 && v4[2] == 100 {
			return false
		}
		if v4[0] == 203 && v4[1] == 0 && v4[2] == 113 {
			return false
		}
		// 198.18.0.0/15 benchmarking.
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return false
		}
		// 240.0.0.0/4 reserved (excluding 255.255.255.255 which IsUnspecified misses).
		if v4[0] >= 240 {
			return false
		}
	} else if v6 := ip.To16(); v6 != nil {
		// 2001:db8::/32 documentation prefix.
		if v6[0] == 0x20 && v6[1] == 0x01 && v6[2] == 0x0d && v6[3] == 0xb8 {
			return false
		}
		// 100::/64 discard-only address block.
		if v6[0] == 0x01 && v6[1] == 0x00 && allZero(v6[2:8]) {
			return false
		}
	}
	return true
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// PinnedHTTPClient returns an HTTP client whose Transport rewrites every
// outbound dial for hostname:port to resolvedIP:port, closing the TOCTOU
// window between AssertURLSafe and the actual TCP connection. Pins are
// scoped to this client only.
func PinnedHTTPClient(hostname, resolvedIP string, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		// Disable environment proxy: HTTP_PROXY / HTTPS_PROXY would route
		// the connection through the proxy host instead of the pinned
		// resolvedIP, bypassing the SSRF guard.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr == nil && host == hostname && resolvedIP != "" {
				return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     false,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
