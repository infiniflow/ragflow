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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ragflow/internal/engine/types"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// dropIndex deletes an index in OpenSearch. Missing indices are tolerated.
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

	resp, err := e.apiClient.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{
		Indices: []string{indexName},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer resp.Inspect().Response.Body.Close()
		if resp.Inspect().Response.StatusCode == 404 {
			return nil
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(bodyBytes)
			if reason != "" {
				return fmt.Errorf("opensearch error: %s", reason)
			}
			return fmt.Errorf("opensearch returned error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete index: %w", err)
	}

	return nil
}

// indexExists checks if an index exists in OpenSearch.
func (e *Engine) indexExists(ctx context.Context, indexName string) (bool, error) {
	if indexName == "" {
		return false, fmt.Errorf("index name cannot be empty")
	}

	resp, err := e.apiClient.Indices.Exists(ctx, opensearchapi.IndicesExistsReq{
		Indices: []string{indexName},
	})
	if resp != nil {
		if resp.Body != nil {
			defer resp.Body.Close()
		}
		if resp.StatusCode == 200 {
			return true, nil
		}
		if resp.StatusCode == 404 {
			return false, nil
		}
	}
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check index existence: %w", err)
	}

	return false, nil
}

// buildMetadataIndexName returns the metadata index name for a tenant.
func buildMetadataIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
}

// isNotFoundError reports whether err represents a 404 or missing index/resource.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, types.ErrIndexNotFound) || errors.Is(err, types.ErrDocumentNotFound) {
		return true
	}
	var structErr *opensearch.StructError
	if errors.As(err, &structErr) {
		if structErr.Status == http.StatusNotFound ||
			structErr.Err.Type == "index_not_found_exception" ||
			structErr.Err.Type == "resource_not_found_exception" {
			return true
		}
	}
	var sErr opensearch.StructError
	if errors.As(err, &sErr) {
		if sErr.Status == http.StatusNotFound ||
			sErr.Err.Type == "index_not_found_exception" ||
			sErr.Err.Type == "resource_not_found_exception" {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "index_not_found_exception") ||
		strings.Contains(msg, "resource_not_found_exception")
}

// extractErrorReason extracts error reason from OpenSearch JSON error body.
func extractErrorReason(bodyBytes []byte) string {
	if len(bodyBytes) == 0 {
		return ""
	}
	var errResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		return ""
	}

	if errStr, ok := errResp["error"].(string); ok && errStr != "" {
		return errStr
	}
	if errorObj, ok := errResp["error"].(map[string]interface{}); ok {
		if rootCauses, ok := errorObj["root_cause"].([]interface{}); ok && len(rootCauses) > 0 {
			if rootCause, ok := rootCauses[0].(map[string]interface{}); ok {
				if reason, ok := rootCause["reason"].(string); ok && reason != "" {
					return reason
				}
			}
		}
		if reason, ok := errorObj["reason"].(string); ok && reason != "" {
			return reason
		}
		if failedShards, ok := errorObj["failed_shards"].([]interface{}); ok && len(failedShards) > 0 {
			if shard, ok := failedShards[0].(map[string]interface{}); ok {
				if reason, ok := shard["reason"].(map[string]interface{}); ok {
					if r, ok := reason["reason"].(string); ok && r != "" {
						return r
					}
				}
			}
		}
	}

	return ""
}


