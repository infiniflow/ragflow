//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package task

import (
	"context"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func strptr(s string) *string { return &s }

// TestExecute_NonPersistSkipsPipelineLog asserts that a non-persist run
// (Doc.ID == CANVAS_DEBUG_DOC_ID) returns before recording the pipeline
// operation log. A non-persist run must produce no persistent side effects.
func TestExecute_NonPersistSkipsPipelineLog(t *testing.T) {
	taskCtx := &TaskContext{
		Ctx: context.Background(),
		Doc: entity.Document{
			ID:         CANVAS_DEBUG_DOC_ID,
			KbID:       "kb-1",
			Name:       strptr("doc.pdf"),
			ParserID:   "parser-1",
			Suffix:     "pdf",
			Type:       "pdf",
			SourceType: "local",
		},
		KB:     entity.Knowledgebase{ID: "kb-1"},
		Tenant: entity.Tenant{ID: "tenant-1"},
	}

	exec, err := NewPipelineExecutor(taskCtx, "canvas-1", 10)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	exec.WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
		return `"dsl"`, "", nil
	})
	exec.WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
		return map[string]any{"chunks": []map[string]any{}}, "dsl", nil
	})

	var logCalled bool
	exec.WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
		logCalled = true
		return nil
	})

	if _, err := exec.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if logCalled {
		t.Errorf("recordPipelineLog must not be called in a non-persist (debug) run")
	}
}

// TestExecute_NonPersistReturnsChunks asserts that a non-persist (debug) run
// returns the pipeline's chunks in the result instead of discarding them. The
// chunks are surfaced for a debug HTTP endpoint to render; no DB/index writes
// happen and the pipeline operation log is never recorded.
func TestExecute_NonPersistReturnsChunks(t *testing.T) {
	taskCtx := &TaskContext{
		Ctx: context.Background(),
		Doc: entity.Document{
			ID:         CANVAS_DEBUG_DOC_ID,
			KbID:       "kb-1",
			Name:       strptr("doc.pdf"),
			ParserID:   "parser-1",
			Suffix:     "pdf",
			Type:       "pdf",
			SourceType: "local",
		},
		KB:     entity.Knowledgebase{ID: "kb-1"},
		Tenant: entity.Tenant{ID: "tenant-1"},
	}

	exec, err := NewPipelineExecutor(taskCtx, "canvas-1", 10)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	exec.WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
		return "dsl", "canvas-1", nil
	})
	exec.WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
		return map[string]any{
			"chunks": []map[string]any{
				{"id": "c1", "content": "hello", "text": "hello"},
				{"id": "c2", "content": "world", "text": "world"},
				{"id": "c3", "content": "debug", "text": "debug"},
			},
			"embedding_token_consumption": 12,
		}, "dsl", nil
	})

	var logCalled bool
	exec.WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
		logCalled = true
		return nil
	})

	result, err := exec.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatalf("Execute returned nil result in non-persist (debug) mode; expected chunks")
	}
	if len(result.Chunks) != 3 {
		t.Errorf("expected 3 chunks in result, got %d", len(result.Chunks))
	}
	if result.ChunkCount != 3 {
		t.Errorf("expected ChunkCount 3, got %d", result.ChunkCount)
	}
	if logCalled {
		t.Errorf("recordPipelineLog must not be called in a non-persist (debug) run")
	}
}
