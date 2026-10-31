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
	}, true, false)

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
	}, false, false)

	queryString, ok := query["query_string"].(map[string]interface{})
	if !ok {
		t.Fatalf("query_string missing from %#v", query)
	}
	assertEqual(t, queryString["fields"], []string{"name^10"})
}

func TestBuildQueryStringQueryLowercasesMatchingText(t *testing.T) {
	// The *_tks/*_ltks fields are whitespace-analyzed and store lowercase
	// tokens; a capitalized query term must not silently match nothing.
	query := buildQueryStringQuery(&types.MatchTextExpr{
		MatchingText: "Isabel Wood co-lead Ross Feldner Bob Musil Bird Watch Wonder Program",
	}, false, false)

	queryString, ok := query["query_string"].(map[string]interface{})
	if !ok {
		t.Fatalf("query_string missing from %#v", query)
	}
	assertEqual(t, queryString["query"],
		"isabel wood co-lead ross feldner bob musil bird watch wonder program")
}

func TestBuildQueryStringQueryKeepsBooleanOperators(t *testing.T) {
	// Only AND/OR/NOT are operators, and only in upper case: a folded "or" is a
	// term clause, which re-bases minimum_should_match and turns a 94-hit search
	// into a 32-hit one (see lowerCaseQueryText).
	query := buildQueryStringQuery(&types.MatchTextExpr{
		MatchingText: `(病毒 OR 勒索)^0.3 (系统)^0.2 ("系统 因为 勒索"~2)^1.5 OR ("MES"^0.7)`,
	}, false, false)

	queryString, ok := query["query_string"].(map[string]interface{})
	if !ok {
		t.Fatalf("query_string missing from %#v", query)
	}
	assertEqual(t, queryString["query"],
		`(病毒 OR 勒索)^0.3 (系统)^0.2 ("系统 因为 勒索"~2)^1.5 OR ("mes"^0.7)`)
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
		KbIDs:      []string{"kb-1"},
		Limit:      30,
		Filter: map[string]interface{}{
			"doc_id":        []string{"doc-1"},
			"available_int": 1,
			"category_kwd":  "allowed",
		},
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
	filterJSON, err := json.Marshal(knn["filter"])
	if err != nil {
		t.Fatal(err)
	}
	filter := string(filterJSON)
	for _, scope := range []string{"kb-1", "doc-1", "available_int", "category_kwd", "allowed"} {
		if !strings.Contains(filter, scope) {
			t.Fatalf("KNN filter %s lost scope %q", filter, scope)
		}
	}
	if strings.Contains(filter, "query_string") {
		t.Fatalf("dense-only KNN filter contains lexical predicate: %s", filter)
	}
}

