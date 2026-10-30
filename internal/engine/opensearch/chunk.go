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
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"
)

var memoryMessageVectorFieldRE = regexp.MustCompile(`^q_\d+_vec$`)

// CreateChunkStore creates a chunk index in OpenSearch.
func (e *Engine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID, language string) error {
	if baseName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index already exists
	exists, err := e.indexExists(ctx, baseName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		if strings.HasPrefix(baseName, "memory_") {
			if vectorSize > 0 {
				if err := e.ensureMemoryMessageVectorMapping(ctx, baseName, vectorSize); err != nil {
					return fmt.Errorf("failed to ensure memory vector mapping: %w", err)
				}
				common.Info("Memory index already exists, ensured vector mapping", zap.String("index_name", baseName), zap.Int("vector_size", vectorSize))
			}
			return nil
		}
		if vectorSize > 0 {
			if err := e.ensureChunkVectorMapping(ctx, baseName, vectorSize); err != nil {
				return fmt.Errorf("failed to ensure chunk vector mapping: %w", err)
			}
			common.Info("Chunk index already exists, ensured vector mapping", zap.String("index_name", baseName), zap.Int("vector_size", vectorSize))
		}
		common.Info("Index already exists, skipping creation", zap.String("index_name", baseName))
		return nil
	}

	// Load mapping based on index type
	var mapping map[string]interface{}
	if strings.HasPrefix(baseName, "memory_") {
		mapping = getMemoryMessageMapping(vectorSize)
	} else if datasetID == "skill" {
		skillMapping, err := loadSkillMapping()
		if err != nil {
			return fmt.Errorf("failed to load skill mapping: %w", err)
		}
		if vectorSize > 0 {
			if mappings, ok := skillMapping["mappings"].(map[string]interface{}); ok {
				if props, ok := mappings["properties"].(map[string]interface{}); ok {
					vecField := fmt.Sprintf("q_%d_vec", vectorSize)
					if _, exists := props[vecField]; !exists {
						props[vecField] = chunkVectorProperty(vectorSize)
					}
				}
			}
		}
		mapping = skillMapping
	} else {
		// OpenSearch settings and mappings for chunk index.
		// Template ragflow_os_mapping matches ragflow_*, but explicit properties guarantee tag_feas and index.knn.
		properties := map[string]interface{}{
			"tag_feas": map[string]interface{}{
				"type": "rank_features",
			},
		}
		if vectorSize > 0 {
			properties[memoryMessageVectorField(vectorSize)] = memoryMessageVectorProperty(vectorSize)
		}
		mapping = map[string]interface{}{
			"settings": map[string]interface{}{
				"index.knn":          true,
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
			"mappings": map[string]interface{}{
				"properties": properties,
			},
		}
	}

	var body io.Reader
	if mapping != nil {
		data, err := json.Marshal(mapping)
		if err != nil {
			return fmt.Errorf("failed to marshal mapping: %w", err)
		}
		body = bytes.NewReader(data)
	}

	resp, err := e.apiClient.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: baseName,
		Body:  body,
	})
	createFailed := err != nil || (resp != nil && resp.Inspect().Response != nil && resp.Inspect().Response.StatusCode >= 400)
	if createFailed {
		if nowExists, exErr := e.indexExists(ctx, baseName); exErr == nil && nowExists {
			if resp != nil && resp.Inspect().Response != nil {
				_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
				_ = resp.Inspect().Response.Body.Close()
			}
			if strings.HasPrefix(baseName, "memory_") && vectorSize > 0 {
				return e.ensureMemoryMessageVectorMapping(ctx, baseName, vectorSize)
			}
			if vectorSize > 0 && datasetID != "skill" {
				return e.ensureChunkVectorMapping(ctx, baseName, vectorSize)
			}
			common.Info("Index created concurrently by another worker", zap.String("index_name", baseName))
			return nil
		}
	}
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(bodyBytes)
			if reason != "" {
				return fmt.Errorf("opensearch error: %s", reason)
			}
			return fmt.Errorf("opensearch returned error: %s, body: %s", resp.Inspect().Response.Status(), string(bodyBytes))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	common.Info("Successfully created OpenSearch index", zap.String("index_name", baseName))
	return nil
}

// InsertChunks inserts chunks into an OpenSearch index and waits for refresh.
func (e *Engine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string, language string) ([]string, error) {
	return e.insertChunks(ctx, chunks, baseName, datasetID, language, "wait_for")
}

// InsertChunksNoRefresh inserts chunks into OpenSearch without waiting for index refresh.
func (e *Engine) InsertChunksNoRefresh(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string, language string) ([]string, error) {
	return e.insertChunks(ctx, chunks, baseName, datasetID, language, "")
}

func (e *Engine) insertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string, language string, refresh string) ([]string, error) {
	common.Info("OpenSearchConnection.InsertChunks called", zap.String("index_name", baseName), zap.Int("chunkCount", len(chunks)))

	if len(chunks) == 0 {
		return []string{}, nil
	}

	if baseName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	isMemoryIndex := strings.HasPrefix(baseName, "memory_")

	// Ensure chunk store exists before updating mappings or inserting chunks.
	// Ingestion can call InsertChunks directly without prior CreateChunkStore.
	exists, err := e.ChunkStoreExists(ctx, baseName, datasetID)
	if err != nil {
		return nil, fmt.Errorf("check chunk store exists: %w", err)
	}
	if !exists {
		firstVectorSize := 0
		for _, chunk := range chunks {
			for field := range chunk {
				if size, ok := parseChunkVectorSize(field); ok {
					firstVectorSize = size
					break
				}
				if size, ok := parseMemoryMessageVectorSize(field); ok {
					firstVectorSize = size
					break
				}
			}
			if firstVectorSize > 0 {
				break
			}
		}
		if err := e.CreateChunkStore(ctx, baseName, datasetID, firstVectorSize, "", language); err != nil {
			return nil, fmt.Errorf("create chunk store for %s: %w", baseName, err)
		}
	}

	if isMemoryIndex {
		if err := e.ensureMemoryMessageVectorMappingsForDocs(ctx, baseName, chunks); err != nil {
			return nil, err
		}
	} else if datasetID != "skill" {
		if err := e.ensureChunkVectorMappingsForDocs(ctx, baseName, chunks); err != nil {
			return nil, err
		}
	}

	const (
		bulkBatchChunks = 500
		bulkBatchBytes  = 20 * 1024 * 1024 // 20MB
	)

	flush := func(buf *bytes.Buffer) error {
		if buf.Len() == 0 {
			return nil
		}
		resp, err := e.apiClient.Bulk(ctx, opensearchapi.BulkReq{
			Body: bytes.NewReader(buf.Bytes()),
			Params: opensearchapi.BulkParams{
				Refresh: refresh,
			},
		})
		if resp != nil && resp.Inspect().Response != nil {
			defer func() {
				_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
				_ = resp.Inspect().Response.Body.Close()
			}()
			if resp.Inspect().Response.StatusCode >= 400 {
				bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
				return fmt.Errorf("opensearch bulk request returned error: %s, body: %s", resp.Inspect().Response.Status(), string(bodyBytes))
			}
		}
		if err != nil {
			return fmt.Errorf("failed to execute bulk request: %w", err)
		}

		if resp.Errors {
			var reasons []string
			for _, itemMap := range resp.Items {
				for _, item := range itemMap {
					if item.Error != nil && item.Error.Reason != "" {
						reasons = append(reasons, item.Error.Reason)
					}
				}
			}
			totalErrors := len(reasons)
			displayReasons := reasons
			if len(displayReasons) > 5 {
				displayReasons = displayReasons[:5]
			}
			common.Error("OpenSearch bulk request had item errors",
				fmt.Errorf("index %s: %d failed items; first reasons: %v", baseName, totalErrors, displayReasons))
			return fmt.Errorf("opensearch bulk request had %d item errors (index %s); first reasons: %v",
				totalErrors, baseName, displayReasons)
		}
		return nil
	}

	var buf bytes.Buffer
	batchChunkCount := 0
	for _, doc := range chunks {
		if isMemoryIndex {
			doc = mapMemoryMessageToESDoc(doc)
		}
		docID, _ := doc["doc_id"].(string)
		chunkID, _ := doc["id"].(string)
		if docID == "" || chunkID == "" {
			common.Warn("Skipping chunk without doc_id or id")
			continue
		}

		action, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{
				"_index": baseName,
				"_id":    chunkID,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal bulk action: %w", err)
		}
		buf.Write(action)
		buf.WriteByte('\n')

		docCopy := copyFields(doc)
		docCopy["kb_id"] = datasetID
		if !isMemoryIndex && docCopy["available_int"] == nil {
			docCopy["available_int"] = 1
		}
		if raw, ok := formatOrderedTagFeas(docCopy["tag_feas"]); ok {
			docCopy["tag_feas"] = raw
		}
		if err := json.NewEncoder(&buf).Encode(docCopy); err != nil {
			return nil, fmt.Errorf("failed to encode document: %w", err)
		}

		batchChunkCount++
		if batchChunkCount >= bulkBatchChunks || buf.Len() >= bulkBatchBytes {
			if err := flush(&buf); err != nil {
				return nil, err
			}
			buf.Reset()
			batchChunkCount = 0
		}
	}

	if err := flush(&buf); err != nil {
		return nil, err
	}

	common.Info("OpenSearchConnection.InsertChunks result", zap.String("index_name", baseName), zap.Int("count", len(chunks)))
	return []string{}, nil
}

