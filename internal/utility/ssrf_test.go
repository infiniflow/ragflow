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
	"strings"
	"testing"
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
