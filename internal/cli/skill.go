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

package cli

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"ragflow/internal/cli/filesystem"
)

// ============================================================================
// Constants (from web/src/pages/skills/validation.ts)
// ============================================================================

const (
	MaxTotalSize = 50 * 1024 * 1024 // 50MB
	MaxFileSize  = 5 * 1024 * 1024  // 5MB per file
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

// Default ignore patterns (from validation.ts)
var defaultIgnorePatterns = []string{
	".git/", ".svn/", ".hg/", "node_modules/", "__MACOSX/",
	".DS_Store", "._*", "*.log", "*.tmp", "*.temp", "*.swp", "*.swo", "*~",
	".env", ".env.*", ".vscode/", ".idea/", "Thumbs.db", "desktop.ini",
	".skill-meta.json",
}

// ============================================================================
// Skill Metadata Types
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
	Error       string
	Details     string
}

// SkillFile represents a file in the skill directory
type SkillFile struct {
	Path    string
	Content []byte
	Size    int64
}

// ============================================================================
// Skill Validator (ported from web/src/pages/skills/validation.ts)
// ============================================================================

// SkillValidator handles skill validation
type SkillValidator struct{}

// NewSkillValidator creates a new validator
func NewSkillValidator() *SkillValidator {
	return &SkillValidator{}
}

// ValidateSkillDirectory validates a skill directory
func (v *SkillValidator) ValidateSkillDirectory(skillPath string, versionOverride string) (*SkillValidationResult, []*SkillFile, error) {
	// Check if directory exists
	info, err := os.Stat(skillPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot access directory %s: %w", skillPath, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", skillPath)
	}

	// Read all files in the directory
	files, err := v.readSkillFiles(skillPath)
	if err != nil {
		return nil, nil, err
	}

	if len(files) == 0 {
		return &SkillValidationResult{Valid: false, Error: "no_files"}, nil, nil
	}

	// Check total size
	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}
	if totalSize > MaxTotalSize {
		return &SkillValidationResult{Valid: false, Error: "total_size_exceeded"}, nil, nil
	}

	// Check individual file sizes and filter valid files
	var validFiles []*SkillFile
	for _, f := range files {
		if f.Size > MaxFileSize {
			return &SkillValidationResult{
				Valid:   false,
				Error:   "file_too_large",
				Details: f.Path,
			}, nil, nil
		}

		// Sanitize and check path
		sanitized := sanitizeRelPath(f.Path)
		if sanitized == "" {
			return &SkillValidationResult{Valid: false, Error: "invalid_path"}, nil, nil
		}

		// Skip junk and ignored files
		if isMacJunkPath(sanitized) || shouldIgnore(sanitized, defaultIgnorePatterns) {
			continue
		}

		validFiles = append(validFiles, f)
	}

	if len(validFiles) == 0 {
		return &SkillValidationResult{Valid: false, Error: "no_valid_files"}, nil, nil
	}

	// Find SKILL.md file
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

	// Parse and validate SKILL.md
	metadata, err := parseFrontmatter(string(skillMdFile.Content))
	if err != nil {
		return &SkillValidationResult{
			Valid:   false,
			Error:   "invalid_frontmatter",
			Details: err.Error(),
		}, nil, nil
	}

	// Validate required fields
	if metadata.Name == "" {
		return &SkillValidationResult{Valid: false, Error: "missing_name"}, nil, nil
	}

	// Validate name format (slug format: lowercase, URL-safe)
	if !isValidSkillName(metadata.Name) {
		return &SkillValidationResult{
			Valid:   false,
			Error:   "invalid_name_format",
			Details: metadata.Name,
		}, nil, nil
	}

	// Use override version if provided, otherwise use metadata version
	version := versionOverride
	if version == "" {
		version = metadata.Version
	}

	// Validate version if provided (should be semver)
	if version != "" && !isValidSemver(version) {
		return &SkillValidationResult{
			Valid:   false,
			Error:   "invalid_version",
			Details: version,
		}, nil, nil
	}

	// Validate all files are text-based
	for _, f := range validFiles {
		if !isTextFile(f.Path, "") {
			return &SkillValidationResult{
				Valid:   false,
				Error:   "invalid_file_type",
				Details: f.Path,
			}, nil, nil
		}
	}

	return &SkillValidationResult{
		Valid:       true,
		Name:        metadata.Name,
		Description: metadata.Description,
		Version:     version,
	}, validFiles, nil
}

