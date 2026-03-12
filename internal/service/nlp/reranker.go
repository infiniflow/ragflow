// Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nlp

import (
	"math"
	"ragflow/internal/engine"
	"sort"
	"strconv"
	"strings"
)

// RerankModel defines the interface for reranker models
// This matches model.RerankModel interface
type RerankModel interface {
	// Similarity calculates similarity between query and texts
	Similarity(query string, texts []string) ([]float64, error)
}

// SearchResult represents the result of a search operation
type SearchResult struct {
	Total       int
	IDs         []string
	QueryVector []float64
	Field       map[string]map[string]interface{} // id -> fields
}

// Rerank performs reranking based on whether a reranker model is provided
// This implements the logic from rag/nlp/search.py L404-L429
// Parameters:
//   - rerankModel: the reranker model (can be nil)
//   - sres: search results
//   - query: the query string
//   - tkWeight: weight for token similarity
//   - vtWeight: weight for vector similarity
//   - useInfinity: whether using Infinity engine
//   - cfield: content field name (default: "content_ltks")
//   - qb: QueryBuilder instance for token processing
//
// Returns:
//   - sim: combined similarity scores
//   - tsim: token similarity scores
//   - vsim: vector similarity scores
func Rerank(
	rerankModel RerankModel,
	resp *engine.SearchResponse,
	keywords []string,
	questionVector []float64,
	sres *SearchResult,
	query string,
	tkWeight, vtWeight float64,
	useInfinity bool,
	cfield string,
	qb *QueryBuilder,
) (sim []float64, tsim []float64, vsim []float64) {
	// If reranker model is provided and there are results, use model reranking
	if rerankModel != nil && resp.Total > 0 {
		return RerankByModel(rerankModel, nil, query, tkWeight, vtWeight, cfield, qb)
	}

	// Otherwise, use fallback logic based on engine type
	if useInfinity {
		// For Infinity: scores are already normalized before fusion
		// Just extract the scores from results
		return RerankInfinityFallback(sres)
	}

	// For Elasticsearch: need to perform reranking
	return RerankStandard(resp, keywords, questionVector, nil, query, tkWeight, vtWeight, cfield, qb)
}

// RerankByModel performs reranking using a reranker model
// Reference: rag/nlp/search.py L333-L354
func RerankByModel(
	rerankModel RerankModel,
	sres *SearchResult,
	query string,
	tkWeight, vtWeight float64,
	cfield string,
	qb *QueryBuilder,
) (sim []float64, tsim []float64, vsim []float64) {
	if sres.Total == 0 || len(sres.IDs) == 0 {
		return []float64{}, []float64{}, []float64{}
	}

	// Extract keywords from query
	_, keywords := qb.Question(query, "qa", 0.6)

	// Build token lists and document texts for each chunk
	insTw := make([][]string, 0, len(sres.IDs))
	docs := make([]string, 0, len(sres.IDs))

	for _, id := range sres.IDs {
		fields := sres.Field[id]
		if fields == nil {
			insTw = append(insTw, []string{})
			docs = append(docs, "")
			continue
		}

		contentLtks := extractContentTokens(fields, cfield)
		titleTks := extractTitleTokens(fields)
		importantKwd := extractImportantKeywords(fields)

		// Combine tokens without repetition (simpler version for model reranking)
		tks := make([]string, 0, len(contentLtks)+len(titleTks)+len(importantKwd))
		tks = append(tks, contentLtks...)
		tks = append(tks, titleTks...)
		tks = append(tks, importantKwd...)
		insTw = append(insTw, tks)

		// Build document text for model reranking
		docText := removeRedundantSpaces(strings.Join(tks, " "))
		docs = append(docs, docText)
	}

	// Calculate token similarity
	tsim = TokenSimilarity(keywords, insTw, qb)

	// Get similarity scores from reranker model
	modelSim, err := rerankModel.Similarity(query, docs)
	if err != nil {
		// If model fails, fall back to token similarity only
		modelSim = make([]float64, len(tsim))
	}

	// Combine token similarity with model similarity
	// Model similarity is treated as vector similarity component
	sim = make([]float64, len(tsim))
	for i := range tsim {
		sim[i] = tkWeight*tsim[i] + vtWeight*modelSim[i]
	}

	return sim, tsim, modelSim
}

