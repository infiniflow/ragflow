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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
	"ragflow/internal/utility"
	"strconv"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"
)

type Engine struct {
	client              *opensearch.Client
	apiClient           *opensearchapi.Client
	config              *config.OpenSearchConfig
	hybridSearchEnabled bool
	hybridPipelineName  string
}

func NewEngine(ctx context.Context, osConfig config.OpenSearchConfig) (*Engine, error) {
	transport := &http.Transport{
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   300,
		MaxConnsPerHost:       500,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2:     true,
	}

	osCfg := opensearch.Config{
		Addresses: []string{osConfig.Hosts},
		Username:  osConfig.Username,
		Password:  osConfig.Password,
		Transport: transport,
	}

	client, err := opensearch.NewClient(osCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: osCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch API client: %w", err)
	}

	newCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	infoResp, err := apiClient.Info(newCtx, &opensearchapi.InfoReq{})
	if err != nil {
		return nil, fmt.Errorf("failed to ping OpenSearch: %w", err)
	}
	if infoResp == nil {
		return nil, fmt.Errorf("OpenSearch returned nil info response")
	}

	engine := &Engine{
		client:             client,
		apiClient:          apiClient,
		config:             &osConfig,
		hybridPipelineName: "ragflow_hybrid_pipeline",
	}

	if err = engine.initHybridSearch(newCtx, infoResp.Version.Number); err != nil {
		common.Warn("OpenSearch hybrid search is disabled",
			zap.String("version", infoResp.Version.Number), zap.Error(err))
	} else {
		engine.hybridSearchEnabled = true
	}

	if err = engine.CreateIndexTemplate(newCtx, "ragflow_mapping", "ragflow_*", "os_mapping.json", 1); err != nil {
		return nil, fmt.Errorf("failed to create chunk index template: %w", err)
	}

	return engine, nil
}

func (e *Engine) GetType() string {
	return "opensearch"
}

func (e *Engine) SupportsPageRank() bool {
	return true
}

func (e *Engine) Ping(ctx context.Context) error {
	_, err := e.apiClient.Info(ctx, &opensearchapi.InfoReq{})
	if err != nil {
		return fmt.Errorf("opensearch ping failed: %w", err)
	}
	return nil
}

func (e *Engine) Close() error {
	return nil
}

