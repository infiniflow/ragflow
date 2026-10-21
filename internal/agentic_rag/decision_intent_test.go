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
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/rag/decisionhead"
)

// The intent is per CALL, so it must survive on the call's own context and be
// visible to whatever reads it — including a judge that a test stubs in.
func TestIntentTravelsOnTheCallContext(t *testing.T) {
	if got := intentFrom(context.Background()); got != "" {
		t.Fatalf("bare context carries %q, want empty", got)
	}
	ctx := withIntent(context.Background(), "  the first name of Sarah Polk's mother  ")
	if got := intentFrom(ctx); got != "the first name of Sarah Polk's mother" {
		t.Fatalf("intent = %q, want the trimmed sentence", got)
	}
	// A blank intent must not shadow anything: it is the no-intent case.
	if withIntent(context.Background(), "   ") != context.Background() {
		t.Error("a blank intent must leave the context untouched")
	}
	// Nesting a second intent replaces the first on that branch only.
	branch := withIntent(ctx, "something else")
	if got := intentFrom(branch); got != "something else" {
		t.Errorf("nested intent = %q", got)
	}
	if got := intentFrom(ctx); got != "the first name of Sarah Polk's mother" {
		t.Errorf("parent context was mutated: %q", got)
	}
}

// With an intent, the question the endpoint is asked carries BOTH: the step's
// need and the overall question. The intent alone would let a narrow or
// mistaken intent discard evidence, and the question alone is the reading that
// lost 30% of the labelled evidence (§13).
func TestDecisionIntentAndQuestionAreBothAsked(t *testing.T) {
	// These tests cover the multi-phrasing mechanics; the shipped default is one.
	t.Setenv("RAGFLOW_DECISION_PHRASINGS", "4")
	var seen map[string]any
	srv := decisionStub(t, []float64{.9, .9, .9, .9}, 200, &seen)
	defer srv.Close()
	b := backendFor(t, srv.URL, 0.15)

	const (
		question = "What is my future wife's name?"
		intent   = "the first name of Sarah Polk's mother"
	)
	if _, _, err := b.ask(context.Background(), question, intent, "passage"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	questions, _ := seen["questions"].(map[string]any)
	if len(questions) != len(decisionStepTemplates) {
		t.Fatalf("asked %d questions, want %d step phrasings", len(questions), len(decisionStepTemplates))
	}
	for _, raw := range questions {
		q, _ := raw.(map[string]any)
		ins, _ := q["instructions"].(string)
		if !strings.Contains(ins, intent) {
			t.Errorf("instructions do not carry the intent: %q", ins)
		}
		if !strings.Contains(ins, question) {
			t.Errorf("instructions do not carry the overall question: %q", ins)
		}
	}
	// The state stays the evidence alone: putting the question in it was
	// measured to cost separation (evidence median 0.215 -> 0.160).
	state, _ := seen["state"].(map[string]any)
	// The passage now travels inside each question, because one request carries
	// several passages; the state is empty on purpose.
	if len(state) != 1 || state["passage"] != "" {
		t.Errorf("state = %v, want only {\"passage\": \"passage\"}", state)
	}

	// Without an intent the reading falls back to the question-only form, so a
	// tool that publishes nothing behaves exactly as the previous arm did.
	seen = nil
	if _, _, err := b.ask(context.Background(), question, "", "passage"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	questions, _ = seen["questions"].(map[string]any)
	if len(questions) != len(decisionRelevanceTemplates) {
		t.Fatalf("no-intent ask used %d questions, want the %d relevance phrasings",
			len(questions), len(decisionRelevanceTemplates))
	}
	for _, raw := range questions {
		q, _ := raw.(map[string]any)
		ins, _ := q["instructions"].(string)
		if !strings.Contains(ins, question) || strings.Contains(ins, "this step") {
			t.Errorf("no-intent instructions = %q, want question-only wording", ins)
		}
	}
}

// The locate exit judges the chunk BODY by default: the snippet is a window
// (one paragraph for a truncated grep hit, the opening for a BM25 preview) and
// the threshold was fitted on whole passages.
func TestSnippetJudgeTextPrefersTheBody(t *testing.T) {
	hit := snippetHit{
		chunk:   runtime.RetrievalChunk{ID: "c1", Content: "the whole passage body"},
		snippet: "a window of the passage",
	}
	if got := snippetJudgeText(hit); got != "the whole passage body" {
		t.Errorf("default = %q, want the body", got)
	}
	t.Setenv(common.EnvDecisionSnippetText, "snippet")
	if got := snippetJudgeText(hit); got != "a window of the passage" {
		t.Errorf("snippet mode = %q, want the window", got)
	}
	t.Setenv(common.EnvDecisionSnippetText, "")
	// An empty body cannot be judged, so the window is the fallback even in
	// body mode.
	hit.chunk.Content = "  "
	if got := snippetJudgeText(hit); got != "a window of the passage" {
		t.Errorf("empty body = %q, want the window", got)
	}
}

// A verdict about the body still drops the WHOLE hit, and a kept hit is served
// exactly as built: the body must never be written into the snippet, or the
// filter would widen a one-paragraph window into the whole chunk while claiming
// to shrink the context.
func TestSnippetFilterJudgesBodyAndKeepsSnippetIntact(t *testing.T) {
	gateMode(t)
	t.Setenv(common.EnvDecisionFilterSnippet, "true")
	t.Setenv(common.EnvDecisionWholeChunk, "true")
	judged := make([]string, 0, 2)
	stubDecision(t,
		func(text string) ([]string, string) { return []string{text}, "" },
		func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
			out := make([]decisionhead.Verdict, len(segments))
			for i, s := range segments {
				judged = append(judged, s)
				// Keep the passage whose body mentions the fact; drop the other.
				out[i] = decisionhead.Verdict{Segment: s, Useful: strings.Contains(s, "KEEP-ME"), Judged: true, Probability: 0.5}
			}
			return out
		},
	)
	hits := []snippetHit{
		{chunk: runtime.RetrievalChunk{ID: "keep", Content: "KEEP-ME the body"}, snippet: "a window without the word"},
		{chunk: runtime.RetrievalChunk{ID: "drop", Content: "nothing of interest"}, snippet: "a window"},
	}
	got := filterSnippetHits(withIntent(WithQuery(context.Background(), "q"), "the fact"), grepChunksToolName, hits)

	if len(judged) != 2 {
		t.Fatalf("judged %d segments, want 2", len(judged))
	}
	for _, s := range judged {
		if strings.Contains(s, "window") {
			t.Errorf("the snippet was judged instead of the body: %q", s)
		}
	}
	if len(got) != 1 {
		t.Fatalf("kept %d hits, want 1", len(got))
	}
	if got[0].chunk.ID != "keep" {
		t.Errorf("kept %q, want the hit whose BODY carries the fact", got[0].chunk.ID)
	}
	if got[0].snippet != "a window without the word" {
		t.Errorf("snippet was rewritten to %q; a body verdict must not widen what the model reads", got[0].snippet)
	}
}
