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
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"ragflow/internal/logger"
)

// SkillProvider handles skill operations using /skills API
// Path structure:
//   - skills/                            -> List all hubs
//   - skills/{space_id}/                   -> List skills in space
//   - skills/{space_id}/{skill_name}/      -> List versions of skill
//   - skills/{space_id}/{skill_name}/{version}/ -> Get skill version info
//
// Note: Uses Go backend API (useAPIBase=true):
//   - GET /skills/hubs                   -> List all hubs
//   - POST /skills/search                -> Search skills
//   - POST /skills/index                 -> Index skills
//   - DELETE /skills/index/{skill_id}    -> Delete skill index

// ============================================================================
// Constants
// ============================================================================

const (
	MaxSkillTotalSize = 50 * 1024 * 1024 // 50MB
	MaxSkillFileSize  = 5 * 1024 * 1024  // 5MB per file
	DefaultSpaceID      = "default"
)

// Text file extensions allowed in skills
var textFileExtensions = map[string]bool{
	"md": true, "mdx": true, "txt": true, "json": true, "json5": true,
	"yaml": true, "yml": true, "toml": true, "js": true, "cjs": true, "mjs": true,
	"ts": true, "tsx": true, "jsx": true, "py": true, "sh": true, "rb": true,
	"go": true, "rs": true, "swift": true, "kt": true, "java": true, "cs": true,
	"cpp": true, "c": true, "h": true, "hpp": true, "sql": true, "csv": true,
	"ini": true, "cfg": true, "env": true, "xml": true, "html": true,
	"css": true, "scss": true, "sass": true, "svg": true,
}

// Default ignore patterns
var defaultIgnorePatterns = []string{
	".git/", ".svn/", ".hg/", "node_modules/", "__MACOSX/",
	".DS_Store", "._*", "*.log", "*.tmp", "*.temp", "*.swp", "*.swo", "*~",
	".env", ".env.*", ".vscode/", ".idea/", "Thumbs.db", "desktop.ini",
	".skill-meta.json",
}

// ============================================================================
// Types
// ============================================================================

// SkillMetadata represents the metadata from SKILL.md frontmatter
type SkillMetadata struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Version     string      `yaml:"version"`
	Author      string      `yaml:"author"`
	Tags        []string    `yaml:"tags"`
	Tools       interface{} `yaml:"tools"`
}

// SkillValidationResult represents the result of skill validation
type SkillValidationResult struct {
	Valid       bool
	Name        string
	Description string
	Version     string
	Tags        []string
	Error       string
	Details     string
}

// SkillFile represents a file in the skill directory
type SkillFile struct {
	Path    string
	Content []byte
	Size    int64
}

// SkillConflictError represents a conflict error
type SkillConflictError struct {
	Type    string // "name" or "version"
	Name    string
	Version string
}

func (e *SkillConflictError) Error() string {
	if e.Type == "version" {
		return fmt.Sprintf("version conflict: version '%s' already exists for skill '%s'", e.Version, e.Name)
	}
	return fmt.Sprintf("name conflict: skill '%s' already exists", e.Name)
}

// ============================================================================
// SkillProvider
// ============================================================================

type SkillProvider struct {
	BaseProvider
	httpClient HTTPClientInterface
}

// NewSkillProvider creates a new SkillProvider
func NewSkillProvider(httpClient HTTPClientInterface) *SkillProvider {
	return &SkillProvider{
		BaseProvider: BaseProvider{
			name:        "skills",
			description: "Skills provider for skill management and search",
			rootPath:    "skills",
		},
		httpClient: httpClient,
	}
}

// Supports returns true if this provider can handle the given path
func (p *SkillProvider) Supports(path string) bool {
	normalized := normalizePath(path)
	return normalized == "skills" || strings.HasPrefix(normalized, "skills/")
}

// isUUID checks if a string is a valid UUID
func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// List lists nodes at the given path
// Path structure: skills/ or skills/{space_id}/ or skills/{space_id}/{skill_name}/...
func (p *SkillProvider) List(ctx stdctx.Context, subPath string, opts *ListOptions) (*Result, error) {
	if subPath == "" {
		// List all hubs
		return p.listSpaces(ctx, opts)
	}

	parts := SplitPath(subPath)
	
	switch len(parts) {
	case 1:
		// skills/{space_id} - list skills in space
		return p.listSkillsInSpace(ctx, parts[0], opts)
	case 2:
		// skills/{space_id}/{skill_name} - list versions of skill
		return p.listSkillVersions(ctx, parts[0], parts[1], opts)
	default:
		// skills/{space_id}/{skill_name}/{version}/... - skill content
		return p.listSkillContent(ctx, parts[0], parts[1], parts[2], parts[3:], opts)
	}
}

