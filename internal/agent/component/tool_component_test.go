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
	out, err := c.Invoke(context.Background(), map[string]any{
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
	for _, componentName := range []string{"ArXiv", "BGPT", "DuckDuckGo", "Email", "Google", "GoogleScholar", "KeenableSearch", "PubMed", "SearXNG", "WenCai", "TavilyExtract", "TavilySearch", "Wikipedia", "YahooFinance"} {
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
			if _, err := canvas.BuildWorkflow(context.Background(), c); err != nil {
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
	if _, err := canvas.BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow with GitHub: %v", err)
	}
}

func TestToolBackedComponentInvokeOrdersReferencesBeforeRendering(t *testing.T) {
	fake := &fakeToolAdapter{out: `{"results":[{"title":"RAGFlow","content":"RAG engine"}],"tool_metadata":{"request_id":"request-1"}}`}
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
	out, err := c.Invoke(runtime.WithState(context.Background(), state), map[string]any{"query": "ragflow"})
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

func TestToolBackedComponentTavilyIntegration(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverCalls++
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
	empty, err := component.Invoke(context.Background(), map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke(empty query): %v", err)
	}
	if serverCalls != 0 || len(empty["json"].([]any)) != 0 || empty["formalized_content"] != "" {
		t.Fatalf("empty query result = %#v, server calls = %d", empty, serverCalls)
	}
	out, err := component.Invoke(runtime.WithState(context.Background(), state), map[string]any{"query": "ragflow", "api_key": "key"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	rendered, _ := out["formalized_content"].(string)
	if !strings.Contains(rendered, "Title: RAGFlow") || !strings.Contains(rendered, "RAG article") {
		t.Fatalf("formalized_content = %q", rendered)
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 1 || results[0].(map[string]any)["custom"] != "preserved" {
		t.Fatalf("raw json results = %#v", out["json"])
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 || chunks[0]["document_name"] != "RAGFlow" || chunks[0]["similarity"] != float64(0.8) {
		t.Fatalf("recorded references = %#v", chunks)
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

	empty, err := component.Invoke(context.Background(), map[string]any{"stock_code": ""})
	if err != nil {
		t.Fatalf("Invoke(empty stock_code): %v", err)
	}
	if serverCalls != 0 || empty["report"] != "" {
		t.Fatalf("empty result = %#v, server calls = %d", empty, serverCalls)
	}

	out, err := component.Invoke(context.Background(), map[string]any{"stock_code": "AAPL"})
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
