package knowledge_compile

import (
	"context"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func wikiContributionTestProduct(slug, content string, chunkIDs ...string) kccommon.Product {
	return kccommon.Product{
		ID:      "doc-page-" + slug,
		DocID:   "doc-1",
		Variant: kccommon.VariantWiki,
		Content: content,
		Meta: map[string]any{
			"kind":             "page",
			"page_type":        "entity",
			"slug":             slug,
			"title":            slug,
			"source_doc_ids":   []string{"doc-1"},
			"source_chunk_ids": chunkIDs,
		},
	}
}

func TestDiffWikiDocumentContributionIgnoresUnchangedPages(t *testing.T) {
	product := wikiContributionTestProduct("cao-cao", "same content", "chunk-1")
	previous := buildWikiDocumentContribution("doc-1", []kccommon.Product{product})
	current := buildWikiDocumentContribution("doc-1", []kccommon.Product{product})
	keys, slugs := diffWikiDocumentContribution(previous, current)
	if len(keys) != 0 || len(slugs) != 0 {
		t.Fatalf("unchanged contribution produced diff: keys=%v slugs=%v", keys, slugs)
	}
}

func TestDiffWikiDocumentContributionIncludesChangedAndRemovedPages(t *testing.T) {
	previous := buildWikiDocumentContribution("doc-1", []kccommon.Product{
		wikiContributionTestProduct("cao-cao", "old content", "chunk-1"),
		wikiContributionTestProduct("liu-bei", "removed content", "chunk-2"),
	})
	current := buildWikiDocumentContribution("doc-1", []kccommon.Product{
		wikiContributionTestProduct("cao-cao", "new content", "chunk-3"),
	})
	keys, slugs := diffWikiDocumentContribution(previous, current)
	for _, key := range []string{"entity\x00cao-cao", "entity\x00liu-bei"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("affected key %q missing from %v", key, keys)
		}
	}
	for _, slug := range []string{"cao-cao", "liu-bei"} {
		if _, ok := slugs[slug]; !ok {
			t.Fatalf("affected slug %q missing from %v", slug, slugs)
		}
	}
}

func TestDiffWikiDocumentContributionTreatsDisabledDocumentAsRetraction(t *testing.T) {
	previous := buildWikiDocumentContribution("doc-1", []kccommon.Product{
		wikiContributionTestProduct("cao-cao", "content", "chunk-1"),
	})
	current := wikiDocumentContribution{DocumentID: "doc-1"}
	keys, _ := diffWikiDocumentContribution(previous, current)
	if _, ok := keys["entity\x00cao-cao"]; !ok {
		t.Fatalf("disabled document did not retract its page: %v", keys)
	}
}

func TestProcessBatchSkipsUnchangedWikiOnlyEvent(t *testing.T) {
	product := wikiContributionTestProduct("cao-cao", "same content", "chunk-1")
	store := &memoryWikiContributionStore{items: map[string]wikiDocumentContribution{}}
	if err := store.Put(t.Context(), "tenant-1", "kb-1", buildWikiDocumentContribution("doc-1", []kccommon.Product{product})); err != nil {
		t.Fatalf("seed contribution: %v", err)
	}
	factoryCalled := false
	consumer := NewConsumer(NewFakeScheduler(),
		WithReader(&fakeReader{products: []kccommon.Product{product}}),
		withWikiContributionStore(store),
		WithDeduperFactory(func(string) (Deduper, error) {
			factoryCalled = true
			return nil, nil
		}),
	)
	if err := consumer.processBatch(context.Background(), "tenant-1", "kb-1", "token-1", []BacklogEntry{{
		DocID: "doc-1", EventType: string(EventTypeCompleted), Variants: []string{"wiki"},
	}}); err != nil {
		t.Fatalf("processBatch failed: %v", err)
	}
	if factoryCalled {
		t.Fatal("unchanged Wiki-only event initialized the dataset merge path")
	}
}
