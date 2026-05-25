package elasticsearch

import (
	"reflect"
	"strings"
	"testing"
)

func TestElasticsearchGetFieldsAppliesMappingsAndFilter(t *testing.T) {
	engine := &elasticsearchEngine{}
	chunks := []map[string]interface{}{
		{
			"_id":                "fallback-chunk",
			"docnm":              "guide.md",
			"important_keywords": "alpha,beta",
			"questions":          "What is alpha?\nWhat is beta?",
			"content":            "Alpha beta body.",
			"authors":            "Ada",
			"position_int":       "00000001_00000002_00000003_00000004_00000005_00000006",
			"page_num_int":       "0000000a_0000000b",
			"top_int":            "0000000c",
			"tag_kwd":            "tag-a###tag-b###",
			"ROW_ID":             "row-7",
		},
	}

	fields := []string{
		"id",
		"docnm_kwd",
		"title_tks",
		"title_sm_tks",
		"important_kwd",
		"important_tks",
		"question_kwd",
		"question_tks",
		"content_with_weight",
		"content_ltks",
		"content_sm_ltks",
		"authors_tks",
		"authors_sm_tks",
		"position_int",
		"page_num_int",
		"top_int",
		"tag_kwd",
		"row_id()",
	}

	got := engine.GetFields(chunks, fields)
	fieldMap, ok := got["fallback-chunk"]
	if !ok {
		t.Fatalf("GetFields keys=%v, want fallback-chunk", got)
	}

	assertEqual(t, fieldMap["id"], "fallback-chunk")
	assertEqual(t, fieldMap["docnm_kwd"], "guide.md")
	assertEqual(t, fieldMap["title_tks"], "guide.md")
	assertEqual(t, fieldMap["title_sm_tks"], "guide.md")
	assertEqual(t, fieldMap["important_kwd"], []interface{}{"alpha", "beta"})
	assertEqual(t, fieldMap["important_tks"], "alpha,beta")
	assertEqual(t, fieldMap["question_kwd"], []interface{}{"What is alpha?", "What is beta?"})
	assertEqual(t, fieldMap["question_tks"], "What is alpha?\nWhat is beta?")
	assertEqual(t, fieldMap["content_with_weight"], "Alpha beta body.")
	assertEqual(t, fieldMap["content_ltks"], "Alpha beta body.")
	assertEqual(t, fieldMap["content_sm_ltks"], "Alpha beta body.")
	assertEqual(t, fieldMap["authors_tks"], "Ada")
	assertEqual(t, fieldMap["authors_sm_tks"], "Ada")
	// Position values are grouped as x, y, width, height, page; trailing values start a new group.
	assertEqual(t, fieldMap["position_int"], [][]int{{1, 2, 3, 4, 5}, {6}})
	assertEqual(t, fieldMap["page_num_int"], []int{10, 11})
	assertEqual(t, fieldMap["top_int"], []int{12})
	assertEqual(t, fieldMap["tag_kwd"], []interface{}{"tag-a", "tag-b"})
	assertEqual(t, fieldMap["row_id()"], "row-7")

	if _, ok := fieldMap["docnm"]; ok {
		t.Fatalf("field filter leaked raw docnm: %#v", fieldMap)
	}
	if _, ok := fieldMap["ROW_ID"]; ok {
		t.Fatalf("ROW_ID should be mapped to row_id(): %#v", fieldMap)
	}
}

func TestElasticsearchGetFieldsEmptyDefaultsAndSkippedIDs(t *testing.T) {
	engine := &elasticsearchEngine{}

	if got := engine.GetFields(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("GetFields(nil)=%#v, want empty non-nil map", got)
	}

	got := engine.GetFields([]map[string]interface{}{
		{"id": "chunk-1"},
		{"id": "", "_id": "fallback-chunk"},
		{"docnm": "missing-id.md"},
	}, nil)

	fieldMap, ok := got["chunk-1"]
	if !ok {
		t.Fatalf("GetFields keys=%v, want chunk-1", got)
	}
	if _, ok := got["missing-id.md"]; ok {
		t.Fatalf("chunk without id should be skipped: %#v", got)
	}
	if fallbackMap, ok := got["fallback-chunk"]; !ok {
		t.Fatalf("GetFields keys=%v, want fallback-chunk", got)
	} else {
		assertEqual(t, fallbackMap["id"], "fallback-chunk")
	}

	emptyArrayFields := []string{
		"doc_type_kwd",
		"important_kwd",
		"question_kwd",
		"tag_kwd",
	}
	for _, field := range emptyArrayFields {
		assertEqual(t, fieldMap[field], []interface{}{})
	}

	emptyTextFields := []string{
		"important_tks",
		"question_tks",
		"authors_tks",
		"authors_sm_tks",
		"title_tks",
		"title_sm_tks",
		"content_ltks",
		"content_sm_ltks",
	}
	for _, field := range emptyTextFields {
		assertEqual(t, fieldMap[field], "")
	}
}

func TestElasticsearchGetAggregationSplitsCountsAndSorts(t *testing.T) {
	engine := &elasticsearchEngine{}
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

func TestElasticsearchGetDocIDsPreservesOrderWithFallback(t *testing.T) {
	engine := &elasticsearchEngine{}
	chunks := []map[string]interface{}{
		{"id": "source-id", "_id": "hit-id"},
		{"_id": "fallback-id"},
		{"id": ""},
		{"id": 42},
		{"id": "last-id"},
	}

	got := engine.GetDocIDs(chunks)
	assertEqual(t, got, []string{"source-id", "fallback-id", "last-id"})

	if got := engine.GetDocIDs(nil); got != nil {
		t.Fatalf("GetDocIDs(nil)=%#v, want nil", got)
	}
}

func TestElasticsearchGetHighlightFallbackAndBoundaries(t *testing.T) {
	engine := &elasticsearchEngine{}
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
	engine := &elasticsearchEngine{}

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
