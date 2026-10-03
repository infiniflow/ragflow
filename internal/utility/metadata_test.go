package utility

import (
	"encoding/json"
	"math"
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

func TestUpdateMetadataTo_PreservesJSONCompatibleValues(t *testing.T) {
	type record struct{ Name string }
	typedRecords := []record{{Name: "first"}}
	structuredList := []any{map[string]any{"depth": 1}}
	structuredMap := map[string]any{"nested": true}
	numberList := []json.Number{"1", "2"}
	result := UpdateMetadataTo(map[string]any{}, map[string]any{
		"empty_string": "", "string_list": []any{"", "one", "one", "two"},
		"bool": true, "integer": int64(7), "unsigned": uint32(8), "floating": 2.5,
		"number": json.Number("9.75"), "number_list": numberList, "nil": nil, "map": structuredMap,
		"structured_list": structuredList, "typed_slice": typedRecords,
	})
	want := map[string]any{
		"empty_string": "", "string_list": []string{"", "one", "two"},
		"bool": true, "integer": int64(7), "unsigned": uint32(8), "floating": 2.5,
		"number": json.Number("9.75"), "number_list": numberList, "nil": nil, "map": structuredMap,
		"structured_list": structuredList, "typed_slice": typedRecords,
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("metadata = %#v, want %#v", result, want)
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
	if result != nil {
		t.Fatalf("nil target = %#v, want nil", result)
	}
}

func TestUpdateMetadataTo_FilterEmptyStrings(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{},
		map[string]any{"tags": []any{"a", "", "b"}},
	)
	tags := result["tags"].([]string)
	if !reflect.DeepEqual(tags, []string{"a", "", "b"}) {
		t.Errorf("empty strings should be retained: got %v", tags)
	}
}

func TestUpdateMetadataTo_EmptyListsAndUnsupportedValuesAreSkipped(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{"kept": "value"},
		map[string]any{
			"empty_strings": []any{}, "empty_typed": []int{},
			"function": func() {}, "channel": make(chan int),
		},
	)
	if !reflect.DeepEqual(result, map[string]any{"kept": "value"}) {
		t.Fatalf("metadata = %#v, want only kept value", result)
	}
}

func TestUpdateMetadataTo_JSONBoundaryAdversarialValues(t *testing.T) {
	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := []any{nil}
	cyclicSlice[0] = cyclicSlice

	result := UpdateMetadataTo(map[string]any{"kept": "value"}, map[string]any{
		"nested_function": map[string]any{"items": []any{"valid", func() {}}},
		"invalid_number":  json.Number("not-a-number"),
		"nested_number":   []any{json.Number("not-a-number")},
		"nan":             math.NaN(),
		"positive_inf":    math.Inf(1),
		"negative_inf":    math.Inf(-1),
		"cyclic_map":      cyclicMap,
		"cyclic_slice":    cyclicSlice,
	})
	if !reflect.DeepEqual(result, map[string]any{"kept": "value"}) {
		t.Fatalf("metadata = %#v, want only the original valid value", result)
	}
}

func TestUpdateMetadataTo_MarshalableStructuredValuesStayConsistent(t *testing.T) {
	type record struct{ Name string }
	type namedMetadata map[string]any
	type namedString string
	type namedStringMetadata map[namedString]any

	directRecord := record{Name: "direct"}
	recordList := []record{{Name: "nested"}}
	integerKeyMap := map[int]string{7: "seven"}
	structuredArray := [1]record{{Name: "array"}}
	result := UpdateMetadataTo(map[string]any{}, namedMetadata{
		"direct_record": directRecord,
		"record_list":   recordList,
		"integer_map":   integerKeyMap,
		"array":         structuredArray,
	})
	result = UpdateMetadataTo(result, namedStringMetadata{namedString("named_key"): "named value"})

	want := map[string]any{
		"direct_record": directRecord,
		"record_list":   recordList,
		"integer_map":   integerKeyMap,
		"array":         structuredArray,
		"named_key":     "named value",
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("metadata = %#v, want %#v", result, want)
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("metadata must remain JSON-serializable: %v", err)
	}
}

