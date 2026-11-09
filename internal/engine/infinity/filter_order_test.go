package infinity

import "testing"

// Both filter builders append clauses while ranging over the condition map.
// Without a stable key order the same logical condition renders two different
// strings across calls, which makes log comparison unreliable and turns the
// predicate of a bulk update/delete into a per-run lottery.
func TestFilterBuildersRenderAConditionInAStableOrder(t *testing.T) {
	condition := map[string]interface{}{
		"doc_id":   "d38a5afe46ac44cf",
		"kb_id":    "kb-1",
		"must_not": map[string]interface{}{"exists": "compile_kwd"},
	}

	wantSearch := equivalentConditionToStrRaw(condition, nil)
	wantFilter := buildFilterFromConditionRaw(condition, nil)
	for i := 0; i < 1000; i++ {
		if got := equivalentConditionToStrRaw(condition, nil); got != wantSearch {
			t.Fatalf("search filter changed between calls: %q then %q", wantSearch, got)
		}
		if got := buildFilterFromConditionRaw(condition, nil); got != wantFilter {
			t.Fatalf("update/delete filter changed between calls: %q then %q", wantFilter, got)
		}
	}
	if wantSearch == "" || wantFilter == "" {
		t.Fatalf("builders dropped the condition: search=%q filter=%q", wantSearch, wantFilter)
	}
}