// RerankStandard performs standard reranking without a reranker model
// Used for Elasticsearch when no reranker model is provided
// Reference: rag/nlp/search.py L294-L331
func RerankStandard(
	resp *engine.SearchResponse,
	keywords []string,
	questionVector []float64,
	sres *SearchResult,
	query string,
	tkWeight, vtWeight float64,
	cfield string,
	qb *QueryBuilder,
) (sim []float64, tsim []float64, vsim []float64) {
	chunkCount := len(resp.Chunks)
	if resp.Total == 0 || chunkCount == 0 {
		return []float64{}, []float64{}, []float64{}
	}

	// Get vector information
	vectorSize := len(questionVector)
	vectorColumn := getVectorColumnName(vectorSize)
	zeroVector := make([]float64, vectorSize)

	// Extract embeddings and tokens from search results
	insEmbd := make([][]float64, 0, chunkCount)
	insTw := make([][]string, 0, chunkCount)

	for index := range resp.Chunks {
		// Extract vector
		chunk := resp.Chunks[index]
		chunkVector := extractVector(chunk, vectorColumn, zeroVector)
		insEmbd = append(insEmbd, chunkVector)

		// Extract tokens
		contentLtks := extractContentTokens(chunk, cfield)
		titleTks := extractTitleTokens(chunk)
		questionTks := extractQuestionTokens(chunk)
		importantKwd := extractImportantKeywords(chunk)

		// Combine tokens with weights: content + title*2 + important_kwd*5 + question_tks*6
		tks := make([]string, 0, len(contentLtks)+len(titleTks)*2+len(importantKwd)*5+len(questionTks)*6)
		tks = append(tks, contentLtks...)
		for i := 0; i < 2; i++ {
			tks = append(tks, titleTks...)
		}
		for i := 0; i < 5; i++ {
			tks = append(tks, importantKwd...)
		}
		for i := 0; i < 6; i++ {
			tks = append(tks, questionTks...)
		}
		insTw = append(insTw, tks)
	}

	if len(insEmbd) == 0 {
		return []float64{}, []float64{}, []float64{}
	}

	// Calculate hybrid similarity
	return HybridSimilarity(questionVector, insEmbd, keywords, insTw, tkWeight, vtWeight, qb)
}

// RerankInfinityFallback extracts scores from Infinity search results
// Infinity normalizes each way score before fusion, so we just extract them
func RerankInfinityFallback(sres *SearchResult) (sim []float64, tsim []float64, vsim []float64) {
	sim = make([]float64, len(sres.IDs))
	for i, id := range sres.IDs {
		if fields := sres.Field[id]; fields != nil {
			if score, ok := fields["_score"].(float64); ok {
				sim[i] = score
			}
		}
	}
	// For Infinity, tsim and vsim are the same as overall similarity
	return sim, sim, sim
}

// HybridSimilarity calculates hybrid similarity between query and documents
// Reference: rag/nlp/query.py L174-L182
func HybridSimilarity(
	avec []float64,
	bvecs [][]float64,
	atks []string,
	btkss [][]string,
	tkWeight, vtWeight float64,
	qb *QueryBuilder,
) (sim []float64, tsim []float64, vsim []float64) {
	// Calculate vector similarities using cosine similarity
	vsim = make([]float64, len(bvecs))
	for i, bvec := range bvecs {
		vsim[i] = cosineSimilarity(avec, bvec)
	}

	tsim = TokenSimilarity(atks, btkss, qb)

	// Check if all vector similarities are zero
	allZero := true
	for _, s := range vsim {
		if s != 0 {
			allZero = false
			break
		}
	}

	if allZero {
		return tsim, tsim, vsim
	}

	// Combine similarities
	sim = make([]float64, len(tsim))
	for i := range tsim {
		sim[i] = vsim[i]*vtWeight + tsim[i]*tkWeight
	}

	return sim, tsim, vsim
}

