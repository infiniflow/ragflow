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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	skillsShBaseURL = "https://skills.sh"
)

var (
	// Regex patterns for parsing skills.sh detail page
	skillsShInstallCmdRe = regexp.MustCompile(`(?i)npx\s+skills\s+add\s+(?P<repo>https?://github\.com/[^\s<]+|[^\s<]+)(?:\s+--skill\s+(?P<skill>[^\s<]+))?`)
	skillsShPageH1Re     = regexp.MustCompile(`(?i)<h1[^>]*>(?P<title>.*?)</h1>`)
	skillsShProseH1Re    = regexp.MustCompile(`(?i)<div[^>]*class=["'][^"']*prose[^"']*["'][^>]*>.*?<h1[^>]*>(?P<title>.*?)</h1>`)
	skillsShProsePRe     = regexp.MustCompile(`(?i)<div[^>]*class=["'][^"']*prose[^"']*["'][^>]*>.*?<p[^>]*>(?P<body>.*?)</p>`)
	skillsShWeeklyRe     = regexp.MustCompile(`Weekly Installs.*?children\\":\\"(?P<count>[0-9.,Kk]+)\\"`)
)

// SkillsShDetail holds parsed information from skills.sh detail page
type SkillsShDetail struct {
	Repo           string `json:"repo"`
	InstallSkill   string `json:"install_skill"`
	PageTitle      string `json:"page_title"`
	BodyTitle      string `json:"body_title"`
	BodySummary    string `json:"body_summary"`
	WeeklyInstalls string `json:"weekly_installs"`
	InstallCommand string `json:"install_command"`
	RepoURL        string `json:"repo_url"`
	DetailURL      string `json:"detail_url"`
}

// SkillsShSource handles skills.sh registry skills
type SkillsShSource struct {
	client HTTPClientInterface
	github *GitHubSource
}

// NewSkillsShSource creates a new skills.sh source adapter
func NewSkillsShSource(client HTTPClientInterface) *SkillsShSource {
	return &SkillsShSource{
		client: client,
		github: NewGitHubSource(client),
	}
}

// SourceID returns the source identifier
func (s *SkillsShSource) SourceID() string {
	return "skills-sh"
}

// TrustLevel returns the trust level for skills.sh
func (s *SkillsShSource) TrustLevel(identifier string) string {
	canonical := s.normalizeIdentifier(identifier)
	// Delegate to github trust level based on the repo
	for _, candidate := range s.candidateIdentifiers(canonical) {
		if level := s.github.TrustLevel(candidate); level != "community" {
			return level
		}
	}
	return "community"
}

// Fetch retrieves a skill from skills.sh
func (s *SkillsShSource) Fetch(identifier string) (*SkillBundle, error) {
	canonical := s.normalizeIdentifier(identifier)

	// Fetch detail page from skills.sh
	detail, err := s.fetchDetailPage(canonical)
	if err != nil {
		// Continue without detail info
		detail = nil
	}

	// Try candidate identifiers
	for _, candidate := range s.candidateIdentifiers(canonical) {
		bundle, err := s.github.Fetch(candidate)
		if err == nil && bundle != nil {
			// Validate SKILL.md exists
			if _, ok := bundle.Files["SKILL.md"]; !ok {
				continue
			}
			// Update bundle with skills.sh info
			bundle.Source = "skills-sh"
			bundle.Identifier = s.wrapIdentifier(canonical)
			bundle.TrustLevel = s.TrustLevel(identifier)
			if detail != nil {
				bundle.Metadata = s.mergeDetailMetadata(bundle.Metadata, detail, canonical)
			}
			return bundle, nil
		}
	}

	// Try to discover identifier
	resolved, err := s.discoverIdentifier(canonical, detail)
	if err == nil && resolved != "" {
		bundle, err := s.github.Fetch(resolved)
		if err == nil && bundle != nil {
			// Validate SKILL.md exists
			if _, ok := bundle.Files["SKILL.md"]; !ok {
				return nil, fmt.Errorf("skill missing required SKILL.md file")
			}
			bundle.Source = "skills-sh"
			bundle.Identifier = s.wrapIdentifier(canonical)
			bundle.TrustLevel = s.TrustLevel(identifier)
			if detail != nil {
				bundle.Metadata = s.mergeDetailMetadata(bundle.Metadata, detail, canonical)
			}
			return bundle, nil
		}
	}

	return nil, fmt.Errorf("skill not found: %s", identifier)
}

