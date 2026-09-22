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
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// stubRetriever returns a fixed result and records the requests it received.
type stubRetriever struct {
	mu       sync.Mutex
	chunks   []map[string]any
	err      error
	requests []RetrieveRequest
}

func (s *stubRetriever) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	// Locked: the search legs run concurrently (see runSearch), so a fixture that appends
	// without a lock is a race.
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
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

// TestNarrowContentRendersHTMLTablesAsLines pins the table branch: an HTML table is
// serialized to its rendered line view before the model sees it (raw <table>/<td> markup is
// the expensive and least readable form), and the row set is not pruned.
func TestNarrowContentRendersHTMLTablesAsLines(t *testing.T) {
	content := "<table><tr><th>Rank</th><th>Rider</th><th>Points</th></tr>" +
		"<tr><td>19</td><td>Danilo</td><td>62</td></tr>" +
		"<tr><td>20</td><td>Erik</td><td>61</td></tr></table>"
	got, ok := NarrowContent(content, []string{"danilo"})
	if !ok {
		t.Fatal("NarrowContent must keep a table whole")
	}
	if strings.Contains(got, "<td>") || strings.Contains(got, "<table") {
		t.Errorf("narrowed table still carries raw HTML: %q", got)
	}
	for _, want := range []string{`"Rank": "19"`, "Danilo", `"Points": "62"`, `"Rank": "20"`} {
		if !strings.Contains(got, want) {
			t.Errorf("narrowed table lost %q: %q", want, got)
		}
	}
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...") {
		t.Errorf("narrowed text must be wrapped in ellipses: %q", got)
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

// stubMetadataResolver is a scripted MetadataResolver: the push-down answer, the
// flattened view and the call log are all set per test.
type stubMetadataResolver struct {
	pushdownIDs   []string
	pushdownOK    bool
	metas         common.MetaData
	flattenErr    error
	pushdownCalls int
	flattenCalls  int
	gotKbIDs      []string
	gotFilters    []map[string]any
	gotLogic      string
	// docMeta is the per-document metadata the context block is built from; docMetaErr lets
	// a test pin that a failed context read still returns the selected ids.
	docMeta    map[string]map[string]any
	docMetaErr error
}

func (s *stubMetadataResolver) FilterDocIDsByMetaPushdown(_ context.Context, kbIDs []string, filters []map[string]any, logic string) ([]string, bool) {
	s.pushdownCalls++
	s.gotKbIDs = kbIDs
	s.gotFilters = filters
	s.gotLogic = logic
	return s.pushdownIDs, s.pushdownOK
}

func (s *stubMetadataResolver) GetFlattedMetaByKBs(context.Context, []string) (common.MetaData, error) {
	s.flattenCalls++
	return s.metas, s.flattenErr
}

func (s *stubMetadataResolver) MetadataForDocIDs(context.Context, []string, []string) (map[string]map[string]any, error) {
	return s.docMeta, s.docMetaErr
}

// TestMetadataSearchScopesHybridToMatchedDocuments pins the retrieval leg: the metadata
// match decides the document set, the hybrid search runs inside it (compiled expansion
// OFF, so nothing outside the set can be pulled in), and the push-down hit means the
// flattened view is never read.
func TestMetadataSearchScopesHybridToMatchedDocuments(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"id": "c1", "doc_id": "d1", "content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	res := &stubMetadataResolver{pushdownOK: true, pushdownIDs: []string{"d1", "d2"}}
	deps.MetadataResolver = res

	filters := []map[string]any{{"key": "title", "op": "contains", "value": "New York"}}
	chunks, _ := MetadataSearch(context.Background(), deps, SearchParams{Question: "how many?", TopN: 20}, filters, "and")

	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if res.flattenCalls != 0 {
		t.Errorf("flatten calls = %d: a push-down hit must not read the flattened view", res.flattenCalls)
	}
	if res.gotLogic != "and" || len(res.gotFilters) != 1 {
		t.Errorf("resolver got filters=%v logic=%q", res.gotFilters, res.gotLogic)
	}
	if len(res.gotKbIDs) != 1 || res.gotKbIDs[0] != "kb1" {
		t.Errorf("resolver kbIDs = %v, want the session's datasets", res.gotKbIDs)
	}
	if len(r.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(r.requests))
	}
	scope := r.requests[0].DocScope
	if len(scope) != 2 || scope[0] != "d1" || scope[1] != "d2" {
		t.Errorf("doc_scope = %v, want the matched documents", scope)
	}
}

