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

package elasticsearch

import (
	"context"
	"fmt"

	"ragflow/internal/engine/types"
)

// GetChunk gets a chunk by ID
func (e *elasticsearchEngine) GetChunk(ctx context.Context, indexName, chunkID string, kbIDs []string) (interface{}, error) {
	// Build unified search request to get the chunk by ID
	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		Limit:      1,
		Offset:     0,
		MetaDataFilter: map[string]interface{}{
			"id": chunkID,
		},
	}

	// Execute search
	searchResp, err := e.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	if len(searchResp.Chunks) == 0 {
		return nil, nil
	}

	return searchResp.Chunks[0], nil
}
