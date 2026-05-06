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
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/cache"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/service/nlp"

	"github.com/cespare/xxhash/v2"
)

// getTagsCacheKey generates a cache key from kb_ids using xxhash64
func getTagsCacheKey(kbIDs []string) string {
	// Normalize: unique + sorted so the key is set-stable regardless of caller order.
	seen := make(map[string]struct{}, len(kbIDs))
	norm := make([]string, 0, len(kbIDs))
	for _, id := range kbIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		norm = append(norm, id)
	}
	sort.Strings(norm)
	hasher := xxhash.New()
	hasher.Write([]byte(strings.Join(norm, "\x00")))
	return fmt.Sprintf("%x", hasher.Sum64())
}

// GetTagsFromCache retrieves cached tags for given kb_ids
// Returns nil if not found (cache miss)
func GetTagsFromCache(kbIDs []string) (map[string]float64, error) {
	if len(kbIDs) == 0 {
		return nil, nil
	}

	redisClient := cache.Get()
	if redisClient == nil {
		common.Warn("Redis client not available, skipping cache lookup")
		return nil, nil
	}

	key := getTagsCacheKey(kbIDs)
	data, err := redisClient.Get(key)
	if err != nil || data == "" {
		// Cache miss or error
		return nil, nil
	}

	var tags map[string]float64
	if err := json.Unmarshal([]byte(data), &tags); err != nil {
		common.Warn("Failed to unmarshal cached tags", zap.Error(err))
		return nil, nil
	}

	return tags, nil
}

// SetTagsToCache stores tags in cache for given kb_ids with 10 minute expiry
func SetTagsToCache(kbIDs []string, tags map[string]float64) error {
	if len(kbIDs) == 0 || tags == nil {
		return nil
	}

	redisClient := cache.Get()
	if redisClient == nil {
		common.Warn("Redis client not available, skipping cache store")
		return nil
	}

	key := getTagsCacheKey(kbIDs)
	data, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags for cache: %w", err)
	}

	// Cache for 10 minutes (600 seconds)
	ok := redisClient.Set(key, string(data), 10*time.Minute)
	if !ok {
		common.Warn("Failed to set tags cache")
		return fmt.Errorf("failed to set tags cache")
	}

	return nil
}

// Knowledgebase type alias for entity.Knowledgebase
type Knowledgebase = entity.Knowledgebase

// GetAllTagsInPortion returns the tag distribution for given KBs
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

	// Use GetAggregation for tag counting
	tagAgg := s.docEngine.GetAggregation(searchResp.Chunks, "tag_kwd")
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
func (s *MetadataService) TagQuery(question string, tenantIDs []string, kbIDs []string, allTags map[string]float64, topnTags int) (map[string]float64, error) {
	if len(kbIDs) == 0 || len(allTags) == 0 || len(tenantIDs) == 0 {
		return make(map[string]float64), nil
	}

	// Build index names for all tenant IDs
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = fmt.Sprintf("ragflow_%s", tenantID)
	}

	// Process question to get match text
	queryBuilder := nlp.GetQueryBuilder()
	matchTextExpr, warns := queryBuilder.Question(question, "qa", 0.0) // min_match=0.0
	if len(warns) > 0 {
		common.Warn("TagQuery: failed to build match text", zap.Any("warnings", warns))
		return make(map[string]float64), nil
	}
	matchText := matchTextExpr.MatchingText

	common.Debug("TagQuery match_text", zap.String("match_text", matchText))

	// Search with match text to get relevant docs
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

	// Use GetAggregation for tag counting
	aggs := s.docEngine.GetAggregation(searchResp.Chunks, "tag_kwd")
	if len(aggs) == 0 {
		return make(map[string]float64), nil
	}

	// Calculate total count
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

	// Sort by score descending
	sort.Slice(scoredTags, func(i, j int) bool {
		return scoredTags[i].score > scoredTags[j].score
	})

	// Take top N tags and normalize dot notation
	resultTags := make(map[string]float64)
	for i := 0; i < topnTags && i < len(scoredTags); i++ {
		normalizedTag := strings.ReplaceAll(scoredTags[i].tag, ".", "_")
		score := max(1.0, scoredTags[i].score)
		if existing, ok := resultTags[normalizedTag]; !ok || score > existing {
			resultTags[normalizedTag] = score
		}
	}

	return resultTags, nil
}

