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

	if _, err := engine.InsertChunks(t.Context(), oneChunk(), "ragflow_tenant", "kb-1"); err != nil {
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

	if _, err := engine.InsertChunksNoRefresh(t.Context(), oneChunk(), "ragflow_tenant", "kb-1"); err != nil {
		t.Fatalf("InsertChunksNoRefresh: %v", err)
	}
	if len(refresh) != 1 {
		t.Fatalf("bulk requests = %d, want 1", len(refresh))
	}
	if refresh[0] != "" {
		t.Fatalf("refresh param = %q, want it omitted so the index decides", refresh[0])
	}
}