// UpdateChunks updates chunks matching condition with newValue.
// It forces condition["kb_id"] = datasetID to strictly prevent cross-dataset mutation.
func (e *Engine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	fullIndexName := baseName
	common.Info("OpenSearchConnection.UpdateChunks called", zap.String("index_name", fullIndexName), zap.Any("condition", condition), zap.Any("new_value", newValue))

	if fullIndexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	exists, err := e.indexExists(ctx, fullIndexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: '%s'", types.ErrIndexNotFound, fullIndexName)
	}

	if strings.HasPrefix(fullIndexName, "memory_") {
		condition["memory_id"] = datasetID
		if messageDocID, ok := condition["id"].(string); ok {
			return e.updateSingleMemoryMessage(ctx, fullIndexName, messageDocID, newValue)
		}
		return e.updateChunksByQuery(ctx, fullIndexName, mapMemoryMessageESConditionFields(condition), mapMemoryMessageESUpdateFields(newValue))
	}

	// Always enforce dataset isolation
	condition["kb_id"] = datasetID

	// Case 1: Single document update
	if chunkID, ok := condition["id"].(string); ok {
		return e.updateSingleChunk(ctx, fullIndexName, chunkID, datasetID, newValue)
	}

	// Case 2: Multi-document update via UpdateByQuery
	return e.updateChunksByQuery(ctx, fullIndexName, condition, newValue)
}

// AdjustChunkPagerank atomically adjusts pagerank_fea and clamps it to [minWeight, maxWeight].
// If next weight <= 0.0, the field is removed from _source because OpenSearch rank_features must be strictly positive.
func (e *Engine) AdjustChunkPagerank(ctx context.Context, indexName, chunkID, kbID string, delta, minWeight, maxWeight float64) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if chunkID == "" {
		return fmt.Errorf("chunk id cannot be empty")
	}
	script := `
		if (ctx._source.kb_id == null || !ctx._source.kb_id.equals(params.kb_id)) {
			ctx.op = 'noop';
		} else {
			double current = 0.0;
			if (ctx._source.containsKey(params.field) && ctx._source[params.field] != null) {
				Object currentValue = ctx._source[params.field];
				if (currentValue instanceof Number) {
					current = ((Number)currentValue).doubleValue();
				} else {
					try {
						current = Double.parseDouble(currentValue.toString());
					} catch (Exception e) {
						current = 0.0;
					}
				}
			}
			double next = current + params.delta;
			if (next < params.min_weight) {
				next = params.min_weight;
			}
			if (next > params.max_weight) {
				next = params.max_weight;
			}
			if (next <= 0.0) {
				ctx._source.remove(params.field);
			} else {
				ctx._source[params.field] = next;
			}
		}
	`
	body, err := json.Marshal(map[string]interface{}{
		"script": map[string]interface{}{
			"source": script,
			"lang":   "painless",
			"params": map[string]interface{}{
				"field":      common.PAGERANK_FLD,
				"kb_id":      kbID,
				"delta":      delta,
				"min_weight": minWeight,
				"max_weight": maxWeight,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal pagerank adjust request: %w", err)
	}

	retryOnConflict := 3
	resp, err := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
		Index:      indexName,
		DocumentID: chunkID,
		Body:       bytes.NewReader(body),
		Params: opensearchapi.UpdateParams{
			RetryOnConflict: &retryOnConflict,
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			return fmt.Errorf("opensearch pagerank adjust error: %s, body: %s", resp.Inspect().Response.Status(), string(bodyBytes))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to adjust chunk pagerank: %w", err)
	}

	if resp.Result == "noop" {
		return fmt.Errorf("chunk %s does not belong to dataset %s", chunkID, kbID)
	}
	return nil
}

func (e *Engine) updateSingleMemoryMessage(ctx context.Context, indexName, messageDocID string, newValue map[string]interface{}) error {
	doc := mapMemoryMessageESUpdateFields(newValue)
	delete(doc, "id")
	if len(doc) == 0 {
		return nil
	}

	updateBody := map[string]interface{}{"doc": doc}
	body, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal memory message update request: %w", err)
	}

	resp, err := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
		Index:      indexName,
		DocumentID: messageDocID,
		Body:       bytes.NewReader(body),
		Params: opensearchapi.UpdateParams{
			Refresh: "wait_for",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, messageDocID)
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			return fmt.Errorf("opensearch memory message update error: %s, body: %s", resp.Inspect().Response.Status(), string(bodyBytes))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to update memory message: %w", err)
	}
	return nil
}

