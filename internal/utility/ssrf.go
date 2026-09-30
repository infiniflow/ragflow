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
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ragflow/internal/common"
)

// maxPinnedRedirects mirrors Go's http.Client default redirect budget. We
// keep the same number so callers that previously relied on Go's default
// (10 redirects) see no behaviour change for legitimate redirect chains.
const maxPinnedRedirects = 10

// PinnedHTTPClient returns an HTTP client whose Transport rewrites every
// outbound dial for hostname:port to resolvedIP:port, closing the TOCTOU
// window between AssertURLSafe and the actual TCP connection. Pins are
// scoped to this client only.
//
// Redirects are validated through the same AssertURLSafe guard as the
// original URL: a hop to a non-public address is refused, and a hop to a
// public address is followed pinned to that hop's resolved IP. Without the
// redirect check a user-supplied URL that passes the guard could reply
// 302 to http://169.254.169.254/latest/meta-data/ (or loopback / private
// RFC1918) and the client would dial it through the un-pinned fallback
// branch (#19544).
var PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	// pins is the per-client pin table: hostname -> resolvedIP. The seed
	// entry is the (hostname, resolvedIP) the caller validated. Each
	// validated redirect adds its own entry, so dials to a redirected host
	// still go through the pin and not through plain DNS resolution.
	pins := &sync.Map{}
	if hostname != "" && resolvedIP != "" {
		pins.Store(hostname, resolvedIP)
	}
	transport := &http.Transport{
		// Disable environment proxy: HTTP_PROXY / HTTPS_PROXY would route
		// the connection through the proxy host instead of the pinned
		// resolvedIP, bypassing the SSRF guard.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr == nil {
				if pinned, ok := pins.Load(host); ok {
					if ipStr, ok := pinned.(string); ok && ipStr != "" {
						return dialer.DialContext(ctx, network, net.JoinHostPort(ipStr, port))
					}
				}
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxPinnedRedirects {
				return errors.New("stopped after " + strconv.Itoa(maxPinnedRedirects) + " redirects")
			}
			// Reuse the same guard the caller used on the seed URL. A
			// hop to a non-public address fails closed here exactly like
			// it would have failed at the original entry point.
			host, ip, err := common.AssertURLSafe(req.URL.String())
			if err != nil {
				return err
			}
			if host != "" && ip != "" {
				pins.Store(host, ip)
			}
			return nil
		},
	}
}