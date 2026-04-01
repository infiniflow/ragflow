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
	"encoding/json"
	"fmt"
	ce "ragflow/internal/cli/contextengine"
	"strings"
)

// PingServer pings the server to check if it's alive
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *RAGFlowClient) PingServer(cmd *Command) (ResponseIf, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.HTTPClient.RequestWithIterations("GET", "/system/ping", false, "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.HTTPClient.Request("GET", "/system/ping", false, "web", nil, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Server is down")
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ping: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	result.Message = string(resp.Body)
	result.Code = 0
	return &result, nil
}

// Show server version to show RAGFlow server version
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *RAGFlowClient) ShowServerVersion(cmd *Command) (ResponseIf, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.HTTPClient.RequestWithIterations("GET", "/system/version", false, "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.HTTPClient.Request("GET", "/system/version", false, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show version: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show version: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result KeyValueResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show version failed: invalid JSON (%w)", err)
	}
	result.Key = "version"
	result.Duration = resp.Duration

	return &result, nil
}

func (c *RAGFlowClient) RegisterUser(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	// Check for benchmark iterations
	var ok bool
	_, ok = cmd.Params["iterations"].(int)
	if ok {
		return nil, fmt.Errorf("failed to register user in benchmark statement")
	}

	var email string
	email, ok = cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("no email")
	}

	var password string
	password, ok = cmd.Params["password"].(string)
	if !ok {
		return nil, fmt.Errorf("no password")
	}

	var nickname string
	nickname, ok = cmd.Params["nickname"].(string)
	if !ok {
		return nil, fmt.Errorf("no nickname")
	}

	payload := map[string]interface{}{
		"email":    email,
		"password": password,
		"nickname": nickname,
	}

	resp, err := c.HTTPClient.Request("POST", "/user/register", false, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to register user: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to register user: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result RegisterResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("register user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// ListUserDatasets lists datasets for current user (user mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListUserDatasets(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	// Determine auth kind based on whether API token is being used
	authKind := "web"
	if c.HTTPClient.useAPIToken {
		authKind = "api"
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", "/datasets", true, authKind, nil, nil, iterations)
	}

	// Normal mode
	resp, err := c.HTTPClient.Request("GET", "/datasets", true, authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// getDatasetID gets dataset ID by name
func (c *RAGFlowClient) getDatasetID(datasetName string) (string, error) {
	resp, err := c.HTTPClient.Request("POST", "/kb/list", false, "web", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to list datasets: HTTP %d", resp.StatusCode)
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return "", fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok || code != 0 {
		msg, _ := resJSON["message"].(string)
		return "", fmt.Errorf("failed to list datasets: %s", msg)
	}

	data, ok := resJSON["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	kbs, ok := data["kbs"].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format: kbs not found")
	}

	for _, kb := range kbs {
		if kbMap, ok := kb.(map[string]interface{}); ok {
			if name, _ := kbMap["name"].(string); name == datasetName {
				if id, _ := kbMap["id"].(string); id != "" {
					return id, nil
				}
			}
		}
	}

	return "", fmt.Errorf("dataset '%s' not found", datasetName)
}

// formatEmptyArray converts empty arrays to "[]" string
func formatEmptyArray(v interface{}) string {
	if v == nil {
		return "[]"
	}
	switch val := v.(type) {
	case []interface{}:
		if len(val) == 0 {
			return "[]"
		}
	case []string:
		if len(val) == 0 {
			return "[]"
		}
	case []int:
		if len(val) == 0 {
			return "[]"
		}
	}
	return fmt.Sprintf("%v", v)
}

// SearchOnDatasets searches for chunks in specified datasets
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) SearchOnDatasets(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	question, ok := cmd.Params["question"].(string)
	if !ok {
		return nil, fmt.Errorf("question not provided")
	}

	datasets, ok := cmd.Params["datasets"].(string)
	if !ok {
		return nil, fmt.Errorf("datasets not provided")
	}

	// Parse dataset names (comma-separated) and convert to IDs
	datasetNames := strings.Split(datasets, ",")
	datasetIDs := make([]string, 0, len(datasetNames))
	for _, name := range datasetNames {
		name = strings.TrimSpace(name)
		id, err := c.getDatasetID(name)
		if err != nil {
			return nil, err
		}
		datasetIDs = append(datasetIDs, id)
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	payload := map[string]interface{}{
		"kb_id":                    datasetIDs,
		"question":                 question,
		"similarity_threshold":     0.2,
		"vector_similarity_weight": 0.3,
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("POST", "/chunk/retrieval_test", false, "web", nil, payload, iterations)
	}

	// Normal mode
	resp, err := c.HTTPClient.Request("POST", "/chunk/retrieval_test", false, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to search on datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to search on datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok || code != 0 {
		msg, _ := resJSON["message"].(string)
		return nil, fmt.Errorf("failed to search on datasets: %s", msg)
	}

	data, ok := resJSON["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	chunks, ok := data["chunks"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: chunks not found")
	}

	// Convert to slice of maps for printing
	tableData := make([]map[string]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		if chunkMap, ok := chunk.(map[string]interface{}); ok {
			row := map[string]interface{}{
				"id":                chunkMap["chunk_id"],
				"content":           chunkMap["content_with_weight"],
				"document_id":       chunkMap["doc_id"],
				"dataset_id":        chunkMap["kb_id"],
				"docnm_kwd":         chunkMap["docnm_kwd"],
				"image_id":          chunkMap["image_id"],
				"similarity":        chunkMap["similarity"],
				"term_similarity":   chunkMap["term_similarity"],
				"vector_similarity": chunkMap["vector_similarity"],
			}
			// Add optional fields that may be empty arrays
			if v, ok := chunkMap["doc_type_kwd"]; ok {
				row["doc_type_kwd"] = formatEmptyArray(v)
			}
			if v, ok := chunkMap["important_kwd"]; ok {
				row["important_kwd"] = formatEmptyArray(v)
			}
			if v, ok := chunkMap["mom_id"]; ok {
				row["mom_id"] = formatEmptyArray(v)
			}
			if v, ok := chunkMap["positions"]; ok {
				row["positions"] = formatEmptyArray(v)
			}
			if v, ok := chunkMap["content_ltks"]; ok {
				row["content_ltks"] = v
			}
			tableData = append(tableData, row)
		}
	}

	PrintTableSimple(tableData)
	return nil, nil
}

// CreateToken creates a new API token
func (c *RAGFlowClient) CreateToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("POST", "/tokens", true, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create token: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var createResult CommonDataResponse
	if err = json.Unmarshal(resp.Body, &createResult); err != nil {
		return nil, fmt.Errorf("create token failed: invalid JSON (%w)", err)
	}

	if createResult.Code != 0 {
		return nil, fmt.Errorf("%s", createResult.Message)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "Token created successfully"
	result.Duration = resp.Duration
	return &result, nil
}

// ListTokens lists all API tokens for the current user
func (c *RAGFlowClient) ListTokens(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("GET", "/tokens", true, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list tokens: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list tokens failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// DropToken deletes an API token
func (c *RAGFlowClient) DropToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	token, ok := cmd.Params["token"].(string)
	if !ok {
		return nil, fmt.Errorf("token not provided")
	}

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/tokens/%s", token), true, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop token: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop token: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop token failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// SetToken sets the API token after validating it
func (c *RAGFlowClient) SetToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	token, ok := cmd.Params["token"].(string)
	if !ok {
		return nil, fmt.Errorf("token not provided")
	}

	// Save current token to restore if validation fails
	savedToken := c.HTTPClient.APIToken
	savedUseAPIToken := c.HTTPClient.useAPIToken

	// Set the new token temporarily for validation
	c.HTTPClient.APIToken = token
	c.HTTPClient.useAPIToken = true

	// Validate token by calling list tokens API
	resp, err := c.HTTPClient.Request("GET", "/tokens", true, "api", nil, nil)
	if err != nil {
		// Restore original token on error
		c.HTTPClient.APIToken = savedToken
		c.HTTPClient.useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if resp.StatusCode != 200 {
		// Restore original token on error
		c.HTTPClient.APIToken = savedToken
		c.HTTPClient.useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		// Restore original token on error
		c.HTTPClient.APIToken = savedToken
		c.HTTPClient.useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		// Restore original token on error
		c.HTTPClient.APIToken = savedToken
		c.HTTPClient.useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: %s", result.Message)
	}

	// Token is valid, keep it set
	var successResult SimpleResponse
	successResult.Code = 0
	successResult.Message = "API token set successfully"
	successResult.Duration = resp.Duration
	return &successResult, nil
}

// ShowToken displays the current API token
func (c *RAGFlowClient) ShowToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.HTTPClient.APIToken == "" {
		return nil, fmt.Errorf("no API token is currently set")
	}

	//fmt.Printf("Token: %s\n", c.HTTPClient.APIToken)

	var result CommonResponse
	result.Code = 0
	result.Message = ""
	result.Data = []map[string]interface{}{
		{
			"token": c.HTTPClient.APIToken,
		},
	}
	result.Duration = 0
	return &result, nil
}

// UnsetToken removes the current API token
func (c *RAGFlowClient) UnsetToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.HTTPClient.APIToken == "" {
		return nil, fmt.Errorf("no API token is currently set")
	}

	c.HTTPClient.APIToken = ""
	c.HTTPClient.useAPIToken = false

	var result SimpleResponse
	result.Code = 0
	result.Message = "API token unset successfully"
	result.Duration = 0
	return &result, nil
}

// CreateIndex creates an index for a dataset
func (c *RAGFlowClient) CreateIndex(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	vectorSize, ok := cmd.Params["vector_size"].(int)
	if !ok {
		return nil, fmt.Errorf("vector_size not provided")
	}

	// Get dataset ID by name
	datasetID, err := c.getDatasetID(datasetName)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"kb_id":       datasetID,
		"vector_size": vectorSize,
	}

	resp, err := c.HTTPClient.Request("POST", "/kb/index", false, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create index: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid response format: code is not a number")
	}

	var result SimpleResponse
	result.Code = int(code)
	if result.Code == 0 {
		result.Message = fmt.Sprintf("Success to create index for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to create index: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// DropIndex drops an index for a dataset
func (c *RAGFlowClient) DropIndex(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	// Get dataset ID by name
	datasetID, err := c.getDatasetID(datasetName)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"kb_id": datasetID,
	}

	resp, err := c.HTTPClient.Request("DELETE", "/kb/index", false, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop index: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop index: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid response format: code is not a number")
	}

	var result SimpleResponse
	result.Code = int(code)
	if result.Code == 0 {
		result.Message = fmt.Sprintf("Success to drop index for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to drop index: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// CreateDocMetaIndex creates the document metadata index for the tenant
func (c *RAGFlowClient) CreateDocMetaIndex(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("POST", "/tenant/doc_meta_index", false, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create doc meta index: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create doc meta index: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid response format: code is not a number")
	}

	var result SimpleResponse
	result.Code = int(code)
	if result.Code == 0 {
		result.Message = "Success to create doc meta index"
	} else {
		result.Message = fmt.Sprintf("Failed to create doc meta index: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// DropDocMetaIndex drops the document metadata index for the tenant
func (c *RAGFlowClient) DropDocMetaIndex(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("DELETE", "/tenant/doc_meta_index", false, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop doc meta index: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop doc meta index: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid response format: code is not a number")
	}

	var result SimpleResponse
	result.Code = int(code)
	if result.Code == 0 {
		result.Message = "Success to drop doc meta index"
	} else {
		result.Message = fmt.Sprintf("Failed to drop doc meta index: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// CreateProvider creates a new model provider
// CREATE PROVIDER <name>
// CREATE PROVIDER <name> <api_key>
func (c *RAGFlowClient) CreateProvider(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	// Get optional api_key
	apiKey, _ := cmd.Params["api_key"].(string)

	// Build payload
	payload := map[string]interface{}{
		"llm_factory": providerName,
		"api_key":     apiKey,
		"verify":      apiKey != "", // Only verify if api_key is provided
	}

	resp, err := c.HTTPClient.Request("POST", "/llm/set_api_key", true, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create provider: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create provider failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ListProviders lists all providers
// LIST PROVIDERS
func (c *RAGFlowClient) ListProviders(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("GET", "/llm/factories", true, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list providers: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list providers failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// DropProvider deletes a provider
// DROP PROVIDER <name>
func (c *RAGFlowClient) DropProvider(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	// Build payload
	payload := map[string]interface{}{
		"llm_factory": providerName,
	}

	resp, err := c.HTTPClient.Request("DELETE", "/llm/factory", true, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop provider: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop provider: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop provider failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// Context related commands

// CEList handles the ls command - lists nodes using Context Engine
func (c *RAGFlowClient) CEList(cmd *Command) (ResponseIf, error) {
	// Get path from command params, default to "datasets"
	path, _ := cmd.Params["path"].(string)
	if path == "" {
		path = "datasets"
	}

	// Parse options
	opts := &ce.ListOptions{}
	if recursive, ok := cmd.Params["recursive"].(bool); ok {
		opts.Recursive = recursive
	}
	if limit, ok := cmd.Params["limit"].(int); ok {
		opts.Limit = limit
	}
	if offset, ok := cmd.Params["offset"].(int); ok {
		opts.Offset = offset
	}

	// Execute list command through Context Engine
	ctx := context.Background()
	result, err := c.ContextEngine.List(ctx, path, opts)
	if err != nil {
		return nil, err
	}

	// Convert to response
	var response CEListResponse
	response.outputFormat = c.OutputFormat
	response.Code = 0
	response.Data = ce.FormatNodes(result.Nodes, string(c.OutputFormat))

	return &response, nil
}

// CESearch handles the search command using Context Engine
func (c *RAGFlowClient) CESearch(cmd *Command) (ResponseIf, error) {
	// Get path and query from command params
	path, _ := cmd.Params["path"].(string)
	if path == "" {
		path = "datasets"
	}
	query, _ := cmd.Params["query"].(string)

	// Parse options
	opts := &ce.SearchOptions{
		Query: query,
	}
	if limit, ok := cmd.Params["limit"].(int); ok {
		opts.Limit = limit
	}
	if offset, ok := cmd.Params["offset"].(int); ok {
		opts.Offset = offset
	}
	if recursive, ok := cmd.Params["recursive"].(bool); ok {
		opts.Recursive = recursive
	}

	// Execute search command through Context Engine
	ctx := context.Background()
	result, err := c.ContextEngine.Search(ctx, path, opts)
	if err != nil {
		return nil, err
	}

	// Convert to response
	var response CESearchResponse
	response.outputFormat = c.OutputFormat
	response.Code = 0
	response.Total = result.Total
	response.Data = ce.FormatNodes(result.Nodes, string(c.OutputFormat))

	return &response, nil
}
