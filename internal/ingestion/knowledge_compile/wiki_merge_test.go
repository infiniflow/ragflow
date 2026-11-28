package knowledge_compile

import (
	"context"
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

type rewriteChatStub struct{ content string }

func (s rewriteChatStub) Chat(context.Context, kccommon.ChatRequest) (*kccommon.ChatResponse, error) {
	return &kccommon.ChatResponse{Content: s.content}, nil
}

func TestMergeWikiProductsBySlugRefreshesSummaryAfterRewrite(t *testing.T) {
	kccommon.SetDepsResolver(func(context.Context, string, string, string) (kccommon.Deps, error) {
		return kccommon.Deps{Chat: rewriteChatStub{
			content: "# Return Policy\n\nReturns are accepted within 14 days of delivery.\n\n## Details\n\nMerged from two documents.",
		}}, nil
	})
	t.Cleanup(func() { kccommon.SetDepsResolver(nil) })

	newPage := func(id, docID, content, summary string) kccommon.Product {
		return kccommon.Product{
			ID:      id,
			DocID:   docID,
			Variant: kccommon.VariantWiki,
			Content: content,
			Meta: map[string]any{
				"kind":      "page",
				"page_type": "entity",
				"slug":      "entity/return-policy",
				"title":     "Return Policy",
				"summary":   summary,
			},
		}
	}
	older := newPage("page-1", "doc-1", "# Return Policy\n\nReturns are not accepted.", "Returns are not accepted.")
	newer := newPage("page-2", "doc-2", "# Return Policy\n\nReturns are accepted within 14 days of delivery.", "Stale candidate summary.")

	consumer := &Consumer{}
	merged, err := consumer.mergeWikiProductsBySlug(t.Context(), "tenant-1", "kb-1", []kccommon.Product{older, newer})
	if err != nil {
		t.Fatalf("mergeWikiProductsBySlug failed: %v", err)
	}
	if len(merged) != 1 {
		t.Fatalf("merged pages = %d, want 1", len(merged))
	}
	const want = "Returns are accepted within 14 days of delivery."
	if got := kccommon.MetaString(merged[0].Meta, "summary"); got != want {
		t.Fatalf("summary after rewrite = %q, want %q", got, want)
	}
}
