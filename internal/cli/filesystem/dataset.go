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

package filesystem

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string][]string
	Duration   float64
}

// HTTPClientInterface defines the interface needed from HTTPClient
type HTTPClientInterface interface {
	Request(method, path string, useAPIBase bool, authKind string, headers map[string]string, jsonBody map[string]interface{}) (*HTTPResponse, error)
}

// DatasetProvider handles datasets and their documents
// Path structure:
//   - datasets/              -> List all datasets
//   - datasets/{name}        -> List documents in dataset
//   - datasets/{name}/{doc_name} -> Get document info
type DatasetProvider struct {
	BaseProvider
	httpClient HTTPClientInterface
}

// NewDatasetProvider creates a new DatasetProvider
func NewDatasetProvider(httpClient HTTPClientInterface) *DatasetProvider {
	return &DatasetProvider{
		BaseProvider: BaseProvider{
			name:        "datasets",
			description: "Dataset management provider",
			rootPath:    "datasets",
		},
		httpClient: httpClient,
	}
}

// Supports returns true if this provider can handle the given path
func (p *DatasetProvider) Supports(path string) bool {
	normalized := normalizePath(path)
	return normalized == "datasets" || strings.HasPrefix(normalized, "datasets/")
}

// List lists nodes at the given path
func (p *DatasetProvider) List(ctx stdctx.Context, subPath string, opts *ListOptions) (*Result, error) {
	// subPath is the path relative to "datasets/"
	// Empty subPath means list all datasets
	// "{name}/files" means list documents in a dataset

	// Check if trying to access hidden .knowledgebase
	if subPath == ".knowledgebase" || strings.HasPrefix(subPath, ".knowledgebase/") {
		return nil, fmt.Errorf("invalid path: .knowledgebase is not accessible")
	}

	if subPath == "" {
		return p.listDatasets(ctx, opts)
	}

	parts := SplitPath(subPath)
	if len(parts) == 1 {
		// datasets/{name} - list documents in the dataset (default behavior)
		return p.listDocuments(ctx, parts[0], opts)
	}

	if len(parts) == 2 {
		// datasets/{name}/{doc_name} - get document info
		return p.getDocumentNode(ctx, parts[0], parts[1])
	}

	return nil, fmt.Errorf("invalid path: %s", subPath)
}

// Search searches for datasets or documents
func (p *DatasetProvider) Search(ctx stdctx.Context, subPath string, opts *SearchOptions) (*Result, error) {
	if opts.Query == "" {
		return p.List(ctx, subPath, &ListOptions{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		})
	}

	// If searching under a specific dataset's files
	parts := SplitPath(subPath)
	if len(parts) >= 2 && parts[1] == "files" {
		datasetName := parts[0]
		return p.searchDocuments(ctx, datasetName, opts)
	}

	// Otherwise search datasets
	return p.searchDatasets(ctx, opts)
}

// Cat retrieves document content
// For datasets:
//   - cat datasets          -> Error: datasets is a directory, not a file
//   - cat datasets/kb_name  -> Error: kb_name is a directory, not a file
//   - cat datasets/kb_name/doc_name -> Would retrieve document content (if implemented)
func (p *DatasetProvider) Cat(ctx stdctx.Context, subPath string) ([]byte, error) {
	if subPath == "" {
		return nil, fmt.Errorf("'datasets' is a directory, not a file")
	}

	parts := SplitPath(subPath)
	if len(parts) == 1 {
		// datasets/{name} - this is a dataset (directory)
		return nil, fmt.Errorf("'%s' is a directory, not a file", parts[0])
	}

	if len(parts) == 2 {
		// datasets/{name}/{doc_name} - this could be a document
		// For now, document content retrieval is not implemented
		return nil, fmt.Errorf("document content retrieval not yet implemented for '%s'", parts[1])
	}

	return nil, fmt.Errorf("invalid path for cat: %s", subPath)
}

