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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/entity"
)

// stubRetriever returns a fixed result and records the requests it received.
type stubRetriever struct {
	chunks   []map[string]any
	err      error
	requests []RetrieveRequest
}

func (s *stubRetriever) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.chunks, nil
}

func newTestSearchDeps(r Retriever) (SearchDeps, *Kbinfos) {
	kb := &Kbinfos{}
	return SearchDeps{
		Backend:  r,
		KbIDs:    []string{"kb1"},
		TenantID: "tenant1",
		KB:       kb,
	}, kb
}

func TestHybridSearchBailsWithoutDatasetsOrBackend(t *testing.T) {
	// No bound datasets -> no search at all (empty kbinfos).
	got, aggs := HybridSearch(context.Background(), SearchDeps{
		Backend: &stubRetriever{chunks: []map[string]any{{"content": "x"}}},
	}, SearchParams{Question: "q"})
	if got != nil || aggs != nil {
		t.Error("no datasets must short-circuit to empty")
	}
	// No backend -> empty.
	deps, _ := newTestSearchDeps(nil)
	if got, _ := HybridSearch(context.Background(), deps, SearchParams{Question: "q"}); got != nil {
		t.Error("no backend must return empty")
	}
}

func TestHybridSearchBuildsEffectiveQuery(t *testing.T) {
	// Weighted query wins, and the result is capped at 400 chars.
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	long := strings.Repeat("weighted ", 100)
	HybridSearch(context.Background(), deps, SearchParams{
		Question: "who made it?", RetrievalQuery: long,
	})
	if len(r.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r.requests))
	}
	q := r.requests[0].Query
	if !strings.HasPrefix(q, "who made it? weighted") {
		t.Errorf("query = %q, want question + weighted query", q)
	}
	if len(q) != maxEffectiveQueryChars {
		t.Errorf("query length = %d, want capped at %d", len(q), maxEffectiveQueryChars)
	}

	// No weighted query -> keywords fall into the query text.
	r = &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ = newTestSearchDeps(r)
	HybridSearch(context.Background(), deps, SearchParams{Question: "who made it?", Keywords: "culdcept"})
	if r.requests[0].Query != "who made it? culdcept" {
		t.Errorf("query = %q, want keywords appended", r.requests[0].Query)
	}

	// Neither -> the bare question.
	r = &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ = newTestSearchDeps(r)
	HybridSearch(context.Background(), deps, SearchParams{Question: "who made it?"})
	if r.requests[0].Query != "who made it?" {
		t.Errorf("query = %q, want the bare question", r.requests[0].Query)
	}
}

// TestHybridSearchEffectiveQueryCapsCodePoints pins the expanded-query cap to code points,
// not bytes: a byte slice both splits a multi-byte rune — handing the retriever invalid
// UTF-8 — and caps a CJK query at ~133 characters, dropping expansion terms the fan-out
// leg weighs on. The ASCII case above cannot tell the two apart (bytes == runes there).
func TestHybridSearchEffectiveQueryCapsCodePoints(t *testing.T) {
	// "who made it" + " " is 12 bytes, so byte 400 lands one byte inside a CJK
	// rune (400-12 = 388 = 3*129 + 1): the byte slice is not even valid UTF-8.
	expansion := strings.Repeat("知识", 300) // 600 runes, 1800 bytes
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	HybridSearch(context.Background(), deps, SearchParams{
		Question: "who made it", RetrievalQuery: expansion,
	})

	if len(r.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r.requests))
	}
	q := r.requests[0].Query
	if !utf8.ValidString(q) {
		t.Fatalf("query is not valid UTF-8 — a byte slice cut a rune in half: %q", q)
	}
	if got := utf8.RuneCountInString(q); got != maxEffectiveQueryChars {
		t.Errorf("query runes = %d, want the cap at %d code points (%d bytes)", got, maxEffectiveQueryChars, len(q))
	}
	full := "who made it " + expansion
	if want := string([]rune(full)[:maxEffectiveQueryChars]); q != want {
		t.Errorf("query = %q, want the first %d code points %q", q, maxEffectiveQueryChars, want)
	}
	// 400 code points of mostly-CJK text is far more than 400 bytes: the cap is a
	// character budget, not a byte budget.
	if len(q) <= maxEffectiveQueryChars {
		t.Errorf("query bytes = %d, want > %d (the cap counts code points)", len(q), maxEffectiveQueryChars)
	}
}

func TestRankFeatureOnlyOnRetrieveLeg(t *testing.T) {
	// rank_feature is passed ONLY by the retrieve channel; the three search legs call the
	// retriever WITHOUT it (the grep leg delegates to bm25), so those requests must stay
	// nil even with a Tagger.
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	deps.KBs = []*entity.Knowledgebase{{}}
	deps.Tagger = stubTagger{t: t}

	HybridSearch(context.Background(), deps, SearchParams{Question: "who made it?"})
	VectorSearch(context.Background(), deps, SearchParams{Question: "who made it?"})
	BM25Search(context.Background(), deps, SearchParams{Question: "who made it?"})
	GrepSearch(context.Background(), deps, SearchParams{Question: "who made it?"})

	want := map[string]float64{"definition": 1.0, "entity": 1.0}
	for i, req := range r.requests {
		if req.RankFeature != nil {
			t.Errorf("request %d (%s) RankFeature = %v, want nil (search.py legs never pass rank_feature)", i, req.Query, req.RankFeature)
		}
	}

	// RAGTools.retrieve is the only leg that carries the label_question boost.
	r2 := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps2, _ := newTestSearchDeps(r2)
	deps2.KBs = []*entity.Knowledgebase{{}}
	deps2.Tagger = stubTagger{t: t}
	deps2.UsingEmbedding = true
	RetrieveSearch(context.Background(), deps2, SearchParams{Question: "who made it?"})
	if len(r2.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r2.requests))
	}
	if got := r2.requests[0].RankFeature; !reflect.DeepEqual(got, want) {
		t.Errorf("RetrieveSearch RankFeature = %v, want %v", got, want)
	}

	// Nil Tagger -> empty rank feature even on the retrieve leg.
	r3 := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps3, _ := newTestSearchDeps(r3)
	deps3.UsingEmbedding = true
	RetrieveSearch(context.Background(), deps3, SearchParams{Question: "unique query no rf"})
	if len(r3.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r3.requests))
	}
	if len(r3.requests[0].RankFeature) != 0 {
		t.Errorf("RankFeature = %v, want empty for nil Tagger", r3.requests[0].RankFeature)
	}
}

// stubTagger implements QuestionLabeler for TestHybridSearchPassesRankFeature.
type stubTagger struct{ t *testing.T }

func (s stubTagger) LabelQuestion(_ context.Context, question string, kbs []*entity.Knowledgebase) map[string]float64 {
	if question != "who made it?" {
		s.t.Errorf("LabelQuestion called with question=%q", question)
	}
	if len(kbs) != 1 {
		s.t.Errorf("LabelQuestion called with %d kbs, want 1", len(kbs))
	}
	return map[string]float64{"definition": 1.0, "entity": 1.0}
}

func TestHybridSearchCachesIdenticalQuery(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	p := SearchParams{Question: "same question"}
	HybridSearch(context.Background(), deps, p)
	HybridSearch(context.Background(), deps, p)
	if len(r.requests) != 1 {
		t.Errorf("backend calls = %d, want 1 (second served from cache)", len(r.requests))
	}
	// A different scope is a different retrieval.
	HybridSearch(context.Background(), deps, SearchParams{Question: "same question", DocScope: []string{"doc9"}})
	if len(r.requests) != 2 {
		t.Errorf("backend calls = %d, want 2 (doc_scope is part of the key)", len(r.requests))
	}
	// A different TopN is a different retrieval.
	HybridSearch(context.Background(), deps, SearchParams{Question: "same question", TopN: 5})
	if len(r.requests) != 3 {
		t.Errorf("backend calls = %d, want 3 (top_n is part of the key)", len(r.requests))
	}
}

func TestSearchCacheKeyIgnoresOrderAndWhitespace(t *testing.T) {
	a := SearchCacheKey("  Who   Made  It ", []string{"b", "a"}, 12, []string{"d2", "d1"})
	b := SearchCacheKey("who made it", []string{"a", "b"}, 12, []string{"d1", "d2"})
	if a != b {
		t.Errorf("cache key not canonical:\n%q\n%q", a, b)
	}
}

func TestHybridSearchStoresMemoryBeforeNarrowing(t *testing.T) {
	// The raw chunk must reach memory even when narrowing rewrites the copy
	// handed downstream.
	r := &stubRetriever{chunks: []map[string]any{{
		"chunk_id": "c1",
		"content":  "Alpha sentence. Culdcept was made by OmiyaSoft in 1999. Omega sentence.",
		"doc_id":   "d1",
	}}}
	deps, kb := newTestSearchDeps(r)
	HybridSearch(context.Background(), deps, SearchParams{
		Question: "who made Culdcept?", Keywords: "culdcept",
	})
	if MemorySize(kb) != 1 {
		t.Fatalf("memory size = %d, want 1", MemorySize(kb))
	}
	if strings.Contains(ChunkTextOf(kb.Memory[0]), "*Culdcept*") {
		t.Error("memory must hold the RAW chunk, not the narrowed/highlighted copy")
	}
}

