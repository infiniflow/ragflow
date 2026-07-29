package common

import "testing"

// TestStableRowID_NULCollision locks that distinct part sequences can never
// share an id even when a part contains a NUL byte (length-prefixed encoding).
func TestStableRowID_NULCollision(t *testing.T) {
	a := StableRowID("a\x00b")
	b := StableRowID("a", "b")
	if a == b {
		t.Fatalf("StableRowID collided for [\"a\\x00b\"] and [\"a\",\"b\"]: %s", a)
	}
	// Determinism: same inputs always yield the same id.
	if StableRowID("a", "b") != b {
		t.Fatal("StableRowID is not deterministic")
	}
	// Order matters.
	if StableRowID("a", "b") == StableRowID("b", "a") {
		t.Fatal("StableRowID ignored part ordering")
	}
}
