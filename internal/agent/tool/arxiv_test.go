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

func TestArxiv_BuildURL(t *testing.T) {
	t.Parallel()

	got := buildArxivURL("transformer", 3)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if u.Host != "export.arxiv.org" {
		t.Errorf("host = %q, want export.arxiv.org", u.Host)
	}
	if u.Path != "/api/query" {
		t.Errorf("path = %q, want /api/query", u.Path)
	}
	q := u.Query()
	if q.Get("search_query") != "all:transformer" {
		t.Errorf("search_query = %q, want all:transformer", q.Get("search_query"))
	}
	if q.Get("max_results") != "3" {
		t.Errorf("max_results = %q, want 3", q.Get("max_results"))
	}
}

func TestArxiv_ParseAtomEntry(t *testing.T) {
	t.Parallel()

	const canned = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <title>arXiv Query: all:rag</title>
  <entry>
    <id>http://arxiv.org/abs/2501.12345v1</id>
    <title>Retrieval-Augmented Generation for
      Open-Domain Question Answering</title>
    <summary>  We present a method
      combining retrieval and generation.  </summary>
    <author><name>Alice Liddell</name></author>
    <author><name>Bob Builder</name></author>
    <link href="http://arxiv.org/abs/2501.12345v1" rel="alternate" type="text/html"/>
    <link href="http://arxiv.org/pdf/2501.12345v1" rel="related" type="application/pdf"/>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2409.99999v2</id>
    <title>Single-Author Paper</title>
    <summary>Brief.</summary>
    <author><name>Carol Danvers</name></author>
    <!-- no pdf link: pickArxivPDF must fall back to abs id derived pdf -->
  </entry>
</feed>`

	results, err := parseArxivAtom([]byte(canned))
	if err != nil {
		t.Fatalf("parseArxivAtom: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	// First entry: explicit pdf link
	r0 := results[0]
	if !strings.Contains(r0.Title, "Retrieval-Augmented Generation") {
		t.Errorf("r0.Title = %q, want to contain 'Retrieval-Augmented Generation'", r0.Title)
	}
	if !strings.HasPrefix(r0.Title, "Retrieval-") {
		// whitespace normalized
		if strings.Contains(r0.Title, "\n") {
			t.Errorf("r0.Title contains raw newline: %q", r0.Title)
		}
	}
	if len(r0.Authors) != 2 {
		t.Errorf("r0.Authors len = %d, want 2", len(r0.Authors))
	}
	if r0.Authors[0] != "Alice Liddell" || r0.Authors[1] != "Bob Builder" {
		t.Errorf("r0.Authors = %v, want [Alice Liddell Bob Builder]", r0.Authors)
	}
	if r0.PDFURL != "http://arxiv.org/pdf/2501.12345v1" {
		t.Errorf("r0.PDFURL = %q, want http://arxiv.org/pdf/2501.12345v1", r0.PDFURL)
	}
	if r0.EntryID != "http://arxiv.org/abs/2501.12345v1" {
		t.Errorf("r0.EntryID = %q, want http://arxiv.org/abs/2501.12345v1", r0.EntryID)
	}
	if !strings.HasPrefix(r0.Summary, "We present") {
		t.Errorf("r0.Summary = %q, want to start with 'We present' (whitespace normalized)", r0.Summary)
	}

	// Second entry: fallback pdf derived from abs id
	r1 := results[1]
	if r1.PDFURL != "http://arxiv.org/pdf/2409.99999v2" {
		t.Errorf("r1.PDFURL = %q, want http://arxiv.org/pdf/2409.99999v2 (derived)", r1.PDFURL)
	}
	if len(r1.Authors) != 1 || r1.Authors[0] != "Carol Danvers" {
		t.Errorf("r1.Authors = %v, want [Carol Danvers]", r1.Authors)
	}
}

func TestArxiv_Info(t *testing.T) {
	t.Parallel()

	tool := NewArxivTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "arxiv" {
		t.Errorf("Name = %q, want arxiv", info.Name)
	}
	if !strings.Contains(info.Desc, "arXiv") {
		t.Errorf("Desc = %q, want to mention arXiv", info.Desc)
	}
}

func TestArxiv_RequiresQuery(t *testing.T) {
	t.Parallel()

	tool := NewArxivTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want to mention query", err)
	}
}

func TestArxiv_FullRoundtrip(t *testing.T) {
	t.Parallel()

	const canned = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>arXiv Query</title>
  <entry>
    <id>http://arxiv.org/abs/2501.12345v1</id>
    <title>Test Paper</title>
    <summary>Summary.</summary>
    <author><name>Author One</name></author>
    <link href="http://arxiv.org/pdf/2501.12345v1" rel="related" type="application/pdf"/>
  </entry>
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(canned))
	}))
	defer srv.Close()

	// rewriteHostTransport points the hard-coded export.arxiv.org at the
	// test server.
	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewArxivToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"query":"rag","max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env arxivEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(env.Results))
	}
	if env.Results[0].Title != "Test Paper" {
		t.Errorf("Title = %q, want Test Paper", env.Results[0].Title)
	}
	if env.Results[0].PDFURL != "http://arxiv.org/pdf/2501.12345v1" {
		t.Errorf("PDFURL = %q, want http://arxiv.org/pdf/2501.12345v1", env.Results[0].PDFURL)
	}
}
