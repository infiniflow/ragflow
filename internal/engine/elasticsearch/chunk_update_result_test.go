package elasticsearch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
)

func TestUpdateChunksRejectsIncompleteSuccessResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"complete", `{"updated":1,"timed_out":false,"version_conflicts":0,"failures":[]}`, false},
		{"timeout", `{"updated":1,"timed_out":true,"failures":[]}`, true},
		{"version_conflict", `{"updated":1,"version_conflicts":1,"failures":[]}`, true},
		{"item_failure", `{"updated":1,"failures":[{"cause":{"type":"es_rejected_execution_exception"}}]}`, true},
		{"malformed_result", `{`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{client: client}
			err = engine.UpdateChunks(t.Context(), map[string]any{"id": []string{"source-1"}}, map[string]any{"available_int": 0}, "ragflow_tenant", "kb-1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("update error=%v, want error=%t for HTTP 200 response %s", err, tc.wantErr, tc.body)
			}
		})
	}
}