func TestHybridSearchReturnsEmptyOnBackendError(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{err: errors.New("ES down")})
	got, aggs := HybridSearch(context.Background(), deps, SearchParams{Question: "q"})
	if len(got) != 0 || len(aggs) != 0 {
		t.Errorf("a backend failure must yield empty results, got %d chunks", len(got))
	}
}

// Narrowing

func TestNarrowOrKeepIsAllOrNothing(t *testing.T) {
	var lines []string
	ctx := WithSteps(context.Background(), StepReporter{
		Text: func(line string) { lines = append(lines, line) },
	})
	var logged bytes.Buffer
	chunks := []map[string]any{
		{"content": "Culdcept was made by OmiyaSoft."},
		{"content": "It was released in 1999."},
	}
	// Keywords hit -> narrowed subset (the non-matching chunk is dropped).
	got := NarrowOrKeep(ctx, chunks, "culdcept", "test", log.New(&logged, "", 0))
	if len(got) != 1 || !strings.Contains(ChunkTextOf(got[0]), "Culdcept") {
		t.Errorf("narrowed = %v, want only the matching chunk", got)
	}
	// How the filter resized the pool is a developer's diagnostic: it goes to the
	// log with Python's own wording (text_processing.py:464) and NOT to the think
	// block, where the leg's result line already reports what came back.
	if !strings.Contains(logged.String(), "[test] Kept 1 of 2 passage(s) that actually mention the keywords.") {
		t.Errorf("narrowed log line = %q", logged.String())
	}
	if len(lines) != 0 {
		t.Errorf("narrowing must not reach the think block: %q", strings.Join(lines, "\n"))
	}

	// No keyword overlap -> ALL chunks kept (this is the whole point: the
	// retriever already ranked them, and dropping everything produced empty
	// results and unverified claims).
	logged.Reset()
	got = NarrowOrKeep(ctx, chunks, "zzz-no-match", "test", log.New(&logged, "", 0))
	if len(got) != len(chunks) {
		t.Errorf("no-match kept %d, want all %d", len(got), len(chunks))
	}
	if !strings.Contains(logged.String(),
		"[test] Keyword narrowing matched nothing — keeping all 2 retrieved passage(s).") {
		t.Errorf("no-match log line = %q", logged.String())
	}
	if len(lines) != 0 {
		t.Errorf("the no-match line must not reach the think block either: %q", strings.Join(lines, "\n"))
	}

	// Empty keywords -> untouched, and silent: nothing was narrowed, so there is
	// nothing to report.
	logged.Reset()
	if got := NarrowOrKeep(ctx, chunks, "", "test", log.New(&logged, "", 0)); len(got) != len(chunks) {
		t.Error("empty keywords must return chunks unchanged")
	}
	if logged.Len() != 0 || len(lines) != 0 {
		t.Errorf("empty keywords must report nothing; log = %q, think = %v", logged.String(), lines)
	}
}

func TestNarrowContentKeepsNeighboursAndHighlights(t *testing.T) {
	content := "Alpha filler. Culdcept was made by OmiyaSoft. Omega filler."
	got, ok := NarrowContent(content, []string{"culdcept"})
	if !ok {
		t.Fatal("NarrowContent must match")
	}
	if !strings.Contains(got, "*Culdcept*") {
		t.Errorf("keyword not highlighted: %q", got)
	}
	// +/-1 neighbour window: Alpha and Omega should be present.
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "Omega") {
		t.Errorf("neighbour sentences missing: %q", got)
	}
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...") {
		t.Errorf("narrowed text must be wrapped in ellipses: %q", got)
	}
	if _, ok := NarrowContent("nothing relevant here", []string{"culdcept"}); ok {
		t.Error("no match must return false")
	}
}

func TestSplitKeywordsFallsBackToBigrams(t *testing.T) {
	// >=3 comma terms -> used as-is, lower-cased.
	got := SplitKeywords("Alpha, Beta, Gamma")
	if len(got) != 3 || got[0] != "alpha" {
		t.Errorf("comma split = %v", got)
	}
	// <3 terms -> bigrams are more discriminative than single words.
	got = SplitKeywords("finale run time")
	if len(got) != 2 || got[0] != "finale run" || got[1] != "run time" {
		t.Errorf("bigram fallback = %v, want [finale run, run time]", got)
	}
	if SplitKeywords("   ") != nil {
		t.Error("blank keywords must yield no terms")
	}
}

func TestHighlightKeywordsPrefersLongestTerm(t *testing.T) {
	// "new york" must win over "york" so the shorter term cannot split it.
	got := HighlightKeywords("welcome to New York city", []string{"york", "new york"})
	// The star marker, and ONE span for the multi-word entity — never "*New* *York*".
	if !strings.Contains(got, "*New York*") {
		t.Errorf("highlight = %q, want the longest term applied", got)
	}
	if strings.Contains(got, "*york*") {
		t.Errorf("shorter term must not win: %q", got)
	}
}

// TestHighlightKeywordsFoldsWithoutByteOffsets pins the matching to rune space.
// `strings.ToLower` is not byte-length-preserving: "İ" (U+0130) is 2 bytes and folds to
// the 1-byte "i" (Go applies the simple 1:1 case mapping, the opposite direction from a
// full Unicode fold, which expands it to 3 bytes). So a byte
// offset taken from the original indexes the folded string at a different
// position: the loop then runs past its end (panic: slice bounds out of range
// [10:9]) or cuts a rune in half (invalid UTF-8). Corpus text reaches this via
// NarrowContent, e.g. a Turkish document.
func TestHighlightKeywordsFoldsWithoutByteOffsets(t *testing.T) {
	// "İstanbul" is 9 bytes but folds to the 8-byte "istanbul", so the old byte
	// loop wrote text[0:8] — one byte short, ending mid-word ("İstanbu").
	if got := HighlightKeywords("İstanbul is here", []string{"istanbul"}); got != "*İstanbul* is here" {
		t.Errorf("highlight = %q, want the whole word wrapped", got)
	}

	// Two shrunken runes push the byte index one past the end of the folded
	// string while the original still has a byte left: the old loop panicked.
	got := HighlightKeywords("İİstanbul", []string{"istanbul"})
	if !utf8.ValidString(got) {
		t.Fatalf("highlight is not valid UTF-8 — a span split a rune: %q", got)
	}
	if !strings.Contains(got, "*İstanbul*") {
		t.Errorf("highlight = %q, want the fold-matched runes wrapped in place", got)
	}
}

// TestHighlightKeywordsFoldsUppercaseKeywords pins that a keyword's OWN casing is
// folded the way the haystack is: terms are matched against the lowercased text (`lows`),
// and the phrase list is built with a strip+lower pass plus case-insensitive matching. A
// caller-supplied "Rocket"
// used to be compared verbatim, so a capitalised keyword never matched and the
// span was silently left unstarred.
func TestHighlightKeywordsFoldsUppercaseKeywords(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		kwds []string
		want string
	}{
		{"single word", "the Rocket launched", []string{"Rocket"}, "the *Rocket* launched"},
		{"multi-word phrase stays one span", "welcome to New York city", []string{"New York"}, "welcome to *New York* city"},
		{"source casing is preserved", "ROCKET is loud", []string{"Rocket"}, "*ROCKET* is loud"},
		{"surrounding spaces are trimmed", "the Rocket launched", []string{"  Rocket  "}, "the *Rocket* launched"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HighlightKeywords(tc.text, tc.kwds); got != tc.want {
				t.Errorf("HighlightKeywords(%q, %v) = %q, want %q", tc.text, tc.kwds, got, tc.want)
			}
		})
	}
}

// TestHighlightKeywordsKeepsPhrasePartsWhole pins the phrase guard: a stem-matched word
// that already occurs inside a keyword phrase is NOT added as a term of its own, so a
// standalone "Braves" is
// left alone and only the "Atlanta Braves" span is starred.
func TestHighlightKeywordsKeepsPhrasePartsWhole(t *testing.T) {
	got := HighlightKeywords("Braves lost. Atlanta Braves won.", []string{"Atlanta Braves"})
	if want := "Braves lost. *Atlanta Braves* won."; got != want {
		t.Errorf("highlight = %q, want %q", got, want)
	}
}

// TestHighlightKeywordsStemMatchesCapitalisedWords pins the word scan: every `[A-Za-z]+`
// word is stemmed, so a capitalised inflected word still contributes its stem term. The
// shared lowercase pattern matched only the
// fragment after the capital ("Nominated" -> "ominated"), so nothing was starred.
func TestHighlightKeywordsStemMatchesCapitalisedWords(t *testing.T) {
	got := HighlightKeywords("Nominated twice.", []string{"nominations"})
	if want := "*Nominated* twice."; got != want {
		t.Errorf("highlight = %q, want %q", got, want)
	}
}

// Doc aggregations

