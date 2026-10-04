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

package knowledge_compile

import (
	"context"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestWikiReadersFilterDisabledDocuments(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}); err != nil {
		t.Fatalf("migrate documents: %v", err)
	}
	activeStatus := "1"
	disabledStatus := "0"
	for _, doc := range []*entity.Document{
		{ID: "active-doc", KbID: "kb1", Status: &activeStatus, ParserConfig: entity.JSONMap{}},
		{ID: "disabled-doc", KbID: "kb1", Status: &disabledStatus, ParserConfig: entity.JSONMap{}},
	} {
		if err := db.Create(doc).Error; err != nil {
			t.Fatalf("create document %s: %v", doc.ID, err)
		}
	}
	previousDB := kcDB
	kcDB = db
	t.Cleanup(func() { kcDB = previousDB })

	eng := &fakeEngine{searchChunks: []map[string]interface{}{{
		"id": "page-1", "doc_id": "active-doc", "compile_kwd": "wiki_page",
		"available_int": 0, "content_with_weight": "page content", "slug_kwd": "entity/page",
		"page_type_kwd": "entity", "title_kwd": "page", "source_doc_ids": []string{"active-doc"},
	}}}
	r := engineReader{eng: eng}
	if _, err := r.LoadDocumentWikiPagesBySlugs(t.Context(), "tenant", "kb1", []string{"entity/page"}); err != nil {
		t.Fatalf("load document Wiki pages: %v", err)
	}
	wikiFilter, ok := eng.lastSearchReq.Filter["doc_id"]
	if !ok || !reflect.DeepEqual(wikiFilter, []string{"active-doc"}) {
		t.Fatalf("document Wiki filter = %#v, want active document IDs", wikiFilter)
	}

	w := engineWriter{eng: eng}
	if _, err := w.loadActiveDocumentWikiPages(t.Context(), "tenant", "kb1"); err != nil {
		t.Fatalf("load active document Wiki pages: %v", err)
	}
	graphFilter, ok := eng.lastSearchReq.Filter["doc_id"]
	if !ok || !reflect.DeepEqual(graphFilter, []string{"active-doc"}) {
		t.Fatalf("graph Wiki filter = %#v, want active document IDs", graphFilter)
	}
}

// TestSearchSimilarFiltersByVariant asserts the B1/KNN contract: SearchSimilar
// scopes the engine query to available_int=1 AND compile_kwd=variant, and the
// in-memory dirty-row guard drops any row whose compile_kwd does not map back to
// the variant (so a foreign row can never become the merge candidate).
func TestSearchSimilarFiltersByVariant(t *testing.T) {
	eng := &fakeEngine{
		searchChunks: []map[string]interface{}{
			// dirty row: wrong variant (must be skipped by the in-memory guard).
			{
				"id":                  "dirty",
				"doc_id":              "kb",
				"available_int":       1,
				"compile_kwd":         "wiki_section", // not wiki_page
				"content_with_weight": "{\"c\":1}",
				"_score":              0.99,
			},
			// good row: correct variant (wiki_page => kind "page").
			{
				"id":                   "page1",
				"doc_id":               "kb",
				"available_int":        1,
				"compile_kwd":          "wiki_page",
				"content_with_weight":  "{\"c\":1}",
				"create_timestamp_flt": 1700000000.0,
				"create_time":          "2023-11-14T22:13:20Z",
				"_score":               0.95,
			},
		},
	}
	r := engineReader{eng: eng}

	p, score, err := r.SearchSimilar(context.Background(), "t1", "kb", kccommon.VariantWiki, []float64{0.1, 0.2, 0.3}, 2, 0.5)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if p.ID != "page1" {
		t.Fatalf("expected the wiki_page row to win, got %q", p.ID)
	}
	if p.Meta["kind"] != "page" {
		t.Fatalf("expected compile_kwd=wiki_page to map to kind 'page', got %v", p.Meta["kind"])
	}
	if p.Merged != true {
		t.Fatalf("expected merged=true for available_int=1 row")
	}
	if _, ok := p.Meta["created_at_unix"]; !ok {
		t.Fatalf("expected created_at_unix round-trip from create_timestamp_flt")
	}
	if score != 0.95 {
		t.Fatalf("expected _score=0.95, got %v", score)
	}

	// Structural filter: the engine request must scope to the variant.
	if eng.lastSearchReq == nil {
		t.Fatal("engine.Search was not called")
	}
	if eng.lastSearchReq.Filter["available_int"] != 1 {
		t.Fatalf("expected available_int=1 filter, got %v", eng.lastSearchReq.Filter["available_int"])
	}
	if eng.lastSearchReq.Filter["compile_kwd"] != string(compileKwdWikiPage) {
		t.Fatalf("expected compile_kwd=%q filter, got %v", compileKwdWikiPage, eng.lastSearchReq.Filter["compile_kwd"])
	}
	// SelectFields must include the round-trip columns added in the 8th review.
	for _, want := range []string{"compile_kwd", "content_with_weight", "create_timestamp_flt", "create_time"} {
		found := false
		for _, f := range eng.lastSearchReq.SelectFields {
			if f == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("SearchSimilar SelectFields missing %q", want)
		}
	}
}

// TestSearchSimilarSkipsNonMerged asserts a row with available_int=0 (a
// per-document row, not a dataset-level merged row) is never a KNN candidate.
func TestSearchSimilarSkipsNonMerged(t *testing.T) {
	eng := &fakeEngine{
		searchChunks: []map[string]interface{}{
			{
				"id":                  "doc1",
				"doc_id":              "doc",
				"available_int":       0,
				"compile_kwd":         "wiki_page",
				"content_with_weight": "{\"c\":1}",
				"_score":              0.99,
			},
		},
	}
	r := engineReader{eng: eng}
	p, _, err := r.SearchSimilar(context.Background(), "t1", "kb", kccommon.VariantWiki, []float64{0.1}, 1, 0.5)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if p.ID != "" {
		t.Fatalf("expected no candidate from a non-merged row, got %q", p.ID)
	}
}