func TestKNNScoresSearchesAllCandidateIndices(t *testing.T) {
	searches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searches++
		if r.URL.Path != "/ragflow_a,ragflow_b/_search" {
			t.Errorf("KNN search path = %s, want both candidate indices once", r.URL.Path)
		}
		if r.URL.Query().Get("allow_partial_search_results") != "false" {
			t.Error("partial KNN results must trigger the local reranking fallback")
		}
		var query map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		knn, _ := query["knn"].(map[string]interface{})
		filter, _ := json.Marshal(knn["filter"])
		if string(filter) != `{"terms":{"id":["a1","a2","b1"]}}` {
			t.Errorf("KNN candidate filter = %s, want candidates from both indices", filter)
		}
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"a1","_score":0.9},{"_id":"a2","_score":0.8},{"_id":"b1","_score":0.7}]}}`))
	}))
	defer server.Close()
	eng := newTestEngine(t, server.URL)
	chunks := []map[string]interface{}{
		{"_id": "a1", "_index": "ragflow_a"},
		{"_id": "a2", "_index": "ragflow_a"},
		{"_id": "b1", "_index": "ragflow_b"},
	}
	result, err := eng.KNNScores(t.Context(), chunks, []float64{0.1, 0.2}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := eng.GetScores(result); !reflect.DeepEqual(got, map[string]float64{"a1": 0.9, "a2": 0.8, "b1": 0.7}) {
		t.Fatalf("KNN scores = %v, want scores from both indices", got)
	}
	result, err = eng.KNNScores(t.Context(), []map[string]interface{}{{"_id": "unscoped"}}, []float64{0.1, 0.2}, 1)
	if err == nil || result != nil || searches != 1 {
		t.Fatalf("missing index: result = %v, err = %v, searches = %d, want fallback without a cluster-wide query", result, err, searches)
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
		"kb_id":          "kb-1",
		"source_doc_ids": []string{"doc-a", "doc-b"},
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
	assertEqual(t, terms["source_doc_ids"], []interface{}{"doc-a", "doc-b"})
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

// TestUpdateChunksPreservesStringSliceCondition guards the document
// availability switch: the doc-service caller of UpdateChunks
// (updateDocumentChunkAvailability) passes a typed []string id list, and a
// builder that only understands []interface{} silently drops the id clause,
// widening the update-by-query to every chunk of the dataset (kb_id is then the
// only remaining filter).
func TestUpdateChunksPreservesStringSliceCondition(t *testing.T) {
	var updateQuery map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			if r.URL.Path != "/ragflow_tenant/_update_by_query" {
				t.Errorf("path=%s, want /ragflow_tenant/_update_by_query", r.URL.Path)
				http.Error(w, "unexpected request path", http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&updateQuery); err != nil {
				t.Errorf("decode update query: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"updated":2}`))
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
	if err = engine.UpdateChunks(ctx,
		map[string]interface{}{"id": []string{"chunk-a", "chunk-b"}},
		map[string]interface{}{"available_int": 1, "source_doc_ids": []string{"doc-a"}},
		"ragflow_tenant", "kb-1"); err != nil {
		t.Fatalf("UpdateChunks: %v", err)
	}

	query, ok := updateQuery["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("update query missing query: %#v", updateQuery)
	}
	boolQuery, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("update query missing bool: %#v", query)
	}
	filter, ok := boolQuery["filter"].([]interface{})
	if !ok {
		t.Fatalf("update query missing filter: %#v", boolQuery)
	}
	var gotIDs, gotKB interface{}
	for _, raw := range filter {
		clause, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if terms, ok := clause["terms"].(map[string]interface{}); ok {
			if ids, ok := terms["id"]; ok {
				gotIDs = ids
			}
		}
		if term, ok := clause["term"].(map[string]interface{}); ok {
			if kb, ok := term["kb_id"]; ok {
				gotKB = kb
			}
		}
	}
	if gotIDs == nil {
		t.Fatalf("update filter dropped the id terms clause (would hit the whole dataset): %#v", filter)
	}
	assertEqual(t, gotIDs, []interface{}{"chunk-a", "chunk-b"})
	assertEqual(t, gotKB, "kb-1")

	script, ok := updateQuery["script"].(map[string]interface{})
	if !ok {
		t.Fatalf("update query missing script: %#v", updateQuery)
	}
	source, _ := script["source"].(string)
	if !strings.Contains(source, "ctx._source.available_int=1;") {
		t.Fatalf("script source=%q, want available_int assignment", source)
	}
	if !strings.Contains(source, "ctx._source.source_doc_ids=params.pp_source_doc_ids;") {
		t.Fatalf("script source=%q, want typed []string value assignment", source)
	}
	scriptParams, ok := script["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("update script missing params: %#v", script)
	}
	assertEqual(t, scriptParams["pp_source_doc_ids"], []interface{}{"doc-a"})
}

