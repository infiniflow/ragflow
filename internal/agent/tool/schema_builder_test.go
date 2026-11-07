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

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type TestSearchToolParams struct {
	Query     string   `json:"query" doc:"Search query string." required:"true"`
	TopK      int      `json:"top_k" doc:"Number of results to return."`
	Threshold float64  `json:"threshold" doc:"Similarity threshold."`
	Channel   string   `json:"channel" doc:"Search channel." enum:"news,general"`
	Tags      []string `json:"tags" doc:"Filter tags."`
	Ignored   string   `json:"-"`
}

func TestBuildParamsFromStruct(t *testing.T) {
	paramsOneOf := BuildParamsFromStruct(TestSearchToolParams{})
	if paramsOneOf == nil {
		t.Fatal("expected non-nil ParamsOneOf")
	}

	js, err := paramsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("failed to convert ParamsOneOf to JSONSchema: %v", err)
	}

	raw, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("failed to marshal JSONSchema: %v", err)
	}
	schemaStr := string(raw)

	// Verify properties in generated schema
	for _, expected := range []string{`"query"`, `"top_k"`, `"threshold"`, `"channel"`, `"tags"`, `"Search query string."`} {
		if !strings.Contains(schemaStr, expected) {
			t.Errorf("schema missing expected substring %s: %s", expected, schemaStr)
		}
	}

	// Verify ignored property is NOT in schema
	if strings.Contains(schemaStr, `"ignored"`) || strings.Contains(schemaStr, `"Ignored"`) {
		t.Errorf("ignored field found in schema: %s", schemaStr)
	}
}

func TestBuildToolInfo(t *testing.T) {
	info, err := BuildToolInfo("test_tool", "A test tool description", TestSearchToolParams{})
	if err != nil {
		t.Fatalf("BuildToolInfo error: %v", err)
	}
	if info.Name != "test_tool" {
		t.Errorf("info.Name = %q; want %q", info.Name, "test_tool")
	}
	if info.Desc != "A test tool description" {
		t.Errorf("info.Desc = %q; want %q", info.Desc, "A test tool description")
	}
	if info.ParamsOneOf == nil {
		t.Fatal("info.ParamsOneOf is nil")
	}
}

func TestBuildComponentSpec(t *testing.T) {
	spec := BuildComponentSpec(TestSearchToolParams{})
	if spec.Inputs == nil {
		t.Fatal("spec.Inputs is nil")
	}
	if spec.Inputs["query"] != "Search query string." {
		t.Errorf("spec.Inputs['query'] = %q; want %q", spec.Inputs["query"], "Search query string.")
	}
	if spec.Inputs["top_k"] != "Number of results to return." {
		t.Errorf("spec.Inputs['top_k'] = %q; want %q", spec.Inputs["top_k"], "Number of results to return.")
	}
	if _, ok := spec.Inputs["ignored"]; ok {
		t.Error("ignored field should not be in ComponentSpec")
	}
}

type TestCanvasParams struct {
	Query string `json:"query" doc:"Search query."`
	TopN  int    `json:"top_n" schema:"-" doc:"Maximum number of results."`
	Hide  string `json:"hide" component:"-" doc:"Hidden from canvas."`
}

func TestBuildComponentSpec_PreservesSchemaMinus(t *testing.T) {
	spec := BuildComponentSpec(TestCanvasParams{})
	if spec.Inputs["top_n"] != "Maximum number of results." {
		t.Errorf("spec.Inputs['top_n'] = %q; want %q", spec.Inputs["top_n"], "Maximum number of results.")
	}
	if _, ok := spec.Inputs["hide"]; ok {
		t.Error("component:'-' field should not be in ComponentSpec")
	}

	// But in ToolInfo ParamsOneOf, top_n must be excluded due to schema:"-"
	params := BuildParamsFromStruct(TestCanvasParams{})
	js, _ := params.ToJSONSchema()
	raw, _ := json.Marshal(js)
	if strings.Contains(string(raw), "top_n") {
		t.Errorf("schema:'-' field must not be in model tool schema: %s", raw)
	}
}

