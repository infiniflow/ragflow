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
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

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
	if !strings.Contains(out, `<search_results count="1"`) {
		t.Errorf("expected one rendered hit, got %q", out)
	}
	for _, want := range []string{
		`rank="1"`,
		`url="https://ragflow.io/docs"`,
		`doc_name="RAGFlow docs"`,
		`source="web"`,
		`chunk_id="web-`,
		`<match_snippet>Agentic RAG line one line two</match_snippet>`,
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
		`doc_name="A &amp; B &quot;quoted&quot;"`,
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
	var result struct {
		XMLName xml.Name `xml:"search_results"`
		Warning string   `xml:"tool_error"`
	}
	if err := xml.Unmarshal([]byte(out), &result); err != nil || result.Warning != "provider: status 429" {
		t.Errorf("partial failure must remain inside the search result XML: err=%v result=%+v", err, result)
	}
}

func TestWebSearchTool_ReferencesRetainServedSnippet(t *testing.T) {
	registry := NewEvidenceRegistry()
	registry.Stamp("corpus-chunk")
	ctx := WithEvidenceRegistry(t.Context(), registry)
	search, _ := fixedWebSearch([]WebResult{{
		Title: "  Public  source ", URL: " https://example.com/source ",
		Content: strings.Repeat("证据", webSearchSnippetRunes),
	}}, nil)
	web := NewWebSearchTool(search)
	for range 2 {
		out, err := web.InvokableRun(ctx, `{"queries":["keyword"]}`)
		if err != nil || !strings.Contains(out, `ref="1"`) {
			t.Fatalf("web result must keep its handle after corpus evidence: err=%v out=%.300q", err, out)
		}
	}
	ids := registry.IDs()
	if len(ids) != 2 {
		t.Fatalf("duplicate web results must share a reference: %v", ids)
	}
	result, ok := registry.WebResult(ids[1])
	if !ok || result.Title != "Public source" || result.URL != "https://example.com/source" || utf8.RuneCountInString(result.Content) != webSearchSnippetRunes+3 || !strings.HasSuffix(result.Content, "...") {
		t.Fatalf("reference did not retain the bounded snippet served to the model: %+v, ok=%t", result, ok)
	}
}

func TestWebSearchTool_ZeroHits(t *testing.T) {
	search, _ := fixedWebSearch(nil, nil)
	out, err := NewWebSearchTool(search).InvokableRun(t.Context(), `{"queries":["keyword"]}`)
	if err != nil || !strings.Contains(out, `<search_results count="0"`) || !strings.Contains(out, "0 web hits") {
		t.Fatalf("unexpected empty web result: err=%v out=%q", err, out)
	}
	if strings.Contains(out, "WORD FORM") || strings.Contains(out, "MEANING search") {
		t.Errorf("empty web results must not claim the corpus was searched: %s", out)
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
	if got := strings.Count(out, "<chunk "); got != webSearchMaxResults {
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
// a template's tool list — toolsFor adds it from the run context, so the template
// list stays a description of the corpus toolset alone.
func TestToolsFor_WebSearchIsRuntimeInjected(t *testing.T) {
	tools := toolsFor(context.Background(), Template{ID: "t", Tools: []string{"think", webSearchToolName}},
		"tenant", nil, nil)
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want only think without a configured provider", len(tools))
	}
	search, seen := fixedWebSearch(nil, nil)
	ctx := WithWebSearch(context.Background(), search)
	for _, explicit := range [][]tool.BaseTool{nil, tools} {
		enabled := toolsFor(ctx, Template{ID: "t", Tools: []string{"think"}}, "tenant", nil, explicit)
		if len(enabled) != 2 {
			t.Fatalf("enabled tools = %d, want think and web_search", len(enabled))
		}
		info, err := enabled[1].Info(ctx)
		if err != nil || info.Name != webSearchToolName {
			t.Fatalf("enabled web tool: info=%v err=%v", info, err)
		}
	}
	if len(*seen) != 0 {
		t.Fatalf("tool assembly must not search, got queries %v", *seen)
	}
	// The ADK tool node runs InvokableTool
	// implementations; a tool that is only a BaseTool would be silently
	// unusable there.
	var _ tool.InvokableTool = NewWebSearchTool(nil)
}
