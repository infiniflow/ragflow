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

func TestGoogle_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		params   googleParams
		wantNum  string
		wantHost string
	}{
		{
			name:     "python defaults",
			params:   googleParams{APIKey: "KEY", Q: "ragflow"},
			wantNum:  "6",
			wantHost: "serpapi.com",
		},
		{
			name:     "canvas num country language",
			params:   googleParams{APIKey: "K", Q: "x y", Num: 12, Start: 10, Country: "cn", Language: "zh-cn"},
			wantNum:  "12",
			wantHost: "serpapi.com",
		},
		{
			name:     "agent aliases",
			params:   googleParams{APIKey: "K", Q: "x", Num: 3},
			wantNum:  "3",
			wantHost: "serpapi.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGoogleURL(tc.apiKey, tc.cx, tc.query, tc.max)
			u, _ := url.Parse(got)
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "/search.json" {
				t.Errorf("path = %q, want /search.json", u.Path)
			}
			q := u.Query()
			if q.Get("api_key") != tc.params.APIKey {
				t.Errorf("api_key = %q, want %q", q.Get("api_key"), tc.params.APIKey)
			}
			if q.Get("engine") != "google" {
				t.Errorf("engine = %q, want google", q.Get("engine"))
			}
			if q.Get("num") != tc.wantNum {
				t.Errorf("num = %q, want %q", q.Get("num"), tc.wantNum)
			}
		})
	}
}

func TestGoogle_ParseResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"organic_results": [
				{"title":"RAGFlow","link":"https://ragflow.io","snippet":"Open source RAG engine"},
				{"title":"GitHub","link":"https://github.com/infiniflow/ragflow","snippet":"Source code"}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewGoogleToolWith(helper)
	out, _ := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","api_key":"K","cx":"C","max_results":5}`)

	var env googleEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0]["title"] != "RAGFlow" {
		t.Errorf("Results[0].title = %q, want RAGFlow", env.Results[0]["title"])
	}
	if env.Results[1]["link"] != "https://github.com/infiniflow/ragflow" {
		t.Errorf("Results[1].link = %q, want https://github.com/infiniflow/ragflow", env.Results[1]["link"])
	}
}

func TestGoogle_RequiresAPIKey(t *testing.T) {
	t.Parallel()

	tool := NewGoogleTool()
	_, err := tool.InvokableRun(context.Background(), `{"q":"x","api_key":""}`)
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("err = %v, want to mention api_key", err)
	}
}

func TestGoogle_InfoAndInputForm(t *testing.T) {
	t.Parallel()

	tool := NewGoogleTool()
	meta := tool.ToolMeta()
	if meta.Name != "google" {
		t.Errorf("Name = %q, want google", meta.Name)
	}
	if !strings.Contains(meta.Description, "Google") {
		t.Errorf("Desc = %q, want to mention Google", meta.Description)
	}
	form := tool.InputForm()
	if _, ok := form["q"]; !ok {
		t.Fatalf("InputForm missing q: %+v", form)
	}
	if _, ok := form["start"]; !ok {
		t.Fatalf("InputForm missing start: %+v", form)
	}
	if _, ok := form["num"]; !ok {
		t.Fatalf("InputForm missing num: %+v", form)
	}
}

func TestGoogle_MergeDefaultsPrefersExplicitInputs(t *testing.T) {
	t.Parallel()

	got := mergeGoogleDefaults(
		googleParams{
			APIKey:  "DEFAULT_KEY",
			Q:       "default query",
			Num:     6,
			Country: "us",
		},
		googleParams{
			Q:   "agent query",
			Num: 10,
		},
	)

	if got.APIKey != "DEFAULT_KEY" {
		t.Fatalf("APIKey = %q, want DEFAULT_KEY", got.APIKey)
	}
	if got.Q != "agent query" {
		t.Fatalf("Q = %q, want agent query", got.Q)
	}
	if got.Num != 10 {
		t.Fatalf("Num = %d, want 10", got.Num)
	}
	if got.Country != "us" {
		t.Fatalf("Country = %q, want us", got.Country)
	}
}

func TestGoogle_BuildByNameAcceptsNodeParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("google", map[string]any{
		"api_key":  "KEY",
		"country":  "cn",
		"language": "zh-cn",
		"num":      12,
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*GoogleTool)
	if !ok {
		t.Fatalf("built type = %T, want *GoogleTool", built)
	}
	if tool.defaults.APIKey != "KEY" {
		t.Fatalf("defaults.APIKey = %q, want KEY", tool.defaults.APIKey)
	}
	if tool.defaults.Country != "cn" || tool.defaults.Language != "zh-cn" {
		t.Fatalf("defaults locale = %q/%q, want cn/zh-cn", tool.defaults.Country, tool.defaults.Language)
	}
	if tool.defaults.Num != 12 {
		t.Fatalf("defaults.Num = %d, want 12", tool.defaults.Num)
	}
}

func TestGoogle_BuildByNameRejectsRemovedAliasNodeParams(t *testing.T) {
	t.Parallel()

	_, err := BuildByName("google", map[string]any{
		"query":       "ragflow",
		"max_results": 5,
	})
	if err == nil {
		t.Fatal("expected error for removed google alias node params")
	}
	if !strings.Contains(err.Error(), "does not accept node-level param") {
		t.Fatalf("err = %q, want unsupported node-level param error", err.Error())
	}
}