// TestMetadataSearchFallsBackToInMemoryFilter pins the fallback: when the push-down is
// not viable, the flattened metadata is filtered in memory with the same conditions.
func TestMetadataSearchFallsBackToInMemoryFilter(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"id": "c1", "doc_id": "d1", "content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	res := &stubMetadataResolver{
		pushdownOK: false,
		metas: common.MetaData{
			"title": {"New York City": {"d1"}, "Boston": {"d2"}},
		},
	}
	deps.MetadataResolver = res

	chunks, _ := MetadataSearch(context.Background(), deps, SearchParams{Question: "q"},
		[]map[string]any{{"key": "title", "op": "contains", "value": "New York"}}, "and")

	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want the in-memory hit", len(chunks))
	}
	if res.flattenCalls != 1 {
		t.Errorf("flatten calls = %d, want 1", res.flattenCalls)
	}
	if scope := r.requests[0].DocScope; len(scope) != 1 || scope[0] != "d1" {
		t.Errorf("doc_scope = %v, want the in-memory match", scope)
	}
}

// TestMetadataSearchEmptyResultIsNotASearch pins the miss path: an empty match (definitive
// from the push-down, or after the session ceiling) must reach the retriever zero times,
// and must not be reported as an error.
func TestMetadataSearchEmptyResultIsNotASearch(t *testing.T) {
	filters := []map[string]any{{"key": "title", "op": "contains", "value": "nowhere"}}

	// Push-down says: definitively no match.
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	deps.MetadataResolver = &stubMetadataResolver{pushdownOK: true}
	if chunks, _ := MetadataSearch(context.Background(), deps, SearchParams{Question: "q"}, filters, "and"); chunks != nil {
		t.Errorf("chunks = %v, want nil", chunks)
	}
	if len(r.requests) != 0 {
		t.Errorf("requests = %d: an empty match must not search", len(r.requests))
	}

	// The session ceiling removes every matched document.
	r2 := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps2, _ := newTestSearchDeps(r2)
	deps2.DocScope = []string{"d9"}
	deps2.MetadataResolver = &stubMetadataResolver{pushdownOK: true, pushdownIDs: []string{"d1"}}
	if chunks, _ := MetadataSearch(context.Background(), deps2, SearchParams{Question: "q"}, filters, "and"); chunks != nil {
		t.Errorf("chunks = %v, want nil", chunks)
	}
	if len(r2.requests) != 0 {
		t.Errorf("requests = %d: a scope that excludes everything must not search", len(r2.requests))
	}
}

// TestMetadataSearchIntersectsTheSessionScope pins the ceiling: the metadata set cannot
// escape the session's document restriction.
func TestMetadataSearchIntersectsTheSessionScope(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"id": "c1", "doc_id": "d2", "content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	deps.DocScope = []string{"d2", "d3"}
	deps.MetadataResolver = &stubMetadataResolver{pushdownOK: true, pushdownIDs: []string{"d1", "d2", "d3"}}

	filters := []map[string]any{{"key": "title", "op": "contains", "value": "x"}}
	if chunks, _ := MetadataSearch(context.Background(), deps, SearchParams{Question: "q"}, filters, "and"); len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	scope := r.requests[0].DocScope
	if len(scope) != 2 || scope[0] != "d2" || scope[1] != "d3" {
		t.Errorf("doc_scope = %v, want the intersection with the session scope", scope)
	}
}

