// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

// mergeSourceLines is a fixed 29-line plain-text payload. The exact wording is
// the RAGFlow introduction paragraph used by the golden-parity fixtures; the
// chunk boundaries below were verified against Python's naive_merge
// (rag/nlp/__init__.py:_merge_paragraph_groups, OVER_CAP) on cl100k_base.
var mergeSourceLines = []string{
	"RAGFlow is an open-source retrieval-augmented generation engine.",
	"It ingests documents of many types and splits them into chunks.",
	"Each chunk is embedded and stored in a vector database for retrieval.",
	"The chunker honours document structure such as headings and tables.",
	"Long paragraphs are divided so each chunk fits a token budget.",
	"Overlap between chunks preserves context across boundaries.",
	"Retrieval returns the most relevant chunks for a user question.",
	"The generator then composes an answer from those chunks.",
	"Evaluation measures faithfulness and answer relevance.",
	"Users tune chunk size to balance recall and precision.",
	"Smaller chunks improve recall but increase storage cost.",
	"Larger chunks keep more context but may dilute relevance.",
	"The system supports many languages and encodings.",
	"Deep parsing extracts text from PDF images and scans.",
	"Tables are recognised and preserved as structured chunks.",
	"Images can be described and attached to neighbouring text.",
	"The API exposes dataset, document, and chat endpoints.",
	"A web UI lets users manage knowledge bases visually.",
	"Batch ingestion processes large corpora efficiently.",
	"Concurrency limits protect the embedding service.",
	"Caching avoids re-embedding unchanged content.",
	"Logging records ingestion progress and errors.",
	"Metrics help operators spot slow components.",
	"Configuration controls parsers and chunking strategies.",
	"Templates encode common ingestion pipelines.",
	"The engine scales horizontally behind a load balancer.",
	"Security isolates tenant data by document id.",
	"Auditing tracks who accessed which knowledge base.",
	"Plugins extend parsing for proprietary formats.",
}

