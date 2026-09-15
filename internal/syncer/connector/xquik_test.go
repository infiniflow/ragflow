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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func validXquikConfig() map[string]any {
	return map[string]any{
		"query":         "ragflow lang:en",
		"query_type":    "Top",
		"page_size":     2,
		"max_pages":     5,
		"batch_size":    2,
		"request_delay": 0,
		"credentials": map[string]any{
			"xquik_api_key": "xq_test_key",
		},
	}
}

func newXquikTestConnector(t *testing.T, config map[string]any) *XquikConnector {
	t.Helper()
	connector, err := NewXquikConnector(config)
	if err != nil {
		t.Fatalf("NewXquikConnector: %v", err)
	}
	return connector
}

func withXquikTestServer(t *testing.T, connector *XquikConnector, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	connector.baseURL = server.URL
	t.Cleanup(func() {
		restAPISSRFAllowLoopback = previous
		server.Close()
	})
	return server
}

func TestNewXquikConnectorValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "missing key", mutate: func(config map[string]any) { config["credentials"] = map[string]any{} }, want: "xquik_api_key"},
		{name: "missing query", mutate: func(config map[string]any) { config["query"] = " " }, want: "query is required"},
		{name: "bad query type", mutate: func(config map[string]any) { config["query_type"] = "Popular" }, want: "Latest or Top"},
		{name: "page size too large", mutate: func(config map[string]any) { config["page_size"] = 10001 }, want: "page_size must be from 1 to 10000"},
		{name: "max pages too large", mutate: func(config map[string]any) { config["max_pages"] = 1001 }, want: "max_pages must be from 1 to 1000"},
		{name: "negative delay", mutate: func(config map[string]any) { config["request_delay"] = -1 }, want: "request_delay must be a non-negative number"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validXquikConfig()
			test.mutate(config)
			_, err := NewXquikConnector(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestXquikOpenSyncUsesWindowAndResumesCursor(t *testing.T) {
	connector := newXquikTestConnector(t, validXquikConfig())
	var (
		mu       sync.Mutex
		requests []url.Values
	)
	withXquikTestServer(t, connector, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "xq_test_key" {
			t.Errorf("x-api-key = %q, want test key", got)
		}
		mu.Lock()
		requests = append(requests, r.URL.Query())
		mu.Unlock()

		response := map[string]any{
			"tweets": []any{
				map[string]any{
					"id":        "101",
					"text":      "First post",
					"createdAt": "2026-08-24T10:15:00Z",
					"url":       "https://x.com/alice/status/101",
					"lang":      "en",
					"author": map[string]any{
						"id": "1", "username": "alice", "name": "Alice", "verified": true,
					},
					"likeCount": 4,
				},
				map[string]any{
					"id":        "102",
					"text":      "Second post",
					"createdAt": "2026-08-24T11:15:00Z",
					"url":       "https://x.com/bob/status/102",
					"author":    map[string]any{"id": "2", "username": "bob", "name": "Bob"},
				},
			},
			"has_next_page": true,
			"next_cursor":   "cursor-2",
		}
		if r.URL.Query().Get("cursor") == "cursor-2" {
			response = map[string]any{
				"tweets": []any{
					map[string]any{
						"id":        "103",
						"text":      "Third post",
						"createdAt": "2026-08-24T12:15:00Z",
						"url":       "https://x.com/cara/status/103",
						"author":    map[string]any{"id": "3", "username": "cara", "name": "Cara"},
					},
				},
				"has_next_page": false,
				"next_cursor":   "",
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))

	windowStart := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &windowStart,
		WindowEnd:   windowEnd,
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(first.Documents) != 2 {
		t.Fatalf("first documents = %d, want 2", len(first.Documents))
	}
	if got := string(first.Documents[0].Blob); got != "Author: @alice\nPublished: 2026-08-24T10:15:00Z\nURL: https://x.com/alice/status/101\n\nFirst post" {
		t.Fatalf("first document body = %q", got)
	}
	if got := first.Documents[0].Metadata["author.username"]; got != "alice" {
		t.Fatalf("author metadata = %v, want alice", got)
	}
	if first.Checkpoint == nil {
		t.Fatal("first checkpoint is nil")
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &windowStart,
		WindowEnd:   windowEnd,
		Resume:      first.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resumed OpenSync: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if err := resumed.Close(); err != nil {
		t.Fatalf("resumed Close: %v", err)
	}
	if len(second.Documents) != 1 || !strings.Contains(string(second.Documents[0].Blob), "Third post") {
		t.Fatalf("resumed documents = %+v, want third post", second.Documents)
	}

	mu.Lock()
	gotRequests := append([]url.Values(nil), requests...)
	mu.Unlock()
	if len(gotRequests) != 3 {
		t.Fatalf("request count = %d, want 3", len(gotRequests))
	}
	for _, query := range gotRequests {
		if query.Get("q") != "ragflow lang:en" || query.Get("queryType") != "Top" || query.Get("limit") != "2" {
			t.Fatalf("search query = %v", query)
		}
		if query.Get("sinceTime") != "2026-08-24T10:00:00Z" || query.Get("untilTime") != "2026-08-25T00:00:00Z" {
			t.Fatalf("window query = %v", query)
		}
	}
	if gotRequests[0].Get("cursor") != "" || gotRequests[1].Get("cursor") != "" || gotRequests[2].Get("cursor") != "cursor-2" {
		t.Fatalf("cursor sequence = [%q %q %q]", gotRequests[0].Get("cursor"), gotRequests[1].Get("cursor"), gotRequests[2].Get("cursor"))
	}
}

func TestXquikValidateConnectorSettingLimitsUsage(t *testing.T) {
	connector := newXquikTestConnector(t, validXquikConfig())
	var captured url.Values
	withXquikTestServer(t, connector, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"tweets": []any{}, "has_next_page": false, "next_cursor": ""}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	if err := connector.ValidateConnectorSetting(context.Background(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
	if captured.Get("limit") != "1" {
		t.Fatalf("validation limit = %q, want 1", captured.Get("limit"))
	}
	if captured.Get("cursor") != "" || captured.Get("sinceTime") != "" || captured.Get("untilTime") != "" {
		t.Fatalf("validation query = %v", captured)
	}
}

func TestXquikContinuesAfterEmptyPageWhenResponseHasMore(t *testing.T) {
	connector := newXquikTestConnector(t, validXquikConfig())
	requests := 0
	withXquikTestServer(t, connector, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		response := map[string]any{
			"tweets":        []any{},
			"has_next_page": true,
			"next_cursor":   "cursor-2",
		}
		if r.URL.Query().Get("cursor") == "cursor-2" {
			response = map[string]any{
				"tweets": []any{map[string]any{
					"id":        "101",
					"text":      "Older post",
					"createdAt": "2026-08-24T12:00:00Z",
				}},
				"has_next_page": false,
				"next_cursor":   "ignored-cursor",
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer func() { _ = session.Close() }()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || !strings.Contains(string(batch.Documents[0].Blob), "Older post") {
		t.Fatalf("documents = %+v, want older post", batch.Documents)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("second NextBatch error = %v, want EOF", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestXquikRejectsRepeatedCursor(t *testing.T) {
	config := validXquikConfig()
	config["batch_size"] = 1
	connector := newXquikTestConnector(t, config)
	requests := 0
	withXquikTestServer(t, connector, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"tweets": []any{map[string]any{
				"id":        r.URL.Query().Get("cursor") + "tweet",
				"text":      "Post",
				"createdAt": "2026-08-24T12:00:00Z",
			}},
			"has_next_page": true,
			"next_cursor":   "same-cursor",
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer func() { _ = session.Close() }()
	if _, err = session.NextBatch(context.Background()); err != nil {
		t.Fatalf("first NextBatch: %v", err)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("second NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 before repeated cursor aborts pagination", requests)
	}
}

func TestXquikOpenPruneUnsupported(t *testing.T) {
	connector := newXquikTestConnector(t, validXquikConfig())
	_, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if !errors.Is(err, ErrPruneUnsupported) {
		t.Fatalf("OpenPrune error = %v, want ErrPruneUnsupported", err)
	}
}