// TestMetadataSearchWithoutResolverIsInert pins the unwired seam: no resolver (or no
// filters) means the leg does nothing at all, never a failed search.
func TestMetadataSearchWithoutResolverIsInert(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"content": "x"}}}
	deps, _ := newTestSearchDeps(r)
	filters := []map[string]any{{"key": "title", "op": "contains", "value": "x"}}

	if chunks, _ := MetadataSearch(context.Background(), deps, SearchParams{Question: "q"}, filters, "and"); chunks != nil {
		t.Errorf("chunks = %v, want nil without a resolver", chunks)
	}
	deps.MetadataResolver = &stubMetadataResolver{pushdownOK: true, pushdownIDs: []string{"d1"}}
	if chunks, _ := MetadataSearch(context.Background(), deps, SearchParams{Question: "q"}, nil, "and"); chunks != nil {
		t.Errorf("chunks = %v, want nil without filters", chunks)
	}
	if len(r.requests) != 0 {
		t.Errorf("requests = %d, want none", len(r.requests))
	}
}

// ===================== metadata catalog =====================

// TestMetadataCatalogForBuildsSortedKeysWithSamples pins what the model gets to see: the
// dataset's real fields, deterministically ordered, each with its strongest values.
func TestMetadataCatalogForBuildsSortedKeysWithSamples(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{
		"title":  {"Boston": {"d1"}, "New York City": {"d2", "d3", "d4"}},
		"author": {"Alice": {"d1"}, "Bob": {"d2"}},
	}}

	cat := MetadataCatalogFor(context.Background(), deps)
	if got := strings.Join(cat.Keys, ","); got != "author,title" {
		t.Fatalf("keys = %q, want author,title (sorted, deterministic)", got)
	}
	samples := cat.Samples["title"]
	if len(samples) != 2 {
		t.Fatalf("title samples = %v, want 2", samples)
	}
	// Ordered by document count: the value most documents carry is the useful one to show.
	if samples[0].Value != "New York City" || samples[0].Docs != 3 {
		t.Errorf("first title sample = %+v, want New York City (3 docs)", samples[0])
	}

	render := cat.Render()
	for _, want := range []string{"AVAILABLE METADATA", "title", "author", "New York City"} {
		if !strings.Contains(render, want) {
			t.Errorf("render missing %q:\n%s", want, render)
		}
	}
	// No caps: every offered field must appear, so a filter can name any of them.
	for _, k := range cat.Keys {
		if !strings.Contains(render, k) {
			t.Errorf("render dropped field %q:\n%s", k, render)
		}
	}
}

// TestMetadataCatalogForDropsSystemAndLabelKeys pins the blacklist: identifiers and
// benchmark annotations must never become filters — a `question_id` filter would let the
// model shrink retrieval to the very documents that answer the question.
func TestMetadataCatalogForDropsSystemAndLabelKeys(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{
		"title":       {"A": {"d1"}},
		"question_id": {"444": {"d1"}},
		"source_uri":  {"s3://x": {"d1"}},
		"pageid":      {"1": {"d1"}},
		"outline":     {"intro": {"d1"}},
		"_version":    {"2": {"d1"}},
	}}

	cat := MetadataCatalogFor(context.Background(), deps)
	if got := strings.Join(cat.Keys, ","); got != "title" {
		t.Fatalf("keys = %q, want only title", got)
	}
	if render := cat.Render(); strings.Contains(render, "question_id") {
		t.Errorf("the benchmark-annotation key leaked into the catalog:\n%s", render)
	}
}

