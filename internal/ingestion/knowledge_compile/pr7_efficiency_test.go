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
	"reflect"
	"testing"

	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// ---------------------------------------------------------------------------
// Change 2: LoadDocProductsByDocs batch reader.
// ---------------------------------------------------------------------------

func TestLoadDocProductsByDocsGroupsByDoc(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "p1", "doc_id": "d1", "compile_kwd": "wiki_page", "available_int": 1, "content_with_weight": "c1"},
		{"id": "p2", "doc_id": "d2", "compile_kwd": "wiki_page", "available_int": 1, "content_with_weight": "c2"},
		{"id": "p3", "doc_id": "d1", "compile_kwd": "structure", "available_int": 1, "content_with_weight": "c3"},
	}}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", []string{"d1", "d2"})
	if err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	filter, ok := eng.lastSearchReq.Filter["doc_id"]
	if !ok {
		t.Fatal("missing doc_id filter")
	}
	if !reflect.DeepEqual(filter, []interface{}{"d1", "d2"}) {
		t.Fatalf("doc_id filter = %#v, want []interface{}{\"d1\",\"d2\"}", filter)
	}
	if len(got["d1"]) != 2 {
		t.Fatalf("d1 products = %d, want 2", len(got["d1"]))
	}
	if len(got["d2"]) != 1 {
		t.Fatalf("d2 products = %d, want 1", len(got["d2"]))
	}
}

func TestLoadDocProductsByDocsEmptySkipsSearch(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "p1", "doc_id": "d1", "compile_kwd": "wiki_page", "available_int": 1, "content_with_weight": "c1"},
	}}
	r := engineReader{eng: eng}
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", nil)
	if err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty docIDs must return empty map, got %d entries", len(got))
	}
	if eng.searchCalls != 0 {
		t.Fatalf("empty docIDs must not issue a Search, got %d calls", eng.searchCalls)
	}
}

func TestLoadDocProductsByDocsDocWithoutProducts(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "p1", "doc_id": "d1", "compile_kwd": "wiki_page", "available_int": 1, "content_with_weight": "c1"},
	}}
	r := engineReader{eng: eng}
	// d2 has no matching rows but is requested: should be present with an empty slice.
	got, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", []string{"d1", "d2"})
	if err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	if len(got["d1"]) != 1 {
		t.Fatalf("d1 products = %d, want 1", len(got["d1"]))
	}
	if v := got["d2"]; len(v) != 0 {
		t.Fatalf("d2 must be empty slice, got %d", len(v))
	}
}

