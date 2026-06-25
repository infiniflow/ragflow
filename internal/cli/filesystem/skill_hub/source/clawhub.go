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

package source

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// progressLogger is a simple logger for user-facing progress messages
type progressLogger struct {
	enabled bool
}

func (l *progressLogger) log(format string, args ...interface{}) {
	if l.enabled {
		fmt.Printf("  → "+format+"\n", args...)
	}
}

func (l *progressLogger) error(format string, args ...interface{}) {
	fmt.Printf("  ✗ "+format+"\n", args...)
}

func (l *progressLogger) success(format string, args ...interface{}) {
	fmt.Printf("  ✓ "+format+"\n", args...)
}

const (
	clawHubBaseURL = "https://clawhub.ai/api/v1"
)

// ClawHubSource handles ClawHub registry skills
// Reference implementation: hermes-agent/tools/skills_hub.py ClawHubSource
// All skills are treated as community trust — ClawHavoc incident showed
// their vetting is insufficient (341 malicious skills found Feb 2026).
type ClawHubSource struct {
	client HTTPClientInterface
	logger progressLogger
}

// NewClawHubSource creates a new ClawHub source adapter
func NewClawHubSource(client HTTPClientInterface) *ClawHubSource {
	return &ClawHubSource{client: client, logger: progressLogger{enabled: true}}
}

// SourceID returns the source identifier
func (s *ClawHubSource) SourceID() string {
	return "clawhub"
}

// TrustLevel returns the trust level for ClawHub
func (s *ClawHubSource) TrustLevel(identifier string) string {
	// ClawHub has community verification
	return "community"
}