func TestDocAggsGroupsByDocument(t *testing.T) {
	chunks := []map[string]any{
		{"doc_id": "d1", "docnm_kwd": "Doc One", "content": "aaaa"},
		{"doc_id": "d1", "docnm_kwd": "Doc One", "content": "bb"},
		{"doc_id": "d2", "docnm_kwd": "Doc Two", "content": "c"},
	}
	aggs := DocAggs(chunks)
	if len(aggs) != 2 {
		t.Fatalf("aggs = %d, want 2 documents", len(aggs))
	}
	// Order follows first appearance, so the log line is stable.
	if aggs[0]["doc_id"] != "d1" || aggs[0]["count"] != 2 {
		t.Errorf("d1 agg = %v, want count 2", aggs[0])
	}
	if aggs[0]["char_count"] != 6 {
		t.Errorf("d1 char_count = %v, want 6 (4+2)", aggs[0]["char_count"])
	}
	if aggs[1]["doc_id"] != "d2" || aggs[1]["count"] != 1 {
		t.Errorf("d2 agg = %v", aggs[1])
	}
	if DocAggs(nil) != nil {
		t.Error("no chunks must yield no aggregations")
	}
}

func TestHybridSearchAggregatesAndRespectsScope(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"doc_id": "d1", "content": "a"},
		{"doc_id": "d2", "content": "b"},
	}}
	deps, _ := newTestSearchDeps(r)
	chunks, aggs := HybridSearch(context.Background(), deps, SearchParams{
		Question: "q", DocScope: []string{"d1"},
	})
	if len(chunks) != 2 || len(aggs) != 2 {
		t.Fatalf("chunks=%d aggs=%d", len(chunks), len(aggs))
	}
	// doc_scope must reach the backend.
	if len(r.requests[0].DocScope) != 1 || r.requests[0].DocScope[0] != "d1" {
		t.Errorf("doc_scope not forwarded: %v", r.requests[0].DocScope)
	}
	// Explicit kb_ids override the session default.
	r.requests = nil
	HybridSearch(context.Background(), deps, SearchParams{Question: "q", KbIDs: []string{"kb9"}})
	if len(r.requests[0].DatasetIDs) != 1 || r.requests[0].DatasetIDs[0] != "kb9" {
		t.Errorf("kb_ids not honoured: %v", r.requests[0].DatasetIDs)
	}
}

func TestHybridSearchAggsCoverFullRetrievedBeforeNarrowing(t *testing.T) {
	// doc_aggs must reflect the FULL pre-narrow retrieved set: the aggregations come
	// straight from the retriever and narrowing only replaces the chunk list. Here the
	// keyword narrows the chunks to doc d1,
	// but the aggregation still counts both retrieved documents.
	r := &stubRetriever{chunks: []map[string]any{
		{"doc_id": "d1", "docnm_kwd": "one", "content": "needle buried here", "id": "c1"},
		{"doc_id": "d2", "docnm_kwd": "two", "content": "nothing relevant", "id": "c2"},
	}}
	deps, _ := newTestSearchDeps(r)
	chunks, aggs := HybridSearch(context.Background(), deps, SearchParams{
		Question: "q", Keywords: "needle",
	})
	if len(chunks) != 1 {
		t.Fatalf("narrowed chunks = %d, want 1 (d1 only)", len(chunks))
	}
	if len(aggs) != 2 {
		t.Fatalf("aggs = %d, want 2 (full pre-narrow set: d1 and d2)", len(aggs))
	}
}

func TestHybridSearchInvokesCompiledExpansion(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	exp := &stubExpander{}
	deps.Expand = exp
	HybridSearch(context.Background(), deps, SearchParams{Question: "q", UseCompiled: true})
	if exp.calls != 1 {
		t.Errorf("compiled expansion calls = %d, want 1", exp.calls)
	}
	// Without UseCompiled it must not run.
	exp.calls = 0
	HybridSearch(context.Background(), deps, SearchParams{Question: "other", UseCompiled: false})
	if exp.calls != 0 {
		t.Error("compiled expansion must be opt-in")
	}
}

type stubExpander struct{ calls int }

func (s *stubExpander) Expand(_ context.Context, _ *Kbinfos, _, _ string, _ []string) error {
	s.calls++
	return nil
}

// lastReq returns the most recent RetrieveRequest the stub recorded, failing if
// none was issued.
func (r *stubRetriever) lastReq(t *testing.T) RetrieveRequest {
	t.Helper()
	if len(r.requests) == 0 {
		t.Fatal("expected a retrieve request, got none")
	}
	return r.requests[len(r.requests)-1]
}

// ptrFloat dereferences a control pointer. nil means "the caller supplied
// nothing, so the retriever keeps its own default"; the search-leg tests below
// assert the opposite (an explicit value is passed), so nil is a failure here.
func ptrFloat(t *testing.T, p *float64) float64 {
	t.Helper()
	if p == nil {
		t.Fatal("control pointer = nil, want an explicit value")
	}
	return *p
}

// TestVectorSearchBailsWithoutEmbedder: with no embedder configured it returns nothing,
// and must NOT even hit the backend.
func TestVectorSearchBailsWithoutEmbedder(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	if got, _ := VectorSearch(context.Background(), deps, SearchParams{Question: "q"}); got != nil {
		t.Error("vector search without embedder must be empty")
	}
	if len(r.requests) != 0 {
		t.Errorf("vector search without embedder must not call the backend, got %d calls", len(r.requests))
	}
}

// TestVectorSearchUsesZeroKeywordWeight mirrors Python vector_search: the
// pure-vector leg carries keyword weight 0 and excludes compiled rows.
func TestVectorSearchUsesZeroKeywordWeight(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.HasEmbedder = true
	VectorSearch(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.KeywordsSimilarityWeight); got != 0 {
		t.Errorf("vector search keyword weight = %v, want 0", got)
	}
	if !req.ExcludeCompiled {
		t.Error("vector search must exclude compiled rows")
	}
}

// TestBM25SearchUsesFullKeywordWeight mirrors Python bm25_search: keyword-only,
// keyword weight 1, threshold 0.0, excludes compiled rows.
func TestBM25SearchUsesFullKeywordWeight(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	BM25Search(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.KeywordsSimilarityWeight); got != 1 {
		t.Errorf("bm25 search keyword weight = %v, want 1", got)
	}
	if got := ptrFloat(t, req.SimilarityThreshold); got != 0 {
		t.Errorf("bm25 search threshold = %v, want 0", got)
	}
	if !req.ExcludeCompiled {
		t.Error("bm25 search must exclude compiled rows")
	}
}

// TestGrepSearchDelegatesToBM25 mirrors Python grep_search: it is bm25_search
// with a keyword-only (weight 1) leg and compiled-row exclusion.
func TestGrepSearchDelegatesToBM25(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	GrepSearch(context.Background(), deps, SearchParams{Question: "q", Keywords: "kw"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.KeywordsSimilarityWeight); got != 1 {
		t.Errorf("grep search keyword weight = %v, want 1", got)
	}
	if !req.ExcludeCompiled {
		t.Error("grep search must exclude compiled rows")
	}
}

// TestGrepSearchSearchingLineSplitsAudiences pins the split every leg now makes:
// the think block says what the leg DOES ("Searching for the exact words …", in the
// family the other legs use, without the phrase every leg shares — "the knowledge
// base" — that separates nothing a reader can act on), while the log keeps Python's
// own phrasing ("Keyword-first locate for …", search.py:418), so a log diff against
// Python still lines up.
func TestGrepSearchSearchingLineSplitsAudiences(t *testing.T) {
	var logged bytes.Buffer
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "x"}}})
	deps.Logger = log.New(&logged, "", 0)
	var text []string
	ctx := WithSteps(context.Background(), StepReporter{Text: func(line string) { text = append(text, line) }})

	GrepSearch(ctx, deps, SearchParams{Question: "曹操是谁"})

	think := strings.Join(text, "")
	if !strings.Contains(think, `[Grep search] Searching for the exact words "曹操是谁".`) {
		t.Errorf("think text = %q, want the reader's sentence", think)
	}
	if strings.Contains(think, "Keyword-first") {
		t.Errorf("the implementation phrasing must not reach the think block: %q", think)
	}
	if !strings.Contains(logged.String(), `[Grep search] Keyword-first locate for "曹操是谁"`) {
		t.Errorf("log = %q, want Python's line kept verbatim", logged.String())
	}
}

