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

package advanced_rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/rag/advanced_rag/harness"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// failingModel errors on every Complete call — the batched-answer LLM seam.
type failingModel struct {
	mu    sync.Mutex
	calls int
}

func (m *failingModel) Complete(context.Context, []schema.Message, []harness.ToolSpec) (*harness.ModelReply, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return nil, errors.New("model unavailable")
}

// Calls reports how many times Complete was invoked.
func (m *failingModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// claimRow builds one claim pseudo-chunk (Python "[evidence] <name> — <desc>"
// row) the way the pool stores it.
func claimRow(id, content string) map[string]any {
	return map[string]any{
		"chunk_id":            id,
		"content_with_weight": content,
		"doc_id":              "d1",
	}
}

// ---------------------------------------------------------------------------
// PrefillSlotsFromEvidence（Python _prefill_slots_from_evidence）
// ---------------------------------------------------------------------------

// TestPrefillSlotsFromEvidence_CoverageBoundary pins the 0.6 word-coverage
// gate（Python _EVIDENCE_PREFILL_COVERAGE=0.6, :1738）: exactly-at and above
// prefill, below does not; the candidate is the de-prefixed first line and the
// strength is the coverage itself.
func TestPrefillSlotsFromEvidence_CoverageBoundary(t *testing.T) {
	// Clue terms: {alpha, tower, opened, paris, 1889} (>=3 code points only).
	clue := "alpha tower opened paris 1889"
	// Covers tower/opened/paris = 3/5 = exactly 0.6.
	atGate := claimRow("claim_a", "[evidence] Eiffel Tower — landmark\nEvidence (verbatim): \"the tower opened in paris near the river\"")
	// Covers tower/opened = 2/5 = 0.4.
	below := claimRow("claim_b", "[evidence] Eiffel Tower — landmark\nEvidence (verbatim): \"the tower opened here long ago\"")

	newState := func() harness.State {
		return harness.NewState([]harness.Variable{
			{ID: 1, Type: "aspect", QuestionClues: []string{clue}},
		}, 0, nil)
	}

	// At the gate: prefilled.
	st := newState()
	if n := PrefillSlotsFromEvidence(&st, &harness.Kbinfos{Chunks: []map[string]any{atGate}}); n != 1 {
		t.Fatalf("coverage exactly 0.6 must prefill, filled = %d", n)
	}
	v := st.State[0]
	if v.Candidate == nil || *v.Candidate != "Eiffel Tower — landmark" {
		t.Fatalf("candidate = %v, want the de-prefixed first line", v.Candidate)
	}
	if v.CandidateStrength == nil || *v.CandidateStrength != 0.6 {
		t.Fatalf("strength = %v, want 0.6", v.CandidateStrength)
	}

	// Below the gate: untouched.
	st = newState()
	if n := PrefillSlotsFromEvidence(&st, &harness.Kbinfos{Chunks: []map[string]any{below}}); n != 0 {
		t.Fatalf("coverage 0.4 must NOT prefill, filled = %d", n)
	}
	if st.State[0].Candidate != nil {
		t.Fatalf("candidate = %v, want nil", st.State[0].Candidate)
	}
}

// TestPrefillSlotsFromEvidence_Guards mirrors the skips（Python :1714-1728,
// :1744）: no claim rows in the pool, already-filled slots, blank clues, and a
// first line that strips to nothing.
func TestPrefillSlotsFromEvidence_Guards(t *testing.T) {
	st := harness.NewState([]harness.Variable{
		{ID: 1, Type: "aspect", QuestionClues: []string{"eiffel tower paris"}},
		{ID: 2, Type: "aspect", Candidate: strPtr("known")},
		{ID: 3, Type: "aspect", QuestionClues: []string{"   "}},
	}, 0, nil)

	// No claim rows: 0 regardless of content.
	if n := PrefillSlotsFromEvidence(&st, &harness.Kbinfos{Chunks: []map[string]any{
		claimRow("c1", "plain chunk"),
	}}); n != 0 {
		t.Fatalf("no claim_ rows must yield 0, got %d", n)
	}
	// nil KB is safe.
	if n := PrefillSlotsFromEvidence(&st, nil); n != 0 {
		t.Fatalf("nil KB must yield 0, got %d", n)
	}

	// Row whose head strips to nothing: even full coverage fills nothing.
	if n := PrefillSlotsFromEvidence(&st, &harness.Kbinfos{Chunks: []map[string]any{
		claimRow("claim_x", "[evidence]  — \nEvidence (verbatim): \"eiffel tower paris\""),
	}}); n != 0 {
		t.Fatalf("empty name must yield 0, got %d", n)
	}
	if st.State[0].Candidate != nil || st.State[2].Candidate != nil {
		t.Fatal("unfilled slots must stay untouched")
	}
	if st.State[1].Candidate == nil || *st.State[1].Candidate != "known" {
		t.Fatal("a filled slot must never be overwritten")
	}
}

// TestRunSlotResearchPass_PrefilledSlotsSkipSession pins the wiring（Python
// :1782-1790）: when the pooled evidence covers every slot at >= 0.6, the
// pass fills them WITHOUT running a single action session (zero model calls).
func TestRunSlotResearchPass_PrefilledSlotsSkipSession(t *testing.T) {
	kb := &harness.Kbinfos{Chunks: []map[string]any{
		claimRow("claim_a", "[evidence] Eiffel Tower — built in Paris by Gustave Eiffel, opened in 1889\nEvidence (verbatim): \"...\""),
	}}
	st := &AgenticState{
		Question: "what is the tower",
		SlotTable: harness.NewState([]harness.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"eiffel tower opened paris 1889"}},
			{ID: 1, Type: "aspect", QuestionClues: []string{"gustave eiffel designed paris tower"}},
		}, 0, nil),
	}
	mdl := &scriptedModel{}
	deps := harness.SessionDeps{Model: mdl, KB: kb}

	res := RunSlotResearchPass(context.Background(), deps, st.Question, st, 120.0)
	if res == nil {
		t.Fatal("expected a result")
	}
	// The prefill removed BOTH sessions: the model was never called.
	if got := len(mdl.seen); got != 0 {
		t.Fatalf("model calls = %d, want 0 (prefill must skip every session)", got)
	}
	for i, want := range []string{
		"Eiffel Tower — built in Paris by Gustave Eiffel, opened in 1889",
		"Eiffel Tower — built in Paris by Gustave Eiffel, opened in 1889",
	} {
		if st.SlotTable.State[i].Candidate == nil || *st.SlotTable.State[i].Candidate != want {
			t.Fatalf("slot %d candidate = %v, want %q", i, st.SlotTable.State[i].Candidate, want)
		}
	}
	if len(res.UnresolvedSlots) != 0 {
		t.Fatalf("unresolved = %d, want 0", len(res.UnresolvedSlots))
	}
}

