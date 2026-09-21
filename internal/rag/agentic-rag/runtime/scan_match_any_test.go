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

package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestTheScanMatchesContainmentNotRankPosition pins what makes the scan a scan: a passage is a hit when
// it literally carries one of the terms, whatever position the index gave it, and a passage that
// carries none is not one.
//
// Every other channel here is a RANKED search, and the ranking answers "what matches best" while an
// enumeration asks "where does this occur" — measured 2026-09-20 (三国/关羽): the answer equalled the
// pool's coverage in six runs (10/10/10/15/11/13) and the missing members were candidates the ranking
// cut (程远志 ranked 9 under a per-query cap of 8, twice in a row).
func TestTheScanMatchesContainmentNotRankPosition(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		// Carries an actor form AND an act word, whatever position the index gave it.
		{"chunk_id": "c-last", "doc_id": "d1", "content": "荀正出马，云长交马一合，砍荀正于马下。"},
		{"chunk_id": "c-mid", "doc_id": "d1", "content": "关羽手起刀落，斩杨龄于马下。"},
		// An act word with NO actor form is not the actor's deed, and neither is the actor alone.
		{"chunk_id": "c-act-only", "doc_id": "d2", "content": "交马一合，砍荀正于马下。"},
		{"chunk_id": "c-actor-only", "doc_id": "d3", "content": "关公曰：此马甚骏。"},
		{"chunk_id": "c-none", "doc_id": "d4", "content": "此人以水攻七军，威震华夏。"},
	}}
	deps, _ := newTestSearchDeps(r)
	res := ScanMatchAny(context.Background(), deps, []string{"关羽 斩", "云长 砍"}, nil, nil, 0)

	if res.Matched != 2 || res.Delivered != 2 || res.Remaining != 0 {
		t.Fatalf("matched/delivered/remaining = %d/%d/%d, want 2/2/0: the probe is a CONJUNCTION of its words, so an act word without the actor is not the actor's deed",
			res.Matched, res.Delivered, res.Remaining)
	}
	got := map[string]bool{}
	for _, w := range res.Windows {
		got[w.ChunkID] = true
	}
	if !got["c-last"] || !got["c-mid"] {
		t.Errorf("windows = %v, want both passages that carry a term", got)
	}
	if got["c-none"] {
		t.Error("a passage carrying no term was delivered as a match")
	}
	// The keyword call is the dense-leg-free one: a scan must not depend on an embedding service.
	if len(r.requests) == 0 || !keywordOnlyLeg(r.requests[0]) {
		t.Errorf("scan request weight = %v, want the keyword-only leg", r.requests)
	}
}

// TestTheScanTruncatesByBudgetAndSaysHowMuchIsLeft pins the other half: the cut is the DELIVERY budget,
// and it is reported.
//
// This is the number an enumeration cannot see for itself — how much of the corpus's matching material
// it has NOT been shown — and reporting it is what stops a delivery cut from reading as "the corpus
// does not carry them" (see ScanResult.Line).
func TestTheScanTruncatesByBudgetAndSaysHowMuchIsLeft(t *testing.T) {
	var chunks []map[string]any
	for i := 0; i < 40; i++ {
		chunks = append(chunks, map[string]any{
			"chunk_id": fmt.Sprintf("c-%02d", i),
			"doc_id":   "d1",
			"content":  fmt.Sprintf("云长手起刀落，斩%d将于马下，后又砍之。", i) + strings.Repeat("后文甚长，", 40),
		})
	}
	r := &stubRetriever{chunks: chunks}
	deps, _ := newTestSearchDeps(r)
	res := ScanMatchAny(context.Background(), deps, []string{"云长 斩"}, nil, nil, 100)

	if res.Matched != 40 {
		t.Fatalf("matched = %d, want all 40 windows in the recalled set", res.Matched)
	}
	if res.Delivered >= res.Matched {
		t.Fatalf("delivered = %d of %d with a 100-char budget, want the delivery cut", res.Delivered, res.Matched)
	}
	if res.Remaining != res.Matched-res.Delivered {
		t.Errorf("remaining = %d, want matched-delivered (%d)", res.Remaining, res.Matched-res.Delivered)
	}
	line := res.Line()
	for _, want := range []string{"[scan] probed 云长 斩", "matched", "remaining", "DELIVERY BUDGET"} {
		if !strings.Contains(line, want) {
			t.Errorf("scan line %q missing %q", line, want)
		}
	}
	// A name nothing carries is not a corpus fact until the delivery has been exhausted, and the line
	// says which cut it was: a scan that matched nothing says so plainly.
	noneDeps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c-x", "doc_id": "d9", "content": "与此无关的一段话。"},
	}})
	none := ScanMatchAny(context.Background(), noneDeps, []string{"关羽 斩"}, nil, nil, 0)
	// A miss names the PROBES and says which of the two happened — the index recalled nothing for them,
	// or it recalled passages and none carries every word of a probe. A blind "no window" is the one
	// thing this line may not be: it cannot be told from "the scan never ran".
	if none.Matched != 0 || !strings.Contains(none.Line(), "关羽 斩") ||
		!strings.Contains(none.Line(), "none of them carries every word") {
		t.Errorf("scan of an absent term = %+v / %q, want the probes named and the miss explained", none, none.Line())
	}
}

