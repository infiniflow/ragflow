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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SkillSource is the interface for skill sources
type SkillSource interface {
	// SourceID returns the source identifier (local, github, clawhub, skillssh)
	SourceID() string

	// Fetch downloads and returns the skill bundle
	Fetch(identifier string) (*SkillBundle, error)

	// Inspect retrieves metadata without downloading full content
	Inspect(identifier string) (*SkillMetadata, error)

	// TrustLevel returns the trust level for this source (builtin/trusted/community)
	TrustLevel(identifier string) string
}

// SourceResolver resolves source references to appropriate adapters
type SourceResolver struct {
	sources map[string]SkillSource
}

// NewSourceResolver creates a new source resolver
func NewSourceResolver(client HTTPClientInterface) *SourceResolver {
	return &SourceResolver{
		sources: map[string]SkillSource{
			"local":    NewLocalSource(),
			"github":   NewGitHubSource(client),
			"clawhub":  NewClawHubSource(client),
			"skillssh": NewSkillsShSource(client),
		},
	}
}

// Resolve parses a source reference and returns the appropriate source adapter
// Supported formats:
//   - ./path, /absolute/path -> local
//   - github.com/owner/repo/path -> github
//   - clawhub://owner/skill-name, clawhub.ai/owner/skill-name -> clawhub
//   - skill://skill-name, skills.sh/skill/name -> skillssh
func (r *SourceResolver) Resolve(ref string) (SkillSource, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", fmt.Errorf("empty source reference")
	}

	// Check for URI schemes
	if strings.HasPrefix(ref, "clawhub://") {
		identifier := strings.TrimPrefix(ref, "clawhub://")
		return r.sources["clawhub"], identifier, nil
	}
	if strings.HasPrefix(ref, "skill://") {
		identifier := strings.TrimPrefix(ref, "skill://")
		return r.sources["skillssh"], identifier, nil
	}

	// Check for local path (starts with ./ or / or ~)
	if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "~/") {
		// Expand ~ to home directory
		if strings.HasPrefix(ref, "~/") {
			home, err := getHomeDir()
			if err != nil {
				return nil, "", fmt.Errorf("cannot resolve home directory: %w", err)
			}
			ref = filepath.Join(home, ref[2:])
		}
		return r.sources["local"], ref, nil
	}

	// Check for github.com domain
	if strings.HasPrefix(ref, "github.com/") || strings.HasPrefix(ref, "https://github.com/") {
		identifier := strings.TrimPrefix(ref, "https://")
		return r.sources["github"], identifier, nil
	}

	// Check for clawhub.ai domain
	if strings.HasPrefix(ref, "clawhub.ai/") || strings.HasPrefix(ref, "https://clawhub.ai/") {
		identifier := strings.TrimPrefix(ref, "https://")
		identifier = strings.TrimPrefix(identifier, "clawhub.ai/")
		return r.sources["clawhub"], identifier, nil
	}

	// Check for skills.sh domain
	if strings.HasPrefix(ref, "skills.sh/") || strings.HasPrefix(ref, "https://skills.sh/") {
		identifier := strings.TrimPrefix(ref, "https://")
		identifier = strings.TrimPrefix(identifier, "skills.sh/")
		return r.sources["skillssh"], identifier, nil
	}

	// Default: treat as local path if it exists, otherwise error
	return r.sources["local"], ref, nil
}

// getHomeDir returns the user's home directory
func getHomeDir() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", fmt.Errorf("cannot determine home directory")
	}
	return home, nil
}

// parseGitHubURL parses a GitHub URL and returns owner, repo, and path
func parseGitHubURL(urlStr string) (owner, repo, path string, err error) {
	// Remove protocol prefix if present
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")

	// Remove github.com/ prefix
	urlStr = strings.TrimPrefix(urlStr, "github.com/")

	parts := strings.Split(urlStr, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid GitHub URL format")
	}

	owner = parts[0]
	repo = parts[1]
	if len(parts) > 2 {
		path = strings.Join(parts[2:], "/")
	}

	return owner, repo, path, nil
}

// extractSkillNameFromPath extracts the skill name from a path
func extractSkillNameFromPath(path string) string {
	base := filepath.Base(path)
	// Remove common suffixes
	base = strings.TrimSuffix(base, ".git")
	return base
}

// isTrustedGitHubRepo checks if a GitHub repo is trusted
func isTrustedGitHubRepo(owner, repo string) bool {
	fullName := owner + "/" + repo
	trusted := map[string]bool{
		"openai/skills":     true,
		"anthropics/skills": true,
		"microsoft/skills":  true,
		"google/skills":     true,
	}
	return trusted[fullName]
}

// Helper to check if URL is valid
func isValidURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