// ---------------------------------------------------------------------------
// BatchFillSlots（Python _batch_fill_slots）
// ---------------------------------------------------------------------------

// TestBatchFillSlots_ClustersOverlappingEvidence: two slots sharing >= 2
// evidence ids with Jaccard >= 0.15 are answered in ONE call; a slot with
// disjoint evidence stays in its own (size-1) cluster and is never asked.
func TestBatchFillSlots_ClustersOverlappingEvidence(t *testing.T) {
	chunks := []map[string]any{}
	for _, id := range []string{"c1", "c2", "c3", "c4", "c9", "c10"} {
		chunks = append(chunks, map[string]any{
			"chunk_id":            id,
			"content_with_weight": "content of " + id,
			"doc_id":              "d1",
		})
	}
	slotTable := harness.NewState([]harness.Variable{
		{ID: 1, Type: "aspect", QuestionClues: []string{"who built it"}},
		{ID: 2, Type: "aspect", QuestionClues: []string{"when opened"}},
		{ID: 3, Type: "aspect", QuestionClues: []string{"unrelated"}},
	}, 0, nil)
	slotEvidence := map[string]SlotEvidence{
		"1": {EvidenceIDs: []string{"c1", "c2", "c3"}}, // vs 2: inter=2, union=4, sim=0.5
		"2": {EvidenceIDs: []string{"c1", "c2", "c4"}},
		"3": {EvidenceIDs: []string{"c9", "c10"}},
	}
	mdl := &scriptedModel{}
	mdl.push(`{"1": "Gustave Eiffel", "2": "1889", "3": "invented"}`)
	deps := harness.SessionDeps{Model: mdl, KB: &harness.Kbinfos{Chunks: chunks}}

	if got := BatchFillSlots(context.Background(), deps, &slotTable, slotEvidence); got != 2 {
		t.Fatalf("filled = %d, want 2", got)
	}
	// Exactly ONE generation call served slots 1 and 2.
	if len(mdl.seen) != 1 {
		t.Fatalf("model calls = %d, want 1", len(mdl.seen))
	}
	if got := *slotTable.State[0].Candidate; got != "Gustave Eiffel" {
		t.Fatalf("slot 1 candidate = %q", got)
	}
	if got := *slotTable.State[1].Candidate; got != "1889" {
		t.Fatalf("slot 2 candidate = %q", got)
	}
	// The disjoint slot was never asked, never filled.
	if slotTable.State[2].Candidate != nil {
		t.Fatalf("slot 3 candidate = %v, want nil", slotTable.State[2].Candidate)
	}
	// The prompt carries both sub-questions and the shared evidence body.
	user := mdl.seen[0][1].Content
	for _, want := range []string{"- 1: who built it", "- 2: when opened", "content of c1", "content of c4"} {
		if !strings.Contains(user, want) {
			t.Fatalf("prompt missing %q:\n%s", want, user)
		}
	}
}

