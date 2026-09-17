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
	"io"
	"log"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/rag/advanced_rag/harness"
)

type muxChunk struct {
	text    string
	isThink bool
}

func newTestMux() (*outerStreamMux, *[]muxChunk) {
	got := make([]muxChunk, 0)
	mux := &outerStreamMux{sink: &AnswerSink{OnDelta: func(delta string, isThink bool) {
		got = append(got, muxChunk{delta, isThink})
	}}}
	return mux, &got
}

// The outer model's stream carries literal <think>/</think> markers, which the
// mux must turn into the sink's isThink flag instead of forwarding as text.
func TestOuterStreamMuxRoutesThinkThenAnswer(t *testing.T) {
	mux, got := newTestMux()
	for _, d := range []string{"<think>", "reasoning", "</think>", "the answer"} {
		text := d
		if err := mux.sender(&text, nil); err != nil {
			t.Fatalf("sender(%q): %v", d, err)
		}
	}

	want := []muxChunk{{"reasoning", true}, {"the answer", false}}
	if len(*got) != len(want) {
		t.Fatalf("got %v, want %v", *got, want)
	}
	for i, w := range want {
		if (*got)[i] != w {
			t.Fatalf("chunk %d = %v, want %v", i, (*got)[i], w)
		}
	}
}

// Python drops outer text once the terminal tool fired, because what follows is
// the aggregate tool result and the answer already streamed from inside.
func TestOuterStreamMuxDropsTextAfterTerminalFired(t *testing.T) {
	mux, got := newTestMux()
	mux.markTerminal()

	plain := "aggregate tool result"
	if err := mux.sender(&plain, nil); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if len(*got) != 0 {
		t.Fatalf("got %v, want the non-think text dropped after terminal fired", *got)
	}

	// Thinking text is still forwarded even after the terminal fired.
	open := "<think>"
	late := "late thinking"
	if err := mux.sender(&open, nil); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if err := mux.sender(&late, nil); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if len(*got) != 1 || (*got)[0] != (muxChunk{"late thinking", true}) {
		t.Fatalf("got %v, want only the late thinking chunk", *got)
	}
}

// The inner run's research log and composed answer reach the same sink.
func TestOuterStreamMuxDeliversInnerStream(t *testing.T) {
	mux, got := newTestMux()
	mux.deliver("[Research] planning\n", true)
	mux.deliver("final answer", false)

	want := []muxChunk{{"[Research] planning\n", true}, {"final answer", false}}
	if len(*got) != len(want) {
		t.Fatalf("got %v, want %v", *got, want)
	}
	for i, w := range want {
		if (*got)[i] != w {
			t.Fatalf("chunk %d = %v, want %v", i, (*got)[i], w)
		}
	}
}

// A nil sink must not panic: the mux is built even when the caller supplied no
// AnswerSink.
func TestOuterStreamMuxNilSinkIsSafe(t *testing.T) {
	mux := &outerStreamMux{}
	text := "anything"
	if err := mux.sender(&text, nil); err != nil {
		t.Fatalf("sender: %v", err)
	}
	mux.deliver("anything", true)
	mux.markTerminal()
}

func TestQuestionKeywordsSeparatesNumbers(t *testing.T) {
	gram := questionKeywords("Population of Paris in 2019")

	if !gram.numbers["2019"] {
		t.Fatalf("numbers = %v, want 2019 separated out", gram.numbers)
	}
	if gram.words["2019"] {
		t.Fatal("2019 must not count as a significant word")
	}
	if !gram.words["population"] || !gram.words["paris"] {
		t.Fatalf("words = %v, want population and paris", gram.words)
	}
	// Stopwords are dropped.
	if gram.words["of"] || gram.words["in"] {
		t.Fatalf("words = %v, stopwords must be dropped", gram.words)
	}
}

func TestCacheSimilarCollapsesReask(t *testing.T) {
	// Python's observed re-ask: "legal population" → "estimated population of
	// Paris in 2019" (overlap 0.75 while numbers match).
	a := questionKeywords("population of Paris 2019")
	b := questionKeywords("legal population of Paris in 2019")

	if !cacheSimilar(a, b) {
		t.Fatal("near-identical re-ask must be judged similar")
	}
}

func TestCacheSimilarRejectsDifferentNumbers(t *testing.T) {
	// Different years are different questions and must not share an answer.
	a := questionKeywords("population of Paris 2019")
	b := questionKeywords("population of Paris 2015")

	if cacheSimilar(a, b) {
		t.Fatal("questions naming different numbers must not be similar")
	}
}

func TestCacheSimilarRejectsDifferentSubjects(t *testing.T) {
	// Paris vs. Brown County ≈ 0.25 overlap.
	a := questionKeywords("population of Paris 2019")
	b := questionKeywords("population of Brown County 2019")

	if cacheSimilar(a, b) {
		t.Fatal("genuinely different questions must not be similar")
	}
}

func TestCacheLookupAndStore(t *testing.T) {
	c := NewRAGCache()
	c.Store("population of Paris 2019", "2.1 million")

	got, ok := c.Lookup("legal population of Paris in 2019")
	if !ok || got != "2.1 million" {
		t.Fatalf("Lookup = %q, %v; want the cached answer", got, ok)
	}
}

func TestCacheLookupMissesUnrelatedQuestion(t *testing.T) {
	c := NewRAGCache()
	c.Store("population of Paris 2019", "2.1 million")

	if _, ok := c.Lookup("who wrote the book about Brown County"); ok {
		t.Fatal("unrelated question must not hit the cache")
	}
}

