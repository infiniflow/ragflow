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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveWebSearchProviderSelectsAnySearch(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  interface{}
		want string
	}{
		{name: "missing key"},
		{name: "empty key", key: ""},
		{name: "whitespace key", key: " \t\n"},
		{name: "non-string key", key: 42},
		{name: "saved key", key: "  anysearch-test  ", want: "anysearch-test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]interface{}{
				"web_search_provider": "anysearch",
				"tavily_api_key":      "tavily-other",
				"youcom_api_key":      "youcom-other",
			}
			if tc.key != nil {
				config["anysearch_api_key"] = tc.key
			}
			provider := resolveWebSearchProvider(config)
			if provider == nil || provider.Provider != "anysearch" || provider.APIKey != tc.want {
				t.Fatalf("provider = %+v, want AnySearch with key %q", provider, tc.want)
			}
		})
	}
	if provider := resolveWebSearchProvider(map[string]interface{}{"anysearch_api_key": "anysearch-test"}); provider != nil {
		t.Fatalf("unselected AnySearch key activated search: %+v", provider)
	}
}

func TestRetrieveAnySearchWebSearchSendsQueryAndOptionalBearer(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		want string
	}{
		{name: "anonymous"},
		{name: "blank key", key: " \t\n"},
		{name: "saved key", key: "  anysearch-test  ", want: "Bearer anysearch-test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			query := "RAGFlow 中文? \"citations\" + diagrams"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost {
					t.Errorf("method = %q, want POST", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != tc.want {
					t.Errorf("Authorization = %q, want %q", got, tc.want)
				}
				if tc.want == "" {
					if _, exists := r.Header["Authorization"]; exists {
						t.Error("anonymous request has an Authorization header")
					}
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
				}
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if body["query"] != query || body["max_results"] != float64(6) || len(body) != 2 {
					t.Errorf("request body = %#v, want query and max_results=6", body)
				}
				_, _ = io.WriteString(w, `{"code":0,"data":{"results":[{"title":"RAGFlow","url":"https://example.com/ragflow","content":" An open-source RAG engine. ","snippet":"Ignored snippet."}]}}`)
			}))
			defer server.Close()
			result, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, tc.key, query)
			if err != nil {
				t.Fatalf("retrieve AnySearch: %v", err)
			}
			if calls != 1 {
				t.Fatalf("requests = %d, want 1", calls)
			}
			assertWebSearchChunk(t, result, "anysearch-https://example.com/ragflow", "RAGFlow", "An open-source RAG engine.")
			chunk := result["chunks"].([]map[string]interface{})[0]
			agg := result["doc_aggs"].([]interface{})[0].(map[string]interface{})
			if chunk["doc_id"] != chunk["chunk_id"] || agg["doc_id"] != chunk["doc_id"] || agg["url"] != chunk["url"] {
				t.Fatalf("citation identifiers disagree: chunk=%#v aggregate=%#v", chunk, agg)
			}
		})
	}
}

