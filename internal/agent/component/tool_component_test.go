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
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
)

type fakeToolAdapter struct {
	args   map[string]any
	calls  int
	events []string
	out    string
	err    error
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (f *fakeToolAdapter) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	f.calls++
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	if f.out != "" || f.err != nil {
		return f.out, f.err
	}
	return `{"results":[{"title":"RAGFlow","url":"https://ragflow.io","content":"RAG engine"}]}`, nil
}

func (f *fakeToolAdapter) ComponentSpec() agenttool.ComponentSpec {
	return agenttool.ComponentSpec{
		Inputs:    map[string]string{"query": "Search query."},
		Outputs:   map[string]string{"formalized_content": "Rendered results.", "json": "Raw results."},
		InputForm: map[string]any{"query": map[string]any{"name": "Query", "type": "line"}},
	}
}

func (f *fakeToolAdapter) BuildReferences(_ context.Context, results []any) ([]map[string]any, []map[string]any) {
	f.events = append(f.events, "references")
	return []map[string]any{{"chunk_id": "1", "content": "RAG engine", "docnm_kwd": "RAGFlow"}},
		[]map[string]any{{"doc_name": "RAGFlow", "doc_id": "1", "count": 1}}
}

func (f *fakeToolAdapter) BuildComponentOutputs(results []any, chunks []map[string]any) map[string]any {
	f.events = append(f.events, "render")
	formalizedContent := ""
	if len(chunks) > 0 {
		formalizedContent, _ = chunks[0]["content"].(string)
	}
	return map[string]any{
		"json":               results,
		"formalized_content": formalizedContent,
	}
}

func TestToolBackedComponentRegisteredGitHubFactory(t *testing.T) {
	c, err := New("GitHub", map[string]any{
		"top_n":   float64(10),
		"query":   "runtime query",
		"outputs": map[string]any{"json": map[string]any{}},
		"setups":  map[string]any{"query": "configured query"},
	})
	if err != nil {
		t.Fatalf("New(GitHub): %v", err)
	}
	if _, ok := c.(*ToolBackedComponent); !ok {
		t.Fatalf("New(GitHub) returned %T, want *ToolBackedComponent", c)
	}
	if c.Name() != "GitHub" {
		t.Fatalf("Name = %q, want GitHub", c.Name())
	}
	form := c.(interface{ GetInputForm() map[string]any }).GetInputForm()
	if query, ok := form["query"].(map[string]any); !ok || query["type"] != "line" {
		t.Fatalf("query input form = %#v, want line", form["query"])
	}
}

func TestToolBackedComponentCanvasBuildWorkflow(t *testing.T) {
	c := &canvas.Canvas{
		Components: map[string]canvas.CanvasComponent{
			"begin_0": {
				Obj:        canvas.CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"github_0"},
			},
			"github_0": {
				Obj:      canvas.CanvasComponentObj{ComponentName: "GitHub", Params: map[string]any{"top_n": float64(10)}},
				Upstream: []string{"begin_0"},
			},
		},
		Path: []string{"begin_0", "github_0"},
	}
	if _, err := canvas.BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow with GitHub: %v", err)
	}
}

func TestToolBackedComponentInvokeOrdersReferencesBeforeRendering(t *testing.T) {
	fake := &fakeToolAdapter{}
	c := &ToolBackedComponent{name: "Search", tool: fake, spec: fake.ComponentSpec()}
	state := runtime.NewCanvasState("run", "task")
	out, err := c.Invoke(runtime.WithState(context.Background(), state), map[string]any{
		"query":   "ragflow",
		"top_n":   float64(10),
		"outputs": map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fake.args["query"] != "ragflow" {
		t.Fatalf("runtime args = %#v", fake.args)
	}
	if fake.args["top_n"] != float64(10) || fake.args["outputs"] == nil {
		t.Fatalf("generic component did not pass all inputs: %#v", fake.args)
	}
	if !reflect.DeepEqual(fake.events, []string{"references", "render"}) {
		t.Fatalf("post-process order = %#v, want references then render", fake.events)
	}
	if out["formalized_content"] != "RAG engine" {
		t.Fatalf("formalized_content = %#v", out["formalized_content"])
	}
	if len(state.GetRetrievalChunks()) != 1 {
		t.Fatalf("retrieval chunks = %#v", state.GetRetrievalChunks())
	}
}

func TestToolBackedComponentReturnsErrorEnvelopeWithoutReferences(t *testing.T) {
	fake := &fakeToolAdapter{
		out: `{"results":[],"_ERROR":"rate limited"}`,
		err: errors.New("rate limited"),
	}
	c := &ToolBackedComponent{name: "Search", tool: fake, spec: fake.ComponentSpec()}
	state := runtime.NewCanvasState("run-error", "task-error")
	out, err := c.Invoke(runtime.WithState(context.Background(), state), map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["_ERROR"] != "rate limited" || out["formalized_content"] != "" {
		t.Fatalf("error outputs = %#v", out)
	}
	if len(state.GetRetrievalChunks()) != 0 {
		t.Fatalf("error path recorded references: %#v", state.GetRetrievalChunks())
	}
}

func TestToolBackedComponentGitHubIntegration(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverCalls++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"items":[{"name":"ragflow","html_url":"https://github.com/infiniflow/ragflow","description":"RAG engine","watchers":12000,"private":false}]}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	helper := agenttool.NewHTTPHelper().WithClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		cloned := request.Clone(request.Context())
		cloned.URL.Scheme = target.Scheme
		cloned.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(cloned)
	})})
	github := agenttool.NewGitHubToolWith(helper)
	component := &ToolBackedComponent{name: "GitHub", tool: github, spec: github.ComponentSpec()}
	state := runtime.NewCanvasState("run-github", "task-github")
	empty, err := component.Invoke(context.Background(), map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke(empty query): %v", err)
	}
	if serverCalls != 0 || len(empty["json"].([]any)) != 0 || empty["formalized_content"] != "" {
		t.Fatalf("empty query result = %#v, server calls = %d", empty, serverCalls)
	}
	out, err := component.Invoke(runtime.WithState(context.Background(), state), map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	rendered, _ := out["formalized_content"].(string)
	if !strings.Contains(rendered, "Title: ragflow") || !strings.Contains(rendered, "RAG engine\n stars:12000") {
		t.Fatalf("formalized_content = %q", rendered)
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 1 || results[0].(map[string]any)["private"] != false {
		t.Fatalf("raw json results = %#v", out["json"])
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 || chunks[0]["document_name"] != "ragflow" {
		t.Fatalf("recorded references = %#v", chunks)
	}
}
