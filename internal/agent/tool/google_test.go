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
		apiKey   string
		cx       string
		query    string
		max      int
		wantNum  string
		wantHost string
	}{
		{
			name:     "default",
			apiKey:   "KEY",
			cx:       "CXID",
			query:    "ragflow",
			max:      0,
			wantNum:  "5",
			wantHost: "www.googleapis.com",
		},
		{
			name:     "clamped high",
			apiKey:   "K",
			cx:       "C",
			query:    "x",
			max:      50,
			wantNum:  "10",
			wantHost: "www.googleapis.com",
		},
		{
			name:     "explicit low",
			apiKey:   "K",
			cx:       "C",
			query:    "x y",
			max:      3,
			wantNum:  "3",
			wantHost: "www.googleapis.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGoogleURL(tc.apiKey, tc.cx, tc.query, tc.max)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "/customsearch/v1" {
				t.Errorf("path = %q, want /customsearch/v1", u.Path)
			}
			q := u.Query()
			if q.Get("key") != tc.apiKey {
				t.Errorf("key = %q, want %q", q.Get("key"), tc.apiKey)
			}
			if q.Get("cx") != tc.cx {
				t.Errorf("cx = %q, want %q", q.Get("cx"), tc.cx)
			}
			if q.Get("q") != tc.query {
				t.Errorf("q = %q, want %q", q.Get("q"), tc.query)
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
			"items": [
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
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","api_key":"K","cx":"C","max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

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
	if env.Results[0].Title != "RAGFlow" {
		t.Errorf("Results[0].Title = %q, want RAGFlow", env.Results[0].Title)
	}
	if env.Results[1].Link != "https://github.com/infiniflow/ragflow" {
		t.Errorf("Results[1].Link = %q, want https://github.com/infiniflow/ragflow", env.Results[1].Link)
	}
}

func TestGoogle_RequiresAPIKeyAndCX(t *testing.T) {
	t.Parallel()

	tool := NewGoogleTool()
	_, err := tool.InvokableRun(context.Background(),
		`{"query":"x","api_key":"","cx":""}`)
	if err == nil {
		t.Fatal("expected error for missing api_key and cx")
	}
	if !strings.Contains(err.Error(), "api_key") || !strings.Contains(err.Error(), "cx") {
		t.Errorf("err = %v, want to mention api_key and cx", err)
	}
}

func TestGoogle_Info(t *testing.T) {
	t.Parallel()

	tool := NewGoogleTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "google" {
		t.Errorf("Name = %q, want google", info.Name)
	}
	if !strings.Contains(info.Desc, "Google") {
		t.Errorf("Desc = %q, want to mention Google", info.Desc)
	}
}
