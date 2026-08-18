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

package elasticsearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// assertStoredField compares a field of the (JSON-decoded) captured bulk
// document against the expected value using reflect.DeepEqual. The bulk body
// round-trips through JSON, so numbers arrive as float64 and arrays as
// []interface{}.
func assertStoredField(t *testing.T, doc map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got, ok := doc[key]
	if !ok {
		t.Errorf("stored doc missing field %q", key)
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("field %q = %#v, want %#v", key, got, want)
	}
}

// TestInsertChunks_WritesIngestionShape is the T0 unit-tier read-back baseline
// (issue #17371). It stands up an in-memory fake Elasticsearch, captures the
// bulk request that InsertChunks sends, and asserts the stored document keeps
// the index-physical field names that the ingestion pipeline currently emits
// (docnm_kwd, content_with_weight, create_timestamp_flt, question_kwd,
// important_kwd, page_num_int, position_int, kb_id overridden to datasetID).
//
// This guards the engine write boundary: when the suffixing is later moved
// from ingestion into this boundary (T2), the document we send to ES must stay
// byte-identical. No real ES is required, so it runs in the default unit tier.
func TestInsertChunks_WritesIngestionShape(t *testing.T) {
	var bulkBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_bulk" {
			bulkBody, _ = io.ReadAll(r.Body)
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errors":false,"items":[]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	engine := newTestEngine(t, srv.URL)

	// Mirrors the shape produced by indexdoc.ProcessChunksForPipeline after T1
	// (kb_id is a plain string). InsertChunks overrides kb_id with datasetID,
	// so the stored value is the datasetID regardless of the input form. The
	// input kb_id ("producer-kb-1") is intentionally DISTINCT from datasetID
	// ("kb-1") so the test actually exercises the override rather than passing
	// through an already-equal value.
	chunk := map[string]interface{}{
		"doc_id":               "doc-1",
		"id":                   "chunk-1",
		"kb_id":                "producer-kb-1",
		"docnm_kwd":            "sample.md",
		"content_with_weight":  "hello world",
		"create_time":          "2026-08-04 00:00:00",
		"create_timestamp_flt": float64(123.0),
		"question_kwd":         []string{"q1", "q2"},
		"important_kwd":        []string{"k1"},
		"page_num_int":         int(1),
		"position_int":         int(2),
	}

	if _, err := engine.InsertChunks(context.Background(), []map[string]interface{}{chunk}, "ragflow_chunk_readback_test", "kb-1"); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bulkBody)), "\n")
	if len(lines) < 2 {
		t.Fatalf("bulk body has %d lines, want >=2: %q", len(lines), string(bulkBody))
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &doc); err != nil {
		t.Fatalf("unmarshal doc line: %v", err)
	}

	assertStoredField(t, doc, "docnm_kwd", "sample.md")
	assertStoredField(t, doc, "content_with_weight", "hello world")
	assertStoredField(t, doc, "create_timestamp_flt", float64(123.0))
	assertStoredField(t, doc, "question_kwd", []interface{}{"q1", "q2"})
	assertStoredField(t, doc, "important_kwd", []interface{}{"k1"})
	assertStoredField(t, doc, "page_num_int", float64(1))
	assertStoredField(t, doc, "position_int", float64(2))
	// InsertChunks overrides kb_id with datasetID on write.
	assertStoredField(t, doc, "kb_id", "kb-1")
}
