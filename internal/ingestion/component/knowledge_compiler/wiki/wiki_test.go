package wiki

import (
	"context"
	"reflect"
	"slices"
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
	if ids := reduced.Entities[0].SourceChunkIDs; !slices.Equal(ids, []string{"c1", "c2"}) {
		t.Fatalf("entity provenance = %#v, want [c1 c2]", ids)
	}
	if len(reduced.Claims) != 2 {
		t.Fatalf("claims=%d, want 2", len(reduced.Claims))
	}
}

func TestParseWikiExtractNormalizesTopicPathAndProvenance(t *testing.T) {
	extract := parseWikiExtract(map[string]any{
		"topics": []any{map[string]any{
			"path": " 三国演义 / 人物 / 蜀汉人物 ", "description": "蜀汉人物", "source_chunk_id": "c1",
		}},
	})
	if len(extract.Topics) != 1 {
		t.Fatalf("topics = %#v, want one", extract.Topics)
	}
	topic := extract.Topics[0]
	if topic.Path != "三国演义/人物/蜀汉人物" || !slices.Equal(topic.SourceChunkIDs, []string{"c1"}) {
		t.Fatalf("topic = %#v", topic)
	}
}

func TestParseWikiExtractAcceptsLegacyStringTopics(t *testing.T) {
	extract := parseWikiExtract(map[string]any{
		"topics": []any{"人物/汉末/曹魏", "桃园结义"},
	})
	if len(extract.Topics) != 2 {
		t.Fatalf("topics = %#v, want two", extract.Topics)
	}
	if extract.Topics[0].Path != "人物/汉末/曹魏" || extract.Topics[1].Path != "桃园结义" {
		t.Fatalf("topics = %#v", extract.Topics)
	}
}

func TestWikiTemplateCustomRulesPrefersGlobalRules(t *testing.T) {
	got := wikiTemplateCustomRules(map[string]any{
		"global_rules": "  Use configured topics.  ",
		"guideline": map[string]any{
			"rules_for_entities":  "entity fallback",
			"rules_for_relations": "relation fallback",
		},
	}, "English")
	if got != "Use configured topics." {
		t.Fatalf("custom rules = %q, want global rules", got)
	}
}

func TestWikiTemplateCustomRulesFallsBackToGuideline(t *testing.T) {
	got := wikiTemplateCustomRules(map[string]any{
		"global_rules": " ",
		"guideline": map[string]any{
			"rules_for_entities":  "extract configured entities",
			"rules_for_relations": "extract configured relations",
		},
	}, "English")
	if !strings.Contains(got, "extract configured entities") || !strings.Contains(got, "extract configured relations") {
		t.Fatalf("custom rules = %q, want guideline fallback", got)
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
		return wikiExtract{Topics: []wikiTopic{{Path: batch[0].ID}}}, nil
	})
	if err != nil {
		t.Fatalf("runMapBatches err = %v", err)
	}
	if len(got) != len(batches) {
		t.Fatalf("runMapBatches len = %d, want %d", len(got), len(batches))
	}
	for i, want := range []string{"slow", "fast-1", "fast-2"} {
		if len(got[i].Topics) != 1 || got[i].Topics[0].Path != want {
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

func TestNormalizeWikiPlanPagesExpandsUniqueMAPTopicLeaf(t *testing.T) {
	pages := normalizeWikiPlanPages([]wikiPlanPage{{
		Slug: "entity/刘备", Title: "刘备", PageType: "entity", Topic: "蜀汉人物",
	}}, wikiExtract{Topics: []wikiTopic{{Path: "三国演义 / 人物 / 蜀汉人物"}}})
	if len(pages) != 1 {
		t.Fatalf("pages = %#v, want one page", pages)
	}
	if got := pages[0].Topic; got != "三国演义/人物/蜀汉人物" {
		t.Fatalf("topic = %q, want complete MAP topic path", got)
	}
}

func TestNormalizeWikiPlanPagesFallsBackForUnknownTopic(t *testing.T) {
	pages := normalizeWikiPlanPages([]wikiPlanPage{{
		Slug: "entity/刘备", Title: "刘备", PageType: "entity", Topic: "历史 / 人物 / 蜀汉人物",
	}}, wikiExtract{Topics: []wikiTopic{{Path: "文学/人物"}}})
	if got := pages[0].Topic; got != common.GeneralWikiTopic {
		t.Fatalf("topic = %q, want %q", got, common.GeneralWikiTopic)
	}
}

func TestNormalizeWikiPlanPagesDoesNotUseEntityTitleAsTopic(t *testing.T) {
	pages := normalizeWikiPlanPages([]wikiPlanPage{{
		Slug: "entity/person/曹操", Title: "曹操", PageType: "entity", Topic: "人物/曹操",
	}}, wikiExtract{Topics: []wikiTopic{{Path: "人物/曹操"}, {Path: "人物/汉末/曹魏"}}})
	if got := pages[0].Topic; got != common.GeneralWikiTopic {
		t.Fatalf("topic = %q, want %q", got, common.GeneralWikiTopic)
	}
}

type topicPathEmbedStub struct{}

func (topicPathEmbedStub) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		switch {
		case strings.Contains(text, "刘备"), strings.Contains(text, "蜀汉"):
			out[i] = []float32{1, 0}
		case strings.Contains(text, "曹操"), strings.Contains(text, "曹魏"):
			out[i] = []float32{0, 1}
		default:
			out[i] = []float32{0.5, 0.5}
		}
	}
	return out, nil
}