func (e *Engine) updateSingleChunk(ctx context.Context, indexName, chunkID, datasetID string, newValue map[string]interface{}) error {
	common.Debug("OpenSearchConnection.updateSingleChunk called", zap.String("indexName", indexName), zap.String("chunkID", chunkID), zap.String("datasetID", datasetID))

	mustClauses := []map[string]interface{}{
		{"term": map[string]interface{}{"id": chunkID}},
	}
	if datasetID != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{"kb_id": datasetID},
		})
	}

	searchReq := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustClauses,
			},
		},
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return fmt.Errorf("failed to marshal search request: %w", err)
	}

	searchResp, err := e.apiClient.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(body),
	})
	if searchResp != nil && searchResp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, searchResp.Inspect().Response.Body)
			_ = searchResp.Inspect().Response.Body.Close()
		}()
		if searchResp.Inspect().Response.StatusCode >= 400 {
			return fmt.Errorf("failed to search for chunk: %s", searchResp.Inspect().Response.Status())
		}
	}
	if err != nil {
		return fmt.Errorf("failed to search for chunk: %w", err)
	}

	if searchResp.Timeout {
		return fmt.Errorf("search chunk %s in index %s timed out", chunkID, indexName)
	}
	if searchResp.Shards.Failed > 0 {
		return fmt.Errorf("search chunk %s in index %s encountered shard failures (%d failed shards)", chunkID, indexName, searchResp.Shards.Failed)
	}

	if len(searchResp.Hits.Hits) == 0 {
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
	}

	actualID := searchResp.Hits.Hits[0].ID
	if actualID == "" {
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
	}

	doc := copyFields(newValue)
	delete(doc, "id")

	removeValue, _ := doc["remove"]
	delete(doc, "remove")
	removeField, _ := removeValue.(string)
	removeDict, _ := removeValue.(map[string]interface{})

	addValue, _ := doc["add"]
	delete(doc, "add")
	addDict, _ := addValue.(map[string]interface{})

	// Remove *_feas fields
	var feasFields []string
	for k := range doc {
		if strings.HasSuffix(k, "feas") {
			feasFields = append(feasFields, k)
		}
	}
	for _, k := range feasFields {
		scriptBody := map[string]interface{}{
			"script": map[string]interface{}{
				"source": fmt.Sprintf("ctx._source.remove(\"%s\");", k),
			},
		}
		sBody, _ := json.Marshal(scriptBody)
		sResp, sErr := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
			Index:      indexName,
			DocumentID: actualID,
			Body:       bytes.NewReader(sBody),
		})
		if sResp != nil && sResp.Inspect().Response != nil {
			status := sResp.Inspect().Response.StatusCode
			_, _ = io.Copy(io.Discard, sResp.Inspect().Response.Body)
			_ = sResp.Inspect().Response.Body.Close()
			if status == http.StatusNotFound {
				return nil
			}
			if status >= 400 {
				return fmt.Errorf("failed to remove feas field %s on chunk %s, status: %d", k, chunkID, status)
			}
		}
		if sErr != nil {
			return fmt.Errorf("failed to remove feas field %s on chunk %s: %w", k, chunkID, sErr)
		}
	}

	// Remove specific field if removeField is set
	if removeField != "" {
		scriptBody := map[string]interface{}{
			"script": map[string]interface{}{
				"source": fmt.Sprintf("ctx._source.remove('%s');", removeField),
			},
		}
		sBody, _ := json.Marshal(scriptBody)
		sResp, sErr := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
			Index:      indexName,
			DocumentID: actualID,
			Body:       bytes.NewReader(sBody),
		})
		if sResp != nil && sResp.Inspect().Response != nil {
			status := sResp.Inspect().Response.StatusCode
			_, _ = io.Copy(io.Discard, sResp.Inspect().Response.Body)
			_ = sResp.Inspect().Response.Body.Close()
			if status == http.StatusNotFound {
				return nil
			}
			if status >= 400 {
				return fmt.Errorf("failed to remove field %s on chunk %s, status: %d", removeField, chunkID, status)
			}
		}
		if sErr != nil {
			return fmt.Errorf("failed to remove field %s on chunk %s: %w", removeField, chunkID, sErr)
		}
	}

	// Remove specific values from array fields (removeDict)
	if removeDict != nil {
		var scripts []string
		params := make(map[string]interface{})
		for kk, vv := range removeDict {
			scripts = append(scripts,
				fmt.Sprintf("if (ctx._source.containsKey('%s') && ctx._source.%s != null) { int i = ctx._source.%s.indexOf(params.p_%s); if (i >= 0) { ctx._source.%s.remove(i); }}",
					kk, kk, kk, kk, kk))
			params[fmt.Sprintf("p_%s", kk)] = vv
		}
		if len(scripts) > 0 {
			scriptBody := map[string]interface{}{
				"script": map[string]interface{}{
					"source": strings.Join(scripts, ""),
					"params": params,
				},
			}
			sBody, _ := json.Marshal(scriptBody)
			sResp, sErr := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
				Index:      indexName,
				DocumentID: actualID,
				Body:       bytes.NewReader(sBody),
			})
			if sResp != nil && sResp.Inspect().Response != nil {
				status := sResp.Inspect().Response.StatusCode
				_, _ = io.Copy(io.Discard, sResp.Inspect().Response.Body)
				_ = sResp.Inspect().Response.Body.Close()
				if status == http.StatusNotFound {
					return nil
				}
				if status >= 400 {
					return fmt.Errorf("failed to remove dict fields on chunk %s, status: %d", chunkID, status)
				}
			}
			if sErr != nil {
				return fmt.Errorf("failed to remove dict fields on chunk %s: %w", chunkID, sErr)
			}
		}
	}

	// Append specific values to array fields (addDict)
	if len(addDict) > 0 {
		var scripts []string
		params := make(map[string]interface{})
		for kk, vv := range addDict {
			if vvStr, ok := vv.(string); ok {
				vvStr = strings.TrimSpace(vvStr)
				scripts = append(scripts,
					fmt.Sprintf("if (!ctx._source.containsKey('%s') || ctx._source.%s == null) { ctx._source.%s = new ArrayList(); } ctx._source.%s.add(params.pp_%s);",
						kk, kk, kk, kk, kk))
				params[fmt.Sprintf("pp_%s", kk)] = vvStr
			}
		}
		if len(scripts) > 0 {
			scriptBody := map[string]interface{}{
				"script": map[string]interface{}{
					"source": strings.Join(scripts, ""),
					"params": params,
				},
			}
			sBody, _ := json.Marshal(scriptBody)
			sResp, sErr := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
				Index:      indexName,
				DocumentID: actualID,
				Body:       bytes.NewReader(sBody),
			})
			if sResp != nil && sResp.Inspect().Response != nil {
				status := sResp.Inspect().Response.StatusCode
				_, _ = io.Copy(io.Discard, sResp.Inspect().Response.Body)
				_ = sResp.Inspect().Response.Body.Close()
				if status == http.StatusNotFound {
					return nil
				}
				if status >= 400 {
					return fmt.Errorf("failed to add dict fields on chunk %s, status: %d", chunkID, status)
				}
			}
			if sErr != nil {
				return fmt.Errorf("failed to add dict fields on chunk %s: %w", chunkID, sErr)
			}
		}
	}

	// Update document fields if any remain
	if len(doc) > 0 {
		if raw, ok := formatOrderedTagFeas(doc["tag_feas"]); ok {
			doc["tag_feas"] = raw
		}
		updateBody := map[string]interface{}{"doc": doc}
		uBody, _ := json.Marshal(updateBody)
		uResp, uErr := e.apiClient.Update(ctx, opensearchapi.UpdateReq{
			Index:      indexName,
			DocumentID: actualID,
			Body:       bytes.NewReader(uBody),
			Params: opensearchapi.UpdateParams{
				Refresh: "wait_for",
			},
		})
		if uResp != nil && uResp.Inspect().Response != nil {
			defer func() {
				_, _ = io.Copy(io.Discard, uResp.Inspect().Response.Body)
				_ = uResp.Inspect().Response.Body.Close()
			}()
			if uResp.Inspect().Response.StatusCode == http.StatusNotFound {
				return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
			}
			if uResp.Inspect().Response.StatusCode >= 400 {
				return fmt.Errorf("opensearch update error: %s", uResp.Inspect().Response.Status())
			}
		}
		if uErr != nil {
			return fmt.Errorf("failed to update document: %w", uErr)
		}
	}

	common.Debug("OpenSearchConnection.updateSingleChunk completed", zap.String("indexName", indexName), zap.String("chunkID", chunkID))
	return nil
}

