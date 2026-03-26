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
	"net/url"
	"strings"
	"time"
)

// FileProvider handles file manager operations using Go backend API
// Path structure:
//   - files/                      -> Root folder (list files)
//   - files/{folder_name}/        -> List folder contents
//   - files/{folder_name}/{sub}/  -> Navigate subdirectories
//   - files/{path}/{filename}     -> Get file info or content
//
// Note: Uses Go backend API on port 9384 (useAPIBase=false, NonAPIBase already includes /v1):
//   - GET /file/list              -> List files (with optional parent_id)
//   - GET /file/get?id={id}       -> Get file by ID
//   - GET /file/parent_folder?file_id={id} -> Get parent folder
//   - GET /file/all_parent_folder?file_id={id} -> Get all parent folders
//   - POST /file/create           -> Create folder
//   - POST /file/delete           -> Delete files
type FileProvider struct {
	BaseProvider
	httpClient HTTPClientInterface
	rootFolder *FileNode // Cache root folder info
}

// FileNode represents a file/folder in the file manager
type FileNode struct {
	ID         string                 `json:"id"`
	ParentID   string                 `json:"parent_id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // "folder" or "file"
	Size       int64                  `json:"size"`
	CreateTime int64                  `json:"create_time"`
	UpdateTime int64                  `json:"update_time"`
	Location   string                 `json:"location,omitempty"`
	Metadata   map[string]interface{} `json:"-"`
}

// NewFileProvider creates a new FileProvider
func NewFileProvider(httpClient HTTPClientInterface) *FileProvider {
	return &FileProvider{
		BaseProvider: BaseProvider{
			name:        "files",
			description: "File manager provider",
			rootPath:    "files",
		},
		httpClient: httpClient,
	}
}

// Supports returns true if this provider can handle the given path
func (p *FileProvider) Supports(path string) bool {
	normalized := normalizePath(path)
	return normalized == "files" || strings.HasPrefix(normalized, "files/")
}

// List lists nodes at the given path
func (p *FileProvider) List(ctx stdctx.Context, subPath string, opts *ListOptions) (*Result, error) {
	// subPath is the path relative to "files/"
	// Empty subPath means list root folder

	if subPath == "" {
		return p.listRootFolder(ctx, opts)
	}

	// Try to resolve the path to a folder
	fileNode, err := p.resolvePath(ctx, subPath)
	if err != nil {
		return nil, err
	}

	if fileNode.Type != "folder" {
		// It's a file, return it as a single node
		return &Result{
			Nodes: []*Node{p.fileToNode(fileNode, subPath)},
			Total: 1,
		}, nil
	}

	// It's a folder, list its contents
	return p.listFolderContents(ctx, fileNode.ID, subPath, opts)
}

// Search searches for files matching the query
func (p *FileProvider) Search(ctx stdctx.Context, subPath string, opts *SearchOptions) (*Result, error) {
	if opts.Query == "" {
		return p.List(ctx, subPath, &ListOptions{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		})
	}

	// Get the parent folder ID if path is specified
	var parentID string
	if subPath != "" {
		fileNode, err := p.resolvePath(ctx, subPath)
		if err != nil {
			return nil, err
		}
		if fileNode.Type != "folder" {
			return nil, fmt.Errorf("path is not a folder: %s", subPath)
		}
		parentID = fileNode.ID
	}

	// Search files with keywords
	return p.searchFiles(ctx, parentID, opts)
}

// Mkdir creates a new folder
func (p *FileProvider) Mkdir(ctx stdctx.Context, subPath string, params map[string]interface{}) (*Node, error) {
	if subPath == "" {
		return nil, fmt.Errorf("cannot create folder without a name")
	}

	parts := SplitPath(subPath)
	folderName := parts[len(parts)-1]
	parentPath := ""
	if len(parts) > 1 {
		parentPath = joinStrings(parts[:len(parts)-1], "/")
	}

	// Get parent folder ID
	var parentID string
	if parentPath == "" {
		// Create under root
		rootFolder, err := p.getRootFolder(ctx)
		if err != nil {
			return nil, err
		}
		parentID = rootFolder.ID
	} else {
		parentNode, err := p.resolvePath(ctx, parentPath)
		if err != nil {
			return nil, err
		}
		if parentNode.Type != "folder" {
			return nil, fmt.Errorf("parent path is not a folder: %s", parentPath)
		}
		parentID = parentNode.ID
	}

	// Create the folder via API
	return p.createFolder(ctx, parentID, folderName)
}

// Cat retrieves file content
func (p *FileProvider) Cat(ctx stdctx.Context, subPath string) ([]byte, error) {
	if subPath == "" {
		return nil, fmt.Errorf("cat requires a file path")
	}

	// Resolve path to file
	fileNode, err := p.resolvePath(ctx, subPath)
	if err != nil {
		return nil, err
	}

	if fileNode.Type == "folder" {
		return nil, fmt.Errorf("'%s' is a directory, not a file", subPath)
	}

	// Get file content via API
	return p.getFileContent(ctx, fileNode.ID, fileNode.Location)
}

// Rm removes a file or folder
func (p *FileProvider) Rm(ctx stdctx.Context, subPath string, recursive bool) error {
	if subPath == "" {
		return fmt.Errorf("cannot remove root folder")
	}

	// Resolve path to file/folder
	fileNode, err := p.resolvePath(ctx, subPath)
	if err != nil {
		return err
	}

	// Check if it's a folder with children
	if fileNode.Type == "folder" && !recursive {
		hasChildren, err := p.hasChildren(ctx, fileNode.ID)
		if err != nil {
			return err
		}
		if hasChildren {
			return fmt.Errorf("folder is not empty, use -r to remove recursively")
		}
	}

	// Delete the file/folder
	return p.deleteFile(ctx, fileNode.ID)
}

// ==================== Helper Methods ====================

// getRootFolder gets the root folder for the current user
// Go backend has a dedicated root_folder endpoint
func (p *FileProvider) getRootFolder(ctx stdctx.Context) (*FileNode, error) {
	// Return cached root folder if available
	if p.rootFolder != nil {
		return p.rootFolder, nil
	}

	// Use Go backend API to get root folder (NonAPIBase already includes /v1)
	resp, err := p.httpClient.Request("GET", "/file/root_folder", false, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			RootFolder map[string]interface{} `json:"root_folder"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	// Extract root folder info from root_folder field
	if apiResp.Data.RootFolder == nil {
		return nil, fmt.Errorf("cannot determine root folder")
	}

	p.rootFolder = p.mapToFileNode(apiResp.Data.RootFolder)
	return p.rootFolder, nil
}

