//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ragflow/internal/parser/media"
)

// ssrfAllowLoopback lets tests exercise the HTTP image fetch path against a
// loopback httptest server. It stays false in production so loopback
// addresses are rejected (SSRF protection).
var ssrfAllowLoopback bool

// isRemoteImageSource reports whether src is an HTTP(S) image source.
func isRemoteImageSource(src string) bool {
	src = strings.TrimSpace(src)
	return strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
}

// remoteImageMIME labels downloaded bytes for the run media cache. A response
// that does not identify as an image keeps an opaque label so the image is
// still cached and uploaded rather than silently dropped.
func remoteImageMIME(data []byte) string {
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/octet-stream"
	}
	return mime
}

// fetchImage fetches a bounded HTTP(S) image payload. Local/private addresses
// and redirects to them are rejected (SSRF guard). Hostnames are resolved
// once and the validated IP is pinned in a custom DialContext to prevent
// DNS-rebinding TOCTOU attacks.
func fetchImage(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("image fetch: invalid image URL: %w", err)
	}

	// pinned maps hostname (without port) → validated IP. The hostname
	// is resolved once per host, and the transport dials the pinned IP
	// directly instead of re-resolving DNS.
	var pinnedMu sync.Mutex
	pinned := make(map[string]net.IP)

	pinHost := func(host string) error {
		ip, err := resolveAndValidateHost(ctx, host)
		if err != nil {
			return err
		}
		h, _, _ := net.SplitHostPort(host)
		if h == "" {
			h = host
		}
		pinnedMu.Lock()
		pinned[h] = ip
		pinnedMu.Unlock()
		return nil
	}

	if err := pinHost(parsed.Host); err != nil {
		return nil, err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			pinnedMu.Lock()
			ip, ok := pinned[host]
			pinnedMu.Unlock()
			if ok {
				return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("image fetch: too many redirects")
			}
			return pinHost(req.URL.Host)
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("image fetch: create image request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image fetch: fetch image %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image fetch: fetch image %s: HTTP %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(media.ImageMaxBytes())+1))
	if err != nil {
		return nil, fmt.Errorf("image fetch: read image %s: %w", rawURL, err)
	}
	if len(body) > media.ImageMaxBytes() {
		return nil, fmt.Errorf("image fetch: image %s exceeds %d-byte limit", rawURL, media.ImageMaxBytes())
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("image fetch: image %s is empty", rawURL)
	}
	return body, nil
}

// resolveAndValidateHost resolves a host (which may include a port),
// validates none of its IPs are internal/private, and returns the
// first public IP for connection pinning.
func resolveAndValidateHost(ctx context.Context, host string) (net.IP, error) {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		if (ip.IsLoopback() && !ssrfAllowLoopback) || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsPrivate() || ip.IsUnspecified() {
			return nil, fmt.Errorf("image fetch: rejected image URL to internal address: %s", host)
		}
		return ip, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("image fetch: cannot resolve image host: %s", hostname)
	}
	for _, addr := range addrs {
		ip = addr.IP
		if (ip.IsLoopback() && !ssrfAllowLoopback) || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsPrivate() || ip.IsUnspecified() {
			return nil, fmt.Errorf("image fetch: rejected image URL resolving to internal address: %s (%s)", host, ip)
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("image fetch: no addresses resolved for host: %s", hostname)
	}
	return addrs[0].IP, nil
}
