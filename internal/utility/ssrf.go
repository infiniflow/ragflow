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
	"strings"
	"time"

	"golang.org/x/net/idna"
)

// PinnedHTTPClient returns an HTTP client whose Transport dials resolvedIP
// for hostname and rejects other hosts (including redirect targets), closing
// the TOCTOU window between AssertURLSafe and the TCP connection.
var PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
	asciiHostname := hostname
	var idnaErr error
	if net.ParseIP(hostname) == nil {
		asciiHostname, idnaErr = idna.Lookup.ToASCII(hostname)
	}
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
			if splitErr != nil || idnaErr != nil || !strings.EqualFold(host, asciiHostname) || resolvedIP == "" {
				return nil, fmt.Errorf("unvalidated HTTP destination %q", addr)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
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
