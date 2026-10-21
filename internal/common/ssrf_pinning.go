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

package common

import (
	"context"
	"net"
	"strings"
	"sync"
)

// PinTable binds validated (hostname, resolvedIP) pairs established by
// AssertURLSafe so the wrapped Transport's DialContext can dial the
// validated IP without resolving the hostname again — closing the
// DNS-rebinding TOCTOU window between AssertURLSafe and the TCP
// connect that strictSSRFTransport.RoundTrip hands the request to.
//
// Without the pin the wrapped *http.Transport runs LookupHost a second
// time inside DialContext, and a hostname whose ownership the caller
// controls (a BaseURL field on a user-configured model driver) can
// return a public IP for the validation lookup and a private IP for
// the dial, defeating the SSRF guard that the strict transport
// exists to enforce.
type PinTable struct {
	mu   sync.RWMutex
	pins map[string]string
}

// NewPinTable returns an empty PinTable.
func NewPinTable() *PinTable {
	return &PinTable{pins: map[string]string{}}
}

// Pin records that hostname resolved to resolvedIP during the
// latest AssertURLSafe call. Later dials for the same hostname
// dial resolvedIP instead of re-resolving.
func (p *PinTable) Pin(hostname, resolvedIP string) {
	if hostname == "" || resolvedIP == "" {
		return
	}
	p.mu.Lock()
	p.pins[strings.ToLower(hostname)] = resolvedIP
	p.mu.Unlock()
}

// Lookup returns the pinned resolvedIP for hostname, if any.
// The hostname match is case-insensitive (ASCII).
func (p *PinTable) Lookup(hostname string) (string, bool) {
	if hostname == "" {
		return "", false
	}
	p.mu.RLock()
	ip, ok := p.pins[strings.ToLower(hostname)]
	p.mu.RUnlock()
	return ip, ok
}

// WrapDialContext returns a DialContext that dials the pinned IP
// for any hostname that has a Pin entry, and delegates to inner
// for hosts that have not been pinned. inner is the transport's
// own DialContext (typically http.DefaultTransport's).
//
// The wrapper also falls back to inner when the requested addr
// already carries an IP literal — the SSRF guard validates
// hostnames, not literal IPs that the transport would dial
// directly without a lookup.
func (p *PinTable) WrapDialContext(inner func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if inner == nil {
		inner = (&net.Dialer{}).DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(addr)
		if splitErr != nil {
			return nil, splitErr
		}
		// No re-resolution for IP literals.
		if net.ParseIP(host) != nil {
			return inner(ctx, network, addr)
		}
		if ip, ok := p.Lookup(host); ok {
			return inner(ctx, network, net.JoinHostPort(ip, port))
		}
		return inner(ctx, network, addr)
	}
}
