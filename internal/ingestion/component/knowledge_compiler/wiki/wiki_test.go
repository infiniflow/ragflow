package wiki

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestReduceExtracts_MergesProvenance(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{
			Entities: []wikiEntity{{Name: "Alpha", Type: "thing", SourceChunkIDs: []string{"c1"}}},
			Claims:   []wikiClaim{{Statement: "Alpha exists", Subject: "Alpha", SourceChunkIDs: []string{"c1"}}},
		},
		{
			Entities: []wikiEntity{{Name: "Alpha", Type: "thing", SourceChunkIDs: []string{"c2"}}},
			Claims:   []wikiClaim{{Statement: "Alpha exists", Subject: "Alpha", SourceChunkIDs: []string{"c2"}}},
		},
	})
	if len(reduced.Entities) != 1 {
		t.Fatalf("entities=%d, want 1", len(reduced.Entities))
	}
	if ids := reduced.Entities[0].SourceChunkIDs; len(ids) != 2 {
		t.Fatalf("entity provenance = %#v, want 2 chunk ids", ids)
	}
	if len(reduced.Claims) != 2 {
		t.Fatalf("claims=%d, want 2", len(reduced.Claims))
	}
}

func TestPackWikiPlanBatches_SplitsLargeInput(t *testing.T) {
	reduced := wikiExtract{
		Entities: []wikiEntity{
			{Name: strings.Repeat("a", 1000)},
			{Name: strings.Repeat("b", 1000)},
			{Name: strings.Repeat("c", 1000)},
		},
	}
	batches := packWikiPlanBatches(reduced, 1)
	if len(batches) < 2 {
		t.Fatalf("expected multiple batches, got %d", len(batches))
	}
}

func TestBuildSourceContext_SelectsKnownChunks(t *testing.T) {
	ctx := buildSourceContext([]common.Chunk{
		{ID: "c1", Text: "alpha text"},
		{ID: "c2", Text: "beta text"},
		{ID: "c3", Text: "gamma text"},
	}, []string{"c2", "c3"})
	if strings.Contains(ctx, "alpha text") {
		t.Fatalf("source context leaked unselected chunk: %q", ctx)
	}
	if !strings.Contains(ctx, "beta text") || !strings.Contains(ctx, "gamma text") {
		t.Fatalf("source context missing selected chunks: %q", ctx)
	}
}

func TestNormalizeWikiPlanPages_FallbacksToEntitiesAndConcepts(t *testing.T) {
	plan := normalizeWikiPlan(wikiPlan{}, "doc-1", wikiExtract{
		Entities: []wikiEntity{{Name: "Alpha", Aliases: []string{"A"}}},
		Concepts: []wikiConcept{{Term: "Beta"}},
	})
	if len(plan.Pages) < 2 {
		t.Fatalf("normalizeWikiPlan generated %d pages, want at least 2", len(plan.Pages))
	}
	if plan.Pages[0].Slug == "" || plan.Pages[1].Slug == "" {
		t.Fatalf("fallback pages missing slugs: %#v", plan.Pages)
	}
}

type reconcileChatStub struct {
	resp string
}

func (s reconcileChatStub) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	return &common.ChatResponse{Content: s.resp}, nil
}

type reconcileEmbedStub struct{}

func (reconcileEmbedStub) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (reconcileEmbedStub) Dimensions() int { return 3 }

type wikiStoreStub struct {
	slugHit *common.WikiPageCandidate
	similar []common.WikiPageCandidate
}

func (s wikiStoreStub) FindSimilarPages(_ context.Context, _, _ string, _ []float32, _ int) ([]common.WikiPageCandidate, error) {
	return s.similar, nil
}

func (s wikiStoreStub) GetPageBySlug(_ context.Context, _, _, slug string) (*common.WikiPageCandidate, error) {
	if s.slugHit != nil && s.slugHit.Slug == slug {
		return s.slugHit, nil
	}
	return nil, nil
}

func TestReconcilePlanPage_MaybeUsesLLMDecision(t *testing.T) {
	p := &wikiPipeline{
		ctx:       context.Background(),
		tenantID:  "t1",
		datasetID: "kb1",
		llmID:     "llm1",
		deps: common.Deps{
			Chat:      reconcileChatStub{resp: `{"action":"UPDATE","slug":"entity/existing","reason":"same entity"}`},
			Embed:     reconcileEmbedStub{},
			WikiPages: wikiStoreStub{similar: []common.WikiPageCandidate{{Slug: "entity/existing", Title: "Existing", Score: 0.81}}},
		},
	}
	got, err := p.reconcilePlanPage(wikiPlanPage{
		Slug:        "entity/new-alpha",
		Title:       "Alpha",
		PageType:    "entity",
		Topic:       "Alpha",
		EntityNames: []string{"Alpha Prime"},
	}, []float32{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatalf("reconcilePlanPage err = %v", err)
	}
	if got == nil || got.Slug != "entity/existing" {
		t.Fatalf("reconcilePlanPage = %#v, want entity/existing", got)
	}
}

func TestReconcilePlanPage_LowScoreSkipsLLM(t *testing.T) {
	p := &wikiPipeline{
		ctx:       context.Background(),
		tenantID:  "t1",
		datasetID: "kb1",
		llmID:     "llm1",
		deps: common.Deps{
			Chat:      reconcileChatStub{resp: `{"action":"UPDATE","slug":"entity/existing","reason":"same entity"}`},
			Embed:     reconcileEmbedStub{},
			WikiPages: wikiStoreStub{similar: []common.WikiPageCandidate{{Slug: "entity/existing", Title: "Existing", Score: 0.6}}},
		},
	}
	got, err := p.reconcilePlanPage(wikiPlanPage{
		Slug:        "entity/new-alpha",
		Title:       "Alpha",
		PageType:    "entity",
		Topic:       "Alpha",
		EntityNames: []string{"Alpha Prime"},
	}, []float32{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatalf("reconcilePlanPage err = %v", err)
	}
	if got != nil {
		t.Fatalf("reconcilePlanPage = %#v, want nil", got)
	}
}

func TestMergeWikiPageContent_PreservesShortExistingPage(t *testing.T) {
	p := &wikiPipeline{
		ctx: context.Background(),
		deps: common.Deps{
			Chat: reconcileChatStub{resp: "# Alpha\n\nAlpha launched a new process in 2026.\n"},
		},
	}
	merged, err := p.mergeWikiPageContent(
		"# Alpha\n\nExisting fact.\n",
		"# Alpha\n\nAlpha launched a new process in 2026.\n",
		"entity/alpha",
	)
	if err != nil {
		t.Fatalf("mergeWikiPageContent err = %v", err)
	}
	if !strings.Contains(merged, "Existing fact.") {
		t.Fatalf("merged page dropped existing content: %q", merged)
	}
	if !strings.Contains(merged, "Alpha launched a new process in 2026.") {
		t.Fatalf("merged page dropped incoming content: %q", merged)
	}
}
