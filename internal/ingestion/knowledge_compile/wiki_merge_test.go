package knowledge_compile

import (
	"testing"
	"time"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestMergeWikiProductsBySlugKeepsSingleDocumentPageContent(t *testing.T) {
	page := kccommon.Product{
		ID:      "doc-page-1",
		DocID:   "doc-1",
		Variant: kccommon.VariantWiki,
		Content: "# Original\n\nDocument-level Wiki content.",
		Meta: map[string]any{
			"kind":      "page",
			"page_type": "entity",
			"slug":      "entity/alpha",
			"title":     "Alpha",
		},
	}

	consumer := &Consumer{}
	merged, err := consumer.mergeWikiProductsBySlug(t.Context(), "tenant-1", "kb-1", []kccommon.Product{page})
	if err != nil {
		t.Fatalf("mergeWikiProductsBySlug failed: %v", err)
	}
	if len(merged) != 1 {
		t.Fatalf("merged pages = %d, want 1", len(merged))
	}
	if merged[0].Content != page.Content {
		t.Fatalf("single-source content changed: got %q, want %q", merged[0].Content, page.Content)
	}
	if merged[0].DocID != "kb-1" || !merged[0].Merged {
		t.Fatalf("dataset page identity not applied: doc_id=%q merged=%t", merged[0].DocID, merged[0].Merged)
	}
}

// A bare slug and its "<page_type>/<slug>" form name the same stored wiki
// page: mergedChunkMap writes slug_kwd in the prefixed form, so the merge key
// and the dataset-level id must canonicalize the same way. Otherwise one page
// yields two rows that share slug_kwd but carry different ids, and a
// slug-filtered read returns whichever row the engine orders first.
func TestWikiPageIdentityNormalizesBareSlug(t *testing.T) {
	bare := kccommon.Product{
		Variant: kccommon.VariantWiki,
		Content: "body",
		Meta: map[string]any{
			"kind": "page", "page_type": "entity", "slug": "alpha", "title": "Alpha",
		},
	}
	full := kccommon.Product{
		Variant: kccommon.VariantWiki,
		Content: "body",
		Meta: map[string]any{
			"kind": "page", "page_type": "entity", "slug": "entity/alpha", "title": "Alpha",
		},
	}
	if got, want := canonicalKey(bare), canonicalKey(full); got != want {
		t.Fatalf("canonicalKey(bare) = %q, canonicalKey(full) = %q; want equal", got, want)
	}
	if got, want := canonicalKey(bare), "entity/alpha"; got != want {
		t.Fatalf("canonicalKey(bare) = %q, want %q", got, want)
	}
	if got, want := datasetLevelID("tenant-1", "kb-1", bare), datasetLevelID("tenant-1", "kb-1", full); got != want {
		t.Fatalf("datasetLevelID(bare) = %q, datasetLevelID(full) = %q; want equal", got, want)
	}
	if got, want := wikiPageMergeKey(bare), wikiPageMergeKey(full); got != want {
		t.Fatalf("wikiPageMergeKey(bare) = %q, wikiPageMergeKey(full) = %q; want equal", got, want)
	}
	row := mergedChunkMap("tenant-1", "kb-1", "run-1", "hash-1", time.Unix(0, 0), bare)
	if got, want := kccommon.MetaString(row, "slug_kwd"), "entity/alpha"; got != want {
		t.Fatalf("slug_kwd = %q, want %q", got, want)
	}
}

func TestWikiPageStorageSlugKeepsPrefixedSlugs(t *testing.T) {
	for _, tc := range []struct {
		pageType, slug, want string
	}{
		{"entity", "entity/person/alpha", "entity/person/alpha"},
		{"topic", "topic/history", "topic/history"},
		{"entity", " alpha ", "entity/alpha"},
		{"", "alpha", "alpha"},
		{"entity", "", ""},
	} {
		if got := wikiPageStorageSlug(tc.pageType, tc.slug); got != tc.want {
			t.Errorf("wikiPageStorageSlug(%q, %q) = %q, want %q", tc.pageType, tc.slug, got, tc.want)
		}
	}
}

// Sections and non-wiki products are not dataset-level pages; their identity
// must keep the raw slug so existing merged-row ids do not move.
func TestWikiPageIdentityLeavesNonPageProductsAlone(t *testing.T) {
	section := kccommon.Product{Variant: kccommon.VariantWiki, Meta: map[string]any{
		"kind": "section", "page_type": "entity", "slug": "body",
	}}
	if got := canonicalKey(section); got != "body" {
		t.Fatalf("canonicalKey(section) = %q, want raw section slug", got)
	}
}

func TestSelectMergedWikiTopicPathUsesSourceSupport(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "doc-1", Meta: map[string]any{
			"topic": "三国演义/人物/蜀汉人物", "source_doc_ids": []string{"doc-1"}, "source_chunk_ids": []string{"c1"},
		}},
		{DocID: "doc-2", Meta: map[string]any{
			"topic": "三国演义 / 人物 / 蜀汉人物", "source_doc_ids": []string{"doc-2"}, "source_chunk_ids": []string{"c2"},
		}},
		{DocID: "doc-3", Meta: map[string]any{
			"topic": "文学/人物", "source_doc_ids": []string{"doc-3"}, "source_chunk_ids": []string{"c3", "c4", "c5"},
		}},
	}
	if got := selectMergedWikiTopicPath(products); got != "三国演义/人物/蜀汉人物" {
		t.Fatalf("selected topic = %q, want path supported by two documents", got)
	}
}
