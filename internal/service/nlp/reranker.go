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
	"ragflow/internal/entity/models"

	"go.uber.org/zap"
)

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
	rerankModel *models.RerankModel,
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
		return RerankByModel(rerankModel, chunks, nil, nil, query, tkWeight, vtWeight, cfield, qb, rankFeature)
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
	rerankModel *models.RerankModel,
	chunks []map[string]interface{},
	ids []string,
	field map[string]map[string]interface{},
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

	common.Info("RerankByModel started", zap.String("query", query), zap.Int("chunkCount", chunkCount), zap.Float64("tkWeight", tkWeight), zap.Float64("vtWeight", vtWeight))

	// Extract keywords from query
	keywords := []string{}
	if qb != nil {
		_, keywords = qb.Question(query, "qa", 0.6)
	}
	common.Info("RerankByModel keywords extracted", zap.Any("keywords", keywords))

	// Build token lists and document texts for each chunk
	insTw := make([][]string, 0, chunkCount)
	docs := make([]string, 0, chunkCount)

	// Process chunks in id order
	for i, chunkID := range ids {
		chunk, ok := field[chunkID]
		if !ok {
			// Fallback to chunks[i] if id not found in field
			if i < len(chunks) {
				chunk = chunks[i]
			} else {
				continue
			}
		}

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
	rerankResponse, err := rerankModel.ModelDriver.Rerank(rerankModel.ModelName, query, docs, rerankModel.APIConfig, &models.RerankConfig{})
	if err != nil {
		common.Error("RerankByModel: rerankModel.Rerank failed; falling back to token-only similarity", err)
		// If model fails, fall back to token similarity only
		rerankResponse = &models.RerankResponse{}
	}

	// Use the Index field from the response to place scores in the correct position,
	// matching the original document order
	modelSim := make([]float64, len(insTw))
	for _, result := range rerankResponse.Data {
		if result.Index >= 0 && result.Index < len(modelSim) {
			modelSim[result.Index] = result.RelevanceScore
		}
	}

	// Reranker drivers do not agree on a score scale: Cohere/Jina/Voyage emit
	// calibrated [0, 1] relevance scores, but NVIDIA returns raw, often
	// negative logits. The hybrid blend below (tkWeight * tksim + vtWeight *
	// modelSim) lives on a fixed [0, 1] scale, so an un-normalized logit
	// weighted by vtWeight=0.7 can sink a relevant chunk below pure keyword
	// matches and dominate the blend. Centralize the normalization here so
	// every provider contributes on the same scale. See
	// NormalizeRerankScores for the contract.
	modelSim = NormalizeRerankScores(modelSim)

	// Combine token similarity with model similarity
	// Model similarity is treated as vector similarity component
	sim = make([]float64, len(insTw))
	for i := range tsim {
		sim[i] = tkWeight*tsim[i] + vtWeight*modelSim[i]
	}

	// Apply rank feature scores (tag_score * 10 + pagerank)
	// Always apply pageranks, even when rankFeature is nil/empty
	sim = applyRankFeatureScoresForIDs(ids, field, sim, rankFeature)

	common.Info("RerankByModel completed")
	return sim, tsim, modelSim
}

// NormalizeRerankScores rescales reranker scores into [0, 1] for the
// hybrid blend in RerankByModel. Mirrors the contract enforced by
// Base.similarity / _normalize_rank in rag/llm/rerank_model.py.
//
// Providers that already return calibrated [0, 1] relevance scores
// (Cohere, Jina, Voyage, ...) are returned unchanged, so
// similarity_threshold filtering and reported vector_similarity keep
// their absolute magnitudes. Only out-of-range output (e.g. NVIDIA's
// unbounded, often negative logits) is rescaled: a batch with usable
// spread is min-max mapped onto [0, 1] (which stops a negative logit
// from dragging a relevant chunk below pure keyword matches once
// weighted by vtweight), while a spreadless batch (including a single
// candidate) is clamped per element so a lone high score is not silently
// zeroed and no NaN leaks into the blend.
//
// An empty input is returned verbatim. Mutates the input slice in place
// to keep the RerankByModel call site allocation-free; the returned
// slice is the same backing array.
func NormalizeRerankScores(scores []float64) []float64 {
	n := len(scores)
	if n == 0 {
		return scores
	}
	minScore := scores[0]
	maxScore := scores[0]
	for _, s := range scores[1:] {
		if s < minScore {
			minScore = s
		}
		if s > maxScore {
			maxScore = s
		}
	}

	// Already in [0, 1]? Keep absolute magnitudes so calibrated providers
	// and degenerate (but valid) batches are NOT collapsed to zero.
	if minScore >= 0.0 && maxScore <= 1.0 {
		return scores
	}

	// Spreadless out-of-range batch: clamp per element instead of
	// collapsing to zero or dividing by ~0.
	span := maxScore - minScore
	if span < 1e-3 {
		for i, s := range scores {
			if s < 0.0 {
				scores[i] = 0.0
			} else if s > 1.0 {
				scores[i] = 1.0
			}
		}
		return scores
	}

	// Min-max rescale onto [0, 1].
	invSpan := 1.0 / span
	for i, s := range scores {
		scores[i] = (s - minScore) * invSpan
	}
	return scores
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

	common.Info("RerankStandard started", zap.Int("chunkCount", chunkCount), zap.Float64("tkWeight", tkWeight), zap.Float64("vtWeight", vtWeight))

	// Compute keywords fresh from query
	if qb != nil && len(keywords) == 0 {
		_, keywords = qb.Question(query, "qa", 0.6)
	}
	common.Info("RerankStandard keywords", zap.Any("keywords", keywords))

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

	common.Info("RerankStandard completed", zap.Int("outputChunks", len(sim)))
	return sim, tsim, vsim
}