func (topicPathEmbedStub) Dimensions() int { return 2 }

func TestBuildTopicCandidateCommunitiesAssignsMAPTopicPaths(t *testing.T) {
	p := &wikiPipeline{
		ctx:  context.Background(),
		deps: common.Deps{Embed: topicPathEmbedStub{}},
		reduced: wikiExtract{
			Entities: []wikiEntity{{Name: "刘备"}, {Name: "曹操"}},
			Topics: []wikiTopic{
				{Path: "三国演义/人物/蜀汉人物"},
				{Path: "三国演义/人物/曹魏人物"},
			},
		},
	}
	communities := p.buildTopicCandidateCommunities()
	if len(communities) != 2 {
		t.Fatalf("communities = %#v, want two", communities)
	}
	for _, community := range communities {
		if len(community.Entities) != 1 || len(community.Topics) != 1 {
			t.Fatalf("community = %#v, want one entity and one topic", community)
		}
		entity := community.Entities[0].Name
		topic := community.Topics[0].Path
		if entity == "刘备" && topic != "三国演义/人物/蜀汉人物" {
			t.Fatalf("刘备 topic = %q", topic)
		}
		if entity == "曹操" && topic != "三国演义/人物/曹魏人物" {
			t.Fatalf("曹操 topic = %q", topic)
		}
	}
}

func TestNormalizeWikiPlanPages_DedupSameTypeSameTitle(t *testing.T) {
	// Scheme A: two entity pages with the same title but different slug
	// transliterations (lu-bu vs lv-bu) collapse to one, and their RelatedKB
	// is folded into the survivor. A same-title page of another type (topic)
	// stays distinct.
	pages := []wikiPlanPage{
		{
			Slug:        "entity/lu-bu",
			Title:       "吕布",
			PageType:    "entity",
			Topic:       "吕布",
			EntityNames: []string{"吕布", "李肃"},
			RelatedKB:   []string{"entity/dong-zhuo"},
			Priority:    1,
		},
		{
			Slug:        "entity/lv-bu",
			Title:       "吕布",
			PageType:    "entity",
			Topic:       "吕布",
			EntityNames: []string{"吕布", "丁原"},
			RelatedKB:   []string{"entity/ding-yuan", "entity/dong-zhuo"},
			Priority:    2,
		},
		{
			Slug:     "topic/lu-bu-legend",
			Title:    "吕布",
			PageType: "topic",
			Topic:    "吕布生平",
			Priority: 3,
		},
	}
	got := normalizeWikiPlanPages(pages, wikiExtract{})
	if len(got) != 2 {
		t.Fatalf("normalizeWikiPlanPages = %d pages, want 2 (entity merged + topic)", len(got))
	}
	// Survivor keeps the first-emitted slug and accumulates RelatedKB.
	var entitySlug string
	for _, p := range got {
		if p.PageType == "entity" {
			entitySlug = p.Slug
			want := []string{"entity/dong-zhuo", "entity/ding-yuan"}
			if len(p.RelatedKB) != 2 {
				t.Fatalf("entity RelatedKB = %v, want %v (deduped union)", p.RelatedKB, want)
			}
		}
	}
	if entitySlug != "entity/lu-bu" {
		t.Fatalf("surviving entity slug = %q, want entity/lu-bu (first emitted)", entitySlug)
	}
}

func TestNormalizeWikiPlanPages_DoesNotMergeAcrossTypes(t *testing.T) {
	// Same title, different page_type: concept vs entity must stay distinct
	// (page identity is page_type/slug).
	pages := []wikiPlanPage{
		{Slug: "concept/lu-bu", Title: "吕布", PageType: "concept", Priority: 1},
		{Slug: "entity/lu-bu", Title: "吕布", PageType: "entity", Priority: 2},
	}
	got := normalizeWikiPlanPages(pages, wikiExtract{})
	if len(got) != 2 {
		t.Fatalf("normalizeWikiPlanPages = %d pages, want 2 (concept + entity)", len(got))
	}
}

