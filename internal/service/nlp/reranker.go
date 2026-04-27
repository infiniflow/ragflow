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
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/logger"

	"go.uber.org/zap"
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
	chunks []map[string]interface{},
	total int,
	keywords []string,
	questionVector []float64,
	query string,
	tkWeight, vtWeight float64,
	useInfinity bool,
	cfield string,
	qb *QueryBuilder,
	rankFeature map[string]float64,
) (sim []float64, tsim []float64, vsim []float64) {
	// If reranker model is provided and there are results, use model reranking
	if rerankModel != nil && total > 0 {
		return RerankByModel(rerankModel, chunks, query, tkWeight, vtWeight, cfield, qb, rankFeature)
	}

	// Otherwise, use fallback logic based on engine type
	if useInfinity {
		// For Infinity: scores are already normalized before fusion
		// Just extract the scores from results
		if chunks == nil || total == 0 || len(chunks) == 0 {
			return []float64{}, []float64{}, []float64{}
		}

		return RerankInfinityFallback(chunks)
	}

	// For Elasticsearch: need to perform reranking and apply rank features
	return RerankStandard(chunks, keywords, questionVector, query, tkWeight, vtWeight, cfield, qb, rankFeature)
}

// RerankByModel performs reranking using a reranker model
func RerankByModel(
	rerankModel RerankModel,
	chunks []map[string]interface{},
	query string,
	tkWeight, vtWeight float64,
	cfield string,
	qb *QueryBuilder,
	rankFeature map[string]float64,
) (sim []float64, tsim []float64, vsim []float64) {
	if chunks == nil || len(chunks) == 0 {
		return []float64{}, []float64{}, []float64{}
	}

	chunkCount := len(chunks)

	logger.Info("RerankByModel started", zap.String("query", query), zap.Int("chunkCount", chunkCount), zap.Float64("tkWeight", tkWeight), zap.Float64("vtWeight", vtWeight))

	// Extract keywords from query
	keywords := []string{}
	if qb != nil {
		_, keywords = qb.Question(query, "qa", 0.6)
	}
	logger.Info("RerankByModel keywords extracted", zap.Any("keywords", keywords))

	// Build token lists and document texts for each chunk
	insTw := make([][]string, 0, chunkCount)
	docs := make([]string, 0, chunkCount)

	for _, chunk := range chunks {
		contentLtks := extractContentTokens(chunk, cfield)
		titleTks := extractTitleTokens(chunk)
		importantKwd := extractImportantKeywords(chunk)

		// Combine tokens without repetition (simpler version for model reranking)
		tks := make([]string, 0, len(contentLtks)+len(titleTks)+len(importantKwd))
		tks = append(tks, contentLtks...)
		tks = append(tks, titleTks...)
		tks = append(tks, importantKwd...)
		insTw = append(insTw, tks)

		// Build document text for model reranking
		docText := RemoveRedundantSpaces(strings.Join(tks, " "))
		docs = append(docs, docText)
	}

	// Calculate token similarity
	tsim = TokenSimilarity(keywords, insTw, qb)

	// Get similarity scores from reranker model
	modelSim, err := rerankModel.Similarity(query, docs)
	if err != nil {
		logger.Error("RerankByModel: rerankModel.Similarity failed; falling back to token-only similarity", err)
		// If model fails, fall back to token similarity only
		modelSim = make([]float64, len(tsim))
	}
	if len(modelSim) != chunkCount {
		logger.Warn("reranker returned mismatched score length; padding/truncating",
			zap.Int("got", len(modelSim)), zap.Int("want", chunkCount))
		fixed := make([]float64, chunkCount)
		copy(fixed, modelSim)
		modelSim = fixed
	}
	// Combine token similarity with model similarity
	// Model similarity is treated as vector similarity component
	sim = make([]float64, chunkCount)
	for i := range tsim {
		sim[i] = tkWeight*tsim[i] + vtWeight*modelSim[i]
	}

	// Apply rank feature scores (tag_score * 10 + pagerank)
	// Always apply pageranks, even when rankFeature is nil/empty
	sim = applyRankFeatureScores(chunks, sim, rankFeature)

	logger.Info("RerankByModel completed")
	return sim, tsim, modelSim
}