// TestBatchFillSlots_ThresholdsBlockMerging pins both gates（Python
// :1574-1575）: fewer than MIN_SHARED shared ids, or a Jaccard below
// MIN_SIM, keeps the slots in separate size-1 clusters — no LLM call at all.
func TestBatchFillSlots_ThresholdsBlockMerging(t *testing.T) {
	mk := func(e1, e2 []string) (harness.State, map[string]SlotEvidence) {
		st := harness.NewState([]harness.Variable{
			{ID: 1, Type: "aspect", QuestionClues: []string{"a"}},
			{ID: 2, Type: "aspect", QuestionClues: []string{"b"}},
		}, 0, nil)
		ev := map[string]SlotEvidence{
			"1": {EvidenceIDs: e1},
			"2": {EvidenceIDs: e2},
		}
		return st, ev
	}
	ids := func(n int) []string {
		out := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, fmt.Sprintf("c%02d", i))
		}
		return out
	}

	cases := []struct {
		name string
		e1   []string
		e2   []string
	}{
		// inter=1 < MIN_SHARED=2.
		{"shared-below-floor", []string{"c01", "c02"}, []string{"c02", "c03"}},
		// inter=2, union=14 → sim≈0.143 < MIN_SIM=0.15.
		{"sim-below-gate", ids(14), []string{"c13", "c14"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, ev := mk(tc.e1, tc.e2)
			mdl := &scriptedModel{}
			deps := harness.SessionDeps{Model: mdl, KB: &harness.Kbinfos{}}
			if got := BatchFillSlots(context.Background(), deps, &st, ev); got != 0 {
				t.Fatalf("filled = %d, want 0", got)
			}
			if len(mdl.seen) != 0 {
				t.Fatalf("model calls = %d, want 0 (below the merge gates)", len(mdl.seen))
			}
		})
	}
}

// TestBatchFillSlots_MaxSlotsPerBatch pins _EVIDENCE_BATCH_MAX_SLOTS=4
// （Python :1576, :1625）: the fifth identical-evidence slot cannot join the
// full batch and is left unfilled this round.
func TestBatchFillSlots_MaxSlotsPerBatch(t *testing.T) {
	vars := make([]harness.Variable, 0, 5)
	ev := map[string]SlotEvidence{}
	for i := 1; i <= 5; i++ {
		vars = append(vars, harness.Variable{ID: i, Type: "aspect", QuestionClues: []string{"same clue"}})
		ev[fmt.Sprint(i)] = SlotEvidence{EvidenceIDs: []string{"c1", "c2"}}
	}
	st := harness.NewState(vars, 0, nil)
	kb := &harness.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "content_with_weight": "shared text one", "doc_id": "d1"},
		{"chunk_id": "c2", "content_with_weight": "shared text two", "doc_id": "d1"},
	}}
	mdl := &scriptedModel{}
	mdl.push(`{"1": "a", "2": "b", "3": "c", "4": "d", "5": "e"}`)
	deps := harness.SessionDeps{Model: mdl, KB: kb}

	if got := BatchFillSlots(context.Background(), deps, &st, ev); got != 4 {
		t.Fatalf("filled = %d, want 4 (MAX_SLOTS)", got)
	}
	if len(mdl.seen) != 1 {
		t.Fatalf("model calls = %d, want 1", len(mdl.seen))
	}
	for i := 0; i < 4; i++ {
		if st.State[i].Candidate == nil {
			t.Fatalf("slot %d must be filled", i+1)
		}
	}
	if st.State[4].Candidate != nil {
		t.Fatalf("slot 5 candidate = %v, want nil (batch cap 4)", st.State[4].Candidate)
	}
}

