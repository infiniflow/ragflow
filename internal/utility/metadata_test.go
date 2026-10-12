package utility

import (
	"reflect"
	"testing"
)

// =============================================================================
// UpdateMetadataTo
// =============================================================================

func TestUpdateMetadataTo_EmptyMeta(t *testing.T) {
	result := UpdateMetadataTo(map[string]any{"a": 1}, nil)
	if result["a"] != 1 {
		t.Errorf("a = %v, want 1", result["a"])
	}
}

func TestUpdateMetadataTo_NewKeysAdded(t *testing.T) {
	result := UpdateMetadataTo(map[string]any{"a": 1}, map[string]any{"b": "x"})
	if result["a"] != 1 {
		t.Errorf("a should be preserved")
	}
	if result["b"] != "x" {
		t.Errorf("b should be added")
	}
}

func TestUpdateMetadataTo_ExistingKeyScalarOverwrite(t *testing.T) {
	// Python update_metadata_to overwrites a scalar target with the incoming
	// (merged-in) value rather than list-ifying it.
	result := UpdateMetadataTo(map[string]any{"author": "Alice"}, map[string]any{"author": "Bob"})
	if result["author"] != "Bob" {
		t.Errorf("author = %v, want Bob (incoming scalar overwrites)", result["author"])
	}
}

func TestUpdateMetadataTo_PreservesTypedValues(t *testing.T) {
	values := map[string]any{
		"integer": int64(42), "decimal": 1.5, "zero": 0, "false": false,
		"null": nil, "object": map[string]any{"title": "Intro"},
		"mixed": []any{"contract", 123, false}, "numbers": []any{1, 2},
		"outline": []any{map[string]any{"title": "Intro", "depth": 1}},
	}
	got := UpdateMetadataTo(map[string]any{}, values)
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("metadata = %#v, want %#v", got, values)
	}
}

