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

// pythonMergeGroups mirrors rag/nlp/__init__.py:_merge_paragraph_groups
// (MergeStrategy.OVER_CAP) — the Python reference chunker merge. It
// accumulates a RUNNING SUM of per-paragraph token counts (size(p)) and merges
// the next paragraph when cur_t + size(p) <= cap. The chunker must reproduce
// this grouping exactly; see the regression this guards against below.
func pythonMergeGroups(paragraphs []string, cap int) [][]string {
	groups := [][]string{}
	cur, curT := []string{}, 0
	for _, p := range paragraphs {
		// Python's naive_merge builds each paragraph as "\n" + sub_sec
		// (rag/nlp/__init__.py:1405), so the per-paragraph token count
		// INCLUDES the leading "\n". Count it the same way so the running
		// sum matches Python's size("\n" + sub_sec).
		pt := tokenizeStr("\n" + p)
		if pt > cap {
			if len(cur) > 0 {
				groups = append(groups, cur)
			}
			groups = append(groups, []string{p})
			cur, curT = []string{}, 0
			continue
		}
		if len(cur) == 0 {
			cur, curT = []string{p}, pt
			continue
		}
		if curT+pt <= cap {
			cur = append(cur, p)
			curT += pt
		} else {
			// OVER_CAP: merge the overflowing paragraph, then close.
			cur = append(cur, p)
			curT += pt
			groups = append(groups, cur)
			cur, curT = []string{}, 0
		}
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	return groups
}

// expectedPythonChunks returns the chunk texts Python's naive_merge produces
// for mergeSourceLines at the given chunk_token_size.
func expectedPythonChunks(t *testing.T, chunkTokenSize int) []string {
	t.Helper()
	groups := pythonMergeGroups(mergeSourceLines, chunkTokenSize)
	want := make([]string, len(groups))
	for i, g := range groups {
		want[i] = strings.Join(g, "\n")
	}
	return want
}

// invokeTokenChunker runs the real TokenChunker component on plain text and
// returns the produced chunk texts, in order.
func invokeTokenChunker(t *testing.T, text string, chunkTokenSize int) []string {
	t.Helper()
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("TokenChunker")
	if !ok || factory == nil {
		t.Fatalf("TokenChunker is not registered")
	}
	param := map[string]any{
		"chunk_token_size": float64(chunkTokenSize),
		"delimiters":       []any{"\n"},
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

// TestTokenChunkerMergeMatchesPython is the red test for the merge-boundary
// divergence (ragflow chunker parity):
//
// The Go merge decision used tokenizeStr(prev + "\n" + incoming) — the BPE
// token count of the JOINED string — while Python's naive_merge accumulates a
// running sum of per-paragraph token counts (cur_t + size(p)). Because BPE
// tokenization is not additive across the "\n" boundary, the two merge
// decisions disagree: Go can merge one paragraph more (or fewer) than Python,
// shifting every downstream boundary by a line and, once a budget is small
// enough, changing the chunk count outright.
//
// This test asserts the Go TokenChunker output equals Python's naive_merge
// output (verified chunk count AND per-chunk text) for two budgets:
//   - chunk_token_size=32: Python emits 9 chunks (Go over-merged to 8 before
//     the running-sum fix).
//   - chunk_token_size=128: both emit 3 chunks, but Python closes chunk 0 one
//     line earlier than Go (boundary offset before the fix).
func TestTokenChunkerMergeMatchesPython(t *testing.T) {
	text := strings.Join(mergeSourceLines, "\n")
	for _, cap := range []int{32, 128} {
		want := expectedPythonChunks(t, cap)
		got := invokeTokenChunker(t, text, cap)
		if len(got) != len(want) {
			t.Fatalf("chunk_token_size=%d: chunk count go=%d, python=%d", cap, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("chunk_token_size=%d chunk[%d] mismatch:\n got=%q\nwant=%q", cap, i, got[i], want[i])
			}
		}
	}
}