// TestMetadataCatalogForDegradesToEmpty pins the failure contract every "no metadata" path
// depends on: a nil resolver, an unreadable index, a metadata-free dataset or no bound
// datasets all yield the EMPTY catalog — never an error and never a panic. An empty
// catalog is what makes the session keep the shipped title-only schema.
func TestMetadataCatalogForDegradesToEmpty(t *testing.T) {
	ctx := context.Background()
	base, _ := newTestSearchDeps(&stubRetriever{})

	// No resolver wired (deployment without the metadata link).
	if cat := MetadataCatalogFor(ctx, base); !cat.Empty() {
		t.Errorf("keys = %v, want empty without a resolver", cat.Keys)
	}
	if ptr := MetadataCatalogPtr(ctx, base); ptr != nil {
		t.Error("MetadataCatalogPtr must be nil for an empty catalog")
	}

	// Index unreadable.
	broken := base
	broken.MetadataResolver = &stubMetadataResolver{flattenErr: errors.New("es down")}
	if cat := MetadataCatalogFor(ctx, broken); !cat.Empty() {
		t.Errorf("keys = %v, want empty when the index is unreadable", cat.Keys)
	}

	// Dataset carries metadata rows but no usable field.
	blank := base
	blank.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{
		"_version": {"2": {"d1"}}, // hidden by the blacklist
		"empty":    {},            // no value to match
	}}
	if cat := MetadataCatalogFor(ctx, blank); !cat.Empty() {
		t.Errorf("keys = %v, want empty when no field is usable", cat.Keys)
	}

	// No bound datasets at all.
	unbound, _ := newTestSearchDeps(&stubRetriever{})
	unbound.KbIDs = nil
	unbound.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{"title": {"A": {"d1"}}}}
	if cat := MetadataCatalogFor(ctx, unbound); !cat.Empty() {
		t.Errorf("keys = %v, want empty without datasets", cat.Keys)
	}
}

// TestActiveToolSpecsRendersCatalogKeyEnum pins the payoff: the advertised `key` enum is
// the dataset's real fields, so a model can name one. Before this it was ["title"] and no
// other field could be asked for however plainly the question named it.
func TestActiveToolSpecsRendersCatalogKeyEnum(t *testing.T) {
	cat := &MetadataCatalog{
		Keys:    []string{"author", "title"},
		Samples: map[string][]MetadataSample{"author": {{Value: "Alice", Docs: 1}}},
	}
	ts := &Toolset{ThinkingMode: "high", MetadataFields: cat}
	spec, ok := findSpec(ts.ActiveToolSpecs(), "metadata_search")
	if !ok {
		t.Fatal("metadata_search missing from the high-mode surface")
	}

	if got := strings.Join(metadataKeyEnum(spec), ","); got != "author,title" {
		t.Errorf("key enum = %q, want the catalog's fields", got)
	}
	desc := spec.Function.Description
	for _, want := range []string{"WHEN TO CALL", "DO NOT CALL", "ARGUMENTS", "OUTPUT", "IF IT FAILS", "AVAILABLE METADATA"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description missing %q:\n%s", want, desc)
		}
	}
	// The key parameter's description is dataset-independent — it points at AVAILABLE METADATA
	// and names no field, so a dataset's field list can never leak into another session's view
	// of the tool; the fields themselves reach the model through the enum asserted above and
	// through the seed's block.
	const wantKeyDesc = "one of this dataset's metadata fields (see AVAILABLE METADATA)"
	if got, _ := metadataKeyParam(spec)["description"].(string); got != wantKeyDesc {
		t.Errorf("key description = %q, want %q", got, wantKeyDesc)
	}
	if n := utf8.RuneCountInString(desc); n > maxToolDescriptionRunes {
		t.Errorf("description = %d runes, cap = %d", n, maxToolDescriptionRunes)
	}
}