// RerankInfinityFallback is used as a fallback when no reranker model is provided for Infinity engine.
// Infinity can return scores in various field names (SCORE, score, SIMILARITY, etc.),
// so we check multiple possible field names. If no score is found, we default to 1.0
// to ensure the chunk passes through any similarity threshold filters.
func RerankInfinityFallback(chunks []map[string]interface{}) (sim []float64, tsim []float64, vsim []float64) {
	common.Info("RerankInfinityFallback started", zap.Int("chunkCount", len(chunks)))

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
	common.Info("RerankInfinityFallback completed")
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
	// rerank_with_knn: sim = tkweight * tksim + vtweight * vtsim
	sim = make([]float64, len(tsim))
	for i := range tsim {
		sim[i] = tkWeight*tsim[i] + vtWeight*vsim[i]
	}

	return sim, tsim, vsim
}

// TokenSimilarity calculates token-based similarity
func TokenSimilarity(atks []string, btkss [][]string, qb *QueryBuilder) []float64 {
	atksDict, atksKeyOrder := tokensToDict(atks, qb)
	btkssDicts := make([]map[string]float64, len(btkss))
	for i, btks := range btkss {
		btkssDicts[i], _ = tokensToDict(btks, qb)
	}

	similarities := make([]float64, len(btkssDicts))
	for i, btkDict := range btkssDicts {
		similarities[i] = tokenDictSimilarity(atksDict, btkDict, atksKeyOrder)
	}

	return similarities
}

// tokensToDict converts tokens to a weighted dictionary.
// Also returns the insertion order of keys to match Python's dict insertion order.
func tokensToDict(tks []string, qb *QueryBuilder) (map[string]float64, []string) {
	d := make(map[string]float64)
	var keyOrder []string
	if qb == nil || qb.termWeight == nil {
		return d, keyOrder
	}
	wts := qb.termWeight.Weights(tks, false)

	for i, tw := range wts {
		t := tw.Term
		c := tw.Weight
		if _, exists := d[t]; !exists {
			keyOrder = append(keyOrder, t)
		}
		d[t] += c * 0.4
		if i+1 < len(wts) {
			_t := wts[i+1].Term
			_c := wts[i+1].Weight
			k := t + _t
			if _, exists := d[k]; !exists {
				keyOrder = append(keyOrder, k)
			}
			d[k] += math.Max(c, _c) * 0.6
		}
	}
	return d, keyOrder
}