func (e *Engine) updateChunksByQuery(ctx context.Context, indexName string, condition map[string]interface{}, newValue map[string]interface{}) error {
	common.Debug("OpenSearchConnection.updateChunksByQuery called", zap.String("indexName", indexName))
	condition = types.CompilationFilter(condition)

	var mustClauses []map[string]interface{}
	var mustNotClauses []map[string]interface{}
	for k, v := range condition {
		if k == "and" || k == "or" {
			mustClauses = append(mustClauses, buildBoolQueryFromConditionRaw(map[string]interface{}{k: v}, nil, false, false))
			continue
		}
		if k == "exists" {
			mustClauses = append(mustClauses, map[string]interface{}{
				"exists": map[string]interface{}{"field": v},
			})
			continue
		}
		if k == "must_not" {
			mustNotClauses = append(mustNotClauses, mustNotExistsClauses(v)...)
			continue
		}
		if k == "knowledge_graph_kwd" {
			if kgClause := buildKnowledgeGraphFilterClause(v); kgClause != nil {
				mustClauses = append(mustClauses, kgClause)
			}
			continue
		}
		if v == nil || v == "" {
			continue
		}
		if items, ok := conditionTermsList(v); ok {
			mustClauses = append(mustClauses, map[string]interface{}{
				"terms": map[string]interface{}{k: items},
			})
			continue
		}
		if _, ok := v.(string); ok {
			mustClauses = append(mustClauses, map[string]interface{}{
				"term": map[string]interface{}{k: v},
			})
			continue
		}
		if isNumericValue(v) {
			mustClauses = append(mustClauses, map[string]interface{}{
				"term": map[string]interface{}{k: v},
			})
			continue
		}
		return fmt.Errorf("unsupported condition value type %T for field %s", v, k)
	}
	if len(mustClauses) == 0 && len(mustNotClauses) == 0 && len(condition) > 0 {
		return fmt.Errorf("OpenSearch update aborted: non-empty condition yielded unfiltered update on index %s", indexName)
	}

	boolClauses := map[string]interface{}{}
	if len(mustClauses) > 0 {
		boolClauses["filter"] = mustClauses
	}
	if len(mustNotClauses) > 0 {
		boolClauses["must_not"] = mustNotClauses
	}
	boolQuery := map[string]interface{}{"bool": boolClauses}

	var scripts []string
	params := make(map[string]interface{})

	for k, v := range newValue {
		if k == "remove" {
			if removeStr, ok := v.(string); ok {
				scripts = append(scripts, fmt.Sprintf("ctx._source.remove('%s');", removeStr))
				continue
			}
			if removeDict, ok := v.(map[string]interface{}); ok {
				for kk, vv := range removeDict {
					scripts = append(scripts,
						fmt.Sprintf("if (ctx._source.containsKey('%s') && ctx._source.%s != null) { int i = ctx._source.%s.indexOf(params.p_%s); if (i >= 0) { ctx._source.%s.remove(i); }}",
							kk, kk, kk, kk, kk))
					params[fmt.Sprintf("p_%s", kk)] = vv
				}
			}
			continue
		}
		if k == "add" {
			if addDict, ok := v.(map[string]interface{}); ok {
				for kk, vv := range addDict {
					vvStr, ok := vv.(string)
					if ok {
						vvStr = strings.TrimSpace(vvStr)
						scripts = append(scripts,
							fmt.Sprintf("if (!ctx._source.containsKey('%s') || ctx._source.%s == null) { ctx._source.%s = new ArrayList(); } ctx._source.%s.add(params.pp_%s);",
								kk, kk, kk, kk, kk))
						params[fmt.Sprintf("pp_%s", kk)] = vvStr
					}
				}
			}
			continue
		}
		if (k == "" || v == nil) && k != "available_int" {
			continue
		}

		if items, ok := conditionTermsList(v); ok {
			params[fmt.Sprintf("pp_%s", k)] = items
			scripts = append(scripts, fmt.Sprintf("ctx._source.%s=params.pp_%s;", k, k))
			continue
		}
		switch val := v.(type) {
		case string:
			params[fmt.Sprintf("pp_%s", k)] = val
			scripts = append(scripts, fmt.Sprintf("ctx._source.%s=params.pp_%s;", k, k))
		case int, int8, int16, int32, int64, float32, float64:
			params[fmt.Sprintf("pp_%s", k)] = val
			scripts = append(scripts, fmt.Sprintf("ctx._source.%s=params.pp_%s;", k, k))
		}
	}

	scriptSource := strings.Join(scripts, "")
	if scriptSource == "" {
		return fmt.Errorf("no supported update fields for update by query")
	}

	updateBody := map[string]interface{}{
		"query": boolQuery,
		"script": map[string]interface{}{
			"source": scriptSource,
			"params": params,
		},
	}

	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	refreshTrue := true
	resp, err := e.apiClient.UpdateByQuery(ctx, opensearchapi.UpdateByQueryReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
		Params: opensearchapi.UpdateByQueryParams{
			Refresh:   &refreshTrue,
			Slices:    5,
			Conflicts: "proceed",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyB, _ := io.ReadAll(resp.Inspect().Response.Body)
			return fmt.Errorf("opensearch update by query error: %s, body: %s", resp.Inspect().Response.Status(), string(bodyB))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to execute update by query: %w", err)
	}

	if resp.TimedOut {
		return fmt.Errorf("opensearch update chunks by query timed out on index %s (updated %d of %d)", indexName, resp.Updated, resp.Total)
	}
	if len(resp.Failures) > 0 {
		return fmt.Errorf("opensearch update chunks by query encountered %d failures on index %s", len(resp.Failures), indexName)
	}
	if resp.VersionConflicts > 0 {
		return fmt.Errorf("opensearch update chunks by query encountered %d version conflicts on index %s", resp.VersionConflicts, indexName)
	}

	common.Debug("OpenSearchConnection.updateChunksByQuery completed", zap.String("indexName", indexName))
	return nil
}

// DeleteChunks deletes chunks from a dataset index by condition.
// Condition kb_id is strictly injected to guarantee dataset isolation.
func (e *Engine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName string, datasetID string) (int64, error) {
	fullIndexName := baseName
	common.Info("Deleting chunks from OpenSearch index", zap.String("index_name", fullIndexName), zap.Any("condition", condition))

	exists, err := e.indexExists(ctx, fullIndexName)
	if err != nil {
		return 0, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		common.Warn("Index does not exist, skipping delete", zap.String("index_name", fullIndexName))
		return 0, nil
	}

	condition = types.CompilationFilter(condition)
	if strings.HasPrefix(fullIndexName, "memory_") {
		condition = mapMemoryMessageESConditionFields(condition)
	}

	var mustClauses []map[string]interface{}
	var filterClauses []map[string]interface{}
	var mustNotClauses []map[string]interface{}

	if idVal, ok := condition["id"]; ok && idVal != nil {
		if id, isString := idVal.(string); isString {
			if id == "" {
				return 0, nil
			}
			mustClauses = append(mustClauses, map[string]interface{}{
				"term": map[string]interface{}{"id": id},
			})
		} else if items, isList := conditionTermsList(idVal); isList {
			if len(items) == 0 {
				return 0, nil
			}
			var ids []interface{}
			for _, item := range items {
				if id, isString := item.(string); isString && id != "" {
					ids = append(ids, id)
				}
			}
			if len(ids) == 0 {
				return 0, nil
			}
			mustClauses = append(mustClauses, map[string]interface{}{
				"terms": map[string]interface{}{"id": ids},
			})
		}
	}

	// Guarantee dataset isolation
	if datasetID != "" {
		condition["kb_id"] = datasetID
	}
	if kbVal, ok := condition["kb_id"]; ok && kbVal != nil {
		if kbID, isString := kbVal.(string); isString {
			if kbID != "" {
				filterClauses = append(filterClauses, map[string]interface{}{
					"term": map[string]interface{}{"kb_id": kbID},
				})
			}
		} else if items, isList := conditionTermsList(kbVal); isList {
			if len(items) == 0 {
				return 0, nil
			}
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{"kb_id": items},
			})
		} else {
			return 0, fmt.Errorf("unsupported kb_id condition type %T", kbVal)
		}
	}

	for k, v := range condition {
		if k == "and" || k == "or" {
			filterClauses = append(filterClauses, buildBoolQueryFromConditionRaw(map[string]interface{}{k: v}, nil, false, false))
			continue
		}
		if k == "id" || k == "kb_id" {
			continue
		}
		if k == "exists" {
			filterClauses = append(filterClauses, map[string]interface{}{
				"exists": map[string]interface{}{"field": v},
			})
		} else if k == "must_not" {
			mustNotClauses = append(mustNotClauses, mustNotExistsClauses(v)...)
		} else if k == "knowledge_graph_kwd" {
			if kgClause := buildKnowledgeGraphFilterClause(v); kgClause != nil {
				filterClauses = append(filterClauses, kgClause)
			}
		} else if v != nil {
			if items, ok := conditionTermsList(v); ok {
				mustClauses = append(mustClauses, map[string]interface{}{
					"terms": map[string]interface{}{k: items},
				})
			} else if _, ok := v.(string); ok {
				mustClauses = append(mustClauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			} else if isNumericValue(v) {
				mustClauses = append(mustClauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			} else {
				return 0, fmt.Errorf("unsupported condition value type %T for field %s", v, k)
			}
		}
	}

	var qry map[string]interface{}
	if len(filterClauses) == 0 && len(mustClauses) == 0 && len(mustNotClauses) == 0 {
		if len(condition) > 0 {
			return 0, fmt.Errorf("OpenSearch delete aborted: non-empty condition yielded match_all query on index %s", fullIndexName)
		}
		qry = map[string]interface{}{"match_all": map[string]interface{}{}}
	} else {
		boolMap := map[string]interface{}{}
		if len(filterClauses) > 0 {
			boolMap["filter"] = filterClauses
		}
		if len(mustClauses) > 0 {
			boolMap["must"] = mustClauses
		}
		if len(mustNotClauses) > 0 {
			boolMap["must_not"] = mustNotClauses
		}
		qry = map[string]interface{}{"bool": boolMap}
	}

	deleteBody := map[string]interface{}{
		"query": qry,
	}

	bodyBytes, err := json.Marshal(deleteBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal delete body: %w", err)
	}

	refreshTrue := true
	resp, err := e.apiClient.Document.DeleteByQuery(ctx, opensearchapi.DocumentDeleteByQueryReq{
		Indices: []string{fullIndexName},
		Body:    bytes.NewReader(bodyBytes),
		Params: opensearchapi.DocumentDeleteByQueryParams{
			Refresh: &refreshTrue,
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return 0, nil
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			return 0, fmt.Errorf("opensearch delete by query returned error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil {
		if isNotFoundError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to execute delete by query: %w", err)
	}

	if resp.TimedOut {
		return int64(resp.Deleted), fmt.Errorf("opensearch delete by query timed out on index %s (deleted %d of %d)", fullIndexName, resp.Deleted, resp.Total)
	}
	if len(resp.Failures) > 0 {
		return int64(resp.Deleted), fmt.Errorf("opensearch delete by query encountered %d failures on index %s", len(resp.Failures), fullIndexName)
	}
	if resp.VersionConflicts > 0 {
		return int64(resp.Deleted), fmt.Errorf("opensearch delete by query encountered %d version conflicts on index %s", resp.VersionConflicts, fullIndexName)
	}

	deleted := int64(resp.Deleted)
	common.Info("Successfully deleted chunks", zap.String("index_name", fullIndexName), zap.Int64("deleted_count", deleted))
	return deleted, nil
}

// GetChunk retrieves a chunk by ID across the given dataset IDs.
func (e *Engine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	if strings.HasPrefix(baseName, "memory_") {
		return e.getMemoryMessage(ctx, baseName, chunkID)
	}

	for _, datasetID := range datasetIDs {
		source, found, err := e.searchChunkInDataset(ctx, baseName, chunkID, datasetID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		common.Info("GetChunk found hit", zap.String("baseName", baseName), zap.String("chunkID", chunkID))
		if source == nil {
			source = make(map[string]interface{})
		}
		source["id"] = chunkID
		return source, nil
	}

	common.Info("GetChunk no hits found", zap.String("baseName", baseName), zap.String("chunkID", chunkID))
	return nil, nil
}

func (e *Engine) searchChunkInDataset(ctx context.Context, baseName, chunkID, datasetID string) (map[string]interface{}, bool, error) {
	searchReq := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"term": map[string]interface{}{"id": chunkID}},
					{"term": map[string]interface{}{"kb_id": datasetID}},
				},
			},
		},
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal search request: %w", err)
	}

	resp, err := e.apiClient.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{baseName},
		Body:    bytes.NewReader(body),
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return nil, false, nil
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			return nil, false, fmt.Errorf("opensearch search chunk error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil {
		if isNotFoundError(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to search for chunk: %w", err)
	}

	if resp.Timeout {
		return nil, false, fmt.Errorf("search chunk %s in index %s timed out", chunkID, baseName)
	}
	if resp.Shards.Failed > 0 {
		return nil, false, fmt.Errorf("search chunk %s in index %s encountered shard failures (%d failed shards)", chunkID, baseName, resp.Shards.Failed)
	}

	if len(resp.Hits.Hits) == 0 {
		return nil, false, nil
	}

	var source map[string]interface{}
	if err := json.Unmarshal(resp.Hits.Hits[0].Source, &source); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal chunk source: %w", err)
	}
	if source == nil {
		source = make(map[string]interface{})
	}
	return source, true, nil
}

