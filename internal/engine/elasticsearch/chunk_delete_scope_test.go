package elasticsearch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
)

// TestDeleteChunksScope checks the outgoing _delete_by_query: memory indexes
// are scoped by memory_id (messages carry no kb_id), datasets by kb_id.
func TestDeleteChunksScope(t *testing.T) {
	for _, tt := range []struct {
		name, index, id, wantField string
		noField                    string
	}{
		{name: "memory", index: "memory_t1", id: "mem-a", wantField: "memory_id", noField: "kb_id"},
		{name: "dataset", index: "ragflow_t1", id: "kb-a", wantField: "kb_id", noField: "memory_id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var query interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodHead && r.URL.Path == "/"+tt.index:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPost && r.URL.Path == "/"+tt.index+"/_delete_by_query":
					var payload map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode request: %v", err)
					}
					query = payload["query"]
					_, _ = w.Write([]byte(`{"deleted":2}`))
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
			n, err := engine.DeleteChunks(t.Context(), map[string]interface{}{}, tt.index, tt.id)
			if err != nil || n != 2 {
				t.Fatalf("DeleteChunks = (%d, %v), want (2, nil)", n, err)
			}
			raw, _ := json.Marshal(query)
			if !scopeQueryHasTerm(query, tt.wantField, tt.id) {
				t.Errorf("delete query lost %s=%q: %s", tt.wantField, tt.id, raw)
			}
			if scopeQueryHasTerm(query, tt.noField, tt.id) {
				t.Errorf("delete query unexpectedly has %s: %s", tt.noField, raw)
			}
		})
	}
}
