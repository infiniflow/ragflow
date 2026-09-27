//go:build integration
// +build integration

package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/server"
)

// wikiIntRow builds a merged dataset-level wiki_page row shaped like the
// knowledge-compiler writer output that the artifact APIs read.
func wikiIntRow(id, pageType, slug, title, topic, summary string, outlinks int) map[string]interface{} {
	fullSlug := pageType + "/" + slug
	return map[string]interface{}{
		"id":                  id,
		"doc_id":              id,
		"available_int":       1,
		"scope_kwd":           "dataset",
		"compile_kwd":         CompileKwdWikiPage,
		"slug_kwd":            fullSlug,
		"artifact_slug_kwd":   fullSlug,
		"title_kwd":           title,
		"page_type_kwd":       pageType,
		"topic_kwd":           topic,
		"summary_with_weight": summary,
		"outlinks_int":        outlinks,
	}
}

// TestWikiKeywordFilter_Integration seeds wiki_page rows into a real document
// engine and asserts ListWikiPages/ListWikiTopics keyword semantics (mirrors
// Python list_wiki_pages/list_wiki_topics): pages match by title/slug/summary
// only; topics match by their own path or by holding a matching page.
//
// Run with: bash build.sh --test-integration ./internal/service/...
func TestWikiKeywordFilter_Integration(t *testing.T) {
	if err := common.InitLogger("info", common.FileOutput{}, ""); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	if err := server.Init(""); err != nil {
		t.Fatalf("init service config: %v", err)
	}
	if err := engine.InitDocEngine(context.Background()); err != nil {
		t.Skipf("no document engine: %v", err)
	}
	de := engine.Get()
	if de == nil {
		t.Skip("no live document engine configured")
	}

	tenantID := "wikiint_t1"
	kbID := "wikiint_kb1"
	idx := wikiIndexName(tenantID)
	ctx := t.Context()

	rows := []map[string]interface{}{
		wikiIntRow("wikiint_p1", "entity", "Daisy", "Daisy", "General", "the field flower", 3),
		wikiIntRow("wikiint_p2", "entity", "Swallow", "Swallow", "General", "the little swallow", 5),
		wikiIntRow("wikiint_p3", "concept", "Love", "Love", "Themes", "devotion and kindness", 7),
	}
	for _, row := range rows {
		row["kb_id"] = kbID
	}
	// Best effort: the store may already exist from a previous run.
	_ = de.CreateChunkStore(ctx, idx, kbID, 1024, "naive")
	if _, err := de.InsertChunks(ctx, rows, idx, kbID); err != nil {
		t.Fatalf("seed wiki rows: %v", err)
	}
	t.Cleanup(func() {
		_, _ = de.DeleteChunks(context.Background(),
			map[string]interface{}{"kb_id": []string{kbID}}, idx, kbID)
	})

	svc := NewDatasetArtifactService()

	// Baseline: no keywords returns every seeded page.
	all, total, err := svc.ListWikiPages(ctx, tenantID, kbID, "", "", "", 1, 30)
	if err != nil {
		t.Fatalf("list pages: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("unfiltered pages = %#v (total %d), want 3", all, total)
	}

	// Keyword filters by title, case-insensitively.
	got, total, err := svc.ListWikiPages(ctx, tenantID, kbID, "", "", "dAiSy", 1, 30)
	if err != nil {
		t.Fatalf("list pages with keyword: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].Title != "Daisy" || got[0].Slug != "Daisy" {
		t.Fatalf("keyword dAiSy pages = %#v (total %d), want only Daisy", got, total)
	}

	// Keyword matches the summary too ("devotion" only occurs in Love's summary).
	got, total, err = svc.ListWikiPages(ctx, tenantID, kbID, "", "", "devotion", 1, 30)
	if err != nil {
		t.Fatalf("list pages with summary keyword: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].Title != "Love" {
		t.Fatalf("keyword devotion pages = %#v (total %d), want only Love", got, total)
	}

	// The page topic is navigation metadata, not a searchable page field:
	// "themes" occurs only in Love's topic, so it must match no page.
	got, total, err = svc.ListWikiPages(ctx, tenantID, kbID, "", "", "themes", 1, 30)
	if err != nil {
		t.Fatalf("list pages with topic keyword: %v", err)
	}
	if total != 0 || len(got) != 0 {
		t.Fatalf("keyword themes pages = %#v (total %d), want none", got, total)
	}

	topics, ttotal, err := svc.ListWikiTopics(ctx, tenantID, kbID, "")
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}
	if ttotal != 2 || len(topics) != 2 {
		t.Fatalf("unfiltered topics = %#v (total %d), want General and Themes", topics, ttotal)
	}

	// A page-name keyword keeps the topic holding that page (Daisy under General).
	topics, ttotal, err = svc.ListWikiTopics(ctx, tenantID, kbID, "daisy")
	if err != nil {
		t.Fatalf("list topics with page keyword: %v", err)
	}
	if ttotal != 1 || len(topics) != 1 || topics[0].Topic != "General" || topics[0].PageCount != 2 {
		t.Fatalf("keyword daisy topics = %#v (total %d), want General with 2 pages", topics, ttotal)
	}

	// A keyword matching the topic path itself keeps that topic.
	topics, ttotal, err = svc.ListWikiTopics(ctx, tenantID, kbID, "themes")
	if err != nil {
		t.Fatalf("list topics with path keyword: %v", err)
	}
	if ttotal != 1 || len(topics) != 1 || topics[0].Topic != "Themes" {
		t.Fatalf("keyword themes topics = %#v (total %d), want Themes", topics, ttotal)
	}

	// No match anywhere yields an empty topic list.
	topics, ttotal, err = svc.ListWikiTopics(ctx, tenantID, kbID, "nonexistent")
	if err != nil {
		t.Fatalf("list topics with unknown keyword: %v", err)
	}
	if ttotal != 0 || len(topics) != 0 {
		t.Fatalf("keyword nonexistent topics = %#v (total %d), want none", topics, ttotal)
	}
}
