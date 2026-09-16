//
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
//

package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/utility"
)

// withConnectorLoopbackTestHook enables the loopback test seam for the duration
// of a test so connectors can exercise the real HTTP path against httptest
// servers bound to loopback.
func withConnectorLoopbackTestHook(t *testing.T) {
	t.Helper()
	prev := connectorAllowLoopbackForTest
	connectorAllowLoopbackForTest = true
	t.Cleanup(func() { connectorAllowLoopbackForTest = prev })
}

func TestValidateConnectorURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool // true = accepted, false = rejected
	}{
		{name: "public literal ip", url: "http://8.8.8.8/x", want: true},
		{name: "loopback", url: "http://127.0.0.1/x", want: false},
		{name: "localhost", url: "http://localhost/x", want: false},
		{name: "cloud metadata", url: "http://169.254.169.254/latest/meta-data/", want: false},
		{name: "private 10/8", url: "http://10.0.0.5/x", want: false},
		{name: "private 192.168/16", url: "http://192.168.1.10/x", want: false},
		{name: "bad scheme", url: "ftp://8.8.8.8/x", want: false},
		{name: "missing host", url: "http://", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connectorAllowLoopbackForTest = false
			err := validateConnectorURL(tt.url)
			if tt.want && err != nil {
				t.Fatalf("validateConnectorURL(%q) = %v, want nil", tt.url, err)
			}
			if !tt.want && err == nil {
				t.Fatalf("validateConnectorURL(%q) = nil, want rejection", tt.url)
			}
		})
	}
}

func TestValidateConnectorURLLoopbackHookAllowsOnlyAllLoopback(t *testing.T) {
	withConnectorLoopbackTestHook(t)
	if err := validateConnectorURL("http://127.0.0.1/x"); err != nil {
		t.Fatalf("loopback should be allowed with hook on: %v", err)
	}
	// A private address is still rejected even with the hook on.
	if err := validateConnectorURL("http://10.0.0.1/x"); err == nil {
		t.Fatalf("private address should be rejected even with hook on")
	}
}

func TestWebDAVConnectorRejectsInternalBaseURL(t *testing.T) {
	connectorAllowLoopbackForTest = false
	t.Cleanup(func() { connectorAllowLoopbackForTest = false })

	for _, base := range []string{"http://127.0.0.1/dav", "http://169.254.169.254/dav", "http://10.0.0.5/dav"} {
		c, err := NewWebDAVConnector(map[string]any{
			"base_url": base,
			"credentials": map[string]any{
				"username": "user",
				"password": "pass",
			},
		})
		if err != nil {
			t.Fatalf("NewWebDAVConnector(%q) failed: %v", base, err)
		}
		if err := c.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "non-public address") {
			t.Fatalf("Validate(%q) = %v, want non-public rejection", base, err)
		}
	}
}

func TestConfluenceConnectorRejectsInternalWikiBase(t *testing.T) {
	connectorAllowLoopbackForTest = false
	t.Cleanup(func() { connectorAllowLoopbackForTest = false })

	for _, base := range []string{"http://127.0.0.1", "http://169.254.169.254", "http://10.0.0.5"} {
		c, err := NewConfluenceConnector(map[string]any{
			"wiki_base": base,
			"is_cloud":  true,
			"credentials": map[string]any{
				"confluence_username":     "user@example.com",
				"confluence_access_token": "token",
			},
		})
		if err != nil {
			t.Fatalf("NewConfluenceConnector(%q) failed: %v", base, err)
		}
		if err := c.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "non-public address") {
			t.Fatalf("Validate(%q) = %v, want non-public rejection", base, err)
		}
	}
}

func TestJiraConnectorRejectsInternalBaseURL(t *testing.T) {
	connectorAllowLoopbackForTest = false
	t.Cleanup(func() { connectorAllowLoopbackForTest = false })

	for _, base := range []string{"http://127.0.0.1", "http://169.254.169.254", "http://10.0.0.5"} {
		c, err := NewJiraConnector(map[string]any{
			"base_url":    base,
			"project_key": "RAG",
			"credentials": map[string]any{
				"jira_api_token": "token",
			},
		})
		if err != nil {
			t.Fatalf("NewJiraConnector(%q) failed: %v", base, err)
		}
		if err := c.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "non-public address") {
			t.Fatalf("Validate(%q) = %v, want non-public rejection", base, err)
		}
	}
}

func TestConnectorRequestPinsRedirectHopAgainstDNSRebinding(t *testing.T) {
	withConnectorLoopbackTestHook(t)
	origLookup := utility.LookupHost
	lookups := 0
	utility.LookupHost = func(host string) ([]string, error) {
		if host == "localhost" {
			lookups++
			if lookups == 1 {
				return []string{"127.0.0.1"}, nil
			}
			// A second resolution simulates DNS rebinding to a private address
			// after validation; the dial must never consult it.
			return []string{"10.0.0.5"}, nil
		}
		return origLookup(host)
	}
	t.Cleanup(func() { utility.LookupHost = origLookup })

	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit.Store(true)
	}))
	defer target.Close()
	targetPort := strings.TrimPrefix(target.URL, "http://127.0.0.1:")
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:"+targetPort+"/final", http.StatusFound)
	}))
	defer source.Close()

	resp, err := connectorRequest(context.Background(), connectorRequestOptions{
		Method:       http.MethodGet,
		RawURL:       source.URL,
		Timeout:      webdavRequestTimeout,
		MaxRedirects: 5,
	})
	if err != nil {
		t.Fatalf("connectorRequest: %v", err)
	}
	resp.Body.Close()
	if !targetHit.Load() {
		t.Fatalf("redirect target was not reached via its pinned address")
	}
	if lookups != 1 {
		t.Fatalf("LookupHost(localhost) called %d times, want 1 (the redirect hop dial must reuse the validated pin, not re-resolve)", lookups)
	}
}