// Search searches for skills matching the query
func (p *SkillProvider) Search(ctx stdctx.Context, subPath string, opts *SearchOptions) (*Result, error) {
	if opts == nil || opts.Query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	// Parse space from path
	spaceName := ""
	parts := SplitPath(subPath)
	if len(parts) > 0 {
		spaceName = parts[0]
	}

	// Space ID can be either a name or UUID
	// If it's not "default" and doesn't look like a UUID, try to convert it
	spaceID := spaceName
	if spaceID != "" && spaceID != "default" && !isUUID(spaceID) {
		spaceUUID, err := p.getSpaceUUIDByName(ctx, spaceID)
		if err == nil {
			spaceID = spaceUUID
		}
		// If lookup fails, use the original spaceID as-is (it might already be a UUID)
	}

	// Build search payload
	page := 1
	pageSize := 10
	if opts.Limit > 0 {
		pageSize = opts.Limit
	}
	if opts.Offset > 0 {
		page = (opts.Offset / pageSize) + 1
	}
	payload := map[string]interface{}{
		"query":      opts.Query,
		"space_id":    spaceID,
		"page":       page,
		"page_size":  pageSize,
	}

	// Call skill search API
	resp, err := p.httpClient.Request("POST", "/skills/search", true, "auto", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Skills []struct {
				SkillID     string   `json:"skill_id"`
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Tags        []string `json:"tags"`
				Score       float64  `json:"score"`
				BM25Score   float64  `json:"bm25_score,omitempty"`
				VectorScore float64  `json:"vector_score,omitempty"`
				CreateTime  int64    `json:"create_time,omitempty"`
			} `json:"skills"`
			Total int `json:"total"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("search failed: %s", result.Msg)
	}

	// Convert to Result format
	nodes := make([]*Node, 0, len(result.Data.Skills))
	for _, skill := range result.Data.Skills {
		var createdAt time.Time
		if skill.CreateTime > 0 {
			createdAt = time.UnixMilli(skill.CreateTime)
		}
		nodes = append(nodes, &Node{
			Name:      skill.Name,
			Type:      NodeTypeDirectory,
			Path:      fmt.Sprintf("skills/%s/%s", spaceName, skill.Name),
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
			Metadata: map[string]interface{}{
				"skill_id":     skill.SkillID,
				"score":        skill.Score,
				"bm25_score":   skill.BM25Score,
				"vector_score": skill.VectorScore,
				"tags":         skill.Tags,
				"description":  skill.Description,
			},
		})
	}

	return &Result{
		Nodes: nodes,
		Total: result.Data.Total,
	}, nil
}

// searchSkillsFromFileSystem performs a simple name-based search via file system
// when the search index is unavailable or empty.
func (p *SkillProvider) searchSkillsFromFileSystem(ctx stdctx.Context, spaceName string, opts *SearchOptions) (*Result, error) {
	listOpts := &ListOptions{
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}
	result, err := p.listSkillsInSpaceFromFileSystem(ctx, spaceName, listOpts)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(opts.Query)
	var matched []*Node
	for _, node := range result.Nodes {
		if strings.Contains(strings.ToLower(node.Name), queryLower) {
			matched = append(matched, node)
		}
	}

	return &Result{
		Nodes: matched,
		Total: len(matched),
	}, nil
}

// Cat retrieves the content of a skill file at the given path
// Path structure: skills/{space_id}/{skill_name}/{version}/.../{file_path}
func (p *SkillProvider) Cat(ctx stdctx.Context, path string) ([]byte, error) {
	parts := SplitPath(path)
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid file path: %s (expected: skills/{space}/{skill}/{version}/.../{file})", path)
	}

	spaceID := parts[0]
	skillName := parts[1]
	version := parts[2]
	_ = JoinPath(parts[3:]...) // file path within version folder (used for nested directories)

	// Get the skill folder ID (search API or file system fallback)
	skillFolderID, err := p.getSkillFolderID(ctx, spaceID, skillName)
	if err != nil {
		return nil, fmt.Errorf("skill '%s' not found in space '%s': %w", skillName, spaceID, err)
	}

	// Find the version folder
	filesResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", skillFolderID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	var filesResult struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(filesResp.Body, &filesResult); err != nil {
		return nil, fmt.Errorf("failed to parse files response: %w", err)
	}

	if filesResult.Code != 0 {
		return nil, fmt.Errorf("failed to list files: %s", filesResult.Msg)
	}

	// Find the version folder
	var versionFolderID string
	for _, file := range filesResult.Data.Files {
		if file.Name == version && file.Type == "folder" {
			versionFolderID = file.ID
			break
		}
	}

	if versionFolderID == "" {
		return nil, fmt.Errorf("version '%s' not found for skill '%s'", version, skillName)
	}

	// Step 4: Navigate to the file through the path
	currentFolderID := versionFolderID
	pathParts := parts[3:]

	// If there's a directory path before the file, navigate through it
	for i := 0; i < len(pathParts)-1; i++ {
		subResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", currentFolderID), true, "auto", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to navigate path: %w", err)
		}

		var subResult struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				Files []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"files"`
			} `json:"data"`
		}

		if err := json.Unmarshal(subResp.Body, &subResult); err != nil {
			return nil, fmt.Errorf("failed to parse navigation response: %w", err)
		}

		if subResult.Code != 0 {
			return nil, fmt.Errorf("navigation failed: %s", subResult.Msg)
		}

		found := false
		for _, file := range subResult.Data.Files {
			if file.Name == pathParts[i] {
				if file.Type != "folder" {
					return nil, fmt.Errorf("'%s' is not a directory", pathParts[i])
				}
				currentFolderID = file.ID
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("directory not found: %s", pathParts[i])
		}
	}

	// Step 5: Find the file in the current directory
	fileName := pathParts[len(pathParts)-1]
	finalResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", currentFolderID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var finalResult struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Type     string `json:"type"`
				Location string `json:"location"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(finalResp.Body, &finalResult); err != nil {
		return nil, fmt.Errorf("failed to parse final response: %w", err)
	}

	if finalResult.Code != 0 {
		return nil, fmt.Errorf("failed to list files: %s", finalResult.Msg)
	}

	// Find the file
	var fileID string
	for _, file := range finalResult.Data.Files {
		if file.Name == fileName {
			fileID = file.ID
			break
		}
	}

	if fileID == "" {
		return nil, fmt.Errorf("file '%s' not found", fileName)
	}

	// Step 6: Download the file content
	// First get file info to get the download URL
	contentResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files/%s", fileID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// For now, return a placeholder - actual file download may need storage access
	// The file content is stored in the storage backend
	return contentResp.Body, nil
}

// listHubs lists all skills spaces
func (p *SkillProvider) listSpaces(ctx stdctx.Context, opts *ListOptions) (*Result, error) {
	resp, err := p.httpClient.Request("GET", "/skills/spaces", true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list hubs: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Spaces []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"spaces"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse hubs response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("failed to list hubs: %s", result.Msg)
	}

	nodes := make([]*Node, 0, len(result.Data.Spaces))
	for _, space := range result.Data.Spaces {
		nodes = append(nodes, &Node{
			Name: space.Name,
			Type: NodeTypeDirectory,
			Path: fmt.Sprintf("skills/%s", space.Name),
			Metadata: map[string]interface{}{
				"id":          space.ID,
				"description": space.Description,
			},
		})
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// listSkillsInSpace lists skills in a specific space
// First tries search API (supports pagination & sorting), falls back to file system if search returns empty
func (p *SkillProvider) listSkillsInSpace(ctx stdctx.Context, spaceName string, opts *ListOptions) (*Result, error) {
	// Get space UUID for search API
	spaceUUID, err := p.getSpaceUUIDByName(ctx, spaceName)
	if err != nil {
		return nil, fmt.Errorf("space '%s' not found: %w", spaceName, err)
	}

	// Set default limit to 10 if not specified
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Try search API first (supports pagination, sorting, and large collections)
	payload := map[string]interface{}{
		"query":      "", // Empty query = list all (match_all)
		"space_id":   spaceUUID,
		"page":       1,
		"page_size":  limit,
		"sort_by":    opts.SortBy,
		"sort_order": opts.SortOrder,
	}

	logger.Debug("Listing skills via search API", zap.String("space", spaceName), zap.String("spaceUUID", spaceUUID), zap.Int("limit", limit))

	resp, err := p.httpClient.Request("POST", "/skills/search", true, "auto", nil, payload)
	if err == nil {
		var result struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				Skills []struct {
					SkillID     string   `json:"skill_id"`
					Name        string   `json:"name"`
					Description string   `json:"description"`
					Tags        []string `json:"tags"`
					Score       float64  `json:"score"`
					CreateTime  int64    `json:"create_time,omitempty"`
					UpdateTime  int64    `json:"update_time,omitempty"`
				} `json:"skills"`
				Total int64 `json:"total"`
			} `json:"data"`
		}

		if err := json.Unmarshal(resp.Body, &result); err == nil && result.Code == 0 {
			logger.Debug("Search API response", zap.Int("skills_count", len(result.Data.Skills)), zap.Int64("total", result.Data.Total))
			// If search returned results, use them
			if len(result.Data.Skills) > 0 {
				nodes := make([]*Node, 0, len(result.Data.Skills))
				for _, skill := range result.Data.Skills {
					updatedAt := time.UnixMilli(skill.UpdateTime)
					if skill.UpdateTime == 0 {
						updatedAt = time.UnixMilli(skill.CreateTime)
					}
					nodes = append(nodes, &Node{
						Name:      skill.Name,
						Type:      NodeTypeDirectory,
						Path:      fmt.Sprintf("skills/%s/%s", spaceName, skill.Name),
						UpdatedAt: updatedAt,
						Metadata: map[string]interface{}{
							"id":          skill.SkillID,
							"tags":        skill.Tags,
							"score":       skill.Score,
							"description": skill.Description,
						},
					})
				}
				logger.Info("Listed skills via SEARCH", zap.String("space", spaceName), zap.Int("count", len(nodes)), zap.Int64("total", result.Data.Total))
				return &Result{
					Nodes:      nodes,
					Total:      int(result.Data.Total),
					HasMore:    int(result.Data.Total) > limit,
					NextOffset: limit,
				}, nil
			}
			// Search returned empty result, fall through to file system
			logger.Debug("Search returned empty result, falling back to file system")
		} else {
			logger.Debug("Search API error", zap.Error(err), zap.Int("code", result.Code), zap.String("msg", result.Msg))
		}
	} else {
		logger.Debug("Search request failed", zap.Error(err))
	}

	// Fall back to file system listing (for skills not yet indexed)
	logger.Info("Listing skills via FILE SYSTEM (search unavailable)", zap.String("space", spaceName))
	return p.listSkillsInSpaceFromFileSystem(ctx, spaceName, opts)
}

// listSkillsInSpaceFromFileSystem lists skills from file system (fallback when search returns empty)
func (p *SkillProvider) listSkillsInSpaceFromFileSystem(ctx stdctx.Context, spaceName string, opts *ListOptions) (*Result, error) {
	// Get the skills space folder ID from file system
	skillsFolderID, err := p.getSkillsFolderID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get skills folder: %w", err)
	}
	logger.Debug("Got skills folder ID", zap.String("skillsFolderID", skillsFolderID))

	// Find the space folder
	spaceFolderID, err := p.findFolderID(ctx, skillsFolderID, spaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to find space folder: %w", err)
	}
	logger.Debug("Got space folder ID", zap.String("spaceName", spaceName), zap.String("spaceFolderID", spaceFolderID))

	// List all subfolders in the space folder (each subfolder is a skill)
	skillsResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", spaceFolderID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}

	var skillsResult struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Type       string `json:"type"`
				UpdateTime int64  `json:"update_time"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(skillsResp.Body, &skillsResult); err != nil {
		return nil, fmt.Errorf("failed to parse skills response: %w", err)
	}

	if skillsResult.Code != 0 {
		return nil, fmt.Errorf("failed to list skills: %s", skillsResult.Msg)
	}
	logger.Debug("File system list response", zap.Int("files_count", len(skillsResult.Data.Files)))

	// Convert folders to nodes
	nodes := make([]*Node, 0)
	for _, file := range skillsResult.Data.Files {
		// Only include folders (skill directories)
		if file.Type == "folder" {
			nodes = append(nodes, &Node{
				Name:      file.Name,
				Type:      NodeTypeDirectory,
				Path:      fmt.Sprintf("skills/%s/%s", spaceName, file.Name),
				UpdatedAt: time.UnixMilli(file.UpdateTime),
				Metadata: map[string]interface{}{
					"id": file.ID,
				},
			})
		}
	}

	// Apply limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	total := len(nodes)
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	logger.Info("Listed skills via FILE SYSTEM", zap.String("space", spaceName), zap.Int("count", len(nodes)), zap.Int("total", total))

	return &Result{
		Nodes:      nodes,
		Total:      total,
		HasMore:    total > limit,
		NextOffset: limit,
	}, nil
}

