package harness

import (
	"context"
	"testing"
)

// TestCrossCheckClaim_NumberMatch asserts a verified report whose numbers appear
// in the evidence chunk passes the cross-check.
func TestCrossCheckClaim_NumberMatch(t *testing.T) {
	agent := &AgentResult{ClaimID: "c0", IsVerified: true, Report: "speed is 88 and 12", EvidenceIDs: []int{0}}
	chunks := map[int]map[string]interface{}{0: {"content_with_weight": "speed is 88 and 12 here"}}
	r := CrossCheckClaim(agent, chunks)
	if !r.CrossCheckPassed {
		t.Errorf("cross-check should pass, got mismatches %v", r.Mismatches)
	}
}

// TestKbinfosMerge_GlobalIndices asserts Merge returns the GLOBAL indices of the
// contributed chunks in the accumulated list, so a later claim's EvidenceIDs
// still resolve correctly after earlier claims pushed more chunks in.
func TestKbinfosMerge_GlobalIndices(t *testing.T) {
	kb := &Kbinfos{}
	// First claim's search returns chunks a,b.
	first := kb.Merge([]map[string]interface{}{
		{"chunk_id": "a", "content_with_weight": "alpha 7"},
		{"chunk_id": "b", "content_with_weight": "beta"},
	}, nil)
	// a->0, b->1 in the global list.
	if len(first) != 2 || first[0] != 0 || first[1] != 1 {
		t.Fatalf("first merge global indices = %v, want [0 1]", first)
	}
	// Second claim's search returns chunk c (now global index 2) and a dup of a.
	second := kb.Merge([]map[string]interface{}{
		{"chunk_id": "c", "content_with_weight": "gamma 9"},
		{"chunk_id": "a", "content_with_weight": "alpha 7"},
	}, nil)
	// c->2, a(dup)->0; NOT per-search [0 1].
	if len(second) != 2 || second[0] != 2 || second[1] != 0 {
		t.Fatalf("second merge global indices = %v, want [2 0]", second)
	}
	// CrossCheckClaim must resolve both claims' evidence against the global list.
	allChunks := map[int]map[string]interface{}{}
	for i, c := range kb.Chunks {
		allChunks[i] = c
	}
	r1 := CrossCheckClaim(&AgentResult{ClaimID: "c1", IsVerified: true, Report: "alpha 7", EvidenceIDs: first}, allChunks)
	if !r1.HasEvidence {
		t.Error("claim 1 should have evidence at global indices")
	}
	r2 := CrossCheckClaim(&AgentResult{ClaimID: "c2", IsVerified: true, Report: "gamma 9", EvidenceIDs: second}, allChunks)
	if !r2.HasEvidence {
		t.Error("claim 2 should have evidence at global index 2 (not per-search 0)")
	}
}

// TestCrossCheckClaim_Unverified asserts an unverified agent fails the check.
func TestCrossCheckClaim_Unverified(t *testing.T) {
	r := CrossCheckClaim(&AgentResult{ClaimID: "c0", IsVerified: false}, nil)
	if r.CrossCheckPassed {
		t.Error("unverified agent must fail cross-check")
	}
}

// TestComputeFusionScore_Sufficient asserts a fully-verified high-score set is
// SUFFICIENT.
func TestComputeFusionScore_Sufficient(t *testing.T) {
	agents := []AgentResult{{ClaimID: "c0", IsVerified: true, Report: "value 42", EvidenceIDs: []int{0}}}
	cross := []ClaimCrossCheckResult{{ClaimID: "c0", CrossCheckPassed: true, CrossCheckScore: 1.0, HasEvidence: true}}
	v := ComputeFusionScore(agents, cross, THINKING_MODES["medium"])
	if v.Status != "SUFFICIENT" {
		t.Errorf("status = %q, want SUFFICIENT", v.Status)
	}
}

// TestComputeFusionScore_NoEvidence asserts a claim with no examined evidence is
// UNANSWERABLE (empty-evidence guard).
func TestComputeFusionScore_NoEvidence(t *testing.T) {
	cross := []ClaimCrossCheckResult{{ClaimID: "c0", CrossCheckPassed: true, CrossCheckScore: 1.0, HasEvidence: false}}
	v := ComputeFusionScore([]AgentResult{{ClaimID: "c0", IsVerified: true}}, cross, THINKING_MODES["medium"])
	if v.Status != "UNANSWERABLE" {
		t.Errorf("status = %q, want UNANSWERABLE", v.Status)
	}
	if len(v.MissingClaims) != 1 {
		t.Errorf("missing claims = %v, want [c0]", v.MissingClaims)
	}
}

// TestRouteSufficiencyVerdict asserts SUFFICIENT → ANSWER.
func TestRouteSufficiencyVerdict(t *testing.T) {
	action, cont := RouteSufficiencyVerdict(SufficiencyVerdict{Status: "SUFFICIENT", Score: 0.9}, "medium", 0, 3)
	if action != "ANSWER" || cont {
		t.Errorf("got (%q,%v), want (ANSWER,false)", action, cont)
	}
}

// TestDirectSearch_Merges asserts direct search merges chunks and flags empty.
func TestDirectSearch_Merges(t *testing.T) {
	kb := &Kbinfos{}
	res := DirectSearch(context.Background(), func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
		return []map[string]interface{}{{"chunk_id": "a", "content_with_weight": "alpha"}}, nil
	}, "q", "", kb)
	if res.EmptyResult || !kb.HasChunks() {
		t.Errorf("expected merged chunks, empty=%v chunks=%d", res.EmptyResult, len(kb.Chunks))
	}
}

// TestDirectSearch_Empty asserts direct search flags empty when no chunks.
func TestDirectSearch_Empty(t *testing.T) {
	kb := &Kbinfos{}
	res := DirectSearch(context.Background(), func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
		return nil, nil
	}, "q", "", kb)
	if !res.EmptyResult {
		t.Error("expected empty_result=true")
	}
}

// TestDecomposeAndSearch_Verifies asserts a searchable claim gets verified and
// the loop stops on ANSWER.
func TestDecomposeAndSearch_Verifies(t *testing.T) {
	claims := []*ClaimTarget{{ClaimID: "c0", Description: "fact 42 about X"}}
	kb := &Kbinfos{}
	res := DecomposeAndSearch(context.Background(),
		func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
			return []map[string]interface{}{{"chunk_id": "x", "content_with_weight": "the value is 42"}}, nil
		},
		"Q", "", claims, "medium", kb)
	if !claims[0].IsVerified {
		t.Error("claim should be verified")
	}
	if res.Verdict == nil {
		t.Error("expected a verdict")
	}
}
