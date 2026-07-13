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
	"math"
	"strconv"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	agentcanvas "ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/tokenizer"
)

type fakeSearXNGInvoker struct {
	args  map[string]any
	calls int
	out   string
	err   error
}

func (f *fakeSearXNGInvoker) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	f.calls++
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	return f.out, f.err
}

func TestSearXNGRegisteredFactoryMatchesPythonSurface(t *testing.T) {
	t.Parallel()

	component, err := New("SearXNG", map[string]any{
		"top_n":       "10",
		"searxng_url": "http://localhost:4000",
		"outputs":     map[string]any{"formalized_content": map[string]any{}},
		"setups":      map[string]any{"query": "configured query"},
	})
	if err != nil {
		t.Fatalf("New(SearXNG): %v", err)
	}
	if component.Name() != "SearXNG" {
		t.Fatalf("Name = %q, want SearXNG", component.Name())
	}
	if _, ok := component.Inputs()["query"]; !ok {
		t.Fatal("Inputs missing query")
	}
	if _, ok := component.Inputs()["searxng_url"]; !ok {
		t.Fatal("Inputs missing searxng_url")
	}
	for _, output := range []string{"formalized_content", "json"} {
		if _, ok := component.Outputs()[output]; !ok {
			t.Fatalf("Outputs missing %s", output)
		}
	}
	form := component.(*searxngComponent).GetInputForm()
	if len(form) != 2 {
		t.Fatalf("GetInputForm size = %d, want 2", len(form))
	}
	query := form["query"].(map[string]any)
	if query["name"] != "Query" || query["type"] != "line" {
		t.Fatalf("query form = %#v", query)
	}
	serverURL := form["searxng_url"].(map[string]any)
	if serverURL["name"] != "SearXNG URL" || serverURL["type"] != "line" || serverURL["placeholder"] != "http://localhost:4000" {
		t.Fatalf("searxng_url form = %#v", serverURL)
	}
}

