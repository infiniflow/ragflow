package elasticsearch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

func TestBuildQueryStringQueryMapsSkillFieldsToTokenFields(t *testing.T) {
	query := buildQueryStringQuery(&types.MatchTextExpr{
		MatchingText: "test",
		Fields:       []string{"name^10", "tags^5", "description^3", "content^1"},
	}, 0, true, false)

	queryString, ok := query["query_string"].(map[string]interface{})
	if !ok {
		t.Fatalf("query_string missing from %#v", query)
	}
	assertEqual(t, queryString["fields"], []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks^1"})
	assertEqual(t, queryString["query"], "test")
}

func TestBuildQueryStringQueryKeepsDocumentFieldsUnchanged(t *testing.T) {
	query := buildQueryStringQuery(&types.MatchTextExpr{
		MatchingText: "test",
		Fields:       []string{"name^10"},
	}, 0, false, false)

	queryString, ok := query["query_string"].(map[string]interface{})
	if !ok {
		t.Fatalf("query_string missing from %#v", query)
	}
	assertEqual(t, queryString["fields"], []string{"name^10"})
}

func TestSearchUsesConfiguredKNNNumCandidates(t *testing.T) {
	if err := common.InitLogger("info", common.FileOutput{}, "elasticsearch_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	var searchQuery map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&searchQuery); err != nil {
			t.Errorf("decode search query: %v", err)
		}
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":0},"hits":[]}}`))
	}))
	defer server.Close()

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	engine := &Engine{client: client}
	_, err = engine.Search(t.Context(), &types.SearchRequest{
		IndexNames: []string{"ragflow_tenant"},
		Limit:      30,
		MatchExprs: []interface{}{&types.MatchDenseExpr{
			VectorColumnName: "q_2_vec",
			EmbeddingData:    []float64{0.1, 0.2},
			TopN:             128,
			ExtraOptions:     map[string]interface{}{"num_candidates": 4096},
		}},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	knn, ok := searchQuery["knn"].(map[string]interface{})
	if !ok {
		t.Fatalf("KNN query missing from %#v", searchQuery)
	}
	if knn["k"] != float64(128) || knn["num_candidates"] != float64(4096) {
		t.Fatalf("KNN parameters = (%v, %v), want (128, 4096)", knn["k"], knn["num_candidates"])
	}
}

func TestUpdateSingleMemoryMessageWaitsForRefresh(t *testing.T) {
	var gotRefresh string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/memory_tenant/_update/memory-1_42" {
			t.Errorf("path=%s, want /memory_tenant/_update/memory-1_42", r.URL.Path)
			http.Error(w, "unexpected request path", http.StatusNotFound)
			return
		}
		gotRefresh = r.URL.Query().Get("refresh")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"updated"}`))
	}))
	defer server.Close()

	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	ctx := t.Context()
	engine := &Engine{client: client}
	if err = engine.updateSingleMemoryMessage(ctx, "memory_tenant", "memory-1_42", map[string]interface{}{"forget_at": "2026-07-27 10:00:00"}); err != nil {
		t.Fatalf("updateSingleMemoryMessage: %v", err)
	}
	if gotRefresh != "wait_for" {
		t.Fatalf("refresh=%q, want wait_for", gotRefresh)
	}
}

func TestDeleteChunksPreservesStringSliceCondition(t *testing.T) {
	var deleteQuery map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			if r.URL.Path != "/ragflow_tenant/_delete_by_query" {
				t.Errorf("path=%s, want /ragflow_tenant/_delete_by_query", r.URL.Path)
				http.Error(w, "unexpected request path", http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&deleteQuery); err != nil {
				t.Errorf("decode delete query: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"deleted":3}`))
		default:
			t.Errorf("method=%s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	ctx := t.Context()
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	engine := &Engine{client: client}
	deleted, err := engine.DeleteChunks(ctx, map[string]interface{}{
		"kb_id":       "kb-1",
		"compile_kwd": []string{"wiki_entity", "wiki_relation"},
	}, "ragflow_tenant", "kb-1")
	if err != nil {
		t.Fatalf("DeleteChunks: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted=%d, want 3", deleted)
	}

	query, ok := deleteQuery["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("delete query missing query: %#v", deleteQuery)
	}
	boolQuery, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("delete query missing bool: %#v", query)
	}
	must, ok := boolQuery["must"].([]interface{})
	if !ok || len(must) != 1 {
		t.Fatalf("delete query must=%#v, want one terms clause", boolQuery["must"])
	}
	terms := must[0].(map[string]interface{})["terms"].(map[string]interface{})
	assertEqual(t, terms["compile_kwd"], []interface{}{"wiki_entity", "wiki_relation"})
}

