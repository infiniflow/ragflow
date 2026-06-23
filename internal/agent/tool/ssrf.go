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

package tool

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrSSRFBlocked is returned when a tool is asked to fetch a URL whose
// host resolves to a loopback, private, link-local or otherwise
// non-public IP range. This blocks the standard SSRF probes against
// internal services and cloud metadata endpoints (AWS, GCP, Azure,
// Alibaba all expose 169.254.169.254).
var ErrSSRFBlocked = errors.New("ssrf: target host is blocked by SSRF guard")

// credentialQueryParams is the lower-cased set of query-parameter names
// we treat as API credentials. Matching is case-insensitive. Any query
// string that uses one of these names has its value redacted in error
// messages and logs so an upstream 4xx/5xx that echoes the URL never
// leaks the secret.
var credentialQueryParams = map[string]struct{}{
	"key":          {},
	"api_key":      {},
	"apikey":       {},
	"token":        {},
	"access_token": {},
	"auth":         {},
}

// validateURLForSSRF parses rawURL and rejects any target whose host
// resolves (via DNS) to a non-public IP. The check is repeated against
// every returned A/AAAA record because a name that resolves to a mix
// of public and private IPs is also dangerous (DNS-rebinding /
// multi-A-record pinning).
func validateURLForSSRF(rawURL string) error {
	_, _, err := ResolveAndValidate(rawURL)
	return err
}

// ResolveAndValidate parses rawURL, performs the SSRF blocklist checks,
// and returns the first non-public IP that the host resolves to. The
// returned IP is safe to dial directly (bypassing a fresh DNS lookup)
// which defeats DNS-rebinding attacks: an attacker cannot swap a
// public record for a private one between this lookup and the connect,
// because the connect is pinned at the transport layer (see
// HTTPHelper.DoPinned in http_helper.go) and never re-resolves the
// hostname. Callers feed (originalHost, pinnedIP) into DoPinned.
//
// Note: pinning is done at the *http.Transport dialer, not by mutating
// the request URL. Mutating u.Host to the IP would break HTTPS — TLS
// ServerName is auto-populated from req.URL.Host, so the SNI would
// become the IP and cert verification would target the IP. The
// transport-layer approach keeps the URL host as the original hostname,
// preserving correct SNI / cert verification for any HTTPS endpoint.
func ResolveAndValidate(rawURL string) (originalHost string, pinnedIP net.IP, err error) {
	u, perr := url.Parse(rawURL)
	if perr != nil {
		return "", nil, fmt.Errorf("ssrf: parse url: %w", perr)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", nil, fmt.Errorf("ssrf: unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return "", nil, fmt.Errorf("ssrf: empty host")
	}

	// Short-circuit the well-known host aliases that DNS lookups may
	// also catch, but defending against the literal name is cheap and
	// saves a syscall on the common probe path.
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") ||
		lower == "metadata.google.internal" || lower == "metadata" ||
		lower == "0.0.0.0" || lower == "::" {
		return "", nil, fmt.Errorf("%w: %s", ErrSSRFBlocked, host)
	}

	// If the host is a literal IP, no DNS lookup is needed.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrLoopback(ip) {
			return "", nil, fmt.Errorf("%w: literal %s", ErrSSRFBlocked, host)
		}
		return host, ip, nil
	}

	ips, lerr := net.LookupIP(host)
	if lerr != nil {
		return "", nil, fmt.Errorf("ssrf: resolve %s: %w", host, lerr)
	}
	var firstSafe net.IP
	for _, ip := range ips {
		if isPrivateOrLoopback(ip) {
			return "", nil, fmt.Errorf("%w: %s -> %s", ErrSSRFBlocked, host, ip)
		}
		if firstSafe == nil {
			firstSafe = ip
		}
	}
	if firstSafe == nil {
		return "", nil, fmt.Errorf("ssrf: %s has no A/AAAA records", host)
	}
	return host, firstSafe, nil
}

// isPrivateOrLoopback reports whether ip is in any of the ranges we
// refuse to fetch from. It is deliberately conservative — link-local
// (169.254.0.0/16, fe80::/10) is rejected because that is where cloud
// metadata services live; multicast and the unspecified address are
// also rejected.
func isPrivateOrLoopback(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}

// SanitizeURL strips query parameters whose names match a small set of
// well-known credential names so error messages and logs that echo the
// request URL do not leak API keys. Anything else is preserved. The
// returned string is always a valid URL; on parse failure the original
// is returned unchanged.
func SanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	changed := false
	for k := range q {
		if _, ok := credentialQueryParams[strings.ToLower(k)]; ok {
			q.Set(k, "REDACTED")
			changed = true
		}
	}
	if !changed {
		return rawURL
	}
	u.RawQuery = q.Encode()
	return u.String()
}
