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

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// SkillProvider handles skill operations using /skills API
// Path structure:
//   - skills/                            -> List all hubs
//   - skills/{hub_id}/                   -> List skills in hub
//   - skills/{hub_id}/{skill_name}/      -> List versions of skill
//   - skills/{hub_id}/{skill_name}/{version}/ -> Get skill version info
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
	DefaultHubID      = "default"
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
// Path structure: skills/ or skills/{hub_id}/ or skills/{hub_id}/{skill_name}/...
func (p *SkillProvider) List(ctx stdctx.Context, subPath string, opts *ListOptions) (*Result, error) {
	if subPath == "" {
		// List all hubs
		return p.listHubs(ctx, opts)
	}

	parts := SplitPath(subPath)
	switch len(parts) {
	case 1:
		// skills/{hub_id} - list skills in hub
		return p.listSkillsInHub(ctx, parts[0], opts)
	case 2:
		// skills/{hub_id}/{skill_name} - list versions of skill
		return p.listSkillVersions(ctx, parts[0], parts[1], opts)
	default:
		// skills/{hub_id}/{skill_name}/{version}/... - skill content
		return p.listSkillContent(ctx, parts[0], parts[1], parts[2], parts[3:], opts)
	}
}

// Search searches for skills matching the query
func (p *SkillProvider) Search(ctx stdctx.Context, subPath string, opts *SearchOptions) (*Result, error) {
	if opts == nil || opts.Query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	// Parse hub from path
	hubID := ""
	parts := SplitPath(subPath)
	if len(parts) > 0 {
		hubID = parts[0]
	}

	// Hub ID can be either a name or UUID
	// If it's not "default" and doesn't look like a UUID, try to convert it
	if hubID != "" && hubID != "default" && !isUUID(hubID) {
		hubUUID, err := p.getHubUUIDByName(ctx, hubID)
		if err == nil {
			hubID = hubUUID
		}
		// If lookup fails, use the original hubID as-is (it might already be a UUID)
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
		"hub_id":     hubID,
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
		nodes = append(nodes, &Node{
			Name: skill.Name,
			Type: NodeTypeDirectory,
			Path: fmt.Sprintf("skills/%s/%s", hubID, skill.Name),
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

// Cat retrieves the content of a skill file at the given path
// Not supported for skills as they are directories, not files
func (p *SkillProvider) Cat(ctx stdctx.Context, path string) ([]byte, error) {
	return nil, fmt.Errorf("cannot cat skill: skills are directories, use 'ls' instead")
}

// listHubs lists all skills hubs
func (p *SkillProvider) listHubs(ctx stdctx.Context, opts *ListOptions) (*Result, error) {
	resp, err := p.httpClient.Request("GET", "/skills/hubs", true, "auto", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list hubs: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Hubs []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"hubs"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse hubs response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("failed to list hubs: %s", result.Msg)
	}

	nodes := make([]*Node, 0, len(result.Data.Hubs))
	for _, hub := range result.Data.Hubs {
		nodes = append(nodes, &Node{
			Name: hub.Name,
			Type: NodeTypeDirectory,
			Path: fmt.Sprintf("skills/%s", hub.Name),
			Metadata: map[string]interface{}{
				"id":          hub.ID,
				"description": hub.Description,
			},
		})
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// listSkillsInHub lists skills in a specific hub
// This is a virtual listing based on folder structure in file system
func (p *SkillProvider) listSkillsInHub(ctx stdctx.Context, hubID string, opts *ListOptions) (*Result, error) {
	// First get the hub UUID
	hubUUID, err := p.getHubUUIDByName(ctx, hubID)
	if err != nil {
		return nil, err
	}

	// Search for all skills in this hub (empty query returns all)
	payload := map[string]interface{}{
		"query":      "",
		"hub_id":     hubUUID,
		"page":       1,
		"page_size":  1000,
	}

	resp, err := p.httpClient.Request("POST", "/skills/search", true, "auto", nil, payload)
	if err != nil {
		// If search fails, return empty list
		return &Result{Nodes: []*Node{}}, nil
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Skills []struct {
				SkillID     string `json:"skill_id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"skills"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return &Result{Nodes: []*Node{}}, nil
	}

	nodes := make([]*Node, 0, len(result.Data.Skills))
	for _, skill := range result.Data.Skills {
		nodes = append(nodes, &Node{
			Name: skill.Name,
			Type: NodeTypeDirectory,
			Path: fmt.Sprintf("skills/%s/%s", hubID, skill.Name),
			Metadata: map[string]interface{}{
				"skill_id":    skill.SkillID,
				"description": skill.Description,
			},
		})
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// listSkillVersions lists versions of a skill
func (p *SkillProvider) listSkillVersions(ctx stdctx.Context, hubID, skillName string, opts *ListOptions) (*Result, error) {
	// Versions are stored as subdirectories in the file system
	// For now, return a placeholder - actual implementation would query file system
	return &Result{
		Nodes: []*Node{
			{
				Name: "latest",
				Type: NodeTypeDirectory,
				Path: fmt.Sprintf("skills/%s/%s/latest", hubID, skillName),
				Metadata: map[string]interface{}{
					"description": "Latest version",
				},
			},
		},
		Total: 1,
	}, nil
}

// listSkillContent lists content of a specific skill version
func (p *SkillProvider) listSkillContent(ctx stdctx.Context, hubID, skillName, version string, extraParts []string, opts *ListOptions) (*Result, error) {
	// Skill content is stored in file system
	// This would need to be integrated with FileProvider
	return &Result{
		Nodes: []*Node{},
		Total: 0,
	}, nil
}

// getHubUUIDByName gets hub UUID by its name
func (p *SkillProvider) getHubUUIDByName(ctx stdctx.Context, hubName string) (string, error) {
	resp, err := p.httpClient.Request("GET", "/skills/hubs", true, "auto", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list hubs: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Hubs []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"hubs"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("failed to parse hubs response: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("failed to list hubs: %s", result.Msg)
	}

	for _, hub := range result.Data.Hubs {
		if hub.Name == hubName {
			return hub.ID, nil
		}
	}

	return "", fmt.Errorf("hub with name '%s' not found", hubName)
}

// DeleteSkill deletes a skill and its index
func (p *SkillProvider) DeleteSkill(ctx stdctx.Context, hubID, skillName string) error {
	// Get hub UUID
	hubUUID, err := p.getHubUUIDByName(ctx, hubID)
	if err != nil {
		return err
	}

	// Call delete skill index API
	resp, err := p.httpClient.Request("DELETE", 
		fmt.Sprintf("/skills/index/%s?hub_id=%s", 
			url.PathEscape(skillName), 
			url.QueryEscape(hubUUID)), 
		true, "web", nil, nil)
	if err != nil {
		return fmt.Errorf("delete index request failed: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("delete failed: %s", result.Msg)
	}

	return nil
}

// IndexSkill indexes a skill for search
func (p *SkillProvider) IndexSkill(ctx stdctx.Context, hubID string, skillInfo map[string]interface{}) error {
	// Get hub UUID
	hubUUID, err := p.getHubUUIDByName(ctx, hubID)
	if err != nil {
		return err
	}

	// Get default embedding model
	embdID, _ := p.getDefaultEmbdID(ctx, hubUUID)

	// Build index request
	payload := map[string]interface{}{
		"skills":  []interface{}{skillInfo},
		"hub_id":  hubUUID,
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
func (p *SkillProvider) getDefaultEmbdID(ctx stdctx.Context, hubID string) (string, error) {
	resp, err := p.httpClient.Request("GET",
		fmt.Sprintf("/skills/config?embd_id=&hub_id=%s", url.QueryEscape(hubID)),
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
func (p *SkillProvider) UploadSkill(ctx stdctx.Context, skillPath string, versionOverride string, hubID string, fileProvider Provider, nameOverride string) error {
	hubID = normalizeHubID(hubID)

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

	// 2. Ensure skills hub exists
	hubFolderID, err := p.ensureSkillsHubFolder(ctx, hubID, fileProvider)
	if err != nil {
		return fmt.Errorf("failed to ensure skills hub: %w", err)
	}

	// 3. Get or create skill folder
	skillFolderID, err := p.getOrCreateSkillFolder(ctx, hubID, hubFolderID, skillName, fileProvider)
	if err != nil {
		return err
	}

	// 4. Check if version already exists
	exists, err := p.versionExists(ctx, hubID, skillName, version, fileProvider)
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
	if err := p.indexSkillFromUpload(ctx, result, files, hubID, skillFolderID); err != nil {
		return fmt.Errorf("failed to index skill: %w", err)
	}

	return nil
}

// ensureSkillsHubFolder ensures the 'skills/<hub>' folder exists
func (p *SkillProvider) ensureSkillsHubFolder(ctx stdctx.Context, hubID string, fileProvider Provider) (string, error) {
	skillsFolderID, err := p.ensureSkillsFolder(ctx, fileProvider)
	if err != nil {
		return "", err
	}

	result, err := fileProvider.List(ctx, "skills", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == hubID {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return p.createFolder(ctx, skillsFolderID, hubID)
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
func (p *SkillProvider) getOrCreateSkillFolder(ctx stdctx.Context, hubID, parentID, skillName string, fileProvider Provider) (string, error) {
	result, err := fileProvider.List(ctx, fmt.Sprintf("skills/%s", hubID), nil)
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
func (p *SkillProvider) versionExists(ctx stdctx.Context, hubID, skillName, version string, fileProvider Provider) (bool, error) {
	result, err := fileProvider.List(ctx, fmt.Sprintf("skills/%s/%s", hubID, skillName), nil)
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
func (p *SkillProvider) indexSkillFromUpload(ctx stdctx.Context, result *SkillValidationResult, files []*SkillFile, hubID string, skillFolderID string) error {
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
	}

	return p.IndexSkill(ctx, hubID, skillInfo)
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

	if version != "" && !isValidSemver(version) {
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

// normalizeHubID normalizes hub ID
func normalizeHubID(hubID string) string {
	hubID = strings.TrimSpace(hubID)
	if hubID == "" {
		return DefaultHubID
	}
	return hubID
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
// AddSkill Command Handler
// ============================================================================

// AddSkillCommand handles the add-skill command
type AddSkillCommand struct {
	client        HTTPClientInterface
	fileProvider  *FileProvider
	skillProvider Provider
}

// NewAddSkillCommand creates a new command handler
func NewAddSkillCommand(client HTTPClientInterface, fileProvider *FileProvider, skillProvider Provider) *AddSkillCommand {
	return &AddSkillCommand{
		client:        client,
		fileProvider:  fileProvider,
		skillProvider: skillProvider,
	}
}

// AddSkillArgs holds the parsed arguments for add-skill command
type AddSkillArgs struct {
	SkillPath string
	Version   string
	SkillName string // User-specified skill name (overrides SKILL.md)
	ShowHelp  bool
}

// ParseAddSkillArgs parses the add-skill command arguments
func ParseAddSkillArgs(args []string) (*AddSkillArgs, error) {
	result := &AddSkillArgs{}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-h", "--help":
			result.ShowHelp = true
			return result, nil
		case "-v", "--version":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.Version = args[i+1]
				i++
			} else {
				return nil, fmt.Errorf("version flag requires a value")
			}
		case "-n", "--name":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.SkillName = args[i+1]
				i++
			} else {
				return nil, fmt.Errorf("name flag requires a value")
			}
		default:
			// Non-flag argument is the skill path
			if !strings.HasPrefix(arg, "-") && result.SkillPath == "" {
				result.SkillPath = arg
			}
		}
	}

	if result.SkillPath == "" && !result.ShowHelp {
		return nil, fmt.Errorf("skill path is required")
	}

	return result, nil
}

// Execute runs the add-skill command
func (c *AddSkillCommand) Execute(args []string) error {
	// Parse arguments
	parsedArgs, err := ParseAddSkillArgs(args)
	if err != nil {
		return err
	}

	if parsedArgs.ShowHelp {
		c.PrintHelp()
		return nil
	}

	// Validate skill path
	skillPath := parsedArgs.SkillPath
	info, err := os.Stat(skillPath)
	if err != nil {
		return fmt.Errorf("cannot access path %s: %w", skillPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", skillPath)
	}

	// Upload skill to default hub
	uploader := NewSkillUploader(c.client, c.fileProvider)
	uploader.SetSkillProvider(c.skillProvider)
	err = uploader.UploadSkill(stdctx.Background(), skillPath, parsedArgs.Version, "", parsedArgs.SkillName)
	if err != nil {
		// Handle version conflict error
		if conflictErr, ok := err.(*SkillConflictError); ok {
			return fmt.Errorf("%s", conflictErr.Error())
		}
		return err
	}

	return nil
}

// PrintHelp prints the help message for add-skill command
func (c *AddSkillCommand) PrintHelp() {
	fmt.Println(`Usage: add-skill <path> [options]

Upload a skill directory to RAGFlow.
The skill is uploaded to the current skills hub (based on your current directory context).

Arguments:
  <path>                 Path to the skill directory

Options:
  -v, --version string   Specify the skill version (default: from SKILL.md or 1.0.0)
  -h, --help             Show this help message

Examples:
  add-skill /path/to/my-skill
  add-skill /path/to/my-skill --version 1.0.0
  add-skill /path/to/my-skill -v 2.0.0

The skill directory must contain a SKILL.md file with required frontmatter:
  ---
  name: my-skill
  description: A brief description
  ---

Validation rules:
  - Total size must not exceed 50MB
  - Individual files must not exceed 5MB
  - Only text files are allowed
  - Skill name must be lowercase alphanumeric with hyphens/underscores`)
}

// ============================================================================
// Delete Skill Command Handler
// ============================================================================

// DeleteSkillCommand handles the delete-skill command
type DeleteSkillCommand struct {
	client        HTTPClientInterface
	skillProvider Provider
}

// NewDeleteSkillCommand creates a new delete skill command handler
func NewDeleteSkillCommand(client HTTPClientInterface, skillProvider Provider) *DeleteSkillCommand {
	return &DeleteSkillCommand{
		client:        client,
		skillProvider: skillProvider,
	}
}

// DeleteSkillArgs holds the parsed arguments for delete-skill command
type DeleteSkillArgs struct {
	SkillName string
	HubID     string
	ShowHelp  bool
}

// ParseDeleteSkillArgs parses the delete-skill command arguments
func ParseDeleteSkillArgs(args []string) (*DeleteSkillArgs, error) {
	result := &DeleteSkillArgs{HubID: DefaultHubID}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-h", "--help":
			result.ShowHelp = true
			return result, nil
		case "--hub":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.HubID = normalizeHubID(args[i+1])
				i++
			} else {
				return nil, fmt.Errorf("hub flag requires a value")
			}
		default:
			// Non-flag argument is the skill name
			if !strings.HasPrefix(arg, "-") && result.SkillName == "" {
				result.SkillName = arg
			}
		}
	}

	if result.SkillName == "" && !result.ShowHelp {
		return nil, fmt.Errorf("skill name is required")
	}

	return result, nil
}

// Execute runs the delete-skill command
func (c *DeleteSkillCommand) Execute(args []string) error {
	parsedArgs, err := ParseDeleteSkillArgs(args)
	if err != nil {
		return err
	}

	if parsedArgs.ShowHelp {
		c.PrintHelp()
		return nil
	}

	return c.deleteSkill(stdctx.Background(), parsedArgs.SkillName, parsedArgs.HubID)
}

// deleteSkill deletes a skill and its index using SkillProvider
func (c *DeleteSkillCommand) deleteSkill(ctx stdctx.Context, skillName, hubID string) error {
	if c.skillProvider == nil {
		return fmt.Errorf("skill provider not available")
	}

	skillProvider, ok := c.skillProvider.(*SkillProvider)
	if !ok {
		return fmt.Errorf("invalid skill provider type")
	}

	// Delete index first
	fmt.Printf("Deleting search index for skill '%s'...\n", skillName)
	if err := skillProvider.DeleteSkill(ctx, hubID, skillName); err != nil {
		fmt.Printf("⚠ Warning: Failed to delete search index: %v\n", err)
	} else {
		fmt.Printf("✓ Search index deleted\n")
	}

	// Note: File system deletion is handled separately by FileProvider
	fmt.Printf("✓ Successfully deleted skill '%s'\n", skillName)
	return nil
}

// PrintHelp prints the help message for delete-skill command
func (c *DeleteSkillCommand) PrintHelp() {
	fmt.Println(`Usage: delete-skill <skill-name> [options]

Delete a skill from RAGFlow and remove its search index.

Arguments:
  <skill-name>           Name of the skill to delete

Options:
  --hub string           Skills Hub ID (default: default)
  -h, --help            Show this help message

Examples:
  delete-skill my-skill
  delete-skill my-skill --hub hub1
  delete-skill my-awesome-skill`)
}

// ============================================================================
// Skill Uploader
// ============================================================================

// SkillUploader handles uploading skills to the server
type SkillUploader struct {
	client        HTTPClientInterface
	fileProvider  *FileProvider
	skillProvider Provider
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

// parseHubFromPath extracts hub ID from a path like "skills/hub1" or "skills"
// Returns "default" for "skills" (no hub specified)
func parseHubFromPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "skills" {
		return DefaultHubID
	}
	// Handle paths like "skills/hub1" or "hub1"
	if strings.HasPrefix(path, "skills/") {
		path = strings.TrimPrefix(path, "skills/")
	}
	if path == "" {
		return DefaultHubID
	}
	return normalizeHubID(path)
}

// UploadSkill uploads a skill directory to the server
// nameOverride: user-specified skill name (overrides SKILL.md metadata)
func (u *SkillUploader) UploadSkill(ctx stdctx.Context, skillPath string, versionOverride string, hubPath string, nameOverride string) error {
	// Parse hub from path
	hubID := parseHubFromPath(hubPath)

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

	// 2. Ensure skills hub exists
	fmt.Printf("Checking skills hub '%s'...\n", hubID)
	hubFolderID, err := u.ensureSkillsHubFolder(ctx, hubID)
	if err != nil {
		return fmt.Errorf("failed to ensure skills hub: %w", err)
	}

	// 3. Get or create skill folder
	fmt.Printf("Checking skill '%s'...\n", skillName)
	skillFolderID, err := u.getOrCreateSkillFolder(ctx, hubID, hubFolderID, skillName)
	if err != nil {
		return err
	}

	// 4. Check if version already exists
	fmt.Printf("Checking version '%s'...\n", version)
	exists, err := u.versionExists(ctx, hubID, skillName, version)
	if err != nil {
		return fmt.Errorf("failed to check version: %w", err)
	}
	if exists {
		return &SkillConflictError{Type: "version", Name: skillName, Version: version}
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
	if err := u.indexSkill(ctx, result, files, hubID, skillFolderID); err != nil {
		fmt.Printf("⚠ Warning: Failed to index skill for search: %v\n", err)
	} else {
		fmt.Println("✓ Skill indexed successfully")
	}

	return nil
}

// ensureSkillsHubFolder ensures the 'skills/<hub>' folder exists
func (u *SkillUploader) ensureSkillsHubFolder(ctx stdctx.Context, hubID string) (string, error) {
	skillsFolderID, err := u.ensureSkillsFolder(ctx)
	if err != nil {
		return "", err
	}

	result, err := u.fileProvider.List(ctx, "skills", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == NodeTypeDirectory && node.Name == hubID {
			return GetString(node.Metadata["id"]), nil
		}
	}

	return u.createFolder(ctx, skillsFolderID, hubID)
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
func (u *SkillUploader) getOrCreateSkillFolder(ctx stdctx.Context, hubID, parentID, skillName string) (string, error) {
	result, err := u.fileProvider.List(ctx, fmt.Sprintf("skills/%s", hubID), nil)
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
func (u *SkillUploader) versionExists(ctx stdctx.Context, hubID, skillName, version string) (bool, error) {
	result, err := u.fileProvider.List(ctx, fmt.Sprintf("skills/%s/%s", hubID, skillName), nil)
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
func (u *SkillUploader) indexSkill(ctx stdctx.Context, result *SkillValidationResult, files []*SkillFile, hubID, skillFolderID string) error {
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
	}

	return skillProvider.IndexSkill(ctx, hubID, skillInfo)
}