func TestDecodeAnySearchWebSearchResultsChecksEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", `{`},
		{"trailing JSON", `{"code":0,"data":{"results":[]}} {}`},
		{"null response", `null`},
		{"array response", `[]`},
		{"missing code", `{"data":{"results":[]}}`},
		{"null code", `{"code":null,"data":{"results":[]}}`},
		{"string code", `{"code":"0","data":{"results":[]}}`},
		{"boolean code", `{"code":false,"data":{"results":[]}}`},
		{"decimal code", `{"code":0.0,"data":{"results":[]}}`},
		{"nonzero code", `{"code":401,"message":"upstream-secret","data":{"results":[]}}`},
		{"missing data", `{"code":0}`},
		{"null data", `{"code":0,"data":null}`},
		{"array data", `{"code":0,"data":[]}`},
		{"missing results", `{"code":0,"data":{}}`},
		{"null results", `{"code":0,"data":{"results":null}}`},
		{"object results", `{"code":0,"data":{"results":{}}}`},
		{"null result", `{"code":0,"data":{"results":[null]}}`},
		{"string result", `{"code":0,"data":{"results":["upstream-secret"]}}`},
		{"wrong title type", `{"code":0,"data":{"results":[{"title":42}]}}`},
		{"wrong url type", `{"code":0,"data":{"results":[{"url":false}]}}`},
		{"wrong content type", `{"code":0,"data":{"results":[{"content":{"secret":"upstream-secret"}}]}}`},
		{"wrong snippet type", `{"code":0,"data":{"results":[{"snippet":[]}]}}`},
		{"malformed second result", `{"code":0,"data":{"results":[{"url":"https://example.com","content":"valid"},null]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := decodeAnySearchWebSearchResults([]byte(tc.body))
			if err == nil || len(results) != 0 {
				t.Fatalf("results=%#v error=%v, want failure without partial results", results, err)
			}
			if strings.Contains(err.Error(), "upstream-secret") {
				t.Fatalf("error exposed upstream content: %v", err)
			}
		})
	}
	for _, body := range []string{
		`{"code":0,"data":{"results":[]}}`,
		`{"code":0,"data":{"results":[{}]}}`,
		`{"code":0,"data":{"results":[{"title":null,"url":null,"content":null,"snippet":null}]}}`,
	} {
		if _, err := decodeAnySearchWebSearchResults([]byte(body)); err != nil {
			t.Fatalf("valid envelope %s: %v", body, err)
		}
	}
}

func TestRetrieveAnySearchWebSearchFiltersBeforeCapAndFallsBackToSnippet(t *testing.T) {
	results := []map[string]string{
		{"url": "https://example.com/blank", "content": " \n", "snippet": " \t"},
		{"url": " \t", "content": "orphan text"},
	}
	for i := 0; i < 8; i++ {
		results = append(results, map[string]string{"title": fmt.Sprintf("Hit %d", i), "url": fmt.Sprintf(" https://example.com/%d ", i), "content": " \n", "snippet": fmt.Sprintf(" Snippet %d ", i)})
	}
	body, err := json.Marshal(map[string]interface{}{"code": 0, "data": map[string]interface{}{"results": results}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	payload, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, "", "ragflow")
	if err != nil {
		t.Fatal(err)
	}
	chunks := payload["chunks"].([]map[string]interface{})
	if len(chunks) != 6 || len(payload["doc_aggs"].([]interface{})) != 6 {
		t.Fatalf("payload = %#v, want six usable results", payload)
	}
	for i, chunk := range chunks {
		if chunk["url"] != fmt.Sprintf("https://example.com/%d", i) || chunk["content_with_weight"] != fmt.Sprintf("Snippet %d", i) {
			t.Fatalf("chunk %d = %#v", i, chunk)
		}
	}
}

func TestRetrieveAnySearchWebSearchReturnsEmptyPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":0,"data":{"results":[]}}`)
	}))
	defer server.Close()
	result, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, "", "ragflow")
	if err != nil {
		t.Fatal(err)
	}
	if len(result["chunks"].([]map[string]interface{})) != 0 || len(result["doc_aggs"].([]interface{})) != 0 {
		t.Fatalf("payload = %#v, want empty chunks and aggregates", result)
	}
}

