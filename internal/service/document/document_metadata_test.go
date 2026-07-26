package document

import "testing"

func TestCloneDocumentMetadata(t *testing.T) {
	if got := cloneDocumentMetadata(nil); got == nil {
		t.Fatal("nil input should return a non-nil empty map")
	}

	orig := map[string]interface{}{
		"list": []interface{}{"a", "b"},
		"str":  "x",
	}
	clone := cloneDocumentMetadata(orig)

	// Mutating the clone must not affect the original.
	clone["str"] = "changed"
	if orig["str"] != "x" {
		t.Error("clone mutation leaked into original scalar")
	}
	cloneList := clone["list"].([]interface{})
	cloneList[0] = "mutated"
	if origList, ok := orig["list"].([]interface{}); ok && origList[0] == "mutated" {
		t.Error("nested slice was not deep-copied")
	}
}

func TestDocumentMetadataValuesEqual(t *testing.T) {
	if !documentMetadataValuesEqual(1, "1") {
		t.Error("1 and \"1\" should compare equal (formatted form)")
	}
	if !documentMetadataValuesEqual(int64(1), "1") {
		t.Error("int64(1) and \"1\" should compare equal")
	}
	if documentMetadataValuesEqual("a", "b") {
		t.Error("distinct values should differ")
	}
}

func TestNormalizeMetadataListValue(t *testing.T) {
	if _, ok := normalizeMetadataListValue("scalar"); ok {
		t.Error("a scalar should not be reported as a list")
	}
	list, ok := normalizeMetadataListValue([]interface{}{"a", "b"})
	if !ok || len(list) != 2 {
		t.Errorf("[]interface{} should normalize: %v (ok=%v)", list, ok)
	}
	list2, ok2 := normalizeMetadataListValue([]string{"a", "b"})
	if !ok2 || len(list2) != 2 {
		t.Errorf("[]string should normalize: %v (ok=%v)", list2, ok2)
	}
}

func TestFirstScalarMetadataValue(t *testing.T) {
	if v, ok := firstScalarMetadataValue([]interface{}{"a", "b"}); !ok || v != "a" {
		t.Errorf("should return first non-nil scalar: %v (ok=%v)", v, ok)
	}
	if _, ok := firstScalarMetadataValue(nil); ok {
		t.Error("nil should report not-found")
	}
}

func TestNormalizeDocumentMetadataUpdateValue(t *testing.T) {
	if v := normalizeDocumentMetadataUpdateValue("42", "number"); v != int64(42) {
		t.Errorf("number string → int64(42), got %v (%T)", v, v)
	}
	list, ok := normalizeDocumentMetadataUpdateValue([]interface{}{"a"}, "list").([]interface{})
	if !ok || len(list) != 1 {
		t.Errorf("list normalize failed: %v (ok=%v)", list, ok)
	}
	if v := normalizeDocumentMetadataUpdateValue(123, "string"); v != "123" {
		t.Errorf("string normalize: got %v", v)
	}
	if v := normalizeDocumentMetadataUpdateValue("bar", "unknown"); v != "bar" {
		t.Errorf("unknown valueType should pass through: got %v", v)
	}
}

func TestAggregateMetadata(t *testing.T) {
	chunks := []map[string]interface{}{
		{"meta_fields": map[string]interface{}{"author": "alice"}},
		{"meta_fields": map[string]interface{}{"author": "bob"}},
		{"meta_fields": map[string]interface{}{"author": "alice"}},
	}
	result := aggregateMetadata(chunks)

	field, ok := result["author"].(map[string]interface{})
	if !ok {
		t.Fatalf("author field missing from summary: %v", result)
	}
	values, ok := field["values"].([][2]interface{})
	if !ok {
		t.Fatalf("values has unexpected shape: %v", field["values"])
	}
	counts := map[string]int{}
	for _, pair := range values {
		if s, ok := pair[0].(string); ok {
			counts[s] = pair[1].(int)
		}
	}
	if counts["alice"] != 2 {
		t.Errorf("alice count = %d, want 2", counts["alice"])
	}
	if counts["bob"] != 1 {
		t.Errorf("bob count = %d, want 1", counts["bob"])
	}
}
