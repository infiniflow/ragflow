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
	"sync/atomic"
	"testing"
)

func TestPubMed_BuildURL(t *testing.T) {
	t.Parallel()

	t.Run("esearch", func(t *testing.T) {
		t.Parallel()
		got := buildPubMedESearchURL("covid vaccine", 7)
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", got, err)
		}
		if u.Host != "eutils.ncbi.nlm.nih.gov" {
			t.Errorf("host = %q, want eutils.ncbi.nlm.nih.gov", u.Host)
		}
		q := u.Query()
		if q.Get("db") != "pubmed" {
			t.Errorf("db = %q, want pubmed", q.Get("db"))
		}
		if q.Get("term") != "covid vaccine" {
			t.Errorf("term = %q, want covid vaccine", q.Get("term"))
		}
		if q.Get("retmax") != "7" {
			t.Errorf("retmax = %q, want 7", q.Get("retmax"))
		}
		if q.Get("retmode") != "json" {
			t.Errorf("retmode = %q, want json", q.Get("retmode"))
		}
	})

	t.Run("esummary", func(t *testing.T) {
		t.Parallel()
		got := buildPubMedESummaryURL([]string{"12345", "67890"})
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", got, err)
		}
		q := u.Query()
		if q.Get("db") != "pubmed" {
			t.Errorf("db = %q, want pubmed", q.Get("db"))
		}
		if q.Get("id") != "12345,67890" {
			t.Errorf("id = %q, want 12345,67890", q.Get("id"))
		}
		if q.Get("retmode") != "json" {
			t.Errorf("retmode = %q, want json", q.Get("retmode"))
		}
	})

	t.Run("esearch clamp high", func(t *testing.T) {
		t.Parallel()
		got := buildPubMedESearchURL("x", 999)
		u, _ := url.Parse(got)
		if u.Query().Get("retmax") != "100" {
			t.Errorf("retmax = %q, want 100 (clamped)", u.Query().Get("retmax"))
		}
	})
}

func TestPubMed_ParseESummary(t *testing.T) {
	t.Parallel()

	var esearchHits, esummaryHits int32
	var lastUA string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "esearch") {
			atomic.AddInt32(&esearchHits, 1)
			_, _ = w.Write([]byte(`{
				"esearchresult": {
					"idlist": ["11111", "22222"]
				}
			}`))
			return
		}
		if strings.Contains(r.URL.Path, "esummary") {
			atomic.AddInt32(&esummaryHits, 1)
			_, _ = w.Write([]byte(`{
				"result": {
					"uids": ["11111","22222"],
					"11111": {
						"title": "Cochrane review of masks",
						"authors": [{"name":"Smith J"},{"name":"Doe A"}],
						"fulljournalname": "Cochrane Database Syst Rev",
						"pubdate": "2020 Nov 1"
					},
					"22222": {
						"title": "Vaccine efficacy meta-analysis",
						"authors": [{"name":"Alice"},{"name":"Bob"},{"name":"Carol"},{"name":"Dave"}],
						"fulljournalname": "Lancet",
						"pubdate": "2021 Mar-Apr"
					}
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewPubMedToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"covid","max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	if esearchHits != 1 {
		t.Errorf("esearch calls = %d, want 1", esearchHits)
	}
	if esummaryHits != 1 {
		t.Errorf("esummary calls = %d, want 1", esummaryHits)
	}
	if !strings.Contains(lastUA, "ragflow") {
		t.Errorf("User-Agent = %q, want to contain ragflow", lastUA)
	}

	var env pubmedEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].PMID != "11111" {
		t.Errorf("Results[0].PMID = %q, want 11111", env.Results[0].PMID)
	}
	if env.Results[0].Title != "Cochrane review of masks" {
		t.Errorf("Results[0].Title = %q, want Cochrane review of masks", env.Results[0].Title)
	}
	if env.Results[0].Authors != "Smith J, Doe A" {
		t.Errorf("Results[0].Authors = %q, want Smith J, Doe A", env.Results[0].Authors)
	}
	if env.Results[0].Journal != "Cochrane Database Syst Rev" {
		t.Errorf("Results[0].Journal = %q, want Cochrane Database Syst Rev", env.Results[0].Journal)
	}
	if env.Results[0].Year != "2020" {
		t.Errorf("Results[0].Year = %q, want 2020", env.Results[0].Year)
	}
	// 4 authors → first 3 + "et al."
	if !strings.HasSuffix(env.Results[1].Authors, "et al.") {
		t.Errorf("Results[1].Authors = %q, want to end with et al.", env.Results[1].Authors)
	}
}

func TestPubMed_EmptyResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "esearch") {
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewPubMedToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"noresultsfound-zzz-9999","max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var env pubmedEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 0 {
		t.Errorf("Results len = %d, want 0", len(env.Results))
	}
}

func TestPubMed_RequiresQuery(t *testing.T) {
	t.Parallel()

	tool := NewPubMedTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want to mention query", err)
	}
}

func TestPubMed_Info(t *testing.T) {
	t.Parallel()

	tool := NewPubMedTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "pubmed" {
		t.Errorf("Name = %q, want pubmed", info.Name)
	}
	if !strings.Contains(info.Desc, "PubMed") {
		t.Errorf("Desc = %q, want to mention PubMed", info.Desc)
	}
}
