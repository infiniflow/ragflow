package common_test

import (
	"reflect"
	"testing"

	"ragflow/internal/common"
)

func TestDeepCopyMap_NilReturnsNil(t *testing.T) {
	var src map[string]any
	got := common.DeepCopyMap(src)
	if got != nil {
		t.Fatalf("DeepCopyMap(nil) = %v, want nil", got)
	}
}

func TestDeepCopyMap_EmptyReturnsEmpty(t *testing.T) {
	src := map[string]any{}
	got := common.DeepCopyMap(src)
	if got == nil {
		t.Fatal("DeepCopyMap(empty) = nil, want non-nil empty map")
	}
	if len(got) != 0 {
		t.Fatalf("DeepCopyMap(empty) len = %d, want 0", len(got))
	}
}

func TestDeepCopyMap_PreservesPrimitives(t *testing.T) {
	src := map[string]any{
		"s": "hello",
		"i": 42,
		"f": 3.14,
		"b": true,
	}
	got := common.DeepCopyMap(src)
	for k, v := range src {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("key %q: got %#v, want %#v", k, got[k], v)
		}
	}
}

func TestDeepCopyMap_NestedMapIsIndependent(t *testing.T) {
	inner := map[string]any{"x": 1}
	src := map[string]any{"nested": inner}
	got := common.DeepCopyMap(src)

	// Mutating the original nested map must not affect the copy.
	inner["x"] = 999
	if got["nested"].(map[string]any)["x"] != 1 {
		t.Fatalf("copy shares nested map reference: got %v, want 1", got["nested"].(map[string]any)["x"])
	}
	// Mutating the copy must not affect the original.
	got["nested"].(map[string]any)["x"] = 7
	if inner["x"] != 999 {
		t.Fatalf("copy mutation leaked to original: got %v, want 999", inner["x"])
	}
}

func TestDeepCopyMap_NestedSliceIsIndependent(t *testing.T) {
	slice := []any{1, 2, 3}
	src := map[string]any{"list": slice}
	got := common.DeepCopyMap(src)

	slice[0] = 999
	if got["list"].([]any)[0].(int) != 1 {
		t.Fatalf("copy shares slice reference: got %v, want 1", got["list"].([]any)[0])
	}
	got["list"].([]any)[0] = 7
	if slice[0] != 999 {
		t.Fatalf("copy slice mutation leaked: got %v, want 999", slice[0])
	}
}

// Adversarial: a Go int must survive the copy with its exact type, not be
// coerced to float64 (the classic JSON round-trip pitfall). This guards the
// non-collection "return by reference" contract against regressions that would
// silently widen int → float64 for JSON-native data.
func TestDeepCopyMap_IntTypePreserved(t *testing.T) {
	src := map[string]any{"n": 42}
	got := common.DeepCopyMap(src)
	if v, ok := got["n"].(int); !ok || v != 42 {
		t.Fatalf("int not preserved: got %#v (%T)", got["n"], got["n"])
	}
}

// Adversarial: non-collection values (structs, unlike nested maps/slices) are
// shared by reference, not deep-copied. This documents and locks the contract:
// the helper only deep-copies maps and slices; everything else is returned as-is.
func TestDeepCopyMap_NonCollectionSharedByReference(t *testing.T) {
	type payload struct{ X int }
	inner := payload{X: 1}
	src := map[string]any{"p": inner}
	got := common.DeepCopyMap(src)
	if _, ok := got["p"].(payload); !ok {
		t.Fatalf("non-collection value type changed: %T", got["p"])
	}
	if got["p"].(payload) != inner {
		t.Fatal("non-collection value was copied instead of shared by reference")
	}
}

// Adversarial: []byte values must survive the copy without base64 corruption.
func TestDeepCopyMap_ByteSliceSafe(t *testing.T) {
	src := map[string]any{"raw": []byte("secret-bytes")}
	got := common.DeepCopyMap(src)
	bs, ok := got["raw"].([]byte)
	if !ok {
		t.Fatalf("[]byte value changed type: %T", got["raw"])
	}
	if string(bs) != "secret-bytes" {
		t.Fatalf("[]byte corrupted: got %q", string(bs))
	}
	bs[0] = 'X'
	if src["raw"].([]byte)[0] != 's' {
		t.Fatal("[]byte copy shares backing array")
	}
}

func TestDeepCopySlice_NilReturnsNil(t *testing.T) {
	var src []any
	if got := common.DeepCopySlice(src); got != nil {
		t.Fatalf("DeepCopySlice(nil) = %v, want nil", got)
	}
}

func TestDeepCopySlice_Independent(t *testing.T) {
	inner := map[string]any{"x": 1}
	src := []any{inner, "v"}
	got := common.DeepCopySlice(src)
	inner["x"] = 999
	if got[0].(map[string]any)["x"] != 1 {
		t.Fatal("DeepCopySlice shares nested map")
	}
}