// getSkillsFolderID gets the ID of the 'skills' folder
func (p *SkillProvider) getSkillsFolderID(ctx stdctx.Context) (string, error) {
	resp, err := p.httpClient.Request("GET", "/files", true, "auto", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list root folders: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("failed to list folders: %s", result.Msg)
	}

	for _, file := range result.Data.Files {
		if file.Name == "skills" && file.Type == "folder" {
			return file.ID, nil
		}
	}

	return "", fmt.Errorf("skills folder not found")
}

// findFolderID finds a folder by name under a parent folder
func (p *SkillProvider) findFolderID(ctx stdctx.Context, parentID, folderName string) (string, error) {
	resp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", parentID), true, "auto", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list folders: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("failed to list folders: %s", result.Msg)
	}

	for _, file := range result.Data.Files {
		if file.Name == folderName && file.Type == "folder" {
			return file.ID, nil
		}
	}

	return "", fmt.Errorf("folder '%s' not found", folderName)
}

// getSkillFolderID gets the folder ID of a skill in a space.
// First tries the search API (which may have cached folder_id from indexing),
// then falls back to direct file system traversal.
func (p *SkillProvider) getSkillFolderID(ctx stdctx.Context, spaceID, skillName string) (string, error) {
	// Try search API first
	spaceUUID, err := p.getSpaceUUIDByName(ctx, spaceID)
	if err == nil {
		payload := map[string]interface{}{
			"query":     skillName,
			"space_id":  spaceUUID,
			"page":      1,
			"page_size": 10,
		}
		resp, err := p.httpClient.Request("POST", "/skills/search", true, "auto", nil, payload)
		if err == nil {
			var searchResult struct {
				Code int    `json:"code"`
				Msg  string `json:"message"`
				Data struct {
					Skills []struct {
						SkillID  string `json:"skill_id"`
						FolderID string `json:"folder_id"`
						Name     string `json:"name"`
					} `json:"skills"`
				} `json:"data"`
			}
			if err := json.Unmarshal(resp.Body, &searchResult); err == nil && searchResult.Code == 0 {
				for _, skill := range searchResult.Data.Skills {
					if skill.Name == skillName {
						return skill.FolderID, nil
					}
				}
			}
		}
	}

	// Fallback: traverse file system directly
	skillsFolderID, err := p.getSkillsFolderID(ctx)
	if err != nil {
		return "", err
	}
	spaceFolderID, err := p.findFolderID(ctx, skillsFolderID, spaceID)
	if err != nil {
		return "", err
	}
	return p.findFolderID(ctx, spaceFolderID, skillName)
}

