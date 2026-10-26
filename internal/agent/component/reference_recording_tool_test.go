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

package component

import (
	"context"
	"errors"
	"fmt"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

type referenceRecordingFakeTool struct {
	out string
	err error
}

func (f *referenceRecordingFakeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "fake_search"}, nil
}

func (f *referenceRecordingFakeTool) InvokableRun(_ context.Context, _ string, _ ...einotool.Option) (string, error) {
	return f.out, f.err
}

func (f *referenceRecordingFakeTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	results, _ := envelope["results"].([]any)
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for i, raw := range results {
		id := fmt.Sprintf("chunk-%d", i)
		chunks = append(chunks, map[string]any{"id": id, "chunk_id": id, "document_id": id, "content": raw})
		docAggs = append(docAggs, map[string]any{"doc_id": id, "doc_name": raw})
	}
	return chunks, docAggs
}

func TestReferenceRecordingTool_RecordsAgentToolReferences(t *testing.T) {
	state := runtime.NewCanvasState("run-refs", "task-refs")
	ctx := runtime.WithState(t.Context(), state)
	inner := &referenceRecordingFakeTool{out: `{"results":["first","second"]}`}
	recorder := &referenceRecordingTool{InvokableTool: inner, builder: inner}

	out, err := recorder.InvokableRun(ctx, `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if out != inner.out {
		t.Fatalf("output = %q, want the inner tool output %q", out, inner.out)
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 2 {
		t.Fatalf("recorded chunks = %d, want 2", len(chunks))
	}
	if chunks[0]["id"] != "chunk-0" || chunks[1]["id"] != "chunk-1" {
		t.Fatalf("recorded chunk ids = %v / %v", chunks[0]["id"], chunks[1]["id"])
	}
	if aggs := state.GetRetrievalDocAggs(); len(aggs) != 2 {
		t.Fatalf("recorded doc aggs = %d, want 2", len(aggs))
	}
}

func TestReferenceRecordingTool_SkipsErrorsAndNonJSON(t *testing.T) {
	state := runtime.NewCanvasState("run-refs", "task-refs")
	ctx := runtime.WithState(t.Context(), state)

	failing := &referenceRecordingFakeTool{err: errors.New("boom")}
	if _, err := (&referenceRecordingTool{InvokableTool: failing, builder: failing}).InvokableRun(ctx, "{}"); err == nil {
		t.Fatal("expected the inner tool error to propagate")
	}
	plain := &referenceRecordingFakeTool{out: "not json"}
	if _, err := (&referenceRecordingTool{InvokableTool: plain, builder: plain}).InvokableRun(ctx, "{}"); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if chunks := state.GetRetrievalChunks(); len(chunks) != 0 {
		t.Fatalf("recorded chunks = %d, want 0 for failed/non-JSON results", len(chunks))
	}
}

func TestReferenceRecordingTool_KeepsSameTitleDifferentDocumentIDs(t *testing.T) {
	state := runtime.NewCanvasState("run-refs", "task-refs")
	ctx := runtime.WithState(t.Context(), state)
	inner := &referenceRecordingFakeTool{out: `{"results":["same title","same title"]}`}
	recorder := &referenceRecordingTool{InvokableTool: inner, builder: inner}

	if _, err := recorder.InvokableRun(ctx, `{"query":"q"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if aggs := state.GetRetrievalDocAggs(); len(aggs) != 2 {
		t.Fatalf("recorded doc aggs = %d, want 2 for distinct document ids", len(aggs))
	}
	reference := state.GetRetrievalReference()
	chunks, _ := reference["chunks"].([]any)
	docAggs, _ := reference["doc_aggs"].([]any)
	if len(chunks) != 2 || len(docAggs) != 2 {
		t.Fatalf("reference = %d chunks / %d doc aggs, want 2/2", len(chunks), len(docAggs))
	}
}

// TestWrapReferenceRecordingTools_WrapsReferenceBuilders asserts the ReAct
// tool list is decorated while buildAgentTools keeps the raw tools.
func TestWrapReferenceRecordingTools_WrapsReferenceBuilders(t *testing.T) {
	raw, err := buildAgentTools(t.Context(), AgentParam{Tools: []string{"youcom"}})
	if err != nil {
		t.Fatalf("buildAgentTools: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("raw tools = %d, want 1", len(raw))
	}
	if _, ok := raw[0].(*referenceRecordingTool); ok {
		t.Fatal("buildAgentTools must return the raw tool for metadata callers")
	}

	wrapped := wrapReferenceRecordingTools(raw)
	if len(wrapped) != 1 {
		t.Fatalf("wrapped tools = %d, want 1", len(wrapped))
	}
	if _, ok := wrapped[0].(*referenceRecordingTool); !ok {
		t.Fatalf("tool type = %T, want *referenceRecordingTool", wrapped[0])
	}
}
