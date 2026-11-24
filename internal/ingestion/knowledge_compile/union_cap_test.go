package knowledge_compile

import "testing"

// TestUnionCapEmptyIsNonNil pins the pre-refactor contract: the union of two
// empty id lists produced a non-nil empty slice, not nil. Callers may marshal
// this into a JSON field where nil and [] differ.
func TestUnionCapEmptyIsNonNil(t *testing.T) {
	got := unionCap(nil, nil)
	if got == nil {
		t.Fatal("unionCap(nil, nil) returned nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("unionCap(nil, nil) = %v, want empty", got)
	}
}