// listSkillVersions lists versions of a skill
func (p *SkillProvider) listSkillVersions(ctx stdctx.Context, spaceID, skillName string, opts *ListOptions) (*Result, error) {
	skillFolderID, err := p.getSkillFolderID(ctx, spaceID, skillName)
	if err != nil {
		return nil, fmt.Errorf("skill '%s' not found in space '%s'", skillName, spaceID)
	}

	// List the skill folder to get versions (subdirectories)
	filesResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", skillFolderID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	var filesResult struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Type       string `json:"type"`
				UpdateTime int64  `json:"update_time"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(filesResp.Body, &filesResult); err != nil {
		return nil, fmt.Errorf("failed to parse files response: %w", err)
	}

	if filesResult.Code != 0 {
		return nil, fmt.Errorf("failed to list files: %s", filesResult.Msg)
	}

	// Convert version folders to nodes
	nodes := make([]*Node, 0)
	for _, file := range filesResult.Data.Files {
		// Only include folders (version directories)
		if file.Type == "folder" {
			nodes = append(nodes, &Node{
				Name:      file.Name,
				Type:      NodeTypeDirectory,
				Path:      fmt.Sprintf("skills/%s/%s/%s", spaceID, skillName, file.Name),
				UpdatedAt: time.UnixMilli(file.UpdateTime),
				Metadata: map[string]interface{}{
					"id": file.ID,
				},
			})
		}
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// listSkillContent lists content of a specific skill version
func (p *SkillProvider) listSkillContent(ctx stdctx.Context, spaceID, skillName, version string, extraParts []string, opts *ListOptions) (*Result, error) {
	// Skill content is stored in file system under skills/{space}/{skill}/{version}/
	// We need to traverse the file system to find the skill folder and list its contents

	// Get the skill folder ID (search API or file system fallback)
	skillFolderID, err := p.getSkillFolderID(ctx, spaceID, skillName)
	if err != nil {
		return nil, fmt.Errorf("skill '%s' not found in space '%s'", skillName, spaceID)
	}

	// List the version folder under the skill folder
	filesResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", skillFolderID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list skill versions: %w", err)
	}

	var filesResult struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Type       string `json:"type"`
				Size       int64  `json:"size"`
				UpdateTime int64  `json:"update_time"`
			} `json:"files"`
		} `json:"data"`
	}

	if err := json.Unmarshal(filesResp.Body, &filesResult); err != nil {
		return nil, fmt.Errorf("failed to parse files response: %w", err)
	}

	if filesResult.Code != 0 {
		return nil, fmt.Errorf("failed to list files: %s", filesResult.Msg)
	}

	// Find the version folder
	var versionFolderID string
	for _, file := range filesResult.Data.Files {
		if file.Name == version && file.Type == "folder" {
			versionFolderID = file.ID
			break
		}
	}

	if versionFolderID == "" {
		return nil, fmt.Errorf("version '%s' not found for skill '%s'", version, skillName)
	}

	// Step 4: If there are extra parts, navigate deeper
	currentFolderID := versionFolderID
	currentPath := fmt.Sprintf("skills/%s/%s/%s", spaceID, skillName, version)

	// Check if the last part is a file (for ls on a specific file)
	var lastFile *struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		Size       int64  `json:"size"`
		UpdateTime int64  `json:"update_time"`
	}

	for i, part := range extraParts {
		isLastPart := (i == len(extraParts)-1)

		// List current folder to find the next part
		subResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", currentFolderID), true, "auto", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to navigate path: %w", err)
		}

		var subResult struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				Files []struct {
					ID         string `json:"id"`
					Name       string `json:"name"`
					Type       string `json:"type"`
					Size       int64  `json:"size"`
					UpdateTime int64  `json:"update_time"`
				} `json:"files"`
			} `json:"data"`
		}

		if err := json.Unmarshal(subResp.Body, &subResult); err != nil {
			return nil, fmt.Errorf("failed to parse navigation response: %w", err)
		}

		if subResult.Code != 0 {
			return nil, fmt.Errorf("navigation failed: %s", subResult.Msg)
		}

		found := false
		for _, file := range subResult.Data.Files {
			if file.Name == part {
				if file.Type != "folder" {
					// This is a file
					if isLastPart {
						// If it's the last part, remember the file for listing
						lastFile = &file
						found = true
						break
					}
					// Not the last part - cannot navigate into a file
					return nil, fmt.Errorf("'%s' is not a directory", part)
				}
				currentFolderID = file.ID
				currentPath = currentPath + "/" + part
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("path not found: %s", part)
		}

		// If we found a file as the last part, return it
		if lastFile != nil {
			return &Result{
				Nodes: []*Node{{
					Name: lastFile.Name,
					Type: NodeTypeFile,
					Path: currentPath + "/" + lastFile.Name,
					Metadata: map[string]interface{}{
						"id":          lastFile.ID,
						"size":        lastFile.Size,
						"update_time": lastFile.UpdateTime,
					},
				}},
				Total: 1,
			}, nil
		}
	}

	// Step 5: List the final folder contents
	finalResp, err := p.httpClient.Request("GET", fmt.Sprintf("/files?parent_id=%s", currentFolderID), true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list folder contents: %w", err)
	}

	var finalResult struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Files []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Type       string `json:"type"`
				Size       int64  `json:"size"`
				UpdateTime int64  `json:"update_time"`
			} `json:"files"`
			Total int `json:"total"`
		} `json:"data"`
	}

	if err := json.Unmarshal(finalResp.Body, &finalResult); err != nil {
		return nil, fmt.Errorf("failed to parse final response: %w", err)
	}

	if finalResult.Code != 0 {
		return nil, fmt.Errorf("failed to list contents: %s", finalResult.Msg)
	}

	// Convert to nodes
	nodes := make([]*Node, 0, len(finalResult.Data.Files))
	for _, file := range finalResult.Data.Files {
		nodeType := NodeTypeFile
		if file.Type == "folder" {
			nodeType = NodeTypeDirectory
		}

		nodes = append(nodes, &Node{
			Name: file.Name,
			Type: nodeType,
			Path: currentPath + "/" + file.Name,
			Size: file.Size,
			UpdatedAt: time.UnixMilli(file.UpdateTime),
			Metadata: map[string]interface{}{
				"id": file.ID,
			},
		})
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// getSpaceUUIDByName gets space UUID by its name
func (p *SkillProvider) getSpaceUUIDByName(ctx stdctx.Context, spaceName string) (string, error) {
	resp, err := p.httpClient.Request("GET", "/skills/spaces", true, "auto", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list hubs: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Spaces []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"spaces"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("failed to parse hubs response: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("failed to list hubs: %s", result.Msg)
	}

	for _, space := range result.Data.Spaces {
		if space.Name == spaceName {
			return space.ID, nil
		}
	}

	return "", fmt.Errorf("space with name '%s' not found", spaceName)
}

// DeleteSkill deletes a skill and its index
func (p *SkillProvider) DeleteSkill(ctx stdctx.Context, spaceID, skillName string) error {
	// Get space UUID
	spaceUUID, err := p.getSpaceUUIDByName(ctx, spaceID)
	if err != nil {
		return err
	}

	// Call delete skill index API
	// API format: DELETE /skills/index?skill_id={skill_name}&space_id={space_id}
	resp, err := p.httpClient.Request("DELETE",
		fmt.Sprintf("/skills/index?skill_id=%s&space_id=%s",
			url.QueryEscape(skillName),
			url.QueryEscape(spaceUUID)),
		true, "auto", nil, nil)
	if err != nil {
		return fmt.Errorf("delete index request failed: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		if result.Msg != "" {
			return fmt.Errorf("delete failed: %s", result.Msg)
		}
		return fmt.Errorf("delete failed with code: %d", result.Code)
	}

	return nil
}

// IndexSkill indexes a skill for search
func (p *SkillProvider) IndexSkill(ctx stdctx.Context, spaceID string, skillInfo map[string]interface{}) error {
	// Get space UUID
	spaceUUID, err := p.getSpaceUUIDByName(ctx, spaceID)
	if err != nil {
		return err
	}

	// Get default embedding model
	embdID, _ := p.getDefaultEmbdID(ctx, spaceUUID)

	// Build index request
	payload := map[string]interface{}{
		"skills":  []interface{}{skillInfo},
		"space_id": spaceUUID,
		"embd_id": embdID,
	}

	// Call index API
	resp, err := p.httpClient.Request("POST", "/skills/index", true, "auto", nil, payload)
	if err != nil {
		return fmt.Errorf("index request failed: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			IndexedCount int `json:"indexed_count"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return fmt.Errorf("failed to parse index response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("index failed: %s", result.Msg)
	}

	return nil
}

