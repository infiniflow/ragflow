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
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestRequestWithIterationsReplaysJSONBody(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		for _, closeConnection := range []bool{false, true} {
			name := method + "/keep-alive"
			if closeConnection {
				name = method + "/connection-close"
			}
			t.Run(name, func(t *testing.T) {
				payload := map[string]interface{}{"question": "检索问题", "top_k": 3}
				wantBody, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				var mu sync.Mutex
				var bodies [][]byte
				client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, readErr := io.ReadAll(r.Body)
					mu.Lock()
					bodies = append(bodies, body)
					iteration := len(bodies)
					mu.Unlock()
					if readErr != nil {
						http.Error(w, readErr.Error(), http.StatusBadRequest)
						return
					}
					if r.Method != method || r.URL.Path != "/api/v1/replay" ||
						r.Header.Get("Content-Type") != "application/json" ||
						r.Header.Get("Authorization") != "Bearer unit-test-token" ||
						r.Header.Get("X-Request-Tag") != "replay" {
						t.Errorf("unexpected request: method=%s path=%s headers=%v", r.Method, r.URL.Path, r.Header)
					}
					w.Header().Set("Content-Type", "application/json")
					if closeConnection {
						w.Header().Set("Connection", "close")
					}
					w.Header().Set("X-Iteration", strconv.Itoa(iteration))
					_ = json.NewEncoder(w).Encode(map[string]int{"code": 0, "iteration": iteration})
				}))
				client.client.Timeout = 2 * time.Second
				token := "unit-test-token"
				client.APIKey = &token
				result, err := client.Request(3, method, "/replay", "api", map[string]string{"X-Request-Tag": "replay"}, payload)
				if err != nil {
					t.Fatal(err)
				}
				if result.Success != 3 || result.Fail != 0 {
					t.Fatalf("successes = %d, failures = %d; want 3 successes and 0 failures", result.Success, result.Fail)
				}
				mu.Lock()
				defer mu.Unlock()
				if len(bodies) != 3 {
					t.Fatalf("received %d requests, want 3", len(bodies))
				}
				for i, body := range bodies {
					if !bytes.Equal(body, wantBody) {
						t.Errorf("request %d body = %q, want %q", i+1, body, wantBody)
					}
				}
				if result.StatusCode != http.StatusOK || result.Header.Get("X-Iteration") != "3" {
					t.Fatalf("last response: status=%d headers=%v", result.StatusCode, result.Header)
				}
				last, err := result.JSON()
				if err != nil || last["iteration"] != float64(3) {
					t.Fatalf("last response JSON = %v, error = %v", last, err)
				}
			})
		}
	}
}

func TestRequestWithIterationsCountsJSONApplicationErrors(t *testing.T) {
	var calls atomic.Int32
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		code := 0
		if calls.Add(1) == 2 {
			code = 100
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"code": code})
	}))
	client.client.Timeout = 2 * time.Second
	result, err := client.Request(3, http.MethodPost, "/mixed", "none", nil, map[string]interface{}{"query": "example"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || result.Success != 2 || result.Fail != 1 {
		t.Fatalf("calls=%d successes=%d failures=%d; want 3 calls, 2 successes, 1 failure", calls.Load(), result.Success, result.Fail)
	}
}

func TestRequestWithIterationsPreservesBodylessPOST(t *testing.T) {
	var calls atomic.Int32
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) != 0 {
			t.Errorf("body = %q, error = %v; want empty body", body, err)
		}
		calls.Add(1)
		_, _ = w.Write([]byte(`{"code": 0}`))
	}))
	client.client.Timeout = 2 * time.Second
	result, err := client.Request(3, http.MethodPost, "/empty", "none", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || result.Success != 3 || result.Fail != 0 {
		t.Fatalf("calls=%d successes=%d failures=%d; want 3 calls, 3 successes, 0 failures", calls.Load(), result.Success, result.Fail)
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
