package opensearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	opensearch "github.com/opensearch-project/opensearch-go/v4"
	"ragflow/internal/engine/types"
)

func TestChunkScanReadsBeyondResultWindowAndClearsCursor(t *testing.T) {
	const total = 10001
	offset := 0
	cleared := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.Method == http.MethodDelete {
			ids, _ := body["scroll_id"].([]interface{})
			cleared = r.URL.Path == "/_search/scroll" && len(ids) == 1 && ids[0] == fmt.Sprint(offset)
			fmt.Fprint(w, `{"succeeded":true}`)
			return
		}
		if r.URL.Path == "/ragflow_tenant/_search" {
			sort, _ := body["sort"].([]interface{})
			if r.URL.Query().Get("scroll") != "1m" || body["from"] != nil || len(sort) != 1 || sort[0] != "_doc" || body["_source"] == nil || body["query"] == nil {
				t.Errorf("bad initial scan: %s %+v", r.URL, body)
			}
		} else if r.URL.Path != "/_search/scroll" || body["scroll_id"] != fmt.Sprint(offset) {
			t.Errorf("bad scroll continuation: %s %+v", r.URL, body)
		}
		end := min(offset+1000, total)
		hits := make([]map[string]interface{}, 0, end-offset)
		for ; offset < end; offset++ {
			hits = append(hits, map[string]interface{}{"_id": fmt.Sprintf("row-%d", offset), "_source": map[string]interface{}{"name_kwd": "entity"}})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"_scroll_id": fmt.Sprint(offset), "hits": map[string]interface{}{"hits": hits}})
	}))
	defer server.Close()
	client, err := opensearch.NewClient(opensearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	e := &Engine{client: client}
	got, err := e.Search(t.Context(), &types.SearchRequest{
		IndexNames: []string{"ragflow_tenant"}, KbIDs: []string{"kb"},
		SelectFields: []string{"name_kwd"}, Filter: map[string]interface{}{"knowledge_graph_kwd": "entity"},
		ScanAll: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != total || len(got.Chunks) != total || got.Chunks[total-1]["id"] != "row-10000" || !cleared {
		t.Fatalf("rows=%d total=%d cursor cleared=%t", len(got.Chunks), got.Total, cleared)
	}
}