// getDefaultEmbdID gets the default embedding model ID from skill search config
func (p *SkillProvider) getDefaultEmbdID(ctx stdctx.Context, spaceID string) (string, error) {
	resp, err := p.httpClient.Request("GET",
		fmt.Sprintf("/skills/config?embd_id=&space_id=%s", url.QueryEscape(spaceID)),
		true, "web", nil, nil)
	if err != nil {
		return "", nil
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			EmbdID string `json:"embd_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", nil
	}

	if result.Code != 0 {
		return "", nil
	}

	return result.Data.EmbdID, nil
}

// ============================================================================
// Skill Upload Functions
// ============================================================================

// UploadSkill uploads a skill directory to the server
// nameOverride: user-specified skill name (overrides SKILL.md metadata)
func (p *SkillProvider) UploadSkill(ctx stdctx.Context, skillPath string, versionOverride string, spaceID string, fileProvider Provider, nameOverride string) error {
	spaceID = normalizeSpaceID(spaceID)

	// 1. Validate the skill directory
	result, files, err := ValidateSkillDirectory(skillPath, versionOverride, nameOverride)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	if !result.Valid {
		return fmt.Errorf("validation failed: %s", GetValidationErrorMessage(result))
	}

	// Get skill name from validation result (SKILL.md metadata or user-specified)
	// Fallback to directory name if not specified
	skillName := result.Name
	if skillName == "" {
		skillName = filepath.Base(skillPath)
		skillName = normalizeSkillName(skillName)
	}

	// Use provided version or default
	version := result.Version
	if version == "" {
		version = "1.0.0"
	}

	// 2. Ensure skills space exists
	spaceFolderID, err := p.ensureSkillsSpaceFolder(ctx, spaceID, fileProvider)
	if err != nil {
		return fmt.Errorf("failed to ensure skills space: %w", err)
	}

	// 3. Get or create skill folder
	skillFolderID, err := p.getOrCreateSkillFolder(ctx, spaceID, spaceFolderID, skillName, fileProvider)
	if err != nil {
		return err
	}

	// 4. Check if version already exists
	exists, err := p.versionExists(ctx, spaceID, skillName, version, fileProvider)
	if err != nil {
		return fmt.Errorf("failed to check version: %w", err)
	}
	if exists {
		return &SkillConflictError{Type: "version", Name: skillName, Version: version}
	}

	// 5. Create version folder
	versionFolderID, err := p.createFolder(ctx, skillFolderID, version)
	if err != nil {
		return fmt.Errorf("failed to create version folder: %w", err)
	}

	// 6. Upload all files
	for _, file := range files {
		sanitized := sanitizeRelPath(file.Path)
		if sanitized == "" || isMacJunkPath(sanitized) || shouldIgnore(sanitized, defaultIgnorePatterns) {
			continue
		}

		err = p.uploadFile(ctx, file, versionFolderID)
		if err != nil {
			return fmt.Errorf("failed to upload file %s: %w", file.Path, err)
		}
	}

	// 7. Index the skill for search
	if err := p.indexSkillFromUpload(ctx, result, files, spaceID, skillFolderID); err != nil {
		return fmt.Errorf("failed to index skill: %w", err)
	}

	return nil
}

// ensureSkillsSpaceFolder ensures the 'skills/<space>' folder exists
func (p *SkillProvider) ensureSkillsSpaceFolder(ctx stdctx.Context, spaceID string, fileProvider Provider) (string, error) {
	skillsFolderID, err := p.ensureSkillsFolder(ctx, fileProvider)
	if err != nil {
		return "", err
	}

	result, err := fileProvider.List(ctx, "skills", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == spaceID {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return p.createFolder(ctx, skillsFolderID, spaceID)
}

// ensureSkillsFolder ensures the 'skills' folder exists
func (p *SkillProvider) ensureSkillsFolder(ctx stdctx.Context, fileProvider Provider) (string, error) {
	result, err := fileProvider.List(ctx, "", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == "skills" {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return p.createFolder(ctx, "", "skills")
}

// getOrCreateSkillFolder gets existing skill folder or creates new one
func (p *SkillProvider) getOrCreateSkillFolder(ctx stdctx.Context, spaceID, parentID, skillName string, fileProvider Provider) (string, error) {
	result, err := fileProvider.List(ctx, fmt.Sprintf("skills/%s", spaceID), nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == skillName {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return p.createFolder(ctx, parentID, skillName)
}

// versionExists checks if a version already exists
func (p *SkillProvider) versionExists(ctx stdctx.Context, spaceID, skillName, version string, fileProvider Provider) (bool, error) {
	result, err := fileProvider.List(ctx, fmt.Sprintf("skills/%s/%s", spaceID, skillName), nil)
	if err != nil {
		return false, err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == version {
			return true, nil
		}
	}
	return false, nil
}

// createFolder creates a new folder and returns its ID
func (p *SkillProvider) createFolder(ctx stdctx.Context, parentID, name string) (string, error) {
	payload := map[string]interface{}{
		"name": name,
		"type": "folder",
	}
	if parentID != "" {
		payload["parent_id"] = parentID
	}

	resp, err := p.httpClient.Request("POST", "/files", true, "auto", nil, payload)
	if err != nil {
		return "", err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("server returned error code: %d", result.Code)
	}

	return result.Data.ID, nil
}

// uploadFile uploads a single file using multipart form
func (p *SkillProvider) uploadFile(ctx stdctx.Context, file *SkillFile, parentID string) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if parentID != "" {
		writer.WriteField("parent_id", parentID)
	}

	part, err := writer.CreateFormFile("file", file.Path)
	if err != nil {
		return err
	}
	if _, err := part.Write(file.Content); err != nil {
		return err
	}
	writer.Close()

	return p.httpClient.UploadMultipart("/files", writer.FormDataContentType(), &buf)
}

// indexSkillFromUpload indexes the skill after upload
func (p *SkillProvider) indexSkillFromUpload(ctx stdctx.Context, result *SkillValidationResult, files []*SkillFile, spaceID string, skillFolderID string) error {
	var contentBuilder strings.Builder
	for _, file := range files {
		if !isTextFile(file.Path, "") {
			continue
		}
		if len(file.Content) > MaxSkillFileSize {
			continue
		}
		sanitized := sanitizeRelPath(file.Path)
		if sanitized == "" || isMacJunkPath(sanitized) || shouldIgnore(sanitized, defaultIgnorePatterns) {
			continue
		}
		contentBuilder.WriteString(fmt.Sprintf("\n=== %s ===\n", file.Path))
		contentBuilder.Write(file.Content)
	}
	content := contentBuilder.String()

	// Use skill name as ID (without version suffix)
	// This ensures all versions of the same skill share the same index document
	skillID := result.Name

	skillInfo := map[string]interface{}{
		"id":          skillID,
		"folder_id":   skillFolderID,
		"name":        result.Name,
		"description": result.Description,
		"tags":        result.Tags,
		"content":     content,
		"version":     result.Version,
	}

	return p.IndexSkill(ctx, spaceID, skillInfo)
}

// ============================================================================
// Validation Functions
// ============================================================================

// ValidateSkillDirectory validates a skill directory
// nameOverride: user-specified skill name (overrides SKILL.md metadata)
func ValidateSkillDirectory(skillPath string, versionOverride string, nameOverride string) (*SkillValidationResult, []*SkillFile, error) {
	info, err := os.Stat(skillPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot access directory %s: %w", skillPath, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", skillPath)
	}

	files, err := readSkillFiles(skillPath)
	if err != nil {
		return nil, nil, err
	}

	if len(files) == 0 {
		return &SkillValidationResult{Valid: false, Error: "no_files"}, nil, nil
	}

	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}
	if totalSize > MaxSkillTotalSize {
		return &SkillValidationResult{Valid: false, Error: "total_size_exceeded"}, nil, nil
	}

	var validFiles []*SkillFile
	for _, f := range files {
		if f.Size > MaxSkillFileSize {
			return &SkillValidationResult{
				Valid:   false,
				Error:   "file_too_large",
				Details: f.Path,
			}, nil, nil
		}

		sanitized := sanitizeRelPath(f.Path)
		if sanitized == "" {
			return &SkillValidationResult{Valid: false, Error: "invalid_path"}, nil, nil
		}

		if isMacJunkPath(sanitized) || shouldIgnore(sanitized, defaultIgnorePatterns) {
			continue
		}

		validFiles = append(validFiles, f)
	}

	if len(validFiles) == 0 {
		return &SkillValidationResult{Valid: false, Error: "no_valid_files"}, nil, nil
	}

	var skillMdFile *SkillFile
	for _, f := range validFiles {
		normalized := strings.ToLower(f.Path)
		if normalized == "skill.md" || strings.HasSuffix(normalized, "/skill.md") {
			skillMdFile = f
			break
		}
	}

	if skillMdFile == nil {
		return &SkillValidationResult{Valid: false, Error: "missing_skill_md"}, nil, nil
	}

	metadata, err := parseFrontmatter(string(skillMdFile.Content))
	if err != nil {
		return &SkillValidationResult{
			Valid:   false,
			Error:   "invalid_frontmatter",
			Details: err.Error(),
		}, nil, nil
	}

	if metadata.Name == "" {
		return &SkillValidationResult{Valid: false, Error: "missing_name"}, nil, nil
	}

	if !isValidSkillName(metadata.Name) {
		return &SkillValidationResult{
			Valid:   false,
			Error:   "invalid_name_format",
			Details: metadata.Name,
		}, nil, nil
	}

	version := versionOverride
	if version == "" {
		version = metadata.Version
	}
	// Set default version if not provided
	if version == "" {
		version = "1.0.0"
	}

	if !isValidSemver(version) {
		return &SkillValidationResult{
			Valid:   false,
			Error:   "invalid_version",
			Details: version,
		}, nil, nil
	}

	for _, f := range validFiles {
		if !isTextFile(f.Path, "") {
			return &SkillValidationResult{
				Valid:   false,
				Error:   "invalid_file_type",
				Details: f.Path,
			}, nil, nil
		}
	}

	// Use user-specified name if provided, otherwise use metadata.Name from SKILL.md
	skillName := metadata.Name
	if nameOverride != "" {
		skillName = nameOverride
	}

	return &SkillValidationResult{
		Valid:       true,
		Name:        skillName,
		Description: metadata.Description,
		Version:     version,
		Tags:        metadata.Tags,
	}, validFiles, nil
}

// readSkillFiles recursively reads all files in the skill directory
func readSkillFiles(skillPath string) ([]*SkillFile, error) {
	var files []*SkillFile

	err := filepath.Walk(skillPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			relPath, err := filepath.Rel(skillPath, path)
			if err != nil {
				return err
			}

			relPath = filepath.ToSlash(relPath)

			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			files = append(files, &SkillFile{
				Path:    relPath,
				Content: content,
				Size:    info.Size(),
			})
		}

		return nil
	})

	return files, err
}

// parseFrontmatter extracts YAML frontmatter from markdown content
func parseFrontmatter(content string) (*SkillMetadata, error) {
	lines := strings.Split(content, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing frontmatter start")
	}

	var endIndex int
	found := false
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIndex = i
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("missing frontmatter end")
	}

	frontmatter := strings.Join(lines[1:endIndex], "\n")
	var metadata SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return &metadata, nil
}

// isValidSkillName checks if skill name follows slug format
func isValidSkillName(name string) bool {
	matched, _ := regexp.MatchString(`^[a-z0-9][a-z0-9_-]*$`, name)
	return matched
}

// isValidSemver checks basic semver format
func isValidSemver(version string) bool {
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+`, version)
	return matched
}