// Inspect retrieves metadata from skills.sh
func (s *SkillsShSource) Inspect(identifier string) (*SkillMetadata, error) {
	canonical := s.normalizeIdentifier(identifier)

	// Fetch detail page
	detail, err := s.fetchDetailPage(canonical)
	if err != nil {
		detail = nil
	}

	// Try to get metadata from github
	meta, err := s.resolveGitHubMeta(canonical, detail)
	if err != nil {
		return nil, err
	}

	// Update with skills.sh info
	meta = s.finalizeInspectMeta(meta, canonical, detail)
	return meta, nil
}

// normalizeIdentifier removes skills.sh prefixes
func (s *SkillsShSource) normalizeIdentifier(identifier string) string {
	prefixes := []string{
		"skills-sh/",
		"skills.sh/",
		"skils-sh/",
		"skils.sh/",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(identifier, prefix) {
			return identifier[len(prefix):]
		}
	}
	return identifier
}

// wrapIdentifier adds skills-sh prefix
func (s *SkillsShSource) wrapIdentifier(identifier string) string {
	return "skills-sh/" + identifier
}

// candidateIdentifiers generates possible GitHub paths for a skill
func (s *SkillsShSource) candidateIdentifiers(identifier string) []string {
	parts := strings.SplitN(identifier, "/", 3)
	if len(parts) < 3 {
		return []string{identifier}
	}

	repo := parts[0] + "/" + parts[1]
	skillPath := strings.TrimPrefix(parts[2], "/")

	candidates := []string{
		fmt.Sprintf("github.com/%s/%s", repo, skillPath),
		fmt.Sprintf("github.com/%s/skills/%s", repo, skillPath),
		fmt.Sprintf("github.com/%s/.agents/skills/%s", repo, skillPath),
		fmt.Sprintf("github.com/%s/.claude/skills/%s", repo, skillPath),
	}

	// Deduplicate
	seen := make(map[string]bool)
	result := []string{}
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			result = append(result, c)
		}
	}
	return result
}

// fetchDetailPage fetches and parses skills.sh detail page
func (s *SkillsShSource) fetchDetailPage(identifier string) (*SkillsShDetail, error) {
	url := fmt.Sprintf("%s/%s", skillsShBaseURL, identifier)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch detail page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skills.sh returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return s.parseDetailPage(identifier, string(body)), nil
}

// parseDetailPage extracts information from skills.sh HTML
func (s *SkillsShSource) parseDetailPage(identifier, html string) *SkillsShDetail {
	parts := strings.SplitN(identifier, "/", 3)
	if len(parts) < 3 {
		return nil
	}

	defaultRepo := parts[0] + "/" + parts[1]
	skillToken := parts[2]
	repo := defaultRepo
	installSkill := skillToken

	// Extract install command
	installCmd := ""
	if match := skillsShInstallCmdRe.FindStringSubmatch(html); match != nil {
		installCmd = strings.TrimSpace(match[0])
		repoValue := strings.TrimSpace(s.extractGroup(match, "repo"))
		skillValue := strings.TrimSpace(s.extractGroup(match, "skill"))
		if skillValue != "" {
			installSkill = skillValue
		}
		if extracted := s.extractRepoSlug(repoValue); extracted != "" {
			repo = extracted
		}
	}

	return &SkillsShDetail{
		Repo:           repo,
		InstallSkill:   installSkill,
		PageTitle:      s.extractFirstMatch(skillsShPageH1Re, html),
		BodyTitle:      s.extractFirstMatch(skillsShProseH1Re, html),
		BodySummary:    s.extractFirstMatch(skillsShProsePRe, html),
		WeeklyInstalls: s.extractWeeklyInstalls(html),
		InstallCommand: installCmd,
		RepoURL:        fmt.Sprintf("https://github.com/%s", repo),
		DetailURL:      fmt.Sprintf("%s/%s", skillsShBaseURL, identifier),
	}
}