// goMergeGroupsOracle is the Go-side reference for the hard-cap merge contract
// (方案 B): it builds text-path units ("\n" + paragraph) exactly like
// mergeByTokenSize and runs them through the mergeUnits core, so the
// component-vs-core comparison below validates the wiring (unitization,
// formatting) while the merge semantics themselves are pinned by the
// token_hardcap_test.go / token_strict_cap_test.go suites.
//
// The Python-side OVER_CAP contract this file previously mirrored is now a
// deliberate Go divergence (go_intentional): Go never lets a text chunk exceed
// the token target, whereas Python's OVER_CAP closes a group right after an
// overflowing merge (a chunk may exceed the target by one unit) and keeps an
// oversized unit whole. The hard-cap behavior is documented in token.go.
//
// Whitespace exactness is OUTSIDE this oracle's contract: every emitted chunk
// is trimmed and the sanity check compares the normalized word stream, so a
// space/newline rewrite inside a chunk is not detected here. Whitespace
// preservation is pinned by the dedicated suites instead — token_oversize_split_test.go
// (lossless concatenation) and token_overlap_test.go (no space-less boundaries).
func goMergeGroupsOracle(paragraphs []string, cap int, overlapPct float64) []string {
	units := make([]schema.ChunkDoc, 0, len(paragraphs))
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		u := "\n" + p
		units = append(units, schema.ChunkDoc{Text: u, TKNums: intPtr(tokenizeStr(u)), CKType: "text"})
	}
	merged := mergeUnits(units, cap, overlapPct, "")
	out := make([]string, 0, len(merged))
	for _, ck := range merged {
		text := removeTag(strings.TrimSpace(ck.Text))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

// TestGoMergeGroupsOracleSanity pins the oracle output invariants that the
// hard-cap merge must satisfy, so the component-vs-core comparison cannot
// silently pass while the contract regresses: every emitted chunk is within the
// token cap, and the concatenated chunk texts reconstruct the source paragraph
// stream (order + no loss).
func TestGoMergeGroupsOracleSanity(t *testing.T) {
	const cap = 8
	inputs := [][]string{
		{"word word word word word word word word word word"},                            // oversized -> hard split
		{"word", "word", "word", "word", "word", "word", "word", "word", "word", "word"}, // pack boundary
		{"", "x", "y", "z"},                 // empty + small
		{"alpha", "beta", "gamma", "delta"}, // plain small
	}
	for ci, in := range inputs {
		chunks := goMergeGroupsOracle(in, cap, 0)
		if len(chunks) == 0 {
			t.Fatalf("case %d: oracle produced no chunks", ci)
		}
		for gi, text := range chunks {
			if n := tokenizeStr(text); n > cap {
				t.Errorf("case %d chunk %d exceeds cap: tokens=%d (cap=%d)", ci, gi, n, cap)
			}
		}
		// Source preservation: the concatenated chunk texts must reproduce the
		// non-empty input paragraph stream (order + no loss). All whitespace is
		// normalized away because paragraph boundaries are joined with "\n" and
		// each emitted chunk is trimmed.
		var wantNormalized strings.Builder
		for _, p := range in {
			for _, w := range strings.Fields(p) {
				wantNormalized.WriteString(w)
			}
		}
		var gotNormalized strings.Builder
		for _, text := range chunks {
			for _, w := range strings.Fields(text) {
				gotNormalized.WriteString(w)
			}
		}
		if gotNormalized.String() != wantNormalized.String() {
			t.Errorf("case %d: chunk stream dropped or reordered source text:\n got=%q\nwant=%q",
				ci, gotNormalized.String(), wantNormalized.String())
		}
	}
}

// expectedGoChunks returns the chunk texts the hard-cap merge produces for
// mergeSourceLines at the given chunk_token_size (overlap 0).
func expectedGoChunks(t *testing.T, chunkTokenSize int) []string {
	t.Helper()
	return goMergeGroupsOracle(mergeSourceLines, chunkTokenSize, 0)
}

// invokeTokenChunker runs the real TokenChunker component on plain text and
// returns the produced chunk texts, in order.
func invokeTokenChunker(t *testing.T, text string, chunkTokenSize int, overlappedPercent float64) []string {
	t.Helper()
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("TokenChunker")
	if !ok || factory == nil {
		t.Fatalf("TokenChunker is not registered")
	}
	param := map[string]any{
		"chunk_token_size":   float64(chunkTokenSize),
		"delimiters":         []any{"\n"},
		"overlapped_percent": overlappedPercent,
	}
	input := map[string]any{
		"output_format": "text",
		"name":          "merge-parity",
		"text":          text,
	}
	comp, err := factory("TokenChunker", param)
	if err != nil {
		t.Fatalf("construct TokenChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("invoke TokenChunker: %v", err)
	}
	if msg, ok := out["_ERROR"].(string); ok && msg != "" {
		t.Fatalf("TokenChunker returned _ERROR: %s", msg)
	}
	raw, _ := out["chunks"].([]map[string]any)
	texts := make([]string, 0, len(raw))
	for _, c := range raw {
		if s, ok := c["text"].(string); ok {
			texts = append(texts, s)
		}
	}
	return texts
}

// TestTokenChunkerMergeMatchesGoOracle validates that the real TokenChunker
// component's text path reproduces the hard-cap merge contract chunk-for-chunk
// (count AND per-chunk text). The oracle goMergeGroupsOracle runs the same
// unitization as the component ("\n" + paragraph) through the mergeUnits core,
// so this test guards the component wiring; the merge semantics themselves are
// pinned by token_hardcap_test.go / token_strict_cap_test.go.
//
// Go intentionally diverges from Python's OVER_CAP here (go_intentional): Go
// never lets a text chunk exceed the target, while Python closes a group right
// after an overflowing merge and keeps oversized units whole.
func TestTokenChunkerMergeMatchesGoOracle(t *testing.T) {
	text := strings.Join(mergeSourceLines, "\n")
	for _, cap := range []int{32, 128} {
		want := expectedGoChunks(t, cap)
		got := invokeTokenChunker(t, text, cap, 0)
		if len(got) != len(want) {
			t.Fatalf("chunk_token_size=%d: chunk count go=%d, oracle=%d", cap, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("chunk_token_size=%d chunk[%d] mismatch:\n got=%q\nwant=%q", cap, i, got[i], want[i])
			}
		}
	}
}

// goMergeWithOverlapOracle mirrors the text path of the hard-cap merge with
// overlap: it builds text-path units ("\n" + paragraph) and runs them through
// the mergeUnits core (scaled overlap threshold + unconditional overlap prefix
// trimmed to fit the hard cap). It is the Go-side oracle for the overlap
// parity tests below.
func goMergeWithOverlapOracle(paragraphs []string, chunkTokenSize int, overlappedPercent float64) []string {
	units := make([]schema.ChunkDoc, 0, len(paragraphs))
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		u := "\n" + p
		units = append(units, schema.ChunkDoc{Text: u, TKNums: intPtr(tokenizeStr(u)), CKType: "text"})
	}
	merged := mergeUnits(units, chunkTokenSize, overlappedPercent, "")
	out := make([]string, 0, len(merged))
	for _, ck := range merged {
		// Mirror production token.go: removeTag first, then TrimSpace, so
		// whitespace between visible text and a trailing position tag survives
		// exactly as the Go TokenChunker emits it.
		text := removeTag(strings.TrimSpace(ck.Text))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

// TestTokenChunkerOverlapMatchesPython is the overlap=20 counterpart of
// TestTokenChunkerMergeMatchesPython (#17948). It asserts the Go TokenChunker
// output equals the Python token_chunker merge contract (unconditional overlap
// prefix, scaled threshold) for the same 29-line payload.
//
// Like #17948 it compares against a Go-port oracle of the Python algorithm
// (no live Python shell-out), so it stays deterministic and does not depend on
// the offline tiktoken cache — the token counts use the real Go tokenizer,
// identical on both sides of the comparison. This guards the NEW behaviour
// introduced by the unified algorithm (decision #4: unconditional overlap
// prefix) against Go/Python drift at overlap>0, which #17948 (overlap=0) does
// not exercise.
// TestTokenChunkerOverlapStripsTagsTrimOrder guards the final-strip ORDER for
// tag-bearing text: production (token.go) does removeTag(TrimSpace(text)), so
// whitespace that sits BETWEEN visible text and a trailing position tag is
// PRESERVED (TrimSpace cannot see past the tag). The oracle goMergeWithOverlap
// must use the SAME order, otherwise the parity test would assert a text that
// the production code never emits.
//
// Input: every paragraph ends with "<space><space>@@page...##", so each merged
// chunk's tail has two spaces before its closing tag. With the production
// order the trailing spaces survive; with the swapped order they are trimmed.
func TestTokenChunkerOverlapStripsTagsTrimOrder(t *testing.T) {
	tagged := []string{
		"alpha one  @@1\t2\t3\t4##",
		"beta two  @@5\t6\t7\t8##",
		"gamma three  @@9\t1\t2\t3##",
	}
	text := strings.Join(tagged, "\n")
	const overlap = 20.0
	for _, cap := range []int{8, 32} {
		want := goMergeWithOverlapOracle(tagged, cap, overlap)
		got := invokeTokenChunker(t, text, cap, overlap)
		if len(got) != len(want) {
			t.Fatalf("chunk_token_size=%d overlap=%v: chunk count go=%d, oracle=%d", cap, overlap, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("chunk_token_size=%d overlap=%v chunk[%d] mismatch:\n got=%q\nwant=%q", cap, overlap, i, got[i], want[i])
			}
		}
	}
}

func TestTokenChunkerOverlapMatchesGoOracle(t *testing.T) {
	text := strings.Join(mergeSourceLines, "\n")
	const overlap = 20.0
	for _, cap := range []int{32, 64, 128} {
		want := goMergeWithOverlapOracle(mergeSourceLines, cap, overlap)
		got := invokeTokenChunker(t, text, cap, overlap)
		if len(got) != len(want) {
			t.Fatalf("chunk_token_size=%d overlap=%v: chunk count go=%d, oracle=%d", cap, overlap, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("chunk_token_size=%d overlap=%v chunk[%d] mismatch:\n got=%q\nwant=%q", cap, overlap, i, got[i], want[i])
			}
		}
	}
}
