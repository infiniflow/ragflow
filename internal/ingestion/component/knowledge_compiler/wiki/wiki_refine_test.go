package wiki

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// refineChatStub returns per-page Markdown keyed by the page title in the
// writer prompt so each page's result is distinct and deterministic.
type refineChatStub struct{}

func (refineChatStub) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	title := "Page"
	for _, cand := range []string{"Alpha", "Beta", "Gamma"} {
		if strings.Contains(req.UserPrompt, cand) {
			title = cand
			break
		}
	}
	return &common.ChatResponse{Content: "# " + title + "\n\nContent for " + title + ".\n"}, nil
}

func refinePipeline() *wikiPipeline {
	return &wikiPipeline{
		ctx:       context.Background(),
		tenantID:  "t1",
		datasetID: "kb1",
		llmID:     "llm1",
		docID:     "doc-1",
		deps: common.Deps{
			Chat:            refineChatStub{},
			ModelContextLen: common.DefaultLLMContextLength,
		},
		reduced: wikiExtract{
			Entities: []wikiEntity{{Name: "Alpha", SourceChunkIDs: []string{"c1"}}},
			Claims:   []wikiClaim{{Statement: "Alpha exists", Subject: "Alpha", SourceChunkIDs: []string{"c1"}}},
		},
		inputs: common.Inputs{
			Chunks: []common.Chunk{{ID: "c1", Text: "Alpha content", Meta: map[string]any{"doc_id": "doc-1"}}},
		},
	}
}

func TestRunRefine_ParallelPagesKeepPlanOrder(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		var wg sync.WaitGroup
		for i, j := range jobs {
			i, j := i, j
			wg.Add(1)
			go func() {
				defer wg.Done()
				if i == 0 {
					time.Sleep(30 * time.Millisecond) // page 0 completes last
				}
				j()
			}()
		}
		wg.Wait()
		return ctx.Err()
	})

	p := refinePipeline()
	p.plan = wikiPlan{
		Pages: []wikiPlanPage{
			{Action: "CREATE", Slug: "entity/alpha", Title: "Alpha", PageType: "entity", Topic: "Alpha", EntityNames: []string{"Alpha"}, Priority: 1},
			{Action: "CREATE", Slug: "entity/beta", Title: "Beta", PageType: "entity", Topic: "Beta", EntityNames: []string{"Beta"}, Priority: 2},
		},
	}
	got, err := p.runRefine()
	if err != nil {
		t.Fatalf("runRefine err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d pages, want 2", len(got))
	}
	if got[0].Title != "Alpha" || got[1].Title != "Beta" {
		t.Fatalf("page order = [%s, %s], want [Alpha, Beta] (plan order preserved)", got[0].Title, got[1].Title)
	}
	if !strings.Contains(got[0].Content, "Content for Alpha") {
		t.Fatalf("page0 content missing: %q", got[0].Content)
	}
}

func TestRunRefine_FirstErrorAborts(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	boom := errors.New("refine failed")
	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		var wg sync.WaitGroup
		errs := make(chan error, len(jobs))
		for _, j := range jobs {
			j := j
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- j()
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

	p := refinePipeline()
	p.plan = wikiPlan{Pages: []wikiPlanPage{
		{Action: "CREATE", Slug: "entity/alpha", Title: "Alpha", Priority: 1},
	}}
	p.deps.Chat = chatFunc(func(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
		return nil, boom
	})
	if _, err := p.runRefine(); err != boom {
		t.Fatalf("runRefine err = %v, want boom", err)
	}
}

func TestRunRefine_CancelledCtxAborts(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		for _, j := range jobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := j(); err != nil {
				return err
			}
		}
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := refinePipeline()
	p.ctx = ctx
	p.plan = wikiPlan{Pages: []wikiPlanPage{
		{Action: "CREATE", Slug: "entity/alpha", Title: "Alpha", Priority: 1},
	}}
	if _, err := p.runRefine(); err == nil {
		t.Fatalf("runRefine err = nil, want context cancelled")
	}
}

