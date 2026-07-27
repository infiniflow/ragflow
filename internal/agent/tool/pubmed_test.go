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
		got := buildPubMedESearchURL("covid vaccine", 7, "user@example.com")
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", got, err)
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
		if q.Get("email") != "user@example.com" {
			t.Errorf("email = %q, want user@example.com", q.Get("email"))
		}
	})

	t.Run("efetch", func(t *testing.T) {
		t.Parallel()
		got := buildPubMedEFetchURL([]string{"12345", "67890"}, "user@example.com")
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", got, err)
		}
		q := u.Query()
		if q.Get("id") != "12345,67890" {
			t.Errorf("id = %q, want 12345,67890", q.Get("id"))
		}
		if q.Get("retmode") != "xml" {
			t.Errorf("retmode = %q, want xml", q.Get("retmode"))
		}
		if q.Get("email") != "user@example.com" {
			t.Errorf("email = %q, want user@example.com", q.Get("email"))
		}
	})
}

func TestPubMed_InvokableRunParsesXML(t *testing.T) {
	t.Parallel()

	var esearchHits, efetchHits int32
	var lastUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastUA = r.Header.Get("User-Agent")
		switch {
		case strings.Contains(r.URL.Path, "esearch"):
			atomic.AddInt32(&esearchHits, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":["12345678"]}}`))
		case strings.Contains(r.URL.Path, "efetch"):
			atomic.AddInt32(&efetchHits, 1)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>12345678</PMID>
      <Article>
        <ArticleTitle>Deep learning for retrieval augmented generation</ArticleTitle>
        <Abstract><AbstractText>A short abstract.</AbstractText></Abstract>
        <Journal>
          <Title>Nature Machine Intelligence</Title>
          <JournalIssue><Volume>10</Volume><Issue>2</Issue></JournalIssue>
        </Journal>
        <Pagination><MedlinePgn>101-110</MedlinePgn></Pagination>
        <AuthorList>
          <Author><LastName>Khan</LastName><ForeName>Furqan</ForeName></Author>
          <Author><LastName>Smith</LastName><ForeName>Jane</ForeName></Author>
        </AuthorList>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1000/example.doi</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	tool := NewPubMedToolWithDefaults(helper, pubmedParams{TopN: 3, Email: "tester@example.com"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"ragflow"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if esearchHits != 1 || efetchHits != 1 {
		t.Fatalf("calls = esearch:%d efetch:%d, want 1/1", esearchHits, efetchHits)
	}
	if !strings.Contains(lastUA, "ragflow") {
		t.Fatalf("User-Agent = %q, want ragflow marker", lastUA)
	}

	var env pubmedEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", err, out)
	}
	if env.Error != "" {
		t.Fatalf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(env.Results))
	}
	result := env.Results[0]
	if result.Title != "Deep learning for retrieval augmented generation" {
		t.Fatalf("Title = %q", result.Title)
	}
	if result.URL != "https://pubmed.ncbi.nlm.nih.gov/12345678" {
		t.Fatalf("URL = %q", result.URL)
	}
	for _, want := range []string{
		"Title: Deep learning for retrieval augmented generation",
		"Authors: Furqan Khan, Jane Smith",
		"Journal: Nature Machine Intelligence",
		"Volume: 10",
		"Issue: 2",
		"Pages: 101-110",
		"DOI: 10.1000/example.doi",
		"Abstract: A short abstract.",
	} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("Content missing %q: %s", want, result.Content)
		}
	}
}

func TestPubMed_InvokableRunEmptyResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	tool := NewPubMedToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"query":"missing"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var env pubmedEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", err, out)
	}
	if len(env.Results) != 0 || env.Error != "" {
		t.Fatalf("env = %+v, want empty results and no error", env)
	}
}

func TestPubMed_InvokableRunEmptyQuery(t *testing.T) {
	t.Parallel()

	tool := NewPubMedTool()
	out, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun(empty): %v", err)
	}
	var envelope pubmedEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || len(envelope.Results) != 0 {
		t.Fatalf("empty result = %s / %v", out, err)
	}
}

func TestPubMed_InfoOnlyExposesQuery(t *testing.T) {
	t.Parallel()

	tool := NewPubMedTool()
	meta := tool.ToolMeta()
	if meta.Name != "pubmed_search" {
		t.Errorf("Name = %q, want pubmed_search", meta.Name)
	}
	if !strings.Contains(meta.Description, "PubMed") {
		t.Errorf("Desc = %q, want to mention PubMed", meta.Description)
	}
}

func TestPubMed_BuildByNameAcceptsNodeParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("pubmed", map[string]any{
		"top_n": 8, "email": "node@example.com", "outputs": map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*PubMedTool)
	if !ok {
		t.Fatalf("built type = %T, want *PubMedTool", built)
	}
	if tool.defaults.TopN != 8 {
		t.Fatalf("defaults.TopN = %d, want 8", tool.defaults.TopN)
	}
	if tool.defaults.Email != "node@example.com" {
		t.Fatalf("defaults.Email = %q, want node@example.com", tool.defaults.Email)
	}
}

func TestPubMed_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()

	pubmed := NewPubMedTool()
	spec := pubmed.ComponentSpec()
	if query, ok := spec.InputForm["query"].(map[string]any); !ok || query["type"] != "line" {
		t.Fatalf("query input form = %#v", spec.InputForm["query"])
	}
	envelope := map[string]any{"results": []any{map[string]any{
		"title": "Paper", "url": "https://pubmed.ncbi.nlm.nih.gov/1", "content": "Title: Paper\nAbstract: Evidence.",
	}}}
	chunks, docAggs := pubmed.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 || chunks[0]["document_name"] != "Paper" {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	outputs := pubmed.BuildComponentOutputs(envelope)
	if results, ok := outputs["json"].([]any); !ok || len(results) != 1 {
		t.Fatalf("json output = %#v", outputs["json"])
	}
	if !strings.Contains(outputs["formalized_content"].(string), "Abstract: Evidence.") {
		t.Fatalf("formalized_content = %q", outputs["formalized_content"])
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("output conversion mutated envelope: %#v", envelope)
	}
}

func TestPubMed_MergeDefaults(t *testing.T) {
	t.Parallel()

	got := mergePubMedDefaults(
		pubmedParams{Query: "configured query", TopN: 8, Email: "node@example.com"},
		pubmedParams{Query: "runtime query"},
	)
	if got.Query != "runtime query" || got.TopN != 8 || got.Email != "node@example.com" {
		t.Fatalf("merged params = %+v, want runtime query with node defaults", got)
	}
}

func TestPubMed_BuildByNameRejectsInvalidTopN(t *testing.T) {
	t.Parallel()

	_, err := BuildByName("pubmed", map[string]any{"top_n": 0})
	if err == nil {
		t.Fatal("expected top_n validation error")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Fatalf("err = %q, want positive integer validation", err.Error())
	}
}

func TestPubMed_BuildByNameRejectsInvalidNodeTypes(t *testing.T) {
	t.Parallel()

	for _, params := range []map[string]any{{"top_n": 1.5}, {"email": 1}, {"email": ""}} {
		if _, err := BuildByName("pubmed", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded", params)
		}
	}
}
