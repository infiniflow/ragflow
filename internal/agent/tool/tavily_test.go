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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTavily_BuildRequest(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth, gotCT, gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	// Point the hard-coded tavily endpoint at the test server via a
	// transport that rewrites the host. Avoids the global-package-var
	// race that breaks parallel tests.
	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTavilyToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","api_key":"key-xyz","max_results":3,"search_depth":"advanced","include_raw_content":true,"include_images":true}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if out == "" {
		t.Fatal("InvokableRun returned empty string")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "search") {
		t.Errorf("path = %q, want .../search", gotPath)
	}
	if gotAuth != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer key-xyz")
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotBody["query"] != "ragflow" {
		t.Errorf("body.query = %v, want ragflow", gotBody["query"])
	}
	if n, ok := gotBody["max_results"].(float64); !ok || int(n) != 3 {
		t.Errorf("body.max_results = %v, want 3", gotBody["max_results"])
	}
	if gotBody["search_depth"] != "advanced" {
		t.Errorf("body.search_depth = %v, want advanced", gotBody["search_depth"])
	}
	// include_raw_content and include_images are always forced to false
	// to match the Python implementation (avoids bloating the context
	// with base64 image data).
	if gotBody["include_raw_content"] != false || gotBody["include_images"] != false {
		t.Errorf("include flags = raw:%v images:%v, want false/false (forced)", gotBody["include_raw_content"], gotBody["include_images"])
	}
}

func TestTavily_ParseResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"url":"https://a.example/","title":"A","content":"alpha"},
				{"url":"https://b.example/","title":"B","content":"beta"}
			],
			"answer": "ignored"
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTavilyToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"x","api_key":"k"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env tavilyEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0]["url"] != "https://a.example/" || env.Results[0]["title"] != "A" {
		t.Errorf("Results[0] = %+v, want url=https://a.example/ title=A", env.Results[0])
	}
	if env.Results[1]["content"] != "beta" {
		t.Errorf("Results[1].content = %q, want beta", env.Results[1]["content"])
	}
}

func TestTavily_RequiresAPIKey(t *testing.T) {
	t.Parallel()

	// envKey always returns "" so we know the failure is from the
	// missing api_key, not from a stray process env var.
	tool := NewTavilyToolWithEnvKey(NewHTTPHelper(), func() string { return "" })
	out, err := tool.InvokableRun(context.Background(), `{"query":"x"}`)
	if err != nil {
		t.Fatalf("InvokableRun should not return a Go error for missing api_key: %v", err)
	}
	if !strings.Contains(out, "api_key") {
		t.Errorf("output = %q, want to mention api_key", out)
	}
}

func TestTavily_APIKeyFromEnv(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTavilyToolWithEnvKey(helper, func() string { return "from-env" })
	if _, err := tool.InvokableRun(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotAuth != "Bearer from-env" {
		t.Errorf("Authorization = %q, want Bearer from-env", gotAuth)
	}
}

func TestTavily_Info(t *testing.T) {
	t.Parallel()

	tool := NewTavilyTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "tavily_search" {
		t.Errorf("Name = %q, want tavily_search", info.Name)
	}
	if !strings.Contains(info.Desc, "Tavily") {
		t.Errorf("Desc = %q, want to mention Tavily", info.Desc)
	}
}

func TestTavily_EmptyQueryReturnsEmptyResults(t *testing.T) {
	t.Parallel()

	tavily := NewTavilyToolWithEnvKey(NewHTTPHelper(), func() string { return "" })
	out, err := tavily.InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun(empty query): %v", err)
	}
	var envelope tavilyEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode empty result: %v", err)
	}
	if len(envelope.Results) != 0 || envelope.Error != "" {
		t.Fatalf("empty query result = %#v", envelope)
	}
}

func TestTavily_PreservesRawResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"url":"https://a.example","title":"A","content":"alpha","score":0.9,"custom":{"request_id":"kept"}}]}`))
	}))
	defer srv.Close()
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	out, err := NewTavilyToolWith(helper).InvokableRun(context.Background(), `{"query":"x","api_key":"k"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var envelope tavilyEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	custom, ok := envelope.Results[0]["custom"].(map[string]any)
	if !ok || custom["request_id"] != "kept" {
		t.Fatalf("raw result fields were lost: %#v", envelope.Results[0])
	}
}

