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

package orchestrator

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/rag/advanced_rag/harness"
)

// stubJSONModel returns a canned JSON value (or an error), and records the last
// prompt it was shown.
type stubJSONModel struct {
	value any
	err   error
	calls int
	last  string
}

func (s *stubJSONModel) GenJSON(_ context.Context, prompt string) (any, error) {
	s.calls++
	s.last = prompt
	if s.err != nil {
		return nil, s.err
	}
	return s.value, nil
}

// ---------------------------------------------------------------------------
// SCA
// ---------------------------------------------------------------------------

func TestSCAReturnsNoSignalWithoutClaims(t *testing.T) {
	mdl := &stubJSONModel{value: map[string]any{"is_sufficient": true}}
	got := SufficientContextAgent(context.Background(), SCADeps{
		KB:    &harness.Kbinfos{},
		Model: mdl,
	}, "q", nil)
	if mdl.calls != 0 {
		t.Error("no claims must short-circuit before any LLM call")
	}
	if got.IsSufficient || got.Claims != nil {
		t.Error("no claims must yield an empty result (no signal)")
	}
}

func TestSCAParsesUnifiedVerdict(t *testing.T) {
	mdl := &stubJSONModel{value: map[string]any{
		"is_sufficient":  false,
		"confidence":     0.42,
		"contradictions": []any{"X says 1999, Y says 2001"},
		"reasoning":      "missing the release year",
		"sub_queries": []any{
			map[string]any{"sub_query": "release year", "satisfied": false,
				"missing_fact": "the year", "search_hint": "Culdcept release date"},
			map[string]any{"sub_query": "developer", "satisfied": true},
		},
		"claims": []any{
			map[string]any{"claim_id": "c1", "grounded": false,
				"missing_information": []any{map[string]any{"what": "release year", "search_hint": "Culdcept 1999"}}},
		},
	}}
	got := SufficientContextAgent(context.Background(), SCADeps{
		KB:    &harness.Kbinfos{},
		Model: mdl,
	}, "q", []ClaimDraft{{ID: "c1", Draft: "OmiyaSoft made it"}})

	if got.IsSufficient {
		t.Error("is_sufficient must be false")
	}
	if got.Confidence != 0.42 {
		t.Errorf("confidence = %v, want 0.42", got.Confidence)
	}
	if len(got.Contradictions) != 1 || got.Contradictions[0] != "X says 1999, Y says 2001" {
		t.Errorf("contradictions = %v", got.Contradictions)
	}
	if len(got.SubQueries) != 2 {
		t.Fatalf("sub_queries = %d, want 2", len(got.SubQueries))
	}
	if got.SubQueries[0].Satisfied || got.SubQueries[0].MissingFact != "the year" {
		t.Errorf("sub_query[0] = %+v", got.SubQueries[0])
	}
	if !got.SubQueries[1].Satisfied {
		t.Error("sub_query[1] must be satisfied")
	}
	cv, ok := got.Claims["c1"]
	if !ok {
		t.Fatal("claim c1 missing from verdicts")
	}
	if cv.Grounded {
		t.Error("claim c1 must be ungrounded")
	}
	if len(cv.MissingInformation) != 1 || cv.MissingInformation[0].What != "release year" {
		t.Errorf("missing_information = %+v", cv.MissingInformation)
	}
}

func TestSCACoercesArrayAndStringDrift(t *testing.T) {
	// Model drift: a bare array, or a JSON string, must still yield a verdict
	// instead of being dropped (which would silently stop replan/rewrite).
	for name, value := range map[string]any{
		"array":  []any{map[string]any{"is_sufficient": true, "confidence": 0.9}},
		"string": `{"is_sufficient": true, "confidence": 0.7}`,
	} {
		mdl := &stubJSONModel{value: value}
		got := SufficientContextAgent(context.Background(), SCADeps{
			KB:    &harness.Kbinfos{},
			Model: mdl,
		}, "q", []ClaimDraft{{ID: "c1", Draft: "d"}})
		if !got.IsSufficient {
			t.Errorf("%s drift: verdict lost", name)
		}
	}
}

func TestSCADerivesGapWhenInsufficientWithEmptyClaims(t *testing.T) {
	// Failsafe: insufficient + empty claims array (a known degradation on long
	// prompts) must still produce a gap, else the orchestrator abandons despite
	// having usable evidence.
	mdl := &stubJSONModel{value: map[string]any{
		"is_sufficient":       false,
		"missing_information": []any{map[string]any{"what": "the year", "search_hint": "1999"}},
	}}
	got := SufficientContextAgent(context.Background(), SCADeps{
		KB:    &harness.Kbinfos{},
		Model: mdl,
	}, "q", []ClaimDraft{{ID: "c1", Draft: "OmiyaSoft made it"}})
	g, ok := got.Claims["_global"]
	if !ok {
		t.Fatal("top-level missing_information must be harvested as the _global gap")
	}
	if len(g.MissingInformation) != 1 || g.MissingInformation[0].What != "the year" {
		t.Errorf("gap = %+v", g.MissingInformation)
	}

	// No structured gap at all: fall back to the drafts themselves.
	mdl = &stubJSONModel{value: map[string]any{"is_sufficient": false}}
	got = SufficientContextAgent(context.Background(), SCADeps{
		KB:    &harness.Kbinfos{},
		Model: mdl,
	}, "q", []ClaimDraft{{ID: "c1", Draft: "OmiyaSoft made it"}})
	g, ok = got.Claims["_global"]
	if !ok || len(g.MissingInformation) == 0 {
		t.Fatal("draft-derived fallback gap missing")
	}
	if g.MissingInformation[0].What != "OmiyaSoft made it" {
		t.Errorf("fallback gap = %q, want the draft text", g.MissingInformation[0].What)
	}
}