// TestDeleteChunksIDStringSlice guards against the id filter being dropped when
// the caller passes a typed []string. Without a dedicated []string branch in
// the id handling, the generic loop skips the "id" key and DeleteChunks would
// build a match_all query and delete every document in the index.
func TestDeleteChunksIDStringSlice(t *testing.T) {
	var deleteQuery map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			if r.URL.Path != "/ragflow_tenant/_delete_by_query" {
				t.Errorf("path=%s, want /ragflow_tenant/_delete_by_query", r.URL.Path)
				http.Error(w, "unexpected request path", http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&deleteQuery); err != nil {
				t.Errorf("decode delete query: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"deleted":2}`))
		default:
			t.Errorf("method=%s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	ctx := t.Context()
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	engine := &Engine{client: client}
	if _, err = engine.DeleteChunks(ctx, map[string]interface{}{
		"id": []string{"doc-a", "doc-b"},
	}, "ragflow_tenant", "kb-1"); err != nil {
		t.Fatalf("DeleteChunks: %v", err)
	}

	query, ok := deleteQuery["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("delete query missing query: %#v", deleteQuery)
	}
	boolQuery, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("delete query missing bool: %#v", query)
	}
	must, ok := boolQuery["must"].([]interface{})
	if !ok || len(must) != 1 {
		t.Fatalf("delete query must=%#v, want one id terms clause (match_all would be a bug)", boolQuery["must"])
	}
	terms, ok := must[0].(map[string]interface{})["terms"].(map[string]interface{})
	if !ok {
		t.Fatalf("must[0] missing terms clause: %#v", must[0])
	}
	assertEqual(t, terms["id"], []interface{}{"doc-a", "doc-b"})
}

func TestElasticsearchGetFieldsFiltersAndUsesIDFallback(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{
			"_id":                 "fallback-chunk",
			"docnm_kwd":           []interface{}{"guide.md"},
			"content_with_weight": "Alpha beta body.",
			"available_int":       float64(1),
			"ignored":             "not requested",
		},
	}

	got := engine.GetFields(chunks, []string{"id", "docnm_kwd", "content_with_weight", "available_int"})
	fieldMap, ok := got["fallback-chunk"]
	if !ok {
		t.Fatalf("GetFields keys=%v, want fallback-chunk", got)
	}

	assertEqual(t, fieldMap["id"], "fallback-chunk")
	assertEqual(t, fieldMap["docnm_kwd"], "guide.md")
	assertEqual(t, fieldMap["content_with_weight"], "Alpha beta body.")
	assertEqual(t, fieldMap["available_int"], float64(1))

	if _, ok := fieldMap["ignored"]; ok {
		t.Fatalf("field filter leaked unrequested field: %#v", fieldMap)
	}
}

func TestElasticsearchGetFieldsEmptyAndSkippedIDs(t *testing.T) {
	engine := &Engine{}

	if got := engine.GetFields(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("GetFields(nil)=%#v, want empty non-nil map", got)
	}

	got := engine.GetFields([]map[string]interface{}{
		{"id": "chunk-1", "docnm_kwd": "doc.md"},
		{"id": "", "_id": "fallback-chunk", "docnm_kwd": "fallback.md"},
		{"docnm": "missing-id.md"},
	}, []string{"id", "docnm_kwd"})

	fieldMap, ok := got["chunk-1"]
	if !ok {
		t.Fatalf("GetFields keys=%v, want chunk-1", got)
	}
	assertEqual(t, fieldMap["id"], "chunk-1")
	assertEqual(t, fieldMap["docnm_kwd"], "doc.md")

	if _, ok := got["missing-id.md"]; ok {
		t.Fatalf("chunk without id should be skipped: %#v", got)
	}
	fallbackMap, ok := got["fallback-chunk"]
	if !ok {
		t.Fatalf("GetFields keys=%v, want fallback-chunk", got)
	}

	assertEqual(t, fallbackMap["id"], "fallback-chunk")
	assertEqual(t, fallbackMap["docnm_kwd"], "fallback.md")
}

func TestElasticsearchGetAggregationSplitsCountsAndSorts(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"tag_kwd": "red###blue###"},
		{"tag_kwd": []interface{}{"blue", " green ", ""}},
		{"tag_kwd": "blue"},
		{"tag_kwd": ""},
		{},
	}

	got := engine.GetAggregation(chunks, "tag_kwd")
	assertEqual(t, aggregationCounts(t, got), map[string]int{"blue": 3, "red": 1, "green": 1})
	if len(got) == 0 || got[0]["key"] != "blue" || got[0]["count"] != 3 {
		t.Fatalf("first aggregation=%#v, want blue count 3", got)
	}
	if len(got) != 3 || got[1]["key"] != "green" || got[2]["key"] != "red" {
		t.Fatalf("tie ordering=%#v, want green before red", got)
	}

	docAgg := engine.GetAggregation([]map[string]interface{}{
		{"docnm_kwd": "guide.md, api.md"},
		{"docnm_kwd": "guide.md"},
	}, "docnm_kwd")
	assertEqual(t, aggregationCounts(t, docAgg), map[string]int{"guide.md": 2, "api.md": 1})

	if got := engine.GetAggregation(chunks, "missing_kwd"); got == nil || len(got) != 0 {
		t.Fatalf("missing aggregation=%#v, want empty non-nil slice", got)
	}
}