func TestShouldReportRefineProgress(t *testing.T) {
	for completed := 1; completed <= 12; completed++ {
		want := completed == 5 || completed == 10 || completed == 12
		if got := shouldReportRefineProgress(completed, 12); got != want {
			t.Fatalf("completed=%d: got %t, want %t", completed, got, want)
		}
	}
}

// Counting bytes makes the budget tests deterministic.
type refineByteTokenizer struct{}

func (refineByteTokenizer) NumTokens(text string) int { return len(text) }

func refineTestPage(p *wikiPipeline) (wikiPageResult, error) {
	return p.runRefinePage(wikiPlanPage{
		Action: "CREATE", Slug: "entity/alpha", Title: "Alpha", PageType: "entity", EntityNames: []string{"Alpha"},
	}, []string{"entity/alpha"}, map[string]string{"entity/alpha": "Alpha"}, nil,
		buildWikiEntityLookup(p.reduced.Entities), buildWikiConceptLookup(p.reduced.Concepts))
}

func TestRunRefinePage_BatchesAllSourcesIntoOnePage(t *testing.T) {
	p := refinePipeline()
	p.deps.ModelContextLen = 131072
	p.deps.ModelMaxOutput = 2048
	p.deps.Tokenizer = refineByteTokenizer{}
	p.inputs.Chunks = nil
	var ids []string
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("c%03d", i)
		ids = append(ids, id)
		p.inputs.Chunks = append(p.inputs.Chunks, common.Chunk{ID: id, Text: strings.Repeat("source fact. ", 40)})
	}
	p.reduced.Entities[0].SourceChunkIDs = ids
	p.reduced.Claims[0].SourceChunkIDs = ids
	marker := regexp.MustCompile(`\[CHUNK_ID ([^\]]+)\]`)
	var seen []string
	draft := ""
	calls := 0
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		if err := p.checkWikiRefineBudget(req); err != nil {
			t.Fatalf("dispatched an oversized request: %v", err)
		}
		if draft != "" && (!strings.Contains(req.UserPrompt, draft) || !strings.Contains(req.UserPrompt, "UPDATE the following wiki page")) {
			t.Fatal("next batch did not integrate the previous draft")
		}
		for _, match := range marker.FindAllStringSubmatch(req.UserPrompt, -1) {
			seen = append(seen, match[1])
		}
		draft = "# Alpha\n\n" + strings.Join(seen, " ")
		return &common.ChatResponse{Content: draft}, nil
	})
	page, err := refineTestPage(p)
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("calls = %d, want multiple source batches", calls)
	}
	if !reflect.DeepEqual(seen, ids) || !reflect.DeepEqual(page.SourceChunkIDs, ids) {
		t.Fatal("source batching lost, duplicated, or reordered chunk provenance")
	}
	if page.ContentRaw != draft || !strings.Contains(page.ContentRaw, "c000") || !strings.Contains(page.ContentRaw, "c399") {
		t.Fatalf("final page did not combine all source batches: %q", page.ContentRaw)
	}
}

