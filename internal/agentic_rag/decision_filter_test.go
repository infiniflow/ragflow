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
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/agentic_rag/decisionhead"
	"ragflow/internal/common"
)

// stubDecision replaces the filter's three dependencies for one test, so the drop
// path is covered in the default tier without the checkpoint on disk.
func stubDecision(
	t *testing.T,
	split func(text string) ([]string, string),
	judge func(ctx context.Context, question string, segments []string) []decisionhead.Verdict,
) {
	t.Helper()
	prevAvailable, prevSplit, prevWhole, prevJudge := decisionAvailable, decisionSplit, decisionSplitWhole, decisionJudge
	decisionAvailable = func(context.Context) bool { return true }
	// The split stubs are written without a context; wrapping keeps every caller
	// of this helper unchanged.
	decisionSplit = func(ctx context.Context, text string) ([]string, string) { return split(text) }
	// Whole-chunk mode hands the passage over unchanged unless a test overrides
	// it; the window policy itself is covered by the decisionhead package's tests.
	decisionSplitWhole = func(_ context.Context, text string, _ int) ([]string, string) { return []string{text}, "" }
	decisionJudge = judge
	t.Cleanup(func() {
		decisionAvailable, decisionSplit, decisionSplitWhole, decisionJudge = prevAvailable, prevSplit, prevWhole, prevJudge
	})
}

// gateMode pins the pre-exemption behaviour for tests that are ABOUT the gate:
// with no exemption every passage is judged and an insufficient one is dropped.
// The default now exempts the leading passages, so a test that means "judge
// everything" has to say so — otherwise it silently stops covering the drop
// path the moment the default moves.
func gateMode(t *testing.T) {
	t.Helper()
	t.Setenv(common.EnvDecisionKeepTopN, "0")
}

// The exemption is the default because the plain gate measured worst; an
// explicit 0 must still be able to restore it, or the knob would be a one-way
// switch.
func TestKeepTopNDefaultsToExemptionAndHonoursExplicitZero(t *testing.T) {
	if got := keepTopN(); got != defaultKeepTopN {
		t.Fatalf("unset = %d, want %d", got, defaultKeepTopN)
	}
	t.Setenv(common.EnvDecisionKeepTopN, "0")
	if got := keepTopN(); got != 0 {
		t.Fatalf("explicit 0 = %d, want the gate restored", got)
	}
	t.Setenv(common.EnvDecisionKeepTopN, "5")
	if got := keepTopN(); got != 5 {
		t.Fatalf("explicit 5 = %d, want 5", got)
	}
	// A typo must not turn into "judge everything": that is the behaviour the
	// default moved away from.
	t.Setenv(common.EnvDecisionKeepTopN, "not-a-number")
	if got := keepTopN(); got != defaultKeepTopN {
		t.Fatalf("garbage = %d, want the default %d", got, defaultKeepTopN)
	}
	t.Setenv(common.EnvDecisionKeepTopN, "-1")
	if got := keepTopN(); got != defaultKeepTopN {
		t.Fatalf("negative = %d, want the default %d", got, defaultKeepTopN)
	}
}

// wholeText splits nothing: the passage is judged as one segment, which is the
// shape a short chunk takes.
func wholeText() func(string) ([]string, string) {
	return func(text string) ([]string, string) { return []string{text}, "" }
}

// verdictsOf echoes the segments back with the given usefulness, mirroring what
// the decisionhead package returns.
func verdictsOf(useful ...bool) func(context.Context, string, []string) []decisionhead.Verdict {
	return func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
		out := make([]decisionhead.Verdict, len(segments))
		for i, segment := range segments {
			// The claim exit reads Probability, so a stub that only sets Useful would
			// look like a 0.0 verdict to it. 0.9 clears both thresholds, 0.1 clears
			// neither.
			p := 0.1
			if useful[i] {
				p = 0.9
			}
			out[i] = decisionhead.Verdict{Segment: segment, Useful: useful[i], Judged: true, Probability: p}
		}
		return out
	}
}

func TestFilterPassesThroughWithoutQuestion(t *testing.T) {
	stubDecision(t, wholeText(), verdictsOf(false))
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "text"}}
	got := filterChunks(context.Background(), chunks)
	if len(got) != 1 || got[0].Content != "text" {
		t.Fatalf("filtered without a question: %+v", got)
	}
}

