package task

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func testStrPtr(s string) *string { return &s }

func makeTaskHandlerTestContext(pipelineID string) *TaskContext {
	return &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-1",
			DocumentID: "doc-1",
		},
		PipelineID: pipelineID,
		Doc: entity.Document{
			ID:           "doc-1",
			KbID:         "kb-1",
			Name:         testStrPtr("test-doc.pdf"),
			ParserID:     "naive",
			ParserConfig: entity.JSONMap{},
		},
		KB: entity.Knowledgebase{
			ID:       "kb-1",
			TenantID: "tenant-1",
			EmbdID:   "embd-1",
		},
		Tenant: entity.Tenant{
			ID:    "tenant-1",
			LLMID: "gpt-4",
		},
	}
}

func newNoopPipelineExecutor(ctx *TaskContext, canvasID string) (*PipelineExecutor, error) {
	if strings.TrimSpace(canvasID) == "" {
		canvasID = "flow-1"
	}
	svc, err := NewPipelineExecutor(ctx, canvasID, 0)
	if err != nil {
		return nil, err
	}
	svc = svc.
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"stub-node"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{
				"chunks": []map[string]any{{
					"text":    "stub pipeline chunk",
					"q_2_vec": []float64{0.1, 0.2},
				}},
			}, dsl, nil
		}).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error {
			return nil
		})
	return svc, nil
}

func newNoopTaskHandler(ctx *TaskContext) *TaskHandler {
	return NewTaskHandler(ctx).WithPipelineExecutorFactory(newNoopPipelineExecutor)
}

func TestTaskHandler_HandleRejectsNilContext(t *testing.T) {
	if _, err := NewTaskHandler(nil).Handle(); err == nil {
		t.Fatal("expected error for nil context")
	}
}

func TestTaskHandler_HandleRequiresPipelineID(t *testing.T) {
	ctx := makeTaskHandlerTestContext("")
	handler := NewTaskHandler(ctx)
	if _, err := handler.Handle(); err == nil {
		t.Fatal("expected error for empty pipeline id")
	}
}

func TestTaskHandler_HandleRunWithFactory(t *testing.T) {
	ctx := makeTaskHandlerTestContext("flow-1")
	ctx.Ctx = context.Background()
	handler := NewTaskHandler(ctx).WithPipelineExecutorFactory(func(ctx *TaskContext, canvasID string) (*PipelineExecutor, error) {
		return newNoopPipelineExecutor(ctx, canvasID)
	})
	if _, err := handler.Handle(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskHandler_Pipeline_UsesTaskContext(t *testing.T) {
	ctx := makeTaskHandlerTestContext("flow-1")
	type ctxKey string
	const key ctxKey = "trace"
	ctx.Ctx = context.WithValue(context.Background(), key, "task-ctx")

	handler := NewTaskHandler(ctx).WithPipelineExecutorFactory(func(ctx *TaskContext, canvasID string) (*PipelineExecutor, error) {
		return mustNewPipelineExecutor(t, ctx, canvasID, 0).
			WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
				return `{"nodes":[{"id":"stub-node"}],"edges":[]}`, canvasID, nil
			}).
			WithRunPipelineFunc(func(runCtx context.Context, dsl string) (map[string]any, string, error) {
				if got := runCtx.Value(key); got != "task-ctx" {
					t.Fatalf("runCtx value = %v, want task-ctx", got)
				}
				return map[string]any{"chunks": []map[string]any{{"text": "stub", "q_2_vec": []float64{0.1, 0.2}}}}, dsl, nil
			}).
			WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
				return nil, nil
			}).
			WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil }), nil
	})
	if _, err := handler.Handle(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskHandler_Pipeline_ShowsProgressAndPipelineLog(t *testing.T) {
	ctx := makeTaskHandlerTestContext("flow-1")
	ctx.Ctx = context.Background()
	ctx.Doc.PipelineID = testStrPtr("flow-1")
	ctx.Doc.Name = testStrPtr("verify-pipeline.pdf")

	var pipelineCalled bool
	var insertCalled bool
	var logCreateCalls int
	var insertedChunkCount int

	handler := NewTaskHandler(ctx).WithPipelineExecutorFactory(func(ctx *TaskContext, canvasID string) (*PipelineExecutor, error) {
		svc := mustNewPipelineExecutor(t, ctx, canvasID, 0).
			WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
				return `{"nodes":[{"id":"stub-node"}],"edges":[]}`, canvasID, nil
			}).
			WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
				pipelineCalled = true
				return map[string]any{
					"chunks": []map[string]any{{
						"text":    "stub pipeline chunk",
						"q_2_vec": []float64{0.1, 0.2},
					}},
				}, dsl, nil
			}).
			WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
				insertCalled = true
				insertedChunkCount = len(chunks)
				return nil, nil
			}).
			WithLogCreateFunc(func(log *entity.PipelineOperationLog) error {
				logCreateCalls++
				return nil
			})
		return svc, nil
	})

	if _, err := handler.Handle(); err != nil {
		t.Fatalf("handler.Handle() error: %v", err)
	}

	if !pipelineCalled {
		t.Fatal("expected mock pipeline.run to be called")
	}
	if !insertCalled {
		t.Fatal("expected insertChunks to be called")
	}
	if insertedChunkCount != 1 {
		t.Fatalf("insertedChunkCount = %d, want 1", insertedChunkCount)
	}

	if logCreateCalls != 1 {
		t.Fatalf("logCreateCalls = %d, want 1", logCreateCalls)
	}
}
