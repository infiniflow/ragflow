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

package youcom

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestEndpointForPicksKeylessWithoutAKey(t *testing.T) {
	cases := []struct {
		name   string
		apiKey string
		want   string
	}{
		{name: "no key", apiKey: "", want: KeylessWebSearchEndpoint},
		{name: "whitespace key", apiKey: "   ", want: KeylessWebSearchEndpoint},
		{name: "key set", apiKey: "ydc-test", want: WebSearchEndpoint},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EndpointFor(tc.apiKey); got != tc.want {
				t.Fatalf("endpoint = %q, want %q", got, tc.want)
			}
		})
	}
}

// captureFetch returns a Fetcher that records the endpoint and headers and
// answers with body.
func captureFetch(body string, endpoint *string, headers *map[string]string) Fetcher {
	return func(_ context.Context, gotEndpoint string, gotHeaders map[string]string) ([]byte, error) {
		*endpoint = gotEndpoint
		*headers = gotHeaders
		return []byte(body), nil
	}
}

func TestSearchMergesSectionsAndSendsConfig(t *testing.T) {
	var endpoint string
	var headers map[string]string
	body := `{"results":{"web":[` +
		`{"url":"https://example.com/a","title":"A","snippets":["first","  ","second"],"description":"ignored"},` +
		`{"url":"https://example.com/b","title":"B","description":"desc fallback"}` +
		`],"news":[{"url":"https://news.example.com/c","title":"C","description":"news desc"}]}}`

	hits, err := Search(t.Context(), captureFetch(body, &endpoint, &headers), Request{
		Endpoint:  EndpointFor(""),
		APIKey:    "",
		Query:     "ragflow",
		Freshness: "week",
		Count:     6,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if parsed.Query().Get("query") != "ragflow" {
		t.Errorf("query = %q, want ragflow", parsed.Query().Get("query"))
	}
	if parsed.Query().Get("count") != "6" {
		t.Errorf("count = %q, want 6", parsed.Query().Get("count"))
	}
	if parsed.Query().Get("freshness") != "week" {
		t.Errorf("freshness = %q, want week", parsed.Query().Get("freshness"))
	}
	if got := headers["User-Agent"]; got != UserAgent {
		t.Errorf("User-Agent = %q, want %q", got, UserAgent)
	}
	// The keyless endpoint rejects an X-API-Key header, so none may be sent.
	if got, ok := headers["X-API-Key"]; ok {
		t.Errorf("X-API-Key = %q, want absent", got)
	}

	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3", len(hits))
	}
	if hits[0].Content != "first\nsecond" {
		t.Errorf("web content = %q, want joined non-blank snippets", hits[0].Content)
	}
	if hits[1].Content != "desc fallback" {
		t.Errorf("description fallback = %q", hits[1].Content)
	}
	if hits[2].Content != "news desc" || hits[2].Title != "C" {
		t.Errorf("news hit = %#v", hits[2])
	}
}

func TestSearchSendsKeyAndOmitsFreshnessForAny(t *testing.T) {
	var endpoint string
	var headers map[string]string
	hits, err := Search(t.Context(), captureFetch(`{"results":{"web":[]}}`, &endpoint, &headers), Request{
		Endpoint:  EndpointFor("ydc-test"),
		APIKey:    "  ydc-test  ",
		Query:     "q",
		Freshness: "any",
		Count:     3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %d, want 0", len(hits))
	}
	if got := headers["X-API-Key"]; got != "ydc-test" {
		t.Errorf("X-API-Key = %q, want trimmed key", got)
	}
	parsed, _ := url.Parse(endpoint)
	if _, ok := parsed.Query()["freshness"]; ok {
		t.Errorf("freshness sent for \"any\": %v", parsed.Query().Get("freshness"))
	}
}

func TestSearchDiscardsBlankContent(t *testing.T) {
	var endpoint string
	var headers map[string]string
	body := `{"results":{"web":[{"url":"https://example.com/blank","title":"Blank","description":"   "}]}}`
	hits, err := Search(t.Context(), captureFetch(body, &endpoint, &headers), Request{Count: 6})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The leaf keeps the hit (the caller decides whether blank content is
	// usable); a blank description must not be turned into non-empty text.
	if len(hits) != 1 || strings.TrimSpace(hits[0].Content) != "" {
		t.Fatalf("hits = %#v, want one blank-content hit", hits)
	}
}

func TestSearchWrapsFetchError(t *testing.T) {
	fetch := func(context.Context, string, map[string]string) ([]byte, error) {
		return nil, errors.New("status 402")
	}
	_, err := Search(t.Context(), fetch, Request{Query: "q", Count: 6})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "youcom: status 402") {
		t.Fatalf("err = %q, want youcom-prefixed fetch error", err.Error())
	}
}

func TestSearchRejectsMalformedBody(t *testing.T) {
	fetch := func(context.Context, string, map[string]string) ([]byte, error) {
		return []byte("not json"), nil
	}
	_, err := Search(t.Context(), fetch, Request{Query: "q", Count: 6})
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if !strings.Contains(err.Error(), "youcom: decode response") {
		t.Fatalf("err = %q, want decode error", err.Error())
	}
}