func TestNormalizeWikiPlanPages_DoesNotMergeTypedEntitiesWithSameTitle(t *testing.T) {
	pages := []wikiPlanPage{
		{Slug: "entity/fruit/苹果", Title: "苹果", PageType: "entity", EntityNames: []string{"苹果"}},
		{Slug: "entity/company/苹果", Title: "苹果", PageType: "entity", EntityNames: []string{"苹果"}},
	}
	got := normalizeWikiPlanPages(pages, wikiExtract{Entities: []wikiEntity{
		{Name: "苹果", Type: "fruit"},
		{Name: "苹果", Type: "company"},
	}})
	if len(got) != 2 {
		t.Fatalf("normalizeWikiPlanPages = %#v, want both typed entity pages", got)
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

func TestAssembleWikiPlanRelatedPagesFromRelations(t *testing.T) {
	pages := []wikiPlanPage{
		{Slug: "entity/alpha", Title: "Alpha", EntityNames: []string{"Alpha"}},
		{Slug: "entity/beta", Title: "Beta", EntityNames: []string{"Beta"}},
	}
	relations := []wikiRelation{{From: "Alpha", To: "Beta", Type: "related"}}

	got := assembleWikiPlanRelatedPages(pages, relations)
	if want := []string{"entity/beta"}; !reflect.DeepEqual(got[0].RelatedKB, want) {
		t.Fatalf("Alpha related links = %v, want %v", got[0].RelatedKB, want)
	}
	if want := []string{"entity/alpha"}; !reflect.DeepEqual(got[1].RelatedKB, want) {
		t.Fatalf("Beta related links = %v, want %v", got[1].RelatedKB, want)
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

func TestReduceExtracts_MergesDuplicateEntitySlug(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Entities: []wikiEntity{{Name: "曹操", Type: "person", Aliases: []string{"孟德"}, SourceChunkIDs: []string{"c1"}}}},
		{Entities: []wikiEntity{{Name: "曹操", Type: "person", SourceChunkIDs: []string{"c2"}}}},
	})
	if len(reduced.Entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(reduced.Entities))
	}
	if got := reduced.Entities[0].SourceChunkIDs; !slices.Equal(got, []string{"c1", "c2"}) {
		t.Fatalf("source chunk ids = %#v, want [c1 c2]", got)
	}
	if got := reduced.Entities[0].Aliases; len(got) != 1 || got[0] != "孟德" {
		t.Fatalf("aliases = %#v, want [孟德]", got)
	}
}

func TestReduceExtracts_DifferentEntityTypesKeepDifferentSlugs(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Entities: []wikiEntity{{Name: "苹果", Type: "fruit"}}},
		{Entities: []wikiEntity{{Name: "苹果", Type: "company"}}},
	})
	if len(reduced.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(reduced.Entities))
	}
	if got := entityPageSlug(reduced.Entities[0].Name, reduced.Entities[0].Type); got == entityPageSlug(reduced.Entities[1].Name, reduced.Entities[1].Type) {
		t.Fatalf("different entity types have the same slug %q", got)
	}
}

func TestReduceExtracts_EntityIdentityDoesNotCollideAtHyphenBoundary(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{{Entities: []wikiEntity{
		{Name: "bar-baz", Type: "foo"},
		{Name: "baz", Type: "foo-bar"},
	}}})
	if len(reduced.Entities) != 2 {
		t.Fatalf("entities = %#v, want two distinct identities", reduced.Entities)
	}
	first := entityPageSlug("bar-baz", "foo")
	second := entityPageSlug("baz", "foo-bar")
	if first == second {
		t.Fatalf("entity slugs collide: %q", first)
	}
}

func TestReduceExtracts_NormalizesEntityWhitespace(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{{Entities: []wikiEntity{
		{Name: "John  Smith", Type: "person"},
		{Name: "John Smith", Type: "person"},
	}}})
	if len(reduced.Entities) != 1 {
		t.Fatalf("entities = %#v, want whitespace-equivalent names merged", reduced.Entities)
	}
}

func TestReduceExtracts_DoesNotMergeSimilarNames(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Entities: []wikiEntity{{Name: "Alpha", Type: "org"}}},
		{Entities: []wikiEntity{{Name: "Alpha Incorporated", Type: "org"}}},
	})
	if len(reduced.Entities) != 2 {
		t.Fatalf("entities = %d, want 2; REDUCE must not perform semantic merging", len(reduced.Entities))
	}
}

func TestReduceExtracts_MergesDuplicateRelationProvenance(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Relations: []wikiRelation{{From: "A", To: "B", Type: "knows", SourceChunkIDs: []string{"c1"}}}},
		{Relations: []wikiRelation{{From: "A", To: "B", Type: "knows", SourceChunkIDs: []string{"c2"}}}},
	})
	if len(reduced.Relations) != 1 || !slices.Equal(reduced.Relations[0].SourceChunkIDs, []string{"c1", "c2"}) {
		t.Fatalf("relations = %#v, want one relation with both source chunks", reduced.Relations)
	}
}

func TestEntityPageSlugIncludesType(t *testing.T) {
	if got, want := entityPageSlug("曹操", "person"), "entity/person/曹操"; got != want {
		t.Fatalf("entityPageSlug = %q, want %q", got, want)
	}
	if got, want := entityPageSlug("曹操", ""), "entity/曹操"; got != want {
		t.Fatalf("entityPageSlug without type = %q, want %q", got, want)
	}
}

func TestCosine32RejectsDifferentDimensions(t *testing.T) {
	if got := cosine32([]float32{1}, []float32{1, 1}); got != 0 {
		t.Fatalf("cosine32 unequal dimensions = %v, want 0", got)
	}
}