// isTextFile checks if file is text-based
func isTextFile(filePath, contentType string) bool {
	if contentType != "" {
		normalized := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		if strings.HasPrefix(normalized, "text/") {
			return true
		}
		textContentTypes := map[string]bool{
			"application/json": true, "application/xml": true, "application/yaml": true,
			"application/x-yaml": true, "application/toml": true, "application/javascript": true,
			"application/typescript": true, "application/markdown": true, "image/svg+xml": true,
		}
		if textContentTypes[normalized] {
			return true
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != "" {
		ext = ext[1:]
	}
	return textFileExtensions[ext]
}

// sanitizeRelPath sanitizes relative path
func sanitizeRelPath(path string) string {
	normalized := regexp.MustCompile(`^\./+`).ReplaceAllString(path, "")
	normalized = strings.TrimLeft(normalized, "/")

	if normalized == "" || strings.HasSuffix(normalized, "/") {
		return ""
	}
	if strings.Contains(normalized, "..") || strings.Contains(normalized, "\\") {
		return ""
	}
	return normalized
}

// isMacJunkPath checks if path is Mac junk file
func isMacJunkPath(path string) bool {
	normalized := strings.ToLower(path)
	if normalized == ".ds_store" || strings.HasSuffix(normalized, "/.ds_store") {
		return true
	}
	if strings.HasPrefix(normalized, "__macosx/") || normalized == "__macosx" {
		return true
	}
	if strings.HasPrefix(normalized, "._") || strings.Contains(normalized, "/._") {
		return true
	}
	return false
}

// shouldIgnore checks if path should be ignored
func shouldIgnore(filePath string, patterns []string) bool {
	normalizedPath := strings.ToLower(filePath)
	for _, pattern := range patterns {
		trimmedPattern := strings.TrimSpace(pattern)
		if trimmedPattern == "" || strings.HasPrefix(trimmedPattern, "#") {
			continue
		}
		if matchPattern(normalizedPath, strings.ToLower(trimmedPattern)) {
			return true
		}
	}
	return false
}

// matchPattern matches path against ignore pattern
func matchPattern(filePath, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(filePath, dirPattern+"/") || filePath == dirPattern
	}

	if filePath == pattern {
		return true
	}

	regex := globToRegex(pattern)
	matched, _ := regexp.MatchString(regex, filePath)
	return matched
}

// globToRegex converts glob pattern to regex
func globToRegex(pattern string) string {
	var regex strings.Builder
	regex.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]

		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				regex.WriteString(".*")
				i++
			} else {
				regex.WriteString("[^/]*")
			}
		case '?':
			regex.WriteString("[^/]")
		case '.':
			regex.WriteString("\\.")
		case '\\', '/', '$', '^', '+', '(', ')', '[', ']', '{', '}':
			regex.WriteString("\\")
			regex.WriteByte(c)
		default:
			regex.WriteByte(c)
		}
	}

	regex.WriteString("$")
	return regex.String()
}