func TestRunRefinePage_WithinBudgetKeepsSingleRequest(t *testing.T) {
	p := refinePipeline()
	p.deps.ModelMaxOutput = 512
	calls := 0
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		evidence := assembleWikiPageEvidence(wikiPlanPage{Title: "Alpha", EntityNames: []string{"Alpha"}}, p.reduced.Claims, buildWikiEntityLookup(p.reduced.Entities), nil)
		want := renderWikiTemplate(wikiRefineWriterUserTemplate, map[string]string{
			"action": "CREATE", "slug": "entity/alpha", "title": "Alpha", "page_type": "entity",
			"all_plan_slugs": "(none — this is the only page)", "related_kb_pages": formatWikiRelatedKB(nil, nil),
			"existing_section": "", "source_context": buildSourceContext(p.inputs.Chunks, []string{"c1"}),
			"evidence_count": fmt.Sprint(len(evidence)), "evidence_blocks": formatWikiEvidenceBlocks(evidence),
		})
		if req.UserPrompt != want || req.SystemPrompt != buildWikiRefineWriterSystem("") || req.LLMID != p.llmID || req.MaxTokens != nil {
			t.Fatalf("single-round request changed: %+v", req)
		}
		return &common.ChatResponse{Content: "# Alpha\n\nAlpha exists."}, nil
	})
	if _, err := refineTestPage(p); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRunRefinePage_RejectsInfeasibleBudgetBeforeChat(t *testing.T) {
	for _, name := range []string{"unknown context", "output fills context", "oversized chunk", "oversized evidence", "oversized existing page", "output override"} {
		t.Run(name, func(t *testing.T) {
			p := refinePipeline()
			p.deps.ModelContextLen = 5000
			p.deps.ModelMaxOutput = 512
			p.deps.Tokenizer = refineByteTokenizer{}
			switch name {
			case "unknown context":
				p.deps.ModelContextLen = 0
			case "output fills context":
				p.deps.ModelMaxOutput = 5000
			case "oversized chunk":
				p.inputs.Chunks[0].Text = strings.Repeat("source ", 1000)
			case "oversized evidence":
				p.reduced.Claims[0].Statement = strings.Repeat("evidence ", 1000)
			case "oversized existing page":
				// Exercise the same guard on UPDATE's final merge request.
				p.deps.Chat = chatFunc(func(context.Context, common.ChatRequest) (*common.ChatResponse, error) {
					t.Fatal("oversized merge was dispatched")
					return nil, nil
				})
				_, err := p.mergeWikiPageContent(strings.Repeat("old ", 1500), "new fact", "entity/alpha")
				assertRefineBudgetError(t, err)
				return
			case "output override":
				cap := 5000
				p.param.ChatSettings.MaxTokens = &cap
			}
			p.deps.Chat = chatFunc(func(context.Context, common.ChatRequest) (*common.ChatResponse, error) {
				t.Fatal("infeasible REFINE request was dispatched")
				return nil, nil
			})
			_, err := refineTestPage(p)
			assertRefineBudgetError(t, err)
		})
	}
}

func assertRefineBudgetError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "REFINE") || !strings.Contains(err.Error(), "estimated_tokens=") || !strings.Contains(err.Error(), "input_budget=") {
		t.Fatalf("error = %v, want REFINE stage and token budget diagnostics", err)
	}
}

func TestRunRefinePage_RechecksGrowingDraft(t *testing.T) {
	p := refinePipeline()
	p.deps.ModelContextLen = 5000
	p.deps.ModelMaxOutput = 512
	p.deps.Tokenizer = refineByteTokenizer{}
	p.inputs.Chunks = nil
	for i := 0; i < 10; i++ {
		p.inputs.Chunks = append(p.inputs.Chunks, common.Chunk{ID: fmt.Sprint(i), Text: strings.Repeat("fact ", 200)})
	}
	p.reduced = wikiExtract{}
	calls := 0
	p.deps.Chat = chatFunc(func(context.Context, common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		return &common.ChatResponse{Content: strings.Repeat("draft ", 1000)}, nil
	})
	_, err := refineTestPage(p)
	assertRefineBudgetError(t, err)
	if calls != 1 {
		t.Fatalf("calls = %d, want only the first batch before the draft outgrows the budget", calls)
	}
}

func refineSourcePipeline(count, size int) *wikiPipeline {
	p := refinePipeline()
	p.deps.ModelContextLen = 8192
	p.deps.ModelMaxOutput = 4096
	p.inputs.Chunks = nil
	var ids []string
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("c%03d", i)
		ids = append(ids, id)
		p.inputs.Chunks = append(p.inputs.Chunks, common.Chunk{ID: id, Text: strings.Repeat("1", size)})
	}
	p.reduced.Entities[0].SourceChunkIDs = ids
	p.reduced.Claims[0].SourceChunkIDs = ids
	return p
}

