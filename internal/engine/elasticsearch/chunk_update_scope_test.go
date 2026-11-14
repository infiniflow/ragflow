package elasticsearch

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/engine/types"

	"github.com/elastic/go-elasticsearch/v8"
)

// TestUpdateSingleChunkPreservesScope checks the actual outgoing ES query,
// without substituting the engine or relying on a real ES service. Empty hits
// deliberately stop execution before any write; this test proves lost query
// constraints, not a mutation of a real Elasticsearch document.
func TestUpdateSingleChunkPreservesScope(t *testing.T) {
	for _, tt := range []struct {
		name      string
		condition map[string]interface{}
		wantTerms map[string]string
	}{
		{
			name:      "dataset",
			condition: map[string]interface{}{"id": "chunk-b"},
			wantTerms: map[string]string{"id": "chunk-b", "kb_id": "kb-a"},
		},
		{
			name:      "dataset_and_document",
			condition: map[string]interface{}{"id": "chunk-b", "doc_id": "doc-a"},
			wantTerms: map[string]string{"id": "chunk-b", "kb_id": "kb-a", "doc_id": "doc-a"},
		},
		{
			name:      "additional_condition",
			condition: map[string]interface{}{"id": "chunk-b", "type_kwd": "wiki_page"},
			wantTerms: map[string]string{"id": "chunk-b", "kb_id": "kb-a", "type_kwd": "wiki_page"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var queries []map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodHead && r.URL.Path == "/ragflow_scope_test":
					w.WriteHeader(http.StatusOK)
				case r.URL.Path == "/ragflow_scope_test/_search":
					var payload map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode ES search request: %v", err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					queries = append(queries, payload)
					_, _ = w.Write([]byte(`{"hits":{"total":{"value":0,"relation":"eq"},"hits":[]}}`))
				default:
					t.Errorf("unexpected ES request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{client: client}
			ctx := t.Context()
			got, err := engine.GetChunk(ctx, "ragflow_scope_test", "chunk-b", []string{"kb-a"})
			if err != nil || got != nil {
				t.Fatalf("GetChunk = (%v, %v), want (nil, nil) for empty hits", got, err)
			}
			err = engine.UpdateChunks(ctx, tt.condition,
				map[string]interface{}{"available_int": 0, "remove": "tag_feas"}, "ragflow_scope_test", "kb-a")
			if !errors.Is(err, types.ErrDocumentNotFound) {
				t.Fatalf("UpdateChunks = %v, want ErrDocumentNotFound for empty hits", err)
			}
			if len(queries) != 2 {
				t.Fatalf("search requests = %d, want read and update lookup", len(queries))
			}
			readJSON, _ := json.Marshal(queries[0])
			updateJSON, _ := json.Marshal(queries[1])
			t.Logf("read lookup: %s", readJSON)
			t.Logf("update lookup: %s", updateJSON)
			for field, want := range tt.wantTerms {
				if !scopeQueryHasTerm(queries[1]["query"], field, want) {
					t.Errorf("update lookup lost required term %s=%q: %s", field, want, updateJSON)
				}
			}
			if !scopeQueryHasTerm(queries[0]["query"], "kb_id", "kb-a") {
				t.Errorf("read lookup lost dataset scope: %s", readJSON)
			}
		})
	}
}

func TestUpdateSingleChunkRejectsUnsupportedCondition(t *testing.T) {
	for _, tt := range []struct {
		name, field string
		value       interface{}
	}{
		{"nil", "doc_id", nil},
		{"object", "doc_id", map[string]interface{}{"value": "doc-a"}},
		{"and", "and", []interface{}{}},
		{"or", "or", []interface{}{}},
		{"must_not", "must_not", map[string]interface{}{"doc_id": "doc-a"}},
		{"exists", "exists", "doc_id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// A nil client also ensures rejection happens before any ES request.
			engine := &Engine{}
			err := engine.updateSingleChunk(t.Context(), "ragflow_scope_test", "chunk-b",
				map[string]interface{}{"id": "chunk-b", "kb_id": "kb-a", tt.field: tt.value},
				map[string]interface{}{"available_int": 0})
			if err == nil || !strings.Contains(err.Error(), "unsupported single chunk update condition") {
				t.Fatalf("error = %v, want unsupported condition rejection", err)
			}
		})
	}
}

// Only descend through conjunctions: a term in an optional should or a must_not
// clause would not enforce the requested scope.
func scopeQueryHasTerm(value interface{}, field, want string) bool {
	query, ok := value.(map[string]interface{})
	if !ok {
		return false
	}
	if term, ok := query["term"].(map[string]interface{}); ok && term[field] == want {
		return true
	}
	if conjunction, ok := query["bool"].(map[string]interface{}); ok {
		for _, key := range []string{"must", "filter"} {
			if scopeQueryHasTerm(conjunction[key], field, want) {
				return true
			}
			clauses, _ := conjunction[key].([]interface{})
			for _, clause := range clauses {
				if scopeQueryHasTerm(clause, field, want) {
					return true
				}
			}
		}
	}
	return false
}