// TestTheScanAsksNothingWhenThereIsNothingToAsk pins the cheap gate: no terms, no request. The terms
// come from the plan's own declaration (runtime.DeclaredProbes), so a question that declared no act
// words never pays for a scan.
func TestTheScanAsksNothingWhenThereIsNothingToAsk(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"chunk_id": "c1", "content": "云长手起刀落，斩华雄于马下。"}}}
	deps, _ := newTestSearchDeps(r)
	if res := ScanMatchAny(context.Background(), deps, []string{"  ", ""}, nil, nil, 0); res.Matched != 0 {
		t.Errorf("scan with no usable term matched %d, want nothing", res.Matched)
	}
	if len(r.requests) != 0 {
		t.Errorf("scan with no usable term made %d request(s), want none", len(r.requests))
	}
	// A plan that declares an entity yields that entity's spellings, and those DO reach the corpus.
	table := State{State: []Variable{{ID: 0, Type: "entity", Terms: []string{"斩", "砍"}, Subjects: []string{"关羽", "云长"}}}}
	terms := DeclaredProbes(table)
	// The probes are the ENTITY's spellings and nothing else. The act words are gone: which verb a source
	// uses for a deed is a fact about the source, and a probe built from a guessed verb reaches the
	// passages phrased the way the guess predicted and misses the rest (see DeclaredProbes).
	want := []string{"关羽", "云长"}
	if !reflect.DeepEqual(terms, want) {
		t.Fatalf("DeclaredProbes = %v, want %v (the declared spellings)", terms, want)
	}
	if res := ScanMatchAny(context.Background(), deps, terms, nil, nil, 0); res.Matched == 0 {
		t.Error("the declared probes matched nothing against a passage carrying the actor and the act word")
	}
}

// TestTheScanAsksBM25OncePerDeclaredProbe pins how the scan widens its recall under this project's
// rule that the engine offers BM25 and nothing else: it ASKS MORE, one keyword call per declared probe,
// and does every bit of pattern matching locally over what came back.
//
// The joined query is the alternative, and it dilutes: measured 2026-09-20, `关羽 劈` ranked 管亥 3rd on
// its own and 15th as part of a batch, and a batch's head is what the union would keep.
func TestTheScanAsksBM25OncePerDeclaredProbe(t *testing.T) {
	terms := []string{"关羽 斩", "云长 斩", "关羽 砍", "云长 砍"}
	// Each probe's own head carries the passage only THAT probe reaches: a vocabulary the source used
	// for one of the four spellings, and nothing else.
	var chunks []map[string]any
	for i, q := range terms {
		chunks = append(chunks, map[string]any{
			"chunk_id": fmt.Sprintf("c-%d", i),
			"doc_id":   "d1",
			"content":  q + "，其头落地。",
		})
	}
	r := &stubRetriever{chunks: chunks}
	deps, _ := newTestSearchDeps(r)
	res := ScanMatchAny(context.Background(), deps, terms, nil, nil, 0)

	if len(r.requests) != len(terms) {
		t.Fatalf("requests = %d, want one BM25 call per declared probe (%d)", len(r.requests), len(terms))
	}
	for i, req := range r.requests {
		if req.Query != terms[i] {
			t.Errorf("request %d queried %q, want the probe %q on its own (a batch dilutes the rare word)",
				i, req.Query, terms[i])
		}
		if !keywordOnlyLeg(req) {
			t.Errorf("request %d used the dense leg: the scan must not need an embedder", i)
		}
	}
	// The matching is OURS: all four passages carry their probe's words, so all four are hits — the
	// engine was never asked to filter anything.
	if res.Matched != len(terms) {
		t.Errorf("matched = %d, want all %d probes' passages (the match happens here, not in the engine)",
			res.Matched, len(terms))
	}
}