// ==================== Dataset Operations ====================

func (p *DatasetProvider) listDatasets(ctx stdctx.Context, opts *ListOptions) (*Result, error) {
	resp, err := p.httpClient.Request("GET", "/datasets", true, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code    int                      `json:"code"`
		Data    []map[string]interface{} `json:"data"`
		Message string                   `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	nodes := make([]*Node, 0, len(apiResp.Data))
	for _, ds := range apiResp.Data {
		node := p.datasetToNode(ds)
		// Skip hidden .knowledgebase dataset (trim whitespace for safety)
		if strings.TrimSpace(node.Name) == ".knowledgebase" {
			continue
		}
		nodes = append(nodes, node)
	}

	total := len(nodes)

	// Apply limit if specified
	if opts != nil && opts.Limit > 0 && opts.Limit < len(nodes) {
		nodes = nodes[:opts.Limit]
	}

	return &Result{
		Nodes: nodes,
		Total: total,
	}, nil
}

func (p *DatasetProvider) getDataset(ctx stdctx.Context, name string) (*Node, error) {
	// Check if trying to access hidden .knowledgebase
	if name == ".knowledgebase" {
		return nil, fmt.Errorf("invalid path: .knowledgebase is not accessible")
	}

	// First list all datasets to find the one with matching name
	resp, err := p.httpClient.Request("GET", "/datasets", true, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code    int                      `json:"code"`
		Data    []map[string]interface{} `json:"data"`
		Message string                   `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	for _, ds := range apiResp.Data {
		if getString(ds["name"]) == name {
			return p.datasetToNode(ds), nil
		}
	}

	return nil, fmt.Errorf("%s: dataset '%s'", ErrNotFound, name)
}

func (p *DatasetProvider) searchDatasets(ctx stdctx.Context, opts *SearchOptions) (*Result, error) {
	// If no query is provided, just list datasets
	if opts.Query == "" {
		return p.listDatasets(ctx, &ListOptions{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		})
	}

	// Use retrieval API for semantic search
	return p.searchWithRetrieval(ctx, opts)
}

// searchWithRetrieval performs semantic search using the retrieval API
func (p *DatasetProvider) searchWithRetrieval(ctx stdctx.Context, opts *SearchOptions) (*Result, error) {
	// Determine kb_ids to search in
	var kbIDs []string
	var datasetsToSearch []*Node

	if len(opts.Dirs) > 0 && opts.Dirs[0] != "datasets" {
		// Search in specific datasets
		for _, dir := range opts.Dirs {
			// Extract dataset name from path (e.g., "datasets/kb1" -> "kb1")
			datasetName := dir
			if strings.HasPrefix(dir, "datasets/") {
				datasetName = dir[len("datasets/"):]
			}
			ds, err := p.getDataset(ctx, datasetName)
			if err != nil {
				// Try case-insensitive match
				allResult, listErr := p.listDatasets(ctx, nil)
				if listErr == nil {
					for _, d := range allResult.Nodes {
						if strings.EqualFold(d.Name, datasetName) {
							ds = d
							err = nil
							break
						}
					}
				}
				if err != nil {
					return nil, fmt.Errorf("dataset not found: %s", datasetName)
				}
			}
			datasetsToSearch = append(datasetsToSearch, ds)
			kbID := getString(ds.Metadata["id"])
			if kbID != "" {
				kbIDs = append(kbIDs, kbID)
			}
		}
	} else {
		// Search in all datasets
		allResult, err := p.listDatasets(ctx, nil)
		if err != nil {
			return nil, err
		}
		datasetsToSearch = allResult.Nodes
		for _, ds := range datasetsToSearch {
			kbID := getString(ds.Metadata["id"])
			if kbID != "" {
				kbIDs = append(kbIDs, kbID)
			}
		}
	}

	if len(kbIDs) == 0 {
		return &Result{
			Nodes: []*Node{},
			Total: 0,
		}, nil
	}

	// Build kb_id -> dataset name mapping
	kbIDToName := make(map[string]string)
	for _, ds := range datasetsToSearch {
		kbID := getString(ds.Metadata["id"])
		if kbID != "" && ds.Name != "" {
			kbIDToName[kbID] = ds.Name
		}
	}

	// Build retrieval request
	payload := map[string]interface{}{
		"kb_id":    kbIDs,
		"question": opts.Query,
	}

	// Set top_k (default to 10 if not specified)
	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	payload["top_k"] = topK

	// Set similarity threshold (default to 0.2 if not specified to match UI behavior)
	threshold := opts.Threshold
	if threshold <= 0 {
		threshold = 0.2
	}
	payload["similarity_threshold"] = threshold

	// Call retrieval API (useAPIBase=false because the route is /v1/chunk/retrieval_test, not /api/v1/...)
	resp, err := p.httpClient.Request("POST", "/chunk/retrieval_test", false, "auto", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("retrieval request failed: %w", err)
	}

	var apiResp struct {
		Code    int                    `json:"code"`
		Data    map[string]interface{} `json:"data"`
		Message string                 `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	// Parse chunks from response
	var nodes []*Node
	if chunksData, ok := apiResp.Data["chunks"].([]interface{}); ok {
		for _, chunk := range chunksData {
			if chunkMap, ok := chunk.(map[string]interface{}); ok {
				node := p.chunkToNodeWithKBMapping(chunkMap, kbIDToName)
				nodes = append(nodes, node)
			}
		}
	}

	// Apply top_k limit if specified (API may return more results)
	if topK > 0 && len(nodes) > topK {
		nodes = nodes[:topK]
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// chunkToNodeWithKBMapping converts a chunk map to a Node with kb_id -> name mapping
func (p *DatasetProvider) chunkToNodeWithKBMapping(chunk map[string]interface{}, kbIDToName map[string]string) *Node {
	// Extract chunk content - try multiple field names
	content := ""
	if v, ok := chunk["content_with_weight"].(string); ok && v != "" {
		content = v
	} else if v, ok := chunk["content"].(string); ok && v != "" {
		content = v
	} else if v, ok := chunk["content_ltks"].(string); ok && v != "" {
		content = v
	} else if v, ok := chunk["text"].(string); ok && v != "" {
		content = v
	}

	// Get chunk_id for URI
	chunkID := ""
	if v, ok := chunk["chunk_id"].(string); ok {
		chunkID = v
	} else if v, ok := chunk["id"].(string); ok {
		chunkID = v
	}

	// Get document name and ID
	docName := ""
	if v, ok := chunk["docnm_kwd"].(string); ok && v != "" {
		docName = v
	} else if v, ok := chunk["docnm"].(string); ok && v != "" {
		docName = v
	} else if v, ok := chunk["doc_name"].(string); ok && v != "" {
		docName = v
	}

	docID := ""
	if v, ok := chunk["doc_id"].(string); ok && v != "" {
		docID = v
	}

	// Get dataset/kb name from mapping or chunk data
	datasetName := ""
	datasetID := ""

	// First try to get kb_id from chunk (could be string or array)
	if v, ok := chunk["kb_id"].(string); ok && v != "" {
		datasetID = v
	} else if v, ok := chunk["kb_id"].([]interface{}); ok && len(v) > 0 {
		if s, ok := v[0].(string); ok {
			datasetID = s
		}
	}

	// Look up dataset name from mapping using kb_id
	if datasetID != "" && kbIDToName != nil {
		if name, ok := kbIDToName[datasetID]; ok && name != "" {
			datasetName = name
		}
	}

	// Fallback to kb_name from chunk if mapping doesn't have it
	if datasetName == "" {
		if v, ok := chunk["kb_name"].(string); ok && v != "" {
			datasetName = v
		}
	}

	// Build URI path: prefer names over IDs for readability
	// Format: datasets/{dataset_name}/{doc_name}
	path := "/datasets"
	if datasetName != "" {
		path += "/" + datasetName
	} else if datasetID != "" {
		path += "/" + datasetID
	}
	if docName != "" {
		path += "/" + docName
	} else if docID != "" {
		path += "/" + docID
	}

	// Use doc_name or chunk_id as the name if content is empty
	name := content
	if name == "" {
		if docName != "" {
			name = docName
		} else if chunkID != "" {
			name = "chunk:" + chunkID[:min(len(chunkID), 16)]
		} else {
			name = "(empty)"
		}
	}

	node := &Node{
		Name:     name,
		Path:     path,
		Type:     NodeTypeDocument,
		Metadata: chunk,
	}

	// Parse timestamps if available
	if createTime, ok := chunk["create_time"]; ok {
		node.CreatedAt = parseTime(createTime)
	}
	if updateTime, ok := chunk["update_time"]; ok {
		node.UpdatedAt = parseTime(updateTime)
	}

	return node
}

// chunkToNode converts a chunk map to a Node (legacy, uses chunk data only)
func (p *DatasetProvider) chunkToNode(chunk map[string]interface{}) *Node {
	return p.chunkToNodeWithKBMapping(chunk, nil)
}

// ==================== Document Operations ====================

func (p *DatasetProvider) listDocuments(ctx stdctx.Context, datasetName string, opts *ListOptions) (*Result, error) {
	// First get the dataset ID
	ds, err := p.getDataset(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	datasetID := getString(ds.Metadata["id"])
	if datasetID == "" {
		return nil, fmt.Errorf("dataset ID not found")
	}

	// Build query parameters
	params := make(map[string]string)
	if opts != nil {
		if opts.Limit > 0 {
			params["page_size"] = fmt.Sprintf("%d", opts.Limit)
		}
		if opts.Offset > 0 {
			params["page"] = fmt.Sprintf("%d", opts.Offset/opts.Limit+1)
		}
	}

	path := fmt.Sprintf("/datasets/%s/documents", datasetID)
	resp, err := p.httpClient.Request("GET", path, true, "auto", params, nil)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code    int `json:"code"`
		Data    struct {
			Docs []map[string]interface{} `json:"docs"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	nodes := make([]*Node, 0, len(apiResp.Data.Docs))
	for _, doc := range apiResp.Data.Docs {
		node := p.documentToNode(doc, datasetName)
		nodes = append(nodes, node)
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

func (p *DatasetProvider) getDocumentNode(ctx stdctx.Context, datasetName, docName string) (*Result, error) {
	node, err := p.getDocument(ctx, datasetName, docName)
	if err != nil {
		return nil, err
	}
	return &Result{
		Nodes: []*Node{node},
		Total: 1,
	}, nil
}

func (p *DatasetProvider) getDocument(ctx stdctx.Context, datasetName, docName string) (*Node, error) {
	// List all documents and find the matching one
	result, err := p.listDocuments(ctx, datasetName, nil)
	if err != nil {
		return nil, err
	}

	for _, node := range result.Nodes {
		if node.Name == docName {
			return node, nil
		}
	}

	return nil, fmt.Errorf("%s: document '%s' in dataset '%s'", ErrNotFound, docName, datasetName)
}

func (p *DatasetProvider) searchDocuments(ctx stdctx.Context, datasetName string, opts *SearchOptions) (*Result, error) {
	// If no query is provided, just list documents
	if opts.Query == "" {
		return p.listDocuments(ctx, datasetName, &ListOptions{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		})
	}

	// Use retrieval API for semantic search in specific dataset
	ds, err := p.getDataset(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	kbID := getString(ds.Metadata["id"])
	if kbID == "" {
		return nil, fmt.Errorf("dataset ID not found for '%s'", datasetName)
	}

	// Build kb_id -> dataset name mapping
	kbIDToName := map[string]string{kbID: datasetName}

	// Build retrieval request for specific dataset
	payload := map[string]interface{}{
		"kb_id":    []string{kbID},
		"question": opts.Query,
	}

	// Set top_k (default to 10 if not specified)
	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	payload["top_k"] = topK

	// Set similarity threshold (default to 0.2 if not specified to match UI behavior)
	threshold := opts.Threshold
	if threshold <= 0 {
		threshold = 0.2
	}
	payload["similarity_threshold"] = threshold

	// Call retrieval API (useAPIBase=false because the route is /v1/chunk/retrieval_test, not /api/v1/...)
	resp, err := p.httpClient.Request("POST", "/chunk/retrieval_test", false, "auto", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("retrieval request failed: %w", err)
	}

	var apiResp struct {
		Code    int                    `json:"code"`
		Data    map[string]interface{} `json:"data"`
		Message string                 `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	// Parse chunks from response
	var nodes []*Node
	if chunksData, ok := apiResp.Data["chunks"].([]interface{}); ok {
		for _, chunk := range chunksData {
			if chunkMap, ok := chunk.(map[string]interface{}); ok {
				node := p.chunkToNodeWithKBMapping(chunkMap, kbIDToName)
				nodes = append(nodes, node)
			}
		}
	}

	// Apply top_k limit if specified (API may return more results)
	if topK > 0 && len(nodes) > topK {
		nodes = nodes[:topK]
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// ==================== Helper Functions ====================

func (p *DatasetProvider) datasetToNode(ds map[string]interface{}) *Node {
	name := getString(ds["name"])
	node := &Node{
		Name:     name,
		Path:     "/datasets/" + name,
		Type:     NodeTypeDirectory,
		Metadata: ds,
	}

	// Parse timestamps - try multiple field names
	if createTime, ok := ds["create_time"]; ok && createTime != nil {
		node.CreatedAt = parseTime(createTime)
	} else if createDate, ok := ds["create_date"]; ok && createDate != nil {
		node.CreatedAt = parseTime(createDate)
	}

	if updateTime, ok := ds["update_time"]; ok && updateTime != nil {
		node.UpdatedAt = parseTime(updateTime)
	} else if updateDate, ok := ds["update_date"]; ok && updateDate != nil {
		node.UpdatedAt = parseTime(updateDate)
	}

	return node
}

func (p *DatasetProvider) documentToNode(doc map[string]interface{}, datasetName string) *Node {
	name := getString(doc["name"])
	node := &Node{
		Name:     name,
		Path:     "datasets/" + datasetName + "/" + name,
		Type:     NodeTypeDocument,
		Metadata: doc,
	}

	// Parse size
	if size, ok := doc["size"]; ok {
		node.Size = int64(getFloat(size))
	}

	// Parse timestamps
	if createTime, ok := doc["create_time"]; ok {
		node.CreatedAt = parseTime(createTime)
	}
	if updateTime, ok := doc["update_time"]; ok {
		node.UpdatedAt = parseTime(updateTime)
	}

	return node
}

func getString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getFloat(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func parseTime(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}

	var ts int64
	switch val := v.(type) {
	case float64:
		ts = int64(val)
	case int64:
		ts = val
	case int:
		ts = int64(val)
	case string:
		// Trim quotes if present
		val = strings.Trim(val, `"`)
		// Try to parse as number (timestamp)
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			ts = parsed
		} else {
			// If it's already a formatted date string, try parsing it
			formats := []string{
				"2006-01-02 15:04:05",
				"2006-01-02T15:04:05",
				"2006-01-02T15:04:05Z",
				"2006-01-02",
			}
			for _, format := range formats {
				if t, err := time.Parse(format, val); err == nil {
					return t
				}
			}
			return time.Time{}
		}
	default:
		return time.Time{}
	}

	// Convert milliseconds to seconds if timestamp is in milliseconds (13 digits)
	if ts > 1e12 {
		ts = ts / 1000
	}

	return time.Unix(ts, 0)
}