func TestFilterPassesThroughWhenDecisionUnavailable(t *testing.T) {
	stubDecision(t, wholeText(), verdictsOf(false))
	decisionAvailable = func(context.Context) bool { return false }
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "text"}}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 1 || got[0].Content != "text" {
		t.Fatalf("filtered with the judge unavailable: %+v", got)
	}
}

func TestFilterKeepsOnlyUsefulSegments(t *testing.T) {
	gateMode(t)
	stubDecision(t,
		func(string) ([]string, string) { return []string{"a|", "b|", "c"}, "" },
		verdictsOf(true, false, true))
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "a|b|c"}}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 1 {
		t.Fatalf("kept %d chunks, want 1", len(got))
	}
	if got[0].Content != "a|c" {
		t.Fatalf("content = %q, want the useful segments only", got[0].Content)
	}
}

func TestFilterDropsChunkWithNoUsefulSegment(t *testing.T) {
	gateMode(t)
	stubDecision(t,
		func(string) ([]string, string) { return []string{"a|", "b"}, "" },
		verdictsOf(false, false))
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "a|b"}}
	if got := filterChunks(WithQuery(context.Background(), "q"), chunks); len(got) != 0 {
		t.Fatalf("kept %+v, want the chunk dropped", got)
	}
}

// The scope is by TOOL, from the corpus-passage allowlist: those are the exits
// whose hits are passages the head was trained to read, so they are the only
// ones that can pay for a prune.
func TestSnippetFilterScopeDefaultsToCorpusLocateTools(t *testing.T) {
	for _, tool := range []string{
		searchBm25ChunksToolName, searchSemanticChunksToolName, searchChunksToolName, grepChunksToolName,
	} {
		if !snippetFilterAppliesTo(tool) {
			t.Errorf("%s must be in the default scope", tool)
		}
	}
	// search_metadata returns a document-id selector and never reaches this exit;
	// web hits carry no corpus document to judge. Both must stay out.
	for _, tool := range []string{metadataSearchToolName, webSearchToolName} {
		if snippetFilterAppliesTo(tool) {
			t.Errorf("%s must not be in the default scope", tool)
		}
	}
}

// An override REPLACES the scope rather than extending it, so a run can narrow
// the filter to a single leg without a rebuild.
func TestSnippetFilterScopeOverrideReplacesDefault(t *testing.T) {
	t.Setenv(common.EnvDecisionSnippetTools, "grep_chunks, custom_tool")
	if !snippetFilterAppliesTo(grepChunksToolName) || !snippetFilterAppliesTo("custom_tool") {
		t.Fatal("override must be honoured")
	}
	if snippetFilterAppliesTo(searchBm25ChunksToolName) {
		t.Fatal("an override must replace the default scope, not extend it")
	}
}

// A tool outside the scope keeps every hit, whatever the model would have said.
func TestSnippetFilterLeavesOutOfScopeToolAlone(t *testing.T) {
	t.Setenv(common.EnvDecisionFilterSnippet, "true")
	stubDecision(t, wholeText(), verdictsOf(false))
	hits := []snippetHit{{chunk: runtime.RetrievalChunk{ID: "c1"}, snippet: "text"}}
	if got := filterSnippetHits(WithQuery(context.Background(), "q"), metadataSearchToolName, hits); len(got) != 1 {
		t.Fatalf("pruned a tool outside the scope: %+v", got)
	}
}

// The grep exit is wired to the same filter as bm25: both render through
// formatLocateResultsXML, and both are in the default scope.
func TestGrepExitRunsTheSnippetFilter(t *testing.T) {
	gateMode(t)
	t.Setenv(common.EnvDecisionFilterSnippet, "true")
	stubDecision(t, wholeText(), verdictsOf(false))
	hits := []snippetHit{{chunk: runtime.RetrievalChunk{ID: "c1"}, snippet: "text"}}
	ctx := WithQuery(WithEvidenceRegistry(context.Background(), NewEvidenceRegistry()), "q")
	out := formatLocateResultsXML(ctx, grepChunksToolName, "q", hits)
	if strings.Contains(out, "text") {
		t.Fatalf("grep hit survived a rejection: %q", out)
	}
}