// discoverIdentifier tries to find the skill in non-standard locations
func (s *SkillsShSource) discoverIdentifier(identifier string, detail *SkillsShDetail) (string, error) {
	parts := strings.SplitN(identifier, "/", 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid identifier format")
	}

	defaultRepo := parts[0] + "/" + parts[1]
	repo := defaultRepo
	if detail != nil && detail.Repo != "" {
		repo = detail.Repo
	}

	skillToken := parts[2]
	tokens := []string{skillToken}
	if detail != nil {
		tokens = append(tokens, detail.InstallSkill, detail.PageTitle, detail.BodyTitle)
	}

	// Try standard skill paths
	basePaths := []string{"skills/", ".agents/skills/", ".claude/skills/"}
	for _, basePath := range basePaths {
		candidate := fmt.Sprintf("github.com/%s/%s%s", repo, basePath, skillToken)
		meta, err := s.github.Inspect(candidate)
		if err == nil && meta != nil {
			return candidate, nil
		}
	}

	// Try tree lookup for nested skills
	treeResult, err := s.findSkillInRepoTree(repo, skillToken)
	if err == nil && treeResult != "" {
		return treeResult, nil
	}

	// Scan repo root directories
	rootURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/", repo)
	req, err := http.NewRequest("GET", rootURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ragflow-cli")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.Type != "dir" {
			continue
		}
		if strings.HasPrefix(entry.Name, ".") || strings.HasPrefix(entry.Name, "_") {
			continue
		}
		if entry.Name == "skills" || entry.Name == ".agents" || entry.Name == ".claude" {
			continue // Already tried
		}

		// Try direct match
		directID := fmt.Sprintf("github.com/%s/%s/%s", repo, entry.Name, skillToken)
		meta, err := s.github.Inspect(directID)
		if err == nil && meta != nil {
			return directID, nil
		}
	}

	return "", fmt.Errorf("skill not found in repo")
}

// findSkillInRepoTree searches for skill in repo tree
func (s *SkillsShSource) findSkillInRepoTree(repo, skillToken string) (string, error) {
	// Get repo tree
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/HEAD?recursive=1", repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ragflow-cli")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var result struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Look for skill directories matching the token
	for _, item := range result.Tree {
		if item.Type != "tree" {
			continue
		}
		parts := strings.Split(item.Path, "/")
		if len(parts) == 0 {
			continue
		}
		dirName := parts[len(parts)-1]
		if s.matchesSkillToken(dirName, skillToken) {
			return fmt.Sprintf("github.com/%s/%s", repo, item.Path), nil
		}
	}

	return "", fmt.Errorf("skill not found in tree")
}

// matchesSkillToken checks if a directory name matches skill token
func (s *SkillsShSource) matchesSkillToken(dirName, skillToken string) bool {
	variants := s.tokenVariants(dirName)
	tokenVariants := s.tokenVariants(skillToken)
	for v := range tokenVariants {
		if variants[v] {
			return true
		}
	}
	return false
}

// tokenVariants generates normalized token variants
func (s *SkillsShSource) tokenVariants(value string) map[string]bool {
	variants := make(map[string]bool)
	if value == "" {
		return variants
	}

	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return variants
	}

	// Base name (last path component)
	parts := strings.Split(value, "/")
	base := parts[len(parts)-1]

	// Clean variant
	clean := strings.TrimPrefix(base, "@")

	variants[value] = true
	variants[strings.ReplaceAll(value, "_", "-")] = true
	variants[strings.ReplaceAll(value, "/", "-")] = true
	variants[base] = true
	variants[strings.ReplaceAll(base, "_", "-")] = true
	variants[clean] = true
	variants[strings.ReplaceAll(clean, "_", "-")] = true

	return variants
}

