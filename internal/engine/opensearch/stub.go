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

package opensearch

import (
	"context"
	"fmt"
	"ragflow/internal/engine/types"
)

// Placeholder stubs satisfying engine.DocEngine during incremental milestones.
// Each method will be replaced by concrete implementations in Commits 7-9.

func (e *Engine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return nil, fmt.Errorf("Search pending Commits 7-8")
}

func (e *Engine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	return nil
}

func (e *Engine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	return nil
}

func (e *Engine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	return nil
}

func (e *Engine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("RunSQL pending Commit 9")
}

func (e *Engine) GetChunkIDs(chunks []map[string]interface{}) []string {
	return nil
}

func (e *Engine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	return nil, fmt.Errorf("KNNScores pending Commit 8")
}

func (e *Engine) GetScores(searchResult map[string]interface{}) map[string]float64 {
	return nil
}
