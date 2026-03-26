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

package contextengine

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"strings"
)

// FileProvider handles file operations using Python backend /files API
// Path structure:
//   - files/                             -> List root folder contents
//   - files/{folder_name}/               -> List folder contents
//   - files/{folder_name}/{file_name}    -> Get file info/content
//
// Note: Uses Python backend API (useAPIBase=true):
//   - GET /files?parent_id={id}         -> List files/folders in parent
//   - GET /files/{file_id}              -> Get file info
//   - POST /files                       -> Create folder or upload file
//   - DELETE /files                     -> Delete files
//   - GET /files/{file_id}/parent       -> Get parent folder
//   - GET /files/{file_id}/ancestors    -> Get ancestor folders

type FileProvider struct {
	BaseProvider
	httpClient  HTTPClientInterface
	folderCache map[string]string // path -> folder ID cache
	rootID      string            // root folder ID
}

// NewFileProvider creates a new FileProvider
func NewFileProvider(httpClient HTTPClientInterface) *FileProvider {
	return &FileProvider{
		BaseProvider: BaseProvider{
			name:        "files",
			description: "File manager provider (Python server)",
			rootPath:    "files",
		},
		httpClient:  httpClient,
		folderCache: make(map[string]string),
	}
}

// Supports returns true if this provider can handle the given path
func (p *FileProvider) Supports(path string) bool {
	normalized := normalizePath(path)
	return normalized == "files" || strings.HasPrefix(normalized, "files/")
}

// List lists nodes at the given path
// Path structure: files/ or files/{folder_name}/ or files/{folder_name}/{file_name}
func (p *FileProvider) List(ctx stdctx.Context, subPath string, opts *ListOptions) (*Result, error) {
	// subPath is the path relative to "files/"
	// Empty subPath means list root folder

	if subPath == "" {
		return p.listRootFolder(ctx, opts)
	}

	parts := SplitPath(subPath)
	if len(parts) == 1 {
		// files/{folder_name} - list contents of this folder
		return p.listFolderByName(ctx, parts[0], opts)
	}

	if len(parts) >= 2 {
		// files/{folder_name}/{file_name}... - get file info
		// Join remaining parts as the file name (could contain "/")
		folderName := parts[0]
		fileName := strings.Join(parts[1:], "/")
		return p.getFileNode(ctx, folderName, fileName)
	}

	return nil, fmt.Errorf("invalid path: %s", subPath)
}

// Search searches for files/folders
func (p *FileProvider) Search(ctx stdctx.Context, subPath string, opts *SearchOptions) (*Result, error) {
	if opts.Query == "" {
		return p.List(ctx, subPath, &ListOptions{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		})
	}

	// For now, search is not implemented - just list and filter by name
	result, err := p.List(ctx, subPath, &ListOptions{
		Limit:  opts.Limit,
		Offset: opts.Offset,
	})
	if err != nil {
		return nil, err
	}

	// Simple name filtering
	var filtered []*Node
	query := strings.ToLower(opts.Query)
	for _, node := range result.Nodes {
		if strings.Contains(strings.ToLower(node.Name), query) {
			filtered = append(filtered, node)
		}
	}

	return &Result{
		Nodes: filtered,
		Total: len(filtered),
	}, nil
}

// Cat retrieves file content
func (p *FileProvider) Cat(ctx stdctx.Context, subPath string) ([]byte, error) {
	if subPath == "" {
		return nil, fmt.Errorf("cat requires a file path: files/{folder}/{file}")
	}

	parts := SplitPath(subPath)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path format, expected: files/{folder}/{file}")
	}

	folderName := parts[0]
	fileName := strings.Join(parts[1:], "/")

	// Get file info first to get file ID
	result, err := p.getFileNode(ctx, folderName, fileName)
	if err != nil {
		return nil, err
	}

	if len(result.Nodes) == 0 {
		return nil, fmt.Errorf("%s: file '%s'", ErrNotFound, fileName)
	}

	fileID := getString(result.Nodes[0].Metadata["id"])
	if fileID == "" {
		return nil, fmt.Errorf("file ID not found")
	}

	// Download file content
	return p.downloadFile(ctx, fileID)
}