func TestCacheReuseBlockedAfterInsufficientRound(t *testing.T) {
	// Python :848-852 — when the last round was not SUFFICIENT the caller is
	// asking again for more evidence, so the cached answer must not be reused.
	c := NewRAGCache()
	c.Store("population of Paris 2019", "2.1 million")
	c.noteVerdict("INSUFFICIENT")

	if _, ok := c.Lookup("legal population of Paris in 2019"); ok {
		t.Fatal("reuse must be blocked after an INSUFFICIENT round")
	}

	c.noteVerdict("SUFFICIENT")
	if _, ok := c.Lookup("legal population of Paris in 2019"); !ok {
		t.Fatal("reuse must resume once the round is sufficient")
	}
}

func TestRagDefaultsToPerTurnCache(t *testing.T) {
	// Go mirrors Python's default-on, per-turn _rag_cache: Rag auto-builds a
	// RAGCache when deps.Cache is nil, so caching is never off. Two independent
	// instances never share, matching Python's per-turn RAGTools._rag_cache
	// (rebuilt every turn).
	a := NewRAGCache()
	b := NewRAGCache()
	a.Store("population of Paris 2019", "2.1 million")

	if _, ok := b.Lookup("legal population of Paris in 2019"); ok {
		t.Fatal("an independent cache must not hit another instance's answer")
	}

	// A nil cache is still inert to direct calls (Rag builds its own internally
	// only for the duration of the call), so no cross-call reuse happens without
	// an explicitly shared instance.
	var nilCache *RAGCache
	if _, ok := nilCache.Lookup("population of Paris 2019"); ok {
		t.Fatal("a nil cache must never hit")
	}
}

func TestCacheSharedWhenCallerInjectsSameInstance(t *testing.T) {
	// A caller may widen reuse beyond one request by injecting the same
	// *RAGCache via RAGTools.Cache (e.g. across `rag` tool calls within a turn).
	c := NewRAGCache()
	c.Store("population of Paris 2019", "2.1 million")
	c2 := c // same instance passed on a later call

	if _, ok := c2.Lookup("legal population of Paris in 2019"); !ok {
		t.Fatal("an injected shared cache must reuse answers across calls")
	}
}

func TestNilCacheIsInert(t *testing.T) {
	var c *RAGCache

	if _, ok := c.Lookup("anything"); ok {
		t.Fatal("nil cache must not hit")
	}
	c.Store("q", "a") // must not panic
	c.noteVerdict("SUFFICIENT")
}

func TestResolveEffectiveQuestionPrefersOriginal(t *testing.T) {
	// The outer rewrite dropped the final target of a multi-hop question.
	got := resolveEffectiveQuestion(
		"when did the purchaser die",
		"when did the purchaser of the shortest abbreviation die",
	)

	if got != "when did the purchaser of the shortest abbreviation die" {
		t.Fatalf("got %q, want the original question", got)
	}
}

func TestResolveEffectiveQuestionKeepsDifferentTurn(t *testing.T) {
	// A genuine re-ask for a different question must keep its own query.
	got := resolveEffectiveQuestion("population of Brown County", "population of Paris 2019")

	if got != "population of Brown County" {
		t.Fatalf("got %q, want the rewrite kept for a different question", got)
	}
}

