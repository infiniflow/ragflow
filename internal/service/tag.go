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
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/engine/infinity"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"ragflow/internal/service/nlp"
)

// Knowledgebase type alias for entity.Knowledgebase
type Knowledgebase = entity.Knowledgebase

// GetAllTagsInPortion returns the tag distribution for given KBs
// Corresponds to rag/nlp/search.py:all_tags_in_portion()
func (s *MetadataService) GetAllTagsInPortion(tenantID string, kbIDs []string) (map[string]float64, error) {
	if len(kbIDs) == 0 {
		return make(map[string]float64), nil
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)

	// Search with large limit to get all tag_kwd values
	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      kbIDs,
		Offset:     0,
		Limit:      10000, // Large limit to get all docs
	}

	searchResp, err := s.docEngine.Search(context.Background(), searchReq)
	if err != nil {
		return nil, err
	}

	// Use GetAggregation for tag counting (matching Python get_aggregation behavior)
	tagAgg := infinity.GetAggregation(searchResp.Chunks, "tag_kwd")
	if len(tagAgg) == 0 {
		return make(map[string]float64), nil
	}

	// Calculate total count for proportion calculation
	total := 0
	for _, tc := range tagAgg {
		total += tc["count"].(int)
	}
	if total == 0 {
		return make(map[string]float64), nil
	}

	// Calculate tag proportions: (count + 1) / (total + 1000)
	S := 1000.0
	allTags := make(map[string]float64)
	for _, tc := range tagAgg {
		allTags[tc["key"].(string)] = float64(tc["count"].(int)+1) / (float64(total) + S)
	}

	return allTags, nil
}

// TagQuery returns weighted tag features for a question
// Corresponds to rag/nlp/search.py:tag_query()
func (s *MetadataService) TagQuery(question string, tenantIDs []string, kbIDs []string, allTags map[string]float64, topnTags int) (map[string]float64, error) {
	if len(kbIDs) == 0 || len(allTags) == 0 || len(tenantIDs) == 0 {
		return make(map[string]float64), nil
	}

	// Build index names for all tenant IDs (matching Python L594-597)
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = fmt.Sprintf("ragflow_%s", tenantID)
	}

	// Process question to get match text
	queryBuilder := nlp.GetQueryBuilder()
	matchTextExpr, _ := queryBuilder.Question(question, "qa", 0.0) // min_match=0.0
	matchText := matchTextExpr.MatchingText

	logger.Debug("TagQuery match_text", zap.String("match_text", matchText))

	// Search with match text to get relevant docs (matching Python L599: idx_nms covers all tenant indices)
	searchReq := &types.SearchRequest{
		IndexNames: indexNames,
		KbIDs:      kbIDs,
		Offset:     0,
		Limit:      1000,
		MatchExprs: []interface{}{matchTextExpr},
	}

	searchResp, err := s.docEngine.Search(context.Background(), searchReq)
	if err != nil {
		return nil, err
	}

	// Use GetAggregation for tag counting (matching Python get_aggregation behavior)
	aggs := infinity.GetAggregation(searchResp.Chunks, "tag_kwd")
	if len(aggs) == 0 {
		return make(map[string]float64), nil
	}

	// Calculate total count (matching Python: cnt = np.sum([c for _, c in aggs]))
	cnt := 0
	for _, agg := range aggs {
		cnt += agg["count"].(int)
	}
	if cnt == 0 {
		return make(map[string]float64), nil
	}

	// Calculate weighted tag features
	// Formula: 0.1 * (c + 1) / (cnt + S) / max(1e-6, all_tags.get(a, 0.0001))
	S := 1000.0
	type tagScore struct {
		tag   string
		score float64
	}
	scoredTags := make([]tagScore, 0, len(aggs))

	for _, agg := range aggs {
		tag := agg["key"].(string)
		c := agg["count"].(int)
		allTagValue := allTags[tag]
		if allTagValue <= 0 {
			allTagValue = 0.0001
		}
		score := 0.1 * float64(c+1) / (float64(cnt) + S) / max(1e-6, allTagValue)
		scoredTags = append(scoredTags, tagScore{tag: tag, score: score})
	}

	// Sort by score descending (matching Python: sorted(..., key=lambda x: x[1] * -1))
	sort.Slice(scoredTags, func(i, j int) bool {
		return scoredTags[i].score > scoredTags[j].score
	})

	// Take top N tags and normalize dot notation (matching Python: a.replace(".", "_"))
	resultTags := make(map[string]float64)
	for i := 0; i < topnTags && i < len(scoredTags); i++ {
		tag := scoredTags[i].tag
		score := scoredTags[i].score
		normalizedTag := strings.ReplaceAll(tag, ".", "_")
		resultTags[normalizedTag] = max(1, score)
	}

	return resultTags, nil
}

// LabelQuestion returns rank features for a question based on KB's tag configuration
// Corresponds to rag/app/tag.py:label_question()
//
// Flow (matching Python):
//  1. Collect tag_kb_ids from KBs' parser_config
//  2. Try to get all_tags from cache (via GetTagsFromCache)
//  3. If cache miss, call GetAllTagsInPortion and cache the result (via SetTagsToCache)
//  4. Call TagQuery to get weighted tag features for the question
func (s *MetadataService) LabelQuestion(question string, kbRecords []*Knowledgebase) map[string]interface{} {
	if len(kbRecords) == 0 {
		return nil
	}

	// Collect tag_kb_ids from KBs' parser_config
	var tagKBBIDs []string
	var tenantID string
	for _, kb := range kbRecords {
		if kb.ParserConfig == nil {
			continue
		}
		tenantID = kb.TenantID

		if tagKBIDs, ok := kb.ParserConfig["tag_kb_ids"].([]interface{}); ok {
			for _, id := range tagKBIDs {
				if idStr, ok := id.(string); ok {
					tagKBBIDs = append(tagKBBIDs, idStr)
				}
			}
		}
	}

	if len(tagKBBIDs) == 0 || tenantID == "" {
		return nil
	}

	// Try to get all_tags from cache first (matching Python L134-139)
	allTags, err := GetTagsFromCache(tagKBBIDs)
	if err != nil {
		logger.Warn("Failed to get tags from cache", zap.Error(err))
	}
	if allTags == nil {
		// Cache miss - compute and cache
		allTags, err = s.GetAllTagsInPortion(tenantID, tagKBBIDs)
		if err != nil {
			logger.Warn("Failed to get all tags in portion", zap.Error(err))
			return nil
		}
		if len(allTags) > 0 {
			// Cache the result for 10 minutes
			if cacheErr := SetTagsToCache(tagKBBIDs, allTags); cacheErr != nil {
				logger.Warn("Failed to set tags cache", zap.Error(cacheErr))
			}
		}
	}

	if len(allTags) == 0 {
		return nil
	}

	// Get topn_tags from first KB's parser_config (default 3)
	topnTags := 3
	if kbRecords[0].ParserConfig != nil {
		if topn, ok := kbRecords[0].ParserConfig["topn_tags"].(int); ok {
			topnTags = topn
		}
	}

	// Query tags for the question
	tagFeatures, err := s.TagQuery(question, []string{tenantID}, tagKBBIDs, allTags, topnTags)
	if err != nil {
		logger.Warn("Failed to query tags", zap.Error(err))
		return nil
	}

	if len(tagFeatures) == 0 {
		return nil
	}

	// Convert float64 map to interface{} map
	result := make(map[string]interface{})
	for k, v := range tagFeatures {
		result[k] = v
	}
	return result
}
