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
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

func TestPipelineExecutor_DefaultLoadDSL_UsesUserCanvas(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	dslMap := entity.JSONMap{"dsl": map[string]any{"graph": map[string]any{"nodes": []any{}, "edges": []any{}}}}
	title := "title 1"
	if err := dao.NewUserCanvasDAO().Create(&entity.UserCanvas{Title: &title, ID: "canvas-1", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", DSL: dslMap}); err != nil {
		t.Fatalf("create user canvas: %v", err)
	}

	ctx := makeTaskCtx()
	svc := mustNewPipelineExecutor(t, ctx, "canvas-1", 0)
	gotDSL, correctedID, err := svc.loadDSLFunc(context.Background(), "canvas-1")
	if err != nil {
		t.Fatalf("loadDSLFunc: %v", err)
	}
	if correctedID != "canvas-1" {
		t.Fatalf("correctedID = %q, want canvas-1", correctedID)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(gotDSL), &decoded); err != nil {
		t.Fatalf("unmarshal dsl: %v", err)
	}
	if _, ok := decoded["dsl"].(map[string]any); !ok {
		t.Fatalf("decoded dsl = %v, want top-level dsl map", decoded)
	}
}

func TestExtractPipelinePayload_UnwrapsSingleTerminalOutput(t *testing.T) {
	dsl := `{"dsl":{"components":{"Parser:A":{"downstream":["Tokenizer:B"]},"Tokenizer:B":{"downstream":[]}}}}`
	out := map[string]any{
		"Tokenizer:B": map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{{"text": "hello world"}},
		},
		"state": map[string]any{"Tokenizer:B": map[string]any{"output_format": "chunks"}},
	}

	payload, err := pipelinepkg.ExtractPayload(dsl, out)
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	if len(chunks) != 1 || chunks[0]["text"] != "hello world" {
		t.Fatalf("chunks = %v, want single hello world chunk", chunks)
	}
}

func TestExtractPipelinePayload_ErrorsOnMultipleTerminals(t *testing.T) {
	dsl := `{"dsl":{"components":{"Tokenizer:A":{"downstream":[]},"Tokenizer:B":{"downstream":[]}}}}`
	_, err := pipelinepkg.ExtractPayload(dsl, map[string]any{
		"Tokenizer:A": map[string]any{"output_format": "chunks"},
		"Tokenizer:B": map[string]any{"output_format": "chunks"},
	})
	if err == nil {
		t.Fatal("expected error for multiple terminals")
	}
	if !strings.Contains(err.Error(), "exactly 1 terminal") {
		t.Fatalf("err = %v, want exactly 1 terminal", err)
	}
}