type TestPointerParams struct {
	Name    *string   `json:"name" doc:"Name pointer." required:"true"`
	Count   *int      `json:"count" doc:"Count pointer."`
	Active  *bool     `json:"active" doc:"Active pointer."`
	Items   []*string `json:"items" doc:"Slice of pointers."`
	Details *struct {
		SubField string `json:"sub_field" doc:"Sub field."`
	} `json:"details" doc:"Nested pointer."`
}

func TestSchemaBuilder_PointerSafety(t *testing.T) {
	// Test 1: nil input
	nilParams := BuildParamsFromStruct(nil)
	if nilParams == nil {
		t.Fatal("expected non-nil ParamsOneOf for nil input")
	}
	nilSpec := BuildComponentSpec(nil)
	if nilSpec.Inputs == nil {
		t.Fatal("expected non-nil ComponentSpec inputs for nil input")
	}

	// Test 2: Typed nil pointer (*TestSearchToolParams)(nil)
	var typedNil *TestSearchToolParams = nil
	typedNilParams := BuildParamsFromStruct(typedNil)
	if typedNilParams == nil {
		t.Fatal("expected non-nil ParamsOneOf for typed nil pointer")
	}
	typedNilSpec := BuildComponentSpec(typedNil)
	if typedNilSpec.Inputs["query"] != "Search query string." {
		t.Errorf("expected query input from typed nil pointer, got %q", typedNilSpec.Inputs["query"])
	}

	// Test 3: Struct containing pointer fields (*string, *int, *bool, []*string)
	ptrParams := BuildParamsFromStruct(TestPointerParams{})
	js, err := ptrParams.ToJSONSchema()
	if err != nil {
		t.Fatalf("failed to convert pointer params to JSONSchema: %v", err)
	}
	raw, _ := json.Marshal(js)
	schemaStr := string(raw)

	for _, expected := range []string{`"name"`, `"count"`, `"active"`, `"items"`} {
		if !strings.Contains(schemaStr, expected) {
			t.Errorf("schema missing expected pointer field %s: %s", expected, schemaStr)
		}
	}
}

func TestDuckDuckGoTool_TopNParity(t *testing.T) {
	tool := NewDuckDuckGoTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("DuckDuckGoTool.Info error: %v", err)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error: %v", err)
	}
	raw, _ := json.Marshal(js)
	schemaStr := string(raw)

	// Tool schema for model must NOT contain top_n
	if strings.Contains(schemaStr, `"top_n"`) {
		t.Errorf("DuckDuckGo model schema should NOT contain top_n: %s", schemaStr)
	}

	// Canvas ComponentSpec MUST contain top_n input
	spec := tool.ComponentSpec()
	if spec.Inputs["top_n"] != "Maximum number of results." {
		t.Errorf("DuckDuckGo Canvas ComponentSpec missing top_n input: %#v", spec.Inputs)
	}
	if spec.Inputs["query"] == "" || spec.Inputs["channel"] == "" {
		t.Errorf("DuckDuckGo Canvas ComponentSpec missing query or channel input: %#v", spec.Inputs)
	}
}

func TestSearch1APITool_ServiceEnumParity(t *testing.T) {
	tool := NewSearch1APITool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Search1APITool.Info error: %v", err)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error: %v", err)
	}
	raw, _ := json.Marshal(js)
	schemaStr := string(raw)

	// Ensure unsupported 'sogou' is NOT in description or enum
	if strings.Contains(schemaStr, "sogou") {
		t.Errorf("Search1API schema should NOT contain sogou: %s", schemaStr)
	}

	// Ensure supported services are present
	for _, svc := range []string{"google", "bing", "duckduckgo", "github", "arxiv", "hackernews", "reuters"} {
		if !strings.Contains(schemaStr, svc) {
			t.Errorf("Search1API schema missing supported service %s: %s", svc, schemaStr)
		}
	}
}

