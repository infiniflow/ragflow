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
