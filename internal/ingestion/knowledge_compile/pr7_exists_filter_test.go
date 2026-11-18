//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package knowledge_compile

import (
	"context"
	"fmt"
	"testing"
)

// TestLoadDocProductsByDocsExistsFilterSent guards the perf fix for PR #22241:
// the batched reader must bound its query to compiled rows via an `exists:
// compile_kwd` clause. Without it the IN-filter also pulls every source chunk
// (hundreds per document), which both defeats the performance win and overflows
// the engine's result window on a large KB. This test fails (RED) until the
// production reader emits the exists clause.
func TestLoadDocProductsByDocsExistsFilterSent(t *testing.T) {
	const nDocs = 500
	const perDoc = 16 // compiled rows only
	docIDs := make([]string, 0, nDocs)
	chunks := make([]map[string]interface{}, 0, nDocs*perDoc)
	for i := 0; i < nDocs; i++ {
		id := fmt.Sprintf("d%d", i)
		docIDs = append(docIDs, id)
		for j := 0; j < perDoc; j++ {
			chunks = append(chunks, map[string]interface{}{
				"id":                  fmt.Sprintf("p_%d_%d", i, j),
				"doc_id":              id,
				"compile_kwd":         "wiki_page",
				"available_int":       1,
				"content_with_weight": "c",
			})
		}
	}
	eng := &fakeEngine{searchChunks: chunks, windowed: true}
	r := engineReader{eng: eng}
	if _, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs); err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	if got, ok := eng.lastSearchReq.Filter["exists"]; !ok || got != "compile_kwd" {
		t.Fatalf("LoadDocProductsByDocs must send exists:compile_kwd; got Filter=%#v", eng.lastSearchReq.Filter)
	}
}

// TestLoadDocProductsByDocsBoundedByExistsAvoidsWindowOverflow is the adversarial
// guard for the same fix: a 500-document batch mixes compiled rows (which carry
// compile_kwd) with ordinary source chunks (which do not). Without the exists
// clause the engine must scan ~20000 rows, so the third page's from+size exceeds
// the result window and the read errors. With the exists clause the query is
// bounded to the ~8000 compiled rows, stays within a single window, and returns
// every compiled product exactly once.
func TestLoadDocProductsByDocsBoundedByExistsAvoidsWindowOverflow(t *testing.T) {
	const nDocs = 500
	const compiledPerDoc = 16
	const sourcePerDoc = 24 // 500 * 40 = 20000 rows unfiltered (> MAX_RESULT_WINDOW)
	docIDs := make([]string, 0, nDocs)
	chunks := make([]map[string]interface{}, 0, nDocs*(compiledPerDoc+sourcePerDoc))
	for i := 0; i < nDocs; i++ {
		id := fmt.Sprintf("d%d", i)
		docIDs = append(docIDs, id)
		for j := 0; j < compiledPerDoc; j++ {
			chunks = append(chunks, map[string]interface{}{
				"id":                  fmt.Sprintf("c_%d_%d", i, j),
				"doc_id":              id,
				"compile_kwd":         "wiki_page",
				"available_int":       1,
				"content_with_weight": "c",
			})
		}
		for j := 0; j < sourcePerDoc; j++ {
			chunks = append(chunks, map[string]interface{}{
				"id":                  fmt.Sprintf("s_%d_%d", i, j),
				"doc_id":              id,
				"available_int":       1,
				"content_with_weight": "source",
			})
		}
	}
	eng := &fakeEngine{searchChunks: chunks, windowed: true}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs)
	if err != nil {
		t.Fatalf("LoadDocProductsByDocs must stay within the result window via exists:compile_kwd; got err=%v", err)
	}
	// Every compiled row must be returned exactly once; source chunks excluded.
	count := 0
	seen := map[string]int{}
	for _, ps := range got {
		for _, p := range ps {
			count++
			seen[p.ID]++
		}
	}
	if count != nDocs*compiledPerDoc {
		t.Fatalf("recovered products = %d, want %d (silent truncation or source-chunk leak)", count, nDocs*compiledPerDoc)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("product %s recovered %d times, want 1 (duplicate across pages)", id, n)
		}
	}
}
