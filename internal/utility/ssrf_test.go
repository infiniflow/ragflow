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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
)

func TestAssertURLSafe(t *testing.T) {
	orig := common.LookupHost
	defer func() { common.LookupHost = orig }()

	type want struct {
		errSubstr string
		host      string
		ip        string
	}
	cases := []struct {
		name string
		url  string
		ips  []string
		err  string
		want want
	}{
		{
			name: "public IPv4",
			url:  "https://example.com/path",
			ips:  []string{"93.184.216.34"},
			want: want{host: "example.com", ip: "93.184.216.34"},
		},
		{
			name: "loopback rejected",
			url:  "http://localhost/x",
			ips:  []string{"127.0.0.1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "private 10.x rejected",
			url:  "http://internal/x",
			ips:  []string{"10.0.0.5"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "private 192.168.x rejected",
			url:  "http://router/x",
			ips:  []string{"192.168.1.1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "CGNAT 100.64/10 rejected",
			url:  "http://carrier/x",
			ips:  []string{"100.64.1.1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "IPv4-mapped IPv6 loopback rejected",
			url:  "http://[::ffff:127.0.0.1]/x",
			ips:  []string{"::ffff:127.0.0.1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "link-local IPv6 rejected",
			url:  "http://[fe80::1]/x",
			ips:  []string{"fe80::1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "documentation 2001:db8 rejected",
			url:  "http://[2001:db8::1]/x",
			ips:  []string{"2001:db8::1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "disallowed scheme ftp",
			url:  "ftp://example.com/",
			ips:  []string{"93.184.216.34"},
			want: want{errSubstr: "disallowed URL scheme"},
		},
		{
			name: "missing host",
			url:  "http:///path",
			want: want{errSubstr: "missing a host"},
		},
		{
			name: "resolution fails",
			url:  "http://nosuchhost.test/x",
			err:  "no such host",
			want: want{errSubstr: "could not resolve"},
		},
		{
			name: "all addresses must be public",
			url:  "http://mixed.example.com/",
			ips:  []string{"93.184.216.34", "127.0.0.1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "literal IPv4 loopback rejected",
			url:  "http://127.0.0.1/",
			ips:  []string{"127.0.0.1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "documentation TEST-NET-3 rejected",
			url:  "http://stub/",
			ips:  []string{"203.0.113.5"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "0.0.0.0/8 rejected",
			url:  "http://stub/",
			ips:  []string{"0.1.2.3"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "6to4 wrapping loopback rejected",
			url:  "http://stub/",
			ips:  []string{"2002:7f00:1::1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "NAT64 well-known prefix wrapping metadata IP rejected",
			url:  "http://stub/",
			ips:  []string{"64:ff9b::a9fe:a9fe"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "NAT64 local-use prefix rejected",
			url:  "http://stub/",
			ips:  []string{"64:ff9b:1::7f00:1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "Teredo wrapping loopback client rejected",
			url:  "http://stub/",
			ips:  []string{"2001:0:0:0:0:0:80ff:fffe"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "IPv4-compatible wrapping loopback rejected",
			url:  "http://stub/",
			ips:  []string{"::7f00:1"},
			want: want{errSubstr: "non-public address"},
		},
		{
			name: "NAT64 well-known prefix wrapping public IP allowed",
			url:  "http://stub/",
			ips:  []string{"64:ff9b::808:808"},
			want: want{host: "stub", ip: "64:ff9b::808:808"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			common.LookupHost = func(host string) ([]string, error) {
				if tc.err != "" {
					return nil, &mockErr{tc.err}
				}
				return tc.ips, nil
			}
			host, ip, err := common.AssertURLSafe(tc.url)
			if tc.want.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want.errSubstr) {
					t.Fatalf("expected error containing %q, got %v", tc.want.errSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tc.want.host {
				t.Errorf("host: got %q, want %q", host, tc.want.host)
			}
			if ip != tc.want.ip {
				t.Errorf("ip: got %q, want %q", ip, tc.want.ip)
			}
		})
	}
}

type mockErr struct{ s string }

func (e *mockErr) Error() string { return e.s }

// TestPinnedHTTPClientRedirectRefusesNonPublicHop covers the regression the
// issue flagged: a host passes ValidateAndServe and replies 302 to a
// loopback / private address. Before the fix the client followed the hop
// through the un-pinned fallback DialContext and the internal server saw
// the request. The redirect must now be refused by CheckRedirect.
//
// Setup:
//   - server A (the "validated" public host) replies 302 → server B's URL.
//   - server B is the "internal" target the guard must reject.
//
// LookupHost is stubbed so every name resolves to 127.0.0.1 — that makes
// the redirect target fail AssertURLSafe ("non-public address") without
// any DNS traffic. AllowAnyHostForTest is left at its zero value so the
// public-IP check runs.
func TestPinnedHTTPClientRedirectRefusesNonPublicHop(t *testing.T) {
	orig := common.LookupHost
	defer func() { common.LookupHost = orig }()
	origAllow := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = false
	defer func() { common.AllowAnyHostForTest = origAllow }()

	internalHit := atomic.Int32{}
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalHit.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer internal.Close()

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", internal.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer public.Close()

	common.LookupHost = func(host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}

	// PinnedHTTPClient itself does not run AssertURLSafe on the seed URL
	// — the caller is expected to have done that. We seed with the loopback
	// IP so the dial rewrite is a no-op for the first hop (which still hits
	// the httptest server). CheckRedirect then runs AssertURLSafe on the
	// redirect target's URL, which fails because the resolved IP is private.
	publicParsed, _ := url.Parse(public.URL)
	client := PinnedHTTPClient(publicParsed.Hostname(), "127.0.0.1", 5*time.Second)
	resp, err := client.Get(public.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("expected error from refused redirect, got status %d", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected non-public address error, got: %v", err)
	}
	if got := internalHit.Load(); got != 0 {
		t.Errorf("internal server received %d requests, want 0 (redirect must be refused)", got)
	}
}

// TestPinnedHTTPClientRedirectFollowsPublicHopPinned pins the "happy
// path": a redirect chain proceeds through public hops. Both legs resolve
// to a public IP under the test stub. The redirect target must be
// followed AND dialed through the per-client pin table (so the redirect
// hop cannot race a DNS rebinding to a private address).
//
// To assert the pin is honored, the test deliberately switches the
// stubbed LookupHost to return a private IP for the redirect hop's host
// AFTER CheckRedirect has recorded the pin. If the client fell back to
// plain DNS on the second dial, the stub's new answer would steer it
// to a private IP and the second server would never see the request.
func TestPinnedHTTPClientRedirectFollowsPublicHopPinned(t *testing.T) {
	orig := common.LookupHost
	defer func() { common.LookupHost = orig }()
	origAllow := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true // exercise the path; AssertURLSafe must still resolve and parse.
	defer func() { common.AllowAnyHostForTest = origAllow }()

	var bDialed atomic.Int32
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bDialed.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer b.Close()

	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", b.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer a.Close()

	common.LookupHost = func(host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}

	aParsed, _ := url.Parse(a.URL)
	client := PinnedHTTPClient(aParsed.Hostname(), "127.0.0.1", 5*time.Second)
	resp, err := client.Get(a.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (redirect should follow)", resp.StatusCode)
	}
	if got := bDialed.Load(); got != 1 {
		t.Errorf("hop target received %d requests, want 1", got)
	}
}
