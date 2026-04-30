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
	stdctx "context"
)

// Provider is the interface for all context providers
// Each provider handles a specific resource type (datasets, chats, agents, etc.)
type Provider interface {
	// Name returns the provider name (e.g., "datasets", "chats")
	Name() string

	// Description returns a human-readable description of the provider
	Description() string

	// Supports returns true if this provider can handle the given path
	Supports(path string) bool

	// List lists nodes at the given path
	List(ctx stdctx.Context, path string, opts *ListOptions) (*Result, error)

	// Search searches for nodes matching the query under the given path
	Search(ctx stdctx.Context, path string, opts *SearchOptions) (*Result, error)

	// Cat retrieves the content of a file/document at the given path
	Cat(ctx stdctx.Context, path string) ([]byte, error)
}

// BaseProvider provides common functionality for all providers
type BaseProvider struct {
	name        string
	description string
	rootPath    string
}

// Name returns the provider name
func (p *BaseProvider) Name() string {
	return p.name
}

// Description returns the provider description
func (p *BaseProvider) Description() string {
	return p.description
}

// GetRootPath returns the root path for this provider
func (p *BaseProvider) GetRootPath() string {
	return p.rootPath
}

// IsRootPath checks if the given path is the root path for this provider
func (p *BaseProvider) IsRootPath(path string) bool {
	return normalizePath(path) == normalizePath(p.rootPath)
}

// ParsePath parses a path and returns the subpath relative to the provider root
func (p *BaseProvider) ParsePath(path string) string {
	normalized := normalizePath(path)
	rootNormalized := normalizePath(p.rootPath)

	if normalized == rootNormalized {
		return ""
	}

	if len(normalized) > len(rootNormalized) && normalized[:len(rootNormalized)+1] == rootNormalized+"/" {
		return normalized[len(rootNormalized)+1:]
	}

	return normalized
}

// SplitPath splits a path into components
func SplitPath(path string) []string {
	path = normalizePath(path)
	if path == "" {
		return []string{}
	}
	parts := splitString(path, '/')
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// normalizePath normalizes a path (removes leading/trailing slashes, handles "." and "..")
func normalizePath(path string) string {
	path = trimSpace(path)
	if path == "" {
		return ""
	}

	// Remove leading slashes
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	// Remove trailing slashes
	for len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	// Handle "." and ".."
	parts := splitString(path, '/')
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			// Skip empty and current directory
			continue
		case "..":
			// Go up one directory
			if len(result) > 0 {
				result = result[:len(result)-1]
			}
		default:
			result = append(result, part)
		}
	}

	return joinStrings(result, "/")
}

// Helper functions to avoid importing strings package in basic operations
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func splitString(s string, sep byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
