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

package sandbox

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestHTTPClientRetriesGETServerErrors(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "retry", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := NewHTTPClient(HTTPConfig{MaxAttempts: 3, BaseBackoff: time.Nanosecond})
	response, err := client.Do(t.Context(), http.MethodGet, server.URL, "", "", nil)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	response.Body.Close()
	if got := attempts.Load(); got != 3 {
		t.Fatalf("GET attempts = %d, want 3", got)
	}
}

func TestHTTPClientDoesNotRetryPOSTServerErrors(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "failed", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	client := NewHTTPClient(HTTPConfig{MaxAttempts: 3, BaseBackoff: time.Nanosecond})
	response, err := client.Do(t.Context(), http.MethodPost, server.URL+"/run", "{}", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer response.Body.Close()
	if got := attempts.Load(); got != 1 {
		t.Fatalf("POST attempts = %d, want 1", got)
	}
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("POST status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST response: %v", err)
	}
	if string(body) != "failed\n" {
		t.Fatalf("POST body = %q, want %q", body, "failed\\n")
	}
}

func TestHTTPClientDoesNotRetryPOSTNetworkErrors(t *testing.T) {
	wantErr := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ETIMEDOUT}
	transport := &errorTransport{err: wantErr}
	client := NewHTTPClient(HTTPConfig{MaxAttempts: 3, BaseBackoff: time.Nanosecond})
	client.client.Transport = transport

	response, err := client.Do(t.Context(), http.MethodPost, "http://sandbox.test/run", "{}", "application/json", nil)
	if response != nil {
		response.Body.Close()
		t.Fatalf("POST response = %v, want nil", response)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("POST error = %v, want %v", err, wantErr)
	}
	if got := transport.attempts.Load(); got != 1 {
		t.Fatalf("POST attempts = %d, want 1", got)
	}
}

func TestHTTPClientRetriesGETNetworkErrors(t *testing.T) {
	wantErr := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ETIMEDOUT}
	transport := &errorTransport{err: wantErr}
	client := NewHTTPClient(HTTPConfig{MaxAttempts: 3, BaseBackoff: time.Nanosecond})
	client.client.Transport = transport

	response, err := client.Do(t.Context(), http.MethodGet, "http://sandbox.test/healthz", "", "", nil)
	if response != nil {
		response.Body.Close()
		t.Fatalf("GET response = %v, want nil", response)
	}
	if !errors.Is(err, syscall.ETIMEDOUT) {
		t.Fatalf("GET error = %v, want timeout", err)
	}
	if got := transport.attempts.Load(); got != 3 {
		t.Fatalf("GET attempts = %d, want 3", got)
	}
}

type errorTransport struct {
	attempts atomic.Int32
	err      error
}

func (t *errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.attempts.Add(1)
	return nil, t.err
}
