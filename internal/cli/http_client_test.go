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

package cli

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func newTestHTTPClient(t *testing.T, handler http.Handler) *HTTPClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	client := NewHTTPClient()
	client.Host = host
	client.Port = port
	client.client = server.Client()
	return client
}

func TestRequestWithIterationsCountsApplicationErrorsAsFailures(t *testing.T) {
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code": 100, "message": "request failed"}`))
	}))

	result, err := client.Request(3, http.MethodGet, "/failure", "web", nil, nil)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if result.Success != 0 || result.Fail != 3 {
		t.Fatalf("successes = %d, failures = %d; want 0 successes and 3 failures", result.Success, result.Fail)
	}
}

func TestRequestWithIterationsPreservesPlainTextSuccess(t *testing.T) {
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("pong"))
	}))

	result, err := client.Request(3, http.MethodGet, "/ping", "none", nil, nil)
	if err != nil {
		t.Fatalf("RequestWithIterations() error = %v", err)
	}
	if result.Success != 3 || result.Fail != 0 {
		t.Fatalf("successes = %d, failures = %d; want 3 successes and 0 failures", result.Success, result.Fail)
	}
}

// TestBuildURLBracketsIPv6Host pins that an IPv6 Host produces a URL that
// net/http accepts. The old "%s:%d" join emitted "http://::1:8080/...", which
// http.NewRequest rejects; the host must be bracketed as "[::1]:8080".
func TestBuildURLBracketsIPv6Host(t *testing.T) {
	c := &HTTPClient{Host: "::1", Port: 8080, APIVersion: "v1"}

	apiURL := c.BuildURL("/datasets", "api")
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest(%q) failed: %v", apiURL, err)
	}
	if req.URL.Host != "[::1]:8080" {
		t.Errorf("API URL host = %q, want [::1]:8080", req.URL.Host)
	}
	if got, want := req.URL.Path, "/api/v1/datasets"; got != want {
		t.Errorf("API URL path = %q, want %q", got, want)
	}

	adminURL := c.BuildURL("/ping", "none")
	req, err = http.NewRequest(http.MethodGet, adminURL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest(%q) failed: %v", adminURL, err)
	}
	if req.URL.Host != "[::1]:8080" {
		t.Errorf("admin URL host = %q, want [::1]:8080", req.URL.Host)
	}
}

// TestBuildURLPreservesIPv4Host guards the non-IPv6 path: bracket joining must
// not alter the familiar "127.0.0.1:8080" form.
func TestBuildURLPreservesIPv4Host(t *testing.T) {
	c := &HTTPClient{Host: "127.0.0.1", Port: 9382, APIVersion: "v1"}

	if got, want := c.APIBase(), "127.0.0.1:9382/api/v1"; got != want {
		t.Errorf("APIBase() = %q, want %q", got, want)
	}
	if got, want := c.NonAPIBase(), "127.0.0.1:9382"; got != want {
		t.Errorf("NonAPIBase() = %q, want %q", got, want)
	}
}