// TestTheScanFallsBackToTheActWordsWhenTheSpellingDiffers pins the one relaxation, and its label.
//
// The actor's spelling is the fragile half of a probe: the source writes 关公 where the plan wrote
// 关羽, and then NO passage carries "both words" — which is how a scan over a corpus full of the deed
// reports nothing (measured 2026-09-21: `[scan] no window in the recalled set carries any of the
// declared probe term(s)` on a question whose passages were in the pool, and the "fix" in the log was
// a bare miss nobody could act on). The act words still land on the deed, so those windows are
// delivered and LABELLED rather than dropped: what a window means stays the answer's judgement.
func TestTheScanFallsBackToTheActWordsWhenTheSpellingDiffers(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "doc_id": "d1", "content": "关公直取管亥，手起一刀，劈于马下。"},
		{"chunk_id": "c2", "doc_id": "d1", "content": "云长手起刀落，斩孔秀于马下。"},
	}}
	deps, _ := newTestSearchDeps(r)
	res := ScanMatchAny(context.Background(), deps, []string{"关羽 劈", "关羽 斩"}, nil, nil, 0)
	if !res.ActOnly {
		t.Fatalf("ActOnly = false; the strict pass matched nothing and the act words must carry the fallback (result %+v)", res)
	}
	if res.Matched != 2 || !strings.Contains(res.Line(), "act-words only") {
		t.Errorf("fallback matched %d window(s), line %q: want both act-word windows, labelled", res.Matched, res.Line())
	}
	// With ONE spelling declared there is nothing to fall back TO beyond that probe's own words, and the
	// line still names what was probed.
	single := ScanMatchAny(context.Background(), deps, []string{"关羽"}, nil, nil, 0)
	if single.Matched != 0 || !strings.Contains(single.Line(), "关羽") {
		t.Errorf("single-word probe = %+v / %q, want an explained miss", single, single.Line())
	}
}

// TestTheScanWindowsReachTheSessionAsMaterial pins the delivery half of the scan: the windows it matched
// are RENDERED into the seed, bounded, each with the handle of the passage it came from — and what is
// not rendered is said, not silently dropped.
//
// Measured 2026-09-21: the scan matched and delivered 99 windows into the pool while the answer listed
// twelve of the sixteen members — a window the session never sees is a window no answer can enumerate,
// and the pool is not a prompt.
func TestTheScanWindowsReachTheSessionAsMaterial(t *testing.T) {
	kb := &Kbinfos{}
	var windows []ScanWindow
	for i := 0; i < 5; i++ {
		windows = append(windows, ScanWindow{
			ChunkID: fmt.Sprintf("c%d", i),
			DocID:   "d1",
			Text:    fmt.Sprintf("%d 云长手起刀落，斩之于马下。", i),
		})
	}
	kb.NoteScanWindows(windows)

	// max <= 0 is what production passes: EVERY delivered window, so the model enumerates from the whole
	// material instead of from a sample of it.
	block := renderScanWindows(kb, 0)
	for _, want := range []string{"SCAN WINDOWS", "[ID:0]", "[ID:4]", "doc d1"} {
		if !strings.Contains(block, want) {
			t.Errorf("scan block %q missing %q", block, want)
		}
	}
	if strings.Contains(block, "more window(s)") {
		t.Errorf("the seed cut the material it was given:\n%s", block)
	}
	// Shown windows are PUBLISHED: they are the handles the answer cites.
	if want := []string{"c0", "c1", "c2", "c3", "c4"}; !reflect.DeepEqual(kb.SessionEvidenceRefs, want) {
		t.Errorf("published = %v, want the shown windows in order (%v)", kb.SessionEvidenceRefs, want)
	}
	// A caller that asks for a line cap still gets one, and what it left out is said.
	capped := renderScanWindows(kb, 3)
	if !strings.Contains(capped, "[ID:2]") || strings.Contains(capped, "[ID:3]") ||
		!strings.Contains(capped, "…and 2 more window(s)") {
		t.Errorf("capped block = %q, want three lines and the count of what is left", capped)
	}
	// An identical window is shown once: overlapping windows repeat a sentence, and a repeat is not a
	// second member.
	dup := &Kbinfos{}
	dup.NoteScanWindows([]ScanWindow{
		{ChunkID: "c-a", DocID: "d1", Text: "云长手起刀落，斩之于马下。"},
		{ChunkID: "c-b", DocID: "d1", Text: "云长手起刀落，斩之于马下。"},
	})
	if n := strings.Count(renderScanWindows(dup, 0), "斩之于马下"); n != 1 {
		t.Errorf("the block shows the same window %d time(s), want once", n)
	}
	// Nothing scanned, nothing rendered — and no empty heading either.
	if got := renderScanWindows(&Kbinfos{}, 0); got != "" {
		t.Errorf("renderScanWindows on a run with no scan = %q, want nothing", got)
	}
}

