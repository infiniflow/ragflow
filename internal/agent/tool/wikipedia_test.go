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

func TestWikipedia_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		lang     string
		query    string
		max      int
		wantHost string
		wantPath string
	}{
		{
			name:     "en default",
			lang:     "",
			query:    "rag flow",
			max:      0,
			wantHost: "en.wikipedia.org",
			wantPath: "/w/api.php",
		},
		{
			name:     "de explicit",
			lang:     "de",
			query:    "Berlin",
			max:      3,
			wantHost: "de.wikipedia.org",
			wantPath: "/w/api.php",
		},
		{
			name:     "spaces encoded",
			lang:     "en",
			query:    "a b c",
			max:      1,
			wantHost: "en.wikipedia.org",
			wantPath: "/w/api.php",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildWikipediaURL(tc.lang, tc.query, tc.max)
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
			if q.Get("action") != "query" {
				t.Errorf("action = %q, want query", q.Get("action"))
			}
			if q.Get("list") != "search" {
				t.Errorf("list = %q, want search", q.Get("list"))
			}
			if q.Get("format") != "json" {
				t.Errorf("format = %q, want json", q.Get("format"))
			}
			if q.Get("srsearch") != tc.query {
				t.Errorf("srsearch = %q, want %q (raw query, not pre-encoded)", q.Get("srsearch"), tc.query)
			}
		})
	}
}

func TestWikipedia_ParseResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"query": {
				"search": [
					{"title":"RAG","snippet":"<span>rag</span> is ..."},
					{"title":"Retrieval-augmented generation","snippet":"<b>RAG</b> is ..."}
				]
			}
		}`))
	}))
	defer srv.Close()

	// Point the hard-coded en.wikipedia.org endpoint at the test server
	// by injecting a transport that rewrites the request host.
	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewWikipediaToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"query":"RAG","lang":"en","max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env wikipediaEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].Title != "RAG" {
		t.Errorf("Results[0].Title = %q, want RAG", env.Results[0].Title)
	}
	if !strings.HasPrefix(env.Results[0].URL, "https://en.wikipedia.org/wiki/") {
		t.Errorf("Results[0].URL = %q, want to start with https://en.wikipedia.org/wiki/", env.Results[0].URL)
	}
}

// rewriteHostTransport returns a RoundTripper that rewrites the request
// host to the given test server URL. Used to point the hard-coded
// en.wikipedia.org endpoint at a httptest.Server without changing the
// production URL builder.
func rewriteHostTransport(srvURL string) http.RoundTripper {
	u, err := url.Parse(srvURL)
	if err != nil {
		panic("rewriteHostTransport: bad srvURL: " + err.Error())
	}
	return &hostSwapRT{inner: http.DefaultTransport, host: u.Host, scheme: u.Scheme}
}

type hostSwapRT struct {
	inner  http.RoundTripper
	host   string
	scheme string
}

func (t *hostSwapRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Scheme = t.scheme
	r2.URL.Host = t.host
	r2.Host = t.host
	return t.inner.RoundTrip(r2)
}

func TestWikipedia_Info(t *testing.T) {
	t.Parallel()

	tool := NewWikipediaTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "wikipedia" {
		t.Errorf("Name = %q, want wikipedia", info.Name)
	}
	if !strings.Contains(info.Desc, "Wikipedia") {
		t.Errorf("Desc = %q, want to mention Wikipedia", info.Desc)
	}
}

func TestWikipedia_RequiresQuery(t *testing.T) {
	t.Parallel()

	tool := NewWikipediaTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want to mention query", err)
	}
}