func TestResolveEffectiveQuestionHandlesEmpty(t *testing.T) {
	if got := resolveEffectiveQuestion("q", ""); got != "q" {
		t.Fatalf("got %q, want q", got)
	}
	if got := resolveEffectiveQuestion("", "orig"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// TestResearchStatusTrailerStopsAfterTwoUnanswerable asserts that once the
// shared counter has reached 2, Rag() appends the "STOP calling rag again"
// trailing sentence (Python rag:902-929) for an INSUFFICIENT verdict — the
// visible effect of the consecutive-unanswerable guard across multiple outer
// rag() calls.
func TestResearchStatusTrailerStopsAfterTwoUnanswerable(t *testing.T) {
	cache := NewRAGCache()
	cache.consecutiveUnanswerable = 2
	resp := &RunResponse{
		Verdict:     VerdictInsufficient,
		SCAFeedback: "evidence is not yet sufficient",
		Answer:      "Partial findings.",
	}
	got := researchStatusTrailer(cache, resp)
	if !strings.Contains(got, "STOP calling rag again") {
		t.Fatalf("trailer = %q, want it to tell the outer agent to STOP calling rag again", got)
	}
}

// TestResearchStatusTrailerInvitesFocusedReaskBelowTwo covers the first
// unsatisfying outer rag() call (counter < 2): Rag() should invite a focused
// re-ask rather than tell the agent to stop.

// TestResearchStatusTrailerInvitesFocusedReaskBelowTwo covers the first
// unsatisfying outer rag() call (counter < 2): Rag() should invite a focused
// re-ask rather than tell the agent to stop.
func TestResearchStatusTrailerInvitesFocusedReaskBelowTwo(t *testing.T) {
	cache := NewRAGCache()
	cache.consecutiveUnanswerable = 1
	resp := &RunResponse{
		Verdict:     VerdictInsufficient,
		SCAFeedback: "evidence is not yet sufficient",
		Answer:      "Partial findings.",
	}
	got := researchStatusTrailer(cache, resp)
	if !strings.Contains(got, "call rag again with a question focused on them") {
		t.Fatalf("trailer = %q, want a focused re-ask invite (not STOP)", got)
	}
	if strings.Contains(got, "STOP calling rag again") {
		t.Fatalf("trailer = %q, did not expect STOP below threshold", got)
	}
}

// TestResearchStatusTrailerSkipsSufficientOrEmpty ensures the note is omitted
// when there is nothing to annotate: a SUFFICIENT verdict, an empty answer, or
// missing SCA feedback all yield "".

// TestResearchStatusTrailerSkipsSufficientOrEmpty ensures the note is omitted
// when there is nothing to annotate: a SUFFICIENT verdict, an empty answer, or
// missing SCA feedback all yield "".
func TestResearchStatusTrailerSkipsSufficientOrEmpty(t *testing.T) {
	cache := NewRAGCache()
	cache.consecutiveUnanswerable = 2

	cases := []struct {
		name string
		resp *RunResponse
	}{
		{"sufficient verdict", &RunResponse{Verdict: VerdictSufficient, SCAFeedback: "ok", Answer: "A"}},
		{"empty answer", &RunResponse{Verdict: VerdictInsufficient, SCAFeedback: "x", Answer: ""}},
		{"missing sca feedback", &RunResponse{Verdict: VerdictInsufficient, SCAFeedback: "", Answer: "A"}},
	}
	for _, c := range cases {
		if got := researchStatusTrailer(cache, c.resp); got != "" {
			t.Errorf("%s: trailer = %q, want empty", c.name, got)
		}
	}
}

// streamingModel is a SessionModel that can also stream.
type streamingModel struct {
	pieces []string
	fail   bool
}

func (m *streamingModel) Complete(_ context.Context, _ []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
	if m.fail {
		return nil, errors.New("boom")
	}
	out := ""
	for _, p := range m.pieces {
		out += p
	}
	return &harness.ModelReply{Content: out}, nil
}

func (m *streamingModel) StreamComplete(_ context.Context, _ []schema.Message, _ []harness.ToolSpec, onDelta func(string, bool) error) (*harness.ModelReply, error) {
	if m.fail {
		return nil, errors.New("stream boom")
	}
	out := ""
	for _, p := range m.pieces {
		if onDelta != nil {
			if err := onDelta(p, false); err != nil {
				return nil, err
			}
		}
		out += p
	}
	return &harness.ModelReply{Content: out}, nil
}

func TestComposeAnswerStreamForwardsDeltas(t *testing.T) {
	m := &streamingModel{pieces: []string{"Hello ", "world"}}
	var got []string
	res, err := ComposeAnswerStream(context.Background(), AnswerDeps{Model: m}, m, nil, "q", false, false,
		func(delta string, _ bool) error {
			got = append(got, delta)
			return nil
		})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Answer != "Hello world" {
		t.Fatalf("Answer = %q, want %q", res.Answer, "Hello world")
	}
	if len(got) != 2 || got[0] != "Hello " || got[1] != "world" {
		t.Fatalf("deltas = %v, want the two pieces in order", got)
	}
}

func TestComposeAnswerStreamReturnsErrorOnFailure(t *testing.T) {
	m := &streamingModel{fail: true}

	if _, err := ComposeAnswerStream(context.Background(), AnswerDeps{Model: m}, m, nil, "q", false, false, nil); err == nil {
		t.Fatal("want an error so the caller can fall back to one-shot")
	}
}

func TestAnswerSinkDeliverAndReset(t *testing.T) {
	var got []string
	resets := 0
	s := &AnswerSink{
		OnDelta: func(delta string, _ bool) { got = append(got, delta) },
		OnReset: func() { resets++; got = nil },
	}

	s.deliver("a", false)
	s.deliver("b", false)
	s.reset()

	if len(got) != 0 || resets != 1 {
		t.Fatalf("after reset: got=%v resets=%d, want empty and 1", got, resets)
	}
	s.deliver("", false) // empty deltas are dropped
	if len(got) != 0 {
		t.Fatalf("got = %v, want empty delta dropped", got)
	}
}

func TestNilAnswerSinkIsInert(t *testing.T) {
	var s *AnswerSink

	s.deliver("x", false) // must not panic
	s.reset()
}

func TestGetCitationGuidelinesUsesDefaultWithoutOverride(t *testing.T) {
	got := GetCitationGuidelines("")
	// The default is the embedded citation_prompt.md (mirrors Python
	// load_prompt("citation_prompt")). It must contain the citation-rules body.
	if !strings.Contains(got, "# Citation Requirements:") ||
		!strings.Contains(got, "Place citations at the end of sentences") {
		t.Fatalf("got %q, want the default citation rules", got)
	}
}

func TestGetCitationGuidelinesHonoursOverride(t *testing.T) {
	// Python renders the user's template and STILL appends the
	// illustrative-IDs caveat after it (generator.py:227-228 concatenates the
	// suffix onto whatever template rendered).
	got := GetCitationGuidelines("Cite as [n].")
	if !strings.HasPrefix(got, "Cite as [n].") {
		t.Fatalf("got %q, want the override first", got)
	}
	if !strings.Contains(got, "IMPORTANT: The example IDs above") {
		t.Fatalf("got %q, want the illustrative-IDs caveat appended", got)
	}
}

func TestSysPromptIncludesSummarizeOnlyWithUnstructured(t *testing.T) {
	withDoc := SysPrompt("", true)
	if !strings.Contains(withDoc, "summarize_document") {
		t.Fatal("summarize_document must be offered when unstructured retrieval exists")
	}
	withoutDoc := SysPrompt("", false)
	if strings.Contains(withoutDoc, "summarize_document") {
		t.Fatal("summarize_document must be omitted without unstructured retrieval")
	}
}

func TestSysPromptPrependsSystemPrompt(t *testing.T) {
	got := SysPrompt("You are helpful.", true)

	if !strings.HasPrefix(got, "You are helpful.\n\n") {
		t.Fatalf("got %q, want the system prompt first", got)
	}
	if !strings.Contains(got, "call the `rag` tool") {
		t.Fatalf("got %q, want the router body kept", got)
	}
}

func TestFitEvidenceKeepsShortEvidence(t *testing.T) {
	evidence := "short evidence"
	if got := FitEvidence("q", evidence); got != evidence {
		t.Fatalf("got %q, want %q unchanged", got, evidence)
	}
}

func TestFitEvidenceReturnsEmptyForEmpty(t *testing.T) {
	if got := FitEvidence("q", ""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestFitEvidenceTrimsOversizedEvidence(t *testing.T) {
	// The budget is fixed (not the model window), so a large pool can never fill
	// the context — evidence beyond the cap must be dropped.
	evidence := strings.Repeat("word ", 200000)
	got := FitEvidence("q", evidence)

	if len(got) >= len(evidence) {
		t.Fatalf("evidence was not trimmed: %d >= %d", len(got), len(evidence))
	}
	if got == "" {
		t.Fatal("evidence must not be trimmed to nothing")
	}
}

// TestOuterReactSessionPublishMergesPerCallResults pins the locked merge: a
// round's rag calls run concurrently, so each publishes its own evidence and
// answer and none may be lost. Run with -race.
func TestOuterReactSessionPublishMergesPerCallResults(t *testing.T) {
	session := &outerReactSession{kb: &harness.Kbinfos{}, resp: &RunResponse{}}
	const calls = 4

	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session.publish(
				&RunResponse{Answer: fmt.Sprintf("answer-%d", i), Verdict: fmt.Sprintf("verdict-%d", i)},
				&harness.Kbinfos{
					Chunks:  []map[string]any{{"chunk_id": fmt.Sprintf("c%d", i)}},
					DocAggs: []map[string]any{{"doc_id": fmt.Sprintf("d%d", i)}},
					Memory:  []map[string]any{{"id": fmt.Sprintf("m%d", i)}},
				})
		}(i)
	}
	wg.Wait()

	if len(session.kb.Chunks) != calls || len(session.kb.DocAggs) != calls || len(session.kb.Memory) != calls {
		t.Fatalf("merged evidence = %d chunks / %d doc_aggs / %d memory, want %d each",
			len(session.kb.Chunks), len(session.kb.DocAggs), len(session.kb.Memory), calls)
	}
	seen := map[string]bool{}
	for _, c := range session.kb.Chunks {
		id, _ := c["chunk_id"].(string)
		seen[id] = true
	}
	for i := 0; i < calls; i++ {
		if !seen[fmt.Sprintf("c%d", i)] {
			t.Errorf("merged chunks lost c%d: %v", i, seen)
		}
	}
	if !strings.HasPrefix(session.resp.Answer, "answer-") {
		t.Errorf("answer = %q, want one call's answer", session.resp.Answer)
	}
	if !strings.HasPrefix(session.resp.Verdict, "verdict-") {
		t.Errorf("verdict = %q, want one call's verdict", session.resp.Verdict)
	}
}

// TestOuterReactSessionToolCallKeepsSharedRequestIntact pins the per-call
// isolation: models.appendToolResults runs a round's rag calls concurrently
// (chat_tools.go), so a call must apply its rewritten question and its cleared
// images to a COPY — never to the request a concurrent call is reading.
// Run with -race.
func TestOuterReactSessionToolCallKeepsSharedRequestIntact(t *testing.T) {
	spec := harness.GetMode("naive")
	session := &outerReactSession{
		ctx:    context.Background(),
		spec:   spec,
		kb:     &harness.Kbinfos{},
		resp:   &RunResponse{Mode: spec},
		logger: log.New(io.Discard, "", 0),
		req: harness.RunRequest{
			Question: "original question",
			Images:   []string{"data:image/png;base64,AAAA"},
		},
	}

	var wg sync.WaitGroup
	for _, q := range []string{"first question", "second question"} {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			if _, err := session.ToolCall("rag", map[string]interface{}{"question": q}); err != nil {
				t.Errorf("ToolCall(%q): %v", q, err)
			}
		}(q)
	}
	wg.Wait()

	if session.req.Question != "original question" {
		t.Errorf("shared request question = %q, want it untouched (each call works on a copy)", session.req.Question)
	}
	if len(session.req.Images) != 1 {
		t.Errorf("shared request images = %v, want them untouched", session.req.Images)
	}
}

// TestOuterReactSessionNarratesToolCalls pins the outer loop's think-block
// narration: the "[Function tool] Running the {name} tool with: {args}" line
// Python's FunctionToolSession emits (tool_decorator.py:311) and the result line
// that closes it — so the top-level step records which tool the outer model
// called, with which arguments, and what came back.
func TestOuterReactSessionNarratesToolCalls(t *testing.T) {
	// summarize_document with no doc_id is the early-return branch: it needs no
	// retrieval wiring, so the narration contract is testable in isolation.
	var buf strings.Builder
	spec := harness.GetMode("naive")
	session := &outerReactSession{
		ctx:    context.Background(),
		spec:   spec,
		kb:     &harness.Kbinfos{},
		resp:   &RunResponse{Mode: spec},
		logger: log.New(&buf, "", 0),
		req:    harness.RunRequest{Question: "q"},
	}

	if _, err := session.ToolCall("summarize_document", map[string]interface{}{}); err != nil {
		t.Fatalf("ToolCall: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"[Function tool] Running the summarize_document tool with: {}",
		"[Function tool] The summarize_document tool could not run: it was called without a doc_id.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("think-log narration missing %q:\n%s", want, out)
		}
	}
}

// TestOuterReactSessionUnknownToolNarrates pins the unknown-name branch: a tool
// the deployment has no binding for reports that nothing ran (MISS/unwired), not
// an infra failure — and it carries the call's arguments like every other step.
func TestOuterReactSessionUnknownToolNarrates(t *testing.T) {
	var buf strings.Builder
	events := make(chan harness.ThinkEvent, 4)
	spec := harness.GetMode("naive")
	session := &outerReactSession{
		ctx: context.Background(), spec: spec, kb: &harness.Kbinfos{},
		resp: &RunResponse{Mode: spec}, logger: log.New(&buf, "", 0),
		req: harness.RunRequest{Question: "q"},
	}
	ctx := harness.WithSteps(context.Background(), harness.StepReporter{
		Events: func(ev harness.ThinkEvent) { events <- ev },
	})
	session.ctx = ctx

	if _, err := session.ToolCall("time_travel", map[string]interface{}{"query": "tomorrow"}); err != nil {
		t.Fatalf("ToolCall: %v", err)
	}
	if out := buf.String(); !strings.Contains(out,
		`[Function tool] The time_travel tool is not wired in this deployment, so nothing ran.`) {
		t.Errorf("think-log narration:\n%s", out)
	}

	var result harness.ThinkEvent
	for len(events) > 0 {
		ev := <-events
		if ev.Kind == harness.ThinkKindToolResult {
			result = ev
		}
	}
	if result.Status != harness.StatusMiss || result.Reason != harness.ReasonUnwired {
		t.Errorf("result event = %#v, want a miss/unwired outcome", result)
	}
	if result.Args != `{"query":"tomorrow"}` {
		t.Errorf("result args = %q, want the call's arguments (that is what pairs them)", result.Args)
	}
}

// TestOuterToolNames pins the tool list the loop's opening step reports: the
// names the outer model was actually given (the pipeline used to hardcode a
// single "rag", regardless of what was bound).
func TestOuterToolNames(t *testing.T) {
	schemas := []map[string]any{
		{"type": "function", "function": map[string]any{"name": "rag"}},
		{"type": "function", "function": map[string]any{"name": "summarize_document"}},
		// A malformed entry contributes nothing rather than a "<nil>" name.
		{"type": "function"},
		{"type": "function", "function": map[string]any{}},
	}
	if got := outerToolNames(schemas); got != "rag, summarize_document" {
		t.Errorf("outerToolNames = %q, want the bound names", got)
	}
	if got := outerToolNames(nil); got != "" {
		t.Errorf("outerToolNames(nil) = %q, want empty", got)
	}
}

// TestOuterLoopEndLine pins the closing step's cases: research ran and answered;
// research ran but composed nothing — the case the call count alone could not see,
// and which would otherwise contradict the rag result line printed right above it
// ("gathered N passages but composed no answer"); or no research ran at all, with
// an answer (the outer model's own reply, which is why it may carry no citations)
// or with none.
func TestOuterLoopEndLine(t *testing.T) {
	cases := []struct {
		name     string
		ragCalls int
		answered bool
		want     string
	}{
		{"answered", 1, true, "The rag tool produced the final answer, done."},
		{"ran-without-answer", 2, false, "The rag tool ran but produced no answer."},
		{"direct-answer", 0, true, "The outer model produced the answer without running research."},
		{"nothing-at-all", 0, false, "The outer model returned no answer and ran no research."},
	}
	for _, tc := range cases {
		if got := outerLoopEndLine(tc.ragCalls, tc.answered); got != tc.want {
			t.Errorf("%s: outerLoopEndLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestOuterLoopAbsentLineStaysOutOfTheThinkBlock pins where that line goes: the
// developer log, not the think block. It describes the harness' wiring rather than
// the question or the research, and a deployment without an outer loop would print
// it on every single run — while the reader-visible trace is simply and honestly
// missing its "[Tool loop]" section.
//
// It also pins that the two reasons stay apart: no outer model wired vs a wired one
// that cannot emit tool calls (the capability probe fails closed, so an operator
// needs the log to tell a failed probe from a deployment that has no outer model).
func TestOuterLoopAbsentLineStaysOutOfTheThinkBlock(t *testing.T) {
	var think, logged strings.Builder
	ctx := harness.WithSteps(context.Background(), harness.StepReporter{
		Text: func(line string) { think.WriteString(line) },
	})
	Rag(ctx, RAGTools{Logger: log.New(&logged, "", 0)}, harness.RunRequest{
		Question:     "anything",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})

	want := "No outer model is wired, so this run has no outer tool loop."
	if !strings.Contains(logged.String(), want) {
		t.Errorf("developer log missing %q; got:\n%s", want, logged.String())
	}
	if strings.Contains(think.String(), "outer tool loop") {
		t.Errorf("the wiring note must not reach the think block; got:\n%s", think.String())
	}

	if got := outerLoopAbsentLine(RAGTools{Outer: &models.ChatModel{}}); got !=
		"The outer model cannot call tools, so this run has no outer tool loop." {
		t.Errorf("outerLoopAbsentLine (tool-incapable model) = %q", got)
	}
}

// TestRagToolResultLine pins the outer `rag` result wording: it reports what the
// answer was grounded in (or why there is none), never the answer itself — and it
// names the sub-question it researched, because the outer loop can have several
// `rag` calls in flight whose result lines would otherwise be identical.
func TestRagToolResultLine(t *testing.T) {
	twoChunks := []map[string]any{{"chunk_id": "c1"}, {"chunk_id": "c2"}}
	label := ` for "曹操是谁"`
	cases := []struct {
		name  string
		resp  *RunResponse
		kb    *harness.Kbinfos
		label string
		want  string
	}{
		{"grounded", &RunResponse{Answer: "cited answer"}, &harness.Kbinfos{Chunks: twoChunks}, label,
			`The rag tool returned a cited answer grounded in 2 passages for "曹操是谁".`},
		{"grounded-singular", &RunResponse{Answer: "a"}, &harness.Kbinfos{Chunks: twoChunks[:1]}, label,
			`The rag tool returned a cited answer grounded in 1 passage for "曹操是谁".`},
		{"no-evidence", &RunResponse{Answer: "answer"}, &harness.Kbinfos{}, label,
			`The rag tool returned no answer for "曹操是谁": research gathered no evidence.`},
		{"evidence-but-no-answer", &RunResponse{}, &harness.Kbinfos{Chunks: twoChunks}, label,
			`The rag tool gathered 2 passages but composed no answer for "曹操是谁".`},
		{"unlabelled", &RunResponse{Answer: "a"}, &harness.Kbinfos{Chunks: twoChunks[:1]}, "",
			"The rag tool returned a cited answer grounded in 1 passage."},
	}
	for _, tc := range cases {
		if got := ragToolResultLine(tc.resp, tc.kb, tc.label); got != tc.want {
			t.Errorf("%s: ragToolResultLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRagFlightSharesConcurrentIdenticalCalls pins the single-flight contract:
// a caller arriving while the flight is open waits on it and replays its
// answer; a caller arriving AFTER the window closed owns a fresh execution
// (window-only dedup) and — like production's defer — must end it. The main
// goroutine holds the window open long enough that the waiters normally take
// the wait path, but the assertions tolerate both arrivals. Run with -race.
func TestRagFlightSharesConcurrentIdenticalCalls(t *testing.T) {
	session := &outerReactSession{}

	owner, wait := session.beginRagFlight("same question")
	if owner == nil || wait != nil {
		t.Fatalf("first caller: owner=%v wait=%v, want (flight, nil)", owner, wait)
	}

	const waiters = 3
	var wg sync.WaitGroup
	got := make([]string, waiters)
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o, w := session.beginRagFlight("same question")
			if o != nil {
				// Arrived after the window closed: this caller owns a fresh
				// execution and must end it (production's ToolCall defers
				// endRagFlight), or later callers would block forever.
				session.endRagFlight("same question", o)
				return
			}
			<-w.done
			got[i] = w.answer
		}(i)
	}

	// Hold the window open so the waiters observe it (scheduling latency is
	// microseconds; the tolerant assertions above cover the pathological case).
	time.Sleep(50 * time.Millisecond)
	owner.answer = "shared answer"
	session.endRagFlight("same question", owner)
	wg.Wait()

	for i, g := range got {
		if g != "shared answer" && g != "" {
			t.Errorf("waiter %d replayed %q, want %q or the owner path (\"\")", i, g, "shared answer")
		}
	}

	// After the flight ends a NEW call owns a fresh execution (window-only
	// dedup: a later round must genuinely re-run).
	owner2, wait2 := session.beginRagFlight("same question")
	if owner2 == nil || wait2 != nil {
		t.Fatalf("post-window caller: owner=%v wait=%v, want (flight, nil)", owner2, wait2)
	}
	session.endRagFlight("same question", owner2)

	// A different question never shares a flight.
	other, otherWait := session.beginRagFlight("different question")
	if other == nil || otherWait != nil {
		t.Fatalf("different question: owner=%v wait=%v, want (flight, nil)", other, otherWait)
	}
	session.endRagFlight("different question", other)
}

// TestOuterReactSessionToolCallWaitsOnInFlightRag pins the ToolCall-level
// behavior: a rag call whose question already has an in-progress execution
// blocks and replays that execution's answer WITHOUT running its own graph —
// no second publish, no second call record (Python's asyncio.gather runs both
// duplicates fully and the terminal fold discards the loser; this wait is the
// approved Go-side single-flight, see ragFlight).
func TestOuterReactSessionToolCallWaitsOnInFlightRag(t *testing.T) {
	spec := harness.GetMode("naive")
	session := &outerReactSession{
		ctx:    context.Background(),
		spec:   spec,
		kb:     &harness.Kbinfos{},
		resp:   &RunResponse{Mode: spec},
		logger: log.New(io.Discard, "", 0),
		req:    harness.RunRequest{Question: "original question"},
	}

	owner, wait := session.beginRagFlight("the question")
	if owner == nil || wait != nil {
		t.Fatalf("pre-registered flight: owner=%v wait=%v, want (flight, nil)", owner, wait)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		owner.answer = "shared answer"
		session.endRagFlight("the question", owner)
	}()

	start := time.Now()
	got, err := session.ToolCall("rag", map[string]interface{}{"question": "the question"})
	if err != nil {
		t.Fatalf("ToolCall: %v", err)
	}
	if got != "shared answer" {
		t.Errorf("ToolCall replayed %q, want %q", got, "shared answer")
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("ToolCall returned after %v — it did NOT wait for the in-flight execution", elapsed)
	}
	// The waiter must not have published or recorded its own call result.
	if len(session.calls) != 0 {
		t.Errorf("calls = %d, want 0 (a waiting call must not publish)", len(session.calls))
	}
}

// ragTestChunkIDs lists a chunk pool's ids in order.
func ragTestChunkIDs(chunks []map[string]any) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		out = append(out, id)
	}
	return out
}

// TestRAGCacheConsecutiveUnanswerableIsSerialized pins that the counter lives
// behind the cache's lock: a round's concurrent rag() calls bump the SAME shared
// cache, so an unsynchronized increment would race and lose updates.
// Run with -race.
func TestRAGCacheConsecutiveUnanswerableIsSerialized(t *testing.T) {
	cache := NewRAGCache()
	const calls = 8

	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.NoteUnanswerable(VerdictInsufficient)
		}()
	}
	wg.Wait()

	if got := cache.ConsecutiveUnanswerable(); got != calls {
		t.Errorf("ConsecutiveUnanswerable = %d, want %d (increments must not be lost)", got, calls)
	}
	cache.NoteUnanswerable(VerdictSufficient)
	if got := cache.ConsecutiveUnanswerable(); got != 0 {
		t.Errorf("after a SUFFICIENT verdict = %d, want 0", got)
	}
	if got := (*RAGCache)(nil).ConsecutiveUnanswerable(); got != 0 {
		t.Errorf("nil cache = %d, want 0", got)
	}
	(*RAGCache)(nil).NoteUnanswerable(VerdictInsufficient) // must not panic
}

// TestOuterReactSessionSelectsWinningCallEvidence pins the citation fix: the
// terminal fold returns the LOWEST-INDEX terminal rag call's answer while
// publish() records completion order, so the session must hand back THAT call's
// evidence — otherwise the answer's [ID:n] markers address another call's chunks.
func TestOuterReactSessionSelectsWinningCallEvidence(t *testing.T) {
	session := &outerReactSession{kb: &harness.Kbinfos{}, resp: &RunResponse{}}
	winner := &harness.Kbinfos{
		Chunks:     []map[string]any{{"chunk_id": "w0"}, {"chunk_id": "w1"}},
		DocAggs:    []map[string]any{{"doc_id": "dw"}},
		Memory:     []map[string]any{{"id": "mw"}},
		PreSummary: "winner summary",
	}
	loser := &harness.Kbinfos{Chunks: []map[string]any{{"chunk_id": "l0"}}}

	// The losing call finishes FIRST, i.e. completion order ≠ index order.
	session.publish(&RunResponse{Answer: "answer-loser"}, loser)
	session.publish(&RunResponse{Answer: "answer-winner"}, winner)

	if got := ragTestChunkIDs(session.kb.Chunks); len(got) != 3 || got[0] != "l0" {
		t.Fatalf("unselected union = %v, want the loser's chunk first (that is the misalignment)", got)
	}

	session.selectEvidence("answer-winner")

	if got := ragTestChunkIDs(session.kb.Chunks); len(got) != 2 || got[0] != "w0" || got[1] != "w1" {
		t.Errorf("chunks after selection = %v, want the winner's [w0 w1] in its own order", got)
	}
	if len(session.kb.DocAggs) != 1 || len(session.kb.Memory) != 1 || session.kb.PreSummary != "winner summary" {
		t.Errorf("winner's evidence fields not restored: %+v", session.kb)
	}
}

// TestOuterReactSessionSelectEvidenceKeepsUnionWithoutAMatch keeps the fallback:
// an answer no rag call produced (the terminal tool was summarize_document, or
// the outer model answered itself) leaves the union in place.
func TestOuterReactSessionSelectEvidenceKeepsUnionWithoutAMatch(t *testing.T) {
	session := &outerReactSession{kb: &harness.Kbinfos{}, resp: &RunResponse{}}
	session.publish(&RunResponse{Answer: "answer-a"}, &harness.Kbinfos{Chunks: []map[string]any{{"chunk_id": "a0"}}})

	session.selectEvidence("a summarize_document result")
	session.selectEvidence("")

	if got := ragTestChunkIDs(session.kb.Chunks); len(got) != 1 || got[0] != "a0" {
		t.Errorf("chunks = %v, want the union kept when no call matches", got)
	}
}

// TestTruncCutsOnRuneBoundaries pins the fix for the "\xe3" that showed up in a
// grep line: trunc used to slice BYTES, cutting a Chinese character in half so
// the trace printed half a rune as an escape (and for CJK it stopped at a third
// of the requested length).
func TestTruncCutsOnRuneBoundaries(t *testing.T) {
	const s = "曹操是谁？"
	if got := trunc(s, 2); got != "曹操" {
		t.Errorf("trunc(%q, 2) = %q, want two whole characters", s, got)
	}
	for n := 0; n <= utf8.RuneCountInString(s)+1; n++ {
		got := trunc(s, n)
		if !utf8.ValidString(got) {
			t.Errorf("trunc(%q, %d) = %q, which is not valid UTF-8", s, n, got)
		}
		if utf8.RuneCountInString(got) > n {
			t.Errorf("trunc(%q, %d) = %q, longer than asked", s, n, got)
		}
	}
	if got := trunc(s, 99); got != s {
		t.Errorf("trunc beyond the length = %q, want it unchanged", got)
	}
}

// retrieveReqRecorder captures every request Retrieve puts on the wire.
type retrieveReqRecorder struct {
	requests []harness.RetrieveRequest
}

func (r *retrieveReqRecorder) Retrieve(_ context.Context, req harness.RetrieveRequest) ([]map[string]any, error) {
	r.requests = append(r.requests, req)
	return []map[string]any{{"chunk_id": "c1", "content": "hit", "doc_id": "d1"}}, nil
}

// newRetrieveDeps binds the recorder to the minimum a search needs to run.
func newRetrieveDeps(r harness.Retriever) harness.SearchDeps {
	return harness.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, TenantID: "tenant1"}
}

// nilTagTagger mirrors a dataset whose label_question finds nothing.
type nilTagTagger struct{}

func (nilTagTagger) LabelQuestion(context.Context, string, []*entity.Knowledgebase) map[string]float64 {
	return nil
}

// TestRetrieveCarriesQuestionTags pins RAGTools.retrieve's rank_feature policy:
// it is the ONE entry point that passes
// rank_feature=label_question(question, self.kbs) (agentic_rag.py:723), so the
// tags the Tagger resolves must reach the retriever.
func TestRetrieveCarriesQuestionTags(t *testing.T) {
	r := &retrieveReqRecorder{}
	sd := newRetrieveDeps(r)
	sd.KBs = []*entity.Knowledgebase{{}}
	sd.Tagger = rfTagger{}
	sd.UsingEmbedding = true

	Retrieve(context.Background(), sd, harness.SearchParams{Question: "who made it?"})

	if len(r.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r.requests))
	}
	want := map[string]float64{"location": 1.0}
	if got := r.requests[0].RankFeature; !reflect.DeepEqual(got, want) {
		t.Errorf("RankFeature = %v, want %v", got, want)
	}
}

