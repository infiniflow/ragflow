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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
)

type fakeToolAdapter struct {
	args              map[string]any
	referenceEnvelope map[string]any
	outputEnvelope    map[string]any
	calls             int
	events            []string
	out               string
	err               error
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

func (f *fakeToolAdapter) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	f.referenceEnvelope = envelope
	f.events = append(f.events, "references")
	return []map[string]any{{"chunk_id": "1", "content": "RAG engine", "docnm_kwd": "RAGFlow"}},
		[]map[string]any{{"doc_name": "RAGFlow", "doc_id": "1", "count": 1}}
}

func (f *fakeToolAdapter) BuildComponentOutputs(envelope map[string]any) map[string]any {
	f.outputEnvelope = envelope
	f.events = append(f.events, "render")
	formalizedContent := ""
	results := anySlice(envelope["results"])
	if len(results) > 0 {
		if result, ok := results[0].(map[string]any); ok {
			formalizedContent, _ = result["content"].(string)
		}
	}
	return map[string]any{
		"json":               results,
		"formalized_content": formalizedContent,
		"tool_metadata":      envelope["tool_metadata"],
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

func TestToolBackedComponentRegisteredFactories(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		params    map[string]any
		outputKey string
		inputKey  string
	}{
		{
			name:      "AnySearchExtract",
			toolName:  "AnySearchExtract",
			params:    map[string]any{"api_key": "stored-key", "url": "", "outputs": map[string]any{"formalized_content": map[string]any{}, "json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "url",
		},
		{
			name:      "AnySearchSearch",
			toolName:  "AnySearchSearch",
			params:    map[string]any{"api_key": "stored-key", "top_n": float64(3), "zone": "intl", "language": "en", "outputs": map[string]any{"formalized_content": map[string]any{}, "json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "ArXiv",
			toolName:  "ArXiv",
			params:    map[string]any{"top_n": float64(3), "sort_by": "relevance", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "BGPT",
			toolName:  "BGPT",
			params:    map[string]any{"api_key": "stored-key", "top_n": float64(3), "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "KeenableSearch",
			toolName:  "KeenableSearch",
			params:    map[string]any{"api_key": "stored-key", "mode": "realtime", "top_n": float64(3), "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "SofyaSearch",
			toolName:  "SofyaSearch",
			params:    map[string]any{"api_key": "stored-key", "search_depth": "snippets", "top_n": float64(3), "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "PubMed",
			toolName:  "PubMed",
			params:    map[string]any{"top_n": float64(3), "email": "node@example.com", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "Google",
			toolName:  "Google",
			params:    map[string]any{"api_key": "stored-key", "country": "cn", "language": "en", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "q",
		},
		{
			name:     "ExeSQL",
			toolName: "ExeSQL",
			params: map[string]any{
				"database": "demo", "username": "root", "host": "db.example.com", "port": float64(3306), "password": "secret",
				"top_n": float64(50), "outputs": map[string]any{"json": map[string]any{}},
			},
			outputKey: "json",
			inputKey:  "sql",
		},
		{
			name:      "GoogleScholar",
			toolName:  "GoogleScholar",
			params:    map[string]any{"top_n": float64(3), "sort_by": "relevance", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "DuckDuckGo",
			toolName:  "DuckDuckGo",
			params:    map[string]any{"top_n": float64(3), "channel": "news", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:     "Email",
			toolName: "Email",
			params: map[string]any{
				"smtp_server": "smtp.example.com", "smtp_port": float64(465), "email": "sender@example.com",
				"password": "secret", "sender_name": "Sender", "outputs": map[string]any{"success": map[string]any{}},
			},
			outputKey: "success",
			inputKey:  "to_email",
		},
		{
			name:      "QueritContents",
			toolName:  "QueritContents",
			params:    map[string]any{"api_key": "stored-key", "format": "markdown", "crawl_timeout": float64(10), "extras_meta": true, "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "urls",
		},
		{
			name:      "QueritSearch",
			toolName:  "QueritSearch",
			params:    map[string]any{"api_key": "stored-key", "count": float64(10), "chunks_per_doc": float64(3), "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "Search1APICrawl",
			toolName:  "Search1APICrawl",
			params:    map[string]any{"api_key": "stored-key", "url": "{begin@url}", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "url",
		},
		{
			name:      "Search1APISearch",
			toolName:  "Search1APISearch",
			params:    map[string]any{"api_key": "stored-key", "channel": "news", "search_service": "reuters", "top_n": float64(3), "outputs": map[string]any{"formalized_content": map[string]any{}, "json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "SearXNG",
			toolName:  "SearXNG",
			params:    map[string]any{"top_n": "10", "searxng_url": "https://searx.example.com", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "WenCai",
			toolName:  "WenCai",
			params:    map[string]any{"top_n": float64(20), "query_type": "stock", "outputs": map[string]any{"report": map[string]any{}}},
			outputKey: "report",
			inputKey:  "query",
		},
		{
			name:      "Wikipedia",
			toolName:  "Wikipedia",
			params:    map[string]any{"top_n": float64(3), "language": "en", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
		{
			name:      "YahooFinance",
			toolName:  "YahooFinance",
			params:    map[string]any{"outputs": map[string]any{"report": map[string]any{}}},
			outputKey: "report",
			inputKey:  "stock_code",
		},
		{
			name:      "TavilyExtract",
			toolName:  "TavilyExtract",
			params:    map[string]any{"api_key": "stored-key", "extract_depth": "advanced", "format": "text", "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "urls",
		},
		{
			name:      "TavilySearch",
			toolName:  "TavilySearch",
			params:    map[string]any{"api_key": "stored-key", "search_depth": "advanced", "max_results": float64(3), "outputs": map[string]any{"json": map[string]any{}}},
			outputKey: "json",
			inputKey:  "query",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.toolName, tt.params)
			if err != nil {
				t.Fatalf("New(%s): %v", tt.toolName, err)
			}
			if _, ok := c.(*ToolBackedComponent); !ok {
				t.Fatalf("New(%s) returned %T, want *ToolBackedComponent", tt.toolName, c)
			}
			if c.Name() != tt.toolName {
				t.Fatalf("Name = %q, want %q", c.Name(), tt.toolName)
			}
			if _, ok := c.Inputs()[tt.inputKey]; !ok {
				t.Fatalf("Inputs missing %q: %#v", tt.inputKey, c.Inputs())
			}
			if _, ok := c.Outputs()[tt.outputKey]; !ok {
				t.Fatalf("Outputs missing %q: %#v", tt.outputKey, c.Outputs())
			}
		})
	}
}

func TestToolBackedComponentWenCaiInvoke(t *testing.T) {
	c, err := New("WenCai", map[string]any{"top_n": float64(20), "query_type": "stock"})
	if err != nil {
		t.Fatalf("New(WenCai): %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"query":     "商业航天",
		"unrelated": "ignored by the tool parameter struct",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["report"] != "" {
		t.Fatalf("outputs = %#v, want empty report", out)
	}
}

func TestToolBackedComponentRegisteredBuildWorkflow(t *testing.T) {
	for _, componentName := range []string{"AnySearchExtract", "AnySearchSearch", "ArXiv", "BGPT", "DuckDuckGo", "Email", "Google", "GoogleScholar", "KeenableSearch", "PubMed", "QueritContents", "QueritSearch", "Search1APICrawl", "Search1APISearch", "SearXNG", "SofyaSearch", "WenCai", "TavilyExtract", "TavilySearch", "Wikipedia", "YahooFinance"} {
		t.Run(componentName, func(t *testing.T) {
			c := &canvas.Canvas{
				Components: map[string]canvas.CanvasComponent{
					"begin_0": {
						Obj:        canvas.CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
						Downstream: []string{"tool_0"},
					},
					"tool_0": {
						Obj:      canvas.CanvasComponentObj{ComponentName: componentName, Params: map[string]any{}},
						Upstream: []string{"begin_0"},
					},
				},
				Path: []string{"begin_0", "tool_0"},
			}
			if _, err := canvas.BuildWorkflow(t.Context(), c); err != nil {
				t.Fatalf("BuildWorkflow with %s: %v", componentName, err)
			}
		})
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
	if _, err := canvas.BuildWorkflow(t.Context(), c); err != nil {
		t.Fatalf("BuildWorkflow with GitHub: %v", err)
	}
}

func TestToolBackedComponentInvokeOrdersReferencesBeforeRendering(t *testing.T) {
	fake := &fakeToolAdapter{out: `{"results":[{"title":"RAGFlow","content":"RAG engine"}],"tool_metadata":{"request_id":"request-1"}}`}
	c := &ToolBackedComponent{name: "Search", tool: fake, spec: fake.ComponentSpec()}
	state := runtime.NewCanvasState("run", "task")
	out, err := c.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{
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
	if fake.referenceEnvelope["tool_metadata"] == nil || fake.outputEnvelope["tool_metadata"] == nil {
		t.Fatalf("complete tool envelope was not passed to post-processors: references=%#v outputs=%#v", fake.referenceEnvelope, fake.outputEnvelope)
	}
	if _, exists := fake.outputEnvelope["chunks"]; exists {
		t.Fatalf("generic component injected references into the tool envelope: %#v", fake.outputEnvelope)
	}
	if metadata, ok := out["tool_metadata"].(map[string]any); !ok || metadata["request_id"] != "request-1" {
		t.Fatalf("tool-specific envelope fields were lost: %#v", out)
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
	out, err := c.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["_ERROR"] != "rate limited" || out["formalized_content"] != "" {
		t.Fatalf("error outputs = %#v", out)
	}
	if results, ok := out["json"].([]any); !ok || len(results) != 0 {
		t.Fatalf("error json output = %#v, want an empty array", out["json"])
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
	empty, err := component.Invoke(t.Context(), nil, map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke(empty query): %v", err)
	}
	if serverCalls != 0 || len(empty["json"].([]any)) != 0 || empty["formalized_content"] != "" {
		t.Fatalf("empty query result = %#v, server calls = %d", empty, serverCalls)
	}
	out, err := component.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{"query": "ragflow"})
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

func TestToolBackedComponentAnySearchPartialFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), `"tag":"x.y"`) {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"code":-1,"message":"Invalid tag: x.y.","request_id":"rid-bad"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"code":0,"request_id":"rid-ok","data":{"results":[{"title":"RAGFlow","url":"https://ragflow.io","content":"RAG engine"}]}}`))
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
	anySearch := agenttool.NewAnySearchToolWith(helper, "")
	component := &ToolBackedComponent{name: "AnySearchSearch", tool: anySearch, spec: anySearch.ComponentSpec()}

	state := runtime.NewCanvasState("run-anysearch", "task-anysearch")
	out, err := component.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{
		"queries": []any{map[string]any{"query": "good"}, map[string]any{"query": "bad", "tag": "x.y"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if failure, exists := out["_ERROR"]; exists {
		t.Fatalf("partial failure set _ERROR %q, which would drop the successful query's references", failure)
	}
	records, _ := out["json"].([]any)
	if len(records) != 2 {
		t.Fatalf("json output = %#v, want both per-query records", out["json"])
	}
	if failed, _ := records[1].(map[string]any); !strings.Contains(failed["_ERROR"].(string), "Invalid tag: x.y.") || failed["request_id"] != "rid-bad" {
		t.Fatalf("failed record = %#v, want its own vendor error and request id", records[1])
	}
	rendered, _ := out["formalized_content"].(string)
	if !strings.Contains(rendered, "RAG engine") || !strings.Contains(rendered, "Error: anysearch: HTTP 400") {
		t.Fatalf("formalized_content = %q", rendered)
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 || chunks[0]["document_name"] != "RAGFlow" {
		t.Fatalf("recorded references = %#v, want the successful query's hit", chunks)
	}

	allFailed := runtime.NewCanvasState("run-anysearch-failed", "task-anysearch-failed")
	out, err = component.Invoke(runtime.WithState(t.Context(), allFailed), nil, map[string]any{
		"queries": []any{map[string]any{"query": "a", "tag": "x.y"}, map[string]any{"query": "b", "tag": "x.y"}},
	})
	if err != nil {
		t.Fatalf("Invoke(all failed): %v", err)
	}
	if failure, _ := out["_ERROR"].(string); failure == "" {
		t.Fatalf("all queries failed but outputs = %#v", out)
	}
	if len(allFailed.GetRetrievalChunks()) != 0 {
		t.Fatalf("error path recorded references: %#v", allFailed.GetRetrievalChunks())
	}
}

// TestToolBackedComponentAnySearchRealCanvasRun compiles and runs a real
// Begin -> AnySearchSearch workflow. It proves that a standalone tool node
// receives the workflow query through the canvas runtime, and that its results
// and references land in the run state.
func TestToolBackedComponentAnySearchRealCanvasRun(t *testing.T) {
	var requests atomic.Int32
	var gotQuery atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(request.Body)
		var sent struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &sent)
		gotQuery.Store(sent.Query)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"request_id":"rid-canvas","data":{"results":[{"title":"RAGFlow","url":"https://ragflow.io","content":"RAG engine"}]}}`))
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
	anySearch := agenttool.NewAnySearchToolWith(helper, "")
	factory := func(name string, params map[string]any) (runtime.Component, error) {
		if name == "AnySearchSearch" {
			return &ToolBackedComponent{name: name, tool: anySearch, spec: anySearch.ComponentSpec()}, nil
		}
		return New(name, params)
	}
	c := &canvas.Canvas{
		Components: map[string]canvas.CanvasComponent{
			"begin_0": {
				Obj:        canvas.CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"search_0"},
			},
			"search_0": {
				Obj: canvas.CanvasComponentObj{ComponentName: "AnySearchSearch", Params: map[string]any{
					"query":   "{sys.query}",
					"outputs": map[string]any{"formalized_content": map[string]any{}, "json": map[string]any{}},
				}},
				Upstream: []string{"begin_0"},
			},
		},
		Path: []string{"begin_0", "search_0"},
	}

	state := runtime.NewCanvasState("run-anysearch-canvas", "task-anysearch-canvas")
	ctx := runtime.WithState(canvas.WithComponentFactory(t.Context(), factory), state)
	compiled, err := canvas.Compile(ctx, c)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err = compiled.Workflow.Invoke(ctx, map[string]any{"query": "canvas nonce query"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if n := requests.Load(); n != 1 {
		t.Fatalf("%d upstream requests, want exactly 1", n)
	}
	if query, _ := gotQuery.Load().(string); query != "canvas nonce query" {
		t.Fatalf("the tool node searched %q, want the workflow query", query)
	}
	records, err := state.GetVar("search_0@json")
	if err != nil {
		t.Fatalf("search_0@json: %v", err)
	}
	if list, ok := records.([]any); !ok || len(list) != 1 {
		t.Fatalf("search_0@json = %#v, want one per-query record", records)
	}
	if chunks := state.GetRetrievalChunks(); len(chunks) != 1 || chunks[0]["document_name"] != "RAGFlow" {
		t.Fatalf("recorded references = %#v", chunks)
	}
}

// TestToolBackedComponentAnySearchKeepsEveryReferenceOfALargeBatch guards the
// run state's per-id dedupe: 5 queries of 10 distinct results must all survive.
func TestToolBackedComponentAnySearchKeepsEveryReferenceOfALargeBatch(t *testing.T) {
	var records []any
	for q := 0; q < 5; q++ {
		var results []any
		for r := 0; r < 10; r++ {
			results = append(results, map[string]any{
				"title":   fmt.Sprintf("Result %d-%d", q, r),
				"url":     fmt.Sprintf("https://example.com/%d/%d", q, r),
				"content": fmt.Sprintf("distinct page text number %d-%d", q, r),
			})
		}
		records = append(records, map[string]any{"index": float64(q), "query": fmt.Sprintf("q%d", q), "results": results})
	}
	envelope := map[string]any{"queries": records}
	anySearch := agenttool.NewAnySearchTool()

	chunks, docAggs := anySearch.BuildReferences(t.Context(), envelope)
	if len(chunks) != 50 || len(docAggs) != 50 {
		t.Fatalf("built %d chunks / %d docs, want 50 / 50", len(chunks), len(docAggs))
	}
	state := runtime.NewCanvasState("run-anysearch-ids", "task-anysearch-ids")
	state.SetRetrievalReferences(chunks, docAggs)
	if got := len(state.GetRetrievalChunks()); got != 50 {
		t.Fatalf("the run state kept %d of 50 references: ids collide and the state dedupes on id", got)
	}
	ids := map[string]bool{}
	for _, chunk := range chunks {
		id, _ := chunk["id"].(string)
		if id == "" || ids[id] {
			t.Fatalf("chunk id %q is empty or repeated", id)
		}
		ids[id] = true
	}
}

// TestToolBackedComponentAnySearchKeepsReferencesWhoseOldHashesCollided uses two
// results that an 8-digit hash bucket maps to the same id (44273655): the run
// state dedupes on id, so with that bucket the second reference was dropped.
func TestToolBackedComponentAnySearchKeepsReferencesWhoseOldHashesCollided(t *testing.T) {
	envelope := map[string]any{"queries": []any{map[string]any{"index": float64(0), "query": "q", "results": []any{
		map[string]any{"title": "First", "url": "https://example.com/page", "content": "collision probe 9995"},
		map[string]any{"title": "Second", "url": "https://example.com/page", "content": "collision probe 27050"},
	}}}}

	chunks, docAggs := agenttool.NewAnySearchTool().BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 || len(docAggs) != 2 {
		t.Fatalf("built %d chunks / %d docs, want 2 / 2", len(chunks), len(docAggs))
	}
	if chunks[0]["id"] == chunks[1]["id"] {
		t.Fatalf("distinct results share the id %v", chunks[0]["id"])
	}
	state := runtime.NewCanvasState("run-anysearch-collision", "task-anysearch-collision")
	state.SetRetrievalReferences(chunks, docAggs)
	if got := len(state.GetRetrievalChunks()); got != 2 {
		t.Fatalf("the run state kept %d of 2 references", got)
	}
}

// anySearchExtractFixture is a fake AnySearch /v1/extract endpoint that records
// how often it was called and which url it was asked for.
type anySearchExtractFixture struct {
	requests atomic.Int32
	lastURL  atomic.Value
	helper   *agenttool.HTTPHelper
}

func newAnySearchExtractFixture(t *testing.T) *anySearchExtractFixture {
	t.Helper()
	fixture := &anySearchExtractFixture{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.requests.Add(1)
		body, _ := io.ReadAll(request.Body)
		var sent struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(body, &sent)
		fixture.lastURL.Store(sent.URL)
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(sent.URL, "/bad"):
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"code":-1,"message":"Unable to extract content from the URL.","request_id":"rid-422","error_code":"extract_failed"}`))
		case strings.HasSuffix(sent.URL, "/long"):
			page, _ := json.Marshal(map[string]any{"code": 0, "request_id": "rid-long", "data": map[string]any{
				"url": sent.URL, "title": "Long page", "content": "partial text\n\n...(content truncated)",
			}})
			_, _ = writer.Write(page)
		default:
			page, _ := json.Marshal(map[string]any{"code": 0, "request_id": "rid-ok", "data": map[string]any{
				"url": sent.URL, "title": "Example Domain", "content": "This domain is for use in documentation examples.",
			}})
			_, _ = writer.Write(page)
		}
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	fixture.helper = agenttool.NewHTTPHelper().WithClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		cloned := request.Clone(request.Context())
		cloned.URL.Scheme = target.Scheme
		cloned.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(cloned)
	})})
	return fixture
}

func (f *anySearchExtractFixture) sentURL() string {
	sent, _ := f.lastURL.Load().(string)
	return sent
}

func TestToolBackedComponentAnySearchExtractPartialAndErrors(t *testing.T) {
	fixture := newAnySearchExtractFixture(t)
	extract := agenttool.NewAnySearchExtractToolWith(fixture.helper, "")
	component := &ToolBackedComponent{name: "AnySearchExtract", tool: extract, spec: extract.ComponentSpec()}
	invoke := func(pageURL string) (map[string]any, *runtime.CanvasState) {
		t.Helper()
		state := runtime.NewCanvasState("run-extract", "task-extract")
		out, err := component.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{"url": pageURL})
		if err != nil {
			t.Fatalf("Invoke(%s): %v", pageURL, err)
		}
		return out, state
	}

	out, state := invoke("https://example.com/")
	if failure, exists := out["_ERROR"]; exists {
		t.Fatalf("success set _ERROR %q", failure)
	}
	page, _ := out["json"].(map[string]any)
	if page["title"] != "Example Domain" || page["truncated"] != false || page["request_id"] != "rid-ok" {
		t.Fatalf("json = %#v, want the page object", out["json"])
	}
	if rendered, _ := out["formalized_content"].(string); !strings.Contains(rendered, "Title: Example Domain") || !strings.Contains(rendered, "documentation examples") {
		t.Fatalf("formalized_content = %q", out["formalized_content"])
	}
	if chunks := state.GetRetrievalChunks(); len(chunks) != 1 || chunks[0]["document_name"] != "Example Domain" || chunks[0]["url"] != "https://example.com/" {
		t.Fatalf("recorded references = %#v", chunks)
	}

	out, state = invoke("https://example.com/long")
	page, _ = out["json"].(map[string]any)
	if _, exists := out["_ERROR"]; exists || page["truncated"] != true {
		t.Fatalf("truncated page outputs = %#v, want a normal result flagged truncated", out)
	}
	if len(state.GetRetrievalChunks()) != 1 {
		t.Fatalf("a truncated page is still a usable reference: %#v", state.GetRetrievalChunks())
	}

	out, state = invoke("https://example.com/bad")
	if failure, _ := out["_ERROR"].(string); !strings.Contains(failure, "HTTP 422") {
		t.Fatalf("outputs = %#v, want the vendor HTTP 422 error", out)
	}
	page, _ = out["json"].(map[string]any)
	if out["formalized_content"] != "" || page == nil || page["content"] != "" || page["request_id"] != "rid-422" {
		t.Fatalf("failure outputs = %#v, want no rendered content and an empty page object", out)
	}
	if len(state.GetRetrievalChunks()) != 0 {
		t.Fatalf("error path recorded references: %#v", state.GetRetrievalChunks())
	}

	before := fixture.requests.Load()
	out, state = invoke("http://127.0.0.1:8080/")
	if failure, _ := out["_ERROR"].(string); !strings.Contains(failure, "not a public") || fixture.requests.Load() != before {
		t.Fatalf("outputs = %#v after %d new requests, want a client-side rejection with no request", out, fixture.requests.Load()-before)
	}
	if len(state.GetRetrievalChunks()) != 0 {
		t.Fatalf("a rejected url recorded references: %#v", state.GetRetrievalChunks())
	}
}

// TestToolBackedComponentAnySearchExtractRealCanvasRun compiles and runs a real
// Begin -> AnySearchExtract workflow. It proves that the url reaches a
// standalone tool node, both when it is a direct workflow input and when it is
// a field declared on the Begin node, and that the page and its reference land
// in the run state.
func TestToolBackedComponentAnySearchExtractRealCanvasRun(t *testing.T) {
	cases := []struct {
		name        string
		beginParams map[string]any
		input       map[string]any
		wantURL     string
	}{
		{
			name:        "direct workflow input",
			beginParams: map[string]any{},
			input:       map[string]any{"query": "unrelated chat text", "url": "https://example.com/direct"},
			wantURL:     "https://example.com/direct",
		},
		{
			name:        "field declared on the Begin node",
			beginParams: map[string]any{"inputs": map[string]any{"url": map[string]any{"name": "URL", "type": "line"}}},
			input:       map[string]any{"query": map[string]any{"url": "https://example.com/from-begin"}},
			wantURL:     "https://example.com/from-begin",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAnySearchExtractFixture(t)
			extract := agenttool.NewAnySearchExtractToolWith(fixture.helper, "")
			factory := func(name string, params map[string]any) (runtime.Component, error) {
				if name == "AnySearchExtract" {
					return &ToolBackedComponent{name: name, tool: extract, spec: extract.ComponentSpec()}, nil
				}
				return New(name, params)
			}
			c := &canvas.Canvas{
				Components: map[string]canvas.CanvasComponent{
					"begin_0": {
						Obj:        canvas.CanvasComponentObj{ComponentName: "Begin", Params: tc.beginParams},
						Downstream: []string{"extract_0"},
					},
					"extract_0": {
						Obj: canvas.CanvasComponentObj{ComponentName: "AnySearchExtract", Params: map[string]any{
							"api_key": "",
							"url":     "",
							"outputs": map[string]any{"formalized_content": map[string]any{}, "json": map[string]any{}},
						}},
						Upstream: []string{"begin_0"},
					},
				},
				Path: []string{"begin_0", "extract_0"},
			}

			state := runtime.NewCanvasState("run-extract-canvas", "task-extract-canvas")
			ctx := runtime.WithState(canvas.WithComponentFactory(t.Context(), factory), state)
			compiled, err := canvas.Compile(ctx, c)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if _, err = compiled.Workflow.Invoke(ctx, tc.input); err != nil {
				t.Fatalf("Invoke: %v", err)
			}

			if n := fixture.requests.Load(); n != 1 {
				t.Fatalf("%d upstream requests, want exactly 1", n)
			}
			if sent := fixture.sentURL(); sent != tc.wantURL {
				t.Fatalf("the tool node extracted %q, want the workflow url %q", sent, tc.wantURL)
			}
			page, err := state.GetVar("extract_0@json")
			if err != nil {
				t.Fatalf("extract_0@json: %v", err)
			}
			if object, ok := page.(map[string]any); !ok || object["title"] != "Example Domain" || object["url"] != tc.wantURL {
				t.Fatalf("extract_0@json = %#v, want the extracted page", page)
			}
			rendered, err := state.GetVar("extract_0@formalized_content")
			if err != nil || !strings.Contains(rendered.(string), "URL: "+tc.wantURL) {
				t.Fatalf("extract_0@formalized_content = %#v, %v", rendered, err)
			}
			if chunks := state.GetRetrievalChunks(); len(chunks) != 1 || chunks[0]["document_name"] != "Example Domain" {
				t.Fatalf("recorded references = %#v", chunks)
			}
		})
	}
}

func TestToolBackedComponentTavilyIntegration(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverCalls++
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["include_raw_content"] != true {
			t.Errorf("include_raw_content = %v, want true", body["include_raw_content"])
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[{"title":"RAGFlow","url":"https://ragflow.io","raw_content":"RAG article","content":"fallback","score":0.8,"custom":"preserved"}]}`))
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
	tavily := agenttool.NewTavilyToolWith(helper)
	component := &ToolBackedComponent{name: "TavilySearch", tool: tavily, spec: tavily.ComponentSpec()}
	state := runtime.NewCanvasState("run-tavily", "task-tavily")
	empty, err := component.Invoke(t.Context(), nil, map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke(empty query): %v", err)
	}
	if serverCalls != 0 || len(empty["json"].([]any)) != 0 || empty["formalized_content"] != "" {
		t.Fatalf("empty query result = %#v, server calls = %d", empty, serverCalls)
	}
	out, err := component.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{"query": "ragflow", "api_key": "key", "include_raw_content": true})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	rendered, _ := out["formalized_content"].(string)
	if !strings.Contains(rendered, "Title: RAGFlow") || !strings.Contains(rendered, "RAG article") {
		t.Fatalf("formalized_content = %q", rendered)
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 1 || results[0].(map[string]any)["custom"] != "preserved" || results[0].(map[string]any)["raw_content"] != "RAG article" {
		t.Fatalf("raw json results = %#v", out["json"])
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 || chunks[0]["document_name"] != "RAGFlow" || chunks[0]["similarity"] != float64(0.8) {
		t.Fatalf("recorded references = %#v", chunks)
	}
}

func TestToolBackedComponentQueritIntegration(t *testing.T) {
	var serverCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":{"result":[{"title":"RAGFlow","url":"https://ragflow.io","snippet":"RAG article","custom":"preserved"}]},"search_id":11099848653006015581,"query_context":{"rewritten":"rag flow"}}`))
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
	querit := agenttool.NewQueritToolWithEnvKey(helper, func() string { return "key" })
	component := &ToolBackedComponent{name: "QueritSearch", tool: querit, spec: querit.ComponentSpec()}
	state := runtime.NewCanvasState("run-querit", "task-querit")
	empty, err := component.Invoke(t.Context(), nil, map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke(empty query): %v", err)
	}
	emptyJSON, emptyJSONOK := empty["json"].(map[string]any)
	if serverCalls.Load() != 0 || empty["formalized_content"] != "" || !emptyJSONOK || len(emptyJSON) != 0 {
		t.Fatalf("empty query result = %#v, server calls = %d", empty, serverCalls.Load())
	}
	out, err := component.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	rendered, _ := out["formalized_content"].(string)
	if !strings.Contains(rendered, "Title: RAGFlow") || !strings.Contains(rendered, "RAG article") {
		t.Fatalf("formalized_content = %q", rendered)
	}
	complete, ok := out["json"].(map[string]any)
	searchID, searchIDOK := complete["search_id"].(json.Number)
	if !ok || !searchIDOK || searchID.String() != "11099848653006015581" || complete["query_context"] == nil {
		t.Fatalf("complete json = %#v", out["json"])
	}
	encoded, err := json.Marshal(complete)
	if err != nil || !strings.Contains(string(encoded), `"search_id":11099848653006015581`) {
		t.Fatalf("re-encoded complete json = %s, %v", encoded, err)
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 || chunks[0]["document_name"] != "RAGFlow" || chunks[0]["similarity"] != 1 {
		t.Fatalf("recorded references = %#v", chunks)
	}
}

func TestParseToolEnvelopeLosslessRejectsTrailingContent(t *testing.T) {
	out := parseToolEnvelopeLossless(`{"results":{"result":[]}} trailing`)
	if out["_raw"] == nil {
		t.Fatalf("trailing content was accepted: %#v", out)
	}
}

func TestToolBackedComponentYahooFinanceIntegration(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverCalls++
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/finance/search":
			if q := request.URL.Query().Get("q"); q != "AAPL" {
				t.Errorf("q = %q", q)
			}
			_, _ = writer.Write([]byte(`{"quotes":[{"symbol":"AAPL","currency":"USD"}],"news":[]}`))
		case "/v8/finance/chart/AAPL":
			_, _ = writer.Write([]byte(`{
				"chart": {
					"result": [{
						"meta": {"regularMarketPrice": 189.5},
						"timestamp": [],
						"indicators": {"quote": [{}]}
					}],
					"error": null
				}
			}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
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
	yahoo := agenttool.NewYahooFinanceToolWith(helper)
	component := &ToolBackedComponent{name: "YahooFinance", tool: yahoo, spec: yahoo.ComponentSpec()}

	empty, err := component.Invoke(t.Context(), nil, map[string]any{"stock_code": ""})
	if err != nil {
		t.Fatalf("Invoke(empty stock_code): %v", err)
	}
	if serverCalls != 0 || empty["report"] != "" {
		t.Fatalf("empty result = %#v, server calls = %d", empty, serverCalls)
	}

	out, err := component.Invoke(t.Context(), nil, map[string]any{"stock_code": "AAPL"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	report, ok := out["report"].(string)
	if !ok || !strings.Contains(report, "# Information:") || !strings.Contains(report, "| symbol | AAPL |") {
		t.Fatalf("report = %#v", out["report"])
	}
	if serverCalls != 2 {
		t.Fatalf("server calls = %d, want 2", serverCalls)
	}
}
