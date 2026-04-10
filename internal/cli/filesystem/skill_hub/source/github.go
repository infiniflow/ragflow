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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// GitHubSource handles GitHub repository skills
type GitHubSource struct {
	client HTTPClientInterface
}

// NewGitHubSource creates a new GitHub source adapter
func NewGitHubSource(client HTTPClientInterface) *GitHubSource {
	return &GitHubSource{client: client}
}

// SourceID returns the source identifier
func (s *GitHubSource) SourceID() string {
	return "github"
}

// TrustLevel returns the trust level based on repository
func (s *GitHubSource) TrustLevel(identifier string) string {
	owner, repo, _, err := parseGitHubURL(identifier)
	if err != nil {
		return "community"
	}
	if isTrustedGitHubRepo(owner, repo) {
		return "trusted"
	}
	return "community"
}

// Fetch retrieves a skill from GitHub
func (s *GitHubSource) Fetch(identifier string) (*SkillBundle, error) {
	owner, repo, pathStr, err := parseGitHubURL(identifier)
	if err != nil {
		return nil, err
	}

	// Default to repo root if no path specified
	if pathStr == "" {
		pathStr = "."
	}

	// Try to get SKILL.md first to determine skill name
	skillName := repo
	meta := &SkillMetadata{Version: "1.0.0"}

	skillMdContent, err := s.fetchFileContent(owner, repo, path.Join(pathStr, "SKILL.md"))
	if err == nil {
		meta = parseSkillFrontmatter(skillMdContent)
		if meta.Name != "" {
			skillName = meta.Name
		}
	}

	// Fetch all files in the directory
	files, err := s.fetchDirectoryContents(owner, repo, pathStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch directory contents: %w", err)
	}

	return &SkillBundle{
		Name:       skillName,
		Files:      files,
		Source:     "github",
		Identifier: identifier,
		TrustLevel: s.TrustLevel(identifier),
		Metadata:   meta,
	}, nil
}

// Inspect retrieves metadata from GitHub
func (s *GitHubSource) Inspect(identifier string) (*SkillMetadata, error) {
	owner, repo, pathStr, err := parseGitHubURL(identifier)
	if err != nil {
		return nil, err
	}

	skillMdPath := path.Join(pathStr, "SKILL.md")
	content, err := s.fetchFileContent(owner, repo, skillMdPath)
	if err != nil {
		// Return basic metadata if SKILL.md not found
		return &SkillMetadata{
			Name:        repo,
			Description: fmt.Sprintf("Skill from %s/%s", owner, repo),
			Version:     "1.0.0",
		}, nil
	}

	return parseSkillFrontmatter(content), nil
}

// fetchFileContent fetches a single file from GitHub
func (s *GitHubSource) fetchFileContent(owner, repo, filePath string) (string, error) {
	var url string
	if filePath == "" || filePath == "." {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents", owner, repo)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, filePath)
	}

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
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var result struct {
		Content string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(result.Content)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	return result.Content, nil
}

// fetchDirectoryContents recursively fetches directory contents from GitHub
func (s *GitHubSource) fetchDirectoryContents(owner, repo, dirPath string) (map[string][]byte, error) {
	var url string
	if dirPath == "" || dirPath == "." {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents", owner, repo)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, dirPath)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ragflow-cli")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var items []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	files := make(map[string][]byte)
	for _, item := range items {
		// Skip hidden files and common ignore patterns
		if strings.HasPrefix(item.Name, ".") {
			continue
		}
		if item.Name == "node_modules" || item.Name == "__pycache__" {
			continue
		}

		if item.Type == "file" {
			// Calculate relative path
			relPath := item.Path
			if dirPath != "" && dirPath != "." {
				relPath = strings.TrimPrefix(item.Path, dirPath+"/")
			}

			content, err := s.downloadFile(item.DownloadURL)
			if err != nil {
				continue // Skip files we can't download
			}
			files[relPath] = content
		} else if item.Type == "dir" {
			// Recursively fetch subdirectory
			subFiles, err := s.fetchDirectoryContents(owner, repo, item.Path)
			if err != nil {
				continue
			}
			for subPath, content := range subFiles {
				relPath := subPath
				if dirPath != "" && dirPath != "." {
					relPath = strings.TrimPrefix(subPath, dirPath+"/")
				}
				files[relPath] = content
			}
		}
	}

	return files, nil
}

// downloadFile downloads a file from the given URL
func (s *GitHubSource) downloadFile(url string) ([]byte, error) {
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