// TestUpdateChunksPreservesMustNotCondition locks the exclusion half of the
// condition contract. A dropped must_not widens the update to exactly the rows
// the caller asked to exclude (e.g. hybrid_search's
// must_not={"exists":"compile_kwd"} excluding compiled products).
func TestUpdateChunksPreservesMustNotCondition(t *testing.T) {
	var updateQuery map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			if r.URL.Path != "/ragflow_tenant/_update_by_query" {
				t.Errorf("path=%s, want /ragflow_tenant/_update_by_query", r.URL.Path)
				http.Error(w, "unexpected request path", http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&updateQuery); err != nil {
				t.Errorf("decode update query: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"updated":1}`))
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
	exclusion := []interface{}{
		map[string]interface{}{"exists": map[string]interface{}{"field": "compile_kwd"}},
	}
	if err = engine.UpdateChunks(ctx,
		map[string]interface{}{
			"doc_id":   "doc-1",
			"must_not": map[string]interface{}{"exists": "compile_kwd"},
		},
		map[string]interface{}{"available_int": 0},
		"ragflow_tenant", "kb-1"); err != nil {
		t.Fatalf("UpdateChunks: %v", err)
	}
	boolQuery := mustBoolQuery(t, updateQuery)
	assertEqual(t, boolQuery["must_not"], exclusion)
	if _, ok := boolQuery["filter"]; !ok {
		t.Fatalf("update query lost the positive filter: %#v", boolQuery)
	}

	// A must_not-only condition is an explicit "everything except" scope rather
	// than a dropped filter, so it is accepted and forwarded. DeleteChunks
	// behaves the same way.
	if err = engine.updateChunksByQuery(ctx, "ragflow_tenant",
		map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}},
		map[string]interface{}{"available_int": 0}); err != nil {
		t.Fatalf("updateChunksByQuery must_not only: %v", err)
	}
	boolQuery = mustBoolQuery(t, updateQuery)
	assertEqual(t, boolQuery["must_not"], exclusion)
	if _, ok := boolQuery["filter"]; ok {
		t.Fatalf("must_not-only update should carry no positive filter: %#v", boolQuery)
	}
}

// mustBoolQuery extracts query.bool from a captured update-by-query body.
func mustBoolQuery(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	query, ok := body["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("update query missing query: %#v", body)
	}
	boolQuery, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("update query missing bool: %#v", query)
	}
	return boolQuery
}

// TestUpdateChunksByQueryRejectsUnfilteredUpdate locks the guard: a non-empty
// condition whose keys all get dropped must fail instead of rewriting every row
// of the index.
func TestUpdateChunksByQueryRejectsUnfilteredUpdate(t *testing.T) {
	engine := &Engine{}
	err := engine.updateChunksByQuery(t.Context(), "ragflow_tenant",
		map[string]interface{}{"unsupported_field": struct{}{}},
		map[string]interface{}{"available_int": 1})
	if err == nil {
		t.Fatal("updateChunksByQuery accepted a condition that yields no filter clause")
	}
	if !strings.Contains(err.Error(), "unfiltered update") {
		t.Fatalf("err=%v, want unfiltered update refusal", err)
	}
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

// TestBuildRankFeatureQueryUsesTopLevelPagerank pins the field naming: tag
// weights address keys inside the tag_feas map, while pagerank addresses the
// top-level pagerank_fea rank_feature field.
func TestBuildRankFeatureQueryUsesTopLevelPagerank(t *testing.T) {
	queries := buildRankFeatureQuery(map[string]float64{
		common.PAGERANK_FLD: 10,
		"tag_a":             0.8,
	})
	if len(queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(queries))
	}
	byField := make(map[string]map[string]interface{}, len(queries))
	for _, q := range queries {
		clause, ok := q["rank_feature"].(map[string]interface{})
		if !ok {
			t.Fatalf("not a rank_feature clause: %#v", q)
		}
		field, _ := clause["field"].(string)
		byField[field] = clause
	}
	pagerank, ok := byField[common.PAGERANK_FLD]
	if !ok {
		t.Fatalf("pagerank clause missing, got fields %#v", byField)
	}
	if got := pagerank["boost"]; got != float64(10) {
		t.Fatalf("pagerank boost = %v, want 10", got)
	}
	tagField := common.TAG_FLD + ".tag_a"
	if _, ok := byField[tagField]; !ok {
		t.Fatalf("tag clause %q missing, got fields %#v", tagField, byField)
	}
}

// TestElasticsearchGetFieldsEncodesNestedObjects covers the rank-feature
// fields: tag_feas is a "rank_features" object in _source, so the projection
// has to render it as JSON (the reranker parses JSON, not Go's "%v" map form),
// while numeric fields keep the stringified form the rest of the projection uses.
func TestElasticsearchGetFieldsEncodesNestedObjects(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{
			"id":                  "chunk-1",
			"tag_feas":            map[string]interface{}{"tag_a": float64(0.5)},
			"pagerank_fea":        float64(7),
			"content_with_weight": "body",
		},
	}

	fieldMap, ok := engine.GetFields(chunks, []string{"id", "tag_feas", "pagerank_fea"})["chunk-1"]
	if !ok {
		t.Fatalf("GetFields lost chunk-1")
	}
	assertEqual(t, fieldMap["tag_feas"], `{"tag_a":0.5}`)
	assertEqual(t, fieldMap["pagerank_fea"], "7")
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

