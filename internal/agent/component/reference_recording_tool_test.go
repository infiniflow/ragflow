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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func TestToolInvocationRecordingToolRedactsSensitiveText(t *testing.T) {
	recorder := &toolInvocationRecorder{}
	inner := &referenceRecordingFakeTool{err: errors.New("request failed: Authorization: Bearer error-secret")}
	tool := &toolInvocationRecordingTool{InvokableTool: inner, name: "fake_search", recorder: recorder}
	_, err := tool.InvokableRun(t.Context(), `{"note":"Authorization: Bearer argument-secret","detail":"access_token=other-secret"}`)
	if err == nil {
		t.Fatal("expected inner tool error")
	}

	entries := recorder.snapshot()
	if len(entries) != 1 {
		t.Fatalf("recorded entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	for _, field := range []string{"result", "error"} {
		got, _ := entry[field].(string)
		if strings.Contains(got, "error-secret") || strings.Contains(strings.ToLower(got), "bearer") {
			t.Errorf("%s contains unredacted credential: %q", field, got)
		}
	}
	arguments, ok := entry["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("arguments = %T, want map", entry["arguments"])
	}
	for key, secret := range map[string]string{"note": "argument-secret", "detail": "other-secret"} {
		got, _ := arguments[key].(string)
		if strings.Contains(got, secret) || strings.Contains(strings.ToLower(got), "bearer") {
			t.Errorf("argument %q contains unredacted credential: %q", key, got)
		}
	}
}

func TestToolInvocationRecordingToolRedactsQuotedAndStructuredCredentials(t *testing.T) {
	rawError := `{"message":"request failed","refresh_token":"refresh secret with spaces","authToken":"auth secret with spaces","authorization":"Basic basic-secret"}`
	args := `{"note":"authorization: \"Bearer argument secret with spaces\"","detail":"token=\"quoted token secret\"","auth_note":"auth=\"quoted auth secret with spaces\""}`
	recorder := &toolInvocationRecorder{}
	inner := &referenceRecordingFakeTool{out: `{"ok":false}`, err: errors.New(rawError)}
	tool := &toolInvocationRecordingTool{InvokableTool: inner, name: "fake_search", recorder: recorder}
	if _, err := tool.InvokableRun(t.Context(), args); err == nil {
		t.Fatal("expected inner tool error")
	}

	entries := recorder.snapshot()
	if len(entries) != 1 {
		t.Fatalf("recorded entries = %d, want 1", len(entries))
	}
	for _, field := range []string{"result", "error"} {
		got, _ := entries[0][field].(string)
		for _, secret := range []string{"refresh secret", "auth secret", "basic-secret", "Bearer argument", "argument secret", "quoted token secret", "quoted auth secret", "basic ", "bearer "} {
			if strings.Contains(got, secret) {
				t.Errorf("%s contains unredacted %q: %s", field, secret, got)
			}
		}
	}
	encodedArgs, err := json.Marshal(entries[0]["arguments"])
	if err != nil {
		t.Fatalf("marshal redacted arguments: %v", err)
	}
	for _, secret := range []string{"Bearer argument", "argument secret", "quoted token secret", "quoted auth secret"} {
		if strings.Contains(string(encodedArgs), secret) {
			t.Errorf("arguments contain unredacted %q: %s", secret, encodedArgs)
		}
	}
}

func TestToolInvocationRecordingToolRedactsStructuredResult(t *testing.T) {
	recorder := &toolInvocationRecorder{}
	inner := &referenceRecordingFakeTool{out: `{"authorization":"Basic result-secret","refresh_token":"refresh result secret"}`}
	tool := &toolInvocationRecordingTool{InvokableTool: inner, name: "fake_search", recorder: recorder}
	if _, err := tool.InvokableRun(t.Context(), `{}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	got, _ := recorder.snapshot()[0]["result"].(string)
	for _, secret := range []string{"result-secret", "refresh result secret", "Basic"} {
		if strings.Contains(got, secret) {
			t.Errorf("result contains unredacted %q: %s", secret, got)
		}
	}
}

func TestRedactToolLogTextCredentials(t *testing.T) {
	for _, input := range []string{
		`Authorization: Basic hidden-credential`,
		`Authorization: Bearer hidden-credential`,
		`Authorization: hidden-credential`,
		`Authorization: "Bearer hidden-credential with spaces"`,
		`authorization='Basic hidden-credential with spaces'`,
		`token=hidden-credential`,
		`refresh_token=hidden-credential`,
		`authToken=hidden-credential`,
		`"token": "hidden-credential with spaces"`,
		`"auth": "hidden-credential with spaces"`,
		`api_key=hidden-credential`,
		`password='hidden-credential with spaces'`,
		`secret=hidden-credential`,
	} {
		t.Run(input, func(t *testing.T) {
			got := redactToolLogText(input)
			if strings.Contains(got, "hidden-credential") || strings.Contains(got, "with spaces") || !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("redacted text = %q, want complete credential removed", got)
			}
		})
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
