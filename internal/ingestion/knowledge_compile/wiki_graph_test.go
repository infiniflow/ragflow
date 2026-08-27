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
	"encoding/json"
	"testing"
)

// page builds a wikiPageProjection with the full "<page_type>/<slug>" identity.
func page(pageType, slug string, outlinks ...string) wikiPageProjection {
	return wikiPageProjection{
		Slug:     pageType + "/" + slug,
		PageType: pageType,
		Title:    slug,
		Outlinks: outlinks,
	}
}

func findEntity(rows []map[string]interface{}, slug string) map[string]interface{} {
	for _, r := range rows {
		if r["compile_kwd"] == compileKwdWikiEntity && r["slug_kwd"] == slug {
			return r
		}
	}
	return nil
}

func findRelation(rows []map[string]interface{}, from, to string) map[string]interface{} {
	for _, r := range rows {
		if r["compile_kwd"] == compileKwdWikiRelation && r["from_kwd"] == from && r["to_kwd"] == to {
			return r
		}
	}
	return nil
}

const kbForTest = "kb1"

// TestProjectWikiGraphRowsCrossBatchEdgeSurvives verifies a relation whose two
// endpoints land in different "batches" (here modeled as two pages) is still
// materialized: the projection is global over the full page set, so edges never
// dangle due to batching.
func TestProjectWikiGraphRowsCrossBatchEdgeSurvives(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "alpha", "entity/beta"),
		page("entity", "beta"),
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if findEntity(rows, "entity/alpha") == nil {
		t.Fatalf("missing entity entity/alpha")
	}
	if findEntity(rows, "entity/beta") == nil {
		t.Fatalf("missing entity entity/beta")
	}
	if findRelation(rows, "entity/alpha", "entity/beta") == nil {
		t.Fatalf("cross-page edge entity/alpha -> entity/beta not materialized")
	}
}

// TestProjectWikiGraphRowsDropAfterReprojection verifies a page removed from the
// set is gone from the reprojected graph: no orphaned entity and no dangling
// relation remain.
func TestProjectWikiGraphRowsDropAfterReprojection(t *testing.T) {
	w := engineWriter{}
	// First projection has alpha -> beta and beta -> gamma.
	first := []wikiPageProjection{
		page("entity", "alpha", "entity/beta"),
		page("entity", "beta", "entity/gamma"),
		page("entity", "gamma"),
	}
	r1, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, first)
	if err != nil {
		t.Fatalf("project1: %v", err)
	}
	if findRelation(r1, "entity/beta", "entity/gamma") == nil {
		t.Fatalf("expected beta->gamma before drop")
	}

	// beta is deleted: reproject from alpha + gamma only.
	second := []wikiPageProjection{
		page("entity", "alpha"), // alpha no longer links anywhere
		page("entity", "gamma"),
	}
	r2, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, second)
	if err != nil {
		t.Fatalf("project2: %v", err)
	}
	if findEntity(r2, "entity/beta") != nil {
		t.Fatalf("beta should be gone after drop")
	}
	if findRelation(r2, "entity/alpha", "entity/beta") != nil {
		t.Fatalf("orphan alpha->beta must not survive after beta dropped")
	}
	if findRelation(r2, "entity/beta", "entity/gamma") != nil {
		t.Fatalf("orphan beta->gamma must not survive after beta dropped")
	}
}

// TestProjectWikiGraphRowsZeroOutlinkWeightIsZero verifies a page with no
// outlinks gets weight_int == 0 (Python parity: raw outlink count, 0 allowed).
func TestProjectWikiGraphRowsZeroOutlinkWeightIsZero(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{page("entity", "lonely")}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	e := findEntity(rows, "entity/lonely")
	if e == nil {
		t.Fatalf("missing entity")
	}
	if wt, ok := e["weight_int"].(int); !ok || wt != 0 {
		t.Fatalf("expected weight_int==0 for zero-outlink page, got %v", e["weight_int"])
	}
}

// TestProjectWikiGraphRowsWeightEqualsOutlinkCount verifies weight_int equals
// the raw outlink count (N outlinks -> weight N), not max(1, N).
func TestProjectWikiGraphRowsWeightEqualsOutlinkCount(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "hub", "entity/a", "entity/b", "entity/c"),
		page("entity", "a"),
		page("entity", "b"),
		page("entity", "c"),
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	e := findEntity(rows, "entity/hub")
	if e == nil {
		t.Fatalf("missing hub entity")
	}
	if wt, ok := e["weight_int"].(int); !ok || wt != 3 {
		t.Fatalf("expected weight_int==3 for 3-outlink page, got %v", e["weight_int"])
	}
}

// TestProjectWikiGraphRowsBareSlugOutlinkCompletesGraph asserts the G1 reader
// parity fix: an outlink given as a bare slug ("beta", not "entity/beta") is
// resolved to the full slug via the page slug map and still yields a relation,
// so the wiki nav graph is complete (matching Python dataset_wiki_generator).
func TestProjectWikiGraphRowsBareSlugOutlinkCompletesGraph(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "alpha", "beta"), // bare outlink resolves to entity/beta
		page("entity", "beta"),
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if findRelation(rows, "entity/alpha", "entity/beta") == nil {
		t.Fatalf("bare-slug outlink must resolve to a relation entity/alpha->entity/beta")
	}
}

