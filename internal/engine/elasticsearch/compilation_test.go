package elasticsearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

func TestCompilationSearchReadsBothGenerationsInOneRequest(t *testing.T) {
	if err := common.InitLogger("info", common.FileOutput{}, "compilation_test"); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]interface{}{
		{"id": "new", "kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_page"},
		{"id": "old", "kb_id": "kb1", "compile_kwd": "wiki_page"},
		{"id": "section", "kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_section"},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad query", 400)
			return
		}
		var hits []map[string]interface{}
		for _, row := range rows {
			if matchesCompilationQuery(body["query"].(map[string]interface{}), row) {
				hits = append(hits, map[string]interface{}{"_id": row["id"], "_source": row})
			}
		}
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"hits": map[string]interface{}{"total": map[string]interface{}{"value": len(hits)}, "hits": hits}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{client: client}
	result, err := eng.Search(t.Context(), &types.SearchRequest{IndexNames: []string{"ragflow_t1"}, KbIDs: []string{"kb1"}, Limit: 10, Filter: map[string]interface{}{"type_kwd": "wiki_page"}})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || result.Total != 2 || len(result.Chunks) != 2 {
		t.Fatalf("requests=%d, result=%+v; want one request returning new and old pages", requests.Load(), result)
	}
}

func TestCompilationQueryMatchesNewAndOldRows(t *testing.T) {
	for _, test := range []struct {
		name        string
		filter, row map[string]interface{}
		want        bool
	}{
		{"new page", map[string]interface{}{"compile_kwd": "wiki_page"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_page"}, true},
		{"old page", map[string]interface{}{"compile_kwd": "wiki_page"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki_page"}, true},
		{"section excluded", map[string]interface{}{"compile_kwd": "wiki_page"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_section"}, false},
		{"tenant dataset scope", map[string]interface{}{"compile_kwd": "wiki_page"}, map[string]interface{}{"kb_id": "kb2", "compile_kwd": "wiki", "type_kwd": "wiki_page"}, false},
		{"old graph", map[string]interface{}{"compile_kwd": "graph"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "hypergraph", "compilation_template_kind_kwd": "knowledge_graph"}, true},
		{"canonical row", map[string]interface{}{"compilation_template_kind_kwd": "timeline"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "graph"}, false},
		{"old inferred timeline preserves graph", map[string]interface{}{"compile_kwd": "graph"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "timeline", "compilation_template_kind_kwd": "knowledge_graph"}, true},
		{"old inferred timeline is not a timeline template", map[string]interface{}{"compile_kwd": "timeline"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "timeline", "compilation_template_kind_kwd": "knowledge_graph"}, false},
		{"blank stamp", map[string]interface{}{"compile_kwd": "graph"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "graph", "compilation_template_kind_kwd": ""}, true},
		{"navigation cleanup", map[string]interface{}{"compile_kwd": "dataset_nav"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "page_index", "type_kwd": "nav_cluster"}, true},
		{"structure survives nav cleanup", map[string]interface{}{"compile_kwd": "dataset_nav"}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "page_index", "type_kwd": "entity"}, false},
		{"wiki graph cleanup", map[string]interface{}{"compile_kwd": []string{"wiki_entity", "wiki_relation"}}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_relation"}, true},
		{"page survives graph cleanup", map[string]interface{}{"compile_kwd": []string{"wiki_entity", "wiki_relation"}}, map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_page"}, false},
		{"mind map type reads old spelling", map[string]interface{}{"entity_type_kwd": "mind_map"}, map[string]interface{}{"kb_id": "kb1", "entity_type_kwd": "mindmap"}, true},
		{"mind map type reads new spelling", map[string]interface{}{"entity_type_kwd": "mindmap"}, map[string]interface{}{"kb_id": "kb1", "entity_type_kwd": "mind_map"}, true},
		{"mind map type remains scoped", map[string]interface{}{"entity_type_kwd": "mind_map"}, map[string]interface{}{"kb_id": "kb2", "entity_type_kwd": "mindmap"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := buildBoolQueryFromCondition(test.filter, []string{"kb1"}, false, false)
			if got := matchesCompilationQuery(query, test.row); got != test.want {
				t.Fatalf("query %v matched = %v, want %v", query, got, test.want)
			}
		})
	}
}

func matchesCompilationQuery(query, row map[string]interface{}) bool {
	if clause, ok := query["bool"].(map[string]interface{}); ok {
		for _, field := range []string{"must", "filter"} {
			for _, child := range compilationQueryChildren(clause[field]) {
				if !matchesCompilationQuery(child, row) {
					return false
				}
			}
		}
		for _, child := range compilationQueryChildren(clause["must_not"]) {
			if matchesCompilationQuery(child, row) {
				return false
			}
		}
		minimum := 0
		switch value := clause["minimum_should_match"].(type) {
		case int:
			minimum = value
		case float64:
			minimum = int(value)
		}
		if minimum > 0 {
			matched := 0
			for _, child := range compilationQueryChildren(clause["should"]) {
				if matchesCompilationQuery(child, row) {
					matched++
				}
			}
			if matched < minimum {
				return false
			}
		}
		return true
	}
	if clause, ok := query["exists"].(map[string]interface{}); ok {
		return row[clause["field"].(string)] != nil
	}
	for _, operator := range []string{"term", "terms"} {
		if clause, ok := query[operator].(map[string]interface{}); ok {
			for field, expected := range clause {
				if items, ok := conditionTermsList(expected); ok {
					matched := false
					for _, value := range items {
						matched = matched || fmt.Sprint(row[field]) == fmt.Sprint(value)
					}
					if !matched {
						return false
					}
				} else if fmt.Sprint(row[field]) != fmt.Sprint(expected) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func compilationQueryChildren(value interface{}) []map[string]interface{} {
	if children, ok := value.([]map[string]interface{}); ok {
		return children
	}
	var out []map[string]interface{}
	if children, ok := value.([]interface{}); ok {
		for _, child := range children {
			out = append(out, child.(map[string]interface{}))
		}
	}
	return out
}