// RerankStandard performs standard reranking without a reranker model
// Used for Elasticsearch when no reranker model is provided
func RerankStandard(
	chunks []map[string]interface{},
	keywords []string,
	questionVector []float64,
	query string,
	tkWeight, vtWeight float64,
	cfield string,
	qb *QueryBuilder,
	rankFeature map[string]float64,
) (sim []float64, tsim []float64, vsim []float64) {
	chunkCount := len(chunks)
	if chunkCount == 0 {
		return []float64{}, []float64{}, []float64{}
	}

	logger.Info("RerankStandard started", zap.Int("chunkCount", chunkCount), zap.Float64("tkWeight", tkWeight), zap.Float64("vtWeight", vtWeight))

	// Compute keywords fresh from query
	if qb != nil && len(keywords) == 0 {
		_, keywords = qb.Question(query, "qa", 0.6)
	}
	logger.Info("RerankStandard keywords", zap.Any("keywords", keywords))

	// Get vector information
	vectorSize := len(questionVector)
	vectorColumn := getVectorColumnName(vectorSize)
	zeroVector := make([]float64, vectorSize)

	// Extract embeddings and tokens from search results
	insEmbd := make([][]float64, 0, chunkCount)
	insTw := make([][]string, 0, chunkCount)

	for index := range chunks {
		// Extract vector
		chunk := chunks[index]
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
	sim, tsim, vsim = HybridSimilarity(questionVector, insEmbd, keywords, insTw, tkWeight, vtWeight, qb)

	// Apply rank feature scores (tag_score * 10 + pagerank)
	// Always apply pageranks, even when rankFeature is nil/empty
	sim = applyRankFeatureScores(chunks, sim, rankFeature)

	logger.Info("RerankStandard completed")
	return sim, tsim, vsim
}

// RerankInfinityFallback is used as a fallback when no reranker model is provided for Infinity engine.
// Infinity can return scores in various field names (SCORE, score, SIMILARITY, etc.),
// so we check multiple possible field names. If no score is found, we default to 1.0
// to ensure the chunk passes through any similarity threshold filters.
func RerankInfinityFallback(chunks []map[string]interface{}) (sim []float64, tsim []float64, vsim []float64) {
	logger.Info("RerankInfinityFallback started", zap.Int("chunkCount", len(chunks)))

	sim = make([]float64, len(chunks))
	for i, chunk := range chunks {
		scoreFound := false
		scoreFields := []string{"SCORE", "score", "SIMILARITY", "similarity", "_score", "score()", "similarity()"}
		for _, field := range scoreFields {
			if score, ok := chunk[field].(float64); ok {
				sim[i] = score
				scoreFound = true
				break
			}
		}
		if !scoreFound {
			sim[i] = 1.0
		}
	}
	logger.Info("RerankInfinityFallback completed")
	return sim, sim, sim
}

// HybridSimilarity calculates hybrid similarity between query and documents
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
func tokensToDict(tks []string, qb *QueryBuilder) map[string]float64 {
	d := make(map[string]float64)
	if qb == nil || qb.termWeight == nil {
		return d
	}
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

	// Remove redundant spaces first to handle irregular spacing in Chinese text
	v = RemoveRedundantSpaces(v)

	// Now split by whitespace to get individual tokens
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
	// Remove redundant spaces first
	v = RemoveRedundantSpaces(v)
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

// RemoveRedundantSpaces removes redundant spaces from text
// First pass: remove spaces after left-boundary characters
// Second pass: remove spaces before right-boundary characters
func RemoveRedundantSpaces(s string) string {
	// First pass: remove spaces after left-boundary characters (opening brackets, etc.)
	// e.g., "（ text" -> "（text", "【 text" -> "【text"
	s = regexp.MustCompile(`([^\sa-z0-9.,\)>]) +([^\s])`).ReplaceAllString(s, "$1$2")

	// Second pass: remove spaces before right-boundary characters (closing brackets, punctuation)
	// e.g., "text ！" -> "text！"
	s = regexp.MustCompile(`([^\s]) +([^\sa-z0-9.,\(])`).ReplaceAllString(s, "$1$2")

	return s
}

// parseFloat parses a string to float64
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// applyRankFeatureScores applies rank feature scores to similarity
// Formula: tag_score * 10 + pagerank (per document)
func applyRankFeatureScores(chunks []map[string]interface{}, sim []float64, rankFeature map[string]float64) []float64 {
	if len(chunks) == 0 || len(sim) == 0 {
		return sim
	}

	// Collect pageranks from each chunk
	pageranks := make([]float64, len(chunks))
	for i, chunk := range chunks {
		if pr, ok := chunk[common.PAGERANK_FLD]; ok {
			if f, ok := toFloat64(pr); ok {
				pageranks[i] = f
			}
		}
	}

	// If no query rank features (no tag features), just add pageranks to sim
	if len(rankFeature) == 0 {
		for i := range sim {
			sim[i] += pageranks[i]
		}
		return sim
	}

	// Compute query denominator: sqrt(sum of squares of query rank feature weights, excluding pagerank)
	qDenor := 0.0
	for t, s := range rankFeature {
		if t != common.PAGERANK_FLD {
			qDenor += s * s
		}
	}
	qDenor = math.Sqrt(qDenor)

	// Compute tag score for each chunk
	tagScores := make([]float64, len(chunks))
	for i, chunk := range chunks {
		tagFeaStr, ok := chunk[common.TAG_FLD].(string)
		if !ok || tagFeaStr == "" {
			tagScores[i] = 0
			continue
		}

		// Parse tag_feas JSON string: {"tag1": 0.5, "tag2": 0.3}
		nor, denor := 0.0, 0.0
		tagFeaMap := parseTagFeasRerank(tagFeaStr)
		for t, sc := range tagFeaMap {
			if weight, exists := rankFeature[t]; exists {
				nor += weight * sc
			}
			denor += sc * sc
		}
		if denor == 0 {
			tagScores[i] = 0
		} else {
			tagScores[i] = nor / math.Sqrt(denor) / qDenor
		}
	}

	// Final score: tag_score * 10 + pagerank
	for i := range sim {
		sim[i] += tagScores[i]*10 + pageranks[i]
	}

	return sim
}

// toFloat64 converts various numeric types to float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

// parseTagFeasRerank parses a tag_feas JSON string into a map
// Format: {"tag1": 0.5, "tag2": 0.3}
func parseTagFeasRerank(tagFeasStr string) map[string]float64 {
	result := make(map[string]float64)
	if tagFeasStr == "" || tagFeasStr == "{}" {
		return result
	}

	// Parse JSON string
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tagFeasStr), &m); err != nil {
		return result
	}
	for k, v := range m {
		if f, ok := toFloat64(v); ok {
			result[k] = f
		}
	}
	return result
}
