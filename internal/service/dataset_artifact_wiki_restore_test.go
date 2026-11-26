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

package service

import (
	"slices"
	"testing"
)

// TestWikiPageIdentityKey pins how a page is addressed in the engine: page type
// and slug joined by a slash, the same value stored as slug_kwd.
func TestWikiPageIdentityKey(t *testing.T) {
	page := WikiPageIdentity{PageType: "entity", Slug: "alpha"}
	if got := page.Key(); got != "entity/alpha" {
		t.Fatalf("Key() = %q, want entity/alpha", got)
	}
}

// TestWikiPageStateHoldsPage covers the comparison a restore makes instead of
// rewriting every page: only a page whose body and headers all match is left
// alone, so a version that moved a single page writes a single page.
func TestWikiPageStateHoldsPage(t *testing.T) {
	snapshot := map[string]interface{}{
		"content_with_weight": "# Alpha",
		"title_kwd":           "Alpha",
		"summary_with_weight": "the alpha page",
		"topic_kwd":           "concepts",
	}
	held := WikiPageState{
		ID:      "page-1",
		Content: "# Alpha",
		Title:   "Alpha",
		Summary: "the alpha page",
		Topic:   "concepts",
	}
	if !held.HoldsPage(snapshot) {
		t.Fatal("expected a page whose body and headers all match to be left alone")
	}

	// The fields a page edit moves together are all compared, because the restore
	// has to put the version's page back as a whole - a body that matches is not
	// enough when the summary still describes a later edit. (The snapshot's other
	// fields are written back but not compared; see HoldsPage for why.)
	for _, differing := range []WikiPageState{
		{Content: "# Alpha, edited after the version", Title: "Alpha", Summary: "the alpha page", Topic: "concepts"},
		{Content: "# Alpha", Title: "Alpha (renamed)", Summary: "the alpha page", Topic: "concepts"},
		{Content: "# Alpha", Title: "Alpha", Summary: "another summary", Topic: "concepts"},
		{Content: "# Alpha", Title: "Alpha", Summary: "the alpha page", Topic: "entities"},
	} {
		if differing.HoldsPage(snapshot) {
			t.Errorf("expected a page differing in %q to be written back", differing)
		}
	}

	// A page the engine does not hold at all reads as differing, which is what
	// makes a restore rebuild it from the snapshot.
	if (WikiPageState{}).HoldsPage(snapshot) {
		t.Error("expected an absent page to differ from the version's page")
	}

	// A snapshot written before the history stored whole pages carries only a body.
	// The restore cannot write the headers such a snapshot does not carry, so they
	// must not count as a difference - otherwise that page is rewritten on every
	// restore and never converges.
	legacy := map[string]interface{}{"content_with_weight": "# Alpha"}
	if !held.HoldsPage(legacy) {
		t.Error("expected a body-only snapshot to match a page whose body already agrees")
	}
	if (WikiPageState{Content: "# Alpha, edited later"}).HoldsPage(legacy) {
		t.Error("expected a differing body to be written back even for a body-only snapshot")
	}
}

// TestWikiPageStateComparesLinks covers the field that moves without the body:
// UpdateWikiPage edits a page's links on their own, so a restore that compared only
// the body and headers would report success and leave the newer links in place.
// The comparison is by membership - the engine owns the list's order - and a
// snapshot from before links were stored must not count them at all.
func TestWikiPageStateComparesLinks(t *testing.T) {
	snapshot := map[string]interface{}{
		"content_with_weight": "# Alpha",
		"outlinks_kwd":        []string{"entity/beta", "entity/gamma"},
	}
	sameLinks := WikiPageState{
		Content: "# Alpha",
		// Same links, other order, read back from the engine as []interface{}.
		Outlinks: canonicalWikiPageList([]interface{}{"entity/gamma", "entity/beta"}),
	}
	if !sameLinks.HoldsPage(snapshot) {
		t.Error("expected a page carrying the same links in another order to be left alone")
	}

	edited := sameLinks
	edited.Outlinks = canonicalWikiPageList([]string{"entity/beta"})
	if edited.HoldsPage(snapshot) {
		t.Error("expected a page whose links were edited to be written back")
	}

	// A snapshot written before links were stored carries none: counting them would
	// rewrite that page on every restore.
	if !(WikiPageState{Content: "# Alpha", Outlinks: canonicalWikiPageList([]string{"entity/beta"})}).HoldsPage(
		map[string]interface{}{"content_with_weight": "# Alpha"}) {
		t.Error("expected a link-less snapshot to leave the links alone")
	}
}

// TestWikiPageStateProjectionCarriesTheKey pins the projection a restore reads its
// current state through: the answer is stored under slug_kwd, so a projection that
// leaves it out reads every page as absent - and a restore answers an absent page
// by rebuilding it, which spends a page version on a page that never moved.
func TestWikiPageStateProjectionCarriesTheKey(t *testing.T) {
	if !slices.Contains(wikiPageStateFields, "slug_kwd") {
		t.Fatalf("wikiPageStateFields = %v, want it to carry the slug_kwd key its result map is built from", wikiPageStateFields)
	}
	if !slices.Contains(wikiPageStateFields, "outlinks_kwd") {
		t.Fatalf("wikiPageStateFields = %v, want it to project the links the comparison reads", wikiPageStateFields)
	}
	if !slices.Contains(wikiPageExistenceFields, "slug_kwd") {
		t.Fatalf("wikiPageExistenceFields = %v, want it to carry the slug_kwd key", wikiPageExistenceFields)
	}
}

// TestWikiPageRestoreFieldsWritesOnlyWhatTheSnapshotCarries pins what a rebuilt
// page is made of: the snapshot's own fields plus the identity that keeps the page
// addressable. A field the snapshot does not carry is left out rather than written
// empty - a body-only snapshot from before whole pages were stored would otherwise
// blank the summary, links and source ids the engine still holds.
func TestWikiPageRestoreFieldsWritesOnlyWhatTheSnapshotCarries(t *testing.T) {
	page := WikiPageIdentity{PageType: "entity", Slug: "alpha"}
	fields := wikiPageRestoreFields(page, map[string]interface{}{
		"content_with_weight": "# Alpha",
		"title_kwd":           "Alpha",
	})

	if got := fields["content_with_weight"]; got != "# Alpha" {
		t.Errorf("content_with_weight = %v, want the snapshot's body", got)
	}
	if got := fields["slug_kwd"]; got != "entity/alpha" {
		t.Errorf("slug_kwd = %v, want the page key", got)
	}
	if got := fields["page_type_kwd"]; got != "entity" {
		t.Errorf("page_type_kwd = %v, want the page type", got)
	}
	// Only merged pages are read and restored, so a rebuilt one has to say so.
	if got := fields["available_int"]; got != 1 {
		t.Errorf("available_int = %v, want 1", got)
	}
	for _, absent := range []string{"summary_with_weight", "topic_kwd", "outlinks_kwd", "source_chunk_ids"} {
		if _, ok := fields[absent]; ok {
			t.Errorf("%s was written from a snapshot that does not carry it", absent)
		}
	}
}
