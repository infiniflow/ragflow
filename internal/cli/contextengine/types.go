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

package context

import "time"

// NodeType represents the type of a node in the context filesystem
type NodeType string

const (
	NodeTypeDirectory NodeType = "directory"
	NodeTypeFile      NodeType = "file"
	NodeTypeDataset   NodeType = "dataset"
	NodeTypeDocument  NodeType = "document"
	NodeTypeChat      NodeType = "chat"
	NodeTypeAgent     NodeType = "agent"
	NodeTypeUnknown   NodeType = "unknown"
)

// Node represents a node in the context filesystem
// This is the unified output format for all providers
type Node struct {
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	Type       NodeType               `json:"type"`
	Size       int64                  `json:"size,omitempty"`
	CreatedAt  time.Time              `json:"created_at,omitempty"`
	UpdatedAt  time.Time              `json:"updated_at,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// CommandType represents the type of command
type CommandType string

const (
	CommandList   CommandType = "ls"
	CommandSearch CommandType = "search"
	CommandMkdir  CommandType = "mkdir"
	CommandGet    CommandType = "get"
	CommandCat    CommandType = "cat"
	CommandPut    CommandType = "put"
	CommandRm     CommandType = "rm"
)

// Command represents a context engine command
type Command struct {
	Type   CommandType            `json:"type"`
	Path   string                 `json:"path"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// ListOptions represents options for list operations
type ListOptions struct {
	Recursive bool   `json:"recursive,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	SortBy    string `json:"sort_by,omitempty"`
	SortOrder string `json:"sort_order,omitempty"` // "asc" or "desc"
}

// SearchOptions represents options for search operations
type SearchOptions struct {
	Query     string `json:"query"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

// Result represents the result of a command execution
type Result struct {
	Nodes      []*Node `json:"nodes"`
	Total      int     `json:"total"`
	HasMore    bool    `json:"has_more"`
	NextOffset int     `json:"next_offset,omitempty"`
	Error      error   `json:"-"`
}

// PathInfo represents parsed path information
type PathInfo struct {
	Provider    string   // The provider name (e.g., "datasets", "chats")
	Path        string   // The full path
	Components  []string // Path components
	IsRoot      bool     // Whether this is the root path for the provider
	ResourceID  string   // Resource ID if applicable
	ResourceName string // Resource name if applicable
}

// ProviderInfo holds metadata about a provider
type ProviderInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RootPath    string `json:"root_path"`
}

// Common error messages
const (
	ErrInvalidPath     = "invalid path"
	ErrProviderNotFound = "provider not found for path"
	ErrNotSupported    = "operation not supported"
	ErrNotFound        = "resource not found"
	ErrUnauthorized    = "unauthorized"
	ErrInternal        = "internal error"
)
