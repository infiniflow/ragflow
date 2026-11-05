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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"go.uber.org/zap"
)

// osIndexNameForTenant maps one tenant id to its physical OpenSearch index.
// Regexp search deliberately has no multi-tenant mode.
func osIndexNameForTenant(tenantID string) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", fmt.Errorf("tenant id cannot be empty")
	}
	if strings.Contains(tenantID, ",") {
		return "", fmt.Errorf("regexp search supports exactly one tenant")
	}
	return "ragflow_" + tenantID, nil
}

// SearchByRegexp executes a regex-match-only search over chunk content.
// It emits a Lucene regexp query on content_with_weight (mapped as keyword).
// Satisfies the regexpSearchable interface in agentic_rag/grep_service.go.
func (e *Engine) SearchByRegexp(ctx context.Context, req *types.RegexpSearchRequest) (*types.SearchResult, error) {
	if req == nil {
		return nil, fmt.Errorf("regexp search request cannot be nil")
	}
	if e == nil || e.transportClient == nil {
		return nil, fmt.Errorf("opensearch engine not initialized")
	}

	indexName, err := osIndexNameForTenant(req.TenantID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Pattern) == "" {
		return nil, fmt.Errorf("regexp pattern cannot be empty")
	}

	const maxRegexpResults = 30
	maxResults := maxRegexpResults
	if req.ReturnAll {
		maxResults = 10000
	}

	boolQuery := buildBoolQueryFromCondition(req.Filter, req.KbIDs, false, false)

	regexpPattern := ".*(" + req.Pattern + ").*"
	regexpClause := map[string]interface{}{
		"regexp": map[string]interface{}{
			"content_with_weight": map[string]interface{}{
				"value":            regexpPattern,
				"flags":            "NONE",
				"case_insensitive": true,
			},
		},
	}

	if boolQuery == nil {
		boolQuery = map[string]interface{}{}
	}
	if boolMap, ok := boolQuery["bool"].(map[string]interface{}); ok {
		if must, ok := boolMap["must"].([]interface{}); ok {
			boolMap["must"] = append(must, regexpClause)
		} else {
			boolMap["must"] = []interface{}{regexpClause}
		}
	} else {
		boolQuery = map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{regexpClause},
			},
		}
	}

	var sortClause []map[string]interface{}
	if req.Sort != nil && len(req.Sort.Fields) > 0 {
		sortClause = parseOrderByExpr(req.Sort)
	}

	queryBody := map[string]interface{}{
		"query": boolQuery,
		"size":  maxResults,
		"from":  0,
	}

	if len(sortClause) > 0 {
		queryBody["sort"] = sortClause
	}

	if len(req.SelectFields) > 0 {
		queryBody["_source"] = req.SelectFields
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding regexp query: %w", err)
	}

	payload := append([]byte(nil), buf.Bytes()...)

	endpoint := fmt.Sprintf("/%s/_search", indexName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create regexp search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.transportClient.Stream(httpReq)
	if err != nil {
		common.Warn("OpenSearch regexp query failed", zap.String("index", indexName), zap.Error(err))
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		common.Warn("OpenSearch regexp error response", zap.String("index", indexName), zap.String("body", string(bodyBytes)))
		return nil, fmt.Errorf("opensearch regexp error on index %q: %s", indexName, strings.TrimSpace(string(bodyBytes)))
	}

	var osResp SearchResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&osResp)
	if decodeErr != nil {
		common.Warn("OpenSearch regexp failed to parse response", zap.String("index", indexName), zap.Error(decodeErr))
		return nil, fmt.Errorf("opensearch regexp parse error on index %q: %w", indexName, decodeErr)
	}
	if err := checkSearchResponse(&osResp, indexName); err != nil {
		return nil, err
	}

	allResults := convertOSResponse(&osResp)
	totalHits := osResp.Hits.Total.Value
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}