func TestElasticsearchGetHighlightUsesRegex(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{
			"_id":                 "full-text",
			"content_with_weight": "Alpha beta.\nbetamax soup. BETA again!",
		},
		{"id": "no-match", "content_with_weight": "Keep the complete text."},
		{"id": "missing"},
	}

	got := engine.GetHighlight(chunks, []string{"beta"}, "content_with_weight")
	assertEqual(t, got, map[string]string{
		"full-text": "Alpha <em>beta</em>.\n<em>betamax</em> soup. <em>BETA</em> again!",
		"no-match":  "Keep the complete text.",
	})
}

func TestElasticsearchGetHighlightExpandsLatinStems(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{
			"_id":                 "latin",
			"content_with_weight": "Required coordinated isolation. Coordinación.",
		},
	}

	got := engine.GetHighlight(chunks, []string{"requir", "coordin", "isol"}, "content_with_weight")
	assertEqual(t, got, map[string]string{
		"latin": "<em>Required</em> <em>coordinated</em> <em>isolation</em>. <em>Coordinación</em>.",
	})
}

func TestElasticsearchGetHighlightPrefersLongKeywords(t *testing.T) {
	engine := &Engine{}

	gotExisting := engine.GetHighlight([]map[string]interface{}{
		{"id": "existing", "content_with_weight": "already <em>marked</em> text"},
	}, []string{"marked"}, "content_with_weight")
	assertEqual(t, gotExisting, map[string]string{"existing": "already <em><em>marked</em></em> text"})

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
	if got := engine.GetHighlight([]map[string]interface{}{{"id": "x", "content": "x"}}, nil, "content"); !reflect.DeepEqual(got, map[string]string{"x": "x"}) {
		t.Fatalf("empty keyword highlight=%#v, want unchanged text", got)
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

// TestSearchKeepsRankFeatureOutOfKNNFilter pins the body-level split: the
// rank_feature clauses belong to the scored query, while knn.filter is a snapshot
// of the bool taken when the kNN clause is built and must keep only the scope and
// lexical predicates.
func TestSearchKeepsRankFeatureOutOfKNNFilter(t *testing.T) {
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
		KbIDs:      []string{"kb-1"},
		Limit:      10,
		Filter:     map[string]interface{}{"available_int": 1},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "alpha", Fields: []string{"content_ltks"}, TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.1, 0.2}, TopN: 128},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10, FusionParams: map[string]interface{}{"weights": "0.7,0.3"}},
		},
		RankFeature: map[string]float64{"pagerank_fea": 10},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	queryJSON, err := json.Marshal(searchQuery["query"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(queryJSON), "rank_feature") || !strings.Contains(string(queryJSON), "pagerank_fea") {
		t.Fatalf("scored query lost its rank_feature clause: %s", queryJSON)
	}

	knn, ok := searchQuery["knn"].(map[string]interface{})
	if !ok {
		t.Fatalf("KNN query missing from %#v", searchQuery)
	}
	filterJSON, err := json.Marshal(knn["filter"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(filterJSON), "rank_feature") {
		t.Fatalf("knn.filter carries rank_feature clauses: %s", filterJSON)
	}
	// The snapshot must still be the scope + lexical predicate, not an empty one.
	if !strings.Contains(string(filterJSON), "query_string") {
		t.Fatalf("knn.filter lost the lexical predicate: %s", filterJSON)
	}
}

// TestSearchKeepsEveryRankFeatureClause covers the multi-feature case: appending
// to the pre-loop should header used to keep only the last clause.
func TestSearchKeepsEveryRankFeatureClause(t *testing.T) {
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
		KbIDs:      []string{"kb-1"},
		Limit:      10,
		Filter:     map[string]interface{}{"available_int": 1},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "alpha", Fields: []string{"content_ltks"}, TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.1, 0.2}, TopN: 128},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10, FusionParams: map[string]interface{}{"weights": "0.7,0.3"}},
		},
		RankFeature: map[string]float64{"tag_1": 0.8, "tag_2": 0.5},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	query, ok := searchQuery["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("query missing from %#v", searchQuery)
	}
	boolMap, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("bool query missing from %#v", query)
	}
	should, _ := boolMap["should"].([]interface{})
	var fields []string
	for _, clause := range should {
		clauseMap, ok := clause.(map[string]interface{})
		if !ok {
			continue
		}
		rankFeature, ok := clauseMap["rank_feature"].(map[string]interface{})
		if !ok {
			continue
		}
		if field, ok := rankFeature["field"].(string); ok {
			fields = append(fields, field)
		}
	}
	if len(fields) != 2 {
		t.Fatalf("rank_feature clauses = %v, want both tag fields", fields)
	}
	joined := strings.Join(fields, ",")
	if !strings.Contains(joined, "tag_feas.tag_1") || !strings.Contains(joined, "tag_feas.tag_2") {
		t.Fatalf("rank_feature fields = %v, want tag_feas.tag_1 and tag_feas.tag_2", fields)
	}
}

