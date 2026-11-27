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
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestRequestWithContextPreservesHeadersAndResponse(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusForbidden} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/system/parser-license/wps" || r.Header.Get("Authorization") != "login-token" || r.Header.Get("X-Test") != "shared" {
					t.Errorf("unexpected request: %s %v", r.URL, r.Header)
				}
				w.Header().Set("X-Response", "retained")
				w.WriteHeader(status)
			}))
			client.LoginToken = new("login-token")
			response, err := client.RequestWithContext(t.Context(), 1, http.MethodGet, "/system/parser-license/wps", "web", map[string]string{"X-Test": "shared"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != status || response.Header.Get("X-Response") != "retained" {
				t.Fatalf("response = %+v", response)
			}
			if status == http.StatusNoContent && (response.Success != 1 || response.Fail != 0 || len(response.Body) != 0) {
				t.Fatalf("empty success = %+v", response)
			}
			if status == http.StatusForbidden && (response.Success != 0 || response.Fail != 1) {
				t.Fatalf("forbidden response = %+v", response)
			}
		})
	}
}

func TestRequestReplaysJSONBody(t *testing.T) {
	requests := 0
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"name":"repeated"}` || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("body = %q, headers = %v, error = %v", body, r.Header, err)
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	response, err := client.Request(3, http.MethodPost, "/repeat", "web", nil, map[string]any{"name": "repeated"})
	if err != nil || requests != 3 || response.Success != 3 || response.Fail != 0 {
		t.Fatalf("requests = %d, response = %+v, error = %v", requests, response, err)
	}
}

func TestRequestPropagatesTransportErrorsAndCancellation(t *testing.T) {
	t.Run("transport", func(t *testing.T) {
		for _, count := range []int{1, 3} {
			client := newTestHTTPClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			client.client.Transport = failingTransport{}
			response, err := client.Request(count, http.MethodGet, "/offline", "web", nil, nil)
			if (count == 1 && !errors.Is(err, io.ErrUnexpectedEOF)) || (count > 1 && err != nil) || response.Success != 0 || response.Fail != count {
				t.Fatalf("count = %d, response = %+v, error = %v", count, response, err)
			}
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		client := newTestHTTPClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("canceled request reached server")
		}))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		response, err := client.RequestWithContext(ctx, 3, http.MethodGet, "/canceled", "web", nil, nil)
		if !errors.Is(err, context.Canceled) || response.Success != 0 || response.Fail != 1 {
			t.Fatalf("response = %+v, error = %v", response, err)
		}
	})
}

func TestRequestWithIterationsRetainsResultsAfterTransportFailure(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(strconv.FormatBool(canceled), func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"code":0}`))
			}))
			transport := client.client.Transport
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			requests := 0
			client.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					return nil, io.ErrUnexpectedEOF
				}
				if canceled {
					cancel()
					return nil, ctx.Err()
				}
				return transport.RoundTrip(req)
			})
			response, err := client.RequestWithContext(ctx, 3, http.MethodGet, "/mixed", "web", nil, nil)
			if canceled {
				if !errors.Is(err, context.Canceled) || requests != 2 || response.Success != 0 || response.Fail != 2 {
					t.Fatalf("canceled response = %+v, requests = %d, error = %v", response, requests, err)
				}
			} else if err != nil || requests != 3 || response.Success != 2 || response.Fail != 1 || response.StatusCode != http.StatusOK || string(response.Body) != `{"code":0}` {
				t.Fatalf("response = %+v, requests = %d, error = %v", response, requests, err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

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
