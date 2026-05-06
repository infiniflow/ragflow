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
// Path structure: files/ or files/{folder_name}/ or files/{folder_name}/{sub_path}/...
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

	// For multi-level paths like myskills/skill-name/dir1, recursively traverse
	return p.listPathRecursive(ctx, parts, opts)
}

// listPathRecursive recursively traverses the path and lists the final component
func (p *FileProvider) listPathRecursive(ctx stdctx.Context, parts []string, opts *ListOptions) (*Result, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	// Start from root to find the first folder
	currentFolderID, err := p.getFolderIDByName(ctx, parts[0])
	if err != nil {
		return nil, err
	}
	currentPath := parts[0]

	// Traverse through intermediate directories
	for i := 1; i < len(parts); i++ {
		partName := parts[i]

		// List contents of current folder to find the next part
		result, err := p.listFilesByParentID(ctx, currentFolderID, currentPath, nil)
		if err != nil {
			return nil, err
		}

		// Find the next component
		found := false
		for _, node := range result.Nodes {
			if node.Name == partName {
				if i == len(parts)-1 {
					// This is the last component - if it's a directory, list its contents
					if node.Type == NodeTypeDirectory {
						childID := getString(node.Metadata["id"])
						if childID == "" {
							return nil, fmt.Errorf("folder ID not found for '%s'", partName)
						}
						newPath := currentPath + "/" + partName
						p.folderCache[newPath] = childID
						return p.listFilesByParentID(ctx, childID, newPath, opts)
					}
					// It's a file - return the file node
					return &Result{
						Nodes: []*Node{node},
						Total: 1,
					}, nil
				}
				// Not the last component - must be a directory
				if node.Type != NodeTypeDirectory {
					return nil, fmt.Errorf("'%s' is not a directory", partName)
				}
				childID := getString(node.Metadata["id"])
				if childID == "" {
					return nil, fmt.Errorf("folder ID not found for '%s'", partName)
				}
				currentFolderID = childID
				currentPath = currentPath + "/" + partName
				p.folderCache[currentPath] = currentFolderID
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("%s: '%s' in '%s'", ErrNotFound, partName, currentPath)
		}
	}

	// Should have returned in the loop, but just in case
	return p.listFilesByParentID(ctx, currentFolderID, currentPath, opts)
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

	// Find the file by recursively traversing the path
	node, err := p.findNodeByPath(ctx, parts)
	if err != nil {
		return nil, err
	}

	if node.Type == NodeTypeDirectory {
		return nil, fmt.Errorf("'%s' is a directory, not a file", subPath)
	}

	fileID := getString(node.Metadata["id"])
	if fileID == "" {
		return nil, fmt.Errorf("file ID not found")
	}

	// Download file content
	return p.downloadFile(ctx, fileID)
}

// findNodeByPath recursively traverses the path to find the target node
func (p *FileProvider) findNodeByPath(ctx stdctx.Context, parts []string) (*Node, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	// Start from root to find the first folder
	currentFolderID, err := p.getFolderIDByName(ctx, parts[0])
	if err != nil {
		return nil, err
	}
	currentPath := parts[0]

	// Traverse through intermediate directories
	for i := 1; i < len(parts); i++ {
		partName := parts[i]

		// List contents of current folder to find the next part
		result, err := p.listFilesByParentID(ctx, currentFolderID, currentPath, nil)
		if err != nil {
			return nil, err
		}

		// Find the next component
		found := false
		for _, node := range result.Nodes {
			if node.Name == partName {
				if i == len(parts)-1 {
					// This is the last component - return it
					return node, nil
				}
				// Not the last component - must be a directory
				if node.Type != NodeTypeDirectory {
					return nil, fmt.Errorf("'%s' is not a directory", partName)
				}
				childID := getString(node.Metadata["id"])
				if childID == "" {
					return nil, fmt.Errorf("folder ID not found for '%s'", partName)
				}
				currentFolderID = childID
				currentPath = currentPath + "/" + partName
				p.folderCache[currentPath] = currentFolderID
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("%s: '%s' in '%s'", ErrNotFound, partName, currentPath)
		}
	}

	return nil, fmt.Errorf("%s: '%s'", ErrNotFound, strings.Join(parts, "/"))
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
// If fileName is a directory, returns the directory contents instead of the directory node
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
			// If it's a directory, list its contents instead of returning the node itself
			if node.Type == NodeTypeDirectory {
				childFolderID := getString(node.Metadata["id"])
				if childFolderID == "" {
					return nil, fmt.Errorf("folder ID not found for '%s'", fileName)
				}
				// Cache the folder ID
				cacheKey := folderName + "/" + fileName
				p.folderCache[cacheKey] = childFolderID
				// Return directory contents
				return p.listFilesByParentID(ctx, childFolderID, cacheKey, nil)
			}
			// Return file node
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

// DeleteFile deletes a file or folder by its ID
func (p *FileProvider) DeleteFile(ctx stdctx.Context, fileID string) error {
	// Use JSON body format expected by Python backend: {"ids": ["file_id"]}
	payload := map[string]interface{}{
		"ids": []string{fileID},
	}
	resp, err := p.httpClient.Request("DELETE", "/files", true, "api", nil, payload)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}

	// Handle empty response (e.g., 204 No Content)
	if len(resp.Body) == 0 {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("delete failed with status code: %d", resp.StatusCode)
	}

	var apiResp struct {
		Code    int         `json:"code"`
		Data    interface{} `json:"data"`
		Message string      `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("delete failed: %s", apiResp.Message)
	}

	return nil
}

// DeleteFolderByPath deletes a folder by its path (e.g., "skills/hub11/skill-name")
func (p *FileProvider) DeleteFolderByPath(ctx stdctx.Context, folderPath string) error {
	parts := SplitPath(folderPath)
	if len(parts) == 0 {
		return fmt.Errorf("empty folder path")
	}

	// Find the folder ID by traversing the path
	var folderID string
	currentPath := ""

	for i, part := range parts {
		if i == 0 {
			// First part - find in root
			id, err := p.getFolderIDByName(ctx, part)
			if err != nil {
				return fmt.Errorf("folder not found: %s", part)
			}
			folderID = id
			currentPath = part
		} else {
			// Subsequent parts - find in parent folder
			result, err := p.listFilesByParentID(ctx, folderID, currentPath, nil)
			if err != nil {
				return fmt.Errorf("failed to list folder contents: %w", err)
			}

			found := false
			for _, node := range result.Nodes {
				if node.Name == part && node.Type == NodeTypeDirectory {
					folderID = getString(node.Metadata["id"])
					if folderID == "" {
						return fmt.Errorf("folder ID not found for: %s", part)
					}
					currentPath = currentPath + "/" + part
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("folder not found: %s in %s", part, currentPath)
			}
		}
	}

	// Delete the folder
	return p.DeleteFile(ctx, folderID)
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