// TestSearchDoesNotAddPageRankToHitScore pins the hybrid score contract: _score is
// the ES hit score and nothing else. pagerank already reaches the hit score
// through the rank_feature clause and the final similarity through the reranker;
// folding it in here again would count it twice.
func TestSearchDoesNotAddPageRankToHitScore(t *testing.T) {
	if err := common.InitLogger("info", common.FileOutput{}, "elasticsearch_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"_id":"chunk-1","_index":"ragflow_tenant","_score":0.87,"_source":{"content_with_weight":"alpha","pagerank_fea":69,"tag_feas":{"tag_1":0.5}}}]}}`))
	}))
	defer server.Close()

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	engine := &Engine{client: client}
	res, err := engine.Search(t.Context(), &types.SearchRequest{
		IndexNames: []string{"ragflow_tenant"},
		KbIDs:      []string{"kb-1"},
		Limit:      10,
		Filter:     map[string]interface{}{"available_int": 1},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "alpha", Fields: []string{"content_ltks"}, TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.1, 0.2}, TopN: 128},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10, FusionParams: map[string]interface{}{"weights": "0.7,0.3"}},
		},
		RankFeature: map[string]float64{"pagerank_fea": 10},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Chunks) != 1 {
		t.Fatalf("Search returned %d chunks, want 1", len(res.Chunks))
	}
	if got, ok := res.Chunks[0]["_score"].(float64); !ok || got != 0.87 {
		t.Fatalf("_score = %#v, want the hit score 0.87 (pagerank must not be folded in)", res.Chunks[0]["_score"])
	}
	if got, ok := res.Chunks[0]["pagerank_fea"].(float64); !ok || got != 69 {
		t.Fatalf("pagerank_fea = %#v, want 69 preserved for the reranker", res.Chunks[0]["pagerank_fea"])
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

func TestCloneFilterSnapshotIsolatesLaterShouldAppends(t *testing.T) {
	boolMap := map[string]interface{}{
		"must":   []interface{}{map[string]interface{}{"term": map[string]interface{}{"kb_id": "kb-1"}}},
		"should": []interface{}{map[string]interface{}{"match_all": map[string]interface{}{}}},
	}
	boolQuery := map[string]interface{}{"bool": boolMap}

	snapshot := cloneFilterSnapshot(boolQuery)

	// Mirror what the rank_feature block does after the kNN clause is built: it
	// appends to the scored query's bool/should.
	boolMap["should"] = append(
		boolMap["should"].([]interface{}),
		map[string]interface{}{"rank_feature": map[string]interface{}{"field": "pagerank_fea", "boost": 10.0}},
	)

	if got := len(boolMap["should"].([]interface{})); got != 2 {
		t.Fatalf("original should has %d clauses, want 2", got)
	}
	snapshotBool, ok := snapshot["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("snapshot bool missing from %#v", snapshot)
	}
	if got := len(snapshotBool["should"].([]interface{})); got != 1 {
		t.Fatalf("snapshot should has %d clauses, want 1 (rank_feature must stay out of knn.filter)", got)
	}
	if got := len(snapshotBool["must"].([]interface{})); got != 1 {
		t.Fatalf("snapshot must has %d clauses, want 1", got)
	}
}

func TestCloneFilterSnapshotNil(t *testing.T) {
	if got := cloneFilterSnapshot(nil); got != nil {
		t.Fatalf("cloneFilterSnapshot(nil) = %#v, want nil", got)
	}
}

func TestCloneFilterSnapshotNonBoolQuery(t *testing.T) {
	query := map[string]interface{}{
		"query_string": map[string]interface{}{"query": "alpha"},
	}
	snapshot := cloneFilterSnapshot(query)
	if snapshot["query_string"] == nil {
		t.Fatalf("snapshot = %#v, want the query_string clause preserved", snapshot)
	}
}