// normalizeSpaceID normalizes space ID
func normalizeSpaceID(spaceID string) string {
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return DefaultSpaceID
	}
	return spaceID
}

// normalizeSkillName normalizes skill name
func normalizeSkillName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	name = re.ReplaceAllString(name, "-")
	re = regexp.MustCompile(`-+`)
	name = re.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	return name
}

// GetValidationErrorMessage returns human-readable error message
func GetValidationErrorMessage(result *SkillValidationResult) string {
	switch result.Error {
	case "no_files":
		return "No files found in the skill directory"
	case "total_size_exceeded":
		return fmt.Sprintf("Total size exceeds limit of %d MB", MaxSkillTotalSize/(1024*1024))
	case "file_too_large":
		return fmt.Sprintf("File too large: %s (max %d MB per file)", result.Details, MaxSkillFileSize/(1024*1024))
	case "invalid_path":
		return "Invalid file path detected"
	case "missing_skill_md":
		return "SKILL.md not found in the skill directory"
	case "invalid_frontmatter":
		if result.Details != "" {
			return fmt.Sprintf("Invalid SKILL.md frontmatter: %s", result.Details)
		}
		return "Invalid SKILL.md frontmatter format"
	case "missing_name":
		return "SKILL.md missing required field: name"
	case "invalid_name_format":
		return fmt.Sprintf("Invalid skill name format: %s (must be lowercase, alphanumeric with hyphens/underscores)", result.Details)
	case "invalid_version":
		return fmt.Sprintf("Invalid version format: %s (must be semver like 1.0.0)", result.Details)
	case "invalid_file_type":
		return fmt.Sprintf("Invalid file type: %s (only text files allowed)", result.Details)
	case "no_valid_files":
		return "No valid files found after filtering"
	default:
		return fmt.Sprintf("Validation failed: %s", result.Error)
	}
}

// GetString safely extracts a string value from interface{}
func GetString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ============================================================================
// Skill Uploader
// ============================================================================

// SkillUploader handles uploading skills to the server
type SkillUploader struct {
	client        HTTPClientInterface
	fileProvider  *FileProvider
	skillProvider Provider
	force         bool // Force mode: overwrite existing versions
}

// NewSkillUploader creates a new uploader
func NewSkillUploader(client HTTPClientInterface, fileProvider *FileProvider) *SkillUploader {
	return &SkillUploader{
		client:       client,
		fileProvider: fileProvider,
	}
}

// SetSkillProvider sets the skill provider
func (u *SkillUploader) SetSkillProvider(provider Provider) {
	u.skillProvider = provider
}

// SetForce sets the force mode (overwrite existing versions)
func (u *SkillUploader) SetForce(force bool) {
	u.force = force
}

// parseSpaceFromPath extracts space ID from a path like "skills/space1" or "skills"
// Returns "default" for "skills" (no space specified)
func parseSpaceFromPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "skills" {
		return DefaultSpaceID
	}
	// Handle paths like "skills/space1" or "hub1"
	if strings.HasPrefix(path, "skills/") {
		path = strings.TrimPrefix(path, "skills/")
	}
	if path == "" {
		return DefaultSpaceID
	}
	return normalizeSpaceID(path)
}

