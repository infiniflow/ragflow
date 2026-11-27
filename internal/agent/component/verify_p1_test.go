//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"sort"
	"strings"
	"testing"
)

// TestVerifyRegistration_P1 verifies required components are registered,
// lookup is case-insensitive, and names are returned in sorted order.
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
	if c, err := New("Message", map[string]any{"text": "registered"}); err != nil || c == nil {
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