func TestKeyTools_MetadataAndParity(t *testing.T) {
	ctx := context.Background()

	// 1. Arxiv tool parity
	arxiv := NewArxivTool()
	arxivInfo, err := arxiv.Info(ctx)
	if err != nil {
		t.Fatalf("Arxiv.Info error: %v", err)
	}
	if arxivInfo.Name != "arxiv_search" {
		t.Errorf("expected arxiv_search name, got %s", arxivInfo.Name)
	}
	arxivJS, err := arxivInfo.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("arxiv ToJSONSchema error: %v", err)
	}
	arxivRaw, _ := json.Marshal(arxivJS)
	if !strings.Contains(string(arxivRaw), "query") {
		t.Errorf("expected query param in arxiv schema: %s", arxivRaw)
	}

	// 2. Yahoo Finance tool parity
	yf := NewYahooFinanceTool()
	yfInfo, err := yf.Info(ctx)
	if err != nil {
		t.Fatalf("YahooFinance.Info error: %v", err)
	}
	if yfInfo.Name != "yahoo_finance" {
		t.Errorf("expected yahoo_finance name, got %s", yfInfo.Name)
	}
	yfJS, err := yfInfo.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("yf ToJSONSchema error: %v", err)
	}
	yfRaw, _ := json.Marshal(yfJS)
	if !strings.Contains(string(yfRaw), "stock_code") {
		t.Errorf("expected stock_code param in yf schema: %s", yfRaw)
	}

	// 3. Tavily tool parity
	tavily := NewTavilyTool()
	tavilyInfo, err := tavily.Info(ctx)
	if err != nil {
		t.Fatalf("Tavily.Info error: %v", err)
	}
	if tavilyInfo.Name != "tavily_search" {
		t.Errorf("expected tavily_search name, got %s", tavilyInfo.Name)
	}

	// 4. Google tool parity
	google := NewGoogleTool()
	googleInfo, err := google.Info(ctx)
	if err != nil {
		t.Fatalf("Google.Info error: %v", err)
	}
	if googleInfo.Name != "google_search" {
		t.Errorf("expected google_search name, got %s", googleInfo.Name)
	}
}

func TestBuildComponentSpec_NoLeakageOfBlankNodeParams(t *testing.T) {
	// arXiv should only expose query in inputs, not top_n or sort_by
	arxiv := NewArxivTool()
	arxivSpec := arxiv.ComponentSpec()
	if _, ok := arxivSpec.Inputs["top_n"]; ok {
		t.Errorf("arxiv ComponentSpec.Inputs must not contain top_n: %#v", arxivSpec.Inputs)
	}
	if _, ok := arxivSpec.Inputs["sort_by"]; ok {
		t.Errorf("arxiv ComponentSpec.Inputs must not contain sort_by: %#v", arxivSpec.Inputs)
	}

	// PubMed should only expose query in inputs, not top_n or email
	pubmed := NewPubMedTool()
	pubmedSpec := pubmed.ComponentSpec()
	if _, ok := pubmedSpec.Inputs["top_n"]; ok {
		t.Errorf("pubmed ComponentSpec.Inputs must not contain top_n: %#v", pubmedSpec.Inputs)
	}
	if _, ok := pubmedSpec.Inputs["email"]; ok {
		t.Errorf("pubmed ComponentSpec.Inputs must not contain email: %#v", pubmedSpec.Inputs)
	}

	// Wikipedia should only expose query in inputs, not lang or max_results
	wiki := NewWikipediaTool()
	wikiSpec := wiki.ComponentSpec()
	if _, ok := wikiSpec.Inputs["lang"]; ok {
		t.Errorf("wikipedia ComponentSpec.Inputs must not contain lang: %#v", wikiSpec.Inputs)
	}
	if _, ok := wikiSpec.Inputs["max_results"]; ok {
		t.Errorf("wikipedia ComponentSpec.Inputs must not contain max_results: %#v", wikiSpec.Inputs)
	}

	// DuckDuckGo SHOULD expose top_n because it has documentation and is a canvas input
	ddg := NewDuckDuckGoTool()
	ddgSpec := ddg.ComponentSpec()
	if ddgSpec.Inputs["top_n"] != "Maximum number of results." {
		t.Errorf("expected duckduckgo to retain top_n in ComponentSpec.Inputs, got: %#v", ddgSpec.Inputs)
	}
}
