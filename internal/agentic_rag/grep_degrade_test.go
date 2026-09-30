/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package agentic_rag

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

// degradeGrepService refuses the pushdown whenever refuse says so — the shape
// the adapter produces for a pattern the engine will not compile.
type degradeGrepService struct {
	refuse func(pattern string) error
	chunks []runtime.RetrievalChunk
	calls  []string
}

func (s *degradeGrepService) Grep(_ context.Context, req runtime.GrepRequest) ([]runtime.RetrievalChunk, error) {
	s.calls = append(s.calls, req.Pattern)
	if s.refuse != nil {
		if err := s.refuse(req.Pattern); err != nil {
			return nil, err
		}
	}
	return s.chunks, nil
}

// degradeBm25Service records the queries the lexical prefilter ran and returns
// one chunk that the caller's regexp matches and one it does not, so a test can
// prove the local filter still decides what ships.
type degradeBm25Service struct {
	queries []string
	chunks  []runtime.RetrievalChunk
}

func (s *degradeBm25Service) SearchBm25(_ context.Context, req runtime.Bm25Request) ([]runtime.RetrievalChunk, error) {
	s.queries = req.Queries
	return s.chunks, nil
}

func pushdownRefusal(pattern string) error {
	return fmt.Errorf("grep: %w — elasticsearch regexp error: Determinizing %q would require more than 10000 states.%s",
		runtime.ErrRegexpPushdown, pattern, regexpPushdownHint(pattern))
}

func TestSplitTopLevelAlternation(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"two branches", `A.*B|C.*D`, []string{`A.*B`, `C.*D`}},
		{"escaped pipe is a literal", `A\|B`, nil},
		{"nested alternation stays whole", `(A|B).*C`, nil},
		{"class content is not a split point", `[A|B]+C`, nil},
		{"empty branch matches everything: refuse to split", `A||B`, nil},
		{"no alternation", `A.*B`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTopLevelAlternation(tc.pattern, grepDegradeMaxSplits)
			if len(got) != len(tc.want) {
				t.Fatalf("branches = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("branch %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
	// Too fragmented to be worth splitting: the lexical prefilter is better.
	many := strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}, "|")
	if got := splitTopLevelAlternation(many, grepDegradeMaxSplits); got != nil {
		t.Fatalf("over-fragmented pattern must not split, got %v", got)
	}
}

func TestLuceneSafeRegexp(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    string
		changed bool
	}{
		{"word boundary is dropped", `\bBoston\b`, `Boston`, true},
		{"anchor escape is rewritten", `\Afoo\z`, `^foo$`, true},
		{"named group becomes plain", `(?P<year>19\d\d)`, `(19\d\d)`, true},
		{"lookahead is not emulated", `foo(?=bar)`, `foo(?=bar)`, false},
		{"inline flags are not dropped", `(?i)foo`, `(?i)foo`, false},
		{"already safe", `A.*B`, `A.*B`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := luceneSafeRegexp(tc.pattern)
			if got != tc.want || changed != tc.changed {
				t.Fatalf("luceneSafeRegexp(%q) = (%q, %v), want (%q, %v)", tc.pattern, got, changed, tc.want, tc.changed)
			}
		})
	}
}

func TestRegexpLiteralTerms(t *testing.T) {
	got := regexpLiteralTerms(`Mapeza.*Platinum|(Zimbabwe|Harare).*coach`, grepDegradeMaxQueries)
	want := []string{"mapeza platinum", "zimbabwe harare coach"}
	if len(got) != len(want) {
		t.Fatalf("queries = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("query %d = %q, want %q", i, got[i], want[i])
		}
	}
	// Single-character leftovers (an escape removed from \b) are noise.
	if terms := regexpLiteralTokens(`\bA\b`); len(terms) != 0 {
		t.Fatalf("one-rune tokens must be dropped, got %v", terms)
	}
}