func TestSearXNGInvokePreservesRawJSONPromptAndReferences(t *testing.T) {
	t.Parallel()

	content := "RAGFlow content ![img](data:image/png;base64,AAAA) remains"
	fake := &fakeSearXNGInvoker{out: `{"results":[{"title":"RAGFlow\nDocs","url":"https://ragflow.io","content":` + mustJSONText(t, content) + `,"engine":"bing","score":0.9},{"title":"Empty","url":"https://example.com","content":""}]}`}
	component := newSearXNGComponentWithInvoker(fake)
	state := runtime.NewCanvasState("run", "task")
	ctx := runtime.WithState(context.Background(), state)
	out, err := component.Invoke(ctx, map[string]any{
		"query":       "  ragflow  ",
		"searxng_url": "http://localhost:4000",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fake.args["query"] != "  ragflow  " || fake.args["searxng_url"] != "http://localhost:4000" {
		t.Fatalf("runtime args = %#v", fake.args)
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("json = %#v, want two raw results", out["json"])
	}
	first := results[0].(map[string]any)
	if first["engine"] != "bing" || first["score"] != float64(0.9) {
		t.Fatalf("raw result lost fields: %#v", first)
	}

	cleaned := "RAGFlow content  remains"
	documentID := hashSearXNGString(cleaned, 100000000)
	referenceID := hashSearXNGString(strconv.Itoa(documentID), 500)
	if documentID != 93760153 || referenceID != 491 {
		t.Fatalf("hash parity = %d/%d, want Python 93760153/491", documentID, referenceID)
	}
	formalized := out["formalized_content"].(string)
	for _, want := range []string{
		"ID: " + strconv.Itoa(referenceID),
		"Title: RAGFlow Docs",
		"URL: https://ragflow.io",
		"Content:\n" + cleaned,
	} {
		if !strings.Contains(formalized, want) {
			t.Fatalf("formalized_content missing %q: %s", want, formalized)
		}
	}

	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 {
		t.Fatalf("retrieval chunks = %#v, want one non-empty-content chunk", chunks)
	}
	if chunks[0]["document_id"] != strconv.Itoa(documentID) || chunks[0]["content"] != cleaned {
		t.Fatalf("retrieval chunk = %#v", chunks[0])
	}
	if chunks[0]["id"] != strconv.Itoa(referenceID) {
		t.Fatalf("chunk id = %#v, want displayed reference ID", chunks[0]["id"])
	}
	if chunks[0]["chunk_id"] != strconv.Itoa(documentID) || chunks[0]["doc_id"] != strconv.Itoa(documentID) {
		t.Fatalf("raw document IDs = %#v, want Python content hash", chunks[0])
	}
	if chunks[0]["similarity"] != 1 {
		t.Fatalf("similarity = %#v, want 1", chunks[0]["similarity"])
	}
	aggs := state.GetRetrievalDocAggs()
	if len(aggs) != 1 || aggs["RAGFlow\nDocs"]["doc_id"] != strconv.Itoa(documentID) {
		t.Fatalf("doc_aggs = %#v", aggs)
	}
}

func TestSearXNGInvokeEmptyQuerySkipsTool(t *testing.T) {
	t.Parallel()

	fake := &fakeSearXNGInvoker{}
	component := newSearXNGComponentWithInvoker(fake)
	out, err := component.Invoke(context.Background(), map[string]any{"query": "   "})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fake.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", fake.calls)
	}
	if out["formalized_content"] != "" {
		t.Fatalf("formalized_content = %#v", out["formalized_content"])
	}
	if results, ok := out["json"].([]any); !ok || len(results) != 0 {
		t.Fatalf("json = %#v, want empty []any", out["json"])
	}
}

func TestSearXNGInvokePreservesErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := &fakeSearXNGInvoker{
		out: `{"results":[],"_ERROR":"Network error: upstream down"}`,
		err: errors.New("upstream down"),
	}
	component := newSearXNGComponentWithInvoker(fake)
	out, err := component.Invoke(context.Background(), map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["_ERROR"] != "Network error: upstream down" || out["formalized_content"] != "" {
		t.Fatalf("output = %#v", out)
	}
}

func TestSearXNGPromptBoundaryMatchesPython(t *testing.T) {
	t.Parallel()

	first := "small content"
	second := strings.Repeat("cross the token budget ", 50)
	third := "must not render"
	chunks := []map[string]any{
		{"id": "1", "content": first},
		{"id": "2", "content": second},
		{"id": "3", "content": third},
	}
	firstTokens := tokenizer.NumTokensFromString(first)
	maxTokens := int(math.Ceil(float64(firstTokens)/0.97)) + 1
	rendered := renderSearXNGReferences(chunks, maxTokens)
	if !strings.Contains(rendered, "ID: 1") || !strings.Contains(rendered, "ID: 2") {
		t.Fatalf("crossing chunk must be included like Python: %s", rendered)
	}
	if strings.Contains(rendered, "ID: 3") {
		t.Fatalf("chunk after budget crossing must be excluded: %s", rendered)
	}
}

func TestSearXNGBuildWorkflowUsesPythonComponentName(t *testing.T) {
	canvas := &agentcanvas.Canvas{
		Components: map[string]agentcanvas.CanvasComponent{
			"begin_0": {
				Obj:        agentcanvas.CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"searxng_0"},
			},
			"searxng_0": {
				Obj: agentcanvas.CanvasComponentObj{ComponentName: "SearXNG", Params: map[string]any{
					"top_n":       "10",
					"searxng_url": "http://localhost:4000",
				}},
				Upstream:   []string{"begin_0"},
				Downstream: []string{"message_0"},
			},
			"message_0": {
				Obj:      agentcanvas.CanvasComponentObj{ComponentName: "Message", Params: map[string]any{}},
				Upstream: []string{"searxng_0"},
			},
		},
		Path: []string{"begin_0", "searxng_0", "message_0"},
	}
	if _, err := agentcanvas.BuildWorkflow(context.Background(), canvas); err != nil {
		t.Fatalf("BuildWorkflow with SearXNG: %v", err)
	}
}

func mustJSONText(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(raw)
}
