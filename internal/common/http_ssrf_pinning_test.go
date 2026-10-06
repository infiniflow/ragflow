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
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

// stubLookupHost replaces common.LookupHost with f and restores it
// when the test ends. f receives the running call count (1-based),
// letting tests simulate "first lookup returns X, subsequent
// lookups return Y" rebinding attacks.
func stubLookupHost(t *testing.T, f func(host string, call int64) ([]string, error)) {
	t.Helper()
	var calls atomic.Int64
	orig := LookupHost
	LookupHost = func(host string) ([]string, error) {
		return f(host, calls.Add(1))
	}
	t.Cleanup(func() { LookupHost = orig })
}

// recordingAddrDialer captures every (network, addr) it is asked to
// dial. The test inspects the captured addr to assert the pin (or
// the rebinding fall-through) was honored.
type recordingAddrDialer struct {
	addrs []string
}

func (r *recordingAddrDialer) DialContext(_ context.Context, _ string, addr string) (net.Conn, error) {
	r.addrs = append(r.addrs, addr)
	return nil, errors.New("recording dialer does not connect")
}

// lookupHostDialer returns a DialContext that resolves the host
// through common.LookupHost — the same indirection AssertURLSafe
// uses. Tests use this to drive the rebinding stub for both the
// validation lookup and the transport's own dial.
func lookupHostDialer() func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := LookupHost(host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
	}
}

// TestStrictSSRFTransportDialsValidatedIP verifies the fix for the
// DNS-rebinding TOCTOU in strictSSRFTransport. The first LookupHost
// call (from AssertURLSafe inside RoundTrip) returns a public IP;
// the rebind target is a loopback IP. The transport must dial the
// validated IP — without the pin it would re-resolve inside its own
// DialContext and dial the rebound loopback.
func TestStrictSSRFTransportDialsValidatedIP(t *testing.T) {
	const validated = "93.184.216.34"
	const rebound = "127.0.0.1"

	stubLookupHost(t, func(_ string, call int64) ([]string, error) {
		// First call = AssertURLSafe = public IP.
		// Every later call = rebinding dialer = loopback.
		if call == 1 {
			return []string{validated}, nil
		}
		return []string{rebound}, nil
	})

	pinTable := NewPinTable()
	rec := &recordingAddrDialer{}

	transport := &http.Transport{
		DialContext: pinTable.WrapDialContext(rec.DialContext),
	}
	rt := &strictSSRFTransport{base: transport, pins: pinTable}

	req, err := http.NewRequest(http.MethodGet, "http://rebinding.example/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := rt.RoundTrip(req); err == nil {
		t.Fatal("expected recording dialer error")
	}

	if got := len(rec.addrs); got != 1 {
		t.Fatalf("dialer invoked %d times, want 1 (pin must be honored, no re-resolve)", got)
	}
	if want := validated + ":80"; rec.addrs[0] != want {
		t.Fatalf("dialer saw %s; want %s (rebound to loopback instead of validated IP)", rec.addrs[0], want)
	}

	stored, ok := pinTable.Lookup("rebinding.example")
	if !ok || stored != validated {
		t.Fatalf("pin table missing entry for rebinding.example: ip=%q ok=%v", stored, ok)
	}
}

// TestStrictSSRFTransportRejectsPrivateRebind ensures the validation
// step still rejects an entirely-private rebind answer — pinning
// only closes the second-lookup gap; AssertURLSafe must still reject
// the initial validation lookup if its answer is private.
func TestStrictSSRFTransportRejectsPrivateRebind(t *testing.T) {
	stubLookupHost(t, func(_ string, _ int64) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	})

	pinTable := NewPinTable()
	transport := &http.Transport{
		DialContext: pinTable.WrapDialContext((&recordingAddrDialer{}).DialContext),
	}
	rt := &strictSSRFTransport{base: transport, pins: pinTable}

	req, err := http.NewRequest(http.MethodGet, "http://private.example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := rt.RoundTrip(req); err == nil {
		t.Fatal("expected AssertURLSafe to reject private rebind, got no error")
	}
	if _, ok := pinTable.Lookup("private.example.com"); ok {
		t.Fatal("rejected requests must not populate the pin table")
	}
}

// TestDriverHTTPClientStrictPinsReboundHost is the end-to-end version
// of the rebinding regression. The configured BaseURL is
// "rebinding.example", and its hostname:
//
//  1. Resolves to a public IP for the first call (from
//     AssertURLSafe). Validation passes.
//  2. Resolves to a loopback IP for the second call (from the
//     transport's own DialContext, which lookupHostDialer wires
//     through common.LookupHost — the same indirection the test
//     stub is overriding).
//
// The first lookup's IP has no listener; the second's loopback IP has
// an httptest server. Pre-fix, the transport's DialContext re-resolves
// and reaches the listener. Post-fix, the pin forces the dialer to
// target the validated public IP and the connection is refused.
func TestDriverHTTPClientStrictPinsReboundHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	port := strconv.Itoa(srv.Listener.Addr().(*net.TCPAddr).Port)

	const reboundHost = "rebinding.example"
	stubLookupHost(t, func(host string, call int64) ([]string, error) {
		if host != reboundHost {
			return nil, &net.DNSError{Err: "no such host", Name: host}
		}
		// First call (from AssertURLSafe) returns the public IP.
		// Every later call (from the transport's DialContext)
		// returns the loopback IP that has the httptest listener.
		if call == 1 {
			return []string{"93.184.216.34"}, nil
		}
		return []string{"127.0.0.1"}, nil
	})

	pinTable := NewPinTable()
	transport := &http.Transport{
		DialContext: pinTable.WrapDialContext(lookupHostDialer()),
	}
	client := &http.Client{Transport: &strictSSRFTransport{base: transport, pins: pinTable}}

	resp, err := client.Get("http://" + reboundHost + ":" + port + "/")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("rebinding request unexpectedly succeeded (status=%d) — strictSSRFTransport is not pinning the validated IP", resp.StatusCode)
	}
}