// The exemption is what makes this tail pruning instead of a gate: the leading
// passages survive whatever the model says, so a wrong verdict can shorten a
// result set but can never empty it.
func TestFilterKeepsTopNRegardlessOfVerdict(t *testing.T) {
	t.Setenv(common.EnvDecisionKeepTopN, "2")
	stubDecision(t, wholeText(), verdictsOf(false))
	chunks := []runtime.RetrievalChunk{
		{ID: "a", Content: "1"}, {ID: "b", Content: "2"}, {ID: "c", Content: "3"},
	}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("kept %+v, want only the first two passages, unjudged", got)
	}
}

func TestSnippetFilterKeepsTopNHits(t *testing.T) {
	t.Setenv(common.EnvDecisionFilterSnippet, "true")
	t.Setenv(common.EnvDecisionKeepTopN, "1")
	stubDecision(t, wholeText(), verdictsOf(false))
	hits := []snippetHit{
		{chunk: runtime.RetrievalChunk{ID: "a"}, snippet: "x"},
		{chunk: runtime.RetrievalChunk{ID: "b"}, snippet: "y"},
	}
	got := filterSnippetHits(WithQuery(context.Background(), "q"), searchBm25ChunksToolName, hits)
	if len(got) != 1 || got[0].chunk.ID != "a" {
		t.Fatalf("kept %+v, want only the top hit", got)
	}
}

// The miss rate is recomputed from this line alone, so the identity and the
// position of every passage must be on it — an exempted passage was never asked
// about, and leaving it out of the log would make the filter look like it had
// kept text it never actually passed on.
func TestDecisionLogCarriesIdentityAndPosition(t *testing.T) {
	p := passage{side: filterSideDeepRead, tool: grepChunksToolName, chunkID: "c1", docID: "d1", chunkIndex: 4, pos: 2}

	ctx := withIntent(context.Background(), "the year the treaty was signed")
	got := fieldMap(decisionFields(ctx, p, 1, decisionhead.Verdict{
		Tokens: 512, Probability: 0.87, Judged: true, Useful: true,
	}))
	// The intent is logged with the verdict: an intent the agent wrote vaguely is
	// a fact about the run that no later reconstruction could recover.
	if got["intent"] != "the year the treaty was signed" {
		t.Errorf("intent missing from the decision log: %+v", got)
	}
	if got["side"] != "deep_read" || got["chunk_id"] != "c1" || got["doc_id"] != "d1" {
		t.Fatalf("identity missing: %+v", got)
	}
	if got["pos"] != 2 || got["chunk_index"] != 4 || got["seg_idx"] != 1 {
		t.Fatalf("position wrong: %+v", got)
	}
	if got["tool"] != "grep_chunks" {
		t.Fatalf("the exit that produced the passage must be on the line: %+v", got)
	}
	if got["exempt"] != false || got["judged"] != true || got["kept"] != true {
		t.Fatalf("verdict wrong: %+v", got)
	}

	want := fieldMap(exemptFields(p, "some text"))
	if want["exempt"] != true || want["judged"] != false || want["kept"] != true {
		t.Fatalf("an exempted passage must read as kept-but-unjudged: %+v", want)
	}
	if want["pos"] != 2 || want["chunk_id"] != "c1" || want["tool"] != "grep_chunks" {
		t.Fatalf("an exempted passage must still be identifiable: %+v", want)
	}
}

// An explicit 0 restores the gate the filter used to be, by default.
func TestExplicitZeroRestoresTheGate(t *testing.T) {
	gateMode(t)
	stubDecision(t, wholeText(), verdictsOf(false))
	chunks := []runtime.RetrievalChunk{{ID: "a", Content: "1"}, {ID: "b", Content: "2"}}
	if got := filterChunks(WithQuery(context.Background(), "q"), chunks); len(got) != 0 {
		t.Fatalf("kept %+v, want both passages dropped without the exemption", got)
	}
}

