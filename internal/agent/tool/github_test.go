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
			wantPerPage: "5",
			wantHost:    "api.github.com",
		},
		{
			name:        "clamped high",
			query:       "x",
			max:         99,
			wantPerPage: "30",
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
		})
	}
}

func TestGitHub_ParseResponse(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": [
				{"full_name":"infiniflow/ragflow","html_url":"https://github.com/infiniflow/ragflow","description":"RAG engine","stargazers_count":12000},
				{"full_name":"foo/bar","html_url":"https://github.com/foo/bar","description":"","stargazers_count":5}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewGitHubToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","max_results":5,"token":"ghp_xyz"}`)
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
	if env.Results[0].FullName != "infiniflow/ragflow" {
		t.Errorf("Results[0].FullName = %q, want infiniflow/ragflow", env.Results[0].FullName)
	}
	if env.Results[0].StargazersCount != 12000 {
		t.Errorf("Results[0].StargazersCount = %d, want 12000", env.Results[0].StargazersCount)
	}
	if env.Results[1].Description != "" {
		t.Errorf("Results[1].Description = %q, want empty", env.Results[1].Description)
	}
	if gotAuth != "Bearer ghp_xyz" {
		t.Errorf("Authorization = %q, want Bearer ghp_xyz", gotAuth)
	}
}

func TestGitHub_RequiresQuery(t *testing.T) {
	t.Parallel()

	tool := NewGitHubTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want to mention query", err)
	}
}

func TestGitHub_Info(t *testing.T) {
	t.Parallel()

	tool := NewGitHubTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "github" {
		t.Errorf("Name = %q, want github", info.Name)
	}
	if !strings.Contains(info.Desc, "GitHub") {
		t.Errorf("Desc = %q, want to mention GitHub", info.Desc)
	}
}
