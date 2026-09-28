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

package agentic_rag

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/rag/agentic-rag/runtime"
)

// Test doubles

// failingModel errors on every Complete call — the batched-answer LLM seam.
type failingModel struct {
	mu    sync.Mutex
	calls int
}

func (m *failingModel) Complete(context.Context, []schema.Message, []runtime.ToolSpec) (*runtime.ModelReply, error) {
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

// claimRow builds one claim pseudo-chunk ("[evidence] <name> — <desc>") the way the pool
// stores it.
func claimRow(id, content string) map[string]any {
	return map[string]any{
		"chunk_id":            id,
		"content_with_weight": content,
		"doc_id":              "d1",
	}
}

// PrefillSlotsFromEvidence

// TestPrefillSlotsFromEvidence_CoverageBoundary pins the 0.6 word-coverage gate: exactly-at
// and above prefill, below does not; the candidate is the de-prefixed first line and the
// strength is the coverage itself.
func TestPrefillSlotsFromEvidence_CoverageBoundary(t *testing.T) {
	// Clue terms: {alpha, tower, opened, paris, 1889} (>=3 code points only).
	clue := "alpha tower opened paris 1889"
	// Covers tower/opened/paris = 3/5 = exactly 0.6.
	atGate := claimRow("claim_a", "[evidence] Eiffel Tower — landmark\nEvidence (verbatim): \"the tower opened in paris near the river\"")
	// Covers tower/opened = 2/5 = 0.4.
	below := claimRow("claim_b", "[evidence] Eiffel Tower — landmark\nEvidence (verbatim): \"the tower opened here long ago\"")

	newState := func() runtime.State {
		return runtime.NewState([]runtime.Variable{
			{ID: 1, Type: "aspect", QuestionClues: []string{clue}},
		}, 0, nil)
	}

	// At the gate: prefilled.
	st := newState()
	if n := PrefillSlotsFromEvidence(&st, &runtime.Kbinfos{Chunks: []map[string]any{atGate}}); n != 1 {
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
	if n := PrefillSlotsFromEvidence(&st, &runtime.Kbinfos{Chunks: []map[string]any{below}}); n != 0 {
		t.Fatalf("coverage 0.4 must NOT prefill, filled = %d", n)
	}
	if st.State[0].Candidate != nil {
		t.Fatalf("candidate = %v, want nil", st.State[0].Candidate)
	}
}

// TestPrefillSlotsFromEvidence_Guards mirrors the skips（,
// 1744）: no claim rows in the pool, already-filled slots, blank clues, and a
// first line that strips to nothing.
func TestPrefillSlotsFromEvidence_Guards(t *testing.T) {
	st := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "aspect", QuestionClues: []string{"eiffel tower paris"}},
		{ID: 2, Type: "aspect", Candidate: strPtr("known")},
		{ID: 3, Type: "aspect", QuestionClues: []string{"   "}},
	}, 0, nil)

	// No claim rows: 0 regardless of content.
	if n := PrefillSlotsFromEvidence(&st, &runtime.Kbinfos{Chunks: []map[string]any{
		claimRow("c1", "plain chunk"),
	}}); n != 0 {
		t.Fatalf("no claim_ rows must yield 0, got %d", n)
	}
	// nil KB is safe.
	if n := PrefillSlotsFromEvidence(&st, nil); n != 0 {
		t.Fatalf("nil KB must yield 0, got %d", n)
	}

	// Row whose head strips to nothing: even full coverage fills nothing.
	if n := PrefillSlotsFromEvidence(&st, &runtime.Kbinfos{Chunks: []map[string]any{
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

// TestRunSlotResearchPass_PrefillFillsWithoutSpendingASession pins the wiring on both sides: the
// pooled evidence fills the slots it covers (>= 0.6) so no session has to go looking for them,
// and the round still runs its ONE session — that session is the answerer as well as the
// researcher, so "every slot is filled" is no longer a reason to skip the model.
func TestRunSlotResearchPass_PrefillFillsWithoutSpendingASession(t *testing.T) {
	kb := &runtime.Kbinfos{Chunks: []map[string]any{
		claimRow("claim_a", "[evidence] Eiffel Tower — built in Paris by Gustave Eiffel, opened in 1889\nEvidence (verbatim): \"...\""),
	}}
	st := &AgenticState{
		Question: "what is the tower",
		SlotTable: runtime.NewState([]runtime.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"eiffel tower opened paris 1889"}},
			{ID: 1, Type: "aspect", QuestionClues: []string{"gustave eiffel designed paris tower"}},
		}, 0, nil),
	}
	mdl := &scriptedModel{}
	deps := runtime.SessionDeps{Model: mdl, KB: kb}

	res := RunSlotResearchPass(context.Background(), context.Background(), deps, st.Question, st, 120.0)
	if res == nil {
		t.Fatal("expected a result")
	}
	// The prefill answered both slots from evidence already in the pool, and the round's one
	// session still runs: it writes the answer, so it cannot be skipped.
	if got := len(mdl.seen); got == 0 {
		t.Fatal("model calls = 0, want the one session this round seeds")
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
