package wiki

import (
	"context"
	"fmt"
	"math"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/progress"
)

func serialProgressBatches(t *testing.T) {
	t.Helper()
	previous := batchSubmitter
	batchSubmitter = func(_ context.Context, jobs []func() error) error {
		for _, job := range jobs {
			if err := job(); err != nil {
				return err
			}
		}
		return nil
	}
	t.Cleanup(func() { batchSubmitter = previous })
}

func wikiProgressContext(t *testing.T, values *[]float64) context.Context {
	t.Helper()
	return runtime.BindComponentFraction(runtime.WithProgressFractionCallback(t.Context(), func(_ string, f float64) {
		*values = append(*values, f)
	}), "Compiler:wiki")
}

func TestWikiProgressMapReportsBeforeAllBatchesFinish(t *testing.T) {
	serialProgressBatches(t)
	var values []float64
	ctx := wikiProgressContext(t, &values)
	_, err := runMapBatches(ctx, [][]common.Chunk{{{ID: "first"}}, {{ID: "second"}}}, progress.Window{Base: 0.2, Span: 0.4}, func(batch []common.Chunk) (wikiExtract, error) {
		if batch[0].ID == "second" && (len(values) != 1 || math.Abs(values[0]-0.4) > 1e-9) {
			t.Errorf("first batch did not advance progress before second started: %v", values)
		}
		return wikiExtract{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || math.Abs(values[1]-0.6) > 1e-9 {
		t.Fatalf("MAP progress = %v, want [0.4 0.6]", values)
	}
}

func TestWikiProgressTopicPlanCountsScheduledJobs(t *testing.T) {
	serialProgressBatches(t)
	var values []float64
	ctx := wikiProgressContext(t, &values)
	p := &wikiPipeline{
		ctx: ctx, llmID: "llm", docID: "doc",
		deps: common.Deps{Chat: batchPlanChatStub{}, Embed: reconcileEmbedStub{}, ModelMaxOutput: 304, Progress: progress.Window{Span: 1}},
	}
	for i := 0; i < 30; i++ {
		p.reduced.Entities = append(p.reduced.Entities, wikiEntity{Name: fmt.Sprintf("entity-%d", i), Type: "project"})
	}
	if _, err := p.runTopicPlan(); err != nil {
		t.Fatal(err)
	}
	// The output budget schedules one planner job; quota-zero communities
	// must not keep the PLAN window below its end (3/6).
	if len(values) != 1 || math.Abs(values[0]-0.5) > 1e-9 {
		t.Fatalf("PLAN progress = %v, want [0.5]", values)
	}
}
