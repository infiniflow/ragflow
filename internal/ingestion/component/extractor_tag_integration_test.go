//go:build integration
// +build integration

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
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

func TestBuildMemoryTagIndex(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	raw := []schema.TagLabel{
		{Content: "machine learning models", Tags: []string{"ML.v1", "AI"}},
		{Content: "deep learning neural nets", Tags: []string{"AI", "DL"}},
		{Content: "   ", Tags: []string{"Ignored"}},
	}
	idx := buildMemoryTagIndex(raw, tok)
	if idx == nil {
		t.Fatal("expected non-nil MemoryTagIndex")
	}
	if len(idx.examples) != 2 {
		t.Fatalf("expected 2 clean examples, got %d", len(idx.examples))
	}
	if _, ok := idx.allTags["ML_v1"]; !ok {
		t.Fatalf("expected tag ML_v1 after dot sanitization, got %v", idx.allTags)
	}
	if _, ok := idx.allTags["ML.v1"]; ok {
		t.Fatal("expected ML.v1 with dot to be replaced")
	}

	var sumProb float64
	for _, p := range idx.allTags {
		sumProb += p
	}
	if sumProb < 0.9999 || sumProb > 1.0001 {
		t.Fatalf("expected background probabilities to sum to 1.0, got %f", sumProb)
	}
}

func TestMatchAndTagChunk_AsymmetricLength(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	// Reference set: 10-word rare example
	rawEx := []schema.TagLabel{
		{Content: "RAGFlow vector database retrieval architecture engine", Tags: []string{"RAG", "VectorDB"}},
		{Content: "general culinary cooking recipe baking", Tags: []string{"Cooking"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	// 800-word long technical chunk containing the reference example words
	longBody := strings.Repeat("RAGFlow is an advanced system that integrates vector database and retrieval architecture engine into scalable workflows. ", 40)
	chunk := map[string]any{"content_with_weight": longBody}

	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk for asymmetric length matching")
	}
	if len(matched.Tags) == 0 {
		t.Fatal("expected non-empty matched tags")
	}
	if matched.TagWeights["RAG"] <= 0 || matched.TagWeights["VectorDB"] <= 0 {
		t.Fatalf("expected positive scores for RAG and VectorDB, got: %v", matched.TagWeights)
	}
	for tag, score := range matched.TagWeights {
		if score < 1 || score > 10 {
			t.Fatalf("tag %s score %d is outside [1, 10]", tag, score)
		}
	}
}

func TestMatchAndTagChunk_TopKWeighted(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "machine learning artificial intelligence deep neural networks", Tags: []string{"AI"}},
		{Content: "financial market stocks bonds banking economy", Tags: []string{"Finance"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	// Chunk has high overlap with AI example, slight overlap with Finance
	chunk := map[string]any{
		"content_with_weight": "machine learning artificial intelligence deep neural networks banking financial",
	}

	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk")
	}
	aiScore := matched.TagWeights["AI"]
	finScore := matched.TagWeights["Finance"]
	if aiScore <= finScore {
		t.Fatalf("expected AI score (%d) > Finance score (%d)", aiScore, finScore)
	}
}

func TestMatchAndTagChunk_DuplicateTagsDedup(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "natural language processing text analysis", Tags: []string{"NLP", "NLP", "AI"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if len(idx.examples[0].Tags) != 2 {
		t.Fatalf("expected 2 unique tags in example, got %v", idx.examples[0].Tags)
	}

	chunk := map[string]any{"content_with_weight": "natural language processing text analysis"}
	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil match")
	}
	if matched.TagWeights["NLP"] < 1 || matched.TagWeights["NLP"] > 10 {
		t.Fatalf("invalid score for NLP: %d", matched.TagWeights["NLP"])
	}
}

func TestMatchAndTagChunk_AllEmptyTags(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "some text without tags", Tags: []string{}},
		{Content: "another text with empty tag", Tags: []string{"", "  "}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx != nil {
		t.Fatalf("expected nil index when all tags are empty, got %v", idx)
	}

	chunk := map[string]any{"content_with_weight": "some text without tags"}
	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched != nil {
		t.Fatalf("expected nil match when index is nil, got %v", matched)
	}
}

func TestMatchAndTagChunk_DeterministicTieBreak(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "shared keyword match document", Tags: []string{"tag_b", "tag_a", "tag_c"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	chunk := map[string]any{"content_with_weight": "shared keyword match document"}
	// Ask for top 2 out of 3 equal-scoring tags
	matched := matchAndTagChunk(chunk, idx, tok, 2)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk")
	}
	if len(matched.Tags) != 2 {
		t.Fatalf("expected exactly 2 tags, got %d", len(matched.Tags))
	}
	// Alphabetical tie-break: tag_a, tag_b
	if matched.Tags[0] != "tag_a" || matched.Tags[1] != "tag_b" {
		t.Fatalf("expected deterministic alphabetical tie-break ['tag_a', 'tag_b'], got %v", matched.Tags)
	}
}

func TestMatchAndTagChunk_TitleFallback(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "engineering bidding specifications procurement procedure", Tags: []string{"BiddingDoc", "Engineering"}},
		{Content: "culinary recipe cooking", Tags: []string{"Cooking"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	// Chunk has no body content, so it falls back to docnm_kwd title
	chunk := map[string]any{
		"docnm_kwd": "engineering_bidding_specifications_procurement_procedure.pdf",
	}

	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk triggered by title fallback")
	}
	if matched.TagWeights["BiddingDoc"] <= 0 || matched.TagWeights["Engineering"] <= 0 {
		t.Fatalf("expected BiddingDoc and Engineering tags matched, got: %v", matched.TagWeights)
	}
	if chunk["tag_kwd"] == nil {
		t.Fatal("expected tag_kwd to be populated on chunk")
	}
}

func BenchmarkMatchAndTagChunk_5000Examples(b *testing.B) {
	requireTokenizerPool(b)
	tok := tokenizer.New("english")
	rawEx := make([]schema.TagLabel, 5000)
	for i := 0; i < 5000; i++ {
		rawEx[i] = schema.TagLabel{
			Content: fmt.Sprintf("sample content document %d with keywords topic%d and subtopic%d for domain categorization", i, i%50, i%100),
			Tags:    []string{fmt.Sprintf("topic_%d", i%50), fmt.Sprintf("subtopic_%d", i%100)},
		}
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		b.Fatal("failed to build index")
	}

	chunk := map[string]any{
		"content_with_weight": "This is a detailed chunk talking about sample content document 42 with keywords topic42 and subtopic42 for domain categorization in practice.",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchAndTagChunk(chunk, idx, tok, 3)
	}
}
