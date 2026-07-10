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

const cannedScholarHTML = `<html><body>
<div class="gs_r gs_or gs_scl" style="display:none"></div>
<div class="gs_r">
  <div class="gs_ri">
    <h3 class="gs_rt">
      <a href="https://example.com/paper1">Attention is all you need</a>
    </h3>
    <div class="gs_a">Ashish Vaswani, Noam Shazeer, Niki Parmar - NeurIPS 2017</div>
    <div class="gs_rs">The dominant sequence transduction models are based on complex recurrent or convolutional neural networks.</div>
  </div>
</div>
<div class="gs_r">
  <div class="gs_ri">
    <h3 class="gs_rt">
      <a href="https://example.com/paper2">BERT: Pre-training of Deep Bidirectional Transformers</a>
    </h3>
    <div class="gs_a">Jacob Devlin, Ming-Wei Chang - NAACL 2019</div>
    <div class="gs_rs">We introduce a new language representation model called BERT.</div>
  </div>
</div>
</body></html>`

func TestGoogleScholar_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		query      string
		topN       int
		sortBy     string
		yearLow    int
		yearHigh   int
		patents    *bool
		wantNum    string
		wantHost   string
		wantQuery  string
		wantParams map[string]string
	}{
		{
			name:      "default",
			query:     "transformer",
			wantNum:   "12",
			wantHost:  "scholar.google.com",
			wantQuery: "transformer",
		},
		{
			name:      "explicit high",
			query:     "x",
			topN:      99,
			wantNum:   "99",
			wantHost:  "scholar.google.com",
			wantQuery: "x",
		},
		{
			name:      "explicit",
			query:     "a b",
			topN:      3,
			wantNum:   "3",
			wantHost:  "scholar.google.com",
			wantQuery: "a b",
		},
		{
			name:       "sort by date",
			query:      "test",
			topN:       5,
			sortBy:     "date",
			wantNum:    "5",
			wantHost:   "scholar.google.com",
			wantQuery:  "test",
			wantParams: map[string]string{"scisbd": "1"},
		},
		{
			name:       "year range",
			query:      "ml",
			topN:       10,
			yearLow:    2020,
			yearHigh:   2024,
			wantNum:    "10",
			wantHost:   "scholar.google.com",
			wantQuery:  "ml",
			wantParams: map[string]string{"as_ylo": "2020", "as_yhi": "2024"},
		},
		{
			name:       "exclude patents",
			query:      "ai",
			topN:       5,
			patents:    boolPtr(false),
			wantNum:    "5",
			wantHost:   "scholar.google.com",
			wantQuery:  "ai",
			wantParams: map[string]string{"as_vis": "1"},
		},
		{
			name:      "include patents (default)",
			query:     "nlp",
			topN:      5,
			patents:   boolPtr(true),
			wantNum:   "5",
			wantHost:  "scholar.google.com",
			wantQuery: "nlp",
		},
		{
			name:      "all params combined",
			query:     "deep learning",
			topN:      8,
			sortBy:    "date",
			yearLow:   2019,
			yearHigh:  2023,
			patents:   boolPtr(false),
			wantNum:   "8",
			wantHost:  "scholar.google.com",
			wantQuery: "deep learning",
			wantParams: map[string]string{
				"scisbd": "1",
				"as_ylo": "2019",
				"as_yhi": "2023",
				"as_vis": "1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGoogleScholarURL(tc.query, tc.max)
			u, _ := url.Parse(got)
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "/scholar" {
				t.Errorf("path = %q, want /scholar", u.Path)
			}
			q := u.Query()
			if q.Get("q") != tc.wantQuery {
				t.Errorf("q = %q, want %q", q.Get("q"), tc.wantQuery)
			}
			if q.Get("num") != tc.wantNum {
				t.Errorf("num = %q, want %q", q.Get("num"), tc.wantNum)
			}
			for k, v := range tc.wantParams {
				if q.Get(k) != v {
					t.Errorf("%s = %q, want %q", k, q.Get(k), v)
				}
			}
		})
	}
}

func TestGoogleScholar_ParseResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte(cannedScholarHTML))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewGoogleScholarToolWith(helper)
	out, _ := tool.InvokableRun(context.Background(),
		`{"query":"transformer","max_results":5}`)

	var env googleScholarEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if !strings.Contains(env.Results[0].Title, "Attention") {
		t.Errorf("Results[0].Title = %q, want to contain Attention", env.Results[0].Title)
	}
	if env.Results[0].Link != "https://example.com/paper1" {
		t.Errorf("Results[0].Link = %q, want https://example.com/paper1", env.Results[0].Link)
	}
	if !strings.Contains(env.Results[0].Authors, "Vaswani") {
		t.Errorf("Results[0].Authors = %q, want to contain Vaswani", env.Results[0].Authors)
	}
	if env.Results[0].Year != "2017" {
		t.Errorf("Results[0].Year = %q, want 2017", env.Results[0].Year)
	}
	if !strings.Contains(env.Results[0].Snippet, "sequence transduction") {
		t.Errorf("Results[0].Snippet = %q, want snippet", env.Results[0].Snippet)
	}
	if env.Results[1].Year != "2019" {
		t.Errorf("Results[1].Year = %q, want 2019", env.Results[1].Year)
	}
}

func TestGoogleScholar_RequiresQuery(t *testing.T) {
	t.Parallel()

	tool := NewGoogleScholarTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want to mention query", err)
	}
}

func TestGoogleScholar_Info(t *testing.T) {
	t.Parallel()

	tool := NewGoogleScholarTool()
	meta := tool.ToolMeta()
	if meta.Name != "google_scholar" {
		t.Errorf("Name = %q, want google_scholar", meta.Name)
	}
	if !strings.Contains(meta.Description, "Scholar") {
		t.Errorf("Desc = %q, want to mention Scholar", meta.Description)
	}
}

func TestGoogleScholar_MergesNodeLevelDefaults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("q") != "rag" {
			t.Errorf("q = %q, want rag", q.Get("q"))
		}
		if q.Get("num") != "7" {
			t.Errorf("num = %q, want 7", q.Get("num"))
		}
		if q.Get("scisbd") != "1" {
			t.Errorf("scisbd = %q, want 1", q.Get("scisbd"))
		}
		if q.Get("as_ylo") != "2020" {
			t.Errorf("as_ylo = %q, want 2020", q.Get("as_ylo"))
		}
		if q.Get("as_yhi") != "2024" {
			t.Errorf("as_yhi = %q, want 2024", q.Get("as_yhi"))
		}
		if q.Get("as_vis") != "1" {
			t.Errorf("as_vis = %q, want 1", q.Get("as_vis"))
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte(cannedScholarHTML))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewGoogleScholarToolWithDefaults(helper, googleScholarParams{
		Query:    "rag",
		TopN:     7,
		SortBy:   "date",
		YearLow:  2020,
		YearHigh: 2024,
		Patents:  boolPtr(false),
	})
	_, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun with defaults: %v", err)
	}
}