// TestRetrieveAsksForHighlight pins the other retrieve-only request flag:
// Python's RAGTools.retrieve is the ONE entry point that passes highlight=True
// (agentic_rag.py:721); search.py's legs all pass highlight=False, which the
// harness asserts separately (TestSearchLegsAskForNoHighlight).
func TestRetrieveAsksForHighlight(t *testing.T) {
	r := &retrieveReqRecorder{}
	sd := newRetrieveDeps(r)
	sd.UsingEmbedding = true

	Retrieve(context.Background(), sd, harness.SearchParams{Question: "who made it?"})

	if len(r.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r.requests))
	}
	if !r.requests[0].Highlight {
		t.Error("Highlight = false, want true (RAGTools.retrieve passes highlight=True)")
	}
}

// TestRetrieveRankFeatureIsNeverNil pins the None semantics: a Tagger that
// resolves nothing — or no Tagger at all — is a VALUE, not an omission. Python
// hands label_question's None straight to retrieval(), which suppresses the
// pagerank default instead of re-enabling it, so the request must carry a
// non-nil empty feature.
func TestRetrieveRankFeatureIsNeverNil(t *testing.T) {
	cases := []struct {
		name   string
		tagger harness.QuestionLabeler
	}{
		{"no tagger", nil},
		{"tagger without tags", nilTagTagger{}},
	}
	for _, tc := range cases {
		r := &retrieveReqRecorder{}
		sd := newRetrieveDeps(r)
		sd.Tagger = tc.tagger
		sd.UsingEmbedding = true

		Retrieve(context.Background(), sd, harness.SearchParams{Question: "q"})

		if len(r.requests) != 1 {
			t.Fatalf("%s: requests = %d, want 1", tc.name, len(r.requests))
		}
		if got := r.requests[0].RankFeature; got == nil || len(got) != 0 {
			t.Errorf("%s: RankFeature = %v, want a non-nil empty map", tc.name, got)
		}
	}
}

