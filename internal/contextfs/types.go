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

package contextfs

import "time"

// NodeType represents the type of a filesystem node
type NodeType string

const (
	NodeTypeRoot     NodeType = "root"
	NodeTypeDataset  NodeType = "dataset"
	NodeTypeDocument NodeType = "document"
	NodeTypeTool     NodeType = "tool"
	NodeTypeSkill    NodeType = "skill"
	NodeTypeMemory   NodeType = "memory"
	NodeTypeDir      NodeType = "dir"
	NodeTypeFile     NodeType = "file"
)

// Node represents a node in the context filesystem
// Only includes name, path, created_at, updated_at fields for ls output
type Node struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListRequest represents a request to list directory contents
type ListRequest struct {
	Path   string `json:"path" form:"path"`
	UserID string `json:"user_id"`
}

// ListResponse represents the response for listing directory contents
type ListResponse struct {
	Code    int     `json:"code"`
	Data    []*Node `json:"data"`
	Message string  `json:"message"`
}

// SearchOptions represents search options
type SearchOptions struct {
	Query    string   `json:"query"`
	Path     string   `json:"path"`
	UserID   string   `json:"user_id"`
	Type     NodeType `json:"type,omitempty"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}

// SearchResult represents a search result
type SearchResult struct {
	Node        *Node   `json:"node"`
	Score       float64 `json:"score"`
	Highlighted string  `json:"highlighted,omitempty"`
}

// SearchResponse represents the response for search
type SearchResponse struct {
	Code    int             `json:"code"`
	Data    []*SearchResult `json:"data"`
	Total   int             `json:"total"`
	Message string          `json:"message"`
}
