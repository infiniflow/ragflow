package utility

import (
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

func TestUpdateMetadataTo_ExistingKeyMergedToList(t *testing.T) {
	result := UpdateMetadataTo(map[string]any{"author": "Alice"}, map[string]any{"author": "Bob"})
	tags, ok := result["author"].([]string)
	if !ok {
		t.Fatalf("author should be []string, got %T", result["author"])
	}
	if len(tags) != 2 || tags[0] != "Alice" || tags[1] != "Bob" {
		t.Errorf("author = %v, want [Alice Bob]", tags)
	}
}

func TestUpdateMetadataTo_StringValuesOnly(t *testing.T) {
	result := UpdateMetadataTo(map[string]any{}, map[string]any{"num": 42, "ok": true})
	if _, exists := result["num"]; exists {
		t.Errorf("numeric values should be skipped")
	}
	if _, exists := result["ok"]; exists {
		t.Errorf("bool values should be skipped")
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

func TestUpdateMetadataTo_StringToListMerge(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{"tags": "a"},
		map[string]any{"tags": "b"},
	)
	tags := result["tags"].([]string)
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %v, want [a b]", tags)
	}
}

func TestUpdateMetadataTo_DeduplicateList(t *testing.T) {
	result := UpdateMetadataTo(
		map[string]any{"tags": "a"},
		map[string]any{"tags": "a"},
	)
	tags := result["tags"].([]string)
	if len(tags) != 1 {
		t.Errorf("tags should be deduplicated: got %v", tags)
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
