package component

import (
	"sort"
	"strings"
	"testing"
)

// TestVerifyRegistration_P1 verifies all components are registered,
// case-insensitive, and returned in sorted order. The expected count is
// read from plan §2.11.10 — P0 (8) + P1 (5) + P2 (4) + P3 (2) + P4 (3) = 22
// at plan completion, plus 7 v1 fixture stubs (Retrieval, TavilySearch,
// ExeSQL, Generate, Answer, Iteration, IterationItem) registered by
// v1_stubs.go to keep the dsl-examples e2e suite compiling. The test
// allows counts between 12 (P0+P1 minus the removed ExitLoop) and 30
// (the 22 plan components + the 7 v1 stubs + Parallel) to roll
// forward as subsequent batches land.
//
// Note: ExitLoop is intentionally NOT in the registry anymore. The
// canvas engine (internal/agent/canvas/canvas.go's legacyNoOpNames)
// accepts the name for DSL v1 compatibility but the Go port no longer
// ships a Component implementation for it — termination is now driven
// by the loop_termination_condition predicate, not by reaching an
// ExitLoop node in the body.
func TestVerifyRegistration_P1(t *testing.T) {
	names := RegisteredNames()
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n] = true
	}

	// Always-present P0+P1 (12 names — ExitLoop removed).
	requiredP0P1 := []string{
		"agent", "begin", "categorize", "dataoperations",
		"invoke", "listoperations", "llm", "message", "stringtransform",
		"switch", "variableaggregator", "variableassigner",
	}
	var missing []string
	for _, e := range requiredP0P1 {
		if !have[e] {
			missing = append(missing, e)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing P0/P1 components: %v (have %d: %v)", missing, len(names), names)
	}
	if got := len(names); got < 12 || got > 30 {
		t.Errorf("expected 12-30 registered (current plan scope + v1 stubs), got %d: %v", got, names)
	}

	// ExitLoop must NOT be in the registry (legacy compat lives at
	// the canvas level, not here).
	if have["exitloop"] {
		t.Errorf("exitloop is registered; expected gone. The legacy no-op handling lives in canvas.legacyNoOpNames.")
	}
	if have["loopitem"] {
		t.Errorf("loopitem is registered; expected gone. The Python-era LoopItem node is collapsed into the workflowx.AddLoopNode wrapper.")
	}

	// Case-insensitive lookup on a real component to prove the
	// registry's name normalization works at lookup time.
	if c, err := New("Message", nil); err != nil || c == nil {
		t.Errorf("New(Message) failed: c=%v err=%v", c, err)
	}

	// Sorted output for stable error messages.
	sortedCopy := make([]string, len(names))
	copy(sortedCopy, names)
	sort.Strings(sortedCopy)
	if !equalStrings(sortedCopy, names) {
		t.Errorf("RegisteredNames() not sorted: got %v", names)
	}

	t.Logf("OK — %d components registered, sorted, case-insensitive lookup works: %s", len(names), strings.Join(names, ", "))
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