// TestSearchLegsShareOneThinkSentenceFamily pins the wording of all five legs'
// "searching" step in ONE place. They land in a single think block, so they have to
// scan as one family ("Searching … for \"q\".") while still saying which leg
// searched — the "[Hybrid search]" prefix is a developer's label, so the difference
// has to be IN the sentence.
//
// "the knowledge base" is deliberately absent from every one of them: all five legs
// search it, so the phrase distinguishes nothing a reader can act on. The LOG side
// keeps Python's own wording (including that phrase where Python has it) — pinned by
// TestGrepSearchSearchingLineSplitsAudiences and the log-diff tests, which is why it
// is not repeated here.
func TestSearchLegsShareOneThinkSentenceFamily(t *testing.T) {
	cases := []struct {
		name string
		run  func(ctx context.Context, deps SearchDeps)
		want string
	}{
		{"hybrid", func(ctx context.Context, d SearchDeps) {
			HybridSearch(ctx, d, SearchParams{Question: "q"})
		},
			`[Hybrid search] Searching by meaning and keyword for "q".`},
		{"vector", func(ctx context.Context, d SearchDeps) {
			VectorSearch(ctx, d, SearchParams{Question: "q"})
		},
			`[Vector search] Searching by meaning for "q".`},
		{"bm25", func(ctx context.Context, d SearchDeps) {
			BM25Search(ctx, d, SearchParams{Question: "q"})
		},
			`[BM25 search] Searching by keyword for "q".`},
		{"retrieve", func(ctx context.Context, d SearchDeps) {
			RetrieveSearch(ctx, d, SearchParams{Question: "q"})
		},
			`[Retrieve] Searching for "q".`},
		{"grep", func(ctx context.Context, d SearchDeps) {
			GrepSearch(ctx, d, SearchParams{Question: "q"})
		},
			`[Grep search] Searching for the exact words "q".`},
	}
	for _, tc := range cases {
		deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "x"}}})
		// VectorSearch is gated on a configured embedder (Python bm25/vector
		// asymmetry): without this it returns before reporting anything.
		deps.HasEmbedder = true
		var text []string
		ctx := WithSteps(context.Background(), StepReporter{Text: func(line string) { text = append(text, line) }})

		tc.run(ctx, deps)

		if got := strings.Join(text, ""); !strings.Contains(got, tc.want) {
			t.Errorf("%s: think = %q, want it to carry %q", tc.name, got, tc.want)
		}
	}
}

// captureLeg runs one search leg and returns what each audience saw: the developer
// log and the think text.
func captureLeg(chunks []map[string]any, run func(ctx context.Context, deps SearchDeps)) (logged, think string) {
	var logBuf bytes.Buffer
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: chunks})
	deps.Logger = log.New(&logBuf, "", 0)
	// VectorSearch is gated on a configured embedder; set for every leg, harmless
	// for the others.
	deps.HasEmbedder = true
	var text []string
	ctx := WithSteps(context.Background(), StepReporter{Text: func(line string) { text = append(text, line) }})

	run(ctx, deps)
	return logBuf.String(), strings.Join(text, "")
}

// TestSearchLegResultLinesReportEveryOutcome pins the second half of a leg's step:
// what it FOUND. Every exit reports one — an empty pool, an all-table pool and a term
// window that matched nothing included — because a leg with no result line cannot be
// told apart from a leg that found nothing, which is the one question the block is
// read for.
//
// The fallbacks count the RETURNED set, not what the term window kept: when grep's
// locate matches nothing it hands back the raw BM25 candidates, and that is what the
// line says.
func TestSearchLegResultLinesReportEveryOutcome(t *testing.T) {
	const q = "曹操生平简介"
	hit := []map[string]any{{"chunk_id": "c1", "doc_id": "d1", "content": "曹操，字孟德。"}}
	// A >=3-row pipe table: IsTableChunk keeps it whole, so grep has no prose to
	// locate in and returns it as-is.
	table := []map[string]any{{"chunk_id": "t1", "doc_id": "d1",
		"content": "a | b | c\nd | e | f\ng | h | i"}}
	proseNoMatch := []map[string]any{{"chunk_id": "p1", "doc_id": "d1",
		"content": "A completely unrelated sentence about weather and clouds."}}

	cases := []struct {
		name      string
		chunks    []map[string]any
		run       func(ctx context.Context, deps SearchDeps)
		wantThink string
		wantLog   string
	}{
		{
			name: "bm25-hit", chunks: hit,
			run:       func(ctx context.Context, d SearchDeps) { BM25Search(ctx, d, SearchParams{Question: q}) },
			wantThink: `[BM25 search] Found 1 passage in 1 document for "` + q + `".`,
			// The log keeps the hybrid/grep shape; on the bm25 leg this line is Go-only.
			wantLog: `[BM25 search] "` + q + `" -> 1 chunk(s): d1:1chunk(`,
		},
		{
			// The leg ran and came back empty: it says so in both projections, and the
			// log line does not dangle a colon.
			name: "bm25-empty", chunks: nil,
			run:       func(ctx context.Context, d SearchDeps) { BM25Search(ctx, d, SearchParams{Question: q}) },
			wantThink: `[BM25 search] Found nothing for "` + q + `".`,
			wantLog:   `[BM25 search] "` + q + `" -> 0 chunk(s)`,
		},
		{
			name: "vector-hit", chunks: hit,
			run:       func(ctx context.Context, d SearchDeps) { VectorSearch(ctx, d, SearchParams{Question: q}) },
			wantThink: `[Vector search] Found 1 passage in 1 document for "` + q + `".`,
			wantLog:   `[Vector search] "` + q + `" -> 1 chunk(s): d1:1chunk(`,
		},
		{
			// grep delegated to bm25 (which reported its own pool) and then found no
			// prose to locate in: the tables ARE the evidence, so the line counts them.
			name: "grep-all-tables", chunks: table,
			run: func(ctx context.Context, d SearchDeps) {
				GrepSearch(ctx, d, SearchParams{Question: "who made Culdcept?"})
			},
			wantThink: `[Grep search] Found 1 passage in 1 document for "who made Culdcept?".`,
			wantLog:   `[Grep search] "who made Culdcept?" -> 1 chunk(s): d1:1chunk(`,
		},
		{
			// The term window matched nothing, so grep returns the raw BM25 candidates
			// — and reports THAT, not "nothing".
			name: "grep-no-match", chunks: proseNoMatch,
			run: func(ctx context.Context, d SearchDeps) {
				GrepSearch(ctx, d, SearchParams{Question: "who made Culdcept?"})
			},
			wantThink: `[Grep search] Found 1 passage in 1 document for "who made Culdcept?".`,
			wantLog:   `[Grep search] "who made Culdcept?" -> 1 chunk(s): d1:1chunk(`,
		},
		{
			// Nothing to locate in and nothing returned: still reported.
			name: "grep-empty-pool", chunks: nil,
			run: func(ctx context.Context, d SearchDeps) {
				GrepSearch(ctx, d, SearchParams{Question: "who made Culdcept?"})
			},
			wantThink: `[Grep search] Found nothing for "who made Culdcept?".`,
			wantLog:   `[Grep search] "who made Culdcept?" -> 0 chunk(s)`,
		},
	}
	for _, tc := range cases {
		logged, think := captureLeg(tc.chunks, tc.run)
		if !strings.Contains(think, tc.wantThink) {
			t.Errorf("%s: think = %q, want it to carry %q", tc.name, think, tc.wantThink)
		}
		if !strings.Contains(logged, tc.wantLog) {
			t.Errorf("%s: log = %q, want it to carry %q", tc.name, logged, tc.wantLog)
		}
		if strings.Contains(logged, "chunk(s): \n") {
			t.Errorf("%s: a zero/empty result must not dangle a colon: %q", tc.name, logged)
		}
	}
}

// TestGrepTermsFromQuery mirrors Python _grep_terms_from_query:
// bare alnum words of length>=2, deduped (order-preserving) and capped at 10.
func TestGrepTermsFromQuery(t *testing.T) {
	// Proper nouns preserved; stopwords/dupes dropped; bare single chars skipped.
	got := GrepTermsFromQuery("Where was Culdcept Saga made? culdcept was made by OmiyaSoft.")
	want := []string{"Where", "was", "Culdcept", "Saga", "made", "by", "OmiyaSoft"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("terms = %v, want %v", got, want)
	}
	// Leading/trailing punctuation trimmed from each token.
	got = GrepTermsFromQuery("apollo.-. 13 mission")
	if !reflect.DeepEqual(got, []string{"apollo", "13", "mission"}) {
		t.Errorf("terms = %v, want [apollo 13 mission]", got)
	}
	// Capped at 10.
	got = GrepTermsFromQuery("a1 a2 a3 a4 a5 a6 a7 a8 a9 a10 a11 a12")
	if len(got) != GrepTermsMax {
		t.Errorf("terms = %d, want capped at %d", len(got), GrepTermsMax)
	}
	// Empty/blank -> nil.
	if GrepTermsFromQuery("") != nil || GrepTermsFromQuery("   ") != nil {
		t.Error("blank query must yield nil terms")
	}
}