func (e *Engine) getMemoryMessage(ctx context.Context, indexName, docID string) (interface{}, error) {
	resp, err := e.apiClient.Document.Get(ctx, opensearchapi.DocumentGetReq{
		Index:      indexName,
		DocumentID: docID,
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			return nil, fmt.Errorf("opensearch memory message get error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get memory message: %w", err)
	}
	if !resp.Found {
		return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
	}

	var source map[string]interface{}
	if err := json.Unmarshal(resp.Source, &source); err != nil {
		return nil, fmt.Errorf("failed to unmarshal memory message source: %w", err)
	}
	if source == nil {
		source = make(map[string]interface{})
	}
	normalizeMemoryMessageChunks([]map[string]interface{}{source})
	source["id"] = docID
	return source, nil
}

// DropChunkStore deletes a chunk index or drops chunks belonging to a specific dataset.
// When datasetID is provided (and is not "skill"), it deletes only chunks belonging
// to that knowledge base, preserving the shared tenant index (ragflow_<tenant>).
func (e *Engine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	if datasetID != "" && datasetID != "skill" {
		exists, err := e.indexExists(ctx, baseName)
		if err != nil || !exists {
			return err
		}
		field := "kb_id"
		if strings.HasPrefix(baseName, "memory_") {
			field = "memory_id"
		}
		_, err = e.DeleteChunks(ctx, map[string]interface{}{field: datasetID}, baseName, datasetID)
		return err
	}
	return e.dropIndex(ctx, baseName)
}

// ChunkStoreExists checks if a chunk index exists.
func (e *Engine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return e.indexExists(ctx, baseName)
}

// Memory mapping helpers
func memoryMessageVectorProperty(vectorSize int) map[string]interface{} {
	return map[string]interface{}{
		"type":      "knn_vector",
		"dimension": vectorSize,
		"method": map[string]interface{}{
			"name":       "hnsw",
			"space_type": "cosinesimil",
		},
	}
}

func memoryMessageVectorField(vectorSize int) string {
	return fmt.Sprintf("q_%d_vec", vectorSize)
}

func parseMemoryMessageVectorSize(field string) (int, bool) {
	if !memoryMessageVectorFieldRE.MatchString(field) {
		return 0, false
	}
	sizeText := strings.TrimSuffix(strings.TrimPrefix(field, "q_"), "_vec")
	vectorSize, err := strconv.Atoi(sizeText)
	if err != nil || vectorSize <= 0 {
		return 0, false
	}
	return vectorSize, true
}

func (e *Engine) memoryMessageVectorMappingExists(ctx context.Context, indexName, fieldName string) (bool, error) {
	resp, err := e.apiClient.Indices.Mapping.Get(ctx, &opensearchapi.MappingGetReq{
		Indices: []string{indexName},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return false, nil
		}
	}
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get memory vector mapping: %w", err)
	}

	indices := resp.GetIndices()
	idxMapping, ok := indices[indexName]
	if !ok {
		return false, nil
	}
	var mappingObj struct {
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.Unmarshal(idxMapping.Mappings, &mappingObj); err != nil {
		return false, err
	}
	_, exists := mappingObj.Properties[fieldName]
	return exists, nil
}

func (e *Engine) ensureMemoryMessageVectorMapping(ctx context.Context, indexName string, vectorSize int) error {
	if vectorSize <= 0 {
		return fmt.Errorf("memory vector size must be positive, got %d", vectorSize)
	}

	fieldName := memoryMessageVectorField(vectorSize)
	exists, err := e.memoryMessageVectorMappingExists(ctx, indexName, fieldName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			fieldName: memoryMessageVectorProperty(vectorSize),
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal memory vector mapping: %w", err)
	}

	resp, err := e.apiClient.Indices.Mapping.Put(ctx, opensearchapi.MappingPutReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(data),
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(bodyBytes)
			if reason != "" {
				return fmt.Errorf("opensearch error updating memory vector mapping %s.%s: %s", indexName, fieldName, reason)
			}
			return fmt.Errorf("opensearch returned error updating memory vector mapping %s.%s: %s, body: %s", indexName, fieldName, resp.Inspect().Response.Status(), string(bodyBytes))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to update memory vector mapping: %w", err)
	}
	return nil
}

func (e *Engine) ensureMemoryMessageVectorMappingsForDocs(ctx context.Context, indexName string, chunks []map[string]interface{}) error {
	seen := map[int]struct{}{}
	for _, chunk := range chunks {
		for field := range chunk {
			vectorSize, ok := parseMemoryMessageVectorSize(field)
			if !ok {
				continue
			}
			if _, ok := seen[vectorSize]; ok {
				continue
			}
			if err := e.ensureMemoryMessageVectorMapping(ctx, indexName, vectorSize); err != nil {
				return err
			}
			seen[vectorSize] = struct{}{}
		}
	}
	return nil
}

func chunkVectorProperty(vectorSize int) map[string]interface{} {
	return map[string]interface{}{
		"type":       "knn_vector",
		"dimension":  vectorSize,
		"index":      true,
		"space_type": "cosinesimil",
	}
}

func parseChunkVectorSize(field string) (int, bool) {
	if !strings.HasPrefix(field, "q_") || !strings.HasSuffix(field, "_vec") {
		return 0, false
	}
	dimStr := field[2 : len(field)-4]
	size, err := strconv.Atoi(dimStr)
	if err != nil || size <= 0 {
		return 0, false
	}
	return size, true
}

func (e *Engine) ensureChunkVectorMapping(ctx context.Context, indexName string, vectorSize int) error {
	if vectorSize <= 0 {
		return fmt.Errorf("chunk vector size must be positive, got %d", vectorSize)
	}

	fieldName := fmt.Sprintf("q_%d_vec", vectorSize)
	exists, err := e.memoryMessageVectorMappingExists(ctx, indexName, fieldName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			fieldName: chunkVectorProperty(vectorSize),
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal chunk vector mapping: %w", err)
	}

	resp, err := e.apiClient.Indices.Mapping.Put(ctx, opensearchapi.MappingPutReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(data),
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(bodyBytes)
			if reason != "" {
				return fmt.Errorf("opensearch error updating chunk vector mapping %s.%s: %s", indexName, fieldName, reason)
			}
			return fmt.Errorf("opensearch returned error updating chunk vector mapping %s.%s: %s, body: %s", indexName, fieldName, resp.Inspect().Response.Status(), string(bodyBytes))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to update chunk vector mapping: %w", err)
	}
	return nil
}

func (e *Engine) ensureChunkVectorMappingsForDocs(ctx context.Context, indexName string, chunks []map[string]interface{}) error {
	seen := map[int]struct{}{}
	for _, chunk := range chunks {
		for field := range chunk {
			vectorSize, ok := parseChunkVectorSize(field)
			if !ok {
				continue
			}
			if _, ok := seen[vectorSize]; ok {
				continue
			}
			if err := e.ensureChunkVectorMapping(ctx, indexName, vectorSize); err != nil {
				return err
			}
			seen[vectorSize] = struct{}{}
		}
	}
	return nil
}

func getMemoryMessageMapping(vectorSize int) map[string]interface{} {
	props := map[string]interface{}{
		"id":         map[string]interface{}{"type": "keyword"},
		"doc_id":     map[string]interface{}{"type": "keyword"},
		"kb_id":      map[string]interface{}{"type": "keyword"},
		"memory_id":  map[string]interface{}{"type": "keyword"},
		"user_id":    map[string]interface{}{"type": "keyword"},
		"agent_id":   map[string]interface{}{"type": "keyword"},
		"session_id": map[string]interface{}{"type": "keyword"},
		"tag_feas":   map[string]interface{}{"type": "rank_features"},
	}
	if vectorSize > 0 {
		props[memoryMessageVectorField(vectorSize)] = memoryMessageVectorProperty(vectorSize)
	}
	return map[string]interface{}{
		"settings": map[string]interface{}{
			"index.knn":          true,
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
		"mappings": map[string]interface{}{
			"properties": props,
		},
	}
}

func isNumericValue(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func loadSkillMapping() (map[string]interface{}, error) {
	path, err := utility.FindConfFileInProject("skill_os_mapping.json")
	if err != nil {
		return nil, fmt.Errorf("failed to locate skill mapping file: %w", err)
	}

	data, err := os.ReadFile(*path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill mapping file: %w", err)
	}

	var mapping map[string]interface{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse skill mapping json: %w", err)
	}
	return mapping, nil
}

func mapMemoryMessageToESDoc(message map[string]interface{}) map[string]interface{} {
	doc := make(map[string]interface{}, len(message)+2)
	for k, v := range message {
		switch k {
		case "message_type":
			doc["message_type_kwd"] = v
		case "status":
			doc["status_int"] = memoryMessageStatusInt(v)
		case "content":
			content, _ := v.(string)
			doc["content_ltks"] = content
			doc["tokenized_content_ltks"] = tokenizeMemoryMessageContent(content)
		default:
			doc[k] = v
		}
	}
	return doc
}

func tokenizeMemoryMessageContent(content string) string {
	tokenized, err := tokenizer.Tokenize(content)
	if err != nil {
		common.Warn("memory message tokenize failed, storing raw content", zap.Error(err))
		return content
	}
	fine, err := tokenizer.FineGrainedTokenize(tokenized)
	if err != nil {
		common.Warn("memory message fine-grained tokenize failed, storing coarse tokens", zap.Error(err))
		return tokenized
	}
	return fine
}

func memoryMessageStatusInt(value interface{}) int {
	if memoryMessageStatusBool(value) {
		return 1
	}
	return 0
}

func memoryMessageStatusBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case json.Number:
		n, err := v.Int64()
		return err == nil && n != 0
	case string:
		return v != "" && v != "0" && !strings.EqualFold(v, "false")
	default:
		return false
	}
}

func mapMemoryMessageESField(field string, useTokenizedContent bool) string {
	name := field
	boost := ""
	if base, suffix, ok := strings.Cut(field, "^"); ok {
		name = base
		boost = "^" + suffix
	}

	switch name {
	case "message_type":
		name = "message_type_kwd"
	case "status":
		name = "status_int"
	case "content":
		if useTokenizedContent {
			name = "tokenized_content_ltks"
		} else {
			name = "content_ltks"
		}
	}
	return name + boost
}

func mapMemoryMessageESUpdateFields(newValue map[string]interface{}) map[string]interface{} {
	doc := make(map[string]interface{}, len(newValue))
	for k, v := range newValue {
		switch k {
		case "remove", "add":
			doc[k] = v
		case "status":
			doc["status_int"] = memoryMessageStatusInt(v)
		default:
			doc[mapMemoryMessageESField(k, false)] = v
		}
	}
	if content, ok := newValue["content"].(string); ok {
		doc["tokenized_content_ltks"] = tokenizeMemoryMessageContent(content)
	}
	return doc
}

func mapMemoryMessageESConditionFields(condition map[string]interface{}) map[string]interface{} {
	mapped := make(map[string]interface{}, len(condition))
	for k, v := range condition {
		if k == "status" && v != nil {
			mapped["status_int"] = memoryMessageStatusInt(v)
			continue
		}
		mapped[mapMemoryMessageESField(k, false)] = v
	}
	return mapped
}

func normalizeMemoryMessageChunks(chunks []map[string]interface{}) {
	for _, chunk := range chunks {
		for key, val := range chunk {
			if memoryMessageVectorFieldRE.MatchString(key) {
				chunk["content_embed"] = val
				delete(chunk, key)
			}
		}
		if val, ok := chunk["message_type_kwd"]; ok {
			chunk["message_type"] = val
			delete(chunk, "message_type_kwd")
		}
		if val, ok := chunk["status_int"]; ok {
			chunk["status"] = memoryMessageStatusBool(val)
			delete(chunk, "status_int")
		}
		if val, ok := chunk["content_ltks"]; ok {
			chunk["content"] = val
			delete(chunk, "content_ltks")
		}
	}
}

func copyFields(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func conditionTermsList(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if ok {
		return items, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil, false
	}
	out := make([]interface{}, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out = append(out, rv.Index(i).Interface())
	}
	return out, true
}

// buildKnowledgeGraphFilterClause creates a bool should filter matching either type_kwd or knowledge_graph_kwd.
// If the condition value is empty (e.g. empty slice or empty string), it returns a match_none clause
// to prevent silently dropping the filter and expanding the delete/update scope.
func buildKnowledgeGraphFilterClause(v interface{}) map[string]interface{} {
	if items, ok := conditionTermsList(v); ok {
		if len(items) == 0 {
			return map[string]interface{}{"match_none": map[string]interface{}{}}
		}
		var graphTypeClauses []interface{}
		for _, field := range []string{"type_kwd", "knowledge_graph_kwd"} {
			graphTypeClauses = append(graphTypeClauses, map[string]interface{}{
				"terms": map[string]interface{}{field: items},
			})
		}
		return map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               graphTypeClauses,
				"minimum_should_match": 1,
			},
		}
	}
	if str, ok := v.(string); ok {
		if str == "" {
			return map[string]interface{}{"match_none": map[string]interface{}{}}
		}
		var graphTypeClauses []interface{}
		for _, field := range []string{"type_kwd", "knowledge_graph_kwd"} {
			graphTypeClauses = append(graphTypeClauses, map[string]interface{}{
				"term": map[string]interface{}{field: str},
			})
		}
		return map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               graphTypeClauses,
				"minimum_should_match": 1,
			},
		}
	}
	return map[string]interface{}{"match_none": map[string]interface{}{}}
}

