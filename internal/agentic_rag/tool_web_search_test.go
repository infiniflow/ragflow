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

package agentic_rag

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// fixedWebSearch returns a provider stub answering every query with the same
// two hits, recording the queries it was handed.
func fixedWebSearch(hits []WebResult, err error) (WebSearchFunc, *[]string) {
	var got []string
	return func(_ context.Context, query string) ([]WebResult, error) {
		got = append(got, query)
		return hits, err
	}, &got
}

func TestWebSearchTool_RendersHits(t *testing.T) {
	search, seen := fixedWebSearch([]WebResult{
		{Title: "RAGFlow docs", URL: "https://ragflow.io/docs", Content: "Agentic RAG  line one\nline two"},
		{Title: "", URL: "https://example.com", Content: "   "},
	}, nil)

	out, err := NewWebSearchTool(search).InvokableRun(context.Background(),
		`{"queries":["ragflow agentic rag"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(*seen) != 1 || (*seen)[0] != "ragflow agentic rag" {
		t.Fatalf("provider queries = %v", *seen)
	}
	// The blank-content hit is dropped: an empty snippet carries nothing.
	if !strings.Contains(out, `<web_results count="1"`) {
		t.Errorf("expected one rendered hit, got %q", out)
	}
	for _, want := range []string{
		`rank="1"`,
		`url="https://ragflow.io/docs"`,
		`title="RAGFlow docs"`,
		"Agentic RAG line one line two",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestWebSearchTool_EscapesHits(t *testing.T) {
	search, _ := fixedWebSearch([]WebResult{
		{Title: `A & B "quoted"`, URL: "https://x.test/?a=1&b=2", Content: "<script>alert(1)</script>"},
	}, nil)

	out, err := NewWebSearchTool(search).InvokableRun(context.Background(), `{"queries":["x"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	for _, want := range []string{
		`title="A &amp; B &quot;quoted&quot;"`,
		`url="https://x.test/?a=1&amp;b=2"`,
		"&lt;script&gt;alert(1)&lt;/script&gt;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing escaped %q: %s", want, out)
		}
	}
}

func TestWebSearchTool_TooManyQueries(t *testing.T) {
	search, seen := fixedWebSearch(nil, nil)
	out, err := NewWebSearchTool(search).InvokableRun(context.Background(),
		`{"queries":["a","b","c","d"]}`)
	if err != nil || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("too many queries must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
	if len(*seen) != 0 {
		t.Errorf("provider must not be called on a rejected call, got %v", *seen)
	}
}

func TestWebSearchTool_EmptyQueries(t *testing.T) {
	out, err := NewWebSearchTool(nil).InvokableRun(context.Background(), `{"queries":["  "]}`)
	if err != nil || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("empty queries must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
}

func TestWebSearchTool_NoProvider(t *testing.T) {
	// A tool built without a provider must surface the reason as a
	// <tool_error> result instead of an empty block the model would read as
	// "the web has nothing".
	out, err := NewWebSearchTool(nil).InvokableRun(context.Background(), `{"queries":["a"]}`)
	if err != nil || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("no provider must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
}

func TestWebSearchTool_PartialFailureKeepsHits(t *testing.T) {
	var got []string
	search := func(_ context.Context, query string) ([]WebResult, error) {
		got = append(got, query)
		if query == "broken" {
			return nil, errors.New("provider: status 429")
		}
		return []WebResult{{Title: "ok", URL: "https://ok.test", Content: "body"}}, nil
	}

	out, err := NewWebSearchTool(search).InvokableRun(context.Background(),
		`{"queries":["broken","fine"]}`)
	if err != nil {
		t.Fatalf("one failing query must not fail the call: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("every query must reach the provider, got %v", got)
	}
	// The failure is visible, not swallowed; the surviving hit is still shipped.
	if !strings.Contains(out, `<tool_error tool="web_search" severity="warn" query="broken">provider: status 429</tool_error>`) {
		t.Errorf("output missing canonical warn tool_error: %s", out)
	}
	if !strings.Contains(out, `url="https://ok.test"`) {
		t.Errorf("output missing surviving hit: %s", out)
	}
}

func TestWebSearchTool_AllQueriesFail(t *testing.T) {
	search, _ := fixedWebSearch(nil, errors.New("provider: status 500"))
	out, err := NewWebSearchTool(search).InvokableRun(context.Background(),
		`{"queries":["a","b"]}`)
	if err != nil || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("all-queries-fail must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
}

func TestWebSearchTool_CapsResults(t *testing.T) {
	hits := make([]WebResult, webSearchMaxResults+3)
	for i := range hits {
		hits[i] = WebResult{Title: "t", URL: "https://x.test", Content: "body"}
	}
	search, _ := fixedWebSearch(hits, nil)

	out, err := NewWebSearchTool(search).InvokableRun(context.Background(), `{"queries":["a"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got := strings.Count(out, "<result "); got != webSearchMaxResults {
		t.Errorf("results = %d, want %d", got, webSearchMaxResults)
	}
}

// TestWebSearchRidesOnTheContext: the provider is carried per run, so the
// explorer and the auditor both see the same capability and a bare context
// yields no tool at all.
func TestWebSearchRidesOnTheContext(t *testing.T) {
	search, _ := fixedWebSearch(nil, nil)
	ctx := WithWebSearch(context.Background(), search)

	got, err := webSearchTools(ctx)[0].Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if got.Name != webSearchToolName {
		t.Errorf("tool = %q, want %q", got.Name, webSearchToolName)
	}
	if webSearchFrom(WithWebSearch(context.Background(), nil)) != nil {
		t.Error("a nil provider must not be carried into the context")
	}
	if tools := webSearchTools(context.Background()); len(tools) != 0 {
		t.Errorf("bare context: tools = %d, want none", len(tools))
	}
}

// TestToolsFor_WebSearchIsRuntimeInjected: web_search is never assembled from
// a template's tool list — Run injects it from the run context, so the template
// list stays a description of the corpus toolset alone.
func TestToolsFor_WebSearchIsRuntimeInjected(t *testing.T) {
	tools := toolsFor(Template{ID: "t", Tools: []string{"think", webSearchToolName}},
		"tenant", nil)
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want only think (web_search is injected by Run)", len(tools))
	}
	// Run appends the tool to the ADK tool node, which runs InvokableTool
	// implementations; a tool that is only a BaseTool would be silently
	// unusable there.
	var _ tool.InvokableTool = NewWebSearchTool(nil)
}