// readSkillFiles recursively reads all files in the skill directory
func (v *SkillValidator) readSkillFiles(skillPath string) ([]*SkillFile, error) {
	var files []*SkillFile

	err := filepath.Walk(skillPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Get relative path from skill root
			relPath, err := filepath.Rel(skillPath, path)
			if err != nil {
				return err
			}

			// Normalize path separator
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

// ============================================================================
// Validation Helper Functions
// ============================================================================

// parseFrontmatter extracts YAML frontmatter from markdown content
func parseFrontmatter(content string) (*SkillMetadata, error) {
	lines := strings.Split(content, "\n")

	// Check frontmatter start
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing frontmatter start")
	}

	// Find end of frontmatter
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

	// Parse YAML frontmatter
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

// isValidSemver checks basic semver format (x.y.z)
func isValidSemver(version string) bool {
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+`, version)
	return matched
}

// isTextFile checks if file is text-based
func isTextFile(filePath, contentType string) bool {
	// Check content type first
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

	// Check extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != "" {
		ext = ext[1:] // Remove leading dot
	}
	return textFileExtensions[ext]
}

// sanitizeRelPath sanitizes relative path to prevent directory traversal
func sanitizeRelPath(path string) string {
	// Remove leading ./ and /
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
	// Handle directory patterns (trailing slash)
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(filePath, dirPattern+"/") || filePath == dirPattern
	}

	// Handle exact match
	if filePath == pattern {
		return true
	}

	// Handle glob patterns
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
				// ** matches any number of directories
				regex.WriteString(".*")
				i++
			} else {
				// * matches any characters except /
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

// ============================================================================
// Error Message Helpers
// ============================================================================

// GetValidationErrorMessage returns human-readable error message
func GetValidationErrorMessage(result *SkillValidationResult) string {
	switch result.Error {
	case "no_files":
		return "No files found in the skill directory"
	case "total_size_exceeded":
		return fmt.Sprintf("Total size exceeds limit of %d MB", MaxTotalSize/(1024*1024))
	case "file_too_large":
		return fmt.Sprintf("File too large: %s (max %d MB per file)", result.Details, MaxFileSize/(1024*1024))
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

// ============================================================================
// Skill Uploader
// ============================================================================

// SkillUploader handles uploading skills to the server
type SkillUploader struct {
	client       *RAGFlowClient
	fileProvider *filesystem.FileProvider
}

// NewSkillUploader creates a new uploader
func NewSkillUploader(client *RAGFlowClient, fileProvider *filesystem.FileProvider) *SkillUploader {
	return &SkillUploader{
		client:       client,
		fileProvider: fileProvider,
	}
}

// UploadSkill uploads a skill directory to the server
func (u *SkillUploader) UploadSkill(ctx stdctx.Context, skillPath string, versionOverride string) error {
	validator := NewSkillValidator()

	// 1. Validate the skill directory
	fmt.Printf("Validating skill at %s...\n", skillPath)
	result, files, err := validator.ValidateSkillDirectory(skillPath, versionOverride)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	if !result.Valid {
		return fmt.Errorf("validation failed: %s", GetValidationErrorMessage(result))
	}

	// Get skill name from directory name (as requested)
	skillName := filepath.Base(skillPath)
	// Normalize skill name
	skillName = normalizeSkillName(skillName)

	// Use provided version or default
	version := result.Version
	if version == "" {
		version = "1.0.0"
	}

	fmt.Printf("✓ Skill '%s' (v%s) is valid\n", skillName, version)

	// 2. Ensure skills folder exists
	fmt.Println("Checking skills folder...")
	skillsFolderID, err := u.ensureSkillsFolder(ctx)
	if err != nil {
		return fmt.Errorf("failed to ensure skills folder: %w", err)
	}

	// 3. Get or create skill folder
	fmt.Printf("Checking skill '%s'...\n", skillName)
	skillFolderID, err := u.getOrCreateSkillFolder(ctx, skillsFolderID, skillName)
	if err != nil {
		return err
	}

	// 4. Check if version already exists
	fmt.Printf("Checking version '%s'...\n", version)
	exists, err := u.versionExists(ctx, skillFolderID, skillName, version)
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
		// Skip ignored files
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
	return nil
}

// ensureSkillsFolder ensures the 'skills' folder exists in file manager
func (u *SkillUploader) ensureSkillsFolder(ctx stdctx.Context) (string, error) {
	// List root to find skills folder
	result, err := u.fileProvider.List(ctx, "", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == filesystem.NodeTypeDirectory && node.Name == "skills" {
			return getString(node.Metadata["id"]), nil
		}
	}

	// Create skills folder
	return u.createFolder(ctx, "", "skills")
}

// getOrCreateSkillFolder gets existing skill folder or creates new one
// Allows same skill name with different versions
func (u *SkillUploader) getOrCreateSkillFolder(ctx stdctx.Context, parentID, skillName string) (string, error) {
	result, err := u.fileProvider.List(ctx, "skills", nil)
	if err != nil {
		return "", err
	}

	for _, node := range result.Nodes {
		if node.Type == filesystem.NodeTypeDirectory && node.Name == skillName {
			// Skill folder exists, return its ID
			return getString(node.Metadata["id"]), nil
		}
	}

	// Create skill folder
	return u.createFolder(ctx, parentID, skillName)
}

// versionExists checks if a version already exists
func (u *SkillUploader) versionExists(ctx stdctx.Context, skillFolderID, skillName, version string) (bool, error) {
	// List skill folder contents using path "skills/{skillName}"
	result, err := u.fileProvider.List(ctx, fmt.Sprintf("skills/%s", skillName), nil)
	if err != nil {
		return false, err
	}

	for _, node := range result.Nodes {
		if node.Type == filesystem.NodeTypeDirectory && node.Name == version {
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

	resp, err := u.client.HTTPClient.Request("POST", "/files", true, "auto", nil, payload)
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
	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add parent_id field
	if parentID != "" {
		writer.WriteField("parent_id", parentID)
	}

	// Add file
	part, err := writer.CreateFormFile("file", file.Path)
	if err != nil {
		return err
	}
	if _, err := part.Write(file.Content); err != nil {
		return err
	}
	writer.Close()

	// Make request using HTTPClient's multipart support
	return u.client.HTTPClient.UploadMultipart("/files", writer.FormDataContentType(), &buf)
}

// normalizeSkillName normalizes skill name for folder creation
func normalizeSkillName(name string) string {
	// Replace spaces with hyphens and convert to lowercase
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	// Remove any characters that aren't alphanumeric or hyphen
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	name = re.ReplaceAllString(name, "-")
	// Remove consecutive hyphens
	re = regexp.MustCompile(`-+`)
	name = re.ReplaceAllString(name, "-")
	// Trim hyphens from start and end
	name = strings.Trim(name, "-")
	return name
}

// SkillConflictError represents a conflict error (name or version)
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
// Search Skills Command Handler
// ============================================================================

// SearchSkillsCommand handles the search-skills command
type SearchSkillsCommand struct {
	client *RAGFlowClient
}

// NewSearchSkillsCommand creates a new search command handler
func NewSearchSkillsCommand(client *RAGFlowClient) *SearchSkillsCommand {
	return &SearchSkillsCommand{client: client}
}

// SearchSkillsArgs holds the parsed arguments for search-skills command
type SearchSkillsArgs struct {
	Query      string
	Page       int
	PageSize   int
	ShowHelp   bool
}

// ParseSearchSkillsArgs parses the search-skills command arguments
func ParseSearchSkillsArgs(args []string) (*SearchSkillsArgs, error) {
	result := &SearchSkillsArgs{
		Page:     1,
		PageSize: 10,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-h", "--help":
			result.ShowHelp = true
			return result, nil
		case "-q", "--query":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.Query = args[i+1]
				i++
			} else {
				return nil, fmt.Errorf("query flag requires a value")
			}
		case "-p", "--page":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				fmt.Sscanf(args[i+1], "%d", &result.Page)
				i++
			}
		case "-n", "--page-size":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				fmt.Sscanf(args[i+1], "%d", &result.PageSize)
				i++
			}
		default:
			// Non-flag argument is treated as query (for convenience)
			if !strings.HasPrefix(arg, "-") && result.Query == "" {
				result.Query = arg
			}
		}
	}

	if result.Query == "" && !result.ShowHelp {
		return nil, fmt.Errorf("query is required (use -q or --query)")
	}

	return result, nil
}

// Execute runs the search-skills command
func (c *SearchSkillsCommand) Execute(args []string) error {
	parsedArgs, err := ParseSearchSkillsArgs(args)
	if err != nil {
		return err
	}

	if parsedArgs.ShowHelp {
		c.PrintHelp()
		return nil
	}

	return c.searchSkills(parsedArgs)
}

// searchSkills performs the actual search
func (c *SearchSkillsCommand) searchSkills(args *SearchSkillsArgs) error {
	payload := map[string]interface{}{
		"query":     args.Query,
		"page":      args.Page,
		"page_size": args.PageSize,
	}

	resp, err := c.client.HTTPClient.Request("POST", "/api/v1/skill/search", true, "json", nil, payload)
	if err != nil {
		return fmt.Errorf("search request failed: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
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
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("search failed: %s", result.Msg)
	}

	// Print results
	if len(result.Data.Skills) == 0 {
		fmt.Println("No skills found matching your query.")
		return nil
	}

	fmt.Printf("Found %d skill(s) matching '%s':\n\n", result.Data.Total, args.Query)

	for i, skill := range result.Data.Skills {
		fmt.Printf("%d. %s (score: %.3f)\n", i+1, skill.Name, skill.Score)
		if skill.Description != "" {
			fmt.Printf("   %s\n", skill.Description)
		}
		if len(skill.Tags) > 0 {
			fmt.Printf("   Tags: %s\n", strings.Join(skill.Tags, ", "))
		}
		if skill.BM25Score > 0 || skill.VectorScore > 0 {
			fmt.Printf("   [BM25: %.3f, Vector: %.3f]\n", skill.BM25Score, skill.VectorScore)
		}
		fmt.Println()
	}

	return nil
}

// PrintHelp prints the help message for search-skills command
func (c *SearchSkillsCommand) PrintHelp() {
	fmt.Println(`Usage: search skills [options] <query>

Search for skills using semantic search.

Arguments:
  <query>                Search query (can also use -q flag)

Options:
  -q, --query string     Search query (required if not provided as argument)
  -p, --page int         Page number (default: 1)
  -n, --page-size int    Number of results per page (default: 10)
  -h, --help            Show this help message

Examples:
  search skills "data processing"
  search skills -q "machine learning"
  search skills -q "api" -p 1 -n 20`)
}

// ============================================================================
// AddSkill Command Handler
// ============================================================================

// AddSkillCommand handles the add-skill command
type AddSkillCommand struct {
	client       *RAGFlowClient
	fileProvider *filesystem.FileProvider
}

// NewAddSkillCommand creates a new command handler
func NewAddSkillCommand(client *RAGFlowClient, fileProvider *filesystem.FileProvider) *AddSkillCommand {
	return &AddSkillCommand{
		client:       client,
		fileProvider: fileProvider,
	}
}

// AddSkillArgs holds the parsed arguments for add-skill command
type AddSkillArgs struct {
	SkillPath      string
	Version        string
	ShowHelp       bool
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

	// Upload skill
	uploader := NewSkillUploader(c.client, c.fileProvider)
	err = uploader.UploadSkill(stdctx.Background(), skillPath, parsedArgs.Version)
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

Arguments:
  <path>                 Path to the skill directory

Options:
  -v, --version string   Specify the skill version (default: from SKILL.md or 1.0.0)
  -h, --help            Show this help message

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

// getString safely extracts a string value from interface{}
func getString(v interface{}) string {
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