// listRootFolder lists the contents of the root folder
func (p *FileProvider) listRootFolder(ctx stdctx.Context, opts *ListOptions) (*Result, error) {
	rootFolder, err := p.getRootFolder(ctx)
	if err != nil {
		return nil, err
	}
	return p.listFolderContents(ctx, rootFolder.ID, "", opts)
}

// listFolderContents lists the contents of a folder by ID
func (p *FileProvider) listFolderContents(ctx stdctx.Context, folderID string, path string, opts *ListOptions) (*Result, error) {
	// Build query parameters
	queryParams := make(map[string]string)
	if opts != nil {
		if opts.Limit > 0 {
			queryParams["page_size"] = fmt.Sprintf("%d", opts.Limit)
		} else {
			queryParams["page_size"] = "100" // Default
		}
		if opts.Offset > 0 {
			page := opts.Offset/opts.Limit + 1
			queryParams["page"] = fmt.Sprintf("%d", page)
		}
	} else {
		queryParams["page_size"] = "100"
	}

	// Always add parent_id for non-root folders
	rootFolder, err := p.getRootFolder(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get root folder: %w", err)
	}

	if folderID != rootFolder.ID {
		queryParams["parent_id"] = folderID
	}

	// Build URL with query parameters for Go backend API (NonAPIBase already includes /v1)
	apiPath := "/file/list"
	if len(queryParams) > 0 {
		apiPath += "?" + buildQueryString(queryParams)
	}

	resp, err := p.httpClient.Request("GET", apiPath, false, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	// Check HTTP status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Body))
	}

	var apiResp struct {
		Code    int `json:"code"`
		Data    struct {
			Total int64                    `json:"total"`
			Files []map[string]interface{} `json:"files"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(resp.Body))
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	nodes := make([]*Node, 0, len(apiResp.Data.Files))
	for _, file := range apiResp.Data.Files {
		fileNode := p.mapToFileNode(file)
		// Skip hidden .knowledgebase folder
		if strings.TrimSpace(fileNode.Name) == ".knowledgebase" {
			continue
		}
		filePath := path
		if filePath != "" {
			filePath = filePath + "/" + fileNode.Name
		} else {
			filePath = fileNode.Name
		}
		nodes = append(nodes, p.fileToNode(fileNode, filePath))
	}

	return &Result{
		Nodes: nodes,
		Total: int(apiResp.Data.Total),
	}, nil
}

// resolvePath resolves a path string to a FileNode
// Path components are looked up by name from the root
func (p *FileProvider) resolvePath(ctx stdctx.Context, path string) (*FileNode, error) {
	parts := SplitPath(path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	// Check if trying to access hidden .knowledgebase
	for _, part := range parts {
		if strings.TrimSpace(part) == ".knowledgebase" {
			return nil, fmt.Errorf("invalid path: .knowledgebase is not accessible")
		}
	}

	// Start from root
	currentFolder, err := p.getRootFolder(ctx)
	if err != nil {
		return nil, err
	}

	// Traverse path components
	for i, part := range parts {
		// Build current path for this level
		currentPath := joinStrings(parts[:i], "/")

		// List contents of current folder
		result, err := p.listFolderContents(ctx, currentFolder.ID, currentPath, nil)
		if err != nil {
			return nil, err
		}

		// Find the matching child
		var found *FileNode
		for _, node := range result.Nodes {
			if node.Name == part {
				// Convert Node back to FileNode using metadata
				if _, ok := node.Metadata["id"]; ok {
					found = p.mapToFileNode(node.Metadata)
				} else {
					return nil, fmt.Errorf("node missing id: %s", part)
				}
				break
			}
		}

		if found == nil {
			return nil, fmt.Errorf("%s: '%s'", ErrNotFound, path)
		}

		// If this is the last part, return it
		if i == len(parts)-1 {
			return found, nil
		}

		// If not the last part, it must be a folder
		if found.Type != "folder" {
			return nil, fmt.Errorf("'%s' is not a directory", part)
		}

		currentFolder = found
	}

	return currentFolder, nil
}

// getFileByID gets file info by ID
func (p *FileProvider) getFileByID(ctx stdctx.Context, fileID string) (*FileNode, error) {
	apiPath := "/file/get?id=" + url.QueryEscape(fileID)
	resp, err := p.httpClient.Request("GET", apiPath, false, "auto", nil, nil)
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

	return p.mapToFileNode(apiResp.Data), nil
}

// getFileContent retrieves file content from storage via API
func (p *FileProvider) getFileContent(ctx stdctx.Context, fileID string, location string) ([]byte, error) {
	apiPath := "/file/content?id=" + url.QueryEscape(fileID)
	resp, err := p.httpClient.Request("GET", apiPath, false, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	// Check if it's an error response (JSON)
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

	// Return raw content
	return resp.Body, nil
}

// searchFiles searches for files with keywords
func (p *FileProvider) searchFiles(ctx stdctx.Context, parentID string, opts *SearchOptions) (*Result, error) {
	// Build query parameters
	queryParams := make(map[string]string)
	if parentID != "" {
		queryParams["parent_id"] = parentID
	}
	if opts.Query != "" {
		queryParams["keywords"] = opts.Query
	}
	if opts.Limit > 0 {
		queryParams["page_size"] = fmt.Sprintf("%d", opts.Limit)
	} else {
		queryParams["page_size"] = "100"
	}
	if opts.Offset > 0 {
		page := opts.Offset/opts.Limit + 1
		queryParams["page"] = fmt.Sprintf("%d", page)
	}

	// Build URL with query parameters for Go backend API (NonAPIBase already includes /v1)
	apiPath := "/file/list"
	if len(queryParams) > 0 {
		apiPath += "?" + buildQueryString(queryParams)
	}

	resp, err := p.httpClient.Request("GET", apiPath, false, "auto", nil, nil)
	if err != nil {
		return nil, err
	}

	// Check HTTP status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Body))
	}

	var apiResp struct {
		Code    int `json:"code"`
		Data    struct {
			Total int64                    `json:"total"`
			Files []map[string]interface{} `json:"files"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(resp.Body))
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	nodes := make([]*Node, 0, len(apiResp.Data.Files))
	for _, file := range apiResp.Data.Files {
		fileNode := p.mapToFileNode(file)
		nodes = append(nodes, p.fileToNode(fileNode, fileNode.Name))
	}

	return &Result{
		Nodes: nodes,
		Total: int(apiResp.Data.Total),
	}, nil
}

// createFolder creates a new folder
func (p *FileProvider) createFolder(ctx stdctx.Context, parentID string, name string) (*Node, error) {
	payload := map[string]interface{}{
		"name":      name,
		"parent_id": parentID,
		"type":      "folder",
	}

	resp, err := p.httpClient.Request("POST", "/file/create", false, "auto", nil, payload)
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

	return p.fileToNode(p.mapToFileNode(apiResp.Data), name), nil
}

// hasChildren checks if a folder has any children
func (p *FileProvider) hasChildren(ctx stdctx.Context, folderID string) (bool, error) {
	result, err := p.listFolderContents(ctx, folderID, "", &ListOptions{Limit: 1})
	if err != nil {
		return false, err
	}
	return len(result.Nodes) > 0, nil
}

// deleteFile deletes a file or folder
func (p *FileProvider) deleteFile(ctx stdctx.Context, fileID string) error {
	payload := map[string]interface{}{
		"ids": []string{fileID},
	}

	resp, err := p.httpClient.Request("POST", "/file/delete", false, "auto", nil, payload)
	if err != nil {
		return err
	}

	var apiResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return err
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API error: %s", apiResp.Message)
	}

	return nil
}

