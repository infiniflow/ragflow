package wiki

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

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

// TestWikiMapMaxTokens_OutputBudgetTracksInputBudget locks the input/output
// budget coupling: the extraction MaxTokens must leave at least the whole
// wikiMapTokenBudget input budget of headroom and, with a roomy model, give the
// output the rest of the context window after the batch's input is reserved.
func TestWikiMapMaxTokens_OutputBudgetTracksInputBudget(t *testing.T) {
	// Unknown model context -> default window (DefaultLLMContextLength). Output
	// gets the whole window minus the input budget.
	got := wikiMapMaxTokens(0)
	if want := common.DefaultLLMContextLength - wikiMapTokenBudget; got != want {
		t.Fatalf("wikiMapMaxTokens(0) = %d, want %d", got, want)
	}
	// A model window that barely fits one batch must still grant at least the
	// input budget of output space (never starve the output).
	if got := wikiMapMaxTokens(2048); got != wikiMapTokenBudget {
		t.Fatalf("wikiMapMaxTokens(2048) = %d, want %d (floor at input budget)", got, wikiMapTokenBudget)
	}
	// A roomy model: output = window - input budget.
	if got := wikiMapMaxTokens(16384); got != 16384-wikiMapTokenBudget {
		t.Fatalf("wikiMapMaxTokens(16384) = %d, want %d", got, 16384-wikiMapTokenBudget)
	}
}

func TestRunMapBatches_PreservesBatchOrderWithSubmitter(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		var wg sync.WaitGroup
		errs := make(chan error, len(jobs))
		for _, job := range jobs {
			job := job
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- job()
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return err
			}
		}
		return ctx.Err()
	})

	batches := [][]common.Chunk{
		{{ID: "slow", Text: "slow"}},
		{{ID: "fast-1", Text: "fast-1"}},
		{{ID: "fast-2", Text: "fast-2"}},
	}
	got, err := runMapBatches(context.Background(), batches, func(batch []common.Chunk) (wikiExtract, error) {
		if batch[0].ID == "slow" {
			time.Sleep(25 * time.Millisecond)
		}
		return wikiExtract{Topics: []string{batch[0].ID}}, nil
	})
	if err != nil {
		t.Fatalf("runMapBatches err = %v", err)
	}
	if len(got) != len(batches) {
		t.Fatalf("runMapBatches len = %d, want %d", len(got), len(batches))
	}
	for i, want := range []string{"slow", "fast-1", "fast-2"} {
		if len(got[i].Topics) != 1 || got[i].Topics[0] != want {
			t.Fatalf("runMapBatches[%d] = %#v, want topic %q", i, got[i], want)
		}
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

func TestMergePlanCandidates_DeduplicatesWithoutLLMMerge(t *testing.T) {
	p := &wikiPipeline{
		docID: "doc-1",
		reduced: wikiExtract{
			Entities: []wikiEntity{{Name: "Alpha"}},
		},
	}
	merged := p.mergePlanCandidates([]wikiPlan{
		{
			Title: "Alpha",
			Pages: []wikiPlanPage{
				{
					Slug:        "entity/alpha",
					Title:       "Alpha",
					PageType:    "entity",
					Topic:       "Alpha",
					EntityNames: []string{"Alpha"},
					RelatedKB:   []string{"entity/beta", "missing", "entity/alpha"},
					Priority:    2,
				},
			},
		},
		{
			Pages: []wikiPlanPage{
				{
					Slug:        "entity/beta",
					Title:       "Beta",
					PageType:    "entity",
					Topic:       "Beta",
					EntityNames: []string{"Beta"},
					RelatedKB:   []string{"entity/alpha"},
					Priority:    1,
				},
				{
					Slug:        "entity/alpha",
					Title:       "Alpha duplicate",
					PageType:    "entity",
					Topic:       "Alpha",
					EntityNames: []string{"Alpha"},
					Priority:    3,
				},
			},
		},
	}, p.reduced)
	if len(merged.Pages) != 2 {
		t.Fatalf("merged pages = %d, want 2", len(merged.Pages))
	}
	if merged.Pages[0].Slug != "entity/beta" || merged.Pages[1].Slug != "entity/alpha" {
		t.Fatalf("merged page order = %#v", merged.Pages)
	}
	if got := merged.Pages[1].RelatedKB; len(got) != 1 || got[0] != "entity/beta" {
		t.Fatalf("alpha related links = %#v, want [entity/beta]", got)
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

// TestReconcilePlanPage_OverlapHeuristicSkipsLLM locks the Go-only enhancement:
// a candidate whose score is inside [maybe, update) but whose topic matches the
// planned page's topic is promoted straight to UPDATE without an LLM round.
func TestReconcilePlanPage_OverlapHeuristicSkipsLLM(t *testing.T) {
	called := false
	p := &wikiPipeline{
		ctx:       context.Background(),
		tenantID:  "t1",
		datasetID: "kb1",
		llmID:     "llm1",
		deps: common.Deps{
			Chat: chatFunc(func(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
				called = true
				return &common.ChatResponse{Content: `{"action":"CREATE"}`}, nil
			}),
			Embed:     reconcileEmbedStub{},
			WikiPages: wikiStoreStub{similar: []common.WikiPageCandidate{{Slug: "topic/alpha", Title: "Alpha topic", Topic: "Alpha", Score: 0.85}}},
		},
	}
	got, err := p.reconcilePlanPage(wikiPlanPage{
		Slug:     "topic/alpha-new",
		Title:    "Alpha Topic",
		PageType: "topic",
		Topic:    "Alpha",
	}, []float32{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatalf("reconcilePlanPage err = %v", err)
	}
	if got == nil || got.Slug != "topic/alpha" {
		t.Fatalf("reconcilePlanPage = %#v, want topic/alpha (topic overlap promotes to UPDATE)", got)
	}
	if called {
		t.Fatalf("overlap heuristic should not invoke the LLM")
	}
}

type chatFunc func(context.Context, common.ChatRequest) (*common.ChatResponse, error)

func (f chatFunc) Chat(ctx context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	return f(ctx, req)
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