func TestUpdateMetadataTo_RejectsUnmarshalableMapKeysAndNestedValues(t *testing.T) {
	result := UpdateMetadataTo(map[string]any{}, map[string]any{
		"bool_key_map": map[bool]string{true: "unsupported"},
		"nested":       map[string]any{"bad_key_map": map[bool]string{true: "unsupported"}},
	})
	if len(result) != 0 {
		t.Fatalf("metadata = %#v, want no unsupported values", result)
	}

	result = UpdateMetadataTo(map[string]any{"kept": "value"}, map[int]any{1: "not metadata"})
	if !reflect.DeepEqual(result, map[string]any{"kept": "value"}) {
		t.Fatalf("top-level metadata keys must be strings: %#v", result)
	}
}

func TestUpdateMetadataTo_TypedNilsFollowJSONBoundary(t *testing.T) {
	type record struct{ Name string }
	var nilPointer *record
	var nilMap map[string]any
	var nilSlice []string

	result := UpdateMetadataTo(map[string]any{}, map[string]any{
		"pointer": nilPointer,
		"map":     nilMap,
		"slice":   nilSlice,
	})
	if got, ok := result["pointer"].(*record); !ok || got != nil {
		t.Fatalf("pointer = %#v, want typed nil *record", result["pointer"])
	}
	if got, ok := result["map"].(map[string]any); !ok || got != nil {
		t.Fatalf("map = %#v, want typed nil map[string]any", result["map"])
	}
	if _, exists := result["slice"]; exists {
		t.Fatalf("empty incoming typed nil slice must be skipped: %#v", result["slice"])
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("metadata must remain JSON-serializable: %v", err)
	}
}

// TestUpdateMetadataTo_ScalarScalarKeepsExistingPythonParity pins the exact
// Python update_metadata_to (common/metadata_utils.py:301) scalar semantics for
// the "re-ingest + same key + both sides scalar" scenario: the loop iterates
// the second argument (existing_meta), so when the target already holds a
// scalar the EXISTING (stored) value wins — NOT a [old,new] list. This guards
// against a regression where scalar+scalar would be list-ified.
func TestUpdateMetadataTo_ScalarScalarKeepsExistingPythonParity(t *testing.T) {
	// mergeDocMetadata calls UpdateMetadataTo(new, existing), so existing is
	// the second argument and must win for a shared scalar key.
	result := UpdateMetadataTo(
		map[string]any{"author": "NEWLY_EXTRACTED"},
		map[string]any{"author": "STORED"},
	)
	if result["author"] != "STORED" {
		t.Errorf("author = %v, want STORED (existing scalar wins, Python parity)", result["author"])
	}
}

func TestUpdateMetadataTo_CollisionMatrix(t *testing.T) {
	structured := []map[string]any{{"title": "outline"}}
	for name, tc := range map[string]struct {
		target map[string]any
		meta   map[string]any
		want   any
	}{
		"all string lists merge in order": {
			target: map[string]any{"key": []any{"old", "shared"}},
			meta:   map[string]any{"key": []string{"shared", "new"}},
			want:   []string{"old", "shared", "new"},
		},
		"scalar target is replaced by incoming list": {
			target: map[string]any{"key": "old"}, meta: map[string]any{"key": []string{"new"}}, want: []string{"new"},
		},
		"string input cannot replace structured list": {
			target: map[string]any{"key": structured}, meta: map[string]any{"key": "extracted"}, want: structured,
		},
		"list input cannot extend typed structured list": {
			target: map[string]any{"key": []int{1, 2}}, meta: map[string]any{"key": []any{"extracted"}}, want: []int{1, 2},
		},
		"string list replaces non-list structured value": {
			target: map[string]any{"key": map[string]any{"old": true}}, meta: map[string]any{"key": []string{"extracted"}}, want: []string{"extracted"},
		},
		"structured incoming value does not replace existing": {
			target: map[string]any{"key": "old"}, meta: map[string]any{"key": map[string]any{"new": true}}, want: "old",
		},
		"bool replaces structured list": {
			target: map[string]any{"key": structured}, meta: map[string]any{"key": false}, want: false,
		},
		"number replaces map": {
			target: map[string]any{"key": map[string]any{"old": true}}, meta: map[string]any{"key": json.Number("12")}, want: json.Number("12"),
		},
		"nil replaces string": {
			target: map[string]any{"key": "old"}, meta: map[string]any{"key": nil}, want: nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := UpdateMetadataTo(tc.target, tc.meta)
			if !reflect.DeepEqual(result["key"], tc.want) {
				t.Fatalf("key = %#v, want %#v", result["key"], tc.want)
			}
		})
	}
}