func TestElasticsearchGetChunkIDsPreservesOrderWithFallback(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"id": "source-id", "_id": "hit-id"},
		{"_id": "fallback-id"},
		{"id": ""},
		{"id": 42},
		{"id": "last-id"},
	}

	got := engine.GetChunkIDs(chunks)
	assertEqual(t, got, []string{"source-id", "fallback-id", "last-id"})

	if got := engine.GetChunkIDs(nil); got == nil || len(got) != 0 {
		t.Fatalf("GetChunkIDs(nil)=%#v, want empty non-nil slice", got)
	}
}

func TestElasticsearchGetHighlightFallbackAndBoundaries(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{
			"_id":     "fallback-id",
			"content": "Alpha beta.\nbetamax soup. BETA again!",
		},
	}

	got := engine.GetHighlight(chunks, []string{"beta"}, "content_with_weight")
	assertEqual(t, got, map[string]string{
		"fallback-id": "Alpha <em>beta</em>... <em>BETA</em> again",
	})
	if gotText := got["fallback-id"]; strings.Contains(gotText, "<em>beta</em>max") {
		t.Fatalf("highlight matched inside a larger token: %q", gotText)
	}

	gotLaterFallback := engine.GetHighlight([]map[string]interface{}{
		{"_id": "first"},
		{"_id": "second", "content": "Gamma beta."},
	}, []string{"beta"}, "content_with_weight")
	assertEqual(t, gotLaterFallback, map[string]string{
		"second": "Gamma <em>beta</em>",
	})

	gotMixedFallback := engine.GetHighlight([]map[string]interface{}{
		{"id": "weighted", "content_with_weight": "Weighted beta."},
		{"id": "plain", "content": "Plain beta."},
	}, []string{"beta"}, "content_with_weight")
	assertEqual(t, gotMixedFallback, map[string]string{
		"plain":    "Plain <em>beta</em>",
		"weighted": "Weighted <em>beta</em>",
	})

	gotEmptyFallback := engine.GetHighlight([]map[string]interface{}{
		{"id": "empty-weighted", "content_with_weight": "", "content": "Empty fallback beta."},
	}, []string{"beta"}, "content_with_weight")
	assertEqual(t, gotEmptyFallback, map[string]string{
		"empty-weighted": "Empty fallback <em>beta</em>",
	})

	gotInvalidFallback := engine.GetHighlight([]map[string]interface{}{
		{"id": "invalid-weighted", "content_with_weight": nil, "content": "Invalid fallback beta."},
	}, []string{"beta"}, "content_with_weight")
	assertEqual(t, gotInvalidFallback, map[string]string{
		"invalid-weighted": "Invalid fallback <em>beta</em>",
	})
}

