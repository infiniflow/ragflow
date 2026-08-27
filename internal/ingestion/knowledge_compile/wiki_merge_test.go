package knowledge_compile

import (
	"testing"

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
