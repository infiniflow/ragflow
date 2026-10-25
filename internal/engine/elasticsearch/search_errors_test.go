package elasticsearch

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	elastic "github.com/elastic/go-elasticsearch/v8"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

type searchErrorTransport func(*http.Request) (*http.Response, error)

// RoundTrip supplies controlled responses and failures through the injected transport.
func (f searchErrorTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newSearchErrorEngine uses the injected transport and disables retries so
// each index query makes a single controlled request.
func newSearchErrorEngine(t *testing.T, transport searchErrorTransport) *Engine {
	t.Helper()
	if err := common.InitLogger("error", common.FileOutput{}, "elasticsearch_search_test"); err != nil {
		t.Fatal(err)
	}
	client, err := elastic.NewClient(elastic.Config{Addresses: []string{"http://search.test"}, Transport: transport, DisableRetry: true})
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{client: client}
}

// searchErrorResponse includes the product header required by the Elasticsearch
// client when accepting a synthetic backend response.
func searchErrorResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"X-Elastic-Product": []string{"Elasticsearch"}, "Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// TestSearchPropagatesIndexErrors forbids returning apparently complete results after any requested index fails.
func TestSearchPropagatesIndexErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		indexes []string
		status  int
		body    string
	}{
		{"backend", []string{"failed"}, 400, `{"error":{"reason":"controlled failure"},"status":400}`},
		{"decode", []string{"failed"}, 200, `not-json`},
		{"unrelated_404", []string{"failed"}, 404, `{"error":{"type":"route_not_found"},"status":404}`},
		{"partial", []string{"ok", "failed"}, 400, `{"error":{"reason":"controlled failure"},"status":400}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Path, "/ok/") {
					return searchErrorResponse(200, `{"hits":{"total":{"value":1},"hits":[{"_id":"one","_source":{"id":"one"}}]}}`), nil
				}
				return searchErrorResponse(test.status, test.body), nil
			})
			result, err := engine.Search(t.Context(), &types.SearchRequest{IndexNames: test.indexes, Limit: 10})
			if err == nil || result != nil || !strings.Contains(err.Error(), "failed") {
				t.Fatalf("result=%+v err=%v, want scoped error and no partial result", result, err)
			}
		})
	}
}

// TestSearchPreservesTransportError lets callers distinguish transport failures through errors.Is.
func TestSearchPreservesTransportError(t *testing.T) {
	controlled := errors.New("controlled transport failure")
	engine := newSearchErrorEngine(t, func(*http.Request) (*http.Response, error) { return nil, controlled })
	result, err := engine.Search(t.Context(), &types.SearchRequest{IndexNames: []string{"failed"}, Limit: 10})
	if result != nil || !errors.Is(err, controlled) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

// TestSearchSuccessfulIndicesRemainAggregated checks fresh request bodies and a genuine empty response.
func TestSearchSuccessfulIndicesRemainAggregated(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "hits", true: "empty"}[empty], func(t *testing.T) {
			requests := 0
			engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				requests++
				if empty {
					return searchErrorResponse(200, `{"hits":{"total":{"value":0},"hits":[]}}`), nil
				}
				return searchErrorResponse(200, `{"hits":{"total":{"value":1},"hits":[{"_id":"one","_source":{"id":"one"}}]}}`), nil
			})
			result, err := engine.Search(t.Context(), &types.SearchRequest{IndexNames: []string{"first", "second"}, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if empty {
				want = 0
			}
			if requests != 2 || len(result.Chunks) != want || result.Total != int64(want) {
				t.Fatalf("requests=%d result=%+v", requests, result)
			}
		})
	}
}

// TestSearchMissingIndexRemainsEmpty preserves the unindexed-tenant case alongside populated indices.
func TestSearchMissingIndexRemainsEmpty(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "mixed"}[mixed], func(t *testing.T) {
			engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Path, "/missing/") {
					return searchErrorResponse(404, `{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`), nil
				}
				return searchErrorResponse(200, `{"hits":{"total":{"value":1},"hits":[{"_id":"one","_source":{"id":"one"}}]}}`), nil
			})
			names := []string{"missing"}
			want := 0
			if mixed {
				names = append(names, "populated")
				want = 1
			}
			result, err := engine.Search(t.Context(), &types.SearchRequest{IndexNames: names, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Chunks) != want || result.Total != int64(want) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

// TestSearchPreservesResponseReadError rejects a truncated error response even when its prefix names a missing index.
func TestSearchPreservesResponseReadError(t *testing.T) {
	controlled := errors.New("controlled response read failure")
	engine := newSearchErrorEngine(t, func(*http.Request) (*http.Response, error) {
		response := searchErrorResponse(404, "")
		response.Body = io.NopCloser(io.MultiReader(strings.NewReader(`{"error":{"type":"index_not_found_exception"}}`), failedSearchReader{controlled}))
		return response, nil
	})
	result, err := engine.Search(t.Context(), &types.SearchRequest{IndexNames: []string{"failed"}, Limit: 10})
	if result != nil || !errors.Is(err, controlled) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type failedSearchReader struct{ err error }

// Read surfaces the injected error after any preceding response prefix has been read.
func (r failedSearchReader) Read([]byte) (int, error) { return 0, r.err }