// Search searches for skills on ClawHub matching the query
func (s *ClawHubSource) Search(query string, limit int) ([]*SkillMetadata, error) {
	if limit <= 0 {
		limit = 10
	}

	// Try direct slug match first for exact queries
	if query != "" && len(query) >= 2 {
		meta, err := s.exactSlugMeta(query)
		if err == nil && meta != nil {
			return []*SkillMetadata{meta}, nil
		}
	}

	// Use the lightweight listing API
	url := fmt.Sprintf("%s/skills", clawHubBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	if query != "" {
		q.Add("search", query)
	}
	q.Add("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search ClawHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ClawHub API returned %d", resp.StatusCode)
	}

	var data struct {
		Items []struct {
			Slug        string      `json:"slug"`
			DisplayName string      `json:"displayName"`
			Name        string      `json:"name"`
			Summary     string      `json:"summary"`
			Description string      `json:"description"`
			Tags        interface{} `json:"tags"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]*SkillMetadata, 0, len(data.Items))
	for _, item := range data.Items {
		slug := item.Slug
		if slug == "" {
			continue
		}
		displayName := item.DisplayName
		if displayName == "" {
			displayName = item.Name
		}
		if displayName == "" {
			displayName = slug
		}
		summary := item.Summary
		if summary == "" {
			summary = item.Description
		}

		results = append(results, &SkillMetadata{
			Name:        displayName,
			Description: summary,
			Version:     "",
			Author:      "",
			Tags:        normalizeTags(item.Tags),
		})
	}

	// Apply search scoring and filtering
	results = s.finalizeSearchResults(query, results, limit)
	return results, nil
}

// Fetch retrieves a skill from ClawHub
// Downloads the skill as a ZIP bundle and extracts text files
// Supports identifier with version: "slug@version" or just "slug" (uses latest)
func (s *ClawHubSource) Fetch(identifier string) (*SkillBundle, error) {
	slug, specifiedVersion := extractSlugAndVersion(identifier)
	s.logger.log("Looking up skill '%s' on ClawHub...", slug)

	// Fetch skill metadata
	skillData, err := s.getSkillData(slug)
	if err != nil {
		s.logger.error("Cannot find skill '%s' on ClawHub: %v", slug, err)
		return nil, fmt.Errorf("skill '%s' not found on ClawHub: %w", slug, err)
	}
	s.logger.success("Found skill: %s", skillData.DisplayName)

	// Determine version to download
	var version string
	if specifiedVersion != "" {
		version = specifiedVersion
		s.logger.log("Using specified version: %s", version)
	} else {
		// Resolve the latest version
		s.logger.log("Resolving latest version...")
		version, err = s.resolveLatestVersion(slug, skillData)
		if err != nil {
			s.logger.error("Cannot determine version for '%s': %v", slug, err)
			return nil, fmt.Errorf("could not resolve latest version for %s: %w", slug, err)
		}
		if version == "" {
			s.logger.error("No versions available for skill '%s'", slug)
			return nil, fmt.Errorf("no version found for skill %s", slug)
		}
		s.logger.success("Latest version: %s", version)
	}

	// Try to get files from version metadata endpoint first (avoids rate-limited /download)
	var files map[string][]byte
	s.logger.log("Fetching skill files (version %s)...", version)
	versionData, err := s.getVersionData(slug, version)
	if err == nil {
		files = s.extractFiles(versionData)
		if len(files) > 0 {
			s.logger.success("Fetched %d files from metadata", len(files))
		}
	}

	// Fallback to ZIP download if metadata method didn't return files
	if len(files) == 0 {
		s.logger.log("Trying ZIP download...")
		// Add delay before download to avoid rate limit
		time.Sleep(3 * time.Second)
		zipFiles, err2 := s.downloadZip(slug, version)
		if err2 != nil {
			s.logger.error("Failed to download skill bundle: %v", err2)
			return nil, fmt.Errorf("failed to download skill '%s': %w", slug, err2)
		}
		files = zipFiles
		s.logger.success("Downloaded %d files via ZIP", len(files))
	}

	// Validate: must have SKILL.md
	if _, ok := files["SKILL.md"]; !ok {
		s.logger.error("Downloaded bundle is missing SKILL.md (required file)")
		return nil, fmt.Errorf("SKILL.md not found in skill %s (version %s)", slug, version)
	}

	return &SkillBundle{
		Name:       slug,
		Files:      files,
		Source:     "clawhub",
		Identifier: slug,
		TrustLevel: s.TrustLevel(identifier),
		Metadata: &SkillMetadata{
			Name:        skillData.DisplayName,
			Description: skillData.Summary,
			Version:     version,
		},
	}, nil
}

// Inspect retrieves metadata from ClawHub without downloading full content
func (s *ClawHubSource) Inspect(identifier string) (*SkillMetadata, error) {
	slug := extractSlug(identifier)

	skillData, err := s.getSkillData(slug)
	if err != nil {
		return nil, err
	}

	return &SkillMetadata{
		Name:        skillData.DisplayName,
		Description: skillData.Summary,
		Version:     "",
		Author:      "",
		Tags:        normalizeTags(skillData.Tags),
	}, nil
}

// getSkillData fetches skill metadata from ClawHub API with retry logic
func (s *ClawHubSource) getSkillData(slug string) (*clawHubSkillData, error) {
	url := fmt.Sprintf("%s/skills/%s", clawHubBaseURL, slug)

	body, err := s.doRequestWithRetry("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// ClawHub API may return nested structure: {"skill": {...}, "latestVersion": ...}
	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, err
	}

	return coerceSkillPayload(rawData), nil
}

// getVersionData fetches version-specific metadata with retry logic
func (s *ClawHubSource) getVersionData(slug, version string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/skills/%s/versions/%s", clawHubBaseURL, slug, version)

	body, err := s.doRequestWithRetry("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// resolveLatestVersion extracts the latest version from skill data with retry logic
func (s *ClawHubSource) resolveLatestVersion(slug string, skillData *clawHubSkillData) (string, error) {
	// Try latestVersion field first
	if skillData.LatestVersion != "" {
		return skillData.LatestVersion, nil
	}

	// Try tags.latest
	if skillData.TagsLatest != "" {
		return skillData.TagsLatest, nil
	}

	// Fallback: fetch versions list and take first
	url := fmt.Sprintf("%s/skills/%s/versions", clawHubBaseURL, slug)

	body, err := s.doRequestWithRetry("GET", url, nil)
	if err != nil {
		return "", err
	}

	var versions []struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &versions); err != nil {
		return "", err
	}

	if len(versions) > 0 && versions[0].Version != "" {
		return versions[0].Version, nil
	}

	return "", nil
}

// downloadZip downloads skill as ZIP bundle and extracts text files
func (s *ClawHubSource) downloadZip(slug, version string) (map[string][]byte, error) {
	// Use the correct endpoint with slug parameter (matching hermes-agent)
	url := fmt.Sprintf("%s/download?slug=%s&version=%s", clawHubBaseURL, slug, version)
	s.logger.log("Downloading ZIP from: %s", url)

	body, err := s.doRequestWithRetry("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	s.logger.log("Downloaded %d bytes, extracting files...", len(body))

	// Extract ZIP
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		s.logger.error("Downloaded file is not a valid ZIP archive: %v", err)
		return nil, fmt.Errorf("invalid ZIP file: %w", err)
	}

	files := make(map[string][]byte)
	skippedCount := 0
	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// Validate path for safety
		name := file.Name
		if !isSafePath(name) {
			skippedCount++
			continue
		}

		// Skip large files (>500KB)
		if file.UncompressedSize64 > 500_000 {
			skippedCount++
			s.logger.log("Skipping large file: %s (%.1f MB)", name, float64(file.UncompressedSize64)/1024/1024)
			continue
		}

		// Read file content
		rc, err := file.Open()
		if err != nil {
			skippedCount++
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			skippedCount++
			continue
		}

		// Only include text files (check for null bytes indicating binary)
		if isTextContent(content) {
			files[name] = content
		} else {
			skippedCount++
			s.logger.log("Skipping binary file: %s", name)
		}
	}

	if skippedCount > 0 {
		s.logger.log("Skipped %d files (unsafe paths, large files, or binary content)", skippedCount)
	}

	if len(files) == 0 {
		s.logger.error("No valid files found in the ZIP archive")
		return nil, fmt.Errorf("no valid files extracted from ZIP")
	}

	return files, nil
}

// extractFiles extracts files from version data structure
func (s *ClawHubSource) extractFiles(versionData map[string]interface{}) map[string][]byte {
	files := make(map[string][]byte)

	// Check for nested version -> files structure
	if nested, ok := versionData["version"].(map[string]interface{}); ok {
		versionData = nested
	}

	fileList, ok := versionData["files"]
	if !ok {
		return files
	}

	// Handle map structure: {"filename": "content"}
	if fileMap, ok := fileList.(map[string]interface{}); ok {
		for name, content := range fileMap {
			if s, ok := content.(string); ok && isSafePath(name) {
				files[name] = []byte(s)
			}
		}
		return files
	}

	// Handle array structure with file metadata
	if fileArray, ok := fileList.([]interface{}); ok {
		for _, item := range fileArray {
			fileMeta, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			name := ""
			if n, ok := fileMeta["path"].(string); ok && n != "" {
				name = n
			} else if n, ok := fileMeta["name"].(string); ok && n != "" {
				name = n
			}
			if name == "" || !isSafePath(name) {
				continue
			}

			// Try inline content first
			if content, ok := fileMeta["content"].(string); ok {
				files[name] = []byte(content)
				continue
			}

			// Try rawUrl/downloadUrl
			var url string
			if u, ok := fileMeta["rawUrl"].(string); ok && u != "" {
				url = u
			} else if u, ok := fileMeta["downloadUrl"].(string); ok && u != "" {
				url = u
			} else if u, ok := fileMeta["url"].(string); ok && u != "" {
				url = u
			}

			if url != "" && strings.HasPrefix(url, "http") {
				content, err := s.fetchText(url)
				if err == nil {
					files[name] = []byte(content)
				}
			}
		}
	}

	return files
}

// fetchText fetches text content from URL
func (s *ClawHubSource) fetchText(url string) (string, error) {
	resp, err := s.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// doRequestWithRetry performs HTTP request with retry logic for 429 rate limiting
func (s *ClawHubSource) doRequestWithRetry(method, url string, body []byte) ([]byte, error) {
	maxRetries := 5
	var lastErr error
	isDownload := strings.Contains(url, "/download")

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Initial delay for download requests to avoid triggering rate limit
		if attempt == 0 && isDownload {
			s.logger.log("Adding initial delay for download request...")
			time.Sleep(5 * time.Second)
		}

		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			s.logger.error("Request setup failed: %v", lastErr)
			continue
		}

		// Simple headers like hermes-agent
		req.Header.Set("User-Agent", "RAGFlow-CLI/1.0")
		req.Header.Set("Accept", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries-1 {
				s.logger.error("Request failed (attempt %d/%d): %v", attempt+1, maxRetries, err)
			}
			continue
		}

		// Read response body immediately
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			if attempt < maxRetries-1 {
				s.logger.error("Response read failed (attempt %d/%d): %v", attempt+1, maxRetries, err)
			}
			continue
		}

		// Handle rate limiting - ClawHub has strict limits, wait 30-60s to reset window
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := resp.Header.Get("Retry-After")
			waitSeconds := 30 // Default: wait 30 seconds
			if retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
					waitSeconds = seconds
				}
			}
			// Ensure minimum 30s wait to reset rate limit window
			if waitSeconds < 30 {
				waitSeconds = 30
			}
			// Cap at 60 seconds
			if waitSeconds > 60 {
				waitSeconds = 60
			}
			s.logger.log("Rate limited by ClawHub, waiting %d seconds...", waitSeconds)
			time.Sleep(time.Duration(waitSeconds) * time.Second)
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("skill not found (HTTP 404)")
			s.logger.error("%v", lastErr)
			return nil, lastErr // Don't retry 404
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			lastErr = fmt.Errorf("access denied (HTTP %d) - check your credentials", resp.StatusCode)
			s.logger.error("%v", lastErr)
			return nil, lastErr // Don't retry auth errors
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("ClawHub API returned HTTP %d", resp.StatusCode)
			if attempt < maxRetries-1 {
				s.logger.error("Server error (attempt %d/%d): HTTP %d", attempt+1, maxRetries, resp.StatusCode)
			}
			continue
		}

		return respBody, nil
	}

	// Provide helpful error message based on the last error
	var userMsg string
	if lastErr != nil {
		errStr := lastErr.Error()
		switch {
		case strings.Contains(errStr, "connection refused"):
			userMsg = "Cannot connect to ClawHub - the service may be down or your network is blocking the connection"
		case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
			userMsg = "Connection to ClawHub timed out - your network may be slow or the service is unresponsive"
		case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "DNS"):
			userMsg = "Cannot resolve ClawHub hostname - check your internet connection or DNS settings"
		case strings.Contains(errStr, "certificate"):
			userMsg = "SSL certificate error - your system may have outdated certificates or someone is intercepting the connection"
		default:
			userMsg = fmt.Sprintf("Network error after %d attempts: %v", maxRetries, lastErr)
		}
	} else {
		userMsg = fmt.Sprintf("Failed after %d attempts - unknown error", maxRetries)
	}

	return nil, fmt.Errorf("%s", userMsg)
}

// exactSlugMeta tries to find skill by exact slug match
func (s *ClawHubSource) exactSlugMeta(query string) (*SkillMetadata, error) {
	slug := extractSlug(query)
	queryTermList := extractQueryTerms(query)

	candidates := []string{}

	// If slug looks valid, add it
	if slug != "" && regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(slug) {
		candidates = append(candidates, slug)
	}

	// Generate variations from query terms
	if len(queryTermList) > 0 {
		baseSlug := strings.Join(queryTermList, "-")
		if len(queryTermList) >= 2 {
			candidates = append(candidates,
				baseSlug+"-agent",
				baseSlug+"-skill",
				baseSlug+"-tool",
				baseSlug+"-assistant",
				baseSlug+"-playbook",
				baseSlug,
			)
		} else {
			candidates = append(candidates, baseSlug)
		}
	}

	seen := make(map[string]bool)
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true

		meta, err := s.Inspect(candidate)
		if err == nil && meta != nil && meta.Name != "" {
			return meta, nil
		}
	}

	return nil, fmt.Errorf("no exact match found")
}

// finalizeSearchResults applies scoring and filtering to search results
func (s *ClawHubSource) finalizeSearchResults(query string, results []*SkillMetadata, limit int) []*SkillMetadata {
	if query == "" {
		deduped := dedupeResults(results)
		if len(deduped) > limit {
			return deduped[:limit]
		}
		return deduped
	}

	// Score and filter
	filtered := make([]*SkillMetadata, 0)
	for _, meta := range results {
		if s.searchScore(query, meta) > 0 {
			filtered = append(filtered, meta)
		}
	}

	// Sort by score
	sort.Slice(filtered, func(i, j int) bool {
		scoreI := s.searchScore(query, filtered[i])
		scoreJ := s.searchScore(query, filtered[j])
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if filtered[i].Name != filtered[j].Name {
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		}
		return strings.ToLower(filtered[i].Description) < strings.ToLower(filtered[j].Description)
	})

	deduped := dedupeResults(filtered)
	if len(deduped) > limit {
		return deduped[:limit]
	}
	return deduped
}

// searchScore calculates relevance score for a skill against query
func (s *ClawHubSource) searchScore(query string, meta *SkillMetadata) int {
	queryNorm := strings.ToLower(strings.TrimSpace(query))
	if queryNorm == "" {
		return 1
	}

	nameLower := strings.ToLower(meta.Name)
	descLower := strings.ToLower(meta.Description)

	queryTermList := extractQueryTerms(queryNorm)
	nameTermList := extractQueryTerms(nameLower)

	score := 0

	// Exact matches (high scores)
	if queryNorm == nameLower {
		score += 130
	}
	if strings.ReplaceAll(nameLower, " ", "-") == queryNorm {
		score += 120
	}
	if strings.HasPrefix(nameLower, queryNorm) {
		score += 90
	}

	// Query terms match name terms
	if len(queryTermList) > 0 && len(nameTermList) >= len(queryTermList) {
		match := true
		for i, term := range queryTermList {
			if i >= len(nameTermList) || nameTermList[i] != term {
				match = false
				break
			}
		}
		if match {
			score += 65
		}
	}

	// Substring matches
	if strings.Contains(nameLower, queryNorm) {
		score += 35
	}
	if strings.Contains(descLower, queryNorm) {
		score += 10
	}

	// Individual term matches
	for _, term := range queryTermList {
		if strings.Contains(nameLower, term) {
			score += 12
		}
		if strings.Contains(descLower, term) {
			score += 3
		}
	}

	return score
}

// Helper types and functions

// clawHubSkillData represents ClawHub skill API response
type clawHubSkillData struct {
	Slug          string      `json:"slug"`
	DisplayName   string      `json:"displayName"`
	Name          string      `json:"name"`
	Summary       string      `json:"summary"`
	Description   string      `json:"description"`
	Tags          interface{} `json:"tags"`
	LatestVersion string      `json:"latestVersion"`
	TagsLatest    string      `json:"tags_latest"` // Extracted from tags dict
}

// coerceSkillPayload handles nested ClawHub API response structures
// ClawHub API may return: {"skill": {...}, "latestVersion": ...} or flat structure
func coerceSkillPayload(data map[string]interface{}) *clawHubSkillData {
	result := &clawHubSkillData{}

	// Check for nested skill structure
	nested, hasNested := data["skill"].(map[string]interface{})
	if hasNested {
		// Merge nested skill data
		for k, v := range nested {
			data[k] = v
		}
		// Keep latestVersion from outer if present
		if lv, ok := data["latestVersion"].(string); ok && lv != "" {
			result.LatestVersion = lv
		}
	}

	// Extract fields
	if v, ok := data["slug"].(string); ok {
		result.Slug = v
	}
	if v, ok := data["displayName"].(string); ok {
		result.DisplayName = v
	}
	if v, ok := data["name"].(string); ok && result.DisplayName == "" {
		result.DisplayName = v
	}
	if v, ok := data["summary"].(string); ok {
		result.Summary = v
	}
	if v, ok := data["description"].(string); ok && result.Summary == "" {
		result.Summary = v
	}
	if v, ok := data["tags"]; ok {
		result.Tags = v
		// Extract latest from tags dict
		if tagMap, ok := v.(map[string]interface{}); ok {
			if latest, ok := tagMap["latest"].(string); ok {
				result.TagsLatest = latest
			}
		}
	}

	return result
}

// extractSlug extracts the skill slug from identifier
func extractSlug(identifier string) string {
	parts := strings.Split(identifier, "/")
	return parts[len(parts)-1]
}

// extractSlugAndVersion extracts the skill slug and optional version from identifier
// Supports formats: "slug", "slug@version", "owner/slug", "owner/slug@version"
func extractSlugAndVersion(identifier string) (slug, version string) {
	// First get the last part (handles owner/slug format)
	parts := strings.Split(identifier, "/")
	lastPart := parts[len(parts)-1]

	// Check for version separator @
	if idx := strings.LastIndex(lastPart, "@"); idx > 0 {
		return lastPart[:idx], lastPart[idx+1:]
	}

	return lastPart, ""
}

// normalizeTags normalizes tags from various formats
func normalizeTags(tags interface{}) []string {
	result := []string{}

	switch v := tags.(type) {
	case []interface{}:
		for _, t := range v {
			if s, ok := t.(string); ok && s != "" && s != "latest" {
				result = append(result, s)
			}
		}
	case []string:
		for _, s := range v {
			if s != "" && s != "latest" {
				result = append(result, s)
			}
		}
	case map[string]interface{}:
		for k := range v {
			if k != "" && k != "latest" {
				result = append(result, k)
			}
		}
	}

	return result
}

// dedupeResults removes duplicate skills by name, keeping first occurrence
func dedupeResults(results []*SkillMetadata) []*SkillMetadata {
	seen := make(map[string]bool)
	unique := []*SkillMetadata{}
	for _, r := range results {
		key := strings.ToLower(r.Name)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, r)
		}
	}
	return unique
}

// extractQueryTerms splits query into normalized terms
func extractQueryTerms(query string) []string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	parts := re.Split(strings.ToLower(query), -1)
	result := []string{}
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isSafePath validates that a path is safe (no directory traversal)
func isSafePath(path string) bool {
	// Clean the path
	clean := filepath.Clean(path)
	
	// Check for absolute paths
	if filepath.IsAbs(clean) {
		return false
	}
	
	// Check for parent directory references
	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return false
		}
	}
	
	return true
}

// isTextContent checks if content appears to be text (not binary)
func isTextContent(data []byte) bool {
	// Check for null bytes (indicates binary)
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