// TestBatchFillSlots_LLMFailureBestEffort mirrors Python :1681-1683: a failed
// model call (or an unparsable reply, or no model at all) fills nothing and
// never panics — the pass continues without the batch.
func TestBatchFillSlots_LLMFailureBestEffort(t *testing.T) {
	mk := func() (harness.State, map[string]SlotEvidence) {
		st := harness.NewState([]harness.Variable{
			{ID: 1, Type: "aspect", QuestionClues: []string{"a"}},
			{ID: 2, Type: "aspect", QuestionClues: []string{"b"}},
		}, 0, nil)
		ev := map[string]SlotEvidence{
			"1": {EvidenceIDs: []string{"c1", "c2"}},
			"2": {EvidenceIDs: []string{"c1", "c2"}},
		}
		return st, ev
	}
	kb := &harness.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "content_with_weight": "text one", "doc_id": "d1"},
		{"chunk_id": "c2", "content_with_weight": "text two", "doc_id": "d1"},
	}}

	t.Run("model-error", func(t *testing.T) {
		st, ev := mk()
		mdl := &failingModel{}
		deps := harness.SessionDeps{Model: mdl, KB: kb}
		if got := BatchFillSlots(context.Background(), deps, &st, ev); got != 0 {
			t.Fatalf("filled = %d, want 0", got)
		}
		if mdl.Calls() != 1 {
			t.Fatalf("model calls = %d, want 1 (the cluster is attempted)", mdl.Calls())
		}
		for _, v := range st.State {
			if v.Candidate != nil {
				t.Fatal("no slot may be filled after a failed call")
			}
		}
	})
	t.Run("unparsable-reply", func(t *testing.T) {
		st, ev := mk()
		mdl := &scriptedModel{}
		mdl.push("sorry, I cannot answer that") // no JSON object
		deps := harness.SessionDeps{Model: mdl, KB: kb}
		if got := BatchFillSlots(context.Background(), deps, &st, ev); got != 0 {
			t.Fatalf("filled = %d, want 0", got)
		}
	})
	t.Run("no-model", func(t *testing.T) {
		st, ev := mk()
		deps := harness.SessionDeps{KB: kb} // Model nil — Python :1674 `return filled`
		if got := BatchFillSlots(context.Background(), deps, &st, ev); got != 0 {
			t.Fatalf("filled = %d, want 0", got)
		}
	})
}

// TestBatchFillSlots_SkipDiagnostics pins the never-firing diagnostics
// （Python :1606-1610）: fewer than two evidence-carrying unresolved slots
// logs the skip line and short-circuits before any model call.
func TestBatchFillSlots_SkipDiagnostics(t *testing.T) {
	st := harness.NewState([]harness.Variable{
		{ID: 1, Type: "aspect", QuestionClues: []string{"a"}},
		{ID: 2, Type: "aspect", QuestionClues: []string{"b"}},
	}, 0, nil)
	ev := map[string]SlotEvidence{
		"1": {EvidenceIDs: []string{"c1"}}, // only ONE slot carries evidence
	}
	mdl := &scriptedModel{}
	deps := harness.SessionDeps{Model: mdl, KB: &harness.Kbinfos{}}
	if got := BatchFillSlots(context.Background(), deps, &st, ev); got != 0 {
		t.Fatalf("filled = %d, want 0", got)
	}
	if len(mdl.seen) != 0 {
		t.Fatalf("model calls = %d, want 0", len(mdl.seen))
	}
}