// Text past the segment bound was never judged, so it is kept: an unpaid bill is
// not a rejection.
func TestFilterAppendsOverflowUnjudged(t *testing.T) {
	gateMode(t)
	stubDecision(t,
		func(string) ([]string, string) { return []string{"a|"}, "b|c" },
		verdictsOf(false))
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "a|b|c"}}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 1 || got[0].Content != "b|c" {
		t.Fatalf("unjudged overflow was not kept: %+v", got)
	}
}

// A segment the model never ruled on is kept, so a degraded judgement can never
// cost the caller evidence.
func TestFilterKeepsUnjudgedSegments(t *testing.T) {
	stubDecision(t,
		wholeText(),
		func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
			out := make([]decisionhead.Verdict, len(segments))
			for i, segment := range segments {
				out[i] = decisionhead.Verdict{Segment: segment, Useful: true, Judged: false}
			}
			return out
		})
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "text"}}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 1 || got[0] != chunks[0] {
		t.Fatalf("an unjudged passage was altered or dropped: %+v", got)
	}
}

// Nothing cut means nothing to rewrite: the passage comes back byte-identical,
// so a run whose filter keeps everything is indistinguishable from one that
// never filtered.
func TestFilterKeepsChunkUntouchedWhenNothingWasCut(t *testing.T) {
	stubDecision(t, wholeText(), verdictsOf(true))
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "text", ChunkIndex: 7}}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 1 || got[0] != chunks[0] {
		t.Fatalf("content = %+v, want the original chunk", got)
	}
}

// The load-bearing ordering rule: a passage the filter drops is dropped BEFORE
// the [ID:n] handles are stamped, so it must not consume one — the passage the
// model does see has to start at 0, not 1.
func TestFilteredChunkConsumesNoRefHandle(t *testing.T) {
	gateMode(t)
	stubDecision(t,
		wholeText(),
		func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
			out := make([]decisionhead.Verdict, len(segments))
			for i, segment := range segments {
				out[i] = decisionhead.Verdict{Segment: segment, Useful: segment != "drop me", Judged: true}
			}
			return out
		})
	chunks := []runtime.RetrievalChunk{
		{ID: "gone", Content: "drop me", DocumentID: "doc-1"},
		{ID: "kept", Content: "useful text", DocumentID: "doc-1"},
	}
	ctx := WithQuery(WithEvidenceRegistry(context.Background(), NewEvidenceRegistry()), "q")
	out := formatChunksXML(ctx, "doc-1", chunks, "", "")

	if strings.Contains(out, `chunk_id="gone"`) {
		t.Fatalf("dropped passage reached the model: %q", out)
	}
	if !strings.Contains(out, `ref="0"`) {
		t.Fatalf("kept passage did not take handle 0: %q", out)
	}
	if strings.Contains(out, `ref="1"`) {
		t.Fatalf("the dropped passage consumed a handle: %q", out)
	}
	if !strings.Contains(out, `fetched="1"`) {
		t.Fatalf("fetched does not count the rendered passages: %q", out)
	}
}

// Without a question on the context the renderer must behave exactly as it did
// before the filter existed — same attribute, same single passage.
func TestChunksXMLUnfilteredKeepsFetchedCount(t *testing.T) {
	chunks := []runtime.RetrievalChunk{{ID: "d1"}, {ID: "d2"}}
	out := formatChunksXML(WithEvidenceRegistry(context.Background(), NewEvidenceRegistry()),
		"doc-1", chunks, "", "")
	if !strings.Contains(out, `fetched="2"`) {
		t.Fatalf("fetched changed on an unfiltered run: %q", out)
	}
}

// The snippet exit is opt-in: an unset env must leave the locate path exactly as
// it was, because a dropped locate hit also removes the anchor the model needs
// before it can deep-read.
func TestSnippetFilterOffByDefault(t *testing.T) {
	stubDecision(t, wholeText(), verdictsOf(false))
	hits := []snippetHit{{chunk: runtime.RetrievalChunk{ID: "c1", Content: "text"}, snippet: "text"}}
	if got := filterSnippetHits(WithQuery(context.Background(), "q"), searchBm25ChunksToolName, hits); len(got) != 1 {
		t.Fatalf("snippet filter ran while disabled: %+v", got)
	}
}