// TestGrepSearchNarrowsProseViaTermWindow: two
// narrowing stages: bm25_search narrows by the keywords hint first
// (search.py:grep_search via _narrow_or_keep), then the prose candidates alone are
// narrowed to the term-grep window (search.py:grep_search) while table chunks pass
// through UN-narrowed.
func TestGrepSearchNarrowsProseViaTermWindow(t *testing.T) {
	// A long multi-line prose chunk: only the line carrying the term should be
	// kept (the long unmatched tail is trimmed by the grep window).
	prose := map[string]any{
		"chunk_id": "p1",
		"content": "Alpha filler sentence about unrelated weather and clouds.\n" +
			"Culdcept was made by OmiyaSoft in 1999 after the original board game.\n" +
			strings.Repeat("Omega filler unrelated text about the sea and the sky. ", 20),
	}
	// A table chunk: >=3 pipe rows must NOT be term-narrowed.
	table := map[string]any{
		"chunk_id": "t1",
		"content":  "team | pts | rank\nA | 10 | 1\nB | 8 | 2\nC | 5 | 3\nD | 2 | 4",
	}
	r := &stubRetriever{chunks: []map[string]any{prose, table}}
	deps, _ := newTestSearchDeps(r)
	// The query's own terms become the BM25 keywords hint (there is none here),
	// so the prose only survives the FIRST stage if it carries those terms:
	// "Culdcept was made" yields the bigrams "culdcept was"/"was made", which
	// the prose sentence does. (With "who made Culdcept?" the hint matches the
	// table alone and the prose is dropped outright — pinned by
	// TestGrepSearchDerivesKeywordsHint.)
	chunks, _ := GrepSearch(context.Background(), deps, SearchParams{Question: "Culdcept was made"})

	// Order: table chunks first, then narrowed prose (table + kept prose).
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2 (table + narrowed prose)", len(chunks))
	}
	if chunks[0]["chunk_id"] != "t1" {
		t.Errorf("first chunk = %v, want table chunk t1", chunks[0]["chunk_id"])
	}
	// The table chunk must be returned WHOLE, every row intact: the pipe-table branch wraps
	// the highlighted content, so the body is unchanged and only the wrapper differs from the
	// raw chunk.
	if got := ChunkTextOf(chunks[0]); !strings.Contains(got, "team | pts | rank") || !strings.Contains(got, "D | 2 | 4") {
		t.Errorf("table chunk was truncated; got %q", got)
	}
	// The prose chunk must be narrowed to the Culdcept term window.
	got := ChunkTextOf(chunks[1])
	if !strings.Contains(got, "Culdcept") {
		t.Errorf("prose not narrowed to term: %q", got)
	}
	if strings.Contains(got, "Omega filler") {
		t.Errorf("prose tail should be trimmed by the grep window: %q", got)
	}
	if len(got) >= len(ChunkTextOf(prose)) {
		t.Errorf("prose was not truncated by the grep window: len=%d", len(got))
	}
}

// TestGrepSearchKeepsRawCandidatesWhenNoMatch: when
// the term-grep matches nothing, the raw BM25 candidates are returned unchanged
// so evidence is never dropped.
func TestGrepSearchKeepsRawCandidatesWhenNoMatch(t *testing.T) {
	prose := map[string]any{
		"chunk_id": "p1",
		"content":  "A completely unrelated sentence about weather and clouds.",
	}
	r := &stubRetriever{chunks: []map[string]any{prose}}
	deps, _ := newTestSearchDeps(r)
	chunks, _ := GrepSearch(context.Background(), deps, SearchParams{Question: "who made Culdcept?"})
	if len(chunks) != 1 || chunks[0]["chunk_id"] != "p1" {
		t.Fatalf("chunks = %v, want the raw candidate preserved", chunks)
	}
	if ChunkTextOf(chunks[0]) != ChunkTextOf(prose) {
		t.Error("unmatched prose must be returned whole, not narrowed")
	}
}

// TestGrepSearchDerivesKeywordsHint: with no
// explicit hint the BM25 pool is built from "query + the query's own extracted
// terms", so a long question's proper nouns stop hiding under stopwords and the
// keyword-narrowing stage runs even without a nav hint. An explicit hint (the
// nav-routed docs' summary) wins — "nav is a hint, not a constraint".
func TestGrepSearchDerivesKeywordsHint(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	GrepSearch(context.Background(), deps, SearchParams{Question: "who made Culdcept?"})
	req := r.lastReq(t)
	if want := "who made Culdcept? who made Culdcept"; req.Query != want {
		t.Errorf("query = %q, want %q (question + derived terms)", req.Query, want)
	}
	if ptrFloat(t, req.KeywordsSimilarityWeight) != 1 || !req.ExcludeCompiled {
		t.Error("grep must stay keyword-only (weight 1) and exclude compiled rows")
	}
	// The derived hint also drives the narrowing stage: a prose candidate whose
	// sentences miss those terms is dropped, while a >=3-row pipe table is kept
	// whole (the narrowing's table branch).
	prose := map[string]any{"chunk_id": "p1", "content": "Unrelated sentence about weather."}
	table := map[string]any{"chunk_id": "t1", "content": "a | b | c\nd | e | f\ng | h | i"}
	r2 := &stubRetriever{chunks: []map[string]any{prose, table}}
	deps2, _ := newTestSearchDeps(r2)
	got, _ := GrepSearch(context.Background(), deps2, SearchParams{Question: "who made Culdcept?"})
	if len(got) != 1 || got[0]["chunk_id"] != "t1" {
		t.Errorf("chunks = %v, want only the table chunk", got)
	}

	// An explicit hint wins over the derived terms.
	r3 := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps3, _ := newTestSearchDeps(r3)
	GrepSearch(context.Background(), deps3, SearchParams{Question: "who made Culdcept?", Keywords: "nav summary"})
	if got := r3.lastReq(t).Query; got != "who made Culdcept? nav summary" {
		t.Errorf("query = %q, want the explicit hint appended", got)
	}
}

// TestSearchCacheIsHybridOnly: the search cache is read and written by the hybrid leg
// alone. A keyword-only leg must neither serve nor be served by it — the key carries no
// weight, threshold or compiled policy, so a shared cache would hand a hybrid result
// (vector weight 0.3) to a BM25/grep call and vice versa.
func TestSearchCacheIsHybridOnly(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	p := SearchParams{Question: "same query"}
	HybridSearch(context.Background(), deps, p) // fills the cache
	BM25Search(context.Background(), deps, p)   // must not be served from it
	GrepSearch(context.Background(), deps, p)   // nor must this one
	if len(r.requests) != 3 {
		t.Fatalf("backend calls = %d, want 3 (the search cache is hybrid-only)", len(r.requests))
	}
	// The hybrid leg still dedups its own repeats.
	HybridSearch(context.Background(), deps, p)
	if len(r.requests) != 3 {
		t.Errorf("backend calls = %d, want 3 (hybrid repeat served from cache)", len(r.requests))
	}
}

// TestHybridSearchExcludesCompiledAndUsesSevenTenthsKeywordWeight mirrors Python
// hybrid_search: keyword weight 0.7 when an embedder is configured, compiled
// rows excluded.
func TestHybridSearchExcludesCompiledAndUsesSevenTenthsKeywordWeight(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.HasEmbedder = true
	HybridSearch(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.KeywordsSimilarityWeight); got != 1-HybridSearchDefaultVectorWeight {
		t.Errorf("hybrid search keyword weight = %v, want %v", got, 1-HybridSearchDefaultVectorWeight)
	}
	if !req.ExcludeCompiled {
		t.Error("hybrid search must exclude compiled rows")
	}
}

// TestRetrieveSearchDoesNotExcludeCompiled
// unlike hybrid_search it does NOT exclude compiled rows, and honours
// UsingEmbedding (weight 0.7 when on).
func TestRetrieveSearchDoesNotExcludeCompiled(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.UsingEmbedding = true
	RetrieveSearch(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if req.ExcludeCompiled {
		t.Error("retrieve search must NOT exclude compiled rows")
	}
	if got := ptrFloat(t, req.KeywordsSimilarityWeight); got != 1-DefaultHybridVectorWeight {
		t.Errorf("retrieve search keyword weight = %v, want %v", got, 1-DefaultHybridVectorWeight)
	}
}

// TestHybridSearchMergesSQLKBs verifies that the hybrid leg folds the session's structured
// (SQL) datasets into the target id list.
func TestHybridSearchMergesSQLKBs(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.SQLKBs = []string{"sqlkb1", "sqlkb2"}
	HybridSearch(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	want := map[string]bool{"kb1": true, "sqlkb1": true, "sqlkb2": true}
	if len(req.DatasetIDs) != len(want) {
		t.Fatalf("expected %d dataset ids, got %v", len(want), req.DatasetIDs)
	}
	for _, id := range req.DatasetIDs {
		if !want[id] {
			t.Errorf("unexpected dataset id %q in %v", id, req.DatasetIDs)
		}
	}
}

// TestNormalizeWebResults verifies the web-search payload shaper drops entries
// without a URL or content and keeps chunk_id unique per snippet.
func TestNormalizeWebResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := map[string]any{
			"chunks": []map[string]any{
				{"url": "u1", "content": "c1", "title": "t1"},
				{"url": "", "content": "c2"},   // missing url -> dropped
				{"url": "u2", "content": ""},   // missing content -> dropped
				{"url": "u3", "content": "c3"}, // kept
				{"url": "u1", "content": "c4"}, // same url as u1 -> must keep (unique chunk_id)
			},
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	resp, _ := http.Post(srv.URL, "application/json", bytes.NewReader([]byte(`{}`)))
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	got := normalizeWebResults(raw)
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3 (u1, u3, u1-second-snippet)", len(got))
	}

	seen := map[string]int{}
	for _, c := range got {
		id, _ := c["chunk_id"].(string)
		seen[id]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("chunk_id %q appeared %d times, must be unique", id, n)
		}
	}

	// Filtering of bad entries.
	if c := normalizeWebResults([]byte(`{"chunks":[{"content":"no-url"}]}`)); len(c) != 0 {
		t.Errorf("entry without url kept: %v", c)
	}
	if c := normalizeWebResults([]byte(`{"chunks":[{"url":"x"}]}`)); len(c) != 0 {
		t.Errorf("entry without content kept: %v", c)
	}
}

