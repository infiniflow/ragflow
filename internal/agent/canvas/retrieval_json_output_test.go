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

// retrieval_json_output_test.go — end-to-end regression for the Retrieval
// node's `json` output on a zero-hit search.
//
// The tool envelope drops its chunk array when the search returns nothing, so
// the node's `json` output used to be missing outright and a downstream
// {{<id>@json}} template failed the run with "Can't find variable". This test
// drives a full Begin → Retrieval → Message canvas so the component's output
// contract and the template resolution are exercised together — the failure
// the Agent chat surfaced.
package canvas

import (
	"context"
	"testing"

	// Blank-import to trigger component init(), which installs the real
	// component factory (same wiring as loop_semantics_test.go). Without it
	// BuildWorkflow falls back to its placeholder echo body and the canvas
	// never invokes the real Retrieval / Message components.
	_ "ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

type zeroHitRetrievalService struct{}

func (zeroHitRetrievalService) Search(context.Context, *gorm.DB, runtime.RetrievalRequest) ([]runtime.RetrievalChunk, error) {
	return nil, nil
}

func TestRetrievalJSONOutputResolvesOnEmptySearch(t *testing.T) {
	previous := runtime.GetRetrievalService()
	runtime.SetRetrievalService(zeroHitRetrievalService{})
	t.Cleanup(func() { runtime.SetRetrievalService(previous) })

	dsl := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"retrieval_0"},
			},
			"retrieval_0": {
				Obj: CanvasComponentObj{ComponentName: "Retrieval", Params: map[string]any{
					"retrieval_from": "dataset",
					"kb_ids":         []any{"kb-1"},
					"query":          "love",
				}},
				Upstream:   []string{"begin"},
				Downstream: []string{"message_0"},
			},
			"message_0": {
				Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{
					"text": "chunks: {retrieval_0@json}",
				}},
				Upstream:   []string{"retrieval_0"},
				Downstream: []string{},
			},
		},
		Path: []string{"begin", "retrieval_0", "message_0"},
	}

	ctx := t.Context()
	compiled, err := Compile(ctx, dsl)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	state := NewCanvasState("run-retrieval-json", "task-retrieval-json")
	if _, runErr := compiled.Workflow.Invoke(withState(ctx, state), map[string]any{"query": "love"}); runErr != nil {
		t.Fatalf("Invoke: %v", runErr)
	}

	chunks, err := state.GetVar("retrieval_0@json")
	if err != nil {
		t.Fatalf("GetVar(retrieval_0@json): %v", err)
	}
	values, ok := chunks.([]any)
	if !ok || len(values) != 0 {
		t.Fatalf("retrieval_0@json = %#v, want an empty array", chunks)
	}
	content, err := state.GetVar("message_0@content")
	if err != nil {
		t.Fatalf("GetVar(message_0@content): %v", err)
	}
	if content != "chunks: []" {
		t.Fatalf("message_0@content = %#v, want %q", content, "chunks: []")
	}
}
