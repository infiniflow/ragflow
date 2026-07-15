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

func TestBGPT_RequestAndRawResults(t *testing.T) {
	t.Parallel()

	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Paper","url":"https://paper.example","methods_and_experimental_techniques":"RCT","quality_score":0.95}]}`))
	}))
	defer srv.Close()
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	bgpt := NewBGPTToolWith(helper)
	out, err := bgpt.InvokableRun(context.Background(), `{"query":"  cancer  ","top_n":3,"api_key":"key","days_back":30}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if body["query"] != "cancer" || body["num_results"] != float64(3) || body["api_key"] != "key" || body["days_back"] != float64(30) {
		t.Fatalf("request body = %#v", body)
	}
	var envelope bgptEnv
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(envelope.Results) != 1 || envelope.Results[0]["quality_score"] != float64(0.95) {
		t.Fatalf("raw upstream result was narrowed: %#v", envelope.Results)
	}
}

func TestBGPT_InfoAndNodeDefaults(t *testing.T) {
	t.Parallel()

	info, err := NewBGPTTool().Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "bgpt_search" {
		t.Fatalf("Info.Name = %q", info.Name)
	}
	built, err := BuildByName("bgpt", map[string]any{
		"api_key":   "stored",
		"top_n":     "7",
		"days_back": float64(14),
		"outputs":   map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	bgpt := built.(*BGPTTool)
	if bgpt.defaults.APIKey != "stored" || bgpt.defaults.TopN != 7 || bgpt.defaults.DaysBack != 14 {
		t.Fatalf("defaults = %+v", bgpt.defaults)
	}
	for _, params := range []map[string]any{{"top_n": 0}, {"days_back": 1.5}, {"api_key": 1}} {
		if _, err := BuildByName("bgpt", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded", params)
		}
	}
}

func TestBGPT_EmptyQuery(t *testing.T) {
	t.Parallel()

	out, err := NewBGPTTool().InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun(empty): %v", err)
	}
	var envelope bgptEnv
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || len(envelope.Results) != 0 {
		t.Fatalf("empty result = %s / %v", out, err)
	}
}

func TestBGPT_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()

	bgpt := NewBGPTTool()
	spec := bgpt.ComponentSpec()
	if query, ok := spec.InputForm["query"].(map[string]any); !ok || query["type"] != "line" {
		t.Fatalf("query input form = %#v", spec.InputForm["query"])
	}
	envelope := map[string]any{"results": []any{map[string]any{
		"title":                               "Paper A",
		"url":                                 "https://paper.example",
		"authors":                             []any{"Lee", "Kim"},
		"methods_and_experimental_techniques": "RCT",
		"sample_size_and_population_characteristics": "120 adults",
		"results_and_conclusions":                    "Improved outcomes",
		"quality_score":                              float64(0.9),
	}}}
	chunks, docAggs := bgpt.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 || chunks[0]["document_name"] != "Paper A" {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	content := chunks[0]["content"].(string)
	for _, want := range []string{"Methods: RCT", "Sample size / population: 120 adults", "Results: Improved outcomes"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q: %q", want, content)
		}
	}
	outputs := bgpt.BuildComponentOutputs(envelope)
	results := outputs["json"].([]any)
	if results[0].(map[string]any)["quality_score"] != float64(0.9) {
		t.Fatalf("raw json was narrowed: %#v", results)
	}
	if !strings.Contains(outputs["formalized_content"].(string), "Title: Paper A") {
		t.Fatalf("formalized_content = %q", outputs["formalized_content"])
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("output conversion mutated envelope: %#v", envelope)
	}
}
