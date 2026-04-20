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
	"encoding/json"
	"fmt"
	"time"

	"ragflow/internal/cache"
	"ragflow/internal/logger"

	"github.com/cespare/xxhash"
	"go.uber.org/zap"
)

// getTagsCacheKey generates a cache key from kb_ids using xxhash64
// Corresponds to rag/graphrag/utils.py:get_tags_from_cache()
func getTagsCacheKey(kbIDs []string) string {
	hasher := xxhash.New()
	hasher.Write([]byte(fmt.Sprintf("%v", kbIDs)))
	return fmt.Sprintf("%x", hasher.Sum64())
}

// GetTagsFromCache retrieves cached tags for given kb_ids
// Returns nil if not found (cache miss)
// Corresponds to rag/graphrag/utils.py:get_tags_from_cache()
func GetTagsFromCache(kbIDs []string) (map[string]float64, error) {
	if len(kbIDs) == 0 {
		return nil, nil
	}

	redisClient := cache.Get()
	if redisClient == nil {
		logger.Warn("Redis client not available, skipping cache lookup")
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
		logger.Warn("Failed to unmarshal cached tags", zap.Error(err))
		return nil, nil
	}

	return tags, nil
}

// SetTagsToCache stores tags in cache for given kb_ids with 10 minute expiry
// Corresponds to rag/graphrag/utils.py:set_tags_to_cache()
func SetTagsToCache(kbIDs []string, tags map[string]float64) error {
	if len(kbIDs) == 0 || tags == nil {
		return nil
	}

	redisClient := cache.Get()
	if redisClient == nil {
		logger.Warn("Redis client not available, skipping cache store")
		return nil
	}

	key := getTagsCacheKey(kbIDs)
	data, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags for cache: %w", err)
	}

	// Cache for 10 minutes (600 seconds), matching Python
	ok := redisClient.Set(key, string(data), 10*time.Minute)
	if !ok {
		logger.Warn("Failed to set tags cache")
		return fmt.Errorf("failed to set tags cache")
	}

	return nil
}