// TestRetrievePreset pins the preset RAGTools.retrieve is built from: compiled
// rows are NOT excluded (Python's retrieve has no must_not compile_kwd), and
// `using_embedding` — honoured by this leg alone — decides whether a dense leg
// runs at all.
func TestRetrievePreset(t *testing.T) {
	// using_embedding on: dense leg engaged at the retrieve default weight.
	r := &retrieveReqRecorder{}
	sd := newRetrieveDeps(r)
	sd.UsingEmbedding = true
	Retrieve(context.Background(), sd, harness.SearchParams{Question: "q"})
	req := r.requests[0]
	if req.ExcludeCompiled {
		t.Error("retrieve must NOT exclude compiled rows")
	}
	if req.DisableVectorLeg {
		t.Error("using_embedding on must keep the dense leg")
	}
	if req.VectorSimilarityWeight == nil {
		t.Fatal("VectorSimilarityWeight missing")
	}
	if got := *req.VectorSimilarityWeight; got != harness.DefaultHybridVectorWeight {
		t.Errorf("weight = %v, want %v", got, harness.DefaultHybridVectorWeight)
	}

	// using_embedding off: no dense leg at all, and no weight on the keyword leg.
	r2 := &retrieveReqRecorder{}
	sd2 := newRetrieveDeps(r2)
	Retrieve(context.Background(), sd2, harness.SearchParams{Question: "q"})
	req2 := r2.requests[0]
	if !req2.DisableVectorLeg {
		t.Error("using_embedding off must disable the dense leg")
	}
	if req2.VectorSimilarityWeight == nil {
		t.Fatal("VectorSimilarityWeight missing")
	}
	if got := *req2.VectorSimilarityWeight; got != 0 {
		t.Errorf("weight = %v, want 0 without using_embedding", got)
	}
}

