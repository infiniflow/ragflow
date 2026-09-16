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

package models

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/utility"
)

// TestStrictSSRFTransportDialsValidatedIP pins the dial target to the IP
// AssertURLSafe approved: the validation lookup returns a public IP and
// every later lookup returns loopback — the DNS-rebinding shape. The
// recording dialer must see the validated address, never the rebound one.
func TestStrictSSRFTransportDialsValidatedIP(t *testing.T) {
	var dialedAddr atomic.Value
	dialedAddr.Store("")
	pins := &utility.PinTable{}
	transport := &http.Transport{
		DialContext: pins.WrapDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialedAddr.Store(addr)
			return nil, errors.New("recorded dial")
		}),
	}
	client := &http.Client{Transport: &strictSSRFTransport{base: transport, pins: pins}}

	var lookups atomic.Int64
	orig := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		if lookups.Add(1) == 1 {
			return []string{"93.184.216.34"}, nil
		}
		return []string{"127.0.0.1"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = orig })

	_, err := client.Get("http://rebind.test/v1/models")
	if err == nil {
		t.Fatal("expected the recorded dial to fail the request")
	}
	if got := dialedAddr.Load().(string); got != "93.184.216.34:80" {
		t.Fatalf("dialed %q, want %q (the validated IP)", got, "93.184.216.34:80")
	}
}

// TestDriverHTTPClientStrictPinsReboundHost drives the real client from
// NewDriverHTTPClient(false): the validation lookup answers 127.0.0.2
// while the transport's own resolution of "localhost" (real DNS) returns
// the 127.0.0.1 listener — the rebind. The dial must target the validated
// 127.0.0.2 (refused instantly, nothing listens there), never the rebound
// listener; on the old code the request reaches it, which is the
// vulnerability. withSSRFBypass skips the public-IP check so
// loopback-range addresses can stand in for the two answers; it does not
// affect the pin path.
func TestDriverHTTPClientStrictPinsReboundHost(t *testing.T) {
	withSSRFBypass(t)
	rebound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer rebound.Close()

	orig := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		return []string{"127.0.0.2"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = orig })

	client := NewDriverHTTPClient(false)
	strictDriverTransport(t, client).Proxy = nil

	resp, err := client.Get("http://localhost:" + serverPort(t, rebound) + "/")
	if err == nil {
		resp.Body.Close()
		t.Fatal("request reached the rebound loopback listener; dial was not pinned to the validated IP")
	}
	if !strings.Contains(err.Error(), "127.0.0.2") {
		t.Fatalf("dial error does not target the validated IP: %v", err)
	}
}

// TestDriverHTTPClientStrictReachesValidatedHost checks the pin routes
// the dial to the approved IP: the host's only record points at the test
// listener, so the request can succeed only when the transport dials the
// validated IP instead of re-resolving the unresolvable name.
// withSSRFBypass skips the public-IP check so the listener's loopback
// address can stand in as the validated one.
func TestDriverHTTPClientStrictReachesValidatedHost(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	orig := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = orig })

	client := NewDriverHTTPClient(false)
	strictDriverTransport(t, client).Proxy = nil

	resp, err := client.Get("http://models.test:" + serverPort(t, server) + "/")
	if err != nil {
		t.Fatalf("request to validated host failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// TestDriverHTTPClientStrictRejectsPrivateHostBeforeDial keeps the guard
// itself intact: a URL resolving to a private address is refused in
// RoundTrip, before any connection is attempted.
func TestDriverHTTPClientStrictRejectsPrivateHostBeforeDial(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()

	orig := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = orig })

	client := NewDriverHTTPClient(false)
	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("request to a loopback host succeeded under the strict guard")
	}
	if !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected SSRF refusal, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server received %d requests; refusal must happen before dialing", n)
	}
}

func strictDriverTransport(t *testing.T, client *http.Client) *http.Transport {
	t.Helper()
	rt := client.Transport
	if lt, ok := rt.(*providerLoggingTransport); ok {
		rt = lt.base
	}
	st, ok := rt.(*strictSSRFTransport)
	if !ok {
		t.Fatalf("client.Transport = %T, want *strictSSRFTransport", rt)
	}
	tr, ok := st.base.(*http.Transport)
	if !ok {
		t.Fatalf("strictSSRFTransport.base = %T, want *http.Transport", st.base)
	}
	return tr
}

func serverPort(t *testing.T, server *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return u.Port()
}
