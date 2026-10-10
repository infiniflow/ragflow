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

func TestInsertCitationMarkers(t *testing.T) {
	ids := []string{"ccc111", "ccc222"}
	got := InsertCitationMarkers(citationFixture, ids)
	// ccc111 appears three times: every occurrence gets the SAME marker (its
	// position in ids), and ccc222 gets [ID:1].
	if n := strings.Count(got, " [ID:0]"); n != 3 {
		t.Errorf("ccc111 marker count = %d, want 3", n)
	}
	if n := strings.Count(got, " [ID:1]"); n != 1 {
		t.Errorf("ccc222 marker count = %d, want 1", n)
	}
	if !strings.Contains(got, "chunk_id: ccc111 [ID:0]") {
		t.Errorf("marker not appended right after the chunk_id field: %s", got)
	}
	// The original text must be preserved apart from the markers.
	if !strings.Contains(got, "Final Answer: **1897**") ||
		strings.Count(got, "chunk_id:") != 4 {
		t.Error("citation rewrite damaged the deliverable text")
	}
	// Idempotent: a second pass must not stack markers.
	if again := InsertCitationMarkers(got, ids); again != got {
		t.Errorf("insertion not idempotent:\nfirst:  %s\nsecond: %s", got, again)
	}
}

// The FOS wraps every matrix/chain line in backticks, and the frontend's
// citation pass skips text inside <code> nodes — the marker must land AFTER
// the line's closing backtick or it renders as literal text.
func TestInsertCitationMarkersOutsideCodeSpan(t *testing.T) {
	in := "- `Tested: 华雄 (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"x\")`\n" +
		"- `Retained: 华雄 - confirmed`"
	got := InsertCitationMarkers(in, []string{"ccc111"})
	lines := strings.Split(got, "\n")
	if !strings.HasSuffix(lines[0], ")` [ID:0]") {
		t.Errorf("marker not placed outside the code span: %s", lines[0])
	}
	if strings.Contains(lines[0][:strings.LastIndex(lines[0], "`")], "[ID:0]") {
		t.Errorf("marker leaked inside the code span: %s", lines[0])
	}
	// The un-cited second line must stay untouched.
	if lines[1] != "- `Retained: 华雄 - confirmed`" {
		t.Errorf("unrelated line modified: %s", lines[1])
	}
}

// Two lines citing the SAME chunk both carry that chunk's marker.
func TestInsertCitationMarkersSharedChunk(t *testing.T) {
	shared := "479d61f35f7aaadd"
	in := "- `Tested: 孟坦 (chunk_id: " + shared + ", snippet: \"a\")`\n" +
		"- `Tested: 韩福 (chunk_id: " + shared + ", snippet: \"b\")`"
	got := InsertCitationMarkers(in, []string{shared})
	if n := strings.Count(got, " [ID:0]`") + strings.Count(got, "` [ID:0]"); n < 2 {
		t.Errorf("shared chunk marked on %d lines, want 2: %s", n, got)
	}
}

func TestInsertCitationMarkersUnresolvableStaysUnmarked(t *testing.T) {
	in := "- Tested: Bob (doc: a.md, doc_id: d1, chunk_id: ccc111, snippet: \"x\")"
	got := InsertCitationMarkers(in, []string{"other999"})
	if got != in {
		t.Errorf("unresolvable id got marked: %s", got)
	}
	// A failed load must not silently produce markers pointing at wrong
	// indices either.
	if got := InsertCitationMarkers(in, nil); got != in {
		t.Errorf("nil id list rewrote the text: %s", got)
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