func TestRunRefinePage_NoTokenizerUsesConservativeBound(t *testing.T) {
	p := refineSourcePipeline(400, 1080)
	p.deps.ModelContextLen = 128000
	p.deps.ModelMaxOutput = 2048
	marker := regexp.MustCompile(`\[CHUNK_ID ([^\]]+)\]`)
	calls, chunks := 0, 0
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		if len(req.SystemPrompt)+1+len(req.UserPrompt)+2048+wikiRefineSafetyTokens > 128000 {
			t.Fatal("nil-tokenizer path dispatched an oversized request")
		}
		chunks += len(marker.FindAllStringSubmatch(req.UserPrompt, -1))
		return &common.ChatResponse{Content: "# Alpha\n\nIntegrated evidence."}, nil
	})
	if _, err := refineTestPage(p); err != nil {
		t.Fatal(err)
	}
	if calls < 2 || chunks != 400 {
		t.Fatalf("calls/chunks = %d/%d, want multiple requests containing all 400 chunks", calls, chunks)
	}
}

func TestRunRefinePage_SmallWindowBatchesWithActualDraft(t *testing.T) {
	p := refineSourcePipeline(4, 1400)
	calls := 0
	const draft = "# Alpha\n\nKeep `{source_context}` and `{title}` literal."
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		if calls > 1 && !strings.Contains(req.UserPrompt, draft) {
			t.Fatal("template substitution changed a literal placeholder in the draft")
		}
		if err := p.checkWikiRefineBudget(req); err != nil {
			t.Fatal(err)
		}
		return &common.ChatResponse{Content: draft}, nil
	})
	if _, err := refineTestPage(p); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want four source batches in the 8k window", calls)
	}
}

func TestRunRefinePage_RepackagesRemainingSourcesAsDraftGrows(t *testing.T) {
	// Measure the ordinary fixed prompt independently of the batching path.
	probe := refinePipeline()
	probe.deps.ModelMaxOutput = 512
	var fixed int
	probe.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		fixed = len(req.SystemPrompt) + 1 + len(req.UserPrompt) - len(buildSourceContext(probe.inputs.Chunks, []string{"c1"}))
		return &common.ChatResponse{Content: "# Alpha"}, nil
	})
	if _, err := refineTestPage(probe); err != nil {
		t.Fatal(err)
	}
	p := refineSourcePipeline(4, 700)
	p.deps.ModelMaxOutput = 512
	blocks := wikiRefineSourceBlocks(p.inputs.Chunks, p.reduced.Claims[0].SourceChunkIDs)
	p.deps.ModelContextLen = fixed + 2*len(blocks[0].Text) + 10 + 512 + wikiRefineSafetyTokens
	marker := regexp.MustCompile(`\[CHUNK_ID ([^\]]+)\]`)
	var sizes []int
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		if err := p.checkWikiRefineBudget(req); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(marker.FindAllStringSubmatch(req.UserPrompt, -1)))
		return &common.ChatResponse{Content: "# Alpha\n\n" + strings.Repeat("x", 503)}, nil
	})
	if _, err := refineTestPage(p); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sizes, []int{2, 1, 1}) {
		t.Fatalf("batch sizes = %v, want [2 1 1] as the draft consumes input space", sizes)
	}
}

func TestRunRefinePage_BlankBatchDoesNotPublishPartialPage(t *testing.T) {
	for _, blankRound := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(blankRound), func(t *testing.T) {
			p := refineSourcePipeline(3, 1400)
			calls := 0
			p.deps.Chat = chatFunc(func(context.Context, common.ChatRequest) (*common.ChatResponse, error) {
				calls++
				if calls == blankRound {
					return &common.ChatResponse{Content: " \n\t"}, nil
				}
				return &common.ChatResponse{Content: "# Alpha\n\nPartial draft."}, nil
			})
			page, err := refineTestPage(p)
			if err == nil || !strings.Contains(err.Error(), "REFINE returned empty content") || page.ContentRaw != "" || calls != blankRound {
				t.Fatalf("blank round %d: page=%q error=%v calls=%d", blankRound, page.ContentRaw, err, calls)
			}
		})
	}
}

