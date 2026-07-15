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
	"net/url"
	"strings"
	"testing"
)

func TestGitHub_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		query       string
		max         int
		wantPerPage string
		wantHost    string
	}{
		{
			name:        "default",
			query:       "ragflow",
			max:         0,
			wantPerPage: "10",
			wantHost:    "api.github.com",
		},
		{
			name:        "preserves configured top n",
			query:       "x",
			max:         99,
			wantPerPage: "99",
			wantHost:    "api.github.com",
		},
		{
			name:        "explicit low",
			query:       "language:go stars:>100",
			max:         3,
			wantPerPage: "3",
			wantHost:    "api.github.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGitHubURL(tc.query, tc.max)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "/search/repositories" {
				t.Errorf("path = %q, want /search/repositories", u.Path)
			}
			q := u.Query()
			if q.Get("q") != tc.query {
				t.Errorf("q = %q, want %q", q.Get("q"), tc.query)
			}
			if q.Get("per_page") != tc.wantPerPage {
				t.Errorf("per_page = %q, want %q", q.Get("per_page"), tc.wantPerPage)
			}
			if q.Get("sort") != "stars" {
				t.Errorf("sort = %q, want stars", q.Get("sort"))
			}
			if q.Get("order") != "desc" {
				t.Errorf("order = %q, want desc", q.Get("order"))
			}
		})
	}
}

func TestGitHub_ParseResponse(t *testing.T) {
	t.Parallel()

	var gotContentType, gotAPIVersion, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAPIVersion = r.Header.Get("X-GitHub-Api-Version")
		gotPerPage = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": [
				{"name":"ragflow","html_url":"https://github.com/infiniflow/ragflow","description":"RAG engine","watchers":12000,"private":false},
				{"name":"bar","html_url":"https://github.com/foo/bar","description":"","watchers":5}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewGitHubToolWithDefaults(helper, githubParams{TopN: 17})
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env githubEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0]["name"] != "ragflow" {
		t.Errorf("Results[0].name = %v, want ragflow", env.Results[0]["name"])
	}
	if env.Results[0]["watchers"] != float64(12000) {
		t.Errorf("Results[0].watchers = %v, want 12000", env.Results[0]["watchers"])
	}
	if env.Results[1]["description"] != "" {
		t.Errorf("Results[1].description = %q, want empty", env.Results[1]["description"])
	}
	if gotContentType != "application/vnd.github+json" {
		t.Errorf("Content-Type = %q, want application/vnd.github+json", gotContentType)
	}
	if gotAPIVersion != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want 2022-11-28", gotAPIVersion)
	}
	if gotPerPage != "17" {
		t.Errorf("per_page = %q, want 17 from the component top_n default", gotPerPage)
	}
}

func TestGitHub_EmptyQueryReturnsEmptyResults(t *testing.T) {
	t.Parallel()

	tool := NewGitHubTool()
	out, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun(empty query): %v", err)
	}
	var envelope githubEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode empty result: %v", err)
	}
	if len(envelope.Results) != 0 || envelope.Error != "" {
		t.Fatalf("empty query result = %#v", envelope)
	}
}

func TestGitHub_BuildByNameUsesPythonNodeParams(t *testing.T) {
	built, err := BuildByName("github", map[string]any{
		"top_n":   float64(17),
		"query":   "runtime query",
		"outputs": map[string]any{"json": map[string]any{}},
		"setups":  map[string]any{"query": "configured query"},
	})
	if err != nil {
		t.Fatalf("BuildByName(github): %v", err)
	}
	github, ok := built.(*GitHubTool)
	if !ok {
		t.Fatalf("BuildByName(github) returned %T, want *GitHubTool", built)
	}
	if github.defaults.TopN != 17 {
		t.Errorf("defaults.TopN = %d, want 17", github.defaults.TopN)
	}
	if _, err := BuildByName("github", map[string]any{"top_n": 100}); err != nil {
		t.Errorf("BuildByName(github) rejected GitHub's maximum top_n: %v", err)
	}
	if _, err := BuildByName("github", map[string]any{"top_n": 0}); err == nil {
		t.Fatal("BuildByName(github) accepted non-positive top_n")
	}
	if _, err := BuildByName("github", map[string]any{"top_n": 1.5}); err == nil {
		t.Fatal("BuildByName(github) accepted fractional top_n")
	}
	if _, err := BuildByName("github", map[string]any{"top_n": "10"}); err == nil {
		t.Fatal("BuildByName(github) accepted string top_n")
	}
	if _, err := BuildByName("github", map[string]any{"top_n": 101}); err == nil {
		t.Fatal("BuildByName(github) accepted top_n above GitHub's per_page limit")
	}
	ignored, err := BuildByName("github", map[string]any{"max_results": 5})
	if err != nil {
		t.Fatalf("BuildByName(github) rejected unrelated Canvas params: %v", err)
	}
	if ignored.(*GitHubTool).defaults.TopN != defaultGitHubTopN {
		t.Fatalf("unrelated params changed top_n: %d", ignored.(*GitHubTool).defaults.TopN)
	}
}

func TestGitHub_ComponentContractMatchesPython(t *testing.T) {
	github := NewGitHubTool()
	spec := github.ComponentSpec()
	if _, ok := spec.Outputs["json"]; !ok {
		t.Fatalf("component outputs missing json: %#v", spec.Outputs)
	}
	if _, ok := spec.Outputs["formalized_content"]; !ok {
		t.Fatalf("component outputs missing formalized_content: %#v", spec.Outputs)
	}
	if query, ok := spec.InputForm["query"].(map[string]any); !ok || query["name"] != "Query" || query["type"] != "line" {
		t.Fatalf("query input form = %#v", spec.InputForm["query"])
	}
}

func TestGitHub_ReferencesAndOutputsPreserveRawResults(t *testing.T) {
	github := NewGitHubTool()
	results := []any{map[string]any{
		"name":        "ragflow",
		"html_url":    "https://github.com/infiniflow/ragflow",
		"description": "RAG engine",
		"watchers":    float64(12000),
		"private":     false,
	}}
	repository := results[0].(map[string]any)
	if repository["private"] != false {
		t.Fatalf("raw repository fields were lost: %#v", repository)
	}
	envelope := map[string]any{"results": results}

	chunks, docAggs := github.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	if chunks[0]["document_name"] != "ragflow" || chunks[0]["similarity"] != 1 {
		t.Fatalf("reference metadata = %#v", chunks[0])
	}
	outputs := github.BuildComponentOutputs(envelope)
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("component output conversion mutated the tool envelope: %#v", envelope)
	}
	if results, ok := outputs["json"].([]any); !ok || len(results) != 1 {
		t.Fatalf("component json output = %#v", outputs["json"])
	}
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"Title: ragflow", "URL: https://github.com/infiniflow/ragflow", "RAG engine\n stars:12000"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered results missing %q: %q", want, rendered)
		}
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

func TestGitHub_Info(t *testing.T) {
	t.Parallel()

	tool := NewGitHubTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "github_search" {
		t.Errorf("Name = %q, want github_search", info.Name)
	}
	if !strings.Contains(info.Desc, "GitHub") {
		t.Errorf("Desc = %q, want to mention GitHub", info.Desc)
	}
}
