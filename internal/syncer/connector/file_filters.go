//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"fmt"
	"path"
	"strings"
)

// fileFilters contains source-independent exclusions. Connectors apply folder
// rules during traversal, before listing descendants, and file rules before
// fetching content. Exclusions must never narrow a prune snapshot.
type fileFilters struct {
	extensions  map[string]bool
	folderNames map[string]bool
	folderPaths map[string]bool
}

func newFileFilters(config map[string]any) (fileFilters, error) {
	var filters fileFilters
	fields := []struct {
		key    string
		target *map[string]bool
	}{
		{"excluded_extensions", &filters.extensions},
		{"excluded_folder_names", &filters.folderNames},
		{"excluded_folder_paths", &filters.folderPaths},
	}
	for _, field := range fields {
		values, err := filterStringList(config[field.key])
		if err != nil {
			return filters, fmt.Errorf("%s: %w", field.key, err)
		}
		*field.target = make(map[string]bool, len(values))
		for _, value := range values {
			normalized, err := normalizeFileFilter(field.key, value)
			if err != nil {
				return filters, err
			}
			(*field.target)[normalized] = true
		}
	}
	return filters, nil
}

func filterStringList(value any) ([]string, error) {
	switch values := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return values, nil
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("expected a list of strings")
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected a list of strings")
	}
}

func normalizeFileFilter(key, value string) (string, error) {
	value = strings.TrimSpace(value)
	if key == "excluded_extensions" {
		value = strings.ToLower(strings.TrimPrefix(value, "."))
	}
	if value == "" || strings.ContainsAny(value, "\\*?\x00\r\n") {
		return "", fmt.Errorf("%s contains an empty or invalid entry", key)
	}
	if key != "excluded_folder_paths" && strings.Contains(value, "/") {
		return "", fmt.Errorf("%s entries cannot contain a path", key)
	}
	if key == "excluded_folder_paths" {
		if strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
			return "", fmt.Errorf("excluded_folder_paths must contain clean relative paths")
		}
	}
	return value, nil
}

func (f fileFilters) excludesFile(name string) bool {
	name = strings.ToLower(name)
	for extension := range f.extensions {
		if strings.HasSuffix(name, "."+extension) {
			return true
		}
	}
	return false
}

// Folder names and paths match exactly, including case. Call this on each
// folder before descending, so an excluded folder excludes its entire subtree.
func (f fileFilters) excludesFolder(relativePath string) bool {
	return f.folderNames[path.Base(relativePath)] || f.folderPaths[relativePath]
}
