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

package knowledge_compile

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// TestLoadDocProductsByDocsSendsNoSearchAfterOrderBy guards the fix for PR
// #22241 review findings: the batched reader must NOT rely on OrderBy: Asc("id")
// to drive search_after deep pagination. On OpenSearch parseOrderByExpr drops the
// "id" field (no sort emitted), so useSearchAfter opens with a nil cursor and the
// read silently re-reads the first page (truncation + duplicates); on Elasticsearch
// 8.x the id-> _id translation is rejected by default (indices.id_field_data off),
// so the whole read fails. A 500-document batch never approaches the result window
// in practice, so plain from/size paging is correct and safe. The reader must
// therefore send NO OrderBy.
func TestLoadDocProductsByDocsSendsNoSearchAfterOrderBy(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "p1", "doc_id": "d1", "compile_kwd": "wiki_page", "available_int": 1, "content_with_weight": "c1"},
	}}
	r := engineReader{eng: eng}
	if _, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", []string{"d1"}); err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	if eng.lastSearchReq == nil {
		t.Fatalf("expected a SearchRequest to be captured")
	}
	if eng.lastSearchReq.OrderBy != nil && len(eng.lastSearchReq.OrderBy.Fields) > 0 {
		t.Fatalf("LoadDocProductsByDocs must not send an OrderBy (would drive broken search_after); got %#v", eng.lastSearchReq.OrderBy)
	}
}

// TestLoadDocProductsByDocsReadsAllRowsWithinWindow is the happy-path guard: a
// batch whose aggregated rows stay under the backend max_result_window must return
// every product exactly once, paged with plain from/size (no search_after).
func TestLoadDocProductsByDocsReadsAllRowsWithinWindow(t *testing.T) {
	const nDocs = 500
	const perDoc = 16 // 500 * 16 = 8000 rows (< MAX_RESULT_WINDOW 10000)
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
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs)
	if err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	count := 0
	for _, ps := range got {
		count += len(ps)
	}
	if count != nDocs*perDoc {
		t.Fatalf("recovered products = %d, want %d (silent truncation past the result window)", count, nDocs*perDoc)
	}
	// no duplicates: each (doc, product id) must appear once.
	seen := map[string]int{}
	for _, ps := range got {
		for _, p := range ps {
			seen[p.ID]++
		}
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("product %s recovered %d times, want 1 (duplicate across pages)", id, n)
		}
	}
}

// TestRecoverDocVariantsBatchChunksBounded guards review finding #1:
// RebuildDataset must not hold the entire KB's compiled products (including
// vectors) in memory at once. recoverDocVariantsBatch walks docs in
// docProductsBatchSize chunks, so each LoadDocProductsByDocs call stays bounded
// and the in-flight product set is released after each chunk.
func TestRecoverDocVariantsBatchChunksBounded(t *testing.T) {
	const nDocs = 1200
	docs := make([]string, 0, nDocs)
	products := make([]kccommon.Product, 0, nDocs)
	for i := 0; i < nDocs; i++ {
		id := fmt.Sprintf("d%d", i)
		docs = append(docs, id)
		products = append(products, kccommon.Product{DocID: id, Kind: "structure"})
	}
	fr := &fakeReader{products: products}
	c := &Consumer{reader: fr}
	got, err := c.recoverDocVariantsBatch(context.Background(), "t", "kb", docs)
	if err != nil {
		t.Fatalf("recoverDocVariantsBatch: %v", err)
	}
	if len(got) != nDocs {
		t.Fatalf("recovered docs = %d, want %d", len(got), nDocs)
	}
	for _, id := range docs {
		vr, ok := got[id]
		if !ok {
			t.Fatalf("missing recovery for %s", id)
		}
		if !reflect.DeepEqual(vr.variants, []string{string(kccommon.VariantStructure)}) {
			t.Fatalf("%s variants = %v, want [structure]", id, vr.variants)
		}
	}
	// Memory-bounding: every batch read must stay within docProductsBatchSize.
	for i, sz := range fr.batchSizes {
		if sz > docProductsBatchSize {
			t.Fatalf("batch %d size = %d, exceeds docProductsBatchSize %d (unbounded read)", i, sz, docProductsBatchSize)
		}
	}
	// 1200 docs chunked at 500 => exactly 3 calls (500 + 500 + 200).
	if len(fr.batchSizes) != 3 {
		t.Fatalf("batch calls = %d, want 3 (chunked into docProductsBatchSize)", len(fr.batchSizes))
	}
	if fr.batchSizes[0] != docProductsBatchSize || fr.batchSizes[1] != docProductsBatchSize || fr.batchSizes[2] != 200 {
		t.Fatalf("batch sizes = %v, want [500 500 200]", fr.batchSizes)
	}
}

// TestRecoverDocVariantsBatchDocWithoutProducts ensures a document with no
// compiled products yields empty variants (the caller skips it) rather than an
// error.
func TestRecoverDocVariantsBatchDocWithoutProducts(t *testing.T) {
	fr := &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
	}}
	c := &Consumer{reader: fr}
	got, err := c.recoverDocVariantsBatch(context.Background(), "t", "kb", []string{"d1", "d2"})
	if err != nil {
		t.Fatalf("recoverDocVariantsBatch: %v", err)
	}
	if vr, ok := got["d2"]; !ok || len(vr.variants) != 0 {
		t.Fatalf("d2 must recover empty variants, got %#v", vr)
	}
	if !reflect.DeepEqual(got["d1"].variants, []string{string(kccommon.VariantStructure)}) {
		t.Fatalf("d1 variants = %v, want [structure]", got["d1"].variants)
	}
}
