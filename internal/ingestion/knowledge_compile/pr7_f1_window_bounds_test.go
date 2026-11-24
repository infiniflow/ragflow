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

// buildCompiledOnlyChunks returns nDocs documents each with compiledPerDoc
// compiled rows (compile_kwd set, no source chunks). This isolates the F1
// failure mode: the doc_id IN batch aggregates more compiled rows than the
// engine result window, independent of the source-chunk inflation that the
// exists:compile_kwd filter already fixes.
func buildCompiledOnlyChunks(nDocs, compiledPerDoc int) ([]string, []map[string]interface{}) {
	docIDs := make([]string, 0, nDocs)
	chunks := make([]map[string]interface{}, 0, nDocs*compiledPerDoc)
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
	}
	return docIDs, chunks
}

// TestLoadDocProductsByDocsCompiledRowsExceedWindow is the adversarial guard for
// F1: a 500-document batch whose COMPILED rows alone (no source chunks) exceed
// MAX_RESULT_WINDOW (10000) must not hard-fail. The exists:compile_kwd filter
// already excludes source chunks, but a product-dense batch can still aggregate
// more than 10k compiled rows; plain from/size cannot page past the window, so
// the reader must fall back to per-document reads for that sub-batch.
func TestLoadDocProductsByDocsCompiledRowsExceedWindow(t *testing.T) {
	const nDocs = 500
	const compiledPerDoc = 30 // 500*30 = 15000 compiled rows > MAX_RESULT_WINDOW(10000)
	docIDs, chunks := buildCompiledOnlyChunks(nDocs, compiledPerDoc)
	eng := &fakeEngine{searchChunks: chunks, windowed: true}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs)
	if err != nil {
		t.Fatalf("compiled-only batch exceeding window must fall back to per-doc reads, not error: %v", err)
	}
	// Fallback must have engaged (batched path alone would error or need >2 calls).
	if eng.searchCalls <= 2 {
		t.Fatalf("expected fallback to per-doc reads (more than 2 Search calls), got %d", eng.searchCalls)
	}
	// Every compiled product returned exactly once, grouped by doc.
	count := 0
	seen := map[string]int{}
	for _, ps := range got {
		for _, p := range ps {
			count++
			seen[p.ID]++
		}
	}
	if count != nDocs*compiledPerDoc {
		t.Fatalf("recovered products = %d, want %d (silent truncation past window)", count, nDocs*compiledPerDoc)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("product %s recovered %d times, want 1 (duplicate across pages/fallback)", id, n)
		}
	}
}

// TestLoadDocProductsByDocsExactlyAtWindowFallsBack is the off-by-one guard:
// a batch whose compiled rows land exactly on MAX_RESULT_WINDOW (10000) still
// triggers a 3rd page request (offset 10000) that plain from/size rejects, so
// the fallback must engage here too.
func TestLoadDocProductsByDocsExactlyAtWindowFallsBack(t *testing.T) {
	const nDocs = 500
	const compiledPerDoc = 20 // 500*20 = 10000 exactly
	docIDs, chunks := buildCompiledOnlyChunks(nDocs, compiledPerDoc)
	eng := &fakeEngine{searchChunks: chunks, windowed: true}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs)
	if err != nil {
		t.Fatalf("exactly-at-window batch must fall back to per-doc reads, not error: %v", err)
	}
	if eng.searchCalls <= 2 {
		t.Fatalf("expected fallback to per-doc reads (more than 2 Search calls), got %d", eng.searchCalls)
	}
	count := 0
	seen := map[string]int{}
	for _, ps := range got {
		for _, p := range ps {
			count++
			seen[p.ID]++
		}
	}
	if count != nDocs*compiledPerDoc {
		t.Fatalf("recovered products = %d, want %d", count, nDocs*compiledPerDoc)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("product %s recovered %d times, want 1", id, n)
		}
	}
}

// TestLoadDocProductsByDocsUnderWindowStaysBatched guards against regressing the
// common path: a batch comfortably under the window must keep using the batched
// IN read (2 Search calls for 8000 rows paged at 5000), never silently falling
// back to one query per document.
func TestLoadDocProductsByDocsUnderWindowStaysBatched(t *testing.T) {
	const nDocs = 500
	const compiledPerDoc = 16 // 500*16 = 8000 < MAX_RESULT_WINDOW(10000)
	docIDs, chunks := buildCompiledOnlyChunks(nDocs, compiledPerDoc)
	eng := &fakeEngine{searchChunks: chunks, windowed: true}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs)
	if err != nil {
		t.Fatalf("under-window batch must not error: %v", err)
	}
	if eng.searchCalls != 2 {
		t.Fatalf("expected batched path (2 Search calls), got %d (regressed to per-doc?)", eng.searchCalls)
	}
	count := 0
	for _, ps := range got {
		count += len(ps)
	}
	if count != nDocs*compiledPerDoc {
		t.Fatalf("recovered products = %d, want %d", count, nDocs*compiledPerDoc)
	}
}

// TestLoadDocProductsByDocsMixedOverflowSubBatches guards the per-sub-batch
// scoping of the fallback: across a single call, the first 500-doc sub-batch is
// product-dense (overflows -> falls back to per-doc) while the second 500-doc
// sub-batch is comfortably under the window (stays batched). The overflow of one
// sub-batch must not disable batching for the next, nor corrupt its results.
func TestLoadDocProductsByDocsMixedOverflowSubBatches(t *testing.T) {
	const densePerDoc = 30  // first 500 docs -> 15000 compiled rows > window
	const sparsePerDoc = 16 // next 500 docs -> 8000 compiled rows < window
	const nDocs = 1000
	docIDs := make([]string, 0, nDocs)
	chunks := make([]map[string]interface{}, 0, nDocs*densePerDoc)
	for i := 0; i < nDocs; i++ {
		id := fmt.Sprintf("d%d", i)
		docIDs = append(docIDs, id)
		perDoc := sparsePerDoc
		if i < 500 {
			perDoc = densePerDoc
		}
		for j := 0; j < perDoc; j++ {
			chunks = append(chunks, map[string]interface{}{
				"id":                  fmt.Sprintf("c_%d_%d", i, j),
				"doc_id":              id,
				"compile_kwd":         "wiki_page",
				"available_int":       1,
				"content_with_weight": "c",
			})
		}
	}
	eng := &fakeEngine{searchChunks: chunks, windowed: true}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs)
	if err != nil {
		t.Fatalf("mixed sub-batches must not error: %v", err)
	}
	// 502 calls for the dense (overflow) sub-batch + 2 for the sparse sub-batch.
	if eng.searchCalls != 504 {
		t.Fatalf("expected 504 Search calls (502 fallback + 2 batched), got %d", eng.searchCalls)
	}
	count := 0
	seen := map[string]int{}
	for _, ps := range got {
		for _, p := range ps {
			count++
			seen[p.ID]++
		}
	}
	want := 500*densePerDoc + 500*sparsePerDoc
	if count != want {
		t.Fatalf("recovered products = %d, want %d", count, want)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("product %s recovered %d times, want 1", id, n)
		}
	}
}
