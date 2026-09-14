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

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	// No bound datasets -> no search at all (Python returns empty kbinfos).
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

// TestHybridSearchEffectiveQueryCapsCodePoints pins the expanded-query cap to
// code points, not bytes.  Python slices a str with `[:400]` (search.py:129/216/254),
// so the cap counts code points: a byte slice both splits a multi-byte rune —
// handing the retriever invalid UTF-8 — and caps a CJK query at ~133 characters,
// dropping expansion terms the fan-out leg weighs on.  The ASCII case above
// cannot tell the two apart (bytes == runes there).
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
	// Python passes rank_feature ONLY from RAGTools.retrieve
	// (agentic_rag.py:668 rank_feature=label_question(question, self.kbs)).
	// search.py's three legs call retriever.retrieval WITHOUT rank_feature
	// (:158-173 hybrid, :223-238 vector, :260-275 bm25; grep_search delegates to
	// bm25_search, :428), so those requests must stay nil even with a Tagger.
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

	// Nil Tagger -> empty rank feature even on the retrieve leg (Python
	// label_question returning None).
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

// ---------------------------------------------------------------------------
// Narrowing
// ---------------------------------------------------------------------------

func TestNarrowOrKeepIsAllOrNothing(t *testing.T) {
	chunks := []map[string]any{
		{"content": "Culdcept was made by OmiyaSoft."},
		{"content": "It was released in 1999."},
	}
	// Keywords hit -> narrowed subset (the non-matching chunk is dropped).
	got := NarrowOrKeep(chunks, "culdcept", "test", nil)
	if len(got) != 1 || !strings.Contains(ChunkTextOf(got[0]), "Culdcept") {
		t.Errorf("narrowed = %v, want only the matching chunk", got)
	}

	// No keyword overlap -> ALL chunks kept (this is the whole point: the
	// retriever already ranked them, and dropping everything produced empty
	// results and unverified claims).
	got = NarrowOrKeep(chunks, "zzz-no-match", "test", nil)
	if len(got) != len(chunks) {
		t.Errorf("no-match kept %d, want all %d", len(got), len(chunks))
	}

	// Empty keywords -> untouched.
	if got := NarrowOrKeep(chunks, "", "test", nil); len(got) != len(chunks) {
		t.Error("empty keywords must return chunks unchanged")
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
	// Python's star marker, and ONE span for the multi-word entity
	// (text_processing.py:391-394) — never "*New* *York*".
	if !strings.Contains(got, "*New York*") {
		t.Errorf("highlight = %q, want the longest term applied", got)
	}
	if strings.Contains(got, "*york*") {
		t.Errorf("shorter term must not win: %q", got)
	}
}

// TestHighlightKeywordsFoldsWithoutByteOffsets pins the matching to rune space.
// `strings.ToLower` is not byte-length-preserving: "İ" (U+0130) is 2 bytes and
// folds to the 1-byte "i" (Go applies the simple 1:1 case mapping, the opposite
// direction from Python's full fold, which expands it to 3 bytes). So a byte
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
// folded the way the haystack is: terms are matched against the lowercased text
// (`lows`), and Python builds its phrase list with `(kw or "").strip().lower()`
// (text_processing.py:396) plus re.IGNORECASE (:411). A caller-supplied "Rocket"
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

// TestHighlightKeywordsKeepsPhrasePartsWhole pins Python's phrase guard
// (text_processing.py:405): a stem-matched word that already occurs inside a
// keyword phrase is NOT added as a term of its own, so a standalone "Braves" is
// left alone and only the "Atlanta Braves" span is starred.
func TestHighlightKeywordsKeepsPhrasePartsWhole(t *testing.T) {
	got := HighlightKeywords("Braves lost. Atlanta Braves won.", []string{"Atlanta Braves"})
	if want := "Braves lost. *Atlanta Braves* won."; got != want {
		t.Errorf("highlight = %q, want %q", got, want)
	}
}

// TestHighlightKeywordsStemMatchesCapitalisedWords pins the word scan: Python
// stems every `[A-Za-z]+` word (:403), so a capitalised inflected word still
// contributes its stem term. The shared lowercase pattern matched only the
// fragment after the capital ("Nominated" -> "ominated"), so nothing was starred.
func TestHighlightKeywordsStemMatchesCapitalisedWords(t *testing.T) {
	got := HighlightKeywords("Nominated twice.", []string{"nominations"})
	if want := "*Nominated* twice."; got != want {
		t.Errorf("highlight = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Doc aggregations
// ---------------------------------------------------------------------------

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
	// doc_aggs must reflect the FULL pre-narrow retrieved set, mirroring Python
	// where _normalize takes doc_aggs from the retriever and _narrow_or_keep only
	// replaces kbinfos["chunks"]. Here the keyword narrows the chunks to doc d1,
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

// TestVectorSearchBailsWithoutEmbedder mirrors Python vector_search: with no
// embedder configured it returns nothing (Python bails when embd_mdl is unset),
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

// TestVectorSearchWeightIsOne mirrors Python vector_search: the pure-vector leg
// carries weight 1.0 and excludes compiled rows.
func TestVectorSearchWeightIsOne(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.HasEmbedder = true
	VectorSearch(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.VectorSimilarityWeight); got != 1.0 {
		t.Errorf("vector search weight = %v, want 1.0", got)
	}
	if !req.ExcludeCompiled {
		t.Error("vector search must exclude compiled rows")
	}
}

// TestBM25SearchUsesZeroWeight mirrors Python bm25_search: keyword-only, vector
// weight unconditionally 0, threshold 0.0, excludes compiled rows.
func TestBM25SearchUsesZeroWeight(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	BM25Search(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.VectorSimilarityWeight); got != 0 {
		t.Errorf("bm25 search weight = %v, want 0", got)
	}
	// Python bm25_search passes embd_mdl=None: no dense leg at all.
	if !req.DisableVectorLeg {
		t.Error("bm25 search must disable the vector leg (embd_mdl=None)")
	}
	if got := ptrFloat(t, req.SimilarityThreshold); got != 0 {
		t.Errorf("bm25 search threshold = %v, want 0", got)
	}
	if !req.ExcludeCompiled {
		t.Error("bm25 search must exclude compiled rows")
	}
}

// TestGrepSearchDelegatesToBM25 mirrors Python grep_search: it is bm25_search
// with a keyword-only (weight 0) leg and compiled-row exclusion.
func TestGrepSearchDelegatesToBM25(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	GrepSearch(context.Background(), deps, SearchParams{Question: "q", Keywords: "kw"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.VectorSimilarityWeight); got != 0 {
		t.Errorf("grep search weight = %v, want 0", got)
	}
	if !req.DisableVectorLeg {
		t.Error("grep search must disable the vector leg (embd_mdl=None)")
	}
	if !req.ExcludeCompiled {
		t.Error("grep search must exclude compiled rows")
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

// TestGrepSearchNarrowsProseViaTermWindow mirrors Python grep_search's two
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
	// table alone and Python drops the prose outright — pinned by
	// TestGrepSearchDerivesKeywordsHint.)
	chunks, _ := GrepSearch(context.Background(), deps, SearchParams{Question: "Culdcept was made"})

	// Order: table chunks first, then narrowed prose (Python keeps table+kept).
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2 (table + narrowed prose)", len(chunks))
	}
	if chunks[0]["chunk_id"] != "t1" {
		t.Errorf("first chunk = %v, want table chunk t1", chunks[0]["chunk_id"])
	}
	// The table chunk must be returned WHOLE, every row intact: Python's
	// pipe-table branch returns "..." + _highlight_keywords(content) + "..."
	// (text_processing.py _narrow_content), so the body is unchanged and only
	// the wrapper differs from the raw chunk.
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

// TestGrepSearchKeepsRawCandidatesWhenNoMatch mirrors Python grep_search: when
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

// TestGrepSearchDerivesKeywordsHint mirrors Python grep_search:421-424: with no
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
	if ptrFloat(t, req.VectorSimilarityWeight) != 0 || !req.ExcludeCompiled {
		t.Error("grep must stay keyword-only (weight 0) and exclude compiled rows")
	}
	// The derived hint also drives the narrowing stage: a prose candidate whose
	// sentences miss those terms is dropped, while a >=3-row pipe table is kept
	// whole (Python _narrow_by_keywords + _narrow_content's table branch).
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

// TestSearchCacheIsHybridOnly mirrors Python: tools.search_cache is read and
// written by hybrid_search alone (/ :207). A keyword-only leg
// must neither serve nor be served by it — the key carries no weight,
// threshold or compiled policy, so a shared cache would hand a hybrid result
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

// TestHybridSearchExcludesCompiledAndWeightsThreeTenths mirrors Python
// hybrid_search: vector weight 0.3 when an embedder is configured, compiled
// rows excluded.
func TestHybridSearchExcludesCompiledAndWeightsThreeTenths(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.HasEmbedder = true
	HybridSearch(context.Background(), deps, SearchParams{Question: "q"})
	req := r.lastReq(t)
	if got := ptrFloat(t, req.VectorSimilarityWeight); got != HybridSearchDefaultVectorWeight {
		t.Errorf("hybrid search weight = %v, want %v", got, HybridSearchDefaultVectorWeight)
	}
	// With an embedder configured Python hybrid_search passes the real
	// embd_mdl: the dense leg RUNS at weight 0.3.
	if req.DisableVectorLeg {
		t.Error("hybrid search with an embedder must keep the vector leg")
	}
	if !req.ExcludeCompiled {
		t.Error("hybrid search must exclude compiled rows")
	}
}

// TestRetrieveSearchDoesNotExcludeCompiled mirrors Python RAGTools.retrieve:
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
	if got := ptrFloat(t, req.VectorSimilarityWeight); got != DefaultHybridVectorWeight {
		t.Errorf("retrieve search weight = %v, want %v", got, DefaultHybridVectorWeight)
	}
}

// TestHybridSearchMergesSQLKBs verifies that hybrid_search folds the session's
// structured (SQL) datasets into the target id list, mirroring Python
// search.py:hybrid_search (`tools.kb_ids + [kb.id for kb in tools.sql_kbs]`).
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

// TestQueryToTerms pins Python _query_to_terms on the shapes the
// fan-out prefetch feeds it. The CJK case is the
// regression guard: Go's RE2 \w is ASCII-only, so tokenizing a Chinese fan-out
// with \w+ produced NO terms, leaving the BM25 leg of the fan-out without the
// keyed terms that give the discriminating entity its own score mass
// (agentic_rag_graph.py:_search_one) — the log line then read "keywords: ".
func TestQueryToTerms(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want []string
	}{
		{"cjk sentence", "关羽 斩杀 有姓名 人物 名单", []string{"关羽", "斩杀", "有姓名", "人物", "名单"}},
		// Case is preserved, as in Python (downstream matching is (?i)).
		{"ascii sentence", "Culdcept Saga release date", []string{"Culdcept", "Saga", "release", "date"}},
		// Python dedups case-sensitively (tok not in terms), so both survive.
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
	// with the keywords Python's implementation produces for them (verified by
	// running rag.advanced_rag.harness.tools.search._query_to_terms plus the keyed
	// filter of agentic_rag_graph.py:_search_one). Note the surviving term is a generic
	// long word, not the entity: Python's len>=3 rule drops every two-character
	// Chinese name (关羽/华雄/颜良/文丑/蔡阳), so this is parity, not a Go bug.
	for _, c := range []struct{ in, want string }{
		{"关羽 斩杀 有姓名 人物 名单", "有姓名"},
		{"关羽 斩将 记录 三国演义", "三国演义"},
		{"关羽 杀 将领 三国志", "三国志"},
	} {
		if got := strings.Join(FanoutKeyedTerms(QueryToTerms(c.in)), " "); got != c.want {
			t.Errorf("fan-out keywords for %q = %q, want %q (Python parity)", c.in, got, c.want)
		}
	}
}

func TestAgenticVectorWeightDefaultsToZero(t *testing.T) {
	// Python's agentic retrieve runs keyword-only: using_embedding defaults to
	// False and no caller passes True (agentic_rag.py:retrieve, 643-646).
	got := floatPtrOrDef(nil, DefaultAgenticVectorWeight)
	if got != 0 {
		t.Fatalf("default vector weight = %v, want 0 (keyword-only, as in Python)", got)
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
		t.Fatal("the fallback itself must stay 0 to match Python")
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

func TestResolveVectorWeightRetrieveMirrorsUsingEmbedding(t *testing.T) {
	// Python RAGTools.retrieve(using_embedding: bool = False).
	// Off → keyword-only (weight 0); on → 0.7 default or the configured override.
	if got := resolveVectorWeight(SearchDeps{UsingEmbedding: false}, ChannelRetrieve); got != 0 {
		t.Fatalf("using_embedding=false → %v, want 0 (keyword-only)", got)
	}
	if got := resolveVectorWeight(SearchDeps{UsingEmbedding: true}, ChannelRetrieve); got != DefaultHybridVectorWeight {
		t.Fatalf("using_embedding=true → %v, want %v", got, DefaultHybridVectorWeight)
	}
	override := 0.5
	if got := resolveVectorWeight(SearchDeps{UsingEmbedding: true, VectorSimilarityWeight: &override}, ChannelRetrieve); got != 0.5 {
		t.Fatalf("using_embedding=true with override → %v, want 0.5", got)
	}
}

func TestResolveVectorWeightHybridDefaultsToThreeTenths(t *testing.T) {
	// Python hybrid_search defaults the vector weight to 0.3
	// (_DEFAULT_HYBRID_VECTOR_WEIGHT,), unlike RAGTools.retrieve's 0.7.
	if got := resolveVectorWeight(SearchDeps{HasEmbedder: true}, ChannelHybrid); got != HybridSearchDefaultVectorWeight {
		t.Fatalf("hybrid → %v, want %v", got, HybridSearchDefaultVectorWeight)
	}
	if got := resolveVectorWeight(SearchDeps{HasEmbedder: false}, ChannelHybrid); got != 0 {
		t.Fatalf("hybrid with no embedder → %v, want 0 (Python: `if embd_mdl`)", got)
	}
}

// TestResolveVectorWeightHybridIgnoresUsingEmbedding is the regression guard for
// the channel-granularity bug: Python's hybrid_search has NO using_embedding
// parameter (search.py:hybrid_search, :143-145), so gating it on that flag disabled the
// semantic leg for search_chunks — losing recall of passages sharing no surface
// words. The gate for this channel is the embedder, nothing else.
func TestResolveVectorWeightHybridIgnoresUsingEmbedding(t *testing.T) {
	if got := resolveVectorWeight(SearchDeps{UsingEmbedding: false, HasEmbedder: true}, ChannelHybrid); got != HybridSearchDefaultVectorWeight {
		t.Fatalf("hybrid with using_embedding=false → %v, want %v: the vector leg "+
			"must NOT depend on using_embedding", got, HybridSearchDefaultVectorWeight)
	}
}

// TestResolveVectorWeightGrepIsAlwaysZero pins Python grep_search: the retrieve
// and grep_* session tools are keyword-only and have no vector leg at all, so no
// flag can turn one on.
func TestResolveVectorWeightGrepIsAlwaysZero(t *testing.T) {
	override := 0.9
	if got := resolveVectorWeight(SearchDeps{UsingEmbedding: true, HasEmbedder: true}, ChannelGrep); got != 0 {
		t.Fatalf("grep → %v, want 0 (grep_search has no vector leg)", got)
	}
	if got := resolveVectorWeight(SearchDeps{UsingEmbedding: true, HasEmbedder: true, VectorSimilarityWeight: &override}, ChannelGrep); got != 0 {
		t.Fatalf("grep with override → %v, want 0 (grep_search has no vector leg)", got)
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
	// Python retrieve:633-635 — an entirely bogus scope falls back to
	// unfiltered retrieval instead of returning nothing.
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
// returns an empty map (no error). The scope must be dropped (unfiltered
// retrieval, mirroring Python retrieve's fall-back), not passed through.
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
// EMPTY (match nothing), never unfiltered. Python's falsy-empty quirk would fall
// through to unfiltered retrieval here; Go deliberately keeps the ceiling
// absolute.
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

// TestResolveDocScopeEmptyWhenSessionScopeSetAndAllUnknown pins Python
// retrieve:631-632 — when the session has a fixed base doc_scope and the
// requested ids survive the ceiling but do not resolve, the result is EMPTY,
// never unfiltered. Encoded as a non-nil empty slice.
func TestResolveDocScopeEmptyWhenSessionScopeSetAndAllUnknown(t *testing.T) {
	got := resolveDocScope(context.Background(), SearchDeps{
		DocIDVerifier: subsetVerifier{knownDocs: map[string]bool{"elsewhere": true}},
		KbIDs:         []string{"kb"},
		DocScope:      []string{"sess1"}, // fixed session base scope
	}, []string{"sess1", "sess2"}, []string{"kb"}, _LOG)

	if got == nil || len(got) != 0 {
		t.Fatalf("scope = %v, want non-nil empty (Python returns empty result, not unfiltered)", got)
	}
}

// TestResolveDocScopeFallsBackToSessionScopeWhenRequestNone pins Python's
// scoped_doc_ids fallback (retrieve:620): a None request scope falls back to the
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

// TestScopedDocIDsMirrorsPythonCeiling pins RAGTools.scoped_doc_ids
// (agentic_rag.py:scoped_doc_ids): no session scope → the caller's scope as-is; a
// session scope with no caller scope → the session scope; both → the
// intersection.
func TestScopedDocIDsMirrorsPythonCeiling(t *testing.T) {
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
