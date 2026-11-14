package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestNormalizeExtractorParams_NilReturnsNonNilEmpty locks that a nil input
// yields a usable (non-nil) empty map, not nil — callers range/assign into it.
func TestNormalizeExtractorParams_NilReturnsNonNilEmpty(t *testing.T) {
	out := NormalizeExtractorParams(nil)
	if out == nil {
		t.Fatal("nil input must yield a non-nil empty map")
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %#v", out)
	}
}

// TestNormalizeExtractorParams_KnownKeyMapIsDistinctCopy locks that each of the
// five modular keys (keywords/questions/tags/summary/metadata), when its value is
// a map, is replaced by a fresh copy so callers can mutate it without touching
// the caller's request map.
func TestNormalizeExtractorParams_KnownKeyMapIsDistinctCopy(t *testing.T) {
	for _, key := range []string{"keywords", "questions", "tags", "summary", "metadata"} {
		t.Run(key, func(t *testing.T) {
			sub := map[string]any{"top_n": 5}
			raw := map[string]any{key: sub}
			out := NormalizeExtractorParams(raw)
			got, ok := out[key].(map[string]any)
			if !ok {
				t.Fatalf("%s: expected map, got %T", key, out[key])
			}
			if mapDataPtr(out[key]) == mapDataPtr(raw[key]) {
				t.Errorf("%s: normalized map must be a distinct copy, not the input map", key)
			}
			// Mutating the normalized copy must not leak into the input.
			got["top_n"] = 999
			if raw[key].(map[string]any)["top_n"] != 5 {
				t.Errorf("%s: mutation of normalized copy leaked into the input map", key)
			}
		})
	}
}

// TestNormalizeExtractorParams_UnknownMapKeyIsShared locks the precise contract
// that ONLY the five modular keys get copied. Any other map-typed value (e.g. a
// legacy/nested key) must stay shared by reference. This is the adversarial guard
// against a refactor that loosens the loop to "copy every map value" — that
// would be a behavior change and must fail.
func TestNormalizeExtractorParams_UnknownMapKeyIsShared(t *testing.T) {
	custom := map[string]any{"a": 1}
	raw := map[string]any{"custom_map": custom}
	out := NormalizeExtractorParams(raw)
	if mapDataPtr(out["custom_map"]) != mapDataPtr(raw["custom_map"]) {
		t.Errorf("non-modular map key must remain shared by reference, got %#v", out["custom_map"])
	}
}

// mapDataPtr returns the underlying data pointer for a map (or 0 for nil /
// non-map). It lets tests assert map identity without the `==` operator, which
// panics on uncomparable map values.
func mapDataPtr(v any) uintptr {
	if v == nil {
		return 0
	}
	return reflect.ValueOf(v).Pointer()
}

// TestNormalizeExtractorParams_TopLevelScalarShared locks that non-map top-level
// values are carried over by reference (the function only clones the five
// modular sub-maps, never the scalars).
func TestNormalizeExtractorParams_TopLevelScalarShared(t *testing.T) {
	raw := map[string]any{"llm_id": "llm-a", "temperature": 0.7}
	out := NormalizeExtractorParams(raw)
	if out["llm_id"] != "llm-a" {
		t.Errorf("llm_id = %v, want llm-a", out["llm_id"])
	}
	if out["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", out["temperature"])
	}
}

// TestNormalizeExtractorParams_OneLevelCopyOfKnownMap locks that the copy of a
// modular sub-map is one level deep: the sub-map's own entries are new keys, but
// nested values (slices/maps) are shared by reference. This is the exact
// behavior the refactor must preserve.
func TestNormalizeExtractorParams_OneLevelCopyOfKnownMap(t *testing.T) {
	inner := []map[string]any{{"key": "cat", "type": "string"}}
	sub := map[string]any{"metadata": inner}
	raw := map[string]any{"metadata": sub}
	out := NormalizeExtractorParams(raw)
	got, ok := out["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %T", out["metadata"])
	}
	gotInner, ok := got["metadata"].([]map[string]any)
	if !ok {
		t.Fatalf("expected metadata slice, got %T", got["metadata"])
	}
	if &gotInner[0] != &inner[0] {
		t.Error("nested slice element inside a modular map must be shared by reference (one-level copy)")
	}
}

// TestComponentParamsDefaults_DeepCopyIsRecursive locks that the per-component
// params returned by ComponentParamsDefaults are recursively deep-copied (not
// shallow). This guards the S5 cleanup: deepCopyMapStr is removed in favor of
// deepCopyValue, which must keep the recursive deep-copy semantics so a mutation
// of a nested value in the result cannot leak into a subsequent call's result.
func TestComponentParamsDefaults_DeepCopyIsRecursive(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"Extractor:X": map[string]any{
				"obj": map[string]any{
					"component_name": "Extractor",
					"params": map[string]any{
						"metadata": map[string]any{
							"metadata": []map[string]any{{"key": "cat", "type": "string"}},
						},
					},
				},
			},
		},
	}
	dslJSON, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal dsl: %v", err)
	}
	result, err := ComponentParamsDefaults(dslJSON)
	if err != nil {
		t.Fatalf("ComponentParamsDefaults: %v", err)
	}
	comp := result["Extractor:X"]
	meta, ok := comp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %T", comp["metadata"])
	}
	list, ok := meta["metadata"].([]any)
	if !ok {
		t.Fatalf("expected metadata slice, got %T", meta["metadata"])
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata entry map, got %T", list[0])
	}
	// Mutate the nested map inside the returned copy.
	entry["key"] = "changed"

	// A fresh call must not see the mutation — proof of recursive deep copy.
	result2, err := ComponentParamsDefaults(dslJSON)
	if err != nil {
		t.Fatalf("ComponentParamsDefaults (2nd): %v", err)
	}
	meta2 := result2["Extractor:X"]["metadata"].(map[string]any)
	list2, ok := meta2["metadata"].([]any)
	if !ok {
		t.Fatalf("expected metadata slice (2nd), got %T", meta2["metadata"])
	}
	entry2, ok := list2[0].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata entry map (2nd), got %T", list2[0])
	}
	if entry2["key"] != "cat" {
		t.Errorf("recursive deep copy broken: nested map leaked mutation, got key=%q", entry2["key"])
	}
}