func mustNotExistsClauses(value interface{}) []map[string]interface{} {
	exclusions, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	if field, ok := exclusions["exists"]; ok && len(exclusions) == 1 {
		return []map[string]interface{}{{"exists": map[string]interface{}{"field": field}}}
	}
	if query := buildBoolQueryFromConditionRaw(exclusions, nil, false, false); query != nil {
		return []map[string]interface{}{query}
	}
	return nil
}

func buildBoolQueryFromConditionRaw(filter map[string]interface{}, kbIDs []string, isSkillIndex, isMemoryIndex bool) map[string]interface{} {
	var mustClauses []interface{}
	var mustNotClauses []interface{}
	var filterClauses []interface{}

	if kbIDs != nil && len(kbIDs) > 0 {
		fieldName := "kb_id"
		if isMemoryIndex {
			fieldName = "memory_id"
		}
		filterClauses = append(filterClauses, map[string]interface{}{
			"terms": map[string]interface{}{fieldName: kbIDs},
		})
	}

	if isSkillIndex {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"status": "1",
			},
		})
	}

	if filter == nil {
		filter = make(map[string]interface{})
	}

	for k, v := range filter {
		if k == "and" || k == "or" {
			var children []interface{}
			for _, child := range types.FilterClauses(v) {
				children = append(children, buildBoolQueryFromConditionRaw(child, nil, false, isMemoryIndex))
			}
			if len(children) > 0 {
				clause := map[string]interface{}{"filter": children}
				if k == "or" {
					clause = map[string]interface{}{"should": children, "minimum_should_match": 1}
				}
				filterClauses = append(filterClauses, map[string]interface{}{"bool": clause})
			}
			continue
		}
		if isMemoryIndex {
			k = mapMemoryMessageESField(k, false)
		}
		if isSkillIndex && k == "status" {
			if v == nil || v == "" {
				continue
			}
			if listVal, ok := v.([]interface{}); ok && len(listVal) > 0 {
				filterClauses = append(filterClauses, map[string]interface{}{
					"terms": map[string]interface{}{"status": listVal},
				})
			} else if strVal, ok := v.(string); ok && strVal != "" {
				filterClauses = append(filterClauses, map[string]interface{}{
					"term": map[string]interface{}{"status": strVal},
				})
			}
			continue
		}
		if k == "available_int" {
			var numVal float64
			switch val := v.(type) {
			case float64:
				numVal = val
			case int:
				numVal = float64(val)
			case int64:
				numVal = float64(val)
			default:
				continue
			}
			if numVal == 0 {
				filterClauses = append(filterClauses, map[string]interface{}{
					"range": map[string]interface{}{"available_int": map[string]interface{}{"lt": 1}},
				})
			} else {
				filterClauses = append(filterClauses, map[string]interface{}{
					"bool": map[string]interface{}{
						"must_not": []map[string]interface{}{
							{"range": map[string]interface{}{"available_int": map[string]interface{}{"lt": 1}}},
						},
					},
				})
			}
			continue
		}
		if k == "must_not" {
			for _, clause := range mustNotExistsClauses(v) {
				mustNotClauses = append(mustNotClauses, clause)
			}
			continue
		}
		if k == "exists" {
			filterClauses = append(filterClauses, map[string]interface{}{"exists": map[string]interface{}{"field": v}})
			continue
		}
		if k == "knowledge_graph_kwd" {
			var graphTypeClauses []interface{}
			for _, field := range []string{"type_kwd", "knowledge_graph_kwd"} {
				switch val := v.(type) {
				case []interface{}:
					if len(val) > 0 {
						graphTypeClauses = append(graphTypeClauses, map[string]interface{}{"terms": map[string]interface{}{field: val}})
					}
				case []string:
					if len(val) > 0 {
						graphTypeClauses = append(graphTypeClauses, map[string]interface{}{"terms": map[string]interface{}{field: val}})
					}
				case string:
					if val != "" {
						graphTypeClauses = append(graphTypeClauses, map[string]interface{}{"term": map[string]interface{}{field: val}})
					}
				}
			}
			if len(graphTypeClauses) > 0 {
				filterClauses = append(filterClauses, map[string]interface{}{"bool": map[string]interface{}{
					"should": graphTypeClauses, "minimum_should_match": 1,
				}})
			}
			continue
		}
		if k == "id" {
			if idClauses := idTermClauses(v); len(idClauses) > 0 {
				filterClauses = append(filterClauses, map[string]interface{}{
					"bool": map[string]interface{}{
						"should":               idClauses,
						"minimum_should_match": 1,
					},
				})
			}
			continue
		}
		if v == nil || v == "" {
			continue
		}
		if isMemoryIndex && k == "session_id_contains" {
			if text, ok := v.(string); ok {
				pattern := strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`).Replace(text)
				filterClauses = append(filterClauses, map[string]interface{}{"wildcard": map[string]interface{}{"session_id": "*" + pattern + "*"}})
			}
			continue
		}
		if isMemoryIndex && k == "session_id" {
			if strVal, ok := v.(string); ok && strVal != "" {
				filterClauses = append(filterClauses, map[string]interface{}{
					"query_string": map[string]interface{}{
						"query":            fmt.Sprintf("*%s*", strVal),
						"fields":           []string{"session_id"},
						"analyze_wildcard": true,
					},
				})
				continue
			}
		}
		if listVal, ok := v.([]interface{}); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{k: listVal},
			})
		} else if strListVal, ok := v.([]string); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{k: strListVal},
			})
		} else if strVal, ok := v.(string); ok && strVal != "" {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: strVal},
			})
		} else if intVal, ok := v.(int); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: intVal},
			})
		} else if floatVal, ok := v.(float64); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: floatVal},
			})
		}
	}

	boolQuery := make(map[string]interface{})
	if len(mustClauses) > 0 {
		boolQuery["must"] = mustClauses
	}
	if len(mustNotClauses) > 0 {
		boolQuery["must_not"] = mustNotClauses
	}
	if len(filterClauses) > 0 {
		boolQuery["filter"] = filterClauses
	}

	if len(boolQuery) == 0 {
		return nil
	}

	return map[string]interface{}{"bool": boolQuery}
}

func idTermClauses(value interface{}) []interface{} {
	switch val := value.(type) {
	case nil:
		return nil
	case string:
		if val == "" {
			return nil
		}
		return []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"id": val}},
			map[string]interface{}{"term": map[string]interface{}{"_id": val}},
		}
	default:
		items, ok := conditionTermsList(value)
		if !ok || len(items) == 0 {
			return nil
		}
		return []interface{}{
			map[string]interface{}{"terms": map[string]interface{}{"id": items}},
			map[string]interface{}{"terms": map[string]interface{}{"_id": items}},
		}
	}
}

func formatOrderedTagFeas(v any) (json.RawMessage, bool) {
	if v == nil {
		return nil, false
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw, true
	}
	if marshaler, ok := v.(json.Marshaler); ok {
		b, err := marshaler.MarshalJSON()
		if err == nil {
			return json.RawMessage(b), true
		}
	}
	type kv struct {
		k string
		v int
	}
	roundScore := func(f float64) int {
		if f < 0 {
			return int(f - 0.5)
		}
		return int(f + 0.5)
	}

	var kvs []kv
	switch m := v.(type) {
	case map[string]int:
		kvs = make([]kv, 0, len(m))
		for k, val := range m {
			kvs = append(kvs, kv{k, val})
		}
	case map[string]float64:
		kvs = make([]kv, 0, len(m))
		for k, val := range m {
			kvs = append(kvs, kv{k, roundScore(val)})
		}
	case map[string]any:
		kvs = make([]kv, 0, len(m))
		for k, val := range m {
			switch score := val.(type) {
			case int:
				kvs = append(kvs, kv{k, score})
			case float64:
				kvs = append(kvs, kv{k, roundScore(score)})
			case int64:
				kvs = append(kvs, kv{k, int(score)})
			case json.Number:
				if f, err := score.Float64(); err == nil {
					kvs = append(kvs, kv{k, roundScore(f)})
				}
			}
		}
	default:
		return nil, false
	}

	if len(kvs) == 0 {
		return json.RawMessage("{}"), true
	}

	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].v != kvs[j].v {
			return kvs[i].v > kvs[j].v
		}
		return kvs[i].k < kvs[j].k
	})

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, item := range kvs {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(item.k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(item.v))
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes()), true
}