func TestUpdateMetadataTo_TypedConflicts(t *testing.T) {
	type tags []string
	tests := []struct {
		name                   string
		target, incoming, want any
	}{
		{"number overwrites scalar", "new", int64(42), int64(42)},
		{"false overwrites list", []string{"new"}, false, false},
		{"null overwrites scalar", "new", nil, nil},
		{"object keeps string", map[string]any{"x": 1}, "old", map[string]any{"x": 1}},
		{"object keeps number", map[string]any{"x": 1}, int64(42), map[string]any{"x": 1}},
		{"object keeps float", map[string]any{"x": 1}, 1.5, map[string]any{"x": 1}},
		{"object keeps bool", map[string]any{"x": 1}, false, map[string]any{"x": 1}},
		{"object keeps null", map[string]any{"x": 1}, nil, map[string]any{"x": 1}},
		{"structured list keeps number", []any{1, 2}, int64(42), []any{1, 2}},
		{"structured list keeps float", []any{1, 2}, 1.5, []any{1, 2}},
		{"structured list keeps bool", []any{1, 2}, false, []any{1, 2}},
		{"structured list keeps null", []any{1, 2}, nil, []any{1, 2}},
		{"typed numeric list keeps number", []int{1, 2}, 42, []int{1, 2}},
		{"typed numeric list keeps string", []float64{1, 2}, "old", []float64{1, 2}},
		{"typed map keeps scalar", map[string]int{"x": 1}, false, map[string]int{"x": 1}},
		{"array keeps scalar", [2]int{1, 2}, 42, [2]int{1, 2}},
		{"incoming array keeps target", 42, [2]int{1, 2}, 42},
		{"object conflict keeps first", map[string]any{"x": 1}, map[string]any{"x": 2}, map[string]any{"x": 1}},
		{"structured list conflict keeps first", []any{1, 2}, []any{3, 4}, []any{1, 2}},
		{"typed numeric list keeps target", "new", []int{1, 2}, "new"},
		{"object keeps target", "new", map[string]any{"x": 1}, "new"},
		{"mixed list keeps target", "new", []any{"old", 1}, "new"},
		{"structured target keeps list", []any{"new", 1}, []string{"old"}, []any{"new", 1}},
		{"structured target keeps string", []any{1, 2}, "old", []any{1, 2}},
		{"decoded string list merges", []any{"a", "b"}, []any{"b", "c"}, []string{"a", "b", "c"}},
		{"decoded string list appends", []any{"a"}, "b", []string{"a", "b"}},
		{"named string slice merges", tags{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
		{"incoming named string slice merges", []string{"a"}, tags{"a", "b"}, []string{"a", "b"}},
		{"named string slice appends", tags{"a"}, "b", []string{"a", "b"}},
		{"string array merges", [2]string{"a", "b"}, []any{"b", "c"}, []string{"a", "b", "c"}},
		{"incoming string array merges", []string{"a"}, [2]string{"a", "b"}, []string{"a", "b"}},
		{"string array appends", [1]string{"a"}, "b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateMetadataTo(map[string]any{"key": tt.target}, map[string]any{"key": tt.incoming})
			value, exists := got["key"]
			if !exists || !reflect.DeepEqual(value, tt.want) {
				t.Fatalf("value = %#v, want %#v", got["key"], tt.want)
			}
		})
	}
}

func TestUpdateMetadataTo_NormalizesTypedStringCollections(t *testing.T) {
	type tags []string
	for name, incoming := range map[string]any{
		"named slice": tags{"a", "", "a", "b"},
		"array":       [4]string{"a", "", "a", "b"},
	} {
		t.Run(name, func(t *testing.T) {
			got := UpdateMetadataTo(map[string]any{}, map[string]any{"tags": incoming})
			if !reflect.DeepEqual(got["tags"], []string{"a", "b"}) {
				t.Fatalf("tags = %#v, want [a b]", got["tags"])
			}
		})
	}
}

func TestUpdateMetadataTo_DoesNotModifySharedListBackingArray(t *testing.T) {
	for _, incoming := range []any{"b", []string{"b"}} {
		t.Run(reflect.TypeOf(incoming).String(), func(t *testing.T) {
			backing := []string{"a", "untouched"}
			got := UpdateMetadataTo(map[string]any{"tags": backing[:1]}, map[string]any{"tags": incoming})
			if !reflect.DeepEqual(got["tags"], []string{"a", "b"}) {
				t.Fatalf("tags = %#v, want [a b]", got["tags"])
			}
			if !reflect.DeepEqual(backing, []string{"a", "untouched"}) {
				t.Fatalf("shared backing array changed: %v", backing)
			}
		})
	}
}

func TestUpdateMetadataTo_ListAppend(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{"tags": []string{"a", "b"}},
		map[string]any{"tags": []string{"c"}},
	)
	tags := result["tags"].([]string)
	if len(tags) != 3 {
		t.Errorf("tags should have 3 elements, got %v", tags)
	}
}

func TestUpdateMetadataTo_StringScalarOverwrite(t *testing.T) {
	// Scalar target is overwritten by the incoming scalar (mirrors Python).
	result := UpdateMetadataTo(
		map[string]any{"tags": "a"},
		map[string]any{"tags": "b"},
	)
	if result["tags"] != "b" {
		t.Errorf("tags = %v, want b", result["tags"])
	}
}

func TestUpdateMetadataTo_DeduplicateList(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{"tags": []string{"a", "b"}},
		map[string]any{"tags": []string{"b", "c"}},
	)
	tags := result["tags"].([]string)
	if len(tags) != 3 {
		t.Errorf("tags should be deduplicated union: got %v", tags)
	}
}

func TestUpdateMetadataTo_NonDictMeta(t *testing.T) {
	result := UpdateMetadataTo(map[string]any{"a": 1}, "not a dict")
	if result["a"] != 1 {
		t.Errorf("original should be unchanged for non-dict meta")
	}
}

func TestUpdateMetadataTo_EmptyInitial(t *testing.T) {
	result := UpdateMetadataTo(nil, map[string]any{"k": "v"})
	if len(result) != 0 {
		t.Errorf("should return empty when initial is nil")
	}
}

func TestUpdateMetadataTo_FilterEmptyStrings(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{},
		map[string]any{"tags": []any{"a", "", "b"}},
	)
	tags := result["tags"].([]string)
	if len(tags) != 2 {
		t.Errorf("empty strings should be filtered: got %v", tags)
	}
}

func TestUpdateMetadataTo_SkipOnlyEmptyStringsList(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{},
		map[string]any{"tags": []any{"", ""}},
	)
	if _, exists := result["tags"]; exists {
		t.Error("all-empty list should be skipped")
	}
}
