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
	"fmt"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPinnedHTTPClientRedirects(t *testing.T) {
	var forbiddenRequests atomic.Int32
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forbiddenRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer forbidden.Close()

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/external":
			http.Redirect(w, r, forbidden.URL, http.StatusFound)
		case "/same-host":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer public.Close()

	const hostname = "public.example"
	client := PinnedHTTPClient(hostname, "127.0.0.1", time.Second)
	baseURL := fmt.Sprintf("http://%s:%s", hostname, strings.TrimPrefix(public.URL, "http://127.0.0.1:"))

	resp, err := client.Get(baseURL + "/external")
	if err == nil {
		resp.Body.Close()
		t.Fatal("redirect to unvalidated host unexpectedly succeeded")
	}
	if forbiddenRequests.Load() != 0 {
		t.Fatalf("forbidden target received %d requests, want 0", forbiddenRequests.Load())
	}

	resp, err = client.Get(baseURL + "/same-host")
	if err != nil {
		t.Fatalf("same-host redirect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("same-host redirect status=%d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestPinnedHTTPClientUnicodeHostname(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	const hostname = "bücher.example"
	client := PinnedHTTPClient(hostname, "127.0.0.1", time.Second)
	target := strings.Replace(server.URL, "127.0.0.1", hostname, 1)
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("pinned Unicode hostname: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestPinnedHTTPClientIPv6Literal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	const hostname = "2001:4860:4860::8888"
	client := PinnedHTTPClient(hostname, "127.0.0.1", time.Second)
	target := fmt.Sprintf("http://[%s]:%s", hostname, strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("pinned IPv6 literal: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

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
