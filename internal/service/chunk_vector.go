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
	"strconv"
	"strings"

	"ragflow/internal/engine/types"
	"ragflow/internal/common"

	"go.uber.org/zap"
)

// vectorFetcher is the consumer-side interface for chunk vector hydration.
type vectorFetcher interface {
	Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error)
	GetType() string
}

// FetchChunkVectors fetches embedding vectors for a set of chunk IDs.
// This is used by citation insertion (insert_citations) to hydrate chunk
// vectors on demand, since the main retrieval path skips vector transport.
//
// On Infinity / OceanBase the chunks already carry vectors, so we skip
// the round-trip. On ES we query by chunk ID list.
//
// Degrades gracefully: if the engine returns an error, zero vectors are
// returned for all chunk IDs rather than failing the caller.
func FetchChunkVectors(engine vectorFetcher, chunkIDs, tenantIDs, kbIDs []string, dim int) map[string][]float64 {
	out := make(map[string][]float64, len(chunkIDs))
	zero := make([]float64, dim)

	if len(chunkIDs) == 0 {
		return out
	}

	// Infinity and OceanBase already ship vectors with chunks; no need to fetch.
	if engine.GetType() == "infinity" || engine.GetType() == "oceanbase" {
		for _, cid := range chunkIDs {
			out[cid] = zero
		}
		return out
	}

	vecField := fmt.Sprintf("q_%d_vec", dim)

	// Query each tenant index for the requested chunk vectors.
	for _, tid := range tenantIDs {
		idxName := fmt.Sprintf("ragflow_%s", tid)
		res, err := engine.Search(context.Background(), &types.SearchRequest{
			IndexNames:   []string{idxName},
			KbIDs:        kbIDs,
			SelectFields: []string{vecField},
			Filter:       map[string]interface{}{"id": chunkIDs},
			Limit:        len(chunkIDs),
		})
		if err != nil {
			common.Warn("FetchChunkVectors search failed, using zero vectors",
				zap.String("index", idxName),
				zap.String("error", err.Error()))
			continue
		}

		for _, chunk := range res.Chunks {
			cid, _ := chunk["id"].(string)
			if cid == "" {
				continue
			}
			if _, exists := out[cid]; exists {
				continue
			}
			out[cid] = parseVectorField(chunk, vecField, dim, zero)
		}
	}

	// Fill any chunk IDs not found across all indices.
	for _, cid := range chunkIDs {
		if _, exists := out[cid]; !exists {
			out[cid] = zero
		}
	}

	return out
}

// parseVectorField extracts a vector from a chunk map. ES stores vectors
// as tab-separated strings; Infinity stores them as []float64 / []interface{}.
func parseVectorField(chunk map[string]interface{}, field string, dim int, zero []float64) []float64 {
	raw, ok := chunk[field]
	if !ok {
		return zero
	}
	switch v := raw.(type) {
	case string:
		return parseVectorString(v, dim, zero)
	case []float64:
		if len(v) == dim {
			return v
		}
	case []interface{}:
		vec := make([]float64, len(v))
		for i, val := range v {
			switch fv := val.(type) {
			case float64:
				vec[i] = fv
			case float32:
				vec[i] = float64(fv)
			case string:
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return zero
				}
				vec[i] = f
			default:
				return zero
			}
		}
		if len(vec) == dim {
			return vec
		}
	}
	return zero
}

// parseVectorString parses a tab-separated vector string from ES.
func parseVectorString(s string, dim int, zero []float64) []float64 {
	parts := strings.Split(s, "\t")
	if len(parts) != dim {
		return zero
	}
	vec := make([]float64, dim)
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return zero
		}
		vec[i] = f
	}
	return vec
}