// TestGrepChunksSplitsExplodingAlternation covers tier 2: the engine refuses the
// combined pattern but accepts each branch, so the tool returns the exact union
// of the branches with NO coverage caveat.
func TestGrepChunksSplitsExplodingAlternation(t *testing.T) {
	svc := &degradeGrepService{
		refuse: func(pattern string) error {
			if strings.Contains(pattern, "|") {
				return pushdownRefusal(pattern)
			}
			return nil
		},
		chunks: []runtime.RetrievalChunk{{ID: "c1", Content: "Boston is home", DocumentID: "d1"}},
	}
	runtime.SetGrepService(svc)
	defer runtime.SetGrepService(nil)

	out, err := NewGrepChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"query":"Boston.*home|Cambridge.*home"}`)
	if err != nil {
		t.Fatalf("split must recover, got error: %v", err)
	}
	if strings.Contains(out, toolErrorMarker) {
		t.Errorf("an exact split needs no caveat, got %.200q", out)
	}
	if !strings.Contains(out, `chunk_id="c1"`) {
		t.Errorf("branch hits must ship, got %.200q", out)
	}
	if len(svc.calls) != 3 { // combined, then branch A, branch B
		t.Fatalf("expected the combined attempt plus one per branch, got %v", svc.calls)
	}
}

// TestGrepChunksSanitizesLuceneUnsafePattern covers tier 1: a `\b` pattern (the
// engine's "invalid character class 98") is rewritten and the SAME pushdown is
// retried, with no caveat because the rewrite only widens the prefilter.
func TestGrepChunksSanitizesLuceneUnsafePattern(t *testing.T) {
	svc := &degradeGrepService{
		refuse: func(pattern string) error {
			if strings.Contains(pattern, `\b`) {
				return pushdownRefusal(pattern)
			}
			return nil
		},
		chunks: []runtime.RetrievalChunk{{ID: "c1", Content: "Boston is home", DocumentID: "d1"}},
	}
	runtime.SetGrepService(svc)
	defer runtime.SetGrepService(nil)

	// The pattern must still have at least two runes after sanitizing: `\b`-only
	// text would leave nothing to search.
	out, err := NewGrepChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"query":"\bBoston\b"}`)
	if err != nil {
		t.Fatalf("sanitized retry must recover, got error: %v", err)
	}
	if strings.Contains(out, toolErrorMarker) {
		t.Errorf("a superset prefilter needs no caveat, got %.200q", out)
	}
	if got := svc.calls[len(svc.calls)-1]; strings.Contains(got, `\b`) {
		t.Errorf("retry pattern must be sanitized, got %q", got)
	}
}

// TestGrepChunksDegradesToLexicalPrefilter covers tier 3: a pattern the engine
// keeps refusing becomes a BM25 prefilter whose hits are re-checked locally, and
// the result carries a warn (not error) notice stating the coverage caveat.
func TestGrepChunksDegradesToLexicalPrefilter(t *testing.T) {
	svc := &degradeGrepService{
		refuse: func(pattern string) error { return pushdownRefusal(pattern) },
	}
	bm25 := &degradeBm25Service{
		chunks: []runtime.RetrievalChunk{
			{ID: "c1", Content: "Maple Park was renamed", DocumentID: "d1"}, // matches the regexp
			{ID: "c2", Content: "unrelated prose", DocumentID: "d2"},        // local filter drops it
		},
	}
	runtime.SetGrepService(svc)
	runtime.SetBm25Service(bm25)
	defer func() {
		runtime.SetGrepService(nil)
		runtime.SetBm25Service(nil)
	}()

	out, err := NewGrepChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"query":"Maple.*Park"}`)
	if err != nil {
		t.Fatalf("lexical prefilter must recover, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="grep_chunks" severity="warn">`) ||
		!strings.Contains(out, "APPROXIMATE") {
		t.Errorf("expected a warn notice about approximate coverage, got %.300q", out)
	}
	if !strings.Contains(out, `chunk_id="c1"`) {
		t.Errorf("matching candidate must ship, got %.300q", out)
	}
	if strings.Contains(out, `chunk_id="c2"`) {
		t.Errorf("the local regexp filter must drop non-matching candidates, got %.300q", out)
	}
	if len(bm25.queries) != 1 || bm25.queries[0] != "maple park" {
		t.Errorf("prefilter queries = %v, want [maple park]", bm25.queries)
	}
}

// TestGrepChunksUnrecoverablePushdownStaysAnError asserts the ladder still ends
// in a real error when nothing recovers: the model must get the "cheaper
// pattern" hint rather than a plausible-looking empty result.
func TestGrepChunksUnrecoverablePushdownStaysAnError(t *testing.T) {
	svc := &degradeGrepService{
		refuse: func(pattern string) error { return pushdownRefusal(pattern) },
	}
	runtime.SetGrepService(svc)
	runtime.SetBm25Service(nil)
	defer runtime.SetGrepService(nil)

	out, err := NewGrepChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"query":"A.*B"}`)
	if err != nil {
		t.Fatalf("failures become results, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="grep_chunks" severity="error">`) ||
		!strings.Contains(out, "CHEAPER pattern") {
		t.Errorf("expected an error tool_error with the retry hint, got %.300q", out)
	}
}