// TestActiveToolSpecsKeepsStaticSpecWithoutCatalog pins the "no metadata = no field
// advertised" contract: without a catalog the shipped spec is returned untouched — same
// struct — and it names NO field, so a model cannot be told `title` is filterable on a
// dataset that never said so. That is also how an empty enum (which some providers reject)
// is avoided.
func TestActiveToolSpecsKeepsStaticSpecWithoutCatalog(t *testing.T) {
	for _, cat := range []*MetadataCatalog{nil, &MetadataCatalog{}} {
		ts := &Toolset{ThinkingMode: "high", MetadataFields: cat}
		spec, ok := findSpec(ts.ActiveToolSpecs(), "metadata_search")
		if !ok {
			t.Fatal("metadata_search missing from the high-mode surface")
		}
		if !reflect.DeepEqual(spec, ToolMap["metadata_search"]) {
			t.Errorf("catalog %v: an empty catalog must leave the shipped spec untouched", cat)
		}
		if got := metadataKeyEnum(spec); len(got) != 0 {
			t.Errorf("key enum = %v, want NO advertised field without a catalog", got)
		}
		key, _ := metadataKeyParam(spec)["enum"]
		if key != nil {
			t.Errorf("key enum key = %v, want it absent (an empty enum is rejected by some providers)", key)
		}
		if strings.Contains(spec.Function.Description, "'title'") {
			t.Errorf("the shipped description still names a field:\n%s", spec.Function.Description)
		}
	}
}

// metadataKeyParam reads the metadata_search `key` parameter object off a spec.
func metadataKeyParam(spec ToolSpec) map[string]any {
	props, _ := spec.Function.Parameters["properties"].(map[string]any)
	filters, _ := props["filters"].(map[string]any)
	items, _ := filters["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	key, _ := itemProps["key"].(map[string]any)
	return key
}

// stubDeclaredMetadata is a scripted DeclaredMetadataResolver.
type stubDeclaredMetadata struct {
	defs []common.MetadataFieldDef
	err  error
}

func (s *stubDeclaredMetadata) DeclaredMetadataFields(context.Context, []string) ([]common.MetadataFieldDef, error) {
	return s.defs, s.err
}

// TestMetadataCatalogForIncludesDeclaredFields pins the declarative half: a dataset that
// declares its fields is described with what each field MEANS and which values it accepts,
// which is what the metadata index alone cannot supply. It also pins that the blacklist
// applies to the declarative source too.
func TestMetadataCatalogForIncludesDeclaredFields(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{
		"author": {"Alice": {"d1", "d2"}},
	}}
	deps.DeclaredMetadata = &stubDeclaredMetadata{defs: []common.MetadataFieldDef{
		{Key: "author", Type: "string", Description: "who wrote it"},
		{Key: "doc_type", Type: "string", Description: "kind of document", Enum: []string{"report", "paper"}},
		{Key: "question_id", Description: "benchmark label"}, // blacklisted
	}}

	cat := MetadataCatalogFor(context.Background(), deps)
	if got := strings.Join(cat.Keys, ","); got != "author,doc_type" {
		t.Fatalf("keys = %q, want the declared fields (minus the blacklist), sorted", got)
	}
	if cat.Fields["author"].Description != "who wrote it" {
		t.Errorf("declared definition lost: %+v", cat.Fields["author"])
	}

	render := cat.Render()
	for _, want := range []string{
		"author — who wrote it",
		"doc_type — kind of document",
		"one of: report / paper",
		`values seen: "Alice" (2 doc(s))`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("render missing %q:\n%s", want, render)
		}
	}
	if strings.Contains(render, "question_id") {
		t.Errorf("the blacklist must cover the declarative source too:\n%s", render)
	}
}

// TestMetadataCatalogForOffersDeclaredFieldWithoutIndexedValues pins the case the
// observational source cannot see at all: a field declared but not yet indexed is still a
// legitimate filter, so it is offered (with its description) and advertised in the schema.
func TestMetadataCatalogForOffersDeclaredFieldWithoutIndexedValues(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{}}
	deps.DeclaredMetadata = &stubDeclaredMetadata{defs: []common.MetadataFieldDef{
		{Key: "doc_type", Description: "kind of document", Enum: []string{"report"}},
	}}

	cat := MetadataCatalogFor(context.Background(), deps)
	if got := strings.Join(cat.Keys, ","); got != "doc_type" {
		t.Fatalf("keys = %q, want the declared field even with no indexed value", got)
	}
	if len(cat.Samples["doc_type"]) != 0 {
		t.Errorf("samples = %v, want none: the index carries no value yet", cat.Samples["doc_type"])
	}

	ts := &Toolset{ThinkingMode: "high", MetadataFields: &cat}
	spec, ok := findSpec(ts.ActiveToolSpecs(), "metadata_search")
	if !ok {
		t.Fatal("metadata_search missing from the high-mode surface")
	}
	if got := strings.Join(metadataKeyEnum(spec), ","); got != "doc_type" {
		t.Errorf("key enum = %q, want the declared field advertised", got)
	}
}