// ==================== Python Server API Methods ====================

// getRootID gets or caches the root folder ID
func (p *FileProvider) getRootID(ctx stdctx.Context) (string, error) {
	if p.rootID != "" {
		return p.rootID, nil
	}

	// List files without parent_id to get root folder
	resp, err := p.httpClient.Request("GET", "/files", true, "auto", nil, nil)
	if err != nil {
		return "", err
	}

	var apiResp struct {
		Code    int                    `json:"code"`
		Data    map[string]interface{} `json:"data"`
		Message string                 `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return "", err
	}

	if apiResp.Code != 0 {
		return "", fmt.Errorf("API error: %s", apiResp.Message)
	}

	// Try to find root folder ID from response
	if rootID, ok := apiResp.Data["root_id"].(string); ok && rootID != "" {
		p.rootID = rootID
		return rootID, nil
	}

	// If no explicit root_id, use empty parent_id for root listing
	return "", nil
}

// listRootFolder lists the contents of root folder
func (p *FileProvider) listRootFolder(ctx stdctx.Context, opts *ListOptions) (*Result, error) {
	// Get root folder ID first
	rootID, err := p.getRootID(ctx)
	if err != nil {
		return nil, err
	}
	// List files using root folder ID as parent
	return p.listFilesByParentID(ctx, rootID, "", opts)
}

// listFilesByParentID lists files/folders by parent ID
func (p *FileProvider) listFilesByParentID(ctx stdctx.Context, parentID string, parentPath string, opts *ListOptions) (*Result, error) {
	// Build query parameters
	queryParams := make([]string, 0)
	if parentID != "" {
		queryParams = append(queryParams, fmt.Sprintf("parent_id=%s", parentID))
	}
	// Always set page=1 and page_size to ensure we get results
	pageSize := 100
	if opts != nil && opts.Limit > 0 {
		pageSize = opts.Limit
	}
	queryParams = append(queryParams, fmt.Sprintf("page_size=%d", pageSize))
	queryParams = append(queryParams, "page=1")

	// Build URL with query string
	path := "/files"
	if len(queryParams) > 0 {
		path = path + "?" + strings.Join(queryParams, "&")
	}

	resp, err := p.httpClient.Request("GET", path, true, "auto", nil, nil)
	if err != nil {
		return nil, err
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

	// Extract files list from data - API returns {"total": N, "files": [...], "parent_folder": {...}}
	var files []map[string]interface{}
	if fileList, ok := apiResp.Data["files"].([]interface{}); ok {
		for _, f := range fileList {
			if fileMap, ok := f.(map[string]interface{}); ok {
				files = append(files, fileMap)
			}
		}
	}

	nodes := make([]*Node, 0, len(files))
	for _, f := range files {
		name := getString(f["name"])
		// Skip hidden .knowledgebase folder
		if strings.TrimSpace(name) == ".knowledgebase" {
			continue
		}

		node := p.fileToNode(f, parentPath)
		nodes = append(nodes, node)

		// Cache folder ID
		if node.Type == NodeTypeDirectory || getString(f["type"]) == "folder" {
			if id := getString(f["id"]); id != "" {
				cacheKey := node.Name
				if parentPath != "" {
					cacheKey = parentPath + "/" + node.Name
				}
				p.folderCache[cacheKey] = id
			}
		}
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// listFolderByName lists contents of a folder by its name
func (p *FileProvider) listFolderByName(ctx stdctx.Context, folderName string, opts *ListOptions) (*Result, error) {
	folderID, err := p.getFolderIDByName(ctx, folderName)
	if err != nil {
		return nil, err
	}

	// List files in the folder using folder ID as parent_id
	return p.listFilesByParentID(ctx, folderID, folderName, opts)
}

// getFolderIDByName finds folder ID by its name in root
func (p *FileProvider) getFolderIDByName(ctx stdctx.Context, folderName string) (string, error) {
	// Check cache first
	if id, ok := p.folderCache[folderName]; ok {
		return id, nil
	}

	// List root folder to find the folder
	rootID, _ := p.getRootID(ctx)
	queryParams := make([]string, 0)
	if rootID != "" {
		queryParams = append(queryParams, fmt.Sprintf("parent_id=%s", rootID))
	}
	queryParams = append(queryParams, "page_size=100", "page=1")

	path := "/files"
	if len(queryParams) > 0 {
		path = path + "?" + strings.Join(queryParams, "&")
	}

	resp, err := p.httpClient.Request("GET", path, true, "auto", nil, nil)
	if err != nil {
		return "", err
	}

	var apiResp struct {
		Code    int                    `json:"code"`
		Data    map[string]interface{} `json:"data"`
		Message string                 `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return "", err
	}

	if apiResp.Code != 0 {
		return "", fmt.Errorf("API error: %s", apiResp.Message)
	}

	// Search for folder by name
	var files []map[string]interface{}
	if fileList, ok := apiResp.Data["files"].([]interface{}); ok {
		for _, f := range fileList {
			if fileMap, ok := f.(map[string]interface{}); ok {
				files = append(files, fileMap)
			}
		}
	} else if fileList, ok := apiResp.Data["docs"].([]interface{}); ok {
		for _, f := range fileList {
			if fileMap, ok := f.(map[string]interface{}); ok {
				files = append(files, fileMap)
			}
		}
	}

	for _, f := range files {
		name := getString(f["name"])
		fileType := getString(f["type"])
		id := getString(f["id"])
		// Match by name and ensure it's a folder
		if name == folderName && fileType == "folder" && id != "" {
			p.folderCache[folderName] = id
			return id, nil
		}
	}

	return "", fmt.Errorf("%s: folder '%s'", ErrNotFound, folderName)
}