// TestQueryToTerms pins the tokenizer on the shapes the fan-out prefetch feeds it. The CJK
// case is the regression guard: Go's RE2 \w is ASCII-only, so tokenizing a Chinese
// fan-out with \w+ produced NO terms, leaving the BM25 leg without the keyed terms that
// give the discriminating entity its own score mass — the log line then read
// "keywords: ".
func TestQueryToTerms(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want []string
	}{
		{"cjk sentence", "关羽 斩杀 有姓名 人物 名单", []string{"关羽", "斩杀", "有姓名", "人物", "名单"}},
		// Case is preserved (downstream matching is case-insensitive).
		{"ascii sentence", "Culdcept Saga release date", []string{"Culdcept", "Saga", "release", "date"}},
		// Dedup is case-sensitive, so both survive.
		{"case-sensitive dedup", "Saga saga", []string{"Saga", "saga"}},
		{"regex alternation", `(?i)\b关羽|张飞\b`, []string{"关羽", "张飞"}},
		{"tokens below two runes dropped", "a b cd", []string{"cd"}},
		{"empty query", "   ", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := QueryToTerms(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("QueryToTerms(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}

	// The fan-out's keyed subset must survive for CJK too: without it the BM25
	// leg runs the bare sentence.
	if got := FanoutKeyedTerms(QueryToTerms("关羽 斩杀 有姓名 人物")); len(got) != 1 || got[0] != "有姓名" {
		t.Errorf("FanoutKeyedTerms = %v, want [有姓名] (>=3 runes, non-stopword)", got)
	}
	// Stopwords are compared case-insensitively, so a capitalized one is dropped
	// while the discriminative terms keep their case.
	if got := FanoutKeyedTerms(QueryToTerms("The Culdcept Saga")); !reflect.DeepEqual(got, []string{"Culdcept", "Saga"}) {
		t.Errorf("FanoutKeyedTerms = %v, want [Culdcept Saga]", got)
	}

	// The exact fan-out queries of a real high-mode run ("关羽杀了多少有姓名的人？"),
	// with the keywords the implementation produces for them (verified by
	// running rag.advanced_rag.harness.tools.search._query_to_terms plus the keyed
	// filter of agentic_rag_graph.py:_search_one). Note the surviving term is a generic
	// long word, not the entity: the len>=3 rule drops every two-character
	// Chinese name (关羽/华雄/颜良/文丑/蔡阳), so this is parity, not a Go bug.
	for _, c := range []struct{ in, want string }{
		{"关羽 斩杀 有姓名 人物 名单", "有姓名"},
		{"关羽 斩将 记录 三国演义", "三国演义"},
		{"关羽 杀 将领 三国志", "三国志"},
	} {
		if got := strings.Join(FanoutKeyedTerms(QueryToTerms(c.in)), " "); got != c.want {
			t.Errorf("fan-out keywords for %q = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAgenticVectorWeightDefaultsToZero(t *testing.T) {
	// The agentic retrieve runs keyword-only: UsingEmbedding defaults to
	// False and no caller passes True (agentic_rag.py:retrieve, 643-646).
	got := floatPtrOrDef(nil, DefaultAgenticVectorWeight)
	if got != 0 {
		t.Fatalf("default vector weight = %v, want 0 (keyword-only)", got)
	}
	if DefaultAgenticVectorWeight != 0 {
		t.Fatalf("DefaultAgenticVectorWeight = %v, want 0", DefaultAgenticVectorWeight)
	}
}

func TestVectorWeightHonoursExplicitZero(t *testing.T) {
	// A pointer keeps "configured 0" distinct from "unset", so keyword-only can
	// be requested explicitly rather than only by omission.
	zero := 0.0
	if got := floatPtrOrDef(&zero, DefaultAgenticVectorWeight); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}

	// Fallback must apply only when unset.
	if DefaultAgenticVectorWeight != 0 {
		t.Fatal("the fallback itself must stay 0")
	}
}

func TestVectorWeightCanEnableHybrid(t *testing.T) {
	// Hybrid retrieval stays available, just not by default.
	hybrid := 0.3
	if got := floatPtrOrDef(&hybrid, DefaultAgenticVectorWeight); got != 0.3 {
		t.Fatalf("got %v, want 0.3 when explicitly configured", got)
	}
}

func TestRetrievalDefaultsUseIntOrDef(t *testing.T) {
	if got := intOrDef(0, 12); got != 12 {
		t.Fatalf("intOrDef(0) = %d, want the fallback 12", got)
	}
	if got := intOrDef(5, 12); got != 5 {
		t.Fatalf("intOrDef(5) = %d, want 5", got)
	}
}

func TestResolveKeywordsSimilarityWeightRetrieveMirrorsUsingEmbedding(t *testing.T) {
	// Python RAGTools.retrieve(using_embedding: bool = False).
	// Off → keyword-only (weight 1); on → 0.3 default or the configured override.
	if got := resolveKeywordsSimilarityWeight(SearchDeps{UsingEmbedding: false}, ChannelRetrieve); got != 1 {
		t.Fatalf("using_embedding=false → %v, want 1 (keyword-only)", got)
	}
	if got := resolveKeywordsSimilarityWeight(SearchDeps{UsingEmbedding: true}, ChannelRetrieve); got != 1-DefaultHybridVectorWeight {
		t.Fatalf("using_embedding=true → %v, want %v", got, 1-DefaultHybridVectorWeight)
	}
	override := 0.5
	if got := resolveKeywordsSimilarityWeight(SearchDeps{UsingEmbedding: true, KeywordsSimilarityWeight: &override}, ChannelRetrieve); got != 0.5 {
		t.Fatalf("using_embedding=true with override → %v, want 0.5", got)
	}
}

func TestResolveKeywordsSimilarityWeightHybridDefaultsToSevenTenths(t *testing.T) {
	// Python hybrid_search defaults the vector weight to 0.3, hence its
	// keyword weight is 0.7.
	// (_DEFAULT_HYBRID_VECTOR_WEIGHT,), unlike RAGTools.retrieve's 0.7.
	if got := resolveKeywordsSimilarityWeight(SearchDeps{HasEmbedder: true}, ChannelHybrid); got != 1-HybridSearchDefaultVectorWeight {
		t.Fatalf("hybrid → %v, want %v", got, 1-HybridSearchDefaultVectorWeight)
	}
	if got := resolveKeywordsSimilarityWeight(SearchDeps{HasEmbedder: false}, ChannelHybrid); got != 1 {
		t.Fatalf("hybrid with no embedder → %v, want 1 (Python: `if embd_mdl`)", got)
	}
}

// TestResolveKeywordsSimilarityWeightHybridIgnoresUsingEmbedding is the regression guard for
// the channel-granularity bug: Python's hybrid_search has NO using_embedding
// parameter (search.py:hybrid_search, :143-145), so gating it on that flag disabled the
// semantic leg for search_chunks — losing recall of passages sharing no surface
// words. The gate for this channel is the embedder, nothing else.
func TestResolveKeywordsSimilarityWeightHybridIgnoresUsingEmbedding(t *testing.T) {
	if got := resolveKeywordsSimilarityWeight(SearchDeps{UsingEmbedding: false, HasEmbedder: true}, ChannelHybrid); got != 1-HybridSearchDefaultVectorWeight {
		t.Fatalf("hybrid with using_embedding=false → %v, want %v: the vector leg "+
			"must NOT depend on using_embedding", got, 1-HybridSearchDefaultVectorWeight)
	}
}

// TestResolveKeywordsSimilarityWeightGrepIsAlwaysOne pins Python grep_search: the retrieve
// and grep_* session tools are keyword-only and have no vector leg at all, so no
// flag can turn one on.
func TestResolveKeywordsSimilarityWeightGrepIsAlwaysOne(t *testing.T) {
	override := 0.1
	if got := resolveKeywordsSimilarityWeight(SearchDeps{UsingEmbedding: true, HasEmbedder: true}, ChannelGrep); got != 1 {
		t.Fatalf("grep → %v, want 1 (grep_search has no vector leg)", got)
	}
	if got := resolveKeywordsSimilarityWeight(SearchDeps{UsingEmbedding: true, HasEmbedder: true, KeywordsSimilarityWeight: &override}, ChannelGrep); got != 1 {
		t.Fatalf("grep with override → %v, want 1 (grep_search has no vector leg)", got)
	}
}

// stubVerifier reports a fixed known set, or fails when err is set.
type stubVerifier struct {
	known map[string]bool
	err   error
}

func (s stubVerifier) KnownDocIDs(_ context.Context, _, _ []string) (map[string]bool, error) {
	return s.known, s.err
}

// subsetVerifier mimics the production DocIDLookup.KnownDocIDs: it returns only
// the candidates that are in knownDocs, so an unknown/foreign document yields
// an empty map rather than a false entry. The fixed-set stubVerifier above
// cannot reproduce that empty-map path, which is exactly where the foreign-doc
// rejection used to regress.
type subsetVerifier struct {
	knownDocs map[string]bool
}

func (s subsetVerifier) KnownDocIDs(_ context.Context, _ []string, candidates []string) (map[string]bool, error) {
	out := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if s.knownDocs[c] {
			out[c] = true
		}
	}
	return out, nil
}

func TestResolveDocScopeKeepsKnownIDs(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: stubVerifier{known: map[string]bool{"a": true}},
	}, []string{"a", "b"}, []string{"kb"}, _LOG)

	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("scope = %v, want [a]", got)
	}
}

func TestResolveDocScopeDropsScopeWhenAllUnknown(t *testing.T) {
	// An entirely bogus scope falls back to unfiltered retrieval instead of returning
	// nothing.
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: stubVerifier{known: map[string]bool{"z": true}},
	}, []string{"a", "b"}, []string{"kb"}, _LOG)

	if got != nil {
		t.Fatalf("scope = %v, want nil (unfiltered)", got)
	}
}

