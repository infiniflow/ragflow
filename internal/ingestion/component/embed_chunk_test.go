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

package component

import (
	"strings"
	"testing"
)

// TestEmbedChunkTextAppliesTokenizerRules pins the three steps a re-embedded
// chunk must share with the pipeline: table tags stripped, title encoded
// verbatim, and the merged vector weighted by filename_embd_weight.
func TestEmbedChunkTextAppliesTokenizerRules(t *testing.T) {
	embedder := &stubEmbedder{
		dim:       3,
		maxTokens: 4096,
		resultsByCall: []embeddingCallResult{
			{vectors: [][]float64{{1, 0, 0}}}, // content
			{vectors: [][]float64{{0, 1, 0}}}, // title
		},
	}

	field, vector, err := EmbedChunkText(t.Context(), embedder, "Doc Title", "before <table><td>cell</td></table> after", 0.25)
	if err != nil {
		t.Fatalf("EmbedChunkText: %v", err)
	}
	if field != "q_3_vec" {
		t.Fatalf("field = %q, want q_3_vec", field)
	}
	if len(embedder.callInputs) != 2 {
		t.Fatalf("encode calls = %d, want 2 (content then title)", len(embedder.callInputs))
	}
	if strings.Contains(embedder.callInputs[0][0], "<table>") || strings.Contains(embedder.callInputs[0][0], "<td>") {
		t.Fatalf("embedded content = %q, want the table tags stripped", embedder.callInputs[0][0])
	}
	if len(embedder.callInputs[1]) != 1 || embedder.callInputs[1][0] != "Doc Title" {
		t.Fatalf("title input = %#v, want the raw document name", embedder.callInputs[1])
	}
	// 0.25*title + 0.75*content.
	if len(vector) != 3 || vector[0] != 0.75 || vector[1] != 0.25 || vector[2] != 0 {
		t.Fatalf("merged vector = %#v, want [0.75 0.25 0]", vector)
	}
}

// TestEmbedChunkTextSkipsChunkWithNothingToEmbed mirrors the pipeline: a chunk
// that is only table structure contributes no embedding at all.
func TestEmbedChunkTextSkipsChunkWithNothingToEmbed(t *testing.T) {
	embedder := &stubEmbedder{dim: 3, maxTokens: 4096}

	field, vector, err := EmbedChunkText(t.Context(), embedder, "Doc Title", "<table><td></td></table>", 0.1)
	if err != nil {
		t.Fatalf("EmbedChunkText: %v", err)
	}
	if field != "" || vector != nil {
		t.Fatalf("field = %q vector = %#v, want nothing to embed", field, vector)
	}
	if embedder.calls.Load() != 0 {
		t.Fatalf("encode calls = %d, want 0", embedder.calls.Load())
	}
}

// TestEmbedChunkTextBlankNameSkipsTitleWeight keeps the pipeline's rule for
// documents without a usable name: no title vector, no weighting.
func TestEmbedChunkTextBlankNameSkipsTitleWeight(t *testing.T) {
	embedder := &stubEmbedder{
		dim:       3,
		maxTokens: 4096,
		resultsByCall: []embeddingCallResult{
			{vectors: [][]float64{{0, 0, 1}}},
		},
	}

	_, vector, err := EmbedChunkText(t.Context(), embedder, "   ", "some text", 0.1)
	if err != nil {
		t.Fatalf("EmbedChunkText: %v", err)
	}
	if len(embedder.callInputs) != 1 {
		t.Fatalf("encode calls = %d, want 1 (content only)", len(embedder.callInputs))
	}
	if len(vector) != 3 || vector[2] != 1 {
		t.Fatalf("vector = %#v, want the unweighted content vector", vector)
	}
}
