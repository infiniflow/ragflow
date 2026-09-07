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

package chunk

import (
	"context"
	"encoding/json"
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
//
// The returned map has an entry for every requested chunkID.  Each vector
// slice is independently allocated — callers may safely modify them.
func FetchChunkVectors(ctx context.Context, engine vectorFetcher, chunkIDs, tenantIDs, kbIDs []string, dim int) map[string][]float64 {
	out := make(map[string][]float64, len(chunkIDs))

	if len(chunkIDs) == 0 || dim <= 0 {
		return out
	}

	// Infinity already ships vectors with chunks; no need to fetch.
	// TODO: OceanBase engine is not yet implemented — add "oceanbase" here when it lands.
	if engine.GetType() == "infinity" || engine.GetType() == "oceanbase" {
		for _, cid := range chunkIDs {
			out[cid] = zeroVector(dim)
		}
		return out
	}

	vecField := fmt.Sprintf("q_%d_vec", dim)

	// Convert chunkIDs to []interface{} because the ES filter builder
	// (buildBoolQueryFromCondition) only handles []interface{} for the
	// "id" key — passing []string would be silently dropped.
	idList := make([]interface{}, len(chunkIDs))
	for i, cid := range chunkIDs {
		idList[i] = cid
	}

	// Query each tenant index for the requested chunk vectors.
	for _, tid := range tenantIDs {
		idxName := fmt.Sprintf("ragflow_%s", tid)
		res, err := engine.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{idxName},
			KbIDs:        kbIDs,
			SelectFields: []string{vecField},
			Filter:       map[string]interface{}{"id": idList},
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
			if v := parseVectorField(chunk, vecField, dim); v != nil {
				out[cid] = v
			} else {
				out[cid] = zeroVector(dim)
			}
		}
	}

	// Fill any chunk IDs not found across all indices with independently
	// allocated zero vectors so callers cannot corrupt each other.
	for _, cid := range chunkIDs {
		if _, exists := out[cid]; !exists {
			out[cid] = zeroVector(dim)
		}
	}

	return out
}

// zeroVector returns a freshly allocated zero vector of the given dimension.
func zeroVector(dim int) []float64 {
	return make([]float64, dim)
}

// parseVectorField extracts a vector from a chunk map. ES stores vectors
// as tab-separated strings; Infinity stores them as []float64 / []interface{}.
// Returns nil when the vector cannot be extracted or has the wrong dimension.
func parseVectorField(chunk map[string]interface{}, field string, dim int) []float64 {
	raw, ok := chunk[field]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case string:
		return parseVectorString(v, dim)
	case []float64:
		if len(v) == dim {
			out := make([]float64, dim)
			copy(out, v)
			return out
		}
	case []interface{}:
		vec := make([]float64, len(v))
		for i, val := range v {
			switch fv := val.(type) {
			case float64:
				vec[i] = fv
			case float32:
				vec[i] = float64(fv)
			case json.Number:
				f, err := fv.Float64()
				if err != nil {
					return nil
				}
				vec[i] = f
			case string:
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil
				}
				vec[i] = f
			default:
				return nil
			}
		}
		if len(vec) == dim {
			return vec
		}
	}
	return nil
}

// parseVectorString parses a tab-separated vector string from ES.
// Returns nil when parsing fails or the dimension does not match.
func parseVectorString(s string, dim int) []float64 {
	parts := strings.Split(s, "\t")
	if len(parts) != dim {
		return nil
	}
	vec := make([]float64, dim)
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil
		}
		vec[i] = f
	}
	return vec
}
