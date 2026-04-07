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

package cli

import (
	"context"
	"fmt"
	"strings"

	ce "ragflow/internal/cli/filesystem"
)

func (c *RAGFlowClient) ContextList(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var path string
	var ok bool
	if cmd.Params["path"] != nil {
		path, ok = cmd.Params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("fail to convert 'path' to string")
		}
	}

	if path == "" {
		path = "."
	}

	var parameter string
	if cmd.Params["parameter"] != nil {
		parameter, ok = cmd.Params["parameter"].(string)
		if !ok {
			return nil, fmt.Errorf("fail to convert 'parameter' to string")
		}
	}

	if parameter == "" {
		fmt.Printf("ls %s\n", path)
	} else {
		fmt.Printf("ls %s -%s\n", path, parameter)
	}

	// Convert to response
	var response ContextListResponse
	response.OutputFormat = c.OutputFormat
	response.Code = 0
	response.Data = nil

	return &response, nil
}

func (c *RAGFlowClient) ContextCat(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	path, ok := cmd.Params["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("fail to convert 'filename' to string")
	}

	fmt.Printf("cat %s\n", path)

	// Convert to response
	var response ContextListResponse
	response.OutputFormat = c.OutputFormat
	response.Code = 0
	response.Data = nil

	return &response, nil
}

func (c *RAGFlowClient) ContextSearch(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	path, ok := cmd.Params["path"].(string)
	if !ok || path == "" {
		path = "datasets"
	}

	query, ok := cmd.Params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	number := 10
	if cmd.Params["number"] != nil {
		number, ok = cmd.Params["number"].(int)
		if !ok {
			return nil, fmt.Errorf("fail to convert 'number' to int")
		}
	}

	fmt.Printf("search query: %s, path: %s, number: %d\n", query, path, number)

	// Check if searching skills
	if path == "skills" || strings.HasPrefix(path, "skills/") {
		// Parse hub ID from path
		hubID := "default"
		if strings.HasPrefix(path, "skills/") {
			hubID = strings.TrimPrefix(path, "skills/")
			if hubID == "" {
				hubID = "default"
			}
		}

		// Get skill provider and perform search
		provider := c.ContextEngine.GetProvider("skills")
		if provider == nil {
			return nil, fmt.Errorf("skill provider not available")
		}
		skillProvider, ok := provider.(*ce.SkillProvider)
		if !ok {
			return nil, fmt.Errorf("invalid skill provider type")
		}

		searchOptions := &ce.SearchOptions{
			Query:  query,
			Limit:  number,
			Offset: 0,
			TopK:   number,
		}
		result, err := skillProvider.Search(context.Background(), hubID, searchOptions)
		if err != nil {
			return nil, err
		}

		// Convert to response
		var response ContextSearchResponse
		response.OutputFormat = c.OutputFormat
		response.Code = 0
		response.Total = result.Total
		response.Data = ce.FormatNodes(result.Nodes, string(c.OutputFormat))
		return &response, nil
	}

	// For dataset search, use ContextEngine
	opts := &ce.SearchOptions{
		Query: query,
		Limit: number,
	}

	result, err := c.ContextEngine.Search(context.Background(), path, opts)
	if err != nil {
		return nil, err
	}

	// Convert to response
	var response ContextSearchResponse
	response.OutputFormat = c.OutputFormat
	response.Code = 0
	response.Total = result.Total
	response.Data = ce.FormatNodes(result.Nodes, string(c.OutputFormat))

	return &response, nil
}