// tokenDictSimilarity calculates similarity between two token dictionaries.
// Uses the query key order (from tokensToDict) to match Python's dict insertion order.
// Python iterates dict.items() in insertion order (guaranteed since Python 3.7).
// Floating-point addition is non-associative, so the iteration order matters.
func tokenDictSimilarity(qtwt, dtwt map[string]float64, qKeyOrder []string) float64 {
	if len(qtwt) == 0 || len(dtwt) == 0 {
		return 0.0
	}

	// Use qKeyOrder if provided, otherwise fall back to sorted keys
	keys := qKeyOrder
	if len(keys) == 0 {
		keys = make([]string, 0, len(qtwt))
		for k := range qtwt {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	// s = sum of query weights for matching tokens
	// NOTE: Use naive left-to-right summation (not PairwiseSum) to match Python's
	// exact float64 behavior in query.py similarity(). Python iterates dict.items()
	// in insertion order with simple +=, which is left-to-right accumulation.
	s := 1e-9
	matchCount := 0
	for _, t := range keys {
		if _, ok := dtwt[t]; ok {
			s += qtwt[t]
			matchCount++
		}
	}

	// q = sum of all query weights (L1 normalization)
	q := 1e-9
	for _, t := range keys {
		q += qtwt[t]
	}

	result := s / q
	return result
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

	// Split by whitespace to get individual tokens
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
	// NOTE: Do NOT call RemoveRedundantSpaces here - it removes spaces between Chinese chars
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

// cosineSimilarity calculates cosine similarity between two vectors.
// Three parallel pairwise sums (dot, ||a||², ||b||²) keep precision
// comparable to numpy's float64 reductions.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	dBuf := make([]float64, len(a))
	aBuf := make([]float64, len(a))
	bBuf := make([]float64, len(a))
	for i := range a {
		dBuf[i] = a[i] * b[i]
		aBuf[i] = a[i] * a[i]
		bBuf[i] = b[i] * b[i]
	}
	dot := common.PairwiseSum(dBuf)
	normA := common.PairwiseSum(aBuf)
	normB := common.PairwiseSum(bBuf)

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dot / (common.PySqrt(normA) * common.PySqrt(normB))
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
	// Sort keys for deterministic float accumulation (Go map iteration is randomized)
	rankFeatureKeys := make([]string, 0, len(rankFeature))
	for k := range rankFeature {
		rankFeatureKeys = append(rankFeatureKeys, k)
	}
	sort.Strings(rankFeatureKeys)

	qDenorBuf := make([]float64, 0, len(rankFeatureKeys))
	for _, t := range rankFeatureKeys {
		if t != common.PAGERANK_FLD {
			s := rankFeature[t]
			qDenorBuf = append(qDenorBuf, s*s)
		}
	}
	qDenor := common.PySqrt(common.PairwiseSum(qDenorBuf))

	// If the query has no usable tag-feature weights (e.g. pagerank-only), fall
	// back to pageranks-only. Mirrors Python's `if q_denor == 0: return pageranks`
	// in _rank_feature_scores(); otherwise the later `... / qDenor` divides by 0
	// and turns matching chunks into NaN, contaminating the final ranking.
	if qDenor == 0 {
		for i := range sim {
			sim[i] += pageranks[i]
		}
		return sim
	}

	// Compute tag score for each chunk
	tagScores := make([]float64, len(chunks))
	for i, chunk := range chunks {
		tagFeaStr, ok := chunk[common.TAG_FLD].(string)
		if !ok || tagFeaStr == "" {
			tagScores[i] = 0
			continue
		}

		// Parse tag_feas JSON string: {"tag1": 0.5, "tag2": 0.3}
		tagFeaMap := parseTagFeasRerank(tagFeaStr)
		// Sort keys for deterministic float accumulation
		tagFeaKeys := make([]string, 0, len(tagFeaMap))
		for k := range tagFeaMap {
			tagFeaKeys = append(tagFeaKeys, k)
		}
		sort.Strings(tagFeaKeys)
		norBuf := make([]float64, 0, len(tagFeaKeys))
		denorBuf := make([]float64, 0, len(tagFeaKeys))
		for _, t := range tagFeaKeys {
			sc := tagFeaMap[t]
			if weight, exists := rankFeature[t]; exists {
				norBuf = append(norBuf, weight*sc)
			}
			denorBuf = append(denorBuf, sc*sc)
		}
		// NOTE: Use naive left-to-right summation to match Python's exact float64
		// behavior in _rank_feature_scores(). Python uses nor += ... and denor += ...
		// in dict iteration order, which is simple left-to-right accumulation.
		var nor, denor float64
		for _, v := range norBuf {
			nor += v
		}
		for _, v := range denorBuf {
			denor += v
		}
		if denor == 0 {
			tagScores[i] = 0
		} else {
			tagScores[i] = nor / common.PySqrt(denor) / qDenor
		}
	}

	// Final score: tag_score * 10 + pagerank
	for i := range sim {
		sim[i] += tagScores[i]*10 + pageranks[i]
	}

	return sim
}

// applyRankFeatureScoresForIDs applies rank feature scores using field map (by chunk IDs)
// This is used when we have the field map from search results
func applyRankFeatureScoresForIDs(ids []string, field map[string]map[string]interface{}, sim []float64, rankFeature map[string]float64) []float64 {
	if len(ids) == 0 || len(sim) == 0 {
		return sim
	}

	// Collect pageranks from each chunk via field map
	pageranks := make([]float64, len(ids))
	for i, chunkID := range ids {
		chunk, ok := field[chunkID]
		if !ok {
			pageranks[i] = 0
			continue
		}
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
	// Sort keys for deterministic float accumulation (Go map iteration is randomized)
	rankFeatureKeys := make([]string, 0, len(rankFeature))
	for k := range rankFeature {
		rankFeatureKeys = append(rankFeatureKeys, k)
	}
	sort.Strings(rankFeatureKeys)

	qDenorBuf := make([]float64, 0, len(rankFeatureKeys))
	for _, t := range rankFeatureKeys {
		if t != common.PAGERANK_FLD {
			s := rankFeature[t]
			qDenorBuf = append(qDenorBuf, s*s)
		}
	}
	// NOTE: Python uses np.sum([s*s for...]) which is pairwise, so PairwiseSum is correct here
	qDenor := common.PySqrt(common.PairwiseSum(qDenorBuf))

	// If the query has no usable tag-feature weights (e.g. pagerank-only), fall
	// back to pageranks-only. Mirrors Python's `if q_denor == 0: return pageranks`
	// in _rank_feature_scores(); otherwise the later `... / qDenor` divides by 0
	// and turns matching chunks into NaN, contaminating the final ranking.
	if qDenor == 0 {
		for i := range sim {
			sim[i] += pageranks[i]
		}
		return sim
	}

	// Compute tag score for each chunk
	tagScores := make([]float64, len(ids))
	for i, chunkID := range ids {
		chunk, ok := field[chunkID]
		if !ok {
			tagScores[i] = 0
			continue
		}
		tagFeaStr, ok := chunk[common.TAG_FLD].(string)
		if !ok || tagFeaStr == "" {
			tagScores[i] = 0
			continue
		}

		// Parse tag_feas JSON string: {"tag1": 0.5, "tag2": 0.3}
		tagFeaMap := parseTagFeasRerank(tagFeaStr)
		// Sort keys for deterministic float accumulation
		tagFeaKeys := make([]string, 0, len(tagFeaMap))
		for k := range tagFeaMap {
			tagFeaKeys = append(tagFeaKeys, k)
		}
		sort.Strings(tagFeaKeys)
		norBuf := make([]float64, 0, len(tagFeaKeys))
		denorBuf := make([]float64, 0, len(tagFeaKeys))
		for _, t := range tagFeaKeys {
			sc := tagFeaMap[t]
			if weight, exists := rankFeature[t]; exists {
				norBuf = append(norBuf, weight*sc)
			}
			denorBuf = append(denorBuf, sc*sc)
		}
		// NOTE: Use naive left-to-right summation to match Python's exact float64
		// behavior in _rank_feature_scores(). Python uses nor += ... and denor += ...
		var nor, denor float64
		for _, v := range norBuf {
			nor += v
		}
		for _, v := range denorBuf {
			denor += v
		}
		if denor == 0 {
			tagScores[i] = 0
		} else {
			tagScores[i] = nor / common.PySqrt(denor) / qDenor
		}
	}

	// Final score: tag_score * 10 + pagerank
	for i := range sim {
		sim[i] += tagScores[i]*10 + pageranks[i]
	}

	return sim
}

// RerankWithKNN performs reranking using KNN scores (ES two-pass approach)
// Matches Python's rerank_with_knn()
//
// TWO-PASS APPROACH (matching Python's Dealer._knn_scores + get_scores pattern):
//
//	PASS 1 (KNNScores / _knn_scores):
//	  - First search returns text-matched chunks with hybrid scores (BM25 + vector fusion)
//	  - Second KNN-only search filtered by those chunk IDs
//	  - ES computes cosine similarity between query vector and stored chunk vectors
//	  - Vectors stay in ES index (no need to ship them to application)
//	  - Returns raw KNN search result containing _id -> _score mappings
//
//	PASS 2 (GetScores / get_scores):
//	  - Extracts doc_id -> score from the KNN result
//	  - Produces the clean vector similarity scores needed for reranking
//
//	RERANK (RerankWithKNN / rerank_with_knn):
//	  - Combines token similarity (keyword overlap) with vector similarity (cosine)
//	  - Formula: sim = tkWeight * tksim + vtWeight * vtsim + rank_features
//	  - Token weighting: content + title*2 + important_kwd*5 + question_tks*6
//	  - Rank features: tag_score * 10 + pagerank (per chunk)
//
// Python equivalent in rag/nlp/search.py:
//
//	knn_scores = await self._knn_scores(sres, idx_names, kb_ids)  # Pass 1
//	knn_scores = self.dataStore.get_scores(res)                    # Pass 2
//	sim, tsim, vsim = self.rerank_with_knn(sres, question, knn_scores, ...)  # Rerank
//
// Parameters:
//   - chunks: search results from first pass (used for fallback)
//   - ids: ordered chunk IDs from search results
//   - field: field map (chunk_id -> chunk fields)
//   - knnScores: cosine similarity scores from GetScores (doc_id -> score)
//   - query: search query string
//   - tkWeight: token similarity weight (typically 0.3)
//   - vtWeight: vector similarity weight (typically 0.7)
//   - cfield: content field name (default "content_ltks")
//   - qb: QueryBuilder for token processing
//   - rankFeature: rank feature weights (e.g., {"pagerank_fea": 10.0})
func RerankWithKNN(
	chunks []map[string]interface{},
	ids []string,
	field map[string]map[string]interface{},
	knnScores map[string]float64,
	query string,
	tkWeight, vtWeight float64,
	cfield string,
	qb *QueryBuilder,
	rankFeature map[string]float64,
) (sim []float64, tsim []float64, vsim []float64) {
	if len(ids) == 0 {
		return []float64{}, []float64{}, []float64{}
	}

	common.Info("RerankWithKNN started", zap.Int("chunkCount", len(ids)), zap.Float64("tkWeight", tkWeight), zap.Float64("vtWeight", vtWeight))

	// Normalize important_kwd - Python checks if it's a string and wraps in list
	// for i in sres.ids:
	//     if isinstance(sres.field[i].get("important_kwd", []), str):
	//         sres.field[i]["important_kwd"] = [sres.field[i]["important_kwd"]]
	for _, chunkID := range ids {
		chunk, ok := field[chunkID]
		if !ok {
			continue
		}
		if v, exists := chunk["important_kwd"]; exists {
			if _, isString := v.(string); isString {
				field[chunkID]["important_kwd"] = []string{v.(string)}
			}
		}
	}

	// Extract keywords from query
	keywords := []string{}
	if qb != nil {
		_, keywords = qb.Question(query, "qa", 0.6)
	}
	common.Info("RerankWithKNN keywords", zap.Any("keywords", keywords))

	// Build token lists matching Python's OrderedDict approach
	insTw := make([][]string, 0, len(ids))
	for i, chunkID := range ids {
		chunk, ok := field[chunkID]
		if !ok {
			if i < len(chunks) {
				chunk = chunks[i]
			} else {
				insTw = append(insTw, []string{})
				continue
			}
		}

		// Normalize text content - split, dedupe while preserving order (OrderedDict effect)
		contentLtks := extractContentTokens(chunk, cfield)
		titleTks := extractTitleTokens(chunk)
		questionTks := extractQuestionTokens(chunk)
		importantKwd := extractImportantKeywords(chunk)

		// Combine tokens with weights: content + title*2 + important_kwd*5 + question_tks*6
		// This matches Python: tks = content_ltks + title_tks * 2 + important_kwd * 5 + question_tks * 6
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

	// Calculate token similarity
	tsim = TokenSimilarity(keywords, insTw, qb)
	common.Info("RerankWithKNN tsim", zap.Float64s("tsim", tsim))

	// Build vector similarity from knnScores - matches Python's np.array([knn_scores.get(chunk_id, 0.0) for chunk_id in sres.ids])
	vsim = make([]float64, len(ids))
	for i, chunkID := range ids {
		vsim[i] = knnScores[chunkID] // Returns 0.0 if not found (Go map default)
	}
	common.Info("RerankWithKNN knnScores", zap.Int("knnScoreCount", len(knnScores)), zap.Float64s("vsim", vsim), zap.Strings("ids", ids), zap.Float64s("knnScores", func() []float64 {
		scores := make([]float64, 0, len(knnScores))
		for _, id := range ids {
			if s, ok := knnScores[id]; ok {
				scores = append(scores, s)
			}
		}
		return scores
	}()))

	// Apply rank feature scores
	sim = make([]float64, len(tsim))
	for i := range tsim {
		sim[i] = tkWeight*tsim[i] + vtWeight*vsim[i]
	}

	// Apply rank feature scores (tag_score * 10 + pagerank)
	sim = applyRankFeatureScoresForIDs(ids, field, sim, rankFeature)
	common.Info("RerankWithKNN rankFeatureScores", zap.Any("rankFeature", rankFeature), zap.Any("simAfterRank", sim))

	common.Info("RerankWithKNN completed", zap.Int("outputChunks", len(sim)))
	return sim, tsim, vsim
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
