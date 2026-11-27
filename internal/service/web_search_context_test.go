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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestContextWebSearchNativeCallbackHTTP(t *testing.T) {
	const canary = "context-test-'quote-\\backslash"
	cases := []struct {
		name, body, want string
		status           int
		wantError        bool
	}{
		{"highlights", `{"results":[{"title":"Source","url":"https://example.com/source","description":"Search description","highlights":{"code":"SUCCESS","highlights":["Query passage"]}}]}`, "Query passage", 200, false},
		{"empty", `{"results":[]}`, "", 200, false},
		{"description-labelled", `{"results":[{"title":"Source","url":"https://example.com/source","description":"Search description","highlights":{"code":"TIMEOUT","highlights":null}}]}`, "Highlights unavailable", 200, false},
		{"future-highlights-status", `{"results":[{"title":"Source","url":"https://example.com/source","description":"Search description","highlights":{"code":"context-test-status-secret","highlights":null}}]}`, "Highlights unavailable", 200, false},
		{"partial-labelled", `{"partial":true,"results":[{"title":"Source","url":"https://example.com/source","highlights":{"code":"SUCCESS","highlights":["Query passage"]}}]}`, "Partial", 200, false},
		{"empty-partial", `{"partial":true,"results":[]}`, "", 200, true},
		{"malformed-json", "not JSON " + canary, "", 200, true},
		{"missing-results", `{}`, "", 200, true},
		{"null-results", `{"results":null}`, "", 200, true},
		{"wrong-results", `{"results":{}}`, "", 200, true},
		{"wrong-field-type", `{"results":[{"title":{},"url":"https://example.com"}]}`, "", 200, true},
		{"unauthorized", canary, "", 401, true},
		{"payment", canary, "", 402, true},
		{"forbidden", canary, "", 403, true},
		{"limited", canary, "", 429, true},
		{"provider-failure", canary, "", 503, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.Host != "api.context.dev" || r.URL.Path != "/v1/web/search" {
					t.Error("wrong official request target")
				}
				if r.Header.Get("Authorization") != "Bearer "+canary {
					t.Error("wrong authorization header")
				}
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error("invalid request body")
				}
				if payload["query"] != "query" || payload["numResults"] != float64(webSearchResultCount) {
					t.Error("wrong query/count")
				}
				highlights, _ := payload["highlightsOptions"].(map[string]interface{})
				if highlights["enabled"] != true {
					t.Error("Highlights must be requested by default")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			transport := server.Client().Transport.(*http.Transport).Clone()
			transport.TLSClientConfig.ServerName = "example.com"
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			client := *contextWebSearchHTTPClient
			client.Transport = transport
			original := contextWebSearchHTTPClient
			contextWebSearchHTTPClient = &client
			defer func() { contextWebSearchHTTPClient = original; transport.CloseIdleConnections() }()
			callback := (&ChatPipelineService{}).webSearchFunc(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": canary, "tavily_api_key": "unrelated-key"})
			if callback == nil {
				t.Fatal("selected Context provider must create a native callback")
			}
			results, err := callback(t.Context(), "query")
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				if contextCredentialReflected(err.Error(), canary) {
					t.Fatal("credential reflected in error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if tc.want == "" {
				if len(results) != 0 {
					t.Fatal("empty success is not empty")
				}
				return
			}
			if len(results) != 1 || results[0].URL != "https://example.com/source" || !strings.Contains(results[0].Content, tc.want) {
				t.Fatal("native citation/passage mapping failed")
			}
		})
	}
}

func contextCredentialReflected(text, key string) bool {
	encoded, _ := json.Marshal(key)
	quoted := strconv.Quote(key)
	for _, variant := range []string{key, string(encoded[1 : len(encoded)-1]), quoted[1 : len(quoted)-1]} {
		if strings.Contains(text, variant) {
			return true
		}
	}
	return false
}

func TestContextSearchRequiresOwnKeyAndPreservesCancellation(t *testing.T) {
	for _, key := range []interface{}{nil, "", "   ", 42} {
		if p := resolveWebSearchProvider(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": key, "tavily_api_key": "unrelated"}); p != nil {
			t.Fatal("missing Context key must not use another provider")
		}
	}
	callback := (&ChatPipelineService{}).webSearchFunc(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": "synthetic"})
	if callback == nil {
		t.Fatal("Context provider unavailable")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := callback(ctx, "query")
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was not preserved")
	}
}

type contextFixtureTransport func(*http.Request) (*http.Response, error)

func (f contextFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestContextNativeCallbackSanitizesTransportCause(t *testing.T) {
	const canary = "context-test-'raw-\\escaped"
	original := contextWebSearchHTTPClient
	client := *original
	client.Transport = contextFixtureTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("reflected transport credential " + canary)
	})
	contextWebSearchHTTPClient = &client
	defer func() { contextWebSearchHTTPClient = original }()
	callback := (&ChatPipelineService{}).webSearchFunc(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": canary})
	if callback == nil {
		t.Fatal("native callback missing")
	}
	_, err := callback(t.Context(), "query")
	if err == nil {
		t.Fatal("transport failure must not succeed")
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		if contextCredentialReflected(e.Error(), canary) {
			t.Fatal("credential survived in error chain")
		}
	}
}

func TestContextNativeCallbackNeverFollowsRedirect(t *testing.T) {
	sinkCalls := 0
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkCalls++; io.WriteString(w, `{"results":[]}`) }))
	defer sink.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if strings.HasPrefix(address, "api.context.dev:") {
			address = server.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	original := contextWebSearchHTTPClient
	client := *original
	client.Transport = transport
	contextWebSearchHTTPClient = &client
	defer func() { contextWebSearchHTTPClient = original; transport.CloseIdleConnections() }()
	callback := (&ChatPipelineService{}).webSearchFunc(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": "synthetic"})
	if callback == nil {
		t.Fatal("native callback missing")
	}
	_, err := callback(t.Context(), "query")
	if err == nil || sinkCalls != 0 {
		t.Fatal("provider API redirect was followed or accepted")
	}
}

func TestContextNativeCallbackDeadline(t *testing.T) {
	original := contextWebSearchHTTPClient
	client := *original
	client.Transport = contextFixtureTransport(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	contextWebSearchHTTPClient = &client
	defer func() { contextWebSearchHTTPClient = original }()
	callback := (&ChatPipelineService{}).webSearchFunc(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": "synthetic"})
	if callback == nil {
		t.Fatal("native callback missing")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_, err := callback(ctx, "query")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline not preserved")
	}
}