func TestRetrieveAnySearchWebSearchReturnsSafeProviderFailures(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"credential-secret query-secret"}`)
			}))
			defer server.Close()
			_, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, "credential-secret", "query-secret")
			if err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) {
				t.Fatalf("error = %v, want safe HTTP status %d", err, status)
			}
			assertAnySearchErrorSafe(t, err)
			if calls != 1 {
				t.Fatalf("requests = %d, want no retry", calls)
			}
		})
	}
	for _, code := range []int{401, 429} {
		_, err := decodeAnySearchWebSearchResults([]byte(fmt.Sprintf(`{"code":%d,"message":"credential-secret query-secret","data":{"results":[]}}`, code)))
		if err == nil || !strings.Contains(err.Error(), fmt.Sprint(code)) {
			t.Fatalf("error = %v, want business code %d", err, code)
		}
		assertAnySearchErrorSafe(t, err)
	}
}

type anySearchTestTransport func(*http.Request) (*http.Response, error)

func (transport anySearchTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func assertAnySearchErrorSafe(t *testing.T, err error) {
	t.Helper()
	for _, secret := range []string{"credential-secret", "query-secret", "transport-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error exposed %q: %v", secret, err)
		}
	}
}

func TestRetrieveAnySearchWebSearchSanitizesTransportAndContextErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "transport", err: errors.New("transport-secret credential-secret query-secret")},
		{name: "canceled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "timeout", err: &net.DNSError{Err: "transport-secret", Name: "credential-secret", IsTimeout: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: anySearchTestTransport(func(*http.Request) (*http.Response, error) { return nil, tc.err })}
			_, err := retrieveAnySearchWebSearch(t.Context(), client, "https://example.com/credential-secret", "credential-secret", "query-secret")
			if err == nil {
				t.Fatal("error is nil")
			}
			assertAnySearchErrorSafe(t, err)
			if (tc.err == context.Canceled || tc.err == context.DeadlineExceeded) && !errors.Is(err, tc.err) {
				t.Fatalf("error %v does not preserve %v", err, tc.err)
			}
			if tc.name == "timeout" && !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("timeout category was lost: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := retrieveAnySearchWebSearch(ctx, &http.Client{}, "http://127.0.0.1:1", "credential-secret", "query-secret")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled request error = %v", err)
	}
	assertAnySearchErrorSafe(t, err)
	_, err = retrieveAnySearchWebSearch(t.Context(), &http.Client{}, "http://%credential-secret", "credential-secret", "query-secret")
	if err == nil {
		t.Fatal("invalid endpoint succeeded")
	}
	assertAnySearchErrorSafe(t, err)
}

type anySearchTestErrorBody struct {
	closed bool
}

func (*anySearchTestErrorBody) Read([]byte) (int, error) {
	return 0, errors.New("transport-secret credential-secret query-secret")
}

func (body *anySearchTestErrorBody) Close() error {
	body.closed = true
	return nil
}

func TestRetrieveAnySearchWebSearchSanitizesBodyReadFailure(t *testing.T) {
	body := &anySearchTestErrorBody{}
	client := &http.Client{Transport: anySearchTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	})}
	_, err := retrieveAnySearchWebSearch(t.Context(), client, "https://example.com", "credential-secret", "query-secret")
	if err == nil {
		t.Fatal("body read failure succeeded")
	}
	assertAnySearchErrorSafe(t, err)
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestRetrieveAnySearchWebSearchHonorsDeadlineAndBodyLimit(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		defer func() {
			close(release)
			server.Close()
		}()
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		_, err := retrieveAnySearchWebSearch(ctx, server.Client(), server.URL, "credential-secret", "query-secret")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline error = %v", err)
		}
		assertAnySearchErrorSafe(t, err)
	})
	t.Run("body limit", func(t *testing.T) {
		body := `{"code":0,"data":{"results":[]}}`
		for _, size := range []int{webSearchMaxResponseBytes, webSearchMaxResponseBytes + 1} {
			t.Run(fmt.Sprint(size), func(t *testing.T) {
				responseBody := body + strings.Repeat(" ", size-len(body))
				client := &http.Client{Transport: anySearchTestTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(responseBody))}, nil
				})}
				_, err := retrieveAnySearchWebSearch(t.Context(), client, "https://example.com", "", "q")
				if size == webSearchMaxResponseBytes && err != nil {
					t.Fatalf("response at limit failed: %v", err)
				}
				if size > webSearchMaxResponseBytes && (err == nil || !strings.Contains(err.Error(), "exceed")) {
					t.Fatalf("oversized response error = %v", err)
				}
			})
		}
	})
}

func TestRetrieveAnySearchWebSearchRefusesRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	for _, status := range []int{301, 302, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/credential-secret", status)
			}))
			defer server.Close()
			client := *anySearchWebSearchHTTPClient
			client.Transport = server.Client().Transport
			_, err := retrieveAnySearchWebSearch(t.Context(), &client, server.URL, "credential-secret", "query-secret")
			if err == nil {
				t.Fatal("redirect succeeded")
			}
			assertAnySearchErrorSafe(t, err)
		})
	}
	if targetCalls != 0 {
		t.Fatalf("redirect destination received %d requests", targetCalls)
	}
}

func TestAnySearchWebSearchUsesNormalConsumersWithoutTavilyFallback(t *testing.T) {
	original := anySearchWebSearchHTTPClient
	t.Cleanup(func() { anySearchWebSearchHTTPClient = original })
	calls := 0
	fail := false
	anySearchWebSearchHTTPClient = &http.Client{Transport: anySearchTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.anysearch.com/v1/search" || r.Header.Get("Authorization") != "Bearer anysearch-test" {
			t.Errorf("unexpected dispatched request: %s Authorization=%q", r.URL, r.Header.Get("Authorization"))
		}
		body := `{"code":0,"data":{"results":[{"url":"https://example.com/ragflow","title":"RAGFlow","content":"Native consumer text."}]}}`
		if fail {
			body = `{"code":401,"message":"credential-secret"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	config := map[string]interface{}{"web_search_provider": "anysearch", "anysearch_api_key": "anysearch-test", "tavily_api_key": "other-test"}
	provider := resolveWebSearchProvider(config)
	service := &ChatPipelineService{}
	researcher := &DeepResearcher{}
	for _, retrieve := range []func(context.Context, *webSearchProviderConfig, string) (map[string]interface{}, error){service.retrieveWebSearch, researcher.retrieveWebSearch} {
		result, err := retrieve(t.Context(), provider, "ragflow")
		if err != nil {
			t.Fatal(err)
		}
		assertWebSearchChunk(t, result, "anysearch-https://example.com/ragflow", "RAGFlow", "Native consumer text.")
	}
	callback := service.harnessWebSearchFunc(config)
	if callback == nil {
		t.Fatal("AnySearch harness callback is nil")
	}
	text, err := callback(t.Context(), []string{" ", "ragflow"})
	if err != nil || len(text) != 1 || text[0] != "Native consumer text." || calls != 3 {
		t.Fatalf("harness result=%#v error=%v requests=%d", text, err, calls)
	}
	agenticCallback := service.agenticWebSearch(config)
	if agenticCallback == nil {
		t.Fatal("AnySearch agentic callback is nil")
	}
	if service.agenticWebSearch(map[string]interface{}{"web_search_provider": "", "anysearch_api_key": "anysearch-test"}) != nil {
		t.Fatal("cleared provider retained an agentic callback")
	}
	hits, err := agenticCallback(t.Context(), "ragflow")
	if err != nil || len(hits) != 1 || hits[0].Title != "RAGFlow" || hits[0].URL != "https://example.com/ragflow" || hits[0].Content != "Native consumer text." || calls != 4 {
		t.Fatalf("agentic result=%#v error=%v requests=%d", hits, err, calls)
	}
	fail = true
	tavilyCalls := 0
	_, err = retrieveWebSearchWithTavily(t.Context(), provider, "ragflow", func(context.Context, string, string) (map[string]interface{}, error) {
		tavilyCalls++
		return nil, errors.New("unexpected Tavily fallback")
	})
	if err == nil || tavilyCalls != 0 {
		t.Fatalf("failure error=%v Tavily requests=%d", err, tavilyCalls)
	}
	text, err = callback(t.Context(), []string{"ragflow"})
	if err != nil || len(text) != 0 || calls != 6 {
		t.Fatalf("harness failure should skip one query: result=%#v error=%v requests=%d", text, err, calls)
	}
	hits, err = agenticCallback(t.Context(), "ragflow")
	if err == nil || len(hits) != 0 || calls != 7 {
		t.Fatalf("agentic failure result=%#v error=%v requests=%d", hits, err, calls)
	}
	assertAnySearchErrorSafe(t, err)
}
