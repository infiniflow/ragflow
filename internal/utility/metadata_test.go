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

func TestUpdateMetadataTo_ExistingKeyScalarOverwrite(t *testing.T) {
	// Python update_metadata_to overwrites a scalar target with the incoming
	// (merged-in) value rather than list-ifying it.
	result := UpdateMetadataTo(map[string]any{"author": "Alice"}, map[string]any{"author": "Bob"})
	if result["author"] != "Bob" {
		t.Errorf("author = %v, want Bob (incoming scalar overwrites)", result["author"])
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
