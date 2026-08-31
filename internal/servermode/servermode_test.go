// The gate predicate is pure policy with no cgo dependency, so it is tested in
// the default unit tier and runs without the native static libraries.
package servermode

import "testing"

func TestNeedsDeepDoc(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		// Modes that run the document parsing pipeline:
		{"api", true},      // serves dataflow debug, which parses in-process
		{"ingestor", true}, // runs the document ingestion/parsing pipeline
		// Modes that never instantiate the DeepDoc analyzer:
		{"admin", false},  // management UI
		{"syncer", false}, // datasource sync
		// Defensive against unknown/empty modes: do not fail-fast.
		{"", false},
		{"unknown", false},
	}
	for _, c := range cases {
		if got := NeedsDeepDoc(c.mode); got != c.want {
			t.Errorf("NeedsDeepDoc(%q) = %v, want %v", c.mode, got, c.want)
		}
	}
}
