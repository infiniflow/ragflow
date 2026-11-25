package infinity

import "testing"

// Both filter builders append clauses while ranging over the condition map.
// Without a stable key order the same logical condition renders two different
// strings across calls, which makes log comparison unreliable and turns the
// predicate of a bulk update/delete into a per-run lottery. The nested
// must_not map is walked by the same builders, so its clause order is pinned
// here as well.
func TestFilterBuildersRenderAConditionInAStableOrder(t *testing.T) {
	condition := map[string]interface{}{
		"doc_id": "d38a5afe46ac44cf",
		"kb_id":  "kb-1",
		"must_not": map[string]interface{}{
			"exists":       "compile_kwd",
			"doc_type_kwd": "text",
		},
	}

	wantSearch := "doc_id='d38a5afe46ac44cf' AND kb_id='kb-1' AND NOT ((filter_fulltext('doc_type_kwd', 'text')) AND compile_kwd!='')"
	wantFilter := "doc_id='d38a5afe46ac44cf' AND kb_id='kb-1' AND NOT ((filter_fulltext('doc_type_kwd', 'text')) AND compile_kwd!=null)"
	if got := equivalentConditionToStrRaw(condition, nil); got != wantSearch {
		t.Fatalf("search filter = %q, want %q", got, wantSearch)
	}
	if got := buildFilterFromConditionRaw(condition, nil); got != wantFilter {
		t.Fatalf("update/delete filter = %q, want %q", got, wantFilter)
	}

	for i := 0; i < 1000; i++ {
		if got := equivalentConditionToStrRaw(condition, nil); got != wantSearch {
			t.Fatalf("search filter changed between calls: %q then %q", wantSearch, got)
		}
		if got := buildFilterFromConditionRaw(condition, nil); got != wantFilter {
			t.Fatalf("update/delete filter changed between calls: %q then %q", wantFilter, got)
		}
	}
}

// Both builders first rewrite mixed stored generations through
// types.CompilationFilter, which appends one "or" group per rewritten field.
// Two rewritten fields must yield the same group order on every call; the
// top-level key sort alone does not cover this map.
func TestFilterBuildersKeepCompilationGroupsInAStableOrder(t *testing.T) {
	condition := map[string]interface{}{
		"compile_kwd": "wiki",
		"type_kwd":    "wiki_page",
		"kb_id":       "kb-1",
	}

	wantSearch := equivalentConditionToStr(condition, nil)
	wantFilter := buildFilterFromCondition(condition, nil)
	if wantSearch == "" || wantFilter == "" {
		t.Fatalf("builders dropped the condition: search=%q filter=%q", wantSearch, wantFilter)
	}
	for i := 0; i < 1000; i++ {
		if got := equivalentConditionToStr(condition, nil); got != wantSearch {
			t.Fatalf("search filter changed between calls: %q then %q", wantSearch, got)
		}
		if got := buildFilterFromCondition(condition, nil); got != wantFilter {
			t.Fatalf("update/delete filter changed between calls: %q then %q", wantFilter, got)
		}
	}
}