// resolveGitHubMeta tries to get metadata from GitHub
func (s *SkillsShSource) resolveGitHubMeta(identifier string, detail *SkillsShDetail) (*SkillMetadata, error) {
	for _, candidate := range s.candidateIdentifiers(identifier) {
		meta, err := s.github.Inspect(candidate)
		if err == nil && meta != nil {
			return meta, nil
		}
	}

	resolved, err := s.discoverIdentifier(identifier, detail)
	if err == nil && resolved != "" {
		return s.github.Inspect(resolved)
	}

	return nil, fmt.Errorf("skill metadata not found")
}

// finalizeInspectMeta updates metadata with skills.sh info
func (s *SkillsShSource) finalizeInspectMeta(meta *SkillMetadata, canonical string, detail *SkillsShDetail) *SkillMetadata {
	if meta == nil {
		meta = &SkillMetadata{}
	}

	meta = &SkillMetadata{
		Name:        meta.Name,
		Description: meta.Description,
		Version:     meta.Version,
		Author:      meta.Author,
		Tags:        meta.Tags,
		Tools:       meta.Tools,
	}

	// Use body summary as description if available
	if detail != nil && detail.BodySummary != "" {
		meta.Description = s.stripHTML(detail.BodySummary)
	} else if detail != nil && detail.WeeklyInstalls != "" && meta.Description != "" {
		meta.Description = fmt.Sprintf("%s · %s weekly installs on skills.sh", meta.Description, detail.WeeklyInstalls)
	}

	return meta
}

// mergeDetailMetadata merges skills.sh detail into bundle metadata
func (s *SkillsShSource) mergeDetailMetadata(meta *SkillMetadata, detail *SkillsShDetail, canonical string) *SkillMetadata {
	if meta == nil {
		meta = &SkillMetadata{}
	}

	// Create new metadata to avoid modifying the original
	merged := &SkillMetadata{
		Name:        meta.Name,
		Description: meta.Description,
		Version:     meta.Version,
		Author:      meta.Author,
		Tags:        meta.Tags,
		Tools:       meta.Tools,
	}

	if detail.BodySummary != "" {
		merged.Description = s.stripHTML(detail.BodySummary)
	}

	return merged
}

// extractFirstMatch extracts first matching group from regex
func (s *SkillsShSource) extractFirstMatch(re *regexp.Regexp, text string) string {
	match := re.FindStringSubmatch(text)
	if match == nil {
		return ""
	}
	for i, name := range re.SubexpNames() {
		if i > 0 && i < len(match) && name != "" {
			return s.stripHTML(strings.TrimSpace(match[i]))
		}
	}
	return ""
}

// extractGroup extracts a named group from regex match
func (s *SkillsShSource) extractGroup(match []string, name string) string {
	// This is a simplified version - in practice we'd need to track group indices
	return ""
}

// extractWeeklyInstalls extracts weekly install count
func (s *SkillsShSource) extractWeeklyInstalls(html string) string {
	match := skillsShWeeklyRe.FindStringSubmatch(html)
	if match == nil {
		return ""
	}
	for i, name := range skillsShWeeklyRe.SubexpNames() {
		if i > 0 && i < len(match) && name == "count" {
			return match[i]
		}
	}
	return ""
}

// extractRepoSlug extracts owner/repo from URL or string
func (s *SkillsShSource) extractRepoSlug(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "https://github.com/")
	value = strings.Trim(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return ""
}

// stripHTML removes HTML tags
func (s *SkillsShSource) stripHTML(value string) string {
	// Simple HTML tag removal
	re := regexp.MustCompile(`<[^>]+>`)
	return strings.TrimSpace(re.ReplaceAllString(value, ""))
}
