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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newBulkReplyEngine returns an Engine whose bulk requests are answered with
// body, and whose update-by-query requests are answered with ubqBody.
func newBulkReplyEngine(t *testing.T, ubqBody, bulkBody string) *Engine {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/_update_by_query") {
			_, _ = w.Write([]byte(ubqBody))
			return
		}
		_, _ = w.Write([]byte(bulkBody))
	}))
	t.Cleanup(server.Close)
	return newTestEngine(t, server.URL)
}

func oneMetadata() []map[string]interface{} {
	return []map[string]interface{}{{
		"id":          "doc-1",
		"kb_id":       "kb-1",
		"meta_fields": map[string]interface{}{"author": "alice"},
	}}
}

func TestInsertMetadataReturnsErrorOnBulkItemError(t *testing.T) {
	engine := newBulkReplyEngine(t, `{}`,
		`{"took":1,"errors":true,"items":[{"index":{"_index":"ragflow_doc_meta_t1","_id":"doc-1","status":400,"error":{"type":"document_parsing_exception","reason":"failed to parse field [meta_fields.author]"}}}]}`)

	_, err := engine.InsertMetadata(t.Context(), oneMetadata(), "t1")
	if err == nil {
		t.Fatal("InsertMetadata returned nil for a bulk response with a failed item")
	}
	for _, want := range []string{"ragflow_doc_meta_t1", "1 item errors", "doc-1: failed to parse field"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestInsertMetadataReportsFailedItemCountBeforeTruncatingReasons(t *testing.T) {
	var items []string
	for i := 0; i < 7; i++ {
		items = append(items, fmt.Sprintf(
			`{"index":{"_id":"doc-%d","status":400,"error":{"reason":"reason-%d"}}}`, i, i))
	}
	body := `{"errors":true,"items":[` + strings.Join(items, ",") + `]}`
	engine := newBulkReplyEngine(t, `{}`, body)

	_, err := engine.InsertMetadata(t.Context(), oneMetadata(), "t1")
	if err == nil {
		t.Fatal("InsertMetadata returned nil for 7 failed items")
	}
	if !strings.Contains(err.Error(), "7 item errors") {
		t.Fatalf("error %q does not report all 7 failed items", err.Error())
	}
	if strings.Contains(err.Error(), "reason-5") || strings.Contains(err.Error(), "reason-6") {
		t.Fatalf("error %q should only show the first 5 reasons", err.Error())
	}
}

func TestInsertMetadataIdentifiesEachFailedDocument(t *testing.T) {
	engine := newBulkReplyEngine(t, `{}`,
		`{"errors":true,"items":[`+
			`{"index":{"_id":"doc-a","status":400,"error":{"reason":"failed to parse field [meta_fields.author]"}}},`+
			`{"index":{"_id":"doc-b","status":400,"error":{"reason":"failed to parse field [meta_fields.author]"}}}]}`)

	_, err := engine.InsertMetadata(t.Context(), oneMetadata(), "t1")
	if err == nil {
		t.Fatal("InsertMetadata returned nil for failed bulk items")
	}
	for _, want := range []string{"doc-a:", "doc-b:", "2 item errors"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestInsertMetadataSucceedsOnCleanBulkResponse(t *testing.T) {
	engine := newBulkReplyEngine(t, `{}`,
		`{"took":1,"errors":false,"items":[{"index":{"_id":"doc-1","status":201}}]}`)

	if _, err := engine.InsertMetadata(t.Context(), oneMetadata(), "t1"); err != nil {
		t.Fatalf("InsertMetadata returned %v for a clean bulk response", err)
	}
}

func TestUpdateMetadataPropagatesFallbackBulkItemError(t *testing.T) {
	engine := newBulkReplyEngine(t, `{"took":1,"total":0,"updated":0,"failures":[]}`,
		`{"errors":true,"items":[{"index":{"_id":"doc-1","status":400,"error":{"reason":"failed to parse field [meta_fields.author]"}}}]}`)

	err := engine.UpdateMetadata(t.Context(), "doc-1", "kb-1",
		map[string]interface{}{"author": "alice"}, "t1")
	if err == nil {
		t.Fatal("UpdateMetadata returned nil when its insert fallback had a failed item")
	}
	if !strings.Contains(err.Error(), "failed to insert metadata") {
		t.Fatalf("error %q does not wrap the fallback failure", err.Error())
	}
}
