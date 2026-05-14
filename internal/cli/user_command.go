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
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	netUrl "net/url"
	"os"
	"path/filepath"
	ce "ragflow/internal/cli/filesystem"
	"strings"
	"time"
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
		return c.HTTPClient.RequestWithIterations("GET", "/system/ping", "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.HTTPClient.Request("GET", "/system/ping", "web", nil, nil)
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
		return c.HTTPClient.RequestWithIterations("GET", "/system/version", "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.HTTPClient.Request("GET", "/system/version", "web", nil, nil)
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

func (c *RAGFlowClient) ListConfigs(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.HTTPClient.RequestWithIterations("GET", "/system/configs", "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.HTTPClient.Request("GET", "/system/configs", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list configs: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var response CommonDataResponse
	if err = json.Unmarshal(resp.Body, &response); err != nil {
		return nil, fmt.Errorf("list configs failed: invalid JSON (%w)", err)
	}

	var result CommonResponse
	result.Code = 0
	result.Data, err = GetConfigs(&response.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func GetConfigs(config *map[string]interface{}) ([]map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	result := []map[string]interface{}{}
	{
		redisHost := GetHost(config, "Redis", "Host", "Port")
		result = append(result, map[string]interface{}{
			"key":   "redis_host",
			"value": redisHost})
	}
	{
		if docEngine, ok := (*config)["DocEngine"].(map[string]interface{}); ok {
			engineType, _ := docEngine["Type"].(string)
			result = append(result, map[string]interface{}{
				"key":   "doc_engine",
				"value": engineType})
			if engineType == "elasticsearch" {
				esCfg, _ := docEngine["ES"].(map[string]interface{})
				esHost, _ := esCfg["Hosts"].(string)
				result = append(result, map[string]interface{}{
					"key":   "elasticsearch_host",
					"value": esHost})
			} else if engineType == "Infinity" {
				infinityCfg, _ := docEngine["Infinity"].(map[string]interface{})
				infinityHost, _ := infinityCfg["URI"]
				result = append(result, map[string]interface{}{
					"key":   "infinity_host",
					"value": infinityHost})
			} else {
				return nil, fmt.Errorf("unknown doc engine: %s", engineType)
			}
		}
	}
	{
		if logConfig, ok := (*config)["Log"].(map[string]interface{}); ok {
			level, _ := logConfig["Level"].(string)
			result = append(result, map[string]interface{}{
				"key":   "log_level",
				"value": level})
		}
	}
	{
		if databaseConfig, ok := (*config)["Database"].(map[string]interface{}); ok {
			driver, _ := databaseConfig["Driver"].(string)
			result = append(result, map[string]interface{}{
				"key":   "database",
				"value": driver})
			driverAddr, _ := databaseConfig["Host"].(string)
			driverPort, _ := databaseConfig["Port"].(float64)
			driverHost := fmt.Sprintf("%s:%0.f", driverAddr, driverPort)
			result = append(result, map[string]interface{}{
				"key":   "database_host",
				"value": driverHost})
		}
	}
	{
		if language, ok := (*config)["Language"].(map[string]interface{}); ok {
			result = append(result, map[string]interface{}{
				"key":   "language",
				"value": language})
		}
	}
	{
		if adminConfig, ok := (*config)["Admin"].(map[string]interface{}); ok {
			adminAddr, _ := adminConfig["Host"].(string)
			adminPort, _ := adminConfig["Port"].(float64)
			adminHost := fmt.Sprintf("%s:%0.f", adminAddr, adminPort)
			result = append(result, map[string]interface{}{
				"key":   "admin",
				"value": adminHost})
		}
	}
	{
		if storageEngineConfig, ok := (*config)["StorageEngine"].(map[string]interface{}); ok {
			engineType, _ := storageEngineConfig["Type"].(string)
			result = append(result, map[string]interface{}{
				"key":   "storage_engine",
				"value": engineType})
			if engineType == "minio" {
				minioCfg, _ := storageEngineConfig["Minio"].(map[string]interface{})
				miniHost, _ := minioCfg["Host"].(string)
				result = append(result, map[string]interface{}{
					"key":   "minio_host",
					"value": miniHost})
			} else {
				return nil, fmt.Errorf("unknown storage engine: %s", engineType)
			}
		}
	}
	return result, nil
}

func GetHost(config *map[string]interface{}, serverType, address, port string) string {
	if config == nil {
		return ""
	}

	result := ""

	if redis, ok := (*config)[serverType].(map[string]interface{}); ok {
		serverAddr, hostOk := redis[address].(string)
		serverPort, portOk := redis[port].(float64)

		if hostOk && portOk {
			result = fmt.Sprintf("%s:%.0f", serverAddr, serverPort)
		}
	}

	return result
}

func (c *RAGFlowClient) SetLogLevel(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	if logLevel, ok := cmd.Params["level"].(string); ok {
		payload := map[string]interface{}{
			"level": logLevel,
		}

		resp, err := c.HTTPClient.Request("PUT", "/system/log", "admin", nil, payload)
		if err != nil {
			return nil, fmt.Errorf("failed to change log level: %w", err)
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("failed to register user: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
		}

		var result SimpleResponse
		if err = json.Unmarshal(resp.Body, &result); err != nil {
			return nil, fmt.Errorf("change log level failed: invalid JSON (%w)", err)
		}
		result.Code = 0
		result.Duration = resp.Duration
		return &result, nil
	}

	return nil, fmt.Errorf("no log level")
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

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	var nickname string
	nickname, ok = cmd.Params["nickname"].(string)
	if !ok {
		return nil, fmt.Errorf("no nickname")
	}

	payload := map[string]interface{}{
		"email":    email,
		"password": encryptedPassword,
		"nickname": nickname,
	}

	resp, err := c.HTTPClient.Request("POST", "/users", "web", nil, payload)
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

// ListDatasets lists datasets for current user (user mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListDatasets(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	// Determine auth kind based on whether API token is being used
	if c.HTTPClient.LoginToken == "" && !c.HTTPClient.useAPIToken {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if c.HTTPClient.useAPIToken {
		authKind = "api"
	}

	if c.HTTPClient.LoginToken != "" {
		authKind = "web"
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", "/datasets", authKind, nil, nil, iterations)
	}

	// Normal mode
	resp, err := c.HTTPClient.Request("GET", "/datasets", authKind, nil, nil)
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
	resp, err := c.HTTPClient.Request("GET", "/datasets", "web", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to list datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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

	data, ok := resJSON["data"].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	for _, kb := range data {
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
		"dataset_ids":              datasetIDs,
		"question":                 question,
		"similarity_threshold":     0.2,
		"vector_similarity_weight": 0.3,
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("POST", "/datasets/search", "web", nil, payload, iterations)
	}

	// Normal mode
	resp, err := c.HTTPClient.Request("POST", "/datasets/search", "web", nil, payload)
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

	resp, err := c.HTTPClient.Request("POST", "/system/tokens", "web", nil, nil)
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

	resp, err := c.HTTPClient.Request("GET", "/system/tokens", "web", nil, nil)
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

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/system/tokens/%s", token), "web", nil, nil)
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
	resp, err := c.HTTPClient.Request("GET", "/tokens", "api", nil, nil)
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

// CreateDataset creates a table for a dataset
func (c *RAGFlowClient) CreateDataset(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.HTTPClient.Request("POST", "/kb/doc_engine_table", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create table: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to create table for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to create table: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// CreateDatasetInDocEngine creates a table for a dataset in doc engine
func (c *RAGFlowClient) CreateDatasetInDocEngine(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.HTTPClient.Request("POST", "/kb/doc_engine_table", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create table: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to create table for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to create table: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// DropDatasetInDocEngine drops a table for a dataset in doc engine
func (c *RAGFlowClient) DropDatasetInDocEngine(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.HTTPClient.Request("DELETE", "/kb/doc_engine_table", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop dataset: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop dataset: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to drop table for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to drop table for dataset: %s: %v", datasetName, resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// CreateMetadataInDocEngine creates the document metadata table for the tenant
func (c *RAGFlowClient) CreateMetadataInDocEngine(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("POST", "/tenant/doc_engine_metadata_table", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata table: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create metadata table: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = "Success to create metadata table"
	} else {
		result.Message = fmt.Sprintf("Failed to create metadata table: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// DropMetadataInDocEngine drops the document metadata table for the tenant
func (c *RAGFlowClient) DropMetadataInDocEngine(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.HTTPClient.Request("DELETE", "/tenant/doc_engine_metadata_table", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop metadata table: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop metadata table: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = "Success to drop metadata table"
	} else {
		result.Message = fmt.Sprintf("Failed to drop metadata table: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// AddProvider creates a new model provider
// ADD PROVIDER <name>
// ADD PROVIDER <name> <api_key>
func (c *RAGFlowClient) AddProvider(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	// Build payload
	payload := map[string]interface{}{
		"provider_name": providerName,
	}

	resp, err := c.HTTPClient.Request("PUT", "/providers", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to add provider: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("add provider failed: invalid JSON (%w)", err)
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

	resp, err := c.HTTPClient.Request("GET", "/providers", "web", nil, nil)
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

// DeleteProvider deletes a provider
// DELETE PROVIDER <name>
func (c *RAGFlowClient) DeleteProvider(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s", providerName)

	// Build payload
	payload := map[string]interface{}{
		"llm_factory": providerName,
	}

	resp, err := c.HTTPClient.Request("DELETE", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to delete provider: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to delete provider: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("delete provider failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// CreateProviderInstance creates a new provider instance
// CREATE PROVIDER <name> INSTANCE <instance_name> <api_key>
func (c *RAGFlowClient) CreateProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	apiKey, ok := cmd.Params["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("API key not provided")
	}

	baseUrl, ok := cmd.Params["base_url"].(string)
	if !ok {
		baseUrl = ""
	}

	region, ok := cmd.Params["region"].(string)
	if !ok {
		region = ""
	}

	url := fmt.Sprintf("/providers/%s/instances", providerName)

	payload := map[string]interface{}{
		"instance_name": instanceName,
		"api_key":       apiKey,
		"base_url":      baseUrl,
		"region":        region,
	}

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider instance: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create provider instance: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create provider instance failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ListProviderInstances lists all instances of a provider
// LIST INSTANCES FROM PROVIDER <name>
func (c *RAGFlowClient) ListProviderInstances(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances", providerName)

	resp, err := c.HTTPClient.Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list instances: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list instances failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ShowProviderInstance shows details of a specific instance
// SHOW INSTANCE <name> FROM PROVIDER <name>
func (c *RAGFlowClient) ShowProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s", providerName, instanceName)

	resp, err := c.HTTPClient.Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show instance: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show instance: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show instance failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ShowInstanceBalance shows balance of a specific instance
// SHOW BALANCE FROM PROVIDER <provider_name> <instance_name>
func (c *RAGFlowClient) ShowInstanceBalance(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/balance", providerName, instanceName)

	resp, err := c.HTTPClient.Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show instance: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show instance: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show instance failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// AlterProviderInstance renames a provider instance
// ALTER INSTANCE <name> NAME <new_name> FROM PROVIDER <name>
func (c *RAGFlowClient) AlterProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	newName, ok := cmd.Params["new_name"].(string)
	if !ok {
		return nil, fmt.Errorf("new name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s", providerName, instanceName)

	payload := map[string]interface{}{
		"llm_name": newName,
	}

	resp, err := c.HTTPClient.Request("PUT", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to alter instance: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to alter instance: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("alter instance failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// DropProviderInstance deletes a provider instance
// DROP INSTANCE <name> FROM PROVIDER <name>
func (c *RAGFlowClient) DropProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	payload := map[string]interface{}{
		"instances": []string{instanceName},
	}

	url := fmt.Sprintf("/providers/%s/instances", providerName)

	resp, err := c.HTTPClient.Request("DELETE", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop instance: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop instance: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop instance failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// DropInstanceModel deletes a provider instance, only works for local deployed model
// DROP MODEL <name> FROM <provider_name> <instance_name>
func (c *RAGFlowClient) DropInstanceModel(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model name not provided")
	}

	payload := map[string]interface{}{
		"models": []string{modelName},
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/models", providerName, instanceName)

	resp, err := c.HTTPClient.Request("DELETE", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop instance: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop instance: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop instance failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *RAGFlowClient) ListInstanceModels(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	var endPoint string
	endPoint = fmt.Sprintf("/providers/%s/instances/%s/models", providerName, instanceName)

	resp, err := c.HTTPClient.Request("GET", endPoint, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list instance models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list instance models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list instance models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *RAGFlowClient) EnableOrDisableModel(cmd *Command, status string) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/models/%s", providerName, instanceName, modelName)

	payload := map[string]interface{}{
		"status": status,
	}

	resp, err := c.HTTPClient.Request("PATCH", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to enable/disable model: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to enable/disable model: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("enable/disable model failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func isValidURL(str string) bool {
	u, err := netUrl.Parse(str)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func (c *RAGFlowClient) ChatToModel(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string

	// Check if composite_model_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_model_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 3 {
			return nil, fmt.Errorf("model name must be in format 'model@instance@provider'")
		}
		providerName = names[2]
		instanceName = names[1]
		modelName = names[0]
	} else if c.CurrentModel != nil {
		// Use current model if set
		providerName = c.CurrentModel.Provider
		instanceName = c.CurrentModel.Instance
		modelName = c.CurrentModel.Model
	} else {
		return nil, fmt.Errorf("model name not provided and no current model set. Use 'use model' command first")
	}

	formattedMessages := []map[string]interface{}{}

	messages, ok := cmd.Params["messages"].([]string)
	if !ok {
		return nil, fmt.Errorf("messages not provided")
	}
	contents := []map[string]interface{}{}
	if len(messages) > 0 {
		for _, message := range messages {
			contents = append(contents, map[string]interface{}{
				"type": "text",
				"text": message,
			})
		}
	}

	images, ok := cmd.Params["images"].([]string)
	if !ok {
		return nil, fmt.Errorf("images not provided")
	}
	if len(images) > 0 {
		for _, image := range images {
			if isValidURL(image) {
				contents = append(contents, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]string{
						"url": image,
					},
				})
			} else {
				// image is a path, read the file and turn it into base64
				imageContent, err := os.ReadFile(image)
				if err != nil {
					return nil, fmt.Errorf("failed to read image: %w", err)
				}
				contents = append(contents, map[string]interface{}{
					"type": "image_file",
					"image_file": map[string]interface{}{
						"content": base64.StdEncoding.EncodeToString(imageContent),
					},
				})
			}
		}
	}

	videos, ok := cmd.Params["videos"].([]string)
	if !ok {
		return nil, fmt.Errorf("images not provided")
	}
	if len(videos) > 0 {
		for _, video := range videos {
			if isValidURL(video) {
				contents = append(contents, map[string]interface{}{
					"type": "video_url",
					"video_url": map[string]interface{}{
						"url": video,
					},
				})
			} else {
				return nil, fmt.Errorf("invalid video URL: %s", video)
			}
		}
	}

	audios, ok := cmd.Params["audios"].([]string)
	if !ok {
		return nil, fmt.Errorf("images not provided")
	}
	if len(audios) > 0 {
		if len(audios) != 1 {
			return nil, fmt.Errorf("only one audio file is supported")
		}
		audioFile := audios[0]
		audioContent, err := os.ReadFile(audioFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read audio: %w", err)
		}
		// file type: wav or mp3
		format := filepath.Ext(audioFile) // file type: wav or mp3
		format = strings.TrimPrefix(format, ".")
		contents = append(contents, map[string]interface{}{
			"type": "input_audio",
			"input_audio": map[string]interface{}{
				"data":   base64.StdEncoding.EncodeToString(audioContent),
				"format": format,
			},
		})
	}

	files, ok := cmd.Params["files"].([]string)
	if !ok {
		return nil, fmt.Errorf("images not provided")
	}
	if len(files) > 0 {
		for _, file := range files {
			if isValidURL(file) {
				contents = append(contents, map[string]interface{}{
					"type": "file_url",
					"file_url": map[string]interface{}{
						"url": file,
					},
				})
			} else {
				return nil, fmt.Errorf("invalid file URL: %s", file)
			}
		}
	}

	formattedText := map[string]interface{}{
		"role":    "user",
		"content": contents,
	}
	formattedMessages = append(formattedMessages, formattedText)

	thinking := cmd.Params["thinking"].(bool)
	stream := cmd.Params["stream"].(bool)
	effort := cmd.Params["effort"].(string)
	verbosity := cmd.Params["verbosity"].(string)

	url := "/chat/completions"

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"messages":      formattedMessages,
		"stream":        stream,
		"thinking":      thinking,
	}

	if thinking {
		payload["effort"] = effort
		payload["verbosity"] = verbosity
	}

	if stream {
		// Call stream http api
		startTime := time.Now()
		reader, err := c.HTTPClient.RequestStream("POST", url, "web", nil, payload)
		if err != nil {
			return nil, fmt.Errorf("failed to chat model: %w", err)
		}
		defer reader.Close()

		// Parse SSE and output to console
		scanner := bufio.NewScanner(reader)
		var fullMessage strings.Builder

		reasoningPrint := true
		messagePrint := true
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)

				if strings.HasPrefix(data, "[REASONING]") {
					data = strings.TrimPrefix(data, "[REASONING]")
					if reasoningPrint {
						fmt.Print("Thinking: ")
						reasoningPrint = false
						thinking = true
					} else {
						fmt.Print(data)
					}
					os.Stdout.Sync()
				}
				if strings.HasPrefix(data, "[MESSAGE]") {
					data = strings.TrimPrefix(data, "[MESSAGE]")
					if messagePrint {
						if thinking {
							fmt.Println()
						}
						fmt.Print("Answer: ")
						messagePrint = false
					} else {
						fmt.Print(data)
						os.Stdout.Sync()
						fullMessage.WriteString(data)
					}
				}
			} else if strings.HasPrefix(line, "event:error") {
				// error event
				if scanner.Scan() {
					errData := strings.TrimPrefix(scanner.Text(), "data:")
					errData = strings.TrimSpace(errData)
					return nil, fmt.Errorf("chat error: %s", errData)
				}
				// If there's an error, return a generic error
				return nil, fmt.Errorf("chat error: received error event from server")
			}
		}
		duration := time.Since(startTime).Seconds()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("error reading stream: %w", err)
		}

		fmt.Println()

		result := &StreamMessageResponse{
			Code:     0,
			Message:  fullMessage.String(),
			Duration: duration,
		}
		return result, nil
	}

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, formatRequestError("Chat request", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list instance models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result NonStreamResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list instance models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *RAGFlowClient) EmbedUserText(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string

	// Check if composite_model_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_model_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 3 {
			return nil, fmt.Errorf("model name must be in format 'model@instance@provider'")
		}
		providerName = names[2]
		instanceName = names[1]
		modelName = names[0]
	} else if c.CurrentModel != nil {
		// Use current model if set
		providerName = c.CurrentModel.Provider
		instanceName = c.CurrentModel.Instance
		modelName = c.CurrentModel.Model
	} else {
		return nil, fmt.Errorf("model name not provided and no current model set. Use 'use model' command first")
	}

	texts, ok := cmd.Params["texts"].([]string)
	if !ok {
		return nil, fmt.Errorf("texts not provided")
	}

	dimension, ok := cmd.Params["dimension"].(int)
	if !ok {
		dimension = 0
	}

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"texts":         texts,
		"dimension":     dimension,
	}

	url := "/embeddings"

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to embed text: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to embed text: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result EmbeddingsResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("embed text failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *RAGFlowClient) RerankUserDocument(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string

	// Check if composite_model_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_model_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 3 {
			return nil, fmt.Errorf("model name must be in format 'model@instance@provider'")
		}
		providerName = names[2]
		instanceName = names[1]
		modelName = names[0]
	} else if c.CurrentModel != nil {
		// Use current model if set
		providerName = c.CurrentModel.Provider
		instanceName = c.CurrentModel.Instance
		modelName = c.CurrentModel.Model
	} else {
		return nil, fmt.Errorf("model name not provided and no current model set. Use 'use model' command first")
	}

	query, ok := cmd.Params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query not provided")
	}

	documents, ok := cmd.Params["documents"].([]string)
	if !ok {
		return nil, fmt.Errorf("documents not provided")
	}

	topN, ok := cmd.Params["top_n"].(int)
	if !ok {
		return nil, fmt.Errorf("top n not provided")
	}

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"query":         query,
		"documents":     documents,
		"top_n":         topN,
	}

	url := "/rerank"

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to rerank document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to rerank document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("rerank document failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *RAGFlowClient) TTSUserCommand(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string

	// Check if composite_model_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_model_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 3 {
			return nil, fmt.Errorf("model name must be in format 'model@instance@provider'")
		}
		providerName = names[2]
		instanceName = names[1]
		modelName = names[0]
	} else if c.CurrentModel != nil {
		// Use current model if set
		providerName = c.CurrentModel.Provider
		instanceName = c.CurrentModel.Instance
		modelName = c.CurrentModel.Model
	} else {
		return nil, fmt.Errorf("model name not provided and no current model set. Use 'use model' command first")
	}

	text, ok := cmd.Params["text"].(string)
	if !ok {
		return nil, fmt.Errorf("text not provided")
	}

	//fileToSave, ok := cmd.Params["file"].(string)
	//if !ok {
	//	return nil, fmt.Errorf("file not provided")
	//}

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"text":          text,
	}

	url := "/audio/speech"

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to TTS document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to TTS document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("TTS document failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	// save file
	//err = os.WriteFile(fileToSave, resp.Body, 0644)
	//if err != nil {
	//	result.Message += fmt.Sprintf("failed to save file: %s", err.Error())
	//	result.Code = 1
	//}

	return &result, nil
}

func (c *RAGFlowClient) ASRUserCommand(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string

	// Check if composite_model_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_model_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 3 {
			return nil, fmt.Errorf("model name must be in format 'model@instance@provider'")
		}
		providerName = names[2]
		instanceName = names[1]
		modelName = names[0]
	} else if c.CurrentModel != nil {
		// Use current model if set
		providerName = c.CurrentModel.Provider
		instanceName = c.CurrentModel.Instance
		modelName = c.CurrentModel.Model
	} else {
		return nil, fmt.Errorf("model name not provided and no current model set. Use 'use model' command first")
	}

	audioFile, ok := cmd.Params["audio_file"].(string)
	if !ok {
		return nil, fmt.Errorf("text not provided")
	}

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"audio_file":    audioFile,
	}

	url := "/audio/transcriptions"

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to ASR document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ASR document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ASR document failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

func (c *RAGFlowClient) OCRUserCommand(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string

	// Check if composite_model_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_model_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 3 {
			return nil, fmt.Errorf("model name must be in format 'model@instance@provider'")
		}
		providerName = names[2]
		instanceName = names[1]
		modelName = names[0]
	} else if c.CurrentModel != nil {
		// Use current model if set
		providerName = c.CurrentModel.Provider
		instanceName = c.CurrentModel.Instance
		modelName = c.CurrentModel.Model
	} else {
		return nil, fmt.Errorf("model name not provided and no current model set. Use 'use model' command first")
	}

	var filename string
	var fileURL string
	var ok bool
	var fileContent []byte

	filename, ok = cmd.Params["file"].(string)
	if ok {
		// read file and convert to base64
		var err error
		fileContent, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	} else {
		fileURL, ok = cmd.Params["url"].(string)
		if !ok {
			return nil, fmt.Errorf("file or url not provided")
		}
	}

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
	}

	if fileContent != nil {
		payload["content"] = fileContent
	} else {
		payload["url"] = fileURL
	}

	url := "/file/ocr"

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to OCR document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to OCR document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("OCR document failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

func (c *RAGFlowClient) CheckProviderConnection(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/connection", providerName, instanceName)

	resp, err := c.HTTPClient.Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to check provider connection: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to check provider connection: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("check provider connection failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// UseModel sets the current model for chat
func (c *RAGFlowClient) UseModel(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if !ok || compositeModelName == "" {
		return nil, fmt.Errorf("model identifier not provided")
	}

	names := strings.Split(compositeModelName, "@")
	if len(names) != 3 {
		return nil, fmt.Errorf("model identifier must be in format 'model@instance@provider'")
	}

	c.CurrentModel = &CurrentModel{
		Provider: names[2],
		Instance: names[1],
		Model:    names[0],
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = fmt.Sprintf("Current model set to: %s/%s/%s", c.CurrentModel.Provider, c.CurrentModel.Instance, c.CurrentModel.Model)
	return &result, nil
}

// ShowCurrentModel displays the current model configuration
func (c *RAGFlowClient) ShowCurrentModel(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.CurrentModel == nil {
		return nil, fmt.Errorf("no current model set. Use 'use model' command first")
	}

	var result CommonResponse
	result.Code = 0
	result.Data = []map[string]interface{}{
		{
			"provider": c.CurrentModel.Provider,
			"instance": c.CurrentModel.Instance,
			"model":    c.CurrentModel.Model,
		},
	}
	return &result, nil
}

func (c *RAGFlowClient) AddCustomModel(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model name not provided")
	}

	// chat, vision, embedding, rerank, tts, asr, ocr
	modelTypes, ok := cmd.Params["model_types"].([]string)
	if !ok {
		return nil, fmt.Errorf("model type not provided")
	}

	maxTokens, ok := cmd.Params["max_tokens"].(int)
	if !ok {
		return nil, fmt.Errorf("max tokens not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/models", providerName, instanceName)

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"model_types":   modelTypes,
		"max_tokens":    maxTokens,
	}

	supportThink, ok := cmd.Params["support_think"].(bool)
	if ok {
		payload["thinking"] = supportThink
	}

	resp, err := c.HTTPClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add custom model: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to add custom model: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("add custom model failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil

}

// Context related commands

// CECat handles the cat command - shows content using Context Engine
func (c *RAGFlowClient) CECat(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	path, ok := cmd.Params["path"].(string)
	if !ok {
		return nil, fmt.Errorf("fail to convert 'path' to string")
	}

	// Execute cat command through Filesystem Engine
	ctx := context.Background()
	content, err := c.ContextEngine.Cat(ctx, path)
	if err != nil {
		return nil, err
	}

	// Convert to response
	var response ContextCatResponse
	response.OutputFormat = c.OutputFormat
	response.Code = 0
	response.Content = string(content)

	return &response, nil
}

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

	// Execute list command through Filesystem Engine
	ctx := context.Background()
	result, err := c.ContextEngine.List(ctx, path, opts)
	if err != nil {
		return nil, err
	}

	// Convert to response
	var response ContextListResponse
	response.OutputFormat = c.OutputFormat
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

	// Execute search command through Filesystem Engine
	ctx := context.Background()
	result, err := c.ContextEngine.Search(ctx, path, opts)
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

// InsertDatasetFromFile inserts dataset chunks from a JSON file
func (c *RAGFlowClient) InsertDatasetFromFile(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}
	filePath, ok := cmd.Params["file_path"].(string)
	if !ok {
		return nil, fmt.Errorf("file_path not provided")
	}

	payload := map[string]interface{}{
		"file_path": filePath,
	}

	resp, err := c.HTTPClient.Request("POST", "/kb/insert_from_file", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to insert dataset from file: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to insert dataset from file: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to insert dataset from file: %s", filePath)
	} else {
		result.Message = fmt.Sprintf("Failed to insert dataset from file: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// InsertMetadataFromFile inserts metadata from a JSON file
func (c *RAGFlowClient) InsertMetadataFromFile(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	filePath, ok := cmd.Params["file_path"].(string)
	if !ok {
		return nil, fmt.Errorf("file_path not provided")
	}

	payload := map[string]interface{}{
		"file_path": filePath,
	}

	resp, err := c.HTTPClient.Request("POST", "/tenant/insert_metadata_from_file", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to insert metadata from file: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to insert metadata from file: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to insert metadata from file: %s", filePath)
	} else {
		result.Message = fmt.Sprintf("Failed to insert metadata from file: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// UpdateChunk updates a chunk in a dataset
func (c *RAGFlowClient) UpdateChunk(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	chunkID, ok := cmd.Params["chunk_id"].(string)
	if !ok {
		return nil, fmt.Errorf("chunk_id not provided")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	jsonBody, ok := cmd.Params["json_body"].(string)
	if !ok {
		return nil, fmt.Errorf("json_body not provided")
	}

	// Look up dataset_id from dataset_name
	datasetID, err := c.getDatasetID(datasetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset ID: %w", err)
	}

	// Try to get doc_id from the chunk retrieval endpoint
	getResp, err := c.HTTPClient.Request("GET", "/chunk/get?chunk_id="+chunkID, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk info: %w", err)
	}

	var docID string
	if getResp.StatusCode == 200 {
		getJSON, err := getResp.JSON()
		if err == nil {
			if data, ok := getJSON["data"].(map[string]interface{}); ok {
				if d, ok := data["doc_id"].(string); ok {
					docID = d
				}
			}
		}
	}

	if docID == "" {
		return nil, fmt.Errorf("could not find document_id for chunk %s. Please provide document_id explicitly", chunkID)
	}

	// Parse the JSON body
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(jsonBody), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}

	// Add IDs to payload
	payload["dataset_id"] = datasetID
	payload["document_id"] = docID
	payload["chunk_id"] = chunkID

	resp, err := c.HTTPClient.Request("POST", "/chunk/update", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to update chunk: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to update chunk: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to update chunk: %s", chunkID)
	} else {
		result.Message = fmt.Sprintf("Failed to update chunk: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// SetMeta sets metadata for a document
func (c *RAGFlowClient) SetMeta(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	docID, ok := cmd.Params["doc_id"].(string)
	if !ok {
		return nil, fmt.Errorf("doc_id not provided")
	}

	metaJSON, ok := cmd.Params["meta"].(string)
	if !ok {
		return nil, fmt.Errorf("meta not provided")
	}

	payload := map[string]interface{}{
		"doc_id": docID,
		"meta":   metaJSON,
	}

	resp, err := c.HTTPClient.Request("POST", "/document/set_meta", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to set metadata: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to set metadata for document: %s", docID)
	} else {
		result.Message = fmt.Sprintf("Failed to set metadata: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// RmTags removes tags from chunks in a dataset
func (c *RAGFlowClient) RmTags(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	kbID, err := c.getDatasetID(datasetName)
	if err != nil {
		return nil, err
	}

	tags, ok := cmd.Params["tags"].([]string)
	if !ok {
		return nil, fmt.Errorf("tags not provided")
	}

	payload := map[string]interface{}{
		"tags": tags,
	}

	resp, err := c.HTTPClient.Request("POST", "/kb/"+kbID+"/rm_tags", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove tags: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to remove tags: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to remove tags from dataset: %s", kbID)
	} else {
		result.Message = fmt.Sprintf("Failed to remove tags: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// RemoveChunks removes chunks from a document
func (c *RAGFlowClient) RemoveChunks(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	docID, ok := cmd.Params["doc_id"].(string)
	if !ok {
		return nil, fmt.Errorf("doc_id not provided")
	}

	payload := map[string]interface{}{
		"doc_id": docID,
	}

	// Check if delete_all is set
	if deleteAll, ok := cmd.Params["delete_all"].(bool); ok && deleteAll {
		payload["delete_all"] = true
	} else if chunkIDs, ok := cmd.Params["chunk_ids"].([]string); ok {
		payload["chunk_ids"] = chunkIDs
	}

	resp, err := c.HTTPClient.Request("POST", "/chunk/rm", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove chunks: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to remove chunks: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		deletedCount := int64(0)
		switch data := resJSON["data"].(type) {
		case float64:
			deletedCount = int64(data)
		case map[string]interface{}:
			if count, ok := data["deleted_count"].(float64); ok {
				deletedCount = int64(count)
			}
		}
		result.Message = fmt.Sprintf("Success to remove chunks from document %s: %d chunks deleted", docID, deletedCount)
	} else {
		result.Message = fmt.Sprintf("Failed to remove chunks: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// formatRequestError Uniformly handle and format network errors in HTTP requests
func formatRequestError(action string, err error) error {
	if err == nil {
		return nil
	}

	var netErr net.Error

	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Errorf("%s failed - connection closed (EOF): upstream overloaded or proxy timeout: %w", action, err)
	case errors.As(err, &netErr) && netErr.Timeout():
		return fmt.Errorf("%s failed - request timeout: server took too long to respond: %w", action, err)
	default:
		return fmt.Errorf("%s failed: %w", action, err)
	}
}