// UploadSkill uploads a skill directory to the server
// nameOverride: user-specified skill name (overrides SKILL.md metadata)
func (u *SkillUploader) UploadSkill(ctx stdctx.Context, skillPath string, versionOverride string, hubPath string, nameOverride string) error {
	// Parse space from path
	spaceID := parseSpaceFromPath(hubPath)

	// 1. Validate the skill directory
	fmt.Printf("Validating skill at %s...\n", skillPath)
	result, files, err := ValidateSkillDirectory(skillPath, versionOverride, nameOverride)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	if !result.Valid {
		return fmt.Errorf("validation failed: %s", GetValidationErrorMessage(result))
	}

	// Get skill name from validation result (SKILL.md metadata or user-specified)
	// Fallback to directory name if not specified
	skillName := result.Name
	if skillName == "" {
		skillName = filepath.Base(skillPath)
		skillName = normalizeSkillName(skillName)
	}

	// Use provided version or default
	version := result.Version
	if version == "" {
		version = "1.0.0"
	}

	fmt.Printf("✓ Skill '%s' (v%s) is valid\n", skillName, version)

	// 2. Ensure skills space exists
	fmt.Printf("Checking skills space '%s'...\n", spaceID)
	spaceFolderID, err := u.ensureSkillsSpaceFolder(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to ensure skills space: %w", err)
	}

	// 3. Get or create skill folder
	fmt.Printf("Checking skill '%s'...\n", skillName)
	skillFolderID, err := u.getOrCreateSkillFolder(ctx, spaceID, spaceFolderID, skillName)
	if err != nil {
		return err
	}

	// 4. Check if version already exists
	fmt.Printf("Checking version '%s'...\n", version)
	exists, err := u.versionExists(ctx, spaceID, skillName, version)
	if err != nil {
		return fmt.Errorf("failed to check version: %w", err)
	}
	if exists {
		if u.force {
			// Force mode: delete existing version folder
			fmt.Printf("Force mode: removing existing version '%s'...\n", version)
			versionPath := fmt.Sprintf("skills/%s/%s/%s", spaceID, skillName, version)
			if err := u.deleteVersionFolder(ctx, versionPath); err != nil {
				return fmt.Errorf("failed to remove existing version: %w", err)
			}
			fmt.Printf("✓ Existing version '%s' removed\n", version)
		} else {
			return &SkillConflictError{Type: "version", Name: skillName, Version: version}
		}
	}

	// 5. Create version folder
	fmt.Println("Creating version folder...")
	versionFolderID, err := u.createFolder(ctx, skillFolderID, version)
	if err != nil {
		return fmt.Errorf("failed to create version folder: %w", err)
	}

	// 6. Upload all files
	fmt.Printf("Uploading %d files...\n", len(files))
	for _, file := range files {
		sanitized := sanitizeRelPath(file.Path)
		if sanitized == "" || isMacJunkPath(sanitized) || shouldIgnore(sanitized, defaultIgnorePatterns) {
			continue
		}

		err = u.uploadFile(ctx, file, versionFolderID)
		if err != nil {
			return fmt.Errorf("failed to upload file %s: %w", file.Path, err)
		}
	}

	fmt.Printf("✓ Successfully uploaded skill '%s' version %s\n", skillName, version)

	// 7. Index the skill for search
	fmt.Println("Indexing skill for search...")
	if err := u.indexSkill(ctx, result, files, spaceID, skillFolderID); err != nil {
		fmt.Printf("⚠ Warning: Failed to index skill for search: %v\n", err)
	} else {
		fmt.Println("✓ Skill indexed successfully")
	}

	return nil
}

// ensureSkillsSpaceFolder ensures the 'skills/<space>' folder exists
func (u *SkillUploader) ensureSkillsSpaceFolder(ctx stdctx.Context, spaceID string) (string, error) {
	skillsFolderID, err := u.ensureSkillsFolder(ctx)
	if err != nil {
		return "", err
	}

	result, err := u.fileProvider.List(ctx, "skills", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == spaceID {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return u.createFolder(ctx, skillsFolderID, spaceID)
}

// ensureSkillsFolder ensures the 'skills' folder exists
func (u *SkillUploader) ensureSkillsFolder(ctx stdctx.Context) (string, error) {
	result, err := u.fileProvider.List(ctx, "", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == "skills" {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return u.createFolder(ctx, "", "skills")
}

// getOrCreateSkillFolder gets existing skill folder or creates new one
func (u *SkillUploader) getOrCreateSkillFolder(ctx stdctx.Context, spaceID, parentID, skillName string) (string, error) {
	result, err := u.fileProvider.List(ctx, fmt.Sprintf("skills/%s", spaceID), nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == skillName {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return u.createFolder(ctx, parentID, skillName)
}

// versionExists checks if a version already exists
func (u *SkillUploader) versionExists(ctx stdctx.Context, spaceID, skillName, version string) (bool, error) {
	result, err := u.fileProvider.List(ctx, fmt.Sprintf("skills/%s/%s", spaceID, skillName), nil)
	if err != nil {
		return false, err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == version {
			return true, nil
		}
	}
	return false, nil
}

// deleteVersionFolder deletes a version folder by path
func (u *SkillUploader) deleteVersionFolder(ctx stdctx.Context, versionPath string) error {
	return u.fileProvider.DeleteFolderByPath(ctx, versionPath)
}

// createFolder creates a new folder and returns its ID
func (u *SkillUploader) createFolder(ctx stdctx.Context, parentID, name string) (string, error) {
	payload := map[string]interface{}{
		"name": name,
		"type": "folder",
	}
	if parentID != "" {
		payload["parent_id"] = parentID
	}

	resp, err := u.client.Request("POST", "/files", true, "auto", nil, payload)
	if err != nil {
		return "", err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("server returned error code: %d", result.Code)
	}

	return result.Data.ID, nil
}

// uploadFile uploads a single file using multipart form
func (u *SkillUploader) uploadFile(ctx stdctx.Context, file *SkillFile, parentID string) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if parentID != "" {
		writer.WriteField("parent_id", parentID)
	}

	part, err := writer.CreateFormFile("file", file.Path)
	if err != nil {
		return err
	}
	if _, err := part.Write(file.Content); err != nil {
		return err
	}
	writer.Close()

	return u.client.UploadMultipart("/files", writer.FormDataContentType(), &buf)
}

// indexSkill indexes the skill for search
func (u *SkillUploader) indexSkill(ctx stdctx.Context, result *SkillValidationResult, files []*SkillFile, spaceID, skillFolderID string) error {
	if u.skillProvider == nil {
		return fmt.Errorf("skill provider not available")
	}

	skillProvider, ok := u.skillProvider.(*SkillProvider)
	if !ok {
		return fmt.Errorf("invalid skill provider type")
	}

	var contentBuilder strings.Builder
	for _, file := range files {
		if !isTextFile(file.Path, "") {
			continue
		}
		if len(file.Content) > MaxSkillFileSize {
			continue
		}
		sanitized := sanitizeRelPath(file.Path)
		if sanitized == "" || isMacJunkPath(sanitized) || shouldIgnore(sanitized, defaultIgnorePatterns) {
			continue
		}
		contentBuilder.WriteString(fmt.Sprintf("\n=== %s ===\n", file.Path))
		contentBuilder.Write(file.Content)
	}
	content := contentBuilder.String()

	// Use skill name as ID (without version suffix)
	// This ensures all versions of the same skill share the same index document
	skillID := result.Name

	skillInfo := map[string]interface{}{
		"id":          skillID,
		"folder_id":   skillFolderID,
		"name":        result.Name,
		"description": result.Description,
		"tags":        result.Tags,
		"content":     content,
		"version":     result.Version,
	}

	return skillProvider.IndexSkill(ctx, spaceID, skillInfo)
}