// getFileNode gets a file node by folder and file name
func (p *FileProvider) getFileNode(ctx stdctx.Context, folderName, fileName string) (*Result, error) {
	folderID, err := p.getFolderIDByName(ctx, folderName)
	if err != nil {
		return nil, err
	}

	// List files in folder to find the file
	result, err := p.listFilesByParentID(ctx, folderID, folderName, nil)
	if err != nil {
		return nil, err
	}

	// Find the specific file
	for _, node := range result.Nodes {
		if node.Name == fileName {
			return &Result{
				Nodes: []*Node{node},
				Total: 1,
			}, nil
		}
	}

	return nil, fmt.Errorf("%s: file '%s' in folder '%s'", ErrNotFound, fileName, folderName)
}

// downloadFile downloads file content
func (p *FileProvider) downloadFile(ctx stdctx.Context, fileID string) ([]byte, error) {
	path := fmt.Sprintf("/files/%s", fileID)
	resp, err := p.httpClient.Request("GET", path, true, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		// Try to parse error response
		var apiResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(resp.Body, &apiResp); err == nil && apiResp.Code != 0 {
			return nil, fmt.Errorf("%s", apiResp.Message)
		}
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	// Return raw file content
	return resp.Body, nil
}

// ==================== Conversion Functions ====================

// fileToNode converts a file map to a Node
func (p *FileProvider) fileToNode(f map[string]interface{}, parentPath string) *Node {
	name := getString(f["name"])
	fileType := getString(f["type"])
	fileID := getString(f["id"])

	// Determine node type
	nodeType := NodeTypeFile
	if fileType == "folder" {
		nodeType = NodeTypeDirectory
	}

	// Build path
	path := name
	if parentPath != "" {
		path = parentPath + "/" + name
	}

	node := &Node{
		Name:     name,
		Path:     path,
		Type:     nodeType,
		Metadata: f,
	}

	// Parse size
	if size, ok := f["size"]; ok {
		node.Size = int64(getFloat(size))
	}

	// Parse timestamps
	if createTime, ok := f["create_time"]; ok && createTime != nil {
		node.CreatedAt = parseTime(createTime)
	}
	if updateTime, ok := f["update_time"]; ok && updateTime != nil {
		node.UpdatedAt = parseTime(updateTime)
	}

	// Store ID for later use
	if fileID != "" {
		if node.Metadata == nil {
			node.Metadata = make(map[string]interface{})
		}
		node.Metadata["id"] = fileID
	}

	return node
}
