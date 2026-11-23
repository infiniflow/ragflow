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
	"fmt"
	"strings"
	"testing"
)

const citationFixture = "## Candidate Matrix\n" +
	"### Sub-question 1: who\n" +
	"- Tested: Bob - clues [1] supported (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"x\")\n" +
	"- Eliminated: Eve - clue [2] contradicts (doc: b.md, doc_id: d2, chunk_id: ccc222, snippet: \"y\")\n" +
	"- Retained: Bob (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"x again\")\n" +
	"## Reasoning Chain\n" +
	"- Clue: signed (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"z\")\n" +
	"- Clue: web hit (source: web, url: http://x, title: T, snippet: \"w\")\n" +
	"Final Answer: **1897**"

func TestExtractCitedChunkIDs(t *testing.T) {
	got := ExtractCitedChunkIDs(citationFixture)
	want := []string{"ccc111", "ccc222"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (first-appearance order, deduplicated)", got, want)
		}
	}
}

func TestExtractCitedChunkIDsEmpty(t *testing.T) {
	for _, in := range []string{"", "no citations here", "- Retained: Bob (source: web, url: http://x)"} {
		if got := ExtractCitedChunkIDs(in); got != nil {
			t.Errorf("ExtractCitedChunkIDs(%q) = %v, want nil", in, got)
		}
	}
}