func TestLoadDocProductsByDocsChunksLargeSet(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "p1", "doc_id": "d1", "compile_kwd": "wiki_page", "available_int": 1, "content_with_weight": "c1"},
	}}
	r := engineReader{eng: eng}
	docIDs := make([]string, 0, 1200)
	for i := 0; i < 1200; i++ {
		docIDs = append(docIDs, "doc-"+itoa(i))
	}
	if _, err := r.LoadDocProductsByDocs(context.Background(), "tenant", "kb", docIDs); err != nil {
		t.Fatalf("LoadDocProductsByDocs: %v", err)
	}
	// 1200 docs at a 500-doc batch must split into 3 searches.
	if len(eng.searchReqs) != 3 {
		t.Fatalf("search calls = %d, want 3 (chunked)", len(eng.searchReqs))
	}
	wantSizes := []int{500, 500, 200}
	for i, req := range eng.searchReqs {
		filter, ok := req.Filter["doc_id"].([]interface{})
		if !ok {
			t.Fatalf("req %d missing doc_id []interface{} filter", i)
		}
		if len(filter) != wantSizes[i] {
			t.Fatalf("req %d filter size = %d, want %d", i, len(filter), wantSizes[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Change 1: buildTopicPagesByKey excludes pages deleted in the same merge step.
// ---------------------------------------------------------------------------

func TestBuildTopicPagesByKeyExcludesDeleted(t *testing.T) {
	pages := []kccommon.Product{
		{ID: "keep", DocID: "kb", Meta: map[string]any{"page_type": "topic", "topic": "a"}},
		{ID: "del", DocID: "kb", Meta: map[string]any{"page_type": "topic", "topic": "b"}},
		{ID: "nt", DocID: "kb", Meta: map[string]any{"page_type": "entity", "topic": "c"}},
	}
	// mark "del" as a topic page by isTopicPage; isTopicPage reads Meta page_type.
	excluded := map[string]struct{}{"del": {}}
	got := buildTopicPagesByKey(pages, excluded)
	// "keep" topic page survives; "del" (deleted) and "nt" (non-topic) excluded.
	if _, ok := got["a"]; !ok {
		t.Fatalf("surviving topic page key \"a\" missing: %v", keysOf(got))
	}
	if _, ok := got["b"]; ok {
		t.Fatalf("deleted topic page key \"b\" must be excluded: %v", keysOf(got))
	}
	if _, ok := got["c"]; ok {
		t.Fatalf("non-topic page key \"c\" must be excluded: %v", keysOf(got))
	}
}

// ---------------------------------------------------------------------------
// Change 3: recoverDocTypesFrom operates on pre-loaded products (batch path).
// ---------------------------------------------------------------------------

func TestRecoverDocTypesFromEquivalent(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
		{DocID: "d1", Kind: "tree"},
		{DocID: "d1", Kind: "structure"}, // duplicate variant, deduped
	}
	got, taskTypes, err := recoverDocTypesFrom(products)
	if err != nil {
		t.Fatalf("recoverDocTypesFrom: %v", err)
	}
	want := []string{string(kccommon.VariantStructure), string(kccommon.VariantTree)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("variants = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(taskTypes, []string{kccommon.TaskTypeGraph, kccommon.TaskTypeTree}) {
		t.Fatalf("task types = %v, want [Graph Tree]", taskTypes)
	}
}

func TestRecoverDocTypesFromUnknownKindHardFails(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
		{DocID: "d1", Kind: "garbage"},
	}
	if _, _, err := recoverDocTypesFrom(products); err == nil {
		t.Fatal("recoverDocTypesFrom must hard-fail on an unknown authoritative kind")
	}
}

func TestRecoverDocTypesFromEmpty(t *testing.T) {
	got, taskTypes, err := recoverDocTypesFrom(nil)
	if err != nil {
		t.Fatalf("recoverDocTypesFrom(nil): %v", err)
	}
	if len(got) != 0 || len(taskTypes) != 0 {
		t.Fatalf("empty products must yield empty variants/taskTypes, got %v/%v", got, taskTypes)
	}
}

// ---------------------------------------------------------------------------
// Change 4: diagnostic construction helpers (gated by IsDebugEnabled at call site).
// ---------------------------------------------------------------------------

func TestComputeDedupVectorAudit(t *testing.T) {
	incoming := []kccommon.Product{{Vector: []float32{1}}, {}}
	candidates := []kccommon.Product{{Vector: []float32{1}}, {Vector: []float32{1}}, {}}
	inVec, candVec := computeDedupVectorAudit(incoming, candidates)
	if inVec != 1 || candVec != 2 {
		t.Fatalf("computeDedupVectorAudit = (%d,%d), want (1,2)", inVec, candVec)
	}
}

func TestComputeCandidatesBreakdown(t *testing.T) {
	candidates := []kccommon.Product{
		{DocID: "kb", Variant: kccommon.VariantWiki, Meta: map[string]any{"slug": "a"}},
		{DocID: "kb", Variant: kccommon.VariantWiki, Meta: map[string]any{"slug": "b"}},
		{DocID: "kb", Variant: kccommon.VariantTree},
	}
	byVariant, slugs := computeCandidatesBreakdown(candidates)
	if byVariant[string(kccommon.VariantWiki)] != 2 || byVariant[string(kccommon.VariantTree)] != 1 {
		t.Fatalf("byVariant = %v, want wiki=2 tree=1", byVariant)
	}
	if len(slugs) != 2 {
		t.Fatalf("wiki slugs = %d, want 2", len(slugs))
	}
}

func TestComputeDocProductsVectorAudit(t *testing.T) {
	products := []kccommon.Product{{Vector: make([]float32, 4)}, {Vector: make([]float32, 4)}, {}}
	vecCount, dims := computeDocProductsVectorAudit(products)
	if vecCount != 2 {
		t.Fatalf("vecCount = %d, want 2", vecCount)
	}
	if dims[4] != 2 {
		t.Fatalf("dims[4] = %d, want 2", dims[4])
	}
}

func TestComputeKNNGroupDumps(t *testing.T) {
	groups := map[string]*mergeGroup{
		"g1": {existing: kccommon.Product{ID: "e1"}, candidates: []kccommon.Product{{DocID: "kb"}, {DocID: "kb"}}, score: 0.9},
	}
	dumps := computeKNNGroupDumps(groups)
	if len(dumps) != 1 {
		t.Fatalf("dumps = %d, want 1", len(dumps))
	}
	if dumps[0].Existing != "e1" || len(dumps[0].Candidates) != 2 || dumps[0].Score != 0.9 {
		t.Fatalf("dump = %+v, want Existing=e1 Candidates=2 Score=0.9", dumps[0])
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func keysOf(m map[string]kccommon.Product) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = types.SearchRequest{}