func TestSnippetFilterDropsInsufficientHits(t *testing.T) {
	gateMode(t)
	t.Setenv(common.EnvDecisionFilterSnippet, "true")
	stubDecision(t, wholeText(), func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
		out := make([]decisionhead.Verdict, len(segments))
		for i, segment := range segments {
			out[i] = decisionhead.Verdict{Segment: segment, Useful: strings.Contains(segment, "keep"), Judged: true}
		}
		return out
	})
	hits := []snippetHit{
		{chunk: runtime.RetrievalChunk{ID: "a"}, snippet: "keep this"},
		{chunk: runtime.RetrievalChunk{ID: "b"}, snippet: "drop this"},
	}
	got := filterSnippetHits(WithQuery(context.Background(), "q"), searchBm25ChunksToolName, hits)
	if len(got) != 1 || got[0].chunk.ID != "a" {
		t.Fatalf("kept %+v, want only the sufficient hit", got)
	}
}

// The whole-passage window is a judging-window change, not a body change: a
// passage the model accepts must reach the caller byte-identical, and it must be
// asked about once rather than sub-segment by sub-segment.
func TestWholeChunkJudgesOnceAndKeepsPassageIntact(t *testing.T) {
	gateMode(t)
	t.Setenv(common.EnvDecisionWholeChunk, "true")
	calls := 0
	stubDecision(t, wholeText(), func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
		calls++
		return verdictsOf(true)(context.Background(), "", segments)
	})
	long := strings.Repeat("a line of passage text\n", 40)
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: long}}
	got := filterChunks(WithQuery(context.Background(), "q"), chunks)
	if len(got) != 1 || got[0].Content != long {
		t.Fatalf("whole-chunk mode altered the served passage: %d chunks", len(got))
	}
	if calls != 1 {
		t.Fatalf("judged %d times, want exactly once per passage", calls)
	}
}

func TestWholeChunkDropsPassageWhenWindowIsNotUseful(t *testing.T) {
	gateMode(t)
	t.Setenv(common.EnvDecisionWholeChunk, "true")
	stubDecision(t, wholeText(), verdictsOf(false))
	chunks := []runtime.RetrievalChunk{{ID: "c1", Content: "text"}}
	if got := filterChunks(WithQuery(context.Background(), "q"), chunks); len(got) != 0 {
		t.Fatalf("kept %+v, want the passage dropped", got)
	}
}

// With serve-once on, the second render of the same body is a STUB: the element
// and its handle survive (so `fetched` and citations still hold) but the text is
// not re-sent, and the model is told why.
func TestServeOnceReplacesRepeatedBodyWithStub(t *testing.T) {
	t.Setenv(common.EnvAgenticServeOnce, "true")
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	chunks := []runtime.RetrievalChunk{{ID: "c1", DocumentID: "doc-1", Content: "the body"}}

	first := formatChunksXML(ctx, "doc-1", chunks, "", "")
	if strings.Contains(first, "repeat=") {
		t.Fatalf("first serve marked as a repeat: %q", first)
	}
	second := formatChunksXML(ctx, "doc-1", chunks, "", "")
	if !strings.Contains(second, `repeat="true"`) {
		t.Fatalf("second serve not marked as a repeat: %q", second)
	}
	if strings.Contains(second, "the body") {
		t.Fatalf("second serve re-sent the body: %q", second)
	}
	if !strings.Contains(second, `ref="0"`) || !strings.Contains(second, `fetched="1"`) {
		t.Fatalf("stub lost its handle or element: %q", second)
	}
	if !strings.Contains(second, "<repeat_note>") {
		t.Fatalf("stub carries no explanation: %q", second)
	}
}

// The same rule covers the snippet exit, which carries most of the served text.
func TestServeOnceReplacesRepeatedSnippet(t *testing.T) {
	t.Setenv(common.EnvAgenticServeOnce, "true")
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	hits := []snippetHit{{chunk: runtime.RetrievalChunk{ID: "c1", DocumentID: "doc-1"}, snippet: "a window"}}

	formatLocateResultsXML(ctx, searchBm25ChunksToolName, "q", hits)
	second := formatLocateResultsXML(ctx, searchBm25ChunksToolName, "q", hits)
	if strings.Contains(second, "a window") || !strings.Contains(second, `repeat="true"`) {
		t.Fatalf("repeated snippet was re-sent: %q", second)
	}
}