func TestTavily_BuildByNameUsesNodeDefaults(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("tavily", map[string]any{
		"api_key":                    "stored-key",
		"search_depth":               "advanced",
		"max_results":                float64(12),
		"days":                       float64(7),
		"include_answer":             true,
		"include_raw_content":        true,
		"include_images":             true,
		"include_image_descriptions": true,
		"query":                      "ignored runtime input",
		"outputs":                    map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tavily, ok := built.(*TavilyTool)
	if !ok {
		t.Fatalf("built type = %T, want *TavilyTool", built)
	}
	if tavily.defaults.APIKey != "stored-key" || tavily.defaults.SearchDepth != "advanced" || tavily.defaults.MaxResults != 12 || tavily.defaults.Days != 7 {
		t.Fatalf("defaults = %+v", tavily.defaults)
	}
	if !tavily.defaults.IncludeAnswer || !tavily.defaults.IncludeRawContent || !tavily.defaults.IncludeImages || !tavily.defaults.IncludeImageDescriptions {
		t.Fatalf("boolean defaults = %+v", tavily.defaults)
	}
}

func TestTavily_ExplicitFlagsOverrideNodeDefaults(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	tavily := newTavilyTool(helper, func() string { return "" }, tavilyParams{
		APIKey: "stored-key", IncludeAnswer: true,
	})
	if _, err := tavily.InvokableRun(context.Background(), `{"query":"ragflow"}`); err != nil {
		t.Fatalf("InvokableRun(node defaults): %v", err)
	}
	if gotBody["include_answer"] != true {
		t.Fatalf("node default include_answer was not sent: %#v", gotBody)
	}
	if gotBody["include_raw_content"] != false || gotBody["include_images"] != false {
		t.Fatalf("include_raw_content/images should be forced false: %#v", gotBody)
	}
	if _, err := tavily.InvokableRun(context.Background(), `{"query":"ragflow","include_answer":false}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotBody["include_answer"] != false {
		t.Fatalf("explicit false include_answer was not preserved: %#v", gotBody)
	}
}

func TestTavily_BuildByNameRejectsInvalidNodeDefaults(t *testing.T) {
	t.Parallel()

	cases := []map[string]any{
		{"api_key": 1},
		{"search_depth": "deep"},
		{"max_results": 0},
		{"max_results": 21},
		{"max_results": 1.5},
		{"days": 0},
		{"include_answer": "yes"},
	}
	for _, params := range cases {
		if _, err := BuildByName("tavily", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded, want validation error", params)
		}
	}
}

func TestTavily_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()

	tavily := NewTavilyTool()
	spec := tavily.ComponentSpec()
	for key, name := range map[string]string{
		"query": "Query", "topic": "Topic", "include_domains": "Include domains", "exclude_domains": "Exclude domains",
	} {
		field, ok := spec.InputForm[key].(map[string]any)
		if !ok || field["name"] != name || field["type"] != "line" {
			t.Fatalf("%s input form = %#v", key, spec.InputForm[key])
		}
	}
	envelope := map[string]any{"results": []any{map[string]any{
		"title":       "RAGFlow",
		"url":         "https://ragflow.io",
		"raw_content": "raw article",
		"content":     "fallback article",
		"score":       float64(0.75),
		"custom":      "preserved",
	}}}
	chunks, docAggs := tavily.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	if chunks[0]["content"] != "raw article" || chunks[0]["similarity"] != float64(0.75) || docAggs[0]["doc_name"] != "RAGFlow" {
		t.Fatalf("reference metadata = %#v / %#v", chunks[0], docAggs[0])
	}
	outputs := tavily.BuildComponentOutputs(envelope)
	results, ok := outputs["json"].([]any)
	if !ok || len(results) != 1 || results[0].(map[string]any)["custom"] != "preserved" {
		t.Fatalf("json output = %#v", outputs["json"])
	}
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"Title: RAGFlow", "URL: https://ragflow.io", "raw article"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("formalized_content missing %q: %q", want, rendered)
		}
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("output conversion mutated envelope: %#v", envelope)
	}
}

func TestTavilyExtract_BuildRequest(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"url":"https://example.com","raw_content":"hello"}]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	tool := NewTavilyExtractToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"urls":"https://a.example, https://b.example","api_key":"key-xyz","extract_depth":"advanced","format":"text"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if out == "" {
		t.Fatal("InvokableRun returned empty string")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "extract") {
		t.Errorf("path = %q, want .../extract", gotPath)
	}
	if gotAuth != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer key-xyz")
	}
	urls, ok := gotBody["urls"].([]any)
	if !ok || len(urls) != 2 || urls[0] != "https://a.example" || urls[1] != "https://b.example" {
		t.Errorf("body.urls = %#v, want two trimmed URLs", gotBody["urls"])
	}
	if gotBody["extract_depth"] != "advanced" {
		t.Errorf("body.extract_depth = %v, want advanced", gotBody["extract_depth"])
	}
	if gotBody["format"] != "text" {
		t.Errorf("body.format = %v, want text", gotBody["format"])
	}
}

func TestTavilyExtract_ParseResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"url":"https://a.example/","raw_content":"alpha","custom":"preserved"}]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	tool := NewTavilyExtractToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"urls":["https://a.example/"],"api_key":"k"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env tavilyExtractEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(env.Results))
	}
	if env.Results[0]["url"] != "https://a.example/" || env.Results[0]["raw_content"] != "alpha" {
		t.Errorf("Results[0] = %+v, want url and raw_content", env.Results[0])
	}
	if env.Results[0]["custom"] != "preserved" {
		t.Fatalf("raw upstream fields were lost: %#v", env.Results[0])
	}
}

func TestTavilyExtract_RequiresAPIKey(t *testing.T) {
	t.Parallel()

	tool := NewTavilyExtractToolWithEnvKey(NewHTTPHelper(), func() string { return "" })
	out, err := tool.InvokableRun(context.Background(), `{"urls":["https://a.example/"]}`)
	if err != nil {
		t.Fatalf("InvokableRun should not return a Go error for missing api_key: %v", err)
	}
	if !strings.Contains(out, "api_key") {
		t.Errorf("output = %q, want to mention api_key", out)
	}
}

func TestTavilyExtract_Info(t *testing.T) {
	t.Parallel()

	tool := NewTavilyExtractTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "tavily_extract" {
		t.Errorf("Name = %q, want tavily_extract", info.Name)
	}
	if !strings.Contains(info.Desc, "Tavily Extract") {
		t.Errorf("Desc = %q, want to mention Tavily Extract", info.Desc)
	}
}

func TestTavilyExtract_ComponentContract(t *testing.T) {
	t.Parallel()

	tavily := NewTavilyExtractTool()
	spec := tavily.ComponentSpec()
	for _, input := range []string{"urls", "extract_depth", "format"} {
		if _, ok := spec.Inputs[input]; !ok {
			t.Fatalf("component inputs missing %s: %#v", input, spec.Inputs)
		}
	}
	if _, ok := spec.Outputs["json"]; !ok {
		t.Fatalf("component outputs missing json: %#v", spec.Outputs)
	}
	for key, name := range map[string]string{
		"urls": "URLs", "extract_depth": "Extract depth", "format": "Format",
	} {
		field, ok := spec.InputForm[key].(map[string]any)
		if !ok || field["name"] != name || field["type"] != "line" {
			t.Fatalf("%s input form = %#v", key, spec.InputForm[key])
		}
	}

	envelope := map[string]any{
		"results": []any{map[string]any{
			"url":         "https://example.com",
			"raw_content": "content",
			"custom":      "preserved",
		}},
	}
	outputs := tavily.BuildComponentOutputs(envelope)
	results, ok := outputs["json"].([]any)
	if !ok || len(results) != 1 || results[0].(map[string]any)["custom"] != "preserved" {
		t.Fatalf("component outputs = %#v", outputs)
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("component output conversion mutated envelope: %#v", envelope)
	}
}

func TestTavilyExtract_BuildByNameAcceptsNodeDefaults(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("tavily_extract", map[string]any{
		"api_key":       "stored-key",
		"urls":          "https://example.com",
		"extract_depth": "advanced",
		"format":        "text",
		"outputs":       map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tavily, ok := built.(*TavilyExtractTool)
	if !ok {
		t.Fatalf("built type = %T, want *TavilyExtractTool", built)
	}
	if tavily.defaults.APIKey != "stored-key" || tavily.defaults.ExtractDepth != "advanced" || tavily.defaults.Format != "text" {
		t.Fatalf("defaults = %+v", tavily.defaults)
	}
	if tavily.defaults.URLs != "https://example.com" {
		t.Fatalf("defaults.URLs = %#v", tavily.defaults.URLs)
	}
}

func TestTavilyExtract_UsesNodeDefaults(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	tavily := newTavilyExtractTool(helper, func() string { return "" }, tavilyExtractParams{
		APIKey:       "stored-key",
		URLs:         "https://stored.example",
		ExtractDepth: "advanced",
		Format:       "text",
	})
	if _, err := tavily.InvokableRun(context.Background(), `{"unrelated":"ignored"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotAuth != "Bearer stored-key" {
		t.Fatalf("Authorization = %q, want stored node API key", gotAuth)
	}
	urls, ok := gotBody["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://stored.example" {
		t.Fatalf("body.urls = %#v", gotBody["urls"])
	}
	if gotBody["extract_depth"] != "advanced" || gotBody["format"] != "text" {
		t.Fatalf("request body = %#v", gotBody)
	}
}

func TestTavilyExtract_BuildByNameRejectsInvalidNodeDefaults(t *testing.T) {
	t.Parallel()

	cases := []map[string]any{
		{"api_key": 1},
		{"extract_depth": "deep"},
		{"format": "html"},
	}
	for _, params := range cases {
		if _, err := BuildByName("tavily_extract", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded, want validation error", params)
		}
	}
}

func TestTavilyExtract_MergeDefaults(t *testing.T) {
	t.Parallel()

	got := mergeTavilyExtractParams(
		tavilyExtractParams{
			APIKey:       "stored-key",
			URLs:         []string{"https://stored.example"},
			ExtractDepth: "advanced",
			Format:       "text",
		},
		tavilyExtractParams{URLs: []string{"https://runtime.example"}},
	)
	if got.APIKey != "stored-key" || got.ExtractDepth != "advanced" || got.Format != "text" {
		t.Fatalf("merged params = %+v", got)
	}
	urls, ok := got.URLs.([]string)
	if !ok || len(urls) != 1 || urls[0] != "https://runtime.example" {
		t.Fatalf("merged URLs = %#v", got.URLs)
	}
}