func TestResolveDocScopePassthroughWithoutVerifier(t *testing.T) {
	scope := []string{"a", "b"}
	got := resolveDocScope(context.Background(), SearchDeps{}, scope, []string{"kb"}, _LOG)

	if len(got) != len(scope) {
		t.Fatalf("scope = %v, want %v unchanged", got, scope)
	}
}

func TestResolveDocScopePassthroughOnVerifierError(t *testing.T) {
	scope := []string{"a"}
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: stubVerifier{err: errors.New("db down")},
	}, scope, []string{"kb"}, _LOG)

	if len(got) != 1 {
		t.Fatalf("scope = %v, want %v (verification unavailable)", got, scope)
	}
}

func TestDocInDatasetsRejectsForeignDocument(t *testing.T) {
	belongs, verified := docInDatasets(context.Background(), SearchDeps{
		DocIDVerifier: stubVerifier{known: map[string]bool{"other": true}},
	}, "doc")

	if !verified || belongs {
		t.Fatalf("belongs=%v verified=%v, want false/true (doc outside the datasets)", belongs, verified)
	}
}

func TestDocInDatasetsAcceptsOwnDocument(t *testing.T) {
	belongs, verified := docInDatasets(context.Background(), SearchDeps{
		DocIDVerifier: stubVerifier{known: map[string]bool{"doc": true}},
	}, "doc")

	if !verified || !belongs {
		t.Fatalf("belongs=%v verified=%v, want true/true", belongs, verified)
	}
}

func TestDocInDatasetsUnverifiedWithoutVerifier(t *testing.T) {
	// Without a verifier the caller must keep the previous behaviour instead of
	// rejecting the document.
	belongs, verified := docInDatasets(context.Background(), SearchDeps{}, "doc")

	if !belongs || verified {
		t.Fatalf("belongs=%v verified=%v, want true/false", belongs, verified)
	}
}

func TestDocInDatasetsUnverifiedOnError(t *testing.T) {
	belongs, verified := docInDatasets(context.Background(), SearchDeps{
		DocIDVerifier: stubVerifier{err: errors.New("db down")},
	}, "doc")

	if !belongs || verified {
		t.Fatalf("belongs=%v verified=%v, want true/false (lookup failed)", belongs, verified)
	}
}

type stubNavRouter struct {
	called bool
	docs   [][2]string
	// scope records the doc scope the router was asked to route within.
	scope []string
}

func (s *stubNavRouter) Route(_ context.Context, _, _, _ string, docScope []string, _ int) ([][2]string, error) {
	s.called = true
	s.scope = append([]string(nil), docScope...)
	return s.docs, nil
}

func TestDocInDatasetsRejectsForeignDocumentViaSubsetVerifier(t *testing.T) {
	belongs, verified := docInDatasets(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"other": true}},
		KbIDs:         []string{"kb"},
	}, "doc")

	if !verified || belongs {
		t.Fatalf("belongs=%v verified=%v, want false/true (foreign doc rejected even with empty known map)", belongs, verified)
	}
}

// TestResolveDocScopeDropsScopeWhenAllUnknownViaSubsetVerifier exercises the
// production empty-map path: when every candidate is foreign the verifier
// returns an empty map (no error). The scope must be dropped (unfiltered retrieval), not
// passed through.
func TestResolveDocScopeDropsScopeWhenAllUnknownViaSubsetVerifier(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"z": true}},
		KbIDs:         []string{"kb"},
	}, []string{"a", "b"}, []string{"kb"}, _LOG)

	if got != nil {
		t.Fatalf("scope = %v, want nil (unfiltered)", got)
	}
}

// TestResolveDocScopeEmptyWhenCeilingRemovesEveryID pins the ceiling's hard
// edge: when the session doc_scope removes every requested id the result is
// EMPTY (match nothing), never unfiltered. A falsy-empty check would fall through to
// unfiltered retrieval here; the ceiling is deliberately kept absolute.
func TestResolveDocScopeEmptyWhenCeilingRemovesEveryID(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"sess1": true}},
		KbIDs:         []string{"kb"},
		DocScope:      []string{"sess1"}, // fixed session base scope
	}, []string{"a", "b"}, []string{"kb"}, _LOG)

	if got == nil || len(got) != 0 {
		t.Fatalf("scope = %v, want non-nil empty (ceiling removed every requested id)", got)
	}
}

// TestResolveDocScopeEmptyWhenSessionScopeSetAndAllUnknown: when the session has a fixed
// base doc_scope and the
// requested ids survive the ceiling but do not resolve, the result is EMPTY,
// never unfiltered. Encoded as a non-nil empty slice.
func TestResolveDocScopeEmptyWhenSessionScopeSetAndAllUnknown(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"elsewhere": true}},
		KbIDs:         []string{"kb"},
		DocScope:      []string{"sess1"}, // fixed session base scope
	}, []string{"sess1", "sess2"}, []string{"kb"}, _LOG)

	if got == nil || len(got) != 0 {
		t.Fatalf("scope = %v, want non-nil empty (match nothing, not unfiltered)", got)
	}
}

// TestResolveDocScopeFallsBackToSessionScopeWhenRequestNone: a nil request scope falls
// back to the
// session's fixed doc_scope before ownership verification.
func TestResolveDocScopeFallsBackToSessionScopeWhenRequestNone(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"sess1": true}},
		KbIDs:         []string{"kb"},
		DocScope:      []string{"sess1"}, // fixed session base scope
	}, nil, []string{"kb"}, _LOG) // request scope None

	if len(got) != 1 || got[0] != "sess1" {
		t.Fatalf("scope = %v, want [sess1] (request None falls back to session scope)", got)
	}
}

// TestResolveDocScopeUnfilteredWhenNoScopeAtAll pins that a nil request scope
// AND no session base scope searches unfiltered (doc_scope = None), encoded as
// nil — never a non-nil empty slice, which the backend would read as
// "match nothing".
func TestResolveDocScopeUnfilteredWhenNoScopeAtAll(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"z": true}},
		KbIDs:         []string{"kb"},
	}, nil, []string{"kb"}, _LOG)

	if got != nil {
		t.Fatalf("scope = %v, want nil (unfiltered)", got)
	}
}

// TestScopedDocIDsCeiling: no session scope → the caller's scope as-is; a session scope
// with no caller scope → the session scope; both → the intersection.
func TestScopedDocIDsCeiling(t *testing.T) {
	cases := []struct {
		name    string
		session []string
		scope   []string
		want    []string
	}{
		{"no session scope passes through", nil, []string{"a", "b"}, []string{"a", "b"}},
		{"session scope with no caller scope", []string{"s1"}, nil, []string{"s1"}},
		{"explicit scope is intersected", []string{"s1", "s2"}, []string{"s2", "x"}, []string{"s2"}},
		{"disjoint ceiling empties the scope", []string{"s1"}, []string{"x"}, []string{}},
	}
	for _, c := range cases {
		got := scopedDocIDs(c.session, c.scope)
		if len(got) != len(c.want) {
			t.Fatalf("%s: scopedDocIDs = %v, want %v", c.name, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: scopedDocIDs = %v, want %v", c.name, got, c.want)
			}
		}
	}
}

// TestResolveDocScopeIntersectsExplicitScopeWithSession pins that a
// model-generated scope can never escape the session's document ceiling: it is
// intersected with it, not used as-is.
func TestResolveDocScopeIntersectsExplicitScopeWithSession(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"sess1": true, "other": true}},
		KbIDs:         []string{"kb"},
		DocScope:      []string{"sess1"}, // session ceiling
	}, []string{"other", "sess1"}, []string{"kb"}, _LOG)

	if len(got) != 1 || got[0] != "sess1" {
		t.Fatalf("scope = %v, want [sess1] (explicit scope must be ceilinged by the session scope)", got)
	}
}

