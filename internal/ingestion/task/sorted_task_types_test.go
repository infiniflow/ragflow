package task

import "testing"

// TestSortedTaskTypesEmptyIsNonNil pins the pre-refactor contract: an empty
// input set produced a non-nil empty slice (make([]string, 0, 0)), not nil.
// This matters for callers that serialize the result: nil marshals to `null`
// while an empty slice marshals to `[]`.
func TestSortedTaskTypesEmptyIsNonNil(t *testing.T) {
	got := sortedTaskTypes(map[string]struct{}{})
	if got == nil {
		t.Fatal("sortedTaskTypes(empty) returned nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("sortedTaskTypes(empty) = %v, want empty", got)
	}
}