// Off by default: without the switch the renderer behaves exactly as before, so
// an upgrade cannot silently stop showing passage text.
func TestServeOnceOffByDefault(t *testing.T) {
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	chunks := []runtime.RetrievalChunk{{ID: "c1", DocumentID: "doc-1", Content: "the body"}}
	formatChunksXML(ctx, "doc-1", chunks, "", "")
	second := formatChunksXML(ctx, "doc-1", chunks, "", "")
	if strings.Contains(second, "repeat=") || !strings.Contains(second, "the body") {
		t.Fatalf("default behaviour changed: %q", second)
	}
}

// The ordering rule holds at the locate exit too: a hit the filter drops must
// not consume an [ID:n] handle.
func TestDroppedSnippetConsumesNoRefHandle(t *testing.T) {
	gateMode(t)
	t.Setenv(common.EnvDecisionFilterSnippet, "true")
	stubDecision(t, wholeText(), func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
		out := make([]decisionhead.Verdict, len(segments))
		for i, segment := range segments {
			out[i] = decisionhead.Verdict{Segment: segment, Useful: segment != "drop me", Judged: true}
		}
		return out
	})
	hits := []snippetHit{
		{chunk: runtime.RetrievalChunk{ID: "gone", DocumentID: "doc-1"}, snippet: "drop me"},
		{chunk: runtime.RetrievalChunk{ID: "kept", DocumentID: "doc-1"}, snippet: "useful"},
	}
	ctx := WithQuery(WithEvidenceRegistry(context.Background(), NewEvidenceRegistry()), "q")
	out := formatLocateResultsXML(ctx, searchBm25ChunksToolName, "q", hits)

	if strings.Contains(out, `chunk_id="gone"`) {
		t.Fatalf("dropped hit reached the model: %q", out)
	}
	if !strings.Contains(out, `ref="0"`) {
		t.Fatalf("kept hit did not take handle 0: %q", out)
	}
	if strings.Contains(out, `ref="1"`) {
		t.Fatalf("the dropped hit consumed a handle: %q", out)
	}
	if !strings.Contains(out, `count="1"`) {
		t.Fatalf("count does not reflect the served hits: %q", out)
	}
}

// The claim exit judges LINES, not pages: a claim is an atomic statement, so one
// the question does not need leaves on its own without taking its neighbours with
// it. This is the only length bound the claim page has.
func TestClaimFilterDropsUnusefulClaimLines(t *testing.T) {
	stubDecision(t, wholeText(), verdictsOf(true, false, true))
	claims := []claimHit{
		{name: "a", chunkID: "c1"},
		{name: "b", chunkID: "c1"},
		{name: "c", chunkID: "c1"},
	}
	got := filterClaims(context.Background(), "q", claims)
	if len(got) != 2 || got[0].name != "a" || got[1].name != "c" {
		t.Fatalf("kept %+v, want a and c", got)
	}
}

func TestClaimFilterPassesThroughWhenDecisionUnavailable(t *testing.T) {
	stubDecision(t, wholeText(), verdictsOf(false, false))
	decisionAvailable = func(context.Context) bool { return false }
	claims := []claimHit{{name: "a"}, {name: "b"}}
	if got := filterClaims(context.Background(), "q", claims); len(got) != 2 {
		t.Fatalf("kept %d claims with the judge unavailable, want 2", len(got))
	}
}

func TestClaimFilterPassesThroughWhenDisabled(t *testing.T) {
	t.Setenv(common.EnvDecisionFilterClaim, "0")
	stubDecision(t, wholeText(), verdictsOf(false, false))
	claims := []claimHit{{name: "a"}, {name: "b"}}
	if got := filterClaims(context.Background(), "q", claims); len(got) != 2 {
		t.Fatalf("kept %d claims with the exit disabled, want 2", len(got))
	}
}