// TestMetadataCatalogForDegradesWhenDeclaredReadFails pins the independence of the two
// halves: an unreadable parser_config leaves the observational source untouched, and vice
// versa (see TestMetadataCatalogForDegradesToEmpty for the index failure).
func TestMetadataCatalogForDegradesWhenDeclaredReadFails(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{metas: common.MetaData{"title": {"A": {"d1"}}}}
	deps.DeclaredMetadata = &stubDeclaredMetadata{err: errors.New("kb row unreadable")}

	cat := MetadataCatalogFor(context.Background(), deps)
	if got := strings.Join(cat.Keys, ","); got != "title" {
		t.Errorf("keys = %q, want the observational half alone", got)
	}
}

// metadataKeyEnum reads the metadata_search `key` enum off a rendered spec.
func metadataKeyEnum(spec ToolSpec) []string {
	props, _ := spec.Function.Parameters["properties"].(map[string]any)
	filters, _ := props["filters"].(map[string]any)
	items, _ := filters["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	key, _ := itemProps["key"].(map[string]any)
	out, _ := key["enum"].([]string)
	return out
}

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

// blockingExpander stands in for a compiled-expansion backend that never answers — the dead
// Elasticsearch behind a 68-second search_chunks call (see compiledExpansionTimeout).
type blockingExpander struct{ entered chan struct{} }

func (b *blockingExpander) Expand(ctx context.Context, _ *Kbinfos, _, _ string, _ []string) error {
	select {
	case <-b.entered:
	default:
		close(b.entered)
	}
	<-ctx.Done()
	return ctx.Err()
}

// TestAnEnrichmentCannotEatTheSearch pins the wall on the compiled expansion.
//
// Measured 2026-09-20 (三国/关羽): `[Hybrid search] Compiled expansion enabled` at 22:18:30, then nine
// `Elasticsearch query failed` warnings at 22:19:32 — 68 seconds inside a step that only ENRICHES a
// result the leg already holds. The round spent a third of the question there, and the session's own
// answer call lost its race with the clock right after.
//
// The enrichment is a seam (SearchDeps.Expand), so this needs no search backend at all: what is
// pinned is that a stuck enrichment cannot hold the leg, and that the leg's own chunks still come
// back when it stalls.
func TestAnEnrichmentCannotEatTheSearch(t *testing.T) {
	prev := compiledExpansionTimeout
	compiledExpansionTimeout = 50 * time.Millisecond
	defer func() { compiledExpansionTimeout = prev }()

	exp := &blockingExpander{entered: make(chan struct{})}
	r := &stubRetriever{chunks: []map[string]any{{"chunk_id": "c1", "content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	deps.Expand = exp

	started := time.Now()
	got, _ := HybridSearch(context.Background(), deps, SearchParams{Question: "q", UseCompiled: true})
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("the leg took %.1fs with a stuck enrichment, want it bounded by %v",
			elapsed.Seconds(), compiledExpansionTimeout)
	}
	if len(got) == 0 {
		t.Error("the leg returned nothing: its own chunks are the point, the enrichment is not")
	}
	select {
	case <-exp.entered:
	default:
		t.Error("the expander never ran, so nothing was bounded")
	}
}

// flakyRetriever fails while the DENSE leg is on, the way a dead embedding service does, and answers
// when the dense leg is off.
type flakyRetriever struct {
	mu     sync.Mutex
	calls  []RetrieveRequest
	chunks []map[string]any
}

// keywordOnlyLeg reports whether a request asks for the keyword leg ALONE. In this API that is
// KeywordsSimilarityWeight 1.0 (see RetrieveRequest: 0.0 vector-only, 0.7 hybrid, 1.0 keyword-only) —
// the dense leg is off, so a keyword-only caller must not need an embedder.
func keywordOnlyLeg(req RetrieveRequest) bool {
	return req.KeywordsSimilarityWeight != nil && *req.KeywordsSimilarityWeight >= 1.0
}

func (f *flakyRetriever) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	if !keywordOnlyLeg(req) {
		return nil, errors.New("GetVector failed: failed to send request")
	}
	return f.chunks, nil
}

// TestADeadEmbedderCostsTheLegItsDenseHalfOnly pins the degradation on the retry path.
//
// The retry used to repeat the same request, so when the embedder was the thing that was down both
// attempts failed and the leg returned NOTHING — discarding the passages its keyword half had already
// matched. Measured 2026-09-20 (三国/关羽): `GetVector failed: failed to send request` cost one
// search_chunks call 56 seconds inside a 75s tool wall, the retry died on `context deadline exceeded`,
// and the round had read 16 passages when its clock ran out — the salvaged answer could name one member
// of sixteen. A dense leg that cannot be computed must cost the leg its dense half, not its results.
func TestADeadEmbedderCostsTheLegItsDenseHalfOnly(t *testing.T) {
	r := &flakyRetriever{chunks: []map[string]any{{"chunk_id": "c1", "content": "hit"}}}
	deps, _ := newTestSearchDeps(r)
	// An embedder IS configured: without one the dense leg is off before the first attempt and the
	// failure this test is about cannot happen.
	deps.HasEmbedder = true
	got, _ := HybridSearch(context.Background(), deps, SearchParams{Question: "q", KbIDs: []string{"kb1"}})
	if len(got) == 0 {
		t.Fatal("the leg discarded its keyword hits along with the vector failure")
	}
	if len(r.calls) != 2 {
		t.Fatalf("attempts = %d, want the failure retried once", len(r.calls))
	}
	if keywordOnlyLeg(r.calls[0]) || !keywordOnlyLeg(r.calls[1]) {
		t.Errorf("attempts used the keyword-alone weight %v/%v, want the retry to go keyword-only",
			r.calls[0].KeywordsSimilarityWeight, r.calls[1].KeywordsSimilarityWeight)
	}
}

// TestOneAttemptCannotEatTheLegsClock pins the bound each attempt runs under: the retry needs room, and
// the leg's clock belongs to the question.
func TestOneAttemptCannotEatTheLegsClock(t *testing.T) {
	if got := searchAttemptBudget(context.Background()); got.Seconds() != searchAttemptMaxS {
		t.Errorf("unbounded context gave %.0fs, want the cap %.0fs", got.Seconds(), searchAttemptMaxS)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if got := searchAttemptBudget(ctx); got.Seconds() > searchAttemptMaxS {
		t.Errorf("a 60s leg gave one attempt %.0fs, want at most %.0fs", got.Seconds(), searchAttemptMaxS)
	}
	// Half of a small clock, but the floor wins when the half is too small to be worth an attempt —
	// and the attempt can never outlive the caller either way (context.WithTimeout takes the earlier
	// deadline of the two).
	tight, cancelTight := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancelTight()
	if got := searchAttemptBudget(tight); got.Seconds() != searchAttemptMinS {
		t.Errorf("a 6s leg gave one attempt %.0fs, want the floor %.0fs", got.Seconds(), searchAttemptMinS)
	}
	spent, cancelSpent := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelSpent()
	if got := searchAttemptBudget(spent); got.Seconds() != searchAttemptMinS {
		t.Errorf("an almost-spent leg gave one attempt %.0fs, want the floor %.0fs", got.Seconds(), searchAttemptMinS)
	}
}