// TestTheDeclaredActWordsFilterTheEntitysPassages pins what makes the material the QUESTION's rather than
// the entity's.
//
// Asking for the entity alone returns everything that mentions it — measured 2026-09-21 (三国/关羽): 249
// windows, an answer whose evidence list ran 249 lines long (33824 characters, of which the prose was
// about 2000), and a reader complaining about exactly that. A question is not its entity: "which named
// people did he kill" is about the entity AND the deed, and the deed's words are the PLAN's declaration
// (DeclaredActWords), not a guess any code makes.
func TestTheDeclaredActWordsFilterTheEntitysPassages(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c-deed", "doc_id": "d1", "content": "云长手起刀落，砍荀正于马下。"},
		{"chunk_id": "c-talk", "doc_id": "d1", "content": "关公曰：此马甚骏，日行千里。"},
	}}
	deps, _ := newTestSearchDeps(r)

	// The entity alone: both passages are material.
	all := ScanMatchAny(context.Background(), deps, []string{"关羽", "云长", "关公"}, nil, nil, 0)
	if all.Matched != 2 {
		t.Fatalf("without act words the scan matched %d passage(s), want both of the entity's", all.Matched)
	}
	// The plan's act words: only the one that states the deed.
	deed := ScanMatchAny(context.Background(), deps, []string{"关羽", "云长", "关公"}, []string{"砍", "斩"}, nil, 0)
	if deed.Matched != 1 || len(deed.Windows) != 1 || deed.Windows[0].ChunkID != "c-deed" {
		t.Fatalf("matched = %d, windows = %v, want only the passage carrying the entity AND a declared act word",
			deed.Matched, deed.Windows)
	}
	// The line says what the material was filtered by, so the log shows the material's scope.
	if !strings.Contains(deed.Line(), "砍") || !strings.Contains(deed.Line(), "declared act words") {
		t.Errorf("scan line = %q, want it to name the filter", deed.Line())
	}
	// An act word with no entity is not this actor's deed, and it is never material.
	r2 := &stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c-other", "doc_id": "d2", "content": "交马一合，砍荀正于马下。"},
	}}
	deps2, _ := newTestSearchDeps(r2)
	if got := ScanMatchAny(context.Background(), deps2, []string{"关羽", "云长"}, []string{"砍"}, nil, 0); got.Matched != 0 {
		t.Errorf("matched = %d, want none: the entity is half of the material and is not optional", got.Matched)
	}

	// The plan's declaration is read as declared: one entry per act word, for every slot that names an
	// actor, and a slot with no actor declares none.
	table := State{State: []Variable{
		{ID: 0, Type: "entity", Subjects: []string{"关羽", "关公"}, Terms: []string{"斩", "杀"}},
		{ID: 1, Type: "count", Terms: []string{"十六"}},
	}}
	if got, want := DeclaredActWords(table), []string{"斩", "杀"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DeclaredActWords = %v, want %v", got, want)
	}
}
