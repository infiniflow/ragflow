package graph

import "testing"

// requireEngine skips the test when running with the graph package's own
// minimal test runner instead of the full pregel engine. Tests that need
// checkpoints, interrupts, time travel, or StateInspector must call this.
func requireEngine(t *testing.T) {
	t.Helper()
	if isTestRunner {
		t.Skip("requires full pregel engine — run from harness root for complete test")
	}
}
