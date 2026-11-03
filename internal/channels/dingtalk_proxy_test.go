// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package channels

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestResolveDingTalkProxyDestination(t *testing.T) {
	originalLookup := common.LookupHost
	t.Cleanup(func() { common.LookupHost = originalLookup })
	common.LookupHost = func(host string) ([]string, error) {
		if host != "api.dingtalk.com" {
			t.Fatalf("unexpected DNS lookup host %q", host)
		}
		return []string{"8.8.8.8"}, nil
	}

	host, ip, err := resolveDingTalkProxyDestination("api.dingtalk.com:443")
	if err != nil {
		t.Fatalf("resolveDingTalkProxyDestination() error = %v", err)
	}
	if host != "api.dingtalk.com" || ip != "8.8.8.8" {
		t.Fatalf("resolved destination = %q, %q; want api.dingtalk.com, 8.8.8.8", host, ip)
	}
}

func TestResolveDingTalkProxyDestinationRejectsUntrustedHostsAndPorts(t *testing.T) {
	for _, address := range []string{
		"attacker.example:443",
		"api.dingtalk.com:80",
		"api.dingtalk.com",
	} {
		t.Run(address, func(t *testing.T) {
			if _, _, err := resolveDingTalkProxyDestination(address); err == nil {
				t.Fatalf("resolveDingTalkProxyDestination(%q) unexpectedly succeeded", address)
			}
		})
	}
}

func TestResolveDingTalkProxyDestinationRejectsPrivateDNS(t *testing.T) {
	originalLookup := common.LookupHost
	t.Cleanup(func() { common.LookupHost = originalLookup })
	common.LookupHost = func(string) ([]string, error) { return []string{"169.254.169.254"}, nil }

	if _, _, err := resolveDingTalkProxyDestination("api.dingtalk.com:443"); err == nil || !strings.Contains(err.Error(), "not safe") {
		t.Fatalf("resolveDingTalkProxyDestination() error = %v, want unsafe-address rejection", err)
	}
}

func TestDingTalkProxyRequiresAuthenticationAndCONNECT(t *testing.T) {
	handler := &dingtalkProxyHandler{authorization: "Basic expected"}
	for _, test := range []struct {
		name       string
		method     string
		authorize  string
		wantStatus int
		host       string
	}{
		{name: "unauthenticated CONNECT", method: http.MethodConnect, wantStatus: http.StatusProxyAuthRequired},
		{name: "authenticated non-CONNECT", method: http.MethodGet, authorize: "Basic expected", wantStatus: http.StatusMethodNotAllowed},
		{name: "authenticated untrusted CONNECT", method: http.MethodConnect, authorize: "Basic expected", wantStatus: http.StatusForbidden, host: "attacker.example:443"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://proxy/", nil)
			req.Host = test.host
			if req.Host == "" {
				req.Host = "api.dingtalk.com:443"
			}
			if test.authorize != "" {
				req.Header.Set("Proxy-Authorization", test.authorize)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != test.wantStatus {
				t.Fatalf("proxy status = %d, want %d", resp.Code, test.wantStatus)
			}
		})
	}
}