// TokenSimilarity calculates token-based similarity
// Reference: rag/nlp/query.py L184-L199
func TokenSimilarity(atks []string, btkss [][]string, qb *QueryBuilder) []float64 {
	atksDict := tokensToDict(atks, qb)
	btkssDicts := make([]map[string]float64, len(btkss))
	for i, btks := range btkss {
		btkssDicts[i] = tokensToDict(btks, qb)
	}

	similarities := make([]float64, len(btkssDicts))
	for i, btkDict := range btkssDicts {
		similarities[i] = tokenDictSimilarity(atksDict, btkDict)
	}

	return similarities
}

// tokensToDict converts tokens to a weighted dictionary
// Reference: rag/nlp/query.py L185-L195
func tokensToDict(tks []string, qb *QueryBuilder) map[string]float64 {
	d := make(map[string]float64)
	wts := qb.termWeight.Weights(tks, false)

	for i, tw := range wts {
		t := tw.Term
		c := tw.Weight
		d[t] += c * 0.4
		if i+1 < len(wts) {
			_t := wts[i+1].Term
			_c := wts[i+1].Weight
			d[t+_t] += math.Max(c, _c) * 0.6
		}
	}

	return d
}

// tokenDictSimilarity calculates similarity between two token dictionaries
// Reference: rag/nlp/query.py L201-L213
func tokenDictSimilarity(qtwt, dtwt map[string]float64) float64 {
	if len(qtwt) == 0 || len(dtwt) == 0 {
		return 0.0
	}

	// s = sum of query weights for matching tokens
	s := 1e-9
	for t, qw := range qtwt {
		if _, ok := dtwt[t]; ok {
			s += qw
		}
	}

	// q = sum of all query weights (L1 normalization)
	q := 1e-9
	for _, qw := range qtwt {
		q += qw
	}

	return s / q
}

// ArgsortDescending returns indices sorted by values in descending order
func ArgsortDescending(values []float64) []int {
	indices := make([]int, len(values))
	for i := range indices {
		indices[i] = i
	}

	sort.Slice(indices, func(i, j int) bool {
		return values[indices[i]] > values[indices[j]]
	})

	return indices
}

// Helper functions

// getVectorColumnName returns the vector column name based on dimension
func getVectorColumnName(dim int) string {
	return "q_" + strconv.Itoa(dim) + "_vec"
}

// extractVector extracts vector from chunk fields
func extractVector(fields map[string]interface{}, column string, zeroVector []float64) []float64 {
	v, ok := fields[column]
	if !ok {
		return zeroVector
	}

	switch val := v.(type) {
	case []float64:
		return val
	case []interface{}:
		vec := make([]float64, len(val))
		for i, v := range val {
			vec[i] = v.(float64)
		}
		return vec
	default:
		return zeroVector
	}
}

// extractContentTokens extracts content tokens from chunk fields
func extractContentTokens(fields map[string]interface{}, cfield string) []string {
	v, ok := fields[cfield].(string)
	if !ok {
		return []string{}
	}

	// Remove duplicates while preserving order
	seen := make(map[string]bool)
	var result []string
	for _, t := range strings.Fields(v) {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

// extractTitleTokens extracts title tokens from chunk fields
func extractTitleTokens(fields map[string]interface{}) []string {
	v, ok := fields["title_tks"].(string)
	if !ok {
		return []string{}
	}
	var result []string
	for _, t := range strings.Fields(v) {
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// extractQuestionTokens extracts question tokens from chunk fields
func extractQuestionTokens(fields map[string]interface{}) []string {
	v, ok := fields["question_tks"].(string)
	if !ok {
		return []string{}
	}
	var result []string
	for _, t := range strings.Fields(v) {
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// extractImportantKeywords extracts important keywords from chunk fields
func extractImportantKeywords(fields map[string]interface{}) []string {
	v, ok := fields["important_kwd"]
	if !ok {
		return []string{}
	}

	switch val := v.(type) {
	case string:
		return []string{val}
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return []string{}
	}
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// removeRedundantSpaces removes redundant spaces from text
func removeRedundantSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// parseFloat parses a string to float64
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