func TestElasticsearchGetHighlightPreservesExistingAndNonEnglish(t *testing.T) {
	engine := &Engine{}

	gotExisting := engine.GetHighlight([]map[string]interface{}{
		{"id": "existing", "content_with_weight": "already <em>marked</em> text"},
	}, []string{"marked"}, "content_with_weight")
	assertEqual(t, gotExisting, map[string]string{"existing": "already <em>marked</em> text"})

	gotNonEnglish := engine.GetHighlight([]map[string]interface{}{
		{"id": "cn", "content_with_weight": "这是世界。你好世界"},
	}, []string{"世界"}, "content_with_weight")
	assertEqual(t, gotNonEnglish, map[string]string{
		"cn": "这是<em>世界</em>。你好<em>世界</em>",
	})

	gotOverlapping := engine.GetHighlight([]map[string]interface{}{
		{"id": "overlap", "content_with_weight": "世界和世"},
	}, []string{"世", "世界", "世界"}, "content_with_weight")
	assertEqual(t, gotOverlapping, map[string]string{
		"overlap": "<em>世界</em>和<em>世</em>",
	})

	if got := engine.GetHighlight([]map[string]interface{}{{"id": "x"}}, []string{"x"}, "content_with_weight"); got == nil || len(got) != 0 {
		t.Fatalf("missing field highlight=%#v, want empty non-nil map", got)
	}
	if got := engine.GetHighlight([]map[string]interface{}{{"id": "x", "content": "x"}}, nil, "content"); got == nil || len(got) != 0 {
		t.Fatalf("empty keyword highlight=%#v, want empty non-nil map", got)
	}
}

func aggregationCounts(t *testing.T, aggregation []map[string]interface{}) map[string]int {
	t.Helper()

	counts := make(map[string]int, len(aggregation))
	for _, item := range aggregation {
		key, ok := item["key"].(string)
		if !ok {
			t.Fatalf("aggregation key type=%T in %#v", item["key"], item)
		}
		count, ok := item["count"].(int)
		if !ok {
			t.Fatalf("aggregation count type=%T in %#v", item["count"], item)
		}
		counts[key] = count
	}
	return counts
}

func assertEqual(t *testing.T, got, want interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// TestMapMemoryMessageToESDoc: logical message fields are mapped to the
// storage document at insert time — message_type/status collapse to
// their storage counterparts and content is tokenized before write,
// mirroring Python memory/utils/es_conn.py:map_message_to_es_fields.
func TestMapMemoryMessageToESDoc(t *testing.T) {
	doc := mapMemoryMessageToESDoc(map[string]interface{}{
		"id":           "mem-1_42",
		"doc_id":       "mem-1",
		"message_id":   int64(42),
		"message_type": "raw",
		"source_id":    0,
		"memory_id":    "mem-1",
		"agent_id":     "a1",
		"content":      "User Input: hello world\nAgent Response: hi there",
		"status":       true,
		"forget_at":    nil,
	})

	assertEqual(t, doc["message_type_kwd"], "raw")
	if _, ok := doc["message_type"]; ok {
		t.Fatalf("message_type must collapse into message_type_kwd, got %#v", doc)
	}
	assertEqual(t, doc["status_int"], 1)
	if _, ok := doc["status"]; ok {
		t.Fatalf("status must collapse into status_int, got %#v", doc)
	}

	raw := "User Input: hello world\nAgent Response: hi there"
	assertEqual(t, doc["content_ltks"], raw)
	tokenized, ok := doc["tokenized_content_ltks"].(string)
	if !ok || tokenized == "" {
		t.Fatalf("tokenized_content_ltks = %#v, want non-empty tokenized string", doc["tokenized_content_ltks"])
	}
	if _, ok := doc["content"]; ok {
		t.Fatalf("content must not be stored verbatim, got %#v", doc)
	}

	// Untouched fields pass through.
	assertEqual(t, doc["id"], "mem-1_42")
	assertEqual(t, doc["doc_id"], "mem-1")
	assertEqual(t, doc["message_id"], int64(42))
	assertEqual(t, doc["agent_id"], "a1")
	if v, ok := doc["forget_at"]; !ok || v != nil {
		t.Fatalf("forget_at = %#v, want explicit nil", v)
	}
}

// TestMapMemoryMessageESUpdateFieldsRefreshesTokens: writing content via
// the update path recomputes tokenized_content_ltks before write, same
// as the Python update path.
func TestMapMemoryMessageESUpdateFieldsRefreshesTokens(t *testing.T) {
	doc := mapMemoryMessageESUpdateFields(map[string]interface{}{
		"content": "fresh content",
		"status":  0,
	})

	assertEqual(t, doc["content_ltks"], "fresh content")
	tokenized, ok := doc["tokenized_content_ltks"].(string)
	if !ok || tokenized == "" {
		t.Fatalf("tokenized_content_ltks = %#v, want refreshed tokenized string", doc["tokenized_content_ltks"])
	}
	assertEqual(t, doc["status_int"], 0)
}