// Unjudged is not a verdict: a budget that ran out must not cost the page its
// evidence.
func TestClaimFilterKeepsUnjudgedLines(t *testing.T) {
	stubDecision(t, wholeText(), func(context.Context, string, []string) []decisionhead.Verdict {
		return []decisionhead.Verdict{{Segment: "a"}, {Segment: "b"}}
	})
	claims := []claimHit{{name: "a"}, {name: "b"}}
	if got := filterClaims(context.Background(), "q", claims); len(got) != 2 {
		t.Fatalf("kept %d claims, want both unjudged lines kept", len(got))
	}
}

// claimPage builds a page whose first `head` statements belong to c1 and whose
// tail spreads over c2..c4, which is the shape a real page has: one ranked chunk
// contributing most of the statements and the rest contributing a few.
func claimPage(head, tail int) []claimHit {
	out := make([]claimHit, 0, head+tail)
	for i := 0; i < head; i++ {
		out = append(out, claimHit{name: fmt.Sprintf("head-%d", i), chunkID: "c1"})
	}
	chunks := []string{"c2", "c3", "c4"}
	for i := 0; i < tail; i++ {
		out = append(out, claimHit{name: fmt.Sprintf("tail-%d", i), chunkID: chunks[i%len(chunks)]})
	}
	return out
}

// The judge bound is enforced BEFORE the judge is asked: the page is cut to the
// bound plus one statement per chunk the bound did not cover, and only that many
// statements are sent. Without this, a 372-statement page is 47 judge round trips
// for a single retrieval round - the cost that dominated the turn.
func TestClaimPageCappedBeforeJudge(t *testing.T) {
	t.Setenv(common.EnvDecisionClaimJudgeCap, "16")
	var judged int
	stubDecision(t, wholeText(), func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
		judged = len(segments)
		out := make([]decisionhead.Verdict, len(segments))
		for i := range segments {
			out[i] = decisionhead.Verdict{Segment: segments[i], Useful: true, Judged: true, Probability: 0.9}
		}
		return out
	})
	claims := claimPage(16, 4)
	got := filterClaims(context.Background(), "q", claims)
	// 16 leading + c2, c3, c4 (c2 repeats at tail-3 and must not be counted twice).
	const want = 19
	if judged != want {
		t.Fatalf("judged %d statements, want the capped page of %d", judged, want)
	}
	if len(got) != want {
		t.Fatalf("kept %d claims, want %d", len(got), want)
	}
	for _, chunk := range []string{"c1", "c2", "c3", "c4"} {
		found := false
		for _, c := range got {
			if c.chunkID == chunk {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("chunk %s lost its representative: %+v", chunk, got)
		}
	}
}

// A dead judge must still not let an unbounded page into the prompt: the bound is
// the page's contract, not the judge's convenience.
func TestClaimPageCappedEvenWhenJudgeUnavailable(t *testing.T) {
	t.Setenv(common.EnvDecisionClaimJudgeCap, "16")
	prev := decisionAvailable
	decisionAvailable = func(context.Context) bool { return false }
	t.Cleanup(func() { decisionAvailable = prev })
	got := filterClaims(context.Background(), "q", claimPage(16, 4))
	if len(got) != 19 {
		t.Fatalf("kept %d claims with the judge down, want the capped page of 19", len(got))
	}
}

// A page inside the bound is passed through untouched - capping must never
// reorder or drop a small page.
func TestClaimPageUnderTheCapIsUntouched(t *testing.T) {
	t.Setenv(common.EnvDecisionClaimJudgeCap, "16")
	stubDecision(t, wholeText(), verdictsOf(true, true, true))
	claims := []claimHit{
		{name: "a", chunkID: "c1"},
		{name: "b", chunkID: "c2"},
		{name: "c", chunkID: "c3"},
	}
	got := filterClaims(context.Background(), "q", claims)
	if len(got) != 3 || got[0].name != "a" || got[1].name != "b" || got[2].name != "c" {
		t.Fatalf("kept %+v, want the page untouched", got)
	}
}

// claimProbVerdicts hands every claim line the given probability and judges all
// of them NOT useful, which is the case the floor exists for: nothing passes the
// absolute threshold, yet the page still has to hold one statement per chunk.
func claimProbVerdicts(probs ...float64) func(context.Context, string, []string) []decisionhead.Verdict {
	return func(_ context.Context, _ string, segments []string) []decisionhead.Verdict {
		out := make([]decisionhead.Verdict, len(segments))
		for i := range segments {
			p := 0.0
			if i < len(probs) {
				p = probs[i]
			}
			out[i] = decisionhead.Verdict{Segment: segments[i], Useful: false, Judged: true, Probability: p}
		}
		return out
	}
}