// ==================== Conversion Functions ====================

// mapToFileNode converts a map to a FileNode
func (p *FileProvider) mapToFileNode(m map[string]interface{}) *FileNode {
	if m == nil {
		return nil
	}

	node := &FileNode{
		ID:       getString(m["id"]),
		ParentID: getString(m["parent_id"]),
		Name:     getString(m["name"]),
		Type:     getString(m["type"]),
		Metadata: m,
	}

	if size, ok := m["size"]; ok {
		node.Size = int64(getFloat(size))
	}

	if loc, ok := m["location"].(string); ok {
		node.Location = loc
	}

	if createTime, ok := m["create_time"]; ok {
		node.CreateTime = int64(getFloat(createTime))
	}

	if updateTime, ok := m["update_time"]; ok {
		node.UpdateTime = int64(getFloat(updateTime))
	}

	return node
}

// fileToNode converts a FileNode to a contextengine Node
func (p *FileProvider) fileToNode(file *FileNode, path string) *Node {
	nodeType := NodeTypeFile
	if file.Type == "folder" {
		nodeType = NodeTypeDirectory
	}

	// Build display path without /files/ prefix
	displayPath := path
	if displayPath != "" {
		displayPath = path
	} else {
		displayPath = file.Name
	}

	node := &Node{
		Name:     file.Name,
		Path:     displayPath,
		Type:     nodeType,
		Size:     file.Size,
		Metadata: file.Metadata,
	}

	// Parse timestamps
	if file.CreateTime > 0 {
		// Convert milliseconds to seconds if needed
		ts := file.CreateTime
		if ts > 1e12 {
			ts = ts / 1000
		}
		node.CreatedAt = time.Unix(ts, 0)
	}

	if file.UpdateTime > 0 {
		ts := file.UpdateTime
		if ts > 1e12 {
			ts = ts / 1000
		}
		node.UpdatedAt = time.Unix(ts, 0)
	}

	return node
}

// buildQueryString builds a URL query string from a map
func buildQueryString(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return values.Encode()
}