// TestRetrieveDefaults pins the two entry-point defaults that are retrieve's
// alone: top_n 6 — deliberately NOT the search tools' 12
// (agentic_rag.py:669-670) — and highlight=True (agentic_rag.py:721), which no
// search.py leg asks for.
func TestRetrieveDefaults(t *testing.T) {
	r := &retrieveReqRecorder{}
	sd := newRetrieveDeps(r)

	Retrieve(context.Background(), sd, harness.SearchParams{Question: "q"})

	req := r.requests[0]
	if req.TopN != 6 {
		t.Errorf("TopN = %d, want 6 (RAGTools.retrieve's own default)", req.TopN)
	}
	if !req.Highlight {
		t.Error("Highlight = false, want true (RAGTools.retrieve is the only leg that asks for it)")
	}
}

// TestRetrieveKeepsAnExplicitZeroThreshold pins the `is None` semantics of
// Python's similarity_threshold (agentic_rag.py:672): a configured 0 means "no
// floor" and must NOT be read as "unset" and replaced by the 0.2 default.
func TestRetrieveKeepsAnExplicitZeroThreshold(t *testing.T) {
	zero := 0.0
	r := &retrieveReqRecorder{}
	sd := newRetrieveDeps(r)
	sd.SimilarityThreshold = &zero

	Retrieve(context.Background(), sd, harness.SearchParams{Question: "q"})

	req := r.requests[0]
	if req.SimilarityThreshold == nil {
		t.Fatal("SimilarityThreshold missing")
	}
	if got := *req.SimilarityThreshold; got != 0 {
		t.Errorf("similarity threshold = %v, want the explicit 0 (no floor)", got)
	}
}

// TestRetrieveThinkSentence pins the leg's think-block sentence, the fifth leg of
// the family harness.TestSearchLegsShareOneThinkSentenceFamily covers for the
// search.py entry points. Retrieve sits beside the other RAGTools methods, so its
// sentence is asserted here: it must read as the same family ("Searching for …")
// with no implementation phrase and no developer label inside the sentence.
func TestRetrieveThinkSentence(t *testing.T) {
	sd := newRetrieveDeps(&retrieveReqRecorder{})
	sd.UsingEmbedding = true
	var text []string
	ctx := harness.WithSteps(context.Background(), harness.StepReporter{
		Text: func(line string) { text = append(text, line) },
	})

	Retrieve(ctx, sd, harness.SearchParams{Question: "q"})

	if got := strings.Join(text, ""); !strings.Contains(got, `[Retrieve] Searching for "q".`) {
		t.Errorf("think = %q, want the family sentence", got)
	}
}