// One statement per CHUNK survives, chosen two ways: the chunk's own best
// probability (a chunk nobody scored well still gets to speak for itself) and the
// page-relative threshold (a statement close to the page's best is on topic even
// when the absolute scale says otherwise). A claim that is neither is dropped.
func TestClaimFilterKeepsOnePerChunkByFloor(t *testing.T) {
	t.Setenv(common.EnvDecisionClaimFloor, "1")
	// pageMax = 0.35, so the relative threshold is 0.14.
	stubDecision(t, wholeText(), claimProbVerdicts(0.10, 0.05, 0.35))
	claims := []claimHit{
		{name: "a", chunkID: "c1"},
		{name: "b", chunkID: "c1"},
		{name: "c", chunkID: "c2"},
	}
	got := filterClaims(context.Background(), "q", claims)
	if len(got) != 2 || got[0].name != "a" || got[1].name != "c" {
		t.Fatalf("kept %+v, want a (c1's best, below the relative threshold) and c (above it)", got)
	}
}

// With the floor off the exit is the old absolute one: every line is dropped and
// the page falls back to its single best-ranked claim.
func TestClaimFilterWithoutFloorDropsTheWholePage(t *testing.T) {
	t.Setenv(common.EnvDecisionClaimFloor, "0")
	stubDecision(t, wholeText(), claimProbVerdicts(0.10, 0.05, 0.20))
	claims := []claimHit{
		{name: "a", chunkID: "c1"},
		{name: "b", chunkID: "c1"},
		{name: "c", chunkID: "c2"},
	}
	got := filterClaims(context.Background(), "q", claims)
	if len(got) != 1 || got[0].name != "a" {
		t.Fatalf("kept %+v, want only the best-ranked claim without the floor", got)
	}
}

// A statement is not a passage. The claim exit rules on its own threshold because
// a thirty-token fact and a thousand-token chunk do not share a score scale, and
// borrowing the passage number is how a correctly recalled statement gets dropped.
func TestClaimThresholdIsSeparateFromPassageThreshold(t *testing.T) {
	t.Setenv(common.EnvDecisionMinProb, "0.30")
	t.Setenv(common.EnvDecisionClaimMinProb, "0.25")
	stubDecision(t, wholeText(), claimProbVerdicts(0.27))
	got := filterClaims(context.Background(), "q", []claimHit{{name: "a", chunkID: "c1"}})
	if len(got) != 1 || got[0].name != "a" {
		t.Fatalf("kept %+v, want the 0.27 statement kept by the 0.25 claim threshold", got)
	}
}

// ...and the defaults are the split the deployment expects: 0.30 for passages,
// 0.25 for statements.
func TestClaimMinProbDefaults(t *testing.T) {
	t.Setenv(common.EnvDecisionMinProb, "")
	t.Setenv(common.EnvDecisionClaimMinProb, "")
	if got := claimMinProb(); got != 0.25 {
		t.Fatalf("claim min prob = %v, want 0.25", got)
	}
	if defaultDecisionMinProb != 0.30 {
		t.Fatalf("passage min prob = %v, want 0.30", defaultDecisionMinProb)
	}
}

// A page the model judged entirely useless keeps its best-ranked claim: an empty
// page makes the callers serve the raw chunk instead, which costs more text than
// the claim it replaced.
func TestClaimFilterKeepsBestRankedClaimWhenEveryLineDrops(t *testing.T) {
	// Floor off: the floor exists to keep something, so the all-dropped fallback is
	// only observable without it.
	t.Setenv(common.EnvDecisionClaimFloor, "0")
	stubDecision(t, wholeText(), verdictsOf(false, false))
	claims := []claimHit{{name: "a"}, {name: "b"}}
	got := filterClaims(context.Background(), "q", claims)
	if len(got) != 1 || got[0].name != "a" {
		t.Fatalf("kept %+v, want the best-ranked claim only", got)
	}
}
