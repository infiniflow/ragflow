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
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// indexNotFoundError marks a search against a physical index that does not
// exist yet (a tenant with no indexed documents). Callers that treat a
// missing index as an empty result — mirroring Python's opensearch_conn and
// the Elasticsearch Go engine — unwrap it via errors.As.
type indexNotFoundError struct {
	index string
}

func (e *indexNotFoundError) Error() string {
	return fmt.Sprintf("index not found: %s", e.index)
}

// isIndexNotFoundBody reports whether a 404 search error body carries an
// index_not_found_exception (as opposed to an unrelated 404).
func isIndexNotFoundBody(body []byte) bool {
	var failure struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &failure) == nil && failure.Error.Type == "index_not_found_exception"
}

func (e *Engine) dropIndex(ctx context.Context, indexName string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return nil
	}

	_, err = e.apiClient.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{
		Indices: []string{indexName},
	})
	if err != nil {
		return fmt.Errorf("failed to delete index: %w", err)
	}

	return nil
}

func (e *Engine) indexExists(ctx context.Context, indexName string) (bool, error) {
	if indexName == "" {
		return false, fmt.Errorf("index name cannot be empty")
	}

	// Use a raw HEAD request: opensearch-go v4's typed Indices.Exists turns a
	// 404 into an error (its do() treats any status > 299 as an error), which
	// would make "index absent" indistinguishable from a real failure.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "/"+indexName, nil)
	if err != nil {
		return false, err
	}
	resp, err := e.client.Perform(req)
	if err != nil {
		return false, fmt.Errorf("failed to check index existence: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("opensearch returned error status %d for index %s: %s", resp.StatusCode, indexName, string(bodyBytes))
	}
}

func buildMetadataIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
}