func TestSCAEvidenceAnchorsUseChunkIndices(t *testing.T) {
	// EvidenceIDs are INDICES into Kbinfos.Chunks (not chunk_id hashes) — that
	// indexing is what makes anchor lookup hit.
	kb := &harness.Kbinfos{Chunks: []map[string]any{
		{"content": "Culdcept was released in 1999 by OmiyaSoft."},
	}}
	contextText := renderClaimContext([]ClaimDraft{
		{ID: "c1", Draft: "OmiyaSoft made Culdcept", EvidenceIDs: []string{"0"}},
	}, kb)
	if !strings.Contains(contextText, "1999") {
		t.Errorf("anchor not resolved from index 0; got %q", contextText)
	}
	// An out-of-range index is simply skipped.
	if s := renderClaimContext([]ClaimDraft{{ID: "c1", Draft: "d", EvidenceIDs: []string{"99"}}}, kb); strings.Contains(s, "Evidence:") {
		t.Error("unknown evidence index must not produce an anchor")
	}
}

func TestBoundedExcerptKeepsTablesWhole(t *testing.T) {
	table := "| a | b |\n| c | d |\n| e | f |\n| g | h |"
	if got := boundedExcerpt(table, "unrelated hint", 50); got != table {
		t.Errorf("table text must be returned whole, got %q", got)
	}
	// Prose is windowed around a hint token.
	prose := strings.Repeat("prefix filler ", 40) + "TARGET sentence here" + strings.Repeat(" trailing filler", 40)
	got := boundedExcerpt(prose, "TARGET", 120)
	if len(got) > 130 {
		t.Errorf("prose excerpt not bounded: %d chars", len(got))
	}
	if !strings.Contains(got, "TARGET") {
		t.Error("excerpt must contain the hint token")
	}
}

// boundedExcerpt must slice by code point, not byte, so a CJK hint inside a CJK
// passage is windowed without splitting a rune (which would emit invalid UTF-8).
func TestBoundedExcerptCJKByRune(t *testing.T) {
	// 12 CJK chars around a TARGET token; with maxChars well below the full
	// length the window should still be valid UTF-8 and contain the token.
	prose := strings.Repeat("字", 40) + "目标句在此" + strings.Repeat("文", 40)
	got := boundedExcerpt(prose, "目标", 120)
	if !utf8.ValidString(got) {
		t.Errorf("CJK excerpt is not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "目标") {
		t.Errorf("CJK excerpt must contain the hint token, got %q", got)
	}
	// The no-hint tail path must also stay valid UTF-8 (text[-tail:] by byte
	// would split the trailing runes).
	got2 := boundedExcerpt(prose, "absent", 60)
	if !utf8.ValidString(got2) {
		t.Errorf("CJK tail excerpt is not valid UTF-8: %q", got2)
	}
}

func TestSCABoostAdaptsVerdict(t *testing.T) {
	res := SCAResult{
		IsSufficient:   false,
		Confidence:     0.3,
		Contradictions: []string{"c1"},
		Claims: map[string]ClaimVerdict{
			"c1": {MissingInformation: []MissingPiece{{What: "the year"}}},
			"c2": {MissingInformation: []MissingPiece{{What: "the year"}}}, // duplicate
		},
	}
	b := res.Boost([]string{"fallback query"})
	if b.IsSufficient {
		t.Error("boost must carry is_sufficient=false")
	}
	if len(b.Missing) != 1 || b.Missing[0] != "the year" {
		t.Errorf("missing = %v, want the deduped 'the year'", b.Missing)
	}
	if !strings.Contains(b.Feedback, "the year") {
		t.Errorf("feedback = %q, must mention the missing piece", b.Feedback)
	}
	if len(b.Followups) != 1 || b.Followups[0] != "fallback query" {
		t.Errorf("followups = %v", b.Followups)
	}
}

// ---------------------------------------------------------------------------
// Query rewriter
// ---------------------------------------------------------------------------

func TestClampAndTruthy(t *testing.T) {
	if got := clamp(float64(1.5)); got != 1.0 {
		t.Errorf("clamp(1.5) = %v, want 1.0", got)
	}
	if got := clamp(float64(-0.2)); got != 0.0 {
		t.Errorf("clamp(-0.2) = %v, want 0.0", got)
	}
	if got := clamp("0.4"); got != 0.4 {
		t.Errorf("clamp(\"0.4\") = %v, want 0.4", got)
	}
	// Unparseable -> Python's default of 1.0.
	if got := clamp("not a number"); got != 1.0 {
		t.Errorf("clamp(bad) = %v, want 1.0", got)
	}
	for _, v := range []any{true, "true", "TRUE", "1", "yes", "false", "0", float64(1)} {
		if !truthy(v) {
			t.Errorf("truthy(%v) = false, want true", v)
		}
	}
	for _, v := range []any{false, "", float64(0), nil} {
		if truthy(v) {
			t.Errorf("truthy(%v) = true, want false", v)
		}
	}
}