func TestRewriteProvenanceReplacesTupleInPlace(t *testing.T) {
	in := "- `Tested: 华雄 (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"x\")`"
	got := RewriteProvenance(in, map[string]int{"ccc111": 0})
	// The whole tuple goes, and the marker lands where it stood.
	if want := "- `Tested: 华雄 [ID:0]`"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteProvenanceSharedChunkAndIdempotence(t *testing.T) {
	in := "- Tested: a (chunk_id: c1, snippet: \"x\")\n- Tested: b (chunk_id: c1, snippet: \"y\")"
	idx := map[string]int{"c1": 0}
	got := RewriteProvenance(in, idx)
	if n := strings.Count(got, "[ID:0]"); n != 2 {
		t.Fatalf("marker count = %d, want 2: %s", n, got)
	}
	if again := RewriteProvenance(got, idx); again != got {
		t.Fatalf("not idempotent:\nfirst:  %s\nsecond: %s", got, again)
	}
}

func TestRewriteProvenanceUnresolvableStaysVisible(t *testing.T) {
	in := "- Tested: Bob (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"x\")"
	if got := RewriteProvenance(in, map[string]int{"other999": 0}); got != in {
		t.Fatalf("an unresolvable tuple changed: %s", got)
	}
	if got := RewriteProvenance(in, nil); got != in {
		t.Fatalf("a nil index rewrote the text: %s", got)
	}
}

func TestRewriteProvenanceBareField(t *testing.T) {
	// An answer that never followed the tuple contract still gets markers.
	if got, want := RewriteProvenance("答案 chunk_id: c1", map[string]int{"c1": 2}), "答案 [ID:2]"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteProvenanceWebTuple(t *testing.T) {
	in := "事实 (source: web, url: https://e/x, title: T, snippet: \"y\")"
	if got, want := RewriteProvenance(in, map[string]int{"https://e/x": 1}), "事实 [ID:1]"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteProvenanceSnippetWithParen(t *testing.T) {
	// A parenthesised snippet must not end the tuple early.
	in := "答案 (doc: A, doc_id: d1, chunk_id: c1, snippet: \"see (this) bit\")"
	if got, want := RewriteProvenance(in, map[string]int{"c1": 0}), "答案 [ID:0]"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractCitedWebSources(t *testing.T) {
	in := "a (source: web, url: https://e/x, title: T1, snippet: \"S1\") " +
		"b (source: web, url: https://e/y, title: T2, snippet: \"S2\") " +
		"again (source: web, url: https://e/x, title: T1, snippet: \"S1\")"
	got := ExtractCitedWebSources(in)
	if len(got) != 2 {
		t.Fatalf("sources = %+v, want 2 (deduped by url)", got)
	}
	if got[0].URL != "https://e/x" || got[0].Title != "T1" || got[0].Snippet != "S1" {
		t.Fatalf("first source = %+v", got[0])
	}
	if got[1].URL != "https://e/y" {
		t.Fatalf("second source = %+v", got[1])
	}
	// A corpus parenthetical is not a web source.
	if got := ExtractCitedWebSources("(doc: a, doc_id: d1, chunk_id: c1, snippet: \"x\")"); len(got) != 0 {
		t.Fatalf("corpus tuple parsed as web: %+v", got)
	}
}

// CitedIDsFromMarkers maps [ID:n] handles to chunk ids in first-citation order,
// deduplicated, dropping any handle the registry never published.
func TestCitedIDsFromMarkers(t *testing.T) {
	r := NewEvidenceRegistry()
	r.Stamp("c1") // [ID:0]
	r.Stamp("c2") // [ID:1]
	answer := "fact A [ID:1]\nfact B [ID:0]\nfact C [ID:1]\nbogus [ID:9]"

	got := CitedIDsFromMarkers(answer, r)
	want := []string{"c2", "c1"}
	if len(got) != len(want) {
		t.Fatalf("CitedIDsFromMarkers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CitedIDsFromMarkers = %v, want %v (first-citation order, deduped)", got, want)
		}
	}
}

func TestCitedIDsFromMarkersAcceptsBareNumericMarkers(t *testing.T) {
	r := NewEvidenceRegistry()
	r.Stamp("c1") // [ID:0]
	r.Stamp("c2") // [ID:1]

	got := CitedIDsFromMarkers("fact A [1]\nfact B [0]\nignored [9]", r)
	want := []string{"c2", "c1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("CitedIDsFromMarkers = %v, want %v", got, want)
	}
}

func TestCitedIDsFromMarkersNoSignal(t *testing.T) {
	if got := CitedIDsFromMarkers("plain [ID:0]", nil); got != nil {
		t.Errorf("nil registry returned %v, want nil", got)
	}
	if got := CitedIDsFromMarkers("no markers here", NewEvidenceRegistry()); got != nil {
		t.Errorf("marker-free answer returned %v, want nil", got)
	}
}

// ResolveCitations compacts run-level handles into the answer's own
// first-citation order, and drops a handle naming a passage the caller will not
// publish.
func TestResolveCitationsCompactsAndDrops(t *testing.T) {
	r := NewEvidenceRegistry()
	r.Stamp("c1") // [ID:0]
	r.Stamp("c2") // [ID:1]
	r.Stamp("c3") // [ID:2]

	answer := "A [ID:2]\nB [ID:0]\nC [ID:1]"
	// c2 is not publishable (it did not resolve): its handle must be dropped.
	marked, cited := ResolveCitations(answer, r, []string{"c3", "c1"})
	if marked != "A [ID:0]\nB [ID:1]\nC " {
		t.Fatalf("marked = %q, want the compacted handles", marked)
	}
	if len(cited) != 2 || cited[0] != "c3" || cited[1] != "c1" {
		t.Fatalf("cited = %v, want [c3 c1]", cited)
	}
}

func TestResolveCitationsAcceptsBareNumericMarkers(t *testing.T) {
	r := NewEvidenceRegistry()
	r.Stamp("c1") // [ID:0]
	r.Stamp("c2") // [ID:1]

	marked, cited := ResolveCitations("A [1]\nB [0]", r, []string{"c2", "c1"})
	if marked != "A [ID:0]\nB [ID:1]" {
		t.Fatalf("marked = %q, want compacted canonical handles", marked)
	}
	if fmt.Sprint(cited) != "[c2 c1]" {
		t.Fatalf("cited = %v, want [c2 c1]", cited)
	}
}

func TestResolveCitationsPreservesUnresolvedBareNumericMarkers(t *testing.T) {
	r := NewEvidenceRegistry()
	r.Stamp("c1")

	marked, cited := ResolveCitations("Year [2024]; evidence [0]", r, []string{"c1"})
	if marked != "Year [2024]; evidence [ID:0]" {
		t.Fatalf("marked = %q, want unresolved bare number preserved", marked)
	}
	if fmt.Sprint(cited) != "[c1]" {
		t.Fatalf("cited = %v, want [c1]", cited)
	}
}

func TestResolveCitationsNoSignal(t *testing.T) {
	answer := "no handles"
	if got, cited := ResolveCitations(answer, nil, []string{"c1"}); got != answer || cited != nil {
		t.Errorf("nil registry rewrote the answer: %q %v", got, cited)
	}
	if got, cited := ResolveCitations(answer, NewEvidenceRegistry(), nil); got != answer || cited != nil {
		t.Errorf("empty publishable rewrote the answer: %q %v", got, cited)
	}
}

// A reasoning model merges several handles into ONE marker ("[ID:47, ID:115]").
// That shape matches neither this package's scanner nor the client's marker
// regex — both stop at the comma — so it survives every citation pass and
// reaches the UI as literal text. Expanding it is what makes it resolvable.
func TestExpandCommaCitations(t *testing.T) {
	if got, want := ExpandCommaCitations("A [ID:47, ID:115] B", 200), "A [ID:47][ID:115] B"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// A handle the registry never published is dropped, not left out of range.
	if got, want := ExpandCommaCitations("A [ID:47, ID:115] B", 100), "A [ID:47] B"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Three handles, loose spacing around the commas and the colons.
	if got, want := ExpandCommaCitations("[ID: 1,ID:2 ,  ID:3]", 10), "[ID:1][ID:2][ID:3]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// A single handle needs no expansion, and marker-free text is untouched.
	for _, keep := range []string{"A [ID:47] B", "no handles, just prose"} {
		if got := ExpandCommaCitations(keep, 200); got != keep {
			t.Errorf("rewrote %q into %q", keep, got)
		}
	}
	// No registry to bound against: leave the text alone.
	unbounded := "A [ID:47, ID:115] B"
	if got := ExpandCommaCitations(unbounded, 0); got != unbounded {
		t.Errorf("unbounded rewrite: %q", got)
	}
}

// The observed failure end to end: the model parked the whole answer inside
// <think> and grouped its handles, so nothing downstream resolved them. After
// expansion the handles compact onto the reference payload the client indexes.
func TestGroupedHandlesResolveAfterExpansion(t *testing.T) {
	r := NewEvidenceRegistry()
	for i := 0; i < 120; i++ {
		r.Stamp(fmt.Sprintf("c%d", i))
	}
	answer := "<think>1948: both went Truman [ID:47, ID:115]\n1972: Nixon [ID:47, ID:83]</think>"

	normalized := ExpandCommaCitations(answer, len(r.IDs()))
	cited := CitedIDsFromMarkers(normalized, r)
	want := []string{"c47", "c115", "c83"}
	if len(cited) != len(want) {
		t.Fatalf("cited = %v, want %v", cited, want)
	}
	for i := range want {
		if cited[i] != want[i] {
			t.Fatalf("cited = %v, want %v", cited, want)
		}
	}

	marked, _ := ResolveCitations(normalized, r, cited)
	// c47 -> [ID:0], c115 -> [ID:1], c83 -> [ID:2], in first-citation order.
	wantMarked := "<think>1948: both went Truman [ID:0][ID:1]\n1972: Nixon [ID:0][ID:2]</think>"
	if marked != wantMarked {
		t.Fatalf("marked = %q, want %q", marked, wantMarked)
	}
}