// LabelQuestion returns rank features for a question based on KB's tag configuration.
//
// Flow:
//  1. Collect tag_kb_ids from KBs' parser_config
//  2. Try to get all_tags from cache (via GetTagsFromCache)
//  3. If cache miss, call GetAllTagsInPortion and cache the result (via SetTagsToCache)
//  4. Get tag KBs by IDs
//  5. Call TagQuery to get weighted tag features for the question
func (s *MetadataService) LabelQuestion(question string, kbs []*Knowledgebase) map[string]float64 {
	if len(kbs) == 0 {
		return nil
	}

	// Collect tag_kb_ids from KBs' parser_config and track last KB
	var tagKBIDs []string
	var lastKB *Knowledgebase
	for _, kb := range kbs {
		if kb.ParserConfig == nil {
			continue
		}
		lastKB = kb
		if rawTagKBIDs, ok := kb.ParserConfig["tag_kb_ids"].([]interface{}); ok {
			for _, id := range rawTagKBIDs {
				if idStr, ok := id.(string); ok {
					tagKBIDs = append(tagKBIDs, idStr)
				}
			}
		}
	}

	if len(tagKBIDs) == 0 {
		return nil
	}

	common.Debug("tag_kb_ids found in parser_config", zap.Strings("tag_kb_ids", tagKBIDs))

	// Get all tags from cache or compute and cache
	allTags, err := GetTagsFromCache(tagKBIDs)
	if err != nil {
		common.Warn("Failed to get tags from cache", zap.Error(err))
	}
	if allTags == nil {
		// Cache miss - compute all_tags_in_portion
		allTags, err = s.GetAllTagsInPortion(lastKB.TenantID, tagKBIDs)
		if err != nil {
			common.Warn("Failed to get all tags in portion", zap.Error(err))
			return nil
		}
		// Store in cache for future lookups
		if err := SetTagsToCache(tagKBIDs, allTags); err != nil {
			common.Warn("Failed to set tags cache", zap.Error(err))
		}
	}

	// Get tag_kbs by IDs
	kbDAO := dao.NewKnowledgebaseDAO()
	tagKBs, err := kbDAO.GetByIDs(tagKBIDs)
	if err != nil || len(tagKBs) == 0 {
		// Return nil if no tag_kbs found
		return nil
	}

	// Get unique tenant IDs from tag_kbs
	tenantIDSet := make(map[string]bool)
	for _, kb := range tagKBs {
		tenantIDSet[kb.TenantID] = true
	}
	var uniqueTenantIDs []string
	for tid := range tenantIDSet {
		uniqueTenantIDs = append(uniqueTenantIDs, tid)
	}
	if len(uniqueTenantIDs) == 0 {
		return nil
	}

	// Get topn_tags from last KB's parser_config
	// JSON-decoded numbers arrive as float64; also tolerate int/int64/json.Number for safety
	topnTags := 3
	if lastKB != nil && lastKB.ParserConfig != nil {
		switch v := lastKB.ParserConfig["topn_tags"].(type) {
		case float64:
			topnTags = int(v)
		case int:
			topnTags = v
		case int64:
			topnTags = int(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				topnTags = int(n)
			}
		}
	}

	// Query tags for the question using unique tenant IDs
	tagFeatures, err := s.TagQuery(question, uniqueTenantIDs, tagKBIDs, allTags, topnTags)
	if err != nil {
		return nil
	}
	if len(tagFeatures) == 0 {
		// Tag kb exists but returned no matching tags - return empty map (not nil)
		// so caller knows tag kb was configured vs not configured at all
		return make(map[string]float64)
	}

	return tagFeatures
}
