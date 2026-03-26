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

package contextengine

import (
	stdctx "context"
	"fmt"
	"strings"
	"time"
)

// Engine is the core of the Context Engine
// It manages providers and routes commands to the appropriate provider
type Engine struct {
	providers []Provider
}

// NewEngine creates a new Context Engine
func NewEngine() *Engine {
	return &Engine{
		providers: make([]Provider, 0),
	}
}

// RegisterProvider registers a provider with the engine
func (e *Engine) RegisterProvider(provider Provider) {
	e.providers = append(e.providers, provider)
}

// GetProviders returns all registered providers
func (e *Engine) GetProviders() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(e.providers))
	for _, p := range e.providers {
		infos = append(infos, ProviderInfo{
			Name:        p.Name(),
			Description: p.Description(),
		})
	}
	return infos
}

// Execute executes a command and returns the result
func (e *Engine) Execute(ctx stdctx.Context, cmd *Command) (*Result, error) {
	switch cmd.Type {
	case CommandList:
		return e.List(ctx, cmd.Path, parseListOptions(cmd.Params))
	case CommandSearch:
		return e.Search(ctx, cmd.Path, parseSearchOptions(cmd.Params))
	case CommandMkdir:
		_, err := e.Mkdir(ctx, cmd.Path, cmd.Params)
		return nil, err
	case CommandCat:
		_, err := e.Cat(ctx, cmd.Path)
		return nil, err
	case CommandRm:
		recursive := false
		if r, ok := cmd.Params["recursive"].(bool); ok {
			recursive = r
		}
		err := e.Rm(ctx, cmd.Path, recursive)
		return nil, err
	default:
		return nil, fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// resolveProvider finds the provider for a given path
func (e *Engine) resolveProvider(path string) (Provider, string, error) {
	path = normalizePath(path)

	for _, provider := range e.providers {
		if provider.Supports(path) {
			// Parse the subpath relative to the provider root
			// Get provider name to calculate subPath
			providerName := provider.Name()
			var subPath string
			if path == providerName {
				subPath = ""
			} else if strings.HasPrefix(path, providerName+"/") {
				subPath = path[len(providerName)+1:]
			} else {
				subPath = path
			}
			return provider, subPath, nil
		}
	}

	return nil, "", fmt.Errorf("%s: %s", ErrProviderNotFound, path)
}

// List lists nodes at the given path
// If path is empty, returns:
//   1. Built-in providers (e.g., datasets)
//   2. Top-level directories from files provider (if any)
func (e *Engine) List(ctx stdctx.Context, path string, opts *ListOptions) (*Result, error) {
	// If path is empty, return list of providers and files root directories
	if path == "" || path == "/" {
		return e.listRoot(ctx, opts)
	}

	provider, subPath, err := e.resolveProvider(path)
	if err != nil {
		// If not found, try to find in files provider as a fallback
		// This allows "ls myfolder" to work as "ls files/myskills"
		if fileProvider := e.getFileProvider(); fileProvider != nil {
			result, ferr := fileProvider.List(ctx, path, opts)
			if ferr == nil {
				return result, nil
			}
		}
		return nil, err
	}

	return provider.List(ctx, subPath, opts)
}

// listRoot returns the root listing:
// 1. Built-in providers (datasets, etc.)
// 2. Top-level directories from files provider
func (e *Engine) listRoot(ctx stdctx.Context, opts *ListOptions) (*Result, error) {
	nodes := make([]*Node, 0)

	// Add built-in providers first
	for _, p := range e.providers {
		// Skip files provider from this list - we'll add its children instead
		if p.Name() == "files" {
			continue
		}
		nodes = append(nodes, &Node{
			Name:      p.Name(),
			Path:      "/" + p.Name(),
			Type:      NodeTypeDirectory,
			CreatedAt: time.Now(),
			Metadata: map[string]interface{}{
				"description": p.Description(),
			},
		})
	}

	// Add top-level directories from files provider
	if fileProvider := e.getFileProvider(); fileProvider != nil {
		filesResult, err := fileProvider.List(ctx, "", opts)
		if err == nil {
			for _, node := range filesResult.Nodes {
				// Only add directories, not files
				if node.Type == NodeTypeDirectory {
					// Remove the /files/ prefix from path
					node.Path = strings.TrimPrefix(node.Path, "/files/")
					nodes = append(nodes, node)
				}
			}
		}
	}

	return &Result{
		Nodes: nodes,
		Total: len(nodes),
	}, nil
}

// getFileProvider returns the files provider if registered
func (e *Engine) getFileProvider() Provider {
	for _, p := range e.providers {
		if p.Name() == "files" {
			return p
		}
	}
	return nil
}

// Search searches for nodes matching the query
func (e *Engine) Search(ctx stdctx.Context, path string, opts *SearchOptions) (*Result, error) {
	provider, subPath, err := e.resolveProvider(path)
	if err != nil {
		return nil, err
	}

	return provider.Search(ctx, subPath, opts)
}

// Mkdir creates a new directory/node
func (e *Engine) Mkdir(ctx stdctx.Context, path string, params map[string]interface{}) (*Node, error) {
	provider, subPath, err := e.resolveProvider(path)
	if err != nil {
		return nil, err
	}

	return provider.Mkdir(ctx, subPath, params)
}

// Cat retrieves the content of a file/document
func (e *Engine) Cat(ctx stdctx.Context, path string) ([]byte, error) {
	provider, subPath, err := e.resolveProvider(path)
	if err != nil {
		// If not found, try to find in files provider as a fallback
		// This allows "cat myfolder/file.txt" to work as "cat files/myfolder/file.txt"
		if fileProvider := e.getFileProvider(); fileProvider != nil {
			return fileProvider.Cat(ctx, path)
		}
		return nil, err
	}

	return provider.Cat(ctx, subPath)
}

// Rm removes a resource
func (e *Engine) Rm(ctx stdctx.Context, path string, recursive bool) error {
	provider, subPath, err := e.resolveProvider(path)
	if err != nil {
		return err
	}

	return provider.Rm(ctx, subPath, recursive)
}

// ParsePath parses a path and returns path information
func (e *Engine) ParsePath(path string) (*PathInfo, error) {
	path = normalizePath(path)
	components := SplitPath(path)

	if len(components) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	providerName := components[0]
	isRoot := len(components) == 1

	// Find the provider
	var provider Provider
	for _, p := range e.providers {
		if p.Name() == providerName || strings.HasPrefix(path, p.Name()) {
			provider = p
			break
		}
	}

	if provider == nil {
		return nil, fmt.Errorf("%s: %s", ErrProviderNotFound, path)
	}

	info := &PathInfo{
		Provider:   providerName,
		Path:       path,
		Components: components,
		IsRoot:     isRoot,
	}

	// Extract resource ID or name if available
	if len(components) >= 2 {
		info.ResourceName = components[1]
	}

	return info, nil
}

// parseListOptions parses command params into ListOptions
func parseListOptions(params map[string]interface{}) *ListOptions {
	opts := &ListOptions{}

	if params == nil {
		return opts
	}

	if recursive, ok := params["recursive"].(bool); ok {
		opts.Recursive = recursive
	}
	if limit, ok := params["limit"].(int); ok {
		opts.Limit = limit
	}
	if offset, ok := params["offset"].(int); ok {
		opts.Offset = offset
	}
	if sortBy, ok := params["sort_by"].(string); ok {
		opts.SortBy = sortBy
	}
	if sortOrder, ok := params["sort_order"].(string); ok {
		opts.SortOrder = sortOrder
	}

	return opts
}

// parseSearchOptions parses command params into SearchOptions
func parseSearchOptions(params map[string]interface{}) *SearchOptions {
	opts := &SearchOptions{}

	if params == nil {
		return opts
	}

	if query, ok := params["query"].(string); ok {
		opts.Query = query
	}
	if limit, ok := params["limit"].(int); ok {
		opts.Limit = limit
	}
	if offset, ok := params["offset"].(int); ok {
		opts.Offset = offset
	}
	if recursive, ok := params["recursive"].(bool); ok {
		opts.Recursive = recursive
	}
	if topK, ok := params["top_k"].(int); ok {
		opts.TopK = topK
	}
	if threshold, ok := params["threshold"].(float64); ok {
		opts.Threshold = threshold
	}
	if dirs, ok := params["dirs"].([]string); ok {
		opts.Dirs = dirs
	}

	return opts
}
