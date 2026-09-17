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
	"ragflow/internal/engine/redis"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

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
func GetTagsFromCache(ctx context.Context, kbIDs []string) (map[string]float64, error) {
	if len(kbIDs) == 0 {
		return nil, nil
	}

	redisClient := redis.Get()
	if redisClient == nil {
		common.Warn("Redis client not available, skipping cache lookup")
		return nil, nil
	}

	key := getTagsCacheKey(kbIDs)
	data, err := redisClient.Get(ctx, key)
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
func SetTagsToCache(ctx context.Context, kbIDs []string, tags map[string]float64) error {
	if len(kbIDs) == 0 || tags == nil {
		return nil
	}

	redisClient := redis.Get()
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
	ok := redisClient.Set(ctx, key, string(data), 10*time.Minute)
	if !ok {
		common.Warn("Failed to set tags cache")
		return fmt.Errorf("failed to set tags cache")
	}

	return nil
}

// Knowledgebase type alias for entity.Knowledgebase
type Knowledgebase = entity.Knowledgebase

// GetAllTagsInPortion returns all tag_kwd values and their occurrence counts
// for documents belonging to the given kbIDs, aggregated across every tenant
// index in tenantIDs — the same tenant scope TagQuery searches. The dataset
// and chunk-search callers can supply authorized knowledgebases from multiple
// tenants, so restricting the aggregation to a single tenant index would omit
// tags that exist only in the other tenants' indices (leaving them absent from
// allTags and scored with the 0.0001 fallback, which inflates their scores).
func (s *MetadataService) GetAllTagsInPortion(ctx context.Context, tenantIDs []string, kbIDs []string) (map[string]float64, error) {
	if len(kbIDs) == 0 || len(tenantIDs) == 0 {
		return make(map[string]float64), nil
	}

	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = fmt.Sprintf("ragflow_%s", tenantID)
	}

	searchReq := &types.SearchRequest{
		IndexNames:   indexNames,
		KbIDs:        kbIDs,
		Offset:       0,
		Limit:        common.MAX_RESULT_WINDOW,
		SelectFields: []string{"tag_kwd"},
	}

	searchResp, err := s.docEngine.Search(ctx, searchReq)
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
func (s *MetadataService) TagQuery(ctx context.Context, question string, tenantIDs []string, kbIDs []string, allTags map[string]float64, topnTags int) (map[string]float64, error) {
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

	searchResp, err := s.docEngine.Search(ctx, searchReq)
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

// LabelQuestion returns rank features for a question based on the dataset's own
// tags.
//
// Flow:
//  1. Use the queried KBs themselves as the tag source and search scope
//  2. Try to get all_tags from cache (via GetTagsFromCache)
//  3. If cache miss, call GetAllTagsInPortion and cache the result (via SetTagsToCache)
//  4. Call TagQuery to get weighted tag features for the question
//
// The Go backend has no separate tag-library dataset as the Python backend
// does. Instead the Go extractor (extractor_tag.go) writes tag_kwd onto each
// chunk during ingestion, so the authoritative tag vocabulary lives on the
// dataset's own chunks and is aggregated from the KBs being queried.
func (s *MetadataService) LabelQuestion(ctx context.Context, question string, kbs []*Knowledgebase) map[string]float64 {
	if len(kbs) == 0 || question == "" {
		return nil
	}

	// Use the queried KBs themselves as both the tag source and the search
	// scope. Track the last KB (for tenant_id / topn_tags) and the unique
	// tenant IDs spanned by the KBs.
	var kbIDs []string
	var lastKB *Knowledgebase
	tenantIDSet := make(map[string]bool)
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		lastKB = kb
		kbIDs = append(kbIDs, kb.ID)
		if kb.TenantID != "" {
			tenantIDSet[kb.TenantID] = true
		}
	}
	if len(kbIDs) == 0 {
		return nil
	}
	uniqueTenantIDs := make([]string, 0, len(tenantIDSet))
	for tid := range tenantIDSet {
		uniqueTenantIDs = append(uniqueTenantIDs, tid)
	}
	if len(uniqueTenantIDs) == 0 {
		return nil
	}

	// Aggregate tag_kwd across the dataset's own chunks (cached by KB set).
	allTags, err := GetTagsFromCache(ctx, kbIDs)
	if err != nil {
		common.Warn("Failed to get tags from cache", zap.Error(err))
	}
	if allTags == nil {
		// Cache miss - compute all_tags_in_portion
		allTags, err = s.GetAllTagsInPortion(ctx, uniqueTenantIDs, kbIDs)
		if err != nil {
			common.Warn("Failed to get all tags in portion", zap.Error(err))
			return nil
		}
		// Store in cache for future lookups
		if err = SetTagsToCache(ctx, kbIDs, allTags); err != nil {
			common.Warn("Failed to set tags cache", zap.Error(err))
		}
	}

	// No tag_kwd on the dataset's chunks means tagging is not in effect for
	// this query: return nil so the caller applies no tag boost (Python's
	// label_question returning None).
	if len(allTags) == 0 {
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

	// Query tags for the question across the dataset's own chunks.
	tagFeatures, err := s.TagQuery(ctx, question, uniqueTenantIDs, kbIDs, allTags, topnTags)
	if err != nil {
		return nil
	}
	if len(tagFeatures) == 0 {
		// Tags configured but the question matched none - return empty map
		// (not nil) so the caller knows tagging was active for this dataset.
		return make(map[string]float64)
	}

	return tagFeatures
}
