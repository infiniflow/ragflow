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

func TestSearXNG_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		base     string
		query    string
		max      int
		wantPath string
		wantHost string
	}{
		{
			name:     "default base",
			base:     "",
			query:    "ragflow",
			max:      0,
			wantPath: "/search",
			wantHost: "localhost:8888",
		},
		{
			name:     "custom base trailing slash",
			base:     "https://searx.example.com/",
			query:    "rag",
			max:      3,
			wantPath: "/search",
			wantHost: "searx.example.com",
		},
		{
			name:     "custom base no slash",
			base:     "https://searx.example.com",
			query:    "rag",
			max:      7,
			wantPath: "/search",
			wantHost: "searx.example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildSearXNGURL(tc.base, tc.query, tc.max)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", u.Path, tc.wantPath)
			}
			q := u.Query()
			if q.Get("q") != tc.query {
				t.Errorf("q = %q, want %q", q.Get("q"), tc.query)
			}
			if q.Get("format") != "json" {
				t.Errorf("format = %q, want json", q.Get("format"))
			}
		})
	}
}

func TestSearXNG_ParseResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"title":"RAGFlow","url":"https://ragflow.io","content":"Open source RAG engine"},
				{"title":"GitHub","url":"https://github.com/infiniflow/ragflow","content":"Source code"}
			]
		}`))
	}))
	defer srv.Close()

	tool := NewSearXNGTool()
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","base_url":`+jsonString(srv.URL)+`,"max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env searxngEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].Title != "RAGFlow" {
		t.Errorf("Results[0].Title = %q, want RAGFlow", env.Results[0].Title)
	}
	if env.Results[1].URL != "https://github.com/infiniflow/ragflow" {
		t.Errorf("Results[1].URL = %q, want https://github.com/infiniflow/ragflow", env.Results[1].URL)
	}
}

func TestSearXNG_Info(t *testing.T) {
	t.Parallel()

	tool := NewSearXNGTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "searxng" {
		t.Errorf("Name = %q, want searxng", info.Name)
	}
	if !strings.Contains(info.Desc, "SearXNG") {
		t.Errorf("Desc = %q, want to mention SearXNG", info.Desc)
	}
}

func TestSearXNG_RequiresQuery(t *testing.T) {
	t.Parallel()

	tool := NewSearXNGTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want to mention query", err)
	}
}
