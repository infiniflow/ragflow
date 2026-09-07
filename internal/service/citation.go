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

package service

import (
	"math"
	"regexp"
	"strings"
)

// sentenceSplitRE splits text on Chinese / English / Arabic sentence-ending
// punctuation.  Matches the Python regex in rag/nlp/search.py:insert_citations.
var sentenceSplitRE = regexp.MustCompile(`([^\|][；。？!！,؛؟.\n]|[a-z؀-ۿ][.?;!،؛؟][ \n])`)

const minSentenceLen = 5

// Embedder abstracts embedding-model access so InsertCitations is testable.
type Embedder interface {
	Encode(texts []string) ([][]float64, error)
}

// InsertCitations decorates answer with [ID:n] citation markers.
//
// Algorithm mirrors Python Dealer.insert_citations:
//  1. Split into sentences, preserving ``` code blocks.
//  2. Drop sentences shorter than minSentenceLen.
//  3. Encode sentences → sentence vectors.
//  4. Compute cosine similarity between each sentence and each chunk vector.
//  5. Threshold descent (0.63 → 0.3, ×0.8 per round): find chunks where
//     similarity > max*0.99.  Up to 4 chunks per sentence.
//  6. Rebuild answer text with [ID:n] markers inserted after cited sentences.
//
// Returns the decorated answer and the set of cited chunk indices.
func InsertCitations(answer string, chunks []SourcedChunk, embedder Embedder, chunkVectors [][]float64) (string, []int) {
	sentences, sentenceIdx := splitAnswer(answer)
	if len(sentences) == 0 || len(chunks) == 0 || len(chunkVectors) == 0 {
		return answer, nil
	}

	sentenceVecs, err := embedder.Encode(sentences)
	if err != nil || len(sentenceVecs) == 0 {
		return answer, nil
	}

	return InsertCitationsWithVectors(answer, chunks, sentenceVecs, chunkVectors, sentences, sentenceIdx)
}

// InsertCitationsWithVectors is the pure core: pre-split sentences, pre-encoded
// vectors.  Separated from the encoding step for testability.
func InsertCitationsWithVectors(
	answer string,
	chunks []SourcedChunk,
	sentenceVecs, chunkVectors [][]float64,
	sentences []string,
	sentenceIdx []int,
) (string, []int) {
	if len(sentences) != len(sentenceVecs) {
		n := len(sentenceVecs)
		if n < len(sentences) {
			sentences = sentences[:n]
			sentenceIdx = sentenceIdx[:n]
		}
	}

	sim := cosineSimMatrix(sentenceVecs, chunkVectors)
	cites := findCitations(sim)

	return applyCitations(answer, sentences, sentenceIdx, cites, chunks)
}

// splitAnswer splits answer text into sentences, preserving ``` code blocks.
func splitAnswer(answer string) ([]string, []int) {
	blocks := strings.Split(answer, "```")
	var rawPieces []string
	for i, block := range blocks {
		if i%2 == 1 {
			// Code block — keep intact, won't receive citations.
			rawPieces = append(rawPieces, "```"+block+"```\n")
		} else {
			// Regular text — split on sentence boundaries.
			rawPieces = append(rawPieces, sentenceSplit(block)...)
		}
	}
	// Rejoin the trailing punctuation that the regex captured as a separate piece.
	for i := 1; i < len(rawPieces); i++ {
		if sentenceSplitRE.MatchString(rawPieces[i]) {
			r := []rune(rawPieces[i])
			rawPieces[i-1] += string(r[0])
			rawPieces[i] = string(r[1:])
		}
	}
	// Filter out short pieces.
	var sentences []string
	var sentenceIdx []int
	for i, t := range rawPieces {
		if len(strings.TrimSpace(t)) >= minSentenceLen {
			sentences = append(sentences, t)
			sentenceIdx = append(sentenceIdx, i)
		}
	}
	return sentences, sentenceIdx
}

func sentenceSplit(text string) []string {
	indices := sentenceSplitRE.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return []string{text}
	}
	var result []string
	prev := 0
	for _, idx := range indices {
		result = append(result, text[prev:idx[1]])
		prev = idx[1]
	}
	if prev < len(text) {
		result = append(result, text[prev:])
	}
	return result
}

// applyCitations rebuilds the answer text with [ID:n] markers inserted after
// each cited sentence position.
func applyCitations(answer string, sentences []string, sentenceIdx []int, cites map[int][]int, chunks []SourcedChunk) (string, []int) {
	blocks := strings.Split(answer, "```")
	var rawPieces []string
	for i, block := range blocks {
		if i%2 == 1 {
			rawPieces = append(rawPieces, "```"+block+"```\n")
		} else {
			rawPieces = append(rawPieces, sentenceSplit(block)...)
		}
	}
	for i := 1; i < len(rawPieces); i++ {
		if sentenceSplitRE.MatchString(rawPieces[i]) {
			r := []rune(rawPieces[i])
			rawPieces[i-1] += string(r[0])
			rawPieces[i] = string(r[1:])
		}
	}

	// Map sentence position → chunk IDs to insert.
	citedChunks := make(map[int]string)
	seenChunks := make(map[int]bool)
	var citedIndices []int
	for i, rawIdx := range sentenceIdx {
		if chunkIdxs, ok := cites[i]; ok {
			var markers []string
			for _, ci := range chunkIdxs {
				if ci < len(chunks) && !seenChunks[ci] {
					seenChunks[ci] = true
					markers = append(markers, " [ID:"+chunks[ci].ID+"]")
					citedIndices = append(citedIndices, ci)
				}
			}
			citedChunks[rawIdx] = strings.Join(markers, "")
		}
	}

	var b strings.Builder
	for i, p := range rawPieces {
		b.WriteString(p)
		if markers, ok := citedChunks[i]; ok {
			b.WriteString(markers)
		}
	}
	return b.String(), citedIndices
}

// ---- Pure computation helpers ----

func cosineSimMatrix(a, b [][]float64) [][]float64 {
	m := make([][]float64, len(a))
	for i := range a {
		m[i] = make([]float64, len(b))
		na := vecNorm(a[i])
		if na == 0 {
			continue
		}
		for j := range b {
			nb := vecNorm(b[j])
			if nb == 0 {
				continue
			}
			m[i][j] = dot(a[i], b[j]) / (na * nb)
		}
	}
	return m
}

func vecNorm(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	return math.Sqrt(s)
}

func dot(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float64
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}

func findCitations(sim [][]float64) map[int][]int {
	cites := make(map[int][]int)
	thr := 0.63
	for thr > 0.3 && len(cites) == 0 {
		for i := range sim {
			mx := maxRow(sim[i]) * 0.99
			if mx < thr {
				continue
			}
			var matches []int
			for j, s := range sim[i] {
				if s > mx {
					matches = append(matches, j)
				}
			}
			if len(matches) > 4 {
				matches = matches[:4]
			}
			if len(matches) > 0 {
				cites[i] = matches
			}
		}
		thr *= 0.8
	}
	return cites
}

func maxRow(row []float64) float64 {
	if len(row) == 0 {
		return 0
	}
	mx := row[0]
	for _, v := range row[1:] {
		if v > mx {
			mx = v
		}
	}
	return mx
}
