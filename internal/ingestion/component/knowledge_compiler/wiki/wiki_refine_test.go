package wiki

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// refineChatStub returns per-page markdown keyed by the page title in the
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
			Chat: refineChatStub{},
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
