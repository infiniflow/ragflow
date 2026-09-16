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
	"io"
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

func TestConnectorRequestRejectsCrossOriginBodyRedirect(t *testing.T) {
	withConnectorLoopbackTestHook(t)

	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit.Store(true)
	}))
	defer target.Close()
	targetPort := strings.TrimPrefix(target.URL, "http://127.0.0.1:")

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cross":
			http.Redirect(w, r, "http://127.0.0.1:"+targetPort+"/final", http.StatusTemporaryRedirect)
		case "/same":
			http.Redirect(w, r, "/same-final", http.StatusTemporaryRedirect)
		case "/same-final":
			got, _ := io.ReadAll(r.Body)
			if string(got) != "secret=abc" {
				t.Errorf("same-origin 307 body = %q, want forwarded body", got)
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer source.Close()

	opts := func(path string) connectorRequestOptions {
		return connectorRequestOptions{
			Method:       http.MethodPost,
			RawURL:       source.URL + path,
			Body:         []byte("secret=abc"),
			Headers:      map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			Timeout:      webdavRequestTimeout,
			MaxRedirects: 5,
		}
	}

	// A 307 to a different origin must not forward the credential-bearing body.
	crossResp, err := connectorRequest(context.Background(), opts("/cross"))
	if crossResp != nil {
		crossResp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("cross-origin body redirect err = %v, want rejection", err)
	}
	if targetHit.Load() {
		t.Fatalf("cross-origin 307 target must not be reached")
	}

	// A same-origin 307 still preserves the request body.
	resp, err := connectorRequest(context.Background(), opts("/same"))
	if err != nil {
		t.Fatalf("same-origin 307: %v", err)
	}
	resp.Body.Close()
}

func TestAssertConnectorURLSafeHTTPS(t *testing.T) {
	orig := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		return []string{"93.184.216.34"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = orig })

	connectorAllowLoopbackForTest = false
	t.Cleanup(func() { connectorAllowLoopbackForTest = false })

	// HTTPS public URLs pass.
	if _, _, err := assertConnectorURLSafeHTTPS("https://api.example.com/v1"); err != nil {
		t.Fatalf("https should be allowed: %v", err)
	}
	// Plain-HTTP (non-loopback) is rejected so credentials stay off the wire.
	if _, _, err := assertConnectorURLSafeHTTPS("http://api.example.com/v1"); err == nil {
		t.Fatalf("plain http should be rejected")
	}
	// Loopback HTTP is rejected outside the test hook.
	if _, _, err := assertConnectorURLSafeHTTPS("http://127.0.0.1:8080/v1"); err == nil {
		t.Fatalf("loopback http should be rejected outside the test hook")
	}

	// Loopback HTTP is permitted under the test hook (httptest servers).
	withConnectorLoopbackTestHook(t)
	if _, _, err := assertConnectorURLSafeHTTPS("http://127.0.0.1:8080/v1"); err != nil {
		t.Fatalf("loopback http should be allowed under the test hook: %v", err)
	}
	// But a plain-http non-loopback target is still rejected even under the hook.
	if _, _, err := assertConnectorURLSafeHTTPS("http://api.example.com/v1"); err == nil {
		t.Fatalf("plain http should still be rejected under the test hook")
	}
}

func TestConnectorStripAuthHeadersRemovesCredentials(t *testing.T) {
	headers := map[string]string{
		"Authorization":   "Bearer tok",
		"PRIVATE-TOKEN":   "gitlab-tok",
		"X-API-Key":       "key",
		"X-Auth-Token":    "auth",
		"Content-Type":    "application/json",
		"X-Custom-Header": "keep",
	}
	stripped := connectorStripAuthHeaders(headers)
	for _, k := range []string{"Authorization", "PRIVATE-TOKEN", "X-API-Key", "X-Auth-Token"} {
		if _, ok := stripped[k]; ok {
			t.Fatalf("sensitive header %q was not stripped", k)
		}
	}
	for _, k := range []string{"Content-Type", "X-Custom-Header"} {
		if stripped[k] == "" {
			t.Fatalf("non-sensitive header %q was stripped", k)
		}
	}
}