// TestProjectWikiGraphRowsDropsDanglingAndSelfLoop asserts a dangling outlink
// (target page absent) and a self-loop (alpha->alpha) are skipped, so the graph
// never references non-existent vertices.
func TestProjectWikiGraphRowsDropsDanglingAndSelfLoop(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "alpha", "alpha", "ghost"), // self-loop + dangling
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if findRelation(rows, "entity/alpha", "entity/alpha") != nil {
		t.Fatalf("self-loop must be skipped")
	}
	if findRelation(rows, "entity/alpha", "entity/ghost") != nil {
		t.Fatalf("dangling outlink must be skipped")
	}
}

// TestProjectWikiGraphRowsFieldMapping verifies the reader-facing display
// columns are written (aliases_kwd from entity_names_kwd, description_with_weight
// from summary_with_weight), and the id is the xxhash colon-namespaced form.
func TestProjectWikiGraphRowsFieldMapping(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		{
			Slug:         "concept/alpha",
			PageType:     "concept",
			Title:        "Alpha",
			Aliases:      []string{"A", "Alpha Prime"},
			Summary:      "the alpha page",
			Outlinks:     nil,
			SourceDocIDs: []string{"d1", "d2"},
		},
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// Use page_type "concept" (not "entity") so the writer's entity_type_kwd
	// mapping ("wiki_"+PageType) is observable — asserting wiki_entity would
	// match by coincidence with the compileKwdWikiEntity constant.
	e := findEntity(rows, "concept/alpha")
	if e == nil {
		t.Fatalf("missing entity")
	}
	if e["aliases_kwd"] == nil {
		t.Fatalf("expected aliases_kwd (mapped from entity_names_kwd) to be set")
	}
	if e["description_with_weight"] != "the alpha page" {
		t.Fatalf("expected description_with_weight=summary, got %v", e["description_with_weight"])
	}
	if e["entity_type_kwd"] != "wiki_concept" {
		t.Fatalf("expected entity_type_kwd=wiki_concept, got %v", e["entity_type_kwd"])
	}
	// id format: wiki_entity:{kb}:{slug} xxhashed to 16 hex chars.
	id, _ := e["id"].(string)
	if len(id) != 16 {
		t.Fatalf("expected 16-char xxhash id, got %q (len %d)", id, len(id))
	}
	// Recompute and compare against wikiGraphXXHash.
	wantID := wikiGraphXXHash("wiki_entity", kbForTest, "concept/alpha")
	if id != wantID {
		t.Fatalf("id mismatch: got %q want %q", id, wantID)
	}
	// content_with_weight JSON must round-trip the slug/page_type/title/aliases/summary/weight.
	raw, _ := e["content_with_weight"].(string)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("content_with_weight not valid JSON: %v", err)
	}
	if decoded["slug"] != "concept/alpha" || decoded["page_type"] != "concept" {
		t.Fatalf("content_with_weight missing slug/page_type: %v", decoded)
	}
}

// TestProjectWikiGraphRowsBareSlugNoCollision verifies two pages with the same
// bare slug but different page types get distinct entities (full-slug identity).
func TestProjectWikiGraphRowsBareSlugNoCollision(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "foo"),
		page("concept", "foo"),
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if findEntity(rows, "entity/foo") == nil || findEntity(rows, "concept/foo") == nil {
		t.Fatalf("both entity/foo and concept/foo must exist as distinct entities")
	}
	e1 := findEntity(rows, "entity/foo")
	e2 := findEntity(rows, "concept/foo")
	if e1["id"] == e2["id"] {
		t.Fatalf("entity/foo and concept/foo must have distinct ids")
	}
}

// TestProjectWikiGraphRowsDanglingEdgeSkipped verifies an outlink to a page not
// present in the projection is skipped (no dangling relation).
func TestProjectWikiGraphRowsDanglingEdgeSkipped(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "alpha", "entity/missing"),
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if findRelation(rows, "entity/alpha", "entity/missing") != nil {
		t.Fatalf("dangling edge to missing target must be skipped")
	}
	if findEntity(rows, "entity/missing") != nil {
		t.Fatalf("missing target must not be materialized as an entity")
	}
}

// TestProjectWikiGraphRowsSelfLoopSkipped verifies a page whose outlinks include
// itself does not produce a self-edge (Python parity: src == tgt is skipped).
func TestProjectWikiGraphRowsSelfLoopSkipped(t *testing.T) {
	w := engineWriter{}
	pages := []wikiPageProjection{
		page("entity", "alpha", "entity/alpha"),
	}
	rows, err := w.projectWikiGraphRows(context.Background(), "t1", kbForTest, pages)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if findRelation(rows, "entity/alpha", "entity/alpha") != nil {
		t.Fatalf("self-loop edge must be skipped (Python parity)")
	}
	if findEntity(rows, "entity/alpha") == nil {
		t.Fatalf("the self-referencing entity must still exist")
	}
}
