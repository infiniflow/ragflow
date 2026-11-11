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

package elasticsearch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
)

const bulkOKBody = `{"took":1,"errors":false,"items":[{"index":{"_index":"ragflow_tenant","_id":"c1","status":201}}]}`

// newBulkCaptureEngine returns an engine whose bulk requests are answered
// locally, recording the refresh parameter of every request it sees.
func newBulkCaptureEngine(t *testing.T, seen *[]string) *Engine {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.URL.Query().Get("refresh"))
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bulkOKBody))
	}))
	t.Cleanup(server.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	return &Engine{client: client}
}

func oneChunk() []map[string]interface{} {
	return []map[string]interface{}{{"id": "c1", "doc_id": "d1", "content_with_weight": "hello"}}
}

// The chunk APIs read chunks straight back after writing them, so the plain
// inserter keeps waiting for a refresh.
func TestInsertChunksWaitsForRefresh(t *testing.T) {
	var refresh []string
	engine := newBulkCaptureEngine(t, &refresh)

	if _, err := engine.InsertChunks(t.Context(), oneChunk(), "ragflow_tenant", "kb-1", ""); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	if len(refresh) != 1 || refresh[0] != "wait_for" {
		t.Fatalf("refresh params = %v, want exactly one wait_for", refresh)
	}
}

// Ingestion must not pay the refresh wait: omitting the parameter leaves the
// decision to the index (refresh_interval=1000ms) and matches Python's
// ingestion, which inserts with refresh=False. Waiting here costs ~0.5s per
// write and buys nothing - the chunks are published on the index's own cycle.
func TestInsertChunksNoRefreshOmitsRefreshParam(t *testing.T) {
	var refresh []string
	engine := newBulkCaptureEngine(t, &refresh)

	if _, err := engine.InsertChunksNoRefresh(t.Context(), oneChunk(), "ragflow_tenant", "kb-1", ""); err != nil {
		t.Fatalf("InsertChunksNoRefresh: %v", err)
	}
	if len(refresh) != 1 {
		t.Fatalf("bulk requests = %d, want 1", len(refresh))
	}
	if refresh[0] != "" {
		t.Fatalf("refresh param = %q, want it omitted so the index decides", refresh[0])
	}
}

func TestRefreshChunkStore(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{name: "success", status: http.StatusOK, body: `{"_shards":{"failed":0}}`},
		{name: "failed shard", status: http.StatusOK, body: `{"_shards":{"failed":1}}`, wantError: true},
		{name: "missing shard result", status: http.StatusOK, body: `{}`, wantError: true},
		{name: "unavailable", status: http.StatusServiceUnavailable, body: `{}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/ragflow_tenant/_refresh" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}, DisableRetry: true})
			if err != nil {
				t.Fatal(err)
			}
			err = (&Engine{client: client}).RefreshChunkStore(t.Context(), "ragflow_tenant", "kb-1")
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestSearchOneIndexRequireComplete(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{name: "timeout", body: `{"timed_out":true,"_shards":{"failed":0},"hits":{"total":{"value":0},"hits":[]}}`},
		{name: "shard failure", body: `{"timed_out":false,"_shards":{"failed":1},"hits":{"total":{"value":0},"hits":[]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := (&Engine{client: client}).searchOneIndex(t.Context(), "ragflow_tenant", []byte(`{}`), true); err == nil {
				t.Fatal("incomplete result accepted as a complete search")
			}
			if _, _, err := (&Engine{client: client}).searchOneIndex(t.Context(), "ragflow_tenant", []byte(`{}`), false); err != nil {
				t.Fatalf("ordinary search rejected partial results: %v", err)
			}
		})
	}
}
