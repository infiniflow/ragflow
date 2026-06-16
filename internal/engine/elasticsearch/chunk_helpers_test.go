package elasticsearch

import (
	"reflect"
	"strings"
	"testing"
)

func TestElasticsearchGetFieldsFiltersAndUsesIDFallback(t *testing.T) {
	engine := &elasticsearchEngine{}
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
	engine := &elasticsearchEngine{}

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
	if fallbackMap, ok := got["fallback-chunk"]; !ok {
		t.Fatalf("GetFields keys=%v, want fallback-chunk", got)
	} else {
		assertEqual(t, fallbackMap["id"], "fallback-chunk")
		assertEqual(t, fallbackMap["docnm_kwd"], "fallback.md")
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

func TestElasticsearchGetChunkIDsPreservesOrderWithFallback(t *testing.T) {
	engine := &elasticsearchEngine{}
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
