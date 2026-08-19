package knowledge_compile

import (
	"strings"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestMergeTopicProductsPreservesEvidenceAndStableTopicIdentity(t *testing.T) {
	products := []kccommon.Product{
		{
			ID: "doc-page-1", DocID: "doc-1", Variant: kccommon.VariantWiki, Content: "# History\n\nfirst",
			Meta: map[string]any{
				"kind": "page", "page_type": "topic", "topic": "History", "title": "History",
				"slug": "topic/history-a", "entity_names": []string{"Alice"},
				"source_doc_ids": []string{"doc-1"}, "source_chunk_ids": []string{"chunk-1"},
			},
		},
		{
			ID: "doc-page-2", DocID: "doc-2", Variant: kccommon.VariantWiki, Content: "# History\n\nsecond",
			Meta: map[string]any{
				"kind": "page", "page_type": "topic", "topic": " history ", "title": "History",
				"slug": "topic/history-b", "entity_names": []string{"Bob"},
				"source_doc_ids": []string{"doc-2"}, "source_chunk_ids": []string{"chunk-2"},
			},
		},
	}
	merged, stale := mergeTopicProducts("tenant", "kb", products)
	if len(merged) != 1 {
		t.Fatalf("merged pages = %d, want 1", len(merged))
	}
	if len(stale) != 0 {
		t.Fatalf("stale dataset pages = %v, want none for document products", stale)
	}
	if !strings.Contains(merged[0].Content, "first") || !strings.Contains(merged[0].Content, "second") {
		t.Fatalf("merged content lost evidence: %q", merged[0].Content)
	}
	if got := metaString(merged[0].Meta, "slug"); got != "topic/"+hashStr("history") {
		t.Fatalf("topic slug = %q, want deterministic topic slug", got)
	}
	if got := metaStringSlice(merged[0].Meta, "entity_names"); len(got) != 2 {
		t.Fatalf("entity names = %v, want both entities", got)
	}
}

func TestMergeTopicProductsKeepsDifferentTopicsSeparate(t *testing.T) {
	products := []kccommon.Product{
		{ID: "a", Variant: kccommon.VariantWiki, Content: "a", Meta: map[string]any{"kind": "page", "page_type": "topic", "topic": "History"}},
		{ID: "b", Variant: kccommon.VariantWiki, Content: "b", Meta: map[string]any{"kind": "page", "page_type": "topic", "topic": "Science"}},
	}
	merged, _ := mergeTopicProducts("tenant", "kb", products)
	if len(merged) != 2 {
		t.Fatalf("merged pages = %d, want 2", len(merged))
	}
}

func TestMergeTopicProductsRemovesSupersededDatasetPage(t *testing.T) {
	products := []kccommon.Product{
		{ID: "old-page", DocID: "kb", Merged: true, Variant: kccommon.VariantWiki, Content: "old", Meta: map[string]any{
			"kind": "page", "page_type": "topic", "topic": "History", "slug": "topic/old",
		}},
		{ID: "new-page", DocID: "kb", Merged: true, Variant: kccommon.VariantWiki, Content: "new", Meta: map[string]any{
			"kind": "page", "page_type": "topic", "topic": "History", "slug": "topic/new",
		}},
	}
	merged, stale := mergeTopicProducts("tenant", "kb", products)
	if len(merged) != 1 || len(stale) != 1 || stale[0] != "new-page" {
		t.Fatalf("merged=%#v stale=%#v, want one survivor and new-page stale", merged, stale)
	}
}
