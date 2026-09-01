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

// TestFilterRelevantNumbers asserts 0<n<1 noise is dropped and duplicates are
// collapsed.
func TestFilterRelevantNumbers(t *testing.T) {
	got := filterRelevantNumbers([]float64{0.5, 0.75, 1976, 48, 48, 1976, 0})
	// 0.5/0.75 dropped (in (0,1)); 48/1976 deduped; 0 kept.
	want := []float64{1976, 48, 0}
	if len(got) != len(want) {
		t.Fatalf("filterRelevantNumbers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("filterRelevantNumbers[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestBoundedPhraseMatch asserts "Ann" does not match "Annual".
func TestBoundedPhraseMatch(t *testing.T) {
	if boundedPhraseMatch("Annual report 2023", "ann") {
		t.Error(`"ann" must not match inside "Annual"`)
	}
	if !boundedPhraseMatch("Ann is the CEO", "ann") {
		t.Error(`"ann" must match as a standalone word`)
	}
	if !boundedPhraseMatch("value 1976 was", "1976") {
		t.Error(`"1976" must match bounded`)
	}
	if boundedPhraseMatch("value 19760 was", "1976") {
		t.Error(`"1976" must not match inside "19760"`)
	}
}

// TestDetectNumericConflict asserts close-but-different figures (ratio ≤1.3) are
// flagged while far-apart figures are not.
func TestDetectNumericConflict(t *testing.T) {
	conflict := detectNumericConflict([]string{
		"2,161,000 from Wikipedia Demographics of Paris",
		"2,145,906 from INSEE",
	})
	if len(conflict) != 1 {
		t.Fatalf("close figures must conflict, got %v", conflict)
	}
	// Far-apart figures are different quantities → no conflict.
	none := detectNumericConflict([]string{"100 from A", "9000 from B"})
	if len(none) != 0 {
		t.Errorf("far-apart figures must not conflict, got %v", none)
	}
}

// TestCrossCheckClaim_UnionMatching asserts a fact verified by ONE chunk is
// verified (union semantics, not per-chunk).
func TestCrossCheckClaim_UnionMatching(t *testing.T) {
	allChunks := map[int]map[string]interface{}{
		0: {"content_with_weight": "Paris population is 2161000 in 2019"},
		1: {"content_with_weight": "unrelated text about trains"},
	}
	r := CrossCheckClaim(&AgentResult{
		ClaimID: "c1", IsVerified: true, Report: "Paris has 2161000 residents",
		EvidenceIDs: []int{0, 1},
	}, allChunks)
	if !r.CrossCheckPassed {
		t.Errorf("number found in chunk 0 must verify the claim (union), mismatches=%v", r.Mismatches)
	}
}

// TestCrossCheckClaim_NoEvidenceFails asserts a claim with no evidence ids fails.
func TestCrossCheckClaim_NoEvidenceFails(t *testing.T) {
	r := CrossCheckClaim(&AgentResult{
		ClaimID: "c1", IsVerified: true, Report: "value 42",
	}, map[int]map[string]interface{}{})
	if r.CrossCheckPassed || r.CrossCheckScore != 0.0 {
		t.Errorf("no evidence → fail with score 0, got passed=%v score=%v", r.CrossCheckPassed, r.CrossCheckScore)
	}
}

// TestCrossCheckClaim_NumericConflictCaps asserts a numeric conflict caps the
// claim below the pass floor.
func TestCrossCheckClaim_NumericConflictCaps(t *testing.T) {
	allChunks := map[int]map[string]interface{}{
		0: {"content_with_weight": "population 2161000"},
		1: {"content_with_weight": "population 2145906"},
	}
	r := CrossCheckClaim(&AgentResult{
		ClaimID: "c1", IsVerified: true, Report: "population",
		EvidenceIDs: []int{0, 1},
		Numbers:     []string{"2,161,000 from A", "2,145,906 from B"},
	}, allChunks)
	if r.CrossCheckPassed {
		t.Error("numeric conflict must cap the claim below pass")
	}
}

// TestComputeFusionScore_HardViolations asserts a weak self-verified claim
// becomes a hard_violation and the status is INSUFFICIENT.
func TestComputeFusionScore_HardViolations(t *testing.T) {
	agents := []AgentResult{
		{ClaimID: "c0", IsVerified: true, Confidence: 0.9, EvidenceIDs: []int{0}},
		{ClaimID: "c1", IsVerified: true, Confidence: 0.9, EvidenceIDs: []int{1}},
	}
	// c1 cross-check score 0 (below floor) → hard violation.
	cross := []ClaimCrossCheckResult{
		{ClaimID: "c0", CrossCheckPassed: true, CrossCheckScore: 1.0, HasEvidence: true},
		{ClaimID: "c1", CrossCheckPassed: false, CrossCheckScore: 0.0, HasEvidence: true},
	}
	allChunks := map[int]map[string]interface{}{
		0: {"content_with_weight": "alpha 42"},
		1: {"content_with_weight": "beta"},
	}
	v := ComputeFusionScore(agents, cross, THINKING_MODES["high"], "Q", nil, allChunks)
	if len(v.HardViolations) != 1 || v.HardViolations[0] != "c1" {
		t.Errorf("hard_violations = %v, want [c1]", v.HardViolations)
	}
	if v.Status != "INSUFFICIENT" {
		t.Errorf("status = %q, want INSUFFICIENT (hard veto)", v.Status)
	}
}

// TestComputeFusionScore_NoiseExclusion asserts an unrelated self-unverified
// claim (cross<0.2) is excluded from hard violations but surfaced in missing.
func TestComputeFusionScore_NoiseExclusion(t *testing.T) {
	agents := []AgentResult{
		{ClaimID: "c0", IsVerified: true, Confidence: 0.9, EvidenceIDs: []int{0}},
		{ClaimID: "noise", IsVerified: false, Confidence: 0.0},
	}
	cross := []ClaimCrossCheckResult{
		{ClaimID: "c0", CrossCheckPassed: true, CrossCheckScore: 1.0, HasEvidence: true},
		{ClaimID: "noise", CrossCheckPassed: false, CrossCheckScore: 0.05, HasEvidence: true},
	}
	allChunks := map[int]map[string]interface{}{
		0: {"content_with_weight": "alpha 42"},
	}
	v := ComputeFusionScore(agents, cross, THINKING_MODES["high"], "Q", nil, allChunks)
	if len(v.HardViolations) != 0 {
		t.Errorf("noise claim must not be a hard violation, got %v", v.HardViolations)
	}
	if !containsStr(v.MissingClaims, "noise") {
		t.Errorf("noise claim must be surfaced in missing, got %v", v.MissingClaims)
	}
}
