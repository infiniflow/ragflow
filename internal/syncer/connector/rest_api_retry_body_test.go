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
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRestAPIFetchPage429ReplaysPostBody(t *testing.T) {
	withRestAPITestHooks(t)
	for _, closeConnection := range []bool{false, true} {
		name := "keep-alive"
		if closeConnection {
			name = "close"
		}
		t.Run(name, func(t *testing.T) {
			for _, config := range []struct {
				name, method string
				body         map[string]any
			}{
				{"POST", http.MethodPost, map[string]any{"filter": "active"}},
				{"POST default body", http.MethodPost, nil},
				{"GET", http.MethodGet, nil},
			} {
				t.Run(config.name, func(t *testing.T) {
					testRestAPIRateLimitBody(t, config.method, config.body, closeConnection, false)
				})
			}
		})
	}
	t.Run("exhaustion", func(t *testing.T) {
		testRestAPIRateLimitBody(t, http.MethodPost, map[string]any{"filter": "active"}, true, true)
	})
}

func testRestAPIRateLimitBody(t *testing.T, method string, body map[string]any, closeConnection, exhausted bool) {
	t.Helper()
	wantBody := ""
	if method == http.MethodPost {
		wantBody = `{}`
		if body != nil {
			wantBody = `{"filter":"active"}`
		}
	}
	type request struct {
		method string
		body   string
		err    error
	}
	var requests []request
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, request{r.Method, string(body), err})
		attempt := len(requests)
		mu.Unlock()
		if attempt <= 2 || exhausted {
			w.Header().Set("Retry-After", "0")
			if closeConnection {
				w.Header().Set("Connection", "close")
			}
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"method":         method,
		"request_body":   body,
		"content_fields": "title",
	})
	var err error
	if exhausted {
		_, err = c.fetchPage(t.Context(), nil)
	} else {
		_, err = c.fetchPageOnce(t.Context(), nil)
	}
	var rateErr *RateLimitTriedTooManyTimesError
	if exhausted {
		if !errors.As(err, &rateErr) {
			t.Errorf("fetchPage error = %v, want rate limit exhaustion", err)
		}
	} else if err != nil {
		t.Errorf("fetchPageOnce: %v", err)
	}
	mu.Lock()
	got := append([]request(nil), requests...)
	mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("requests = %+v, want exactly three", got)
	}
	for i, req := range got {
		if req.err != nil || req.body != wantBody || req.method != method {
			t.Errorf("request %d = %+v, want method %s body %q without error", i, req, method, wantBody)
		}
	}
}

func TestRestAPIFetchPage429PostCancellation(t *testing.T) {
	withRestAPITestHooks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Retry-After", "30")
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusTooManyRequests)
		time.AfterFunc(20*time.Millisecond, cancel)
	}))
	defer server.Close()
	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"method":         http.MethodPost,
		"request_body":   map[string]any{"filter": "active"},
		"content_fields": "title",
	})
	if _, err := c.fetchPageOnce(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("fetchPageOnce error = %v, want cancellation", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
}
