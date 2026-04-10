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
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LocalSource handles local filesystem skills
type LocalSource struct{}

// NewLocalSource creates a new local source adapter
func NewLocalSource() *LocalSource {
	return &LocalSource{}
}

// SourceID returns the source identifier
func (s *LocalSource) SourceID() string {
	return "local"
}

// TrustLevel returns the trust level for local sources
func (s *LocalSource) TrustLevel(identifier string) string {
	return "community" // Local skills default to community trust level
}

// Fetch retrieves a skill from the local filesystem
func (s *LocalSource) Fetch(identifier string) (*SkillBundle, error) {
	// Validate path exists
	info, err := os.Stat(identifier)
	if err != nil {
		return nil, fmt.Errorf("cannot access path %s: %w", identifier, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", identifier)
	}

	// Read SKILL.md
	skillMdPath := filepath.Join(identifier, "SKILL.md")
	content, err := os.ReadFile(skillMdPath)
	if err != nil {
		return nil, fmt.Errorf("SKILL.md not found in %s: %w", identifier, err)
	}

	// Parse frontmatter
	meta := parseSkillFrontmatter(string(content))
	skillName := meta.Name
	if skillName == "" {
		skillName = filepath.Base(identifier)
	}

	// Collect all files
	files := make(map[string][]byte)
	ignorePatterns := []string{
		".git/", ".svn/", ".hg/", "node_modules/", "__MACOSX/",
		".DS_Store", "._*", "*.log", "*.tmp", "*.temp", "*.swp", "*.swo", "*~",
		".env", ".env.*", ".vscode/", ".idea/", "Thumbs.db", "desktop.ini",
	}

	err = filepath.Walk(identifier, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(identifier, path)

		// Check ignore patterns
		for _, pattern := range ignorePatterns {
			if matched, _ := filepath.Match(pattern, relPath); matched {
				return nil
			}
			if strings.Contains(relPath, pattern) {
				return nil
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[relPath] = data
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &SkillBundle{
		Name:       skillName,
		Files:      files,
		Source:     "local",
		Identifier: identifier,
		TrustLevel: s.TrustLevel(identifier),
		Metadata:   meta,
	}, nil
}

// Inspect retrieves metadata without reading all files
func (s *LocalSource) Inspect(identifier string) (*SkillMetadata, error) {
	info, err := os.Stat(identifier)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory")
	}

	skillMdPath := filepath.Join(identifier, "SKILL.md")
	content, err := os.ReadFile(skillMdPath)
	if err != nil {
		return nil, err
	}

	meta := parseSkillFrontmatter(string(content))
	if meta.Name == "" {
		meta.Name = filepath.Base(identifier)
	}

	return meta, nil
}

// parseSkillFrontmatter extracts YAML frontmatter from SKILL.md content
func parseSkillFrontmatter(content string) *SkillMetadata {
	meta := &SkillMetadata{
		Version: "1.0.0",
	}

	// Look for YAML frontmatter
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return meta
	}

	// Find end of frontmatter
	endIdx := strings.Index(content[3:], "---")
	if endIdx == -1 {
		return meta
	}

	frontmatter := content[3 : endIdx+3]
	yaml.Unmarshal([]byte(frontmatter), meta)

	return meta
}

// isTextFile checks if a file is a text file based on extension
func isTextFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" && ext[0] == '.' {
		ext = ext[1:]
	}

	textExts := map[string]bool{
		"md": true, "mdx": true, "txt": true, "json": true, "json5": true,
		"yaml": true, "yml": true, "toml": true, "js": true, "cjs": true, "mjs": true,
		"ts": true, "tsx": true, "jsx": true, "py": true, "sh": true, "rb": true,
		"go": true, "rs": true, "swift": true, "kt": true, "java": true, "cs": true,
		"cpp": true, "c": true, "h": true, "hpp": true, "sql": true, "csv": true,
		"ini": true, "cfg": true, "env": true, "xml": true, "html": true,
		"css": true, "scss": true, "sass": true, "svg": true,
	}

	return textExts[ext]
}
