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

package tool

import (
	"math"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	modelModule "ragflow/internal/entity/models"
)

// TestSplitKeywords_BigramFallback asserts comma terms are used when >=3, and
// space-split bigrams otherwise.
func TestSplitKeywords_BigramFallback(t *testing.T) {
	got := splitKeywords("alpha, beta, gamma")
	if len(got) != 3 {
		t.Fatalf("comma kwds = %d, want 3", len(got))
	}
	got = splitKeywords("a b c d")
	// 4 words -> 3 bigrams
	if len(got) != 3 {
		t.Fatalf("bigram kwds = %d, want 3", len(got))
	}
	if got[0] != "a b" {
		t.Errorf("bigram[0] = %q, want \"a b\"", got[0])
	}
}

// TestNarrowContent_KeepsKeywordSentence asserts narrowing keeps the
// keyword-bearing sentence and its neighbours, and highlights the keyword.
func TestNarrowContent_KeepsKeywordSentence(t *testing.T) {
	content := "The introduction is boring. The key insight about rocket engines is here. The conclusion is short."
	nc, ok := narrowContent(content, []string{"rocket"})
	if !ok {
		t.Fatal("expected keyword match")
	}
	if !strings.Contains(nc, "key insight") {
		t.Errorf("narrowed content missing keyword sentence: %q", nc)
	}
	if !strings.Contains(nc, "*rocket*") {
		t.Errorf("narrowed content missing highlight: %q", nc)
	}
}

// TestNarrowContent_NoKeyword asserts narrowing returns false when no keyword
// occurs.
func TestNarrowContent_NoKeyword(t *testing.T) {
	if _, ok := narrowContent("no match here at all", []string{"zzz"}); ok {
		t.Fatal("expected no match")
	}
}

// TestSplitSentences_DigitPeriod asserts "3.14" is not split into two sentences.
func TestSplitSentences_DigitPeriod(t *testing.T) {
	sents := splitSentences("pi is 3.14 and e is 2.71. end.")
	found := false
	for _, s := range sents {
		if strings.Contains(s, "3.14") {
			found = true
		}
	}
	if !found {
		t.Fatalf("digit-period sentence lost: %v", sents)
	}
}

// TestNarrowContent_KeepsTerminators pins Python _split_plain: each sentence
// keeps its terminator, and only a period next to a digit is not one.
func TestNarrowContent_KeepsTerminators(t *testing.T) {
	cases := []struct{ content, kw, want string }{
		{"RAGFlow是开源的。它支持Go。别的内容。", "go", "...RAGFlow是开源的。它支持*Go*。别的内容。..."},
		{"截至2023年底员工总数为500。2024年公司扩张。", "公司", "...截至2023年底员工总数为500。2024年*公司*扩张。..."},
		{"价格是3；5号到货。", "到货", "...价格是3；5号*到货*。..."},
		{"Alpha is fast. Beta is slow.", "beta", "...Alpha is fast. *Beta* is slow...."},
		{"pi is 3.14 here.", "pi", "...*pi* is 3.14 here...."},
	}
	for _, c := range cases {
		got, ok := narrowContent(c.content, []string{c.kw})
		if !ok || got != c.want {
			t.Errorf("narrowContent(%q, %q)\n got %q\nwant %q", c.content, c.kw, got, c.want)
		}
	}
}

// TestAgenticSearchKeywordWeights pins the fusion weight each mode asks for.
// KeywordsSimilarityWeight is the keyword share: the nlp layer derives the
// vector share as 1-weight, treats a zero keyword share as dense-only and a
// total keyword share as keyword-only. The Python tools this package was
// ported from passed vector_similarity_weight (1.0 for vector_search, 0 for
// bm25_search), so copying those constants verbatim inverts all three modes.
func TestAgenticSearchKeywordWeights(t *testing.T) {
	previous := GetRetrievalService()
	service := &capturingRetrievalService{}
	SetRetrievalService(service)
	t.Cleanup(func() { SetRetrievalService(previous) })

	state := runtime.NewCanvasState("run-weights", "session-weights")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := runtime.WithState(t.Context(), state)

	embeddingModel := &modelModule.EmbeddingModel{}
	tests := []struct {
		mode             string
		wantKeyword      float64
		wantVectorOnly   bool
		wantEmbedding    bool
		wantVectorWeight float64
	}{
		{mode: toolHybridSearch, wantKeyword: 0.7, wantVectorOnly: false, wantEmbedding: true, wantVectorWeight: 0.3},
		{mode: toolVectorSearch, wantKeyword: 0, wantVectorOnly: true, wantEmbedding: true, wantVectorWeight: 1},
		{mode: toolBM25Search, wantKeyword: 1, wantVectorOnly: false, wantEmbedding: false, wantVectorWeight: 0},
	}
	for _, testCase := range tests {
		t.Run(testCase.mode, func(t *testing.T) {
			if _, err := NewAgenticSearchTool(testCase.mode).InvokableRun(ctx, `{"query":"hello","kb_ids":["kb-1"]}`); err != nil {
				t.Fatalf("InvokableRun: %v", err)
			}
			if service.req.KeywordsSimilarityWeight == nil {
				t.Fatal("KeywordsSimilarityWeight is nil")
			}
			if got := *service.req.KeywordsSimilarityWeight; math.Abs(got-testCase.wantKeyword) > 1e-9 {
				t.Fatalf("KeywordsSimilarityWeight = %v, want %v", got, testCase.wantKeyword)
			}

			nlpRequest := nlpRequestFromRetrieval(service.req, []string{"tenant-1"}, 12, embeddingModel, false)
			if nlpRequest.VectorOnly != testCase.wantVectorOnly {
				t.Fatalf("VectorOnly = %v, want %v", nlpRequest.VectorOnly, testCase.wantVectorOnly)
			}
			if (nlpRequest.EmbeddingModel != nil) != testCase.wantEmbedding {
				t.Fatalf("EmbeddingModel set = %v, want %v", nlpRequest.EmbeddingModel != nil, testCase.wantEmbedding)
			}
			if nlpRequest.VectorSimilarityWeight == nil || math.Abs(*nlpRequest.VectorSimilarityWeight-testCase.wantVectorWeight) > 1e-9 {
				t.Fatalf("VectorSimilarityWeight = %v, want %v", nlpRequest.VectorSimilarityWeight, testCase.wantVectorWeight)
			}
		})
	}
}

// TestNarrowByKeywords_DropsKeywordless asserts chunks without keywords are
// dropped and deduped by narrowed content hash.
func TestNarrowByKeywords_DropsKeywordless(t *testing.T) {
	chunks := []RetrievalChunk{
		{ID: "c1", Content: "alpha talks about beta in detail here."},
		{ID: "c2", Content: "unrelated content entirely."},
		{ID: "c3", Content: "alpha talks about beta in detail here."}, // dup of c1
	}
	out := narrowByKeywords(chunks, "beta")
	if len(out) != 1 {
		t.Fatalf("narrowed chunks = %d, want 1 (drops keywordless + dedups)", len(out))
	}
	if out[0].ID != "c1" {
		t.Errorf("kept chunk = %q, want c1", out[0].ID)
	}
}
