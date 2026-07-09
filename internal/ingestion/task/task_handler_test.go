package task

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

func testStrPtr(s string) *string { return &s }

func makeTaskHandlerTestContext(taskType string) *TaskContext {
	var progressCalls []float64
	pipelineID := ""
	if strings.HasPrefix(taskType, "dataflow") {
		pipelineID = "flow-1"
	}
	return &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-1",
			DocumentID: "doc-1",
		},
		TaskType:   taskType,
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
		ProgressFunc: func(prog float64, msg string) {
			progressCalls = append(progressCalls, prog)
		},
	}
}

func newNoopDataflowService(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error) {
	if strings.TrimSpace(dataflowID) == "" {
		dataflowID = "flow-1"
	}
	svc, err := NewDataflowService(ctx, dataflowID, 0, 0)
	if err != nil {
		return nil, err
	}
	svc = svc.
		WithLoadDSLFunc(func(ctx context.Context, dataflowID string) (string, string, error) {
			return `{"nodes":[{"id":"stub-node"}],"edges":[]}`, dataflowID, nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{
				"chunks": []map[string]any{
					{"text": "stub dataflow chunk", "q_2_vec": []float64{0.1, 0.2}},
				},
			}, dsl, nil
		}).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error {
			return nil
		}).
		WithDocService(&stubDocService{}).
		WithChunkCounter(&stubChunkCounter{}).
		WithGetEmbeddingModelFunc(func(tenantID, embdID string) (*models.EmbeddingModel, error) {
			return nil, nil
		})
	return svc, nil
}

func newNoopTaskHandler(ctx *TaskContext) *TaskHandler {
	return NewTaskHandler(ctx).WithDataflowServiceFactory(newNoopDataflowService)
}

func TestTaskHandler_Dispatch(t *testing.T) {
	tests := []struct {
		name      string
		taskType  string
		wantErr   bool
		wantPanic bool
	}{
		{"memory", "memory", false, false},
		{"dataflow", "dataflow", false, false},
		{"dataflow with suffix", "dataflow_test", false, false},
		{"raptor", "raptor", false, false},
		{"graphrag", "graphrag", false, false},
		{"mindmap", "mindmap", false, false},
		{"evaluation", "evaluation", false, false},
		{"reembedding", "reembedding", false, false},
		{"clone", "clone", false, false},
		{"standard (empty task_type)", "", false, false},
		{"standard (unknown task_type)", "unknown_type", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := makeTaskHandlerTestContext(tt.taskType)
			handler := newNoopTaskHandler(ctx)
			err := handler.Handle()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTaskHandler_DefaultDataflowServiceInjectsProgress(t *testing.T) {
	ctx := makeTaskHandlerTestContext("dataflow")
	handler := NewTaskHandler(ctx).WithDataflowServiceFactory(func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error) {
		svc, err := NewDataflowService(ctx, dataflowID, 0, 0)
		if err != nil {
			t.Fatalf("NewDataflowService: %v", err)
		}
		if svc.progressFunc == nil {
			t.Fatal("expected default progress func to be injected")
		}
		return newNoopDataflowService(ctx, dataflowID)
	})
	if err := handler.Handle(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskHandler_Dataflow_UsesTaskContext(t *testing.T) {
	ctx := makeTaskHandlerTestContext("dataflow")
	type ctxKey string
	const key ctxKey = "trace"
	ctx.Ctx = context.WithValue(context.Background(), key, "task-ctx")

	handler := NewTaskHandler(ctx).WithDataflowServiceFactory(func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error) {
		return mustNewDataflowService(t, ctx, dataflowID, 0, 0).
			WithLoadDSLFunc(func(ctx context.Context, dataflowID string) (string, string, error) {
				return `{"nodes":[{"id":"stub-node"}],"edges":[]}`, dataflowID, nil
			}).
			WithRunPipelineFunc(func(runCtx context.Context, dsl string) (map[string]any, string, error) {
				if got := runCtx.Value(key); got != "task-ctx" {
					t.Fatalf("runCtx value = %v, want task-ctx", got)
				}
				return map[string]any{"chunks": []map[string]any{{"text": "stub", "q_2_vec": []float64{0.1, 0.2}}}}, dsl, nil
			}).
			WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
				return nil, nil
			}).
			WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil }).
			WithDocService(&stubDocService{}).
			WithChunkCounter(&stubChunkCounter{}), nil
	})
	if err := handler.Handle(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskHandler_Dataflow_ShowsProgressAndPipelineLog(t *testing.T) {
	ctx := makeTaskHandlerTestContext("dataflow")
	ctx.Doc.PipelineID = testStrPtr("flow-1")
	ctx.Doc.Name = testStrPtr("verify-dataflow.pdf")

	var pipelineCalled bool
	var insertCalled bool
	var logCreateCalls int
	var insertedChunkCount int
	var progressProgs []float64
	var progressMsgs []string

	ctx.ProgressFunc = func(prog float64, msg string) {
		progressProgs = append(progressProgs, prog)
		progressMsgs = append(progressMsgs, msg)
		t.Logf("progress: prog=%.2f msg=%q", prog, msg)
	}

	handler := NewTaskHandler(ctx).WithDataflowServiceFactory(func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error) {
		svc := mustNewDataflowService(t, ctx, dataflowID, 0, 0).
			WithLoadDSLFunc(func(ctx context.Context, dataflowID string) (string, string, error) {
				return `{"nodes":[{"id":"stub-node"}],"edges":[]}`, dataflowID, nil
			}).
			WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
				pipelineCalled = true
				t.Log("mock pipeline.run called")
				return map[string]any{
					"chunks": []map[string]any{
						{"text": "stub dataflow chunk", "q_2_vec": []float64{0.1, 0.2}},
					},
				}, dsl, nil
			}).
			WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
				insertCalled = true
				insertedChunkCount = len(chunks)
				t.Logf("mock insertChunks called: %d chunks", len(chunks))
				return nil, nil
			}).
			WithLogCreateFunc(func(log *entity.PipelineOperationLog) error {
				logCreateCalls++
				if log.PipelineID != nil {
					t.Logf("mock pipeline log created: pipeline_id=%s", *log.PipelineID)
				}
				return nil
			}).
			WithDocService(&stubDocService{}).
			WithChunkCounter(&stubChunkCounter{}).
			WithGetEmbeddingModelFunc(func(tenantID, embdID string) (*models.EmbeddingModel, error) {
				return nil, nil
			})
		return svc, nil
	})

	if err := handler.Handle(); err != nil {
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
	if len(progressProgs) == 0 {
		t.Fatal("expected progress callbacks, got none")
	}

	foundStartIndex := false
	for _, msg := range progressMsgs {
		if strings.Contains(msg, "Start to index") {
			foundStartIndex = true
			break
		}
	}
	if !foundStartIndex {
		t.Fatalf("expected progress message containing %q, got %v", "Start to index", progressMsgs)
	}

	if got := progressProgs[len(progressProgs)-1]; got != 1.0 {
		t.Fatalf("final progress = %v, want 1.0", got)
	}

	lastMsg := progressMsgs[len(progressMsgs)-1]
	if !strings.Contains(lastMsg, "Indexing done") {
		t.Fatalf("final progress msg = %q, want substring %q", lastMsg, "Indexing done")
	}

	if logCreateCalls != 1 {
		t.Fatalf("logCreateCalls = %d, want 1", logCreateCalls)
	}
}
