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

package harness

import "testing"

// TestSufficiencyLadder_HardVeto asserts a code-proven evidence gap vetoes a full
// answer even when the AutoRater says sufficient.
func TestSufficiencyLadder_HardVeto(t *testing.T) {
	out := SufficiencyLadder(LadderInput{
		AutoSufficient: true, AutoConfidence: 0.9, AgentConfidence: 0.9,
		CHigh: 0.7, CLow: 0.4, LLMFloor: 0.5, AllowsReconcile: true,
		Cycle: 0, MaxCycles: 3,
		HardViolations: map[string][]string{"c2": {}},
	})
	if out.Action != ActionAnswerWithCaveat || out.ShouldContinue {
		t.Errorf("hard veto → caveated non-continuing answer, got action=%s continue=%v", out.Action, out.ShouldContinue)
	}
}

// TestSufficiencyLadder_InsufficientGap asserts !auto_sufficient with missing
// pieces → GAP (continue searching).
func TestSufficiencyLadder_InsufficientGap(t *testing.T) {
	out := SufficiencyLadder(LadderInput{
		AutoSufficient: false, AutoConfidence: 0.8, Missing: []string{"population figure"},
		AgentConfidence: 0.3, CHigh: 0.7, CLow: 0.4, LLMFloor: 0.5,
		AllowsReconcile: true, Cycle: 0, MaxCycles: 3,
	})
	if out.Action != ActionGap || !out.ShouldContinue {
		t.Errorf("insufficient with missing → GAP + continue, got action=%s continue=%v", out.Action, out.ShouldContinue)
	}
}

// TestSufficiencyLadder_UnanswerableNoMissing asserts !auto_sufficient without
// missing pieces → UNANSWERABLE.
func TestSufficiencyLadder_UnanswerableNoMissing(t *testing.T) {
	out := SufficiencyLadder(LadderInput{
		AutoSufficient: false, AutoConfidence: 0.8, AgentConfidence: 0.1,
		CHigh: 0.7, CLow: 0.4, LLMFloor: 0.5, AllowsReconcile: true,
		Cycle: 0, MaxCycles: 3,
	})
	if out.Action != ActionUnanswerable {
		t.Errorf("insufficient + no missing → UNANSWERABLE, got %s", out.Action)
	}
}

// TestSufficiencyLadder_Reconcile asserts low AutoRater confidence with reconcile
// enabled → RECONCILE (continue).
func TestSufficiencyLadder_Reconcile(t *testing.T) {
	out := SufficiencyLadder(LadderInput{
		AutoSufficient: true, AutoConfidence: 0.3, AgentConfidence: 0.6,
		CHigh: 0.7, CLow: 0.4, LLMFloor: 0.5, AllowsReconcile: true,
		Cycle: 0, MaxCycles: 3,
	})
	if out.Action != ActionReconcile || !out.ShouldContinue {
		t.Errorf("low AutoRater confidence + reconcile → RECONCILE, got action=%s continue=%v", out.Action, out.ShouldContinue)
	}
}

// TestSufficiencyLadder_AnswerConfidenceTiers asserts agent confidence drives
// ANSWER vs ANSWER_WITH_CAVEAT.
func TestSufficiencyLadder_AnswerConfidenceTiers(t *testing.T) {
	// high confidence → ANSWER.
	hi := SufficiencyLadder(LadderInput{
		AutoSufficient: true, AutoConfidence: 0.9, AgentConfidence: 0.9,
		CHigh: 0.7, CLow: 0.4, LLMFloor: 0.5, AllowsReconcile: true,
		Cycle: 0, MaxCycles: 3,
	})
	if hi.Action != ActionAnswer {
		t.Errorf("high confidence → ANSWER, got %s", hi.Action)
	}
	// mid confidence → ANSWER_WITH_CAVEAT.
	mid := SufficiencyLadder(LadderInput{
		AutoSufficient: true, AutoConfidence: 0.9, AgentConfidence: 0.5,
		CHigh: 0.7, CLow: 0.4, LLMFloor: 0.5, AllowsReconcile: true,
		Cycle: 0, MaxCycles: 3,
	})
	if mid.Action != ActionAnswerWithCaveat {
		t.Errorf("mid confidence → ANSWER_WITH_CAVEAT, got %s", mid.Action)
	}
}

// TestSufficiencyLadder_ContradictionSurfacesCaveat asserts contradictions force a
// caveated answer regardless of confidence.
func TestSufficiencyLadder_ContradictionSurfacesCaveat(t *testing.T) {
	out := SufficiencyLadder(LadderInput{
		AutoSufficient: true, AutoConfidence: 0.9, AgentConfidence: 0.9,
		Contradictions: []string{"2,161,000 vs 2,145,906"},
		CHigh:          0.7, CLow: 0.4, LLMFloor: 0.5, AllowsReconcile: true,
		Cycle: 0, MaxCycles: 3,
	})
	if out.Action != ActionAnswerWithCaveat {
		t.Errorf("contradiction → ANSWER_WITH_CAVEAT, got %s", out.Action)
	}
}

// TestAggregateAgentConfidence asserts the mean excludes unverified claims and
// hard-violated claims.
func TestAggregateAgentConfidence(t *testing.T) {
	results := []AgentResult{
		{ClaimID: "a", IsVerified: true, Confidence: 0.9},
		{ClaimID: "b", IsVerified: true, Confidence: 0.5},
		{ClaimID: "c", IsVerified: false, Confidence: 0.9}, // unverified → excluded
	}
	got := AggregateAgentConfidence(results, map[string][]string{"b": {}}) // b violated → excluded
	if got != 0.9 {
		t.Errorf("AggregateAgentConfidence = %v, want 0.9 (only a counts)", got)
	}
}