// TestGrepSearchMatchesPatternOverKeywordCandidates pins the integration: the
// keyword leg samples the corpus, and the PATTERN decides which of those
// candidates are evidence — each returned as the clause around its match.
//
// The two properties a term-locate pass cannot give: a candidate the pattern does
// not match is not evidence whatever its retrieval score (c3), and the match is
// reported with the sentence that carries it rather than the whole chunk.
func TestGrepSearchMatchesPatternOverKeywordCandidates(t *testing.T) {
	long := strings.Repeat("前情提要。", 80) + "华雄出马，关公温酒斩之。" + strings.Repeat("余者不表。", 80)
	r := &stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "content": long},
		{"chunk_id": "c2", "content": "荀正 引军来战，被云长一刀斩于马下。"},
		{"chunk_id": "c3", "content": "曹操引军回许都，不在话下。"},
	}}
	deps, _ := newTestSearchDeps(r)

	got, _ := GrepSearch(context.Background(), deps, SearchParams{Question: "华雄|荀正|管亥"})
	if len(got) != 2 {
		t.Fatalf("got %d chunk(s), want 2: only the candidates the pattern matches are evidence", len(got))
	}
	ids := map[string]bool{}
	for _, c := range got {
		ids[ChunkIDOf(c)] = true
	}
	if !ids["c1"] || !ids["c2"] || ids["c3"] {
		t.Errorf("kept %v, want c1+c2 and NOT c3 (a candidate the pattern does not match is not evidence)", ids)
	}
	for _, c := range got {
		if ChunkIDOf(c) != "c1" {
			continue
		}
		if n := len([]rune(ChunkTextOf(c))); n > 200 {
			t.Errorf("c1 window = %d runes, want the clause around the match, not the whole %d-rune chunk", n, len([]rune(long)))
		}
	}

	// Nothing matched: the raw candidates are returned so evidence is never
	// dropped (and the reach line says what happened).
	none, _ := GrepSearch(context.Background(), deps, SearchParams{Question: "杨龄|夏侯存"})
	if len(none) != 3 {
		t.Errorf("no-match grep returned %d chunk(s), want all 3 raw candidates kept", len(none))
	}
}

// TestPatternNeverReachesTheEngine pins the invariant that makes `|` and `.*`
// usable at all: recall is given the pattern's OPERANDS, and the pattern itself is
// applied here.
//
// Handing the pattern to a keyword leg is what turned a batch probe into a silent
// "the corpus does not carry it": the engine either escapes `|`/`.*` into
// literals or reads them as its own syntax, and an empty result is then
// indistinguishable from an absent fact.
func TestPatternNeverReachesTheEngine(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "content": "关公马快，早赶上文丑，脑后一刀，斩于马下。"},
	}}
	deps, _ := newTestSearchDeps(r)

	for _, q := range []string{"华雄|荀正|管亥", "关公.*斩"} {
		GrepSearch(context.Background(), deps, SearchParams{Question: q})
		for _, req := range r.requests {
			for _, bad := range []string{"|", ".*", ".+"} {
				if strings.Contains(req.Query, bad) {
					t.Errorf("query %q handed %q to the engine (request %q): pattern syntax must stay on this side", q, bad, req.Query)
				}
			}
		}
	}
	// And the recall that WAS issued is the operands, so the engine is still
	// asked about every alternative.
	joined := ""
	for _, req := range r.requests {
		joined += " " + req.Query
	}
	for _, want := range []string{"华雄", "荀正", "管亥", "关公", "斩"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no request carried %q; recall must be issued per operand (%q)", want, joined)
		}
	}
}

// TestPerTermSearchRecordsConfirmedMembers pins that the weave's per-term search
// doubles as the member record: a term searched ON ITS OWN that comes back with a
// passage is a confirmed member with its evidence.
func TestPerTermSearchRecordsConfirmedMembers(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c-yan", "content": "荀正 引军来战，被云长一刀斩于马下。"},
	}}
	deps, kb := newTestSearchDeps(r)

	GrepSearch(context.Background(), deps, SearchParams{Question: "荀正|管亥"})
	got := kb.ReachedTerms()
	if len(got) != 1 || got[0].Term != "荀正" {
		t.Fatalf("ReachedTerms = %+v, want 荀正 recorded with the passage that carries it", got)
	}
	if got[0].ChunkID != "c-yan" {
		t.Errorf("recorded chunk = %q, want the passage the term's own search returned", got[0].ChunkID)
	}
}

// TestPatternRecallIsPerOperandAndWide pins the LOCATOR fix: a structural pattern
// ("关公.*斩") asks the engine about each of its operands, at a width that gives
// the pattern ground to match in.
//
// The pattern used to be only a FILTER: the legs' own topN (10 for `retrieve`)
// decided what it could ever see, so a query about how a deed is written could
// match inside ten passages, and reported per-term reach over those ten. That is
// the one retrieval path that does not depend on the model already knowing the
// name it is looking for — and it was looking through a keyhole.
func TestPatternRecallIsPerOperandAndWide(t *testing.T) {
	answer := []map[string]any{{"chunk_id": "c1", "content": "关公勒马，一刀斩之"}}

	// Structural pattern: one search per operand, each WIDE.
	wide := &stubRetriever{chunks: answer}
	deps, _ := newTestSearchDeps(wide)
	GrepSearch(context.Background(), deps, SearchParams{
		Question: "关公.*斩|云长.*斩", TopN: 10, KbIDs: []string{"kb1"},
	})
	if len(wide.requests) < 2 {
		t.Fatalf("requests = %d, want one per operand", len(wide.requests))
	}
	for _, r := range wide.requests {
		if r.TopN < patternRecallTopN {
			t.Errorf("operand %q recalled TopN=%d, want >= %d", r.Query, r.TopN, patternRecallTopN)
		}
	}

	// An alternation of NAMES keeps the leg's own topN: a rare name's top ten is
	// enough, and widening there would only cost the engine.
	narrow := &stubRetriever{chunks: answer}
	depsNames, _ := newTestSearchDeps(narrow)
	GrepSearch(context.Background(), depsNames, SearchParams{
		Question: "华雄|荀正", TopN: 10, KbIDs: []string{"kb1"},
	})
	if len(narrow.requests) == 0 {
		t.Fatal("no requests: the alternation must still search per name")
	}
	for _, r := range narrow.requests {
		if r.TopN != 10 {
			t.Errorf("name probe %q recalled TopN=%d, want the leg's own 10", r.Query, r.TopN)
		}
	}
}

// TestCallerBatchRecognisesTheBatchTheModelWrites pins the weave's trigger.
//
// The per-term seat allocation exists for the batches the model writes, and over
// the runs of 2026-09-15 it wrote them with SPACES: `关羽 斩 华雄 颜良 文丑 蔡阳`,
// never once with `|`. A trigger keyed on `|` therefore left the whole mechanism
// dead code while precisely those batches went to a single ranked top-N — the
// ranking that hands nearly every seat to the passages matching the most terms at
// once, which is how four names the corpus carries came back with nothing.
func TestCallerBatchRecognisesTheBatchTheModelWrites(t *testing.T) {
	for _, q := range []string{
		"关羽 斩 华雄 颜良 文丑 蔡阳",
		"华雄|颜良|文丑",
		"关公.*斩|云长.*斩",
	} {
		if !callerBatch(q) {
			t.Errorf("callerBatch(%q) = false, want the batch recognised", q)
		}
	}
	// A sentence is not a batch, in either language: that is what keeps the extra
	// searches off the questions that are not enumerating anything.
	for _, q := range []string{
		"三国演义中，关羽杀了多少有姓名的人物？",
		"What was the outcome of the battle at Red Cliffs?",
		"关羽",
	} {
		if callerBatch(q) {
			t.Errorf("callerBatch(%q) = true, want a sentence left on the single search", q)
		}
	}
}

// TestProbeItemsAreTheCallersOwnWords pins the reach ledger's reading of a call.
//
// The ledger is read back as the session's to-do list ("probed, came back with a
// passage, not recorded"), so it may only hold what the call PROPOSED as items —
// the pieces of a batch, or a query that is one word. Measured (2026-09-15): the
// line read `FOUND BUT NOT RECORDED=三国、演义、关羽、五关…+15` in a run whose
// sessions were missing six members, none of which was on the list, because the
// windows an unbroken clause decomposes into had been probed AND recorded.
func TestProbeItemsAreTheCallersOwnWords(t *testing.T) {
	got := probeItemsOf([]string{"关羽 古城 蔡阳 斩 颜良 文丑 华雄 庞德 荀正", "韩福"})
	for _, want := range []string{"蔡阳", "颜良", "华雄", "荀正", "韩福"} {
		if !got[strings.ToLower(want)] {
			t.Errorf("probeItemsOf missed the proposed item %q", want)
		}
	}
	// A one-rune verb is stripped as a term edge, not proposed as an item.
	if got["斩"] {
		t.Error("a single-rune fragment must not count as a proposed item")
	}

	// A question is not a proposal, and an unbroken clause yields no item either.
	if items := probeItemsOf([]string{"三国演义中关羽一共杀死多少有姓名的人物"}); len(items) != 0 {
		t.Errorf("a sentence proposed %v as items, want nothing", items)
	}

	// The batch case that produced the junk: the windows of 关羽过五关斩六将 must
	// not appear, while the caller's own words do.
	batch := probeItemsOf([]string{"三国演义 关羽过五关斩六将 六将姓名"})
	for _, window := range []string{"国演", "演义", "羽过", "过五", "关斩", "斩六"} {
		if batch[window] {
			t.Errorf("window %q must never reach the ledger", window)
		}
	}
	if !batch["三国演义"] {
		t.Error("the caller's own word 三国演义 must be a candidate item")
	}
}
