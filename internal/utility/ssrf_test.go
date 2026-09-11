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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAssertURLSafe(t *testing.T) {
	orig := LookupHost
	defer func() { LookupHost = orig }()

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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			LookupHost = func(host string) ([]string, error) {
				if tc.err != "" {
					return nil, &mockErr{tc.err}
				}
				return tc.ips, nil
			}
			host, ip, err := AssertURLSafe(tc.url)
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

// TestPinnedHTTPClientRevalidatesRedirects: the pin only covers the hostname
// AssertURLSafe validated. A redirect to any other host must go through the
// same guard (and be pinned in turn), otherwise an attacker-controlled public
// server can 302 the client onto loopback, link-local or RFC1918 targets.
func TestPinnedHTTPClientRevalidatesRedirects(t *testing.T) {
	origLookup := LookupHost
	defer func() { LookupHost = origLookup }()
	// Every hostname resolves to loopback: the redirect target must then be
	// rejected as non-public by AssertURLSafe.
	LookupHost = func(host string) ([]string, error) { return []string{"127.0.0.1"}, nil }

	internalHit := false
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHit = true
		_, _ = w.Write([]byte("secret"))
	}))
	defer internal.Close()

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/latest/meta-data/", http.StatusFound)
	}))
	defer public.Close()

	_, publicPort, err := net.SplitHostPort(strings.TrimPrefix(public.URL, "http://"))
	if err != nil {
		t.Fatalf("split public addr: %v", err)
	}
	// "public.stub" stands in for a hostname AssertURLSafe accepted and pinned.
	client := PinnedHTTPClient("public.stub", "127.0.0.1", 5*time.Second)

	resp, err := client.Get("http://public.stub:" + publicPort + "/")
	if err == nil {
		resp.Body.Close()
	}
	if internalHit {
		t.Fatalf("redirect target on a non-public address was fetched; the SSRF guard was bypassed")
	}
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected the redirect to be rejected as non-public, got err=%v", err)
	}
}

// A redirect to a host that passes the guard is followed, and the new hop is
// dialed through its own pin rather than a fresh DNS lookup.
func TestPinnedHTTPClientFollowsValidatedRedirectsPinned(t *testing.T) {
	origAssert := AssertURLSafe
	defer func() { AssertURLSafe = origAssert }()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("landed"))
	}))
	defer target.Close()
	_, targetPort, _ := net.SplitHostPort(strings.TrimPrefix(target.URL, "http://"))

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to a hostname that only the pin table can resolve.
		http.Redirect(w, r, "http://second.stub:"+targetPort+"/", http.StatusFound)
	}))
	defer first.Close()
	_, firstPort, _ := net.SplitHostPort(strings.TrimPrefix(first.URL, "http://"))

	AssertURLSafe = func(rawURL string) (string, string, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", "", err
		}
		return u.Hostname(), "127.0.0.1", nil
	}

	client := PinnedHTTPClient("first.stub", "127.0.0.1", 5*time.Second)
	resp, err := client.Get("http://first.stub:" + firstPort + "/")
	if err != nil {
		t.Fatalf("validated redirect should be followed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "landed" {
		t.Fatalf("expected the redirect target body, got %q", body)
	}
}


// TestPinnedHTTPClientRefusesHTTPSToHTTPDowngrade: a redirect from an https
// origin to an http target must be refused before any validation, so headers
// such as Authorization are never replayed over cleartext.
func TestPinnedHTTPClientRefusesHTTPSToHTTPDowngrade(t *testing.T) {
	origAssert := AssertURLSafe
	defer func() { AssertURLSafe = origAssert }()
	assertCalled := false
	AssertURLSafe = func(rawURL string) (string, string, error) {
		assertCalled = true
		return "public.stub", "127.0.0.1", nil
	}

	client := PinnedHTTPClient("public.stub", "127.0.0.1", 5*time.Second)

	first, err := http.NewRequest(http.MethodGet, "https://public.stub/login", nil)
	if err != nil {
		t.Fatalf("build origin request: %v", err)
	}
	first.Header.Set("Authorization", "Bearer secret")
	next, err := http.NewRequest(http.MethodGet, "http://public.stub/login", nil)
	if err != nil {
		t.Fatalf("build redirect request: %v", err)
	}
	next.Header.Set("Authorization", "Bearer secret")

	err = client.CheckRedirect(next, []*http.Request{first})
	if err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("https->http redirect must be refused, got err=%v", err)
	}
	if assertCalled {
		t.Fatalf("the downgrade must be rejected before the target is validated or pinned")
	}

	// The same hop staying on https is still validated and allowed.
	same, _ := http.NewRequest(http.MethodGet, "https://public.stub/next", nil)
	if err := client.CheckRedirect(same, []*http.Request{first}); err != nil {
		t.Fatalf("https->https redirect should be allowed, got %v", err)
	}
	if !assertCalled {
		t.Fatalf("an allowed hop must go through AssertURLSafe")
	}
}
