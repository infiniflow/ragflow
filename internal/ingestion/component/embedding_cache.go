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

package component

import (
	"context"
	"encoding/json"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/schema"

	"go.uber.org/zap"
)

const embeddingCacheWriteBudget = 32 << 20

// embeddingCacheForWrite checks an upper bound before backfilling a document's
// cache misses. Cache lookup and indexing still work when writes are skipped.
func embeddingCacheForWrite(store chunkcache.Store, modelID string, results []EmbeddingResult) chunkcache.Store {
	if store == nil || modelID == "" {
		return nil
	}
	// Include the JSON encoding, key and manifest overhead. A finite float64
	// encoded by encoding/json plus a separator fits in 32 bytes.
	const bytesPerFloat = 32
	const bytesPerEntry = 256
	remaining := embeddingCacheWriteBudget
	for _, result := range results {
		if remaining < bytesPerEntry || len(result.Vector) > (remaining-bytesPerEntry)/bytesPerFloat {
			common.Info("Skipping embedding cache writes for large run",
				zap.Int("cache_misses", len(results)), zap.Int("write_budget_bytes", embeddingCacheWriteBudget))
			return nil
		}
		remaining -= bytesPerEntry + len(result.Vector)*bytesPerFloat
	}
	return store
}

func cacheEmbedding(ctx context.Context, store chunkcache.Store, modelID string, chunk schema.ChunkDoc, input string, vector []float64) {
	if store == nil {
		return
	}
	chunkID, ok := chunk.GetExtraString("id")
	if !ok {
		return
	}
	data, err := json.Marshal(vector)
	if err == nil {
		chunkcache.Set(ctx, store, chunkcache.Key("emb", modelID, chunkID, input), string(data))
	}
}