func (e *Engine) initHybridSearch(ctx context.Context, version string) error {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return fmt.Errorf("cannot parse OpenSearch version %q", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("cannot parse OpenSearch major version %q: %w", parts[0], err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("cannot parse OpenSearch minor version %q: %w", parts[1], err)
	}
	if major < 2 || (major == 2 && minor < 10) {
		return fmt.Errorf("hybrid search requires OpenSearch >= 2.10, got %s", version)
	}

	weights := []float64{0.5, 0.5}
	pipelineBody := map[string]interface{}{
		"description": "RAGFlow hybrid search normalization pipeline (BM25 + KNN).",
		"phase_results_processors": []interface{}{
			map[string]interface{}{
				"normalization-processor": map[string]interface{}{
					"normalization": map[string]interface{}{"technique": "min_max"},
					"combination": map[string]interface{}{
						"technique":  "arithmetic_mean",
						"parameters": map[string]interface{}{"weights": weights},
					},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(pipelineBody)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "/_search/pipeline/"+e.hybridPipelineName, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create pipeline request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Perform(req)
	if err != nil {
		return fmt.Errorf("failed to create search pipeline: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create search pipeline (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (e *Engine) CreateIndexTemplate(ctx context.Context, templateName, indexPattern, mappingFileName string, priority ...int) error {
	if templateName == "" || indexPattern == "" {
		return fmt.Errorf("template name and index pattern cannot be empty")
	}

	p := 1
	if len(priority) > 0 {
		p = priority[0]
	}

	if mappingFileName == "" {
		mappingFileName = "os_mapping.json"
	}

	mappingPath, err := utility.FindConfFileInProject(mappingFileName)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(*mappingPath)
	if err != nil {
		return fmt.Errorf("failed to read mapping file %q: %w", *mappingPath, err)
	}

	var mapping map[string]interface{}
	if err = json.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping file %q: %w", *mappingPath, err)
	}

	templateBody := map[string]interface{}{
		"index_patterns": []string{indexPattern},
		"priority":       p,
		"template": map[string]interface{}{
			"settings": mapping["settings"],
			"mappings": mapping["mappings"],
		},
	}

	templateBytes, err := json.Marshal(templateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	resp, err := e.apiClient.IndexTemplate.Create(ctx, opensearchapi.IndexTemplateCreateReq{
		IndexTemplate: templateName,
		Body:          bytes.NewReader(templateBytes),
	})
	if err != nil {
		return fmt.Errorf("failed to create index template: %w", err)
	}
	if !resp.Acknowledged {
		return fmt.Errorf("index template creation not acknowledged")
	}
	return nil
}

func (e *Engine) GetClusterStats(ctx context.Context) (map[string]interface{}, error) {
	resp, err := e.apiClient.Cluster.Stats(ctx, &opensearchapi.ClusterStatsReq{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster stats: %w", err)
	}
	respBytes, _ := json.Marshal(resp)
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse cluster stats: %w", err)
	}
	return result, nil
}

func (e *Engine) GetIndexStats(ctx context.Context, indices []string) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	for _, idx := range indices {
		exists, err := e.indexExists(ctx, idx)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		resp, err := e.apiClient.Indices.Get(ctx, opensearchapi.IndicesGetReq{
			Indices: []string{idx},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get index %s stats: %w", idx, err)
		}
		respBytes, _ := json.Marshal(resp)
		var idxResult map[string]interface{}
		if err := json.Unmarshal(respBytes, &idxResult); err != nil {
			return nil, fmt.Errorf("failed to parse index %s stats: %w", idx, err)
		}
		result = append(result, idxResult)
	}
	return result, nil
}

// ========== Perform-based helpers (ES/OS HTTP API compatible) ==========

func (e *Engine) doSearch(ctx context.Context, indices []string, body io.Reader, params map[string]string) (map[string]interface{}, error) {
	path := "/" + strings.Join(indices, ",") + "/_search"
	if len(indices) == 0 {
		path = "/_search"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read search response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound && isIndexNotFoundBody(respBody) {
		return nil, &indexNotFoundError{index: strings.Join(indices, ",")}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}
	return result, nil
}

func (e *Engine) doBulk(ctx context.Context, body io.Reader, refresh string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/_bulk", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	if refresh != "" {
		q := req.URL.Query()
		q.Set("refresh", refresh)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("bulk request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bulk response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bulk error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bulk response: %w", err)
	}
	return result, nil
}

func (e *Engine) doIndex(ctx context.Context, index, docID string, body io.Reader, refresh string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/%s/_doc/%s", index, docID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if refresh != "" {
		q := req.URL.Query()
		q.Set("refresh", refresh)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("index request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read index response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("index error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse index response: %w", err)
	}
	return result, nil
}

func (e *Engine) doUpdate(ctx context.Context, index, docID string, body io.Reader, refresh string, params map[string]string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/%s/_update/%s", index, docID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	if refresh != "" {
		q.Set("refresh", refresh)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("update request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read update response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("update error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse update response: %w", err)
	}
	return result, nil
}

func (e *Engine) doDelete(ctx context.Context, index, docID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/%s/_doc/%s", index, docID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read delete response: %w", err)
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		return nil, fmt.Errorf("delete error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse delete response: %w", err)
	}
	return result, nil
}

func (e *Engine) doUpdateByQuery(ctx context.Context, indices []string, body io.Reader) (map[string]interface{}, error) {
	path := "/" + strings.Join(indices, ",") + "/_update_by_query"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	q := req.URL.Query()
	q.Set("slices", "5")
	q.Set("conflicts", "proceed")
	q.Set("refresh", "true")
	req.URL.RawQuery = q.Encode()

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("update_by_query request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read update_by_query response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("update_by_query error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse update_by_query response: %w", err)
	}
	return result, nil
}

func (e *Engine) doDeleteByQuery(ctx context.Context, indices []string, body io.Reader) (map[string]interface{}, error) {
	path := "/" + strings.Join(indices, ",") + "/_delete_by_query"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	q := req.URL.Query()
	q.Set("slices", "5")
	q.Set("conflicts", "proceed")
	q.Set("refresh", "true")
	req.URL.RawQuery = q.Encode()

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("delete_by_query request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read delete_by_query response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("delete_by_query error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse delete_by_query response: %w", err)
	}
	return result, nil
}

func (e *Engine) doIndicesCreate(ctx context.Context, index string, body io.Reader) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "/"+index, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("indices create request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read indices create response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("indices create error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if len(respBody) > 0 {
		json.Unmarshal(respBody, &result)
	}
	return result, nil
}

func (e *Engine) doIndicesGetMapping(ctx context.Context, indices []string) (map[string]interface{}, error) {
	path := "/" + strings.Join(indices, ",") + "/_mapping"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("get mapping request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read mapping response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get mapping error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse mapping response: %w", err)
	}
	return result, nil
}

func (e *Engine) doIndicesPutMapping(ctx context.Context, indices []string, body io.Reader) (map[string]interface{}, error) {
	path := "/" + strings.Join(indices, ",") + "/_mapping"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("put mapping request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read put mapping response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("put mapping error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if len(respBody) > 0 {
		json.Unmarshal(respBody, &result)
	}
	return result, nil
}
