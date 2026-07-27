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
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
)

type fakeGitHubInvoker struct {
	args map[string]any
	out  string
	err  error
}

func (f *fakeGitHubInvoker) InvokableRun(_ context.Context, argsJSON string) (string, error) {
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	return f.out, f.err
}

func TestGitHub_RegisteredFactory(t *testing.T) {
	c, err := New("GitHub", map[string]any{"top_n": float64(10)})
	if err != nil {
		t.Fatalf("New(GitHub) errored: %v", err)
	}
	if got := c.Name(); got != "GitHub" {
		t.Fatalf("Name() = %q, want GitHub", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("GitHub component does not expose GetInputForm")
	}
	query, ok := formGetter.GetInputForm()["query"].(map[string]any)
	if !ok || query["type"] != "line" {
		t.Fatalf("GetInputForm query = %#v, want line input", query)
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestGitHub_CanvasBuildWorkflow(t *testing.T) {
	c := &canvas.Canvas{
		Components: map[string]canvas.CanvasComponent{
			"begin_0": {
				Obj:        canvas.CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"github_0"},
			},
			"github_0": {
				Obj:        canvas.CanvasComponentObj{ComponentName: "GitHub", Params: map[string]any{"top_n": float64(10)}},
				Upstream:   []string{"begin_0"},
				Downstream: []string{},
			},
		},
		Path: []string{"begin_0", "github_0"},
	}
	if _, err := canvas.BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow with GitHub component: %v", err)
	}
}

func TestGitHub_InvokeMatchesPythonOutputsAndReferences(t *testing.T) {
	fake := &fakeGitHubInvoker{out: `{"results":[{"name":"ragflow","html_url":"https://github.com/infiniflow/ragflow","description":"RAG engine","watchers":12000}]}`}
	c := newGitHubComponentWithInvoker(fake)
	state := runtime.NewCanvasState("run-github", "task-github")
	ctx := runtime.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := fake.args["query"]; got != "ragflow" {
		t.Errorf("query = %v, want ragflow", got)
	}
	content, _ := out["formalized_content"].(string)
	for _, want := range []string{"Title: ragflow", "URL: https://github.com/infiniflow/ragflow", "RAG engine\n stars:12000"} {
		if !strings.Contains(content, want) {
			t.Errorf("formalized_content missing %q: %q", want, content)
		}
	}
	items := anySlice(out["json"])
	if len(items) != 1 {
		t.Fatalf("json length = %d, want 1", len(items))
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 {
		t.Fatalf("recorded reference chunks = %d, want 1", len(chunks))
	}
	if chunks[0]["document_name"] != "ragflow" || chunks[0]["url"] != "https://github.com/infiniflow/ragflow" {
		t.Errorf("reference chunk metadata = %#v", chunks[0])
	}
	if chunks[0]["similarity"] != 1 {
		t.Errorf("reference similarity = %v, want 1", chunks[0]["similarity"])
	}
	encodedState, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var statePayload struct {
		Retrieval struct {
			DocAggs map[string]any `json:"doc_aggs"`
		} `json:"retrieval"`
	}
	if err := json.Unmarshal(encodedState, &statePayload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if _, ok := statePayload.Retrieval.DocAggs["ragflow"]; !ok {
		t.Fatalf("doc_aggs missing ragflow: %#v", statePayload.Retrieval.DocAggs)
	}
}

func TestGitHub_InvokeEmptyQueryMatchesPython(t *testing.T) {
	fake := &fakeGitHubInvoker{}
	c := newGitHubComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := out["formalized_content"]; got != "" {
		t.Errorf("formalized_content = %v, want empty", got)
	}
	if items := anySlice(out["json"]); len(items) != 0 {
		t.Errorf("json = %#v, want empty", items)
	}
	if fake.args != nil {
		t.Fatal("GitHub tool was called for an empty query")
	}
}

func TestGitHub_LimitReferencesKeepsBoundaryChunk(t *testing.T) {
	chunks := []map[string]any{
		{"content": "first repository description"},
		{"content": "second repository description"},
	}
	limited := limitGitHubReferences(chunks, 1)
	if len(limited) != 1 {
		t.Fatalf("limited chunks = %d, want 1", len(limited))
	}
}