func TestRunRefinePage_SingleBlankReplyKeepsPlaceholder(t *testing.T) {
	p := refinePipeline()
	p.deps.Chat = chatFunc(func(context.Context, common.ChatRequest) (*common.ChatResponse, error) {
		return &common.ChatResponse{Content: " \n\t"}, nil
	})
	page, err := refineTestPage(p)
	if err != nil || !strings.Contains(page.ContentRaw, "Page generation produced no content.") {
		t.Fatalf("single blank reply: page=%q error=%v", page.ContentRaw, err)
	}
}

func TestRunRefinePage_LaterSourceUsesUpdatedDraft(t *testing.T) {
	probe := refinePipeline()
	var fixed int
	probe.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		fixed = len(req.SystemPrompt) + 1 + len(req.UserPrompt) - len(buildSourceContext(probe.inputs.Chunks, []string{"c1"}))
		return &common.ChatResponse{Content: "# Alpha"}, nil
	})
	if _, err := refineTestPage(probe); err != nil {
		t.Fatal(err)
	}
	p := refineSourcePipeline(2, 300)
	p.inputs.Chunks[1].Text = strings.Repeat("2", 700)
	stored := "# Alpha\n\n" + strings.Repeat("x", 1000)
	draft := stored[:len(stored)-200]
	section := "## Existing page content (UPDATE — integrate new evidence into this)\n\n" + stored + "\n"
	p.deps.ModelMaxOutput = 2048
	p.deps.ModelContextLen = fixed + len(section) + 600 + 2048 + wikiRefineSafetyTokens
	p.deps.WikiPages = wikiStoreStub{slugHit: &common.WikiPageCandidate{Slug: "entity/alpha", ContentMDRaw: stored}}
	calls := 0
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		if err := p.checkWikiRefineBudget(req); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if !strings.Contains(req.UserPrompt, "[CHUNK_ID c000]") || strings.Contains(req.UserPrompt, "[CHUNK_ID c001]") {
				t.Fatal("first request should contain only the smaller source")
			}
			return &common.ChatResponse{Content: draft}, nil
		}
		if calls != 2 || !strings.Contains(req.UserPrompt, "[CHUNK_ID c001]") || !strings.Contains(req.UserPrompt, draft) {
			t.Fatal("later source was not checked with the new draft")
		}
		return &common.ChatResponse{Content: stored}, nil
	})
	page, err := p.runRefinePage(wikiPlanPage{
		Action: "UPDATE", Slug: "entity/alpha", Title: "Alpha", PageType: "entity", EntityNames: []string{"Alpha"},
	}, []string{"entity/alpha"}, map[string]string{"entity/alpha": "Alpha"}, nil, buildWikiEntityLookup(p.reduced.Entities), nil)
	if err != nil || calls != 2 || len(page.SourceChunkIDs) != 2 {
		t.Fatalf("UPDATE result: err=%v calls=%d sources=%v", err, calls, page.SourceChunkIDs)
	}
}

type refineCountingTokenizer struct{ calls int }

func (c *refineCountingTokenizer) NumTokens(text string) int {
	c.calls++
	return len(text)
}

func TestRunRefinePage_ExhaustedAllowanceDoesNotScanSuffix(t *testing.T) {
	p := refineSourcePipeline(400, 1400)
	counter := &refineCountingTokenizer{}
	p.deps.Tokenizer = counter
	calls := 0
	p.deps.Chat = chatFunc(func(context.Context, common.ChatRequest) (*common.ChatResponse, error) {
		calls++
		return &common.ChatResponse{Content: strings.Repeat("d", 4032)}, nil
	})
	_, err := refineTestPage(p)
	assertRefineBudgetError(t, err)
	if calls != 1 || counter.calls > 20 {
		t.Fatalf("writer calls=%d token counts=%d, want one draft and bounded counting with 399 pending sources", calls, counter.calls)
	}
}
