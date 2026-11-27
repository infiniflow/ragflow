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

package connector

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRestAPIFetchPageRedirectQuery(t *testing.T) {
	withRestAPITestHooks(t)
	const initialURI = "/start?cursor=request&page=3&source=uri&tenant=public+%26+%23%E4%BD%A0%E5%A5%BD"
	targets := []struct {
		name     string
		location string
		wantURI  string
	}{
		{"path with query", "/items?cursor=redirect&signed=sig", "/items?cursor=redirect&signed=sig"},
		{"relative path", "items?cursor=redirect", "/items?cursor=redirect"},
		{"absolute URL", "{origin}/items?cursor=redirect", "/items?cursor=redirect"},
		{"query only preserves escaping", "?cursor=next%2Fpart&signed=a%2Bb", "/start?cursor=next%2Fpart&signed=a%2Bb"},
		{"path without query", "/items", "/items"},
		{"empty query", "?", "/start?"},
		{"fragment inherits query", "#continued", initialURI},
	}
	type observedRequest struct {
		method string
		uri    string
		body   string
	}
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			for _, target := range targets {
				t.Run(fmt.Sprintf("%d/%s/%s", status, method, target.name), func(t *testing.T) {
					var requests []observedRequest
					var mu sync.Mutex
					var attempts atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Errorf("read request body: %v", err)
						}
						mu.Lock()
						requests = append(requests, observedRequest{r.Method, r.RequestURI, string(body)})
						mu.Unlock()
						if attempts.Add(1) == 1 {
							w.Header().Set("Location", strings.ReplaceAll(target.location, "{origin}", "http://"+r.Host))
							w.WriteHeader(status)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						w.Write([]byte(`{"ok":true}`))
					}))
					defer server.Close()
					c := mustRestAPIConnector(t, map[string]any{
						"url":            server.URL + "/start?cursor=url&source=uri",
						"method":         method,
						"query_params":   map[string]any{"cursor": "config", "tenant": "public & #你好"},
						"request_body":   map[string]any{"filter": "active"},
						"content_fields": "title",
					})
					if _, err := c.fetchPage(t.Context(), map[string]any{"cursor": "request", "page": 3}); err != nil {
						t.Fatalf("fetchPage: %v", err)
					}
					mu.Lock()
					gotRequests := append([]observedRequest(nil), requests...)
					mu.Unlock()
					if len(gotRequests) != 2 {
						t.Fatalf("requests = %+v, want two requests", gotRequests)
					}
					first, second := gotRequests[0], gotRequests[1]
					wantBody := ""
					if method == http.MethodPost {
						wantBody = `{"filter":"active"}`
					}
					if first != (observedRequest{method, initialURI, wantBody}) {
						t.Fatalf("initial request = %+v", first)
					}
					wantMethod := method
					if status == 301 || status == 302 || status == 303 {
						wantMethod, wantBody = http.MethodGet, ""
					}
					if want := (observedRequest{wantMethod, target.wantURI, wantBody}); second != want {
						t.Errorf("redirect request = %+v, want %+v", second, want)
					}
					if got := attempts.Load(); got != 2 {
						t.Errorf("requests = %d, want 2", got)
					}
				})
			}
		}
	}
}

func TestRestAPIFetchPageRedirectQueryAcrossHops(t *testing.T) {
	withRestAPITestHooks(t)
	const initialURI = "/start?cursor=initial"
	const nextURI = "/items?cursor=next%2Fpage"
	type request struct{ method, uri, body string }
	want := []request{
		{http.MethodPost, initialURI, `{"filter":"active"}`},
		{http.MethodPost, nextURI, `{"filter":"active"}`},
		{http.MethodGet, nextURI, ""},
		{http.MethodGet, "/items?", ""},
	}
	var requests []request
	var mu sync.Mutex
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		mu.Lock()
		requests = append(requests, request{r.Method, r.RequestURI, string(body)})
		mu.Unlock()
		switch attempts.Add(1) {
		case 1:
			w.Header().Set("Location", nextURI)
			w.WriteHeader(http.StatusTemporaryRedirect)
		case 2:
			w.Header().Set("Location", "#continued")
			w.WriteHeader(http.StatusFound)
		case 3:
			w.Header().Set("Location", "?")
			w.WriteHeader(http.StatusPermanentRedirect)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()
	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL + "/start",
		"method":         http.MethodPost,
		"query_params":   map[string]any{"cursor": "initial"},
		"request_body":   map[string]any{"filter": "active"},
		"content_fields": "title",
	})
	if _, err := c.fetchPage(t.Context(), nil); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if got := attempts.Load(); got != int32(len(want)) {
		t.Fatalf("requests = %d, want %d", got, len(want))
	}
	mu.Lock()
	gotRequests := append([]request(nil), requests...)
	mu.Unlock()
	for i, expected := range want {
		if got := gotRequests[i]; got != expected {
			t.Errorf("hop %d = %+v, want %+v", i, got, expected)
		}
	}
}
