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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	netUrl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/ingestion"
	"ragflow/internal/ingestion/parser"
	"ragflow/internal/utility"
	"regexp"
	"strings"
	"time"
)

// Show server version to show RAGFlow server version
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *CLI) ShowServerVersion(cmd *Command) (ResponseIf, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return httpClient.RequestWithIterations("GET", "/system/version", "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := httpClient.Request("GET", "/system/version", "web", nil, nil)
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

func (c *CLI) ListConfigs(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return httpClient.RequestWithIterations("GET", "/system/configs", "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := httpClient.Request("GET", "/system/configs", "web", nil, nil)
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

func (c *CLI) SetLogLevel(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if logLevel, ok := cmd.Params["level"].(string); ok {
		payload := map[string]interface{}{
			"level": logLevel,
		}

		httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
		resp, err := httpClient.Request("PUT", "/system/log", "admin", nil, payload)
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

func (c *CLI) RegisterUser(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
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

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("POST", "/users", "web", nil, payload)
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
func (c *CLI) ListDatasets(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API token is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if httpClient.useAPIToken {
		authKind = "api"
	}

	if httpClient.LoginToken != nil {
		authKind = "web"
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return httpClient.RequestWithIterations("GET", "/datasets", authKind, nil, nil, iterations)
	}

	// Normal mode
	resp, err := httpClient.Request("GET", "/datasets", authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list datasets failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// ListDatasetDocumentUserCommand lists dataset documents
func (c *CLI) ListDatasetDocumentUserCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	// Determine auth kind based on whether API token is being used
	if httpClient.LoginToken == nil && !httpClient.useAPIToken {
		return nil, fmt.Errorf("no authorization")
	}

	datasetID, ok := cmd.Params["dataset_id"].(string)
	if !ok {
		return nil, fmt.Errorf("no dataset id")
	}

	page := 1
	pageSize := 10
	keywords := ""
	returnEmptyMetadata := "true"
	url := fmt.Sprintf("/datasets/%s/documents?page=%d&page_size=%d&keywords=%s&return_empty_metadata=%s", datasetID, page, pageSize, keywords, returnEmptyMetadata)

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return httpClient.RequestWithIterations("GET", url, "web", nil, nil, iterations)
	}

	// Normal mode
	resp, err := httpClient.Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list documents: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ListDocumentsResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list documents failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// getDatasetID gets dataset ID by name
func (c *CLI) getDatasetID(datasetName string) (string, error) {

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("GET", "/datasets", "web", nil, nil)
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

// GetMetadata gets metadata for one or more datasets
func (c *CLI) GetMetadata(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetNames, ok := cmd.Params["dataset_names"].([]string)
	if !ok || len(datasetNames) == 0 {
		return nil, fmt.Errorf("dataset_names not provided")
	}

	// Convert dataset names to IDs
	datasetIDs := make([]string, 0, len(datasetNames))
	for _, name := range datasetNames {
		id, err := c.getDatasetID(name)
		if err != nil {
			return nil, err
		}
		datasetIDs = append(datasetIDs, id)
	}

	// Build comma-separated dataset_ids for query param
	datasetIDsStr := strings.Join(datasetIDs, ",")

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("GET", "/datasets/metadata/flattened?dataset_ids="+datasetIDsStr, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list metadata: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list metadata: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result MetadataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list metadata failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
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
func (c *CLI) SearchOnDatasets(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	// Add optional parameters from command
	if val, ok := cmd.Params["top_k"]; ok {
		payload["top_k"] = val
	}
	if val, ok := cmd.Params["similarity_threshold"]; ok {
		payload["similarity_threshold"] = val
	}
	if val, ok := cmd.Params["vector_similarity_weight"]; ok {
		payload["vector_similarity_weight"] = val
	}
	if val, ok := cmd.Params["keyword"]; ok {
		payload["keyword"] = val
	}
	if val, ok := cmd.Params["use_kg"]; ok {
		payload["use_kg"] = val
	}
	if val, ok := cmd.Params["rerank_id"]; ok {
		payload["rerank_id"] = val
	}
	if val, ok := cmd.Params["tenant_rerank_id"]; ok {
		payload["tenant_rerank_id"] = val
	}
	if val, ok := cmd.Params["page_size"]; ok {
		payload["page_size"] = val
	}
	if val, ok := cmd.Params["page"]; ok {
		payload["page"] = val
	}
	if val, ok := cmd.Params["search_id"]; ok {
		if s, ok := val.(string); ok {
			payload["search_id"] = s
		}
	}
	if val, ok := cmd.Params["cross_languages"]; ok {
		if list, ok := val.([]string); ok {
			payload["cross_languages"] = list
		}
	}
	if val, ok := cmd.Params["doc_ids"]; ok {
		if list, ok := val.([]string); ok {
			payload["doc_ids"] = list
		}
	}
	if val, ok := cmd.Params["meta_data_filter"]; ok {
		// Accept either a raw JSON string from the CLI or a pre-decoded
		// map[string]interface{} (future-proofing for callers that
		// construct the command programmatically). The string form is
		// the public CLI surface; the map form is for unit tests.
		switch v := val.(type) {
		case string:
			var decoded map[string]interface{}
			if err := json.Unmarshal([]byte(v), &decoded); err != nil {
				return nil, fmt.Errorf("invalid meta_data_filter JSON: %w", err)
			}
			payload["meta_data_filter"] = decoded
		case map[string]interface{}:
			payload["meta_data_filter"] = v
		default:
			return nil, fmt.Errorf("meta_data_filter must be JSON string or object")
		}
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return httpClient.RequestWithIterations("POST", "/datasets/search", "web", nil, payload, iterations)
	}

	// Normal mode
	resp, err := httpClient.Request("POST", "/datasets/search", "web", nil, payload)
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
func (c *CLI) CreateToken(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("POST", "/system/tokens", "web", nil, nil)
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
func (c *CLI) ListTokens(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("GET", "/system/tokens", "web", nil, nil)
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
func (c *CLI) DropToken(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	token, ok := cmd.Params["token"].(string)
	if !ok {
		return nil, fmt.Errorf("token not provided")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", fmt.Sprintf("/system/tokens/%s", token), "web", nil, nil)
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
func (c *CLI) SetToken(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	token, ok := cmd.Params["token"].(string)
	if !ok {
		return nil, fmt.Errorf("token not provided")
	}

	// Save current token to restore if validation fails
	savedToken := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken
	savedUseAPIToken := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken

	// Set the new token temporarily for validation
	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken = &token
	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken = true

	// Validate token by calling list tokens API
	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/tokens", "api", nil, nil)
	if err != nil {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if resp.StatusCode != 200 {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken = savedUseAPIToken
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
func (c *CLI) ShowToken(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil {
		return nil, fmt.Errorf("no API token is currently set")
	}

	//fmt.Printf("Token: %s\n", c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken)

	var result CommonResponse
	result.Code = 0
	result.Message = ""
	result.Data = []map[string]interface{}{
		{
			"token": c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken,
		},
	}
	result.Duration = 0
	return &result, nil
}

// UnsetToken removes the current API token
func (c *CLI) UnsetToken(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil {
		return nil, fmt.Errorf("no API token is currently set")
	}

	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken = nil
	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIToken = false

	var result SimpleResponse
	result.Code = 0
	result.Message = "API token unset successfully"
	result.Duration = 0
	return &result, nil
}

// CreateChunkStore creates a chunk store in doc engine
func (c *CLI) CreateChunkStore(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/tenant/chunk_store", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk store: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create chunk store: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to create chunk store for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to create chunk store: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// DropChunkStore drops a chunk store in doc engine
func (c *CLI) DropChunkStore(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", "/tenant/chunk_store", "web", nil, payload)
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
		result.Message = fmt.Sprintf("Success to drop chunk store for dataset: %s", datasetName)
	} else {
		result.Message = fmt.Sprintf("Failed to drop chunk store for dataset: %s: %v", datasetName, resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// CreateMetadataStore creates the document metadata store for the tenant
func (c *CLI) CreateMetadataStore(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/tenant/metadata_store", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata store: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create metadata store: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = "Success to create metadata store"
	} else {
		result.Message = fmt.Sprintf("Failed to create metadata store: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// DropMetadataStore drops the document metadata store for the tenant
func (c *CLI) DropMetadataStore(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", "/tenant/metadata_store", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop metadata store: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop metadata store: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = "Success to drop metadata store"
	} else {
		result.Message = fmt.Sprintf("Failed to drop metadata store: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// AddProvider creates a new model provider
// ADD PROVIDER <name>
// ADD PROVIDER <name> <api_key>
func (c *CLI) AddProvider(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PUT", "/providers", "web", nil, payload)
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
func (c *CLI) ListProviders(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/providers", "web", nil, nil)
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
func (c *CLI) DeleteProvider(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", url, "web", nil, payload)
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
// CREATE PROVIDER <name> INSTANCE <instance_name> KEY <api_key> URL <base_url> REGION <region>
func (c *CLI) CreateProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
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
func (c *CLI) ListProviderInstances(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances", providerName)

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
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
func (c *CLI) ShowProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
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
func (c *CLI) ShowInstanceBalance(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
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
func (c *CLI) AlterProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PUT", url, "web", nil, payload)
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
func (c *CLI) DropProviderInstance(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", url, "web", nil, payload)
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

// DROP MODEL <name1 name2 name3> FROM <provider_name> <instance_name>
// Remove MODEL <name1 name2 name3> FROM <provider_name> <instance_name>
func (c *CLI) DropInstanceModel(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	modelNames, ok := cmd.Params["model_names"].([]string)
	if !ok {
		return nil, fmt.Errorf("model name not provided")
	}

	payload := map[string]interface{}{
		"models": modelNames,
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/models", providerName, instanceName)

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", url, "web", nil, payload)
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

func (c *CLI) ListInstanceModels(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
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

func (c *CLI) EnableOrDisableModel(cmd *Command, status string) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PATCH", url, "web", nil, payload)
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

func (c *CLI) ChatToModel(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
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
		"messages": formattedMessages,
		"stream":   stream,
		"thinking": thinking,
	}
	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	if thinking {
		payload["effort"] = effort
		payload["verbosity"] = verbosity
	}

	if stream {
		// Call stream http api
		startTime := time.Now()
		reader, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].RequestStream("POST", url, "web", nil, payload)
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, formatRequestError("Chat request", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to chat model: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result NonStreamResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to chat model: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) EmbedUserText(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	// Check if composite_model_name is provided in command
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
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
		"texts":     texts,
		"dimension": dimension,
	}
	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	url := "/embeddings"

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
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

func (c *CLI) RerankUserDocument(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	// Check if composite_model_name is provided in command
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
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
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}
	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	url := "/rerank"

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
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

func (c *CLI) TTSUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	// Check if composite_model_name is provided in command
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
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
		"text": text,
	}
	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	ttsConfigPayload := make(map[string]interface{})

	explicitFormat, hasExplicitFormat := cmd.Params["format"].(string)

	if paramStr, ok := cmd.Params["param_str"].(string); ok && paramStr != "" {
		var dynamicParams map[string]interface{}
		if err := json.Unmarshal([]byte(paramStr), &dynamicParams); err != nil {
			return nil, fmt.Errorf("param string must be valid JSON. Error: %w", err)
		}

		ttsConfigPayload["params"] = dynamicParams

		if !hasExplicitFormat {
			var findFormat func(map[string]interface{}) string
			findFormat = func(m map[string]interface{}) string {
				if val, ok := m["format"]; ok {
					return fmt.Sprintf("%v", val)
				}
				if val, ok := m["response_format"]; ok {
					return fmt.Sprintf("%v", val)
				}
				for _, v := range m {
					if subMap, ok := v.(map[string]interface{}); ok {
						if res := findFormat(subMap); res != "" {
							return res
						}
					}
				}
				return ""
			}
			if ext := findFormat(dynamicParams); ext != "" {
				explicitFormat = ext
			}
		}
	}

	if explicitFormat != "" {
		ttsConfigPayload["format"] = explicitFormat
	} else {
		ttsConfigPayload["format"] = "mp3"
	}

	if len(ttsConfigPayload) > 0 {
		payload["tts_config"] = ttsConfigPayload
	}

	url := "/audio/speech"

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to TTS document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to TTS document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var ttsResult struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Audio string `json:"audio"`
		} `json:"data"`
	}

	if err = json.Unmarshal(resp.Body, &ttsResult); err != nil {
		return nil, fmt.Errorf("TTS document failed: invalid JSON (%w)", err)
	}

	if ttsResult.Code != 0 {
		return nil, fmt.Errorf("%s", ttsResult.Message)
	}

	// Convert Base64 back to the original audio byte stream
	audioBytes, err := base64.StdEncoding.DecodeString(ttsResult.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio base64: %w", err)
	}

	shouldPlay, _ := cmd.Params["play"].(bool)
	shouldSave, _ := cmd.Params["save"].(bool)
	saveDir, _ := cmd.Params["save_path"].(string)

	// format file name
	safeModelName := strings.ReplaceAll(modelName, "/", "_")
	safeModelName = strings.ReplaceAll(safeModelName, ":", "-")
	fileName := fmt.Sprintf("%s_output.%s", safeModelName, explicitFormat)

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	localPath := filepath.Join(cwd, fileName)

	if err := os.WriteFile(localPath, audioBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write local audio file: %w", err)
	}

	if shouldPlay {
		cmdExec := exec.Command("aplay", localPath)
		if err := cmdExec.Run(); err != nil {
			fmt.Printf("Play error: %v (Hint: did you use 'format: wav' in your params?)\n", err)
		}
	}

	var finalMessage string
	if shouldSave {
		if saveDir == "" {
			saveDir = cwd
		} else {
			absSaveDir, err := filepath.Abs(saveDir)
			if err == nil {
				saveDir = absSaveDir
			}

			if err := os.MkdirAll(saveDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create save directory: %w", err)
			}

			finalPath := filepath.Join(saveDir, fileName)
			if err := os.WriteFile(finalPath, audioBytes, 0644); err != nil {
				return nil, fmt.Errorf("failed to save file to target directory: %w", err)
			}

			if saveDir != cwd {
				os.Remove(localPath)
			}

			finalMessage = fmt.Sprintf("Saved to directory: %s", finalPath)
		}
	} else {
		defer os.Remove(localPath)
		finalMessage = "TTS Task Completed (Audio not saved)"
	}

	if finalMessage != "" && shouldSave {
		fmt.Println(finalMessage)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "SUCCESS"
	result.Duration = resp.Duration

	return &result, nil
}

func (c *CLI) ASRUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	// Check if composite_model_name is provided in command
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
	}

	audioFile, ok := cmd.Params["audio_file"].(string)
	if !ok {
		return nil, fmt.Errorf("audio file not provided")
	}

	payload := map[string]interface{}{
		"file": audioFile,
	}

	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	asrConfigPayload := make(map[string]interface{})
	if paramStr, ok := cmd.Params["param_str"].(string); ok && paramStr != "" {
		var dynamicParams map[string]interface{}
		if err := json.Unmarshal([]byte(paramStr), &dynamicParams); err != nil {
			return nil, fmt.Errorf("param string must be valid JSON. Error: %w", err)
		}
		asrConfigPayload["params"] = dynamicParams
	}

	if len(asrConfigPayload) > 0 {
		payload["asr_config"] = asrConfigPayload
	}

	url := "/audio/transcriptions"

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to ASR document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ASR document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var rawResult struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}

	if err = json.Unmarshal(resp.Body, &rawResult); err != nil {
		return nil, fmt.Errorf("ASR document failed: invalid JSON (%w)", err)
	}

	if rawResult.Code != 0 {
		return nil, fmt.Errorf("%s", rawResult.Message)
	}

	var result CommonResponse
	result.Code = rawResult.Code
	result.Data = []map[string]interface{}{
		{"text": rawResult.Data["text"].(string)},
	}
	result.Duration = resp.Duration

	return &result, nil
}

func (c *CLI) OCRUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	// Check if composite_model_name is provided in command
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
	}
	var filename string
	var fileURL string
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

	payload := map[string]interface{}{}

	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	if fileContent != nil {
		payload["content"] = fileContent
	} else {
		payload["url"] = fileURL
	}

	url := "/file/ocr"

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
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

func (c *CLI) ParseFileUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName, modelName string
	var err error

	// Check if composite_model_name is provided in command
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		if c.CurrentModel == nil {
			return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
		}

		// Use current model if set
		if c.CurrentModel.ModelID != "" {
			modelID = c.CurrentModel.ModelID
		} else {
			providerName = c.CurrentModel.Provider
			instanceName = c.CurrentModel.Instance
			modelName = c.CurrentModel.Model
		}
	}

	var filename string
	var fileURL string
	var fileContent []byte

	filename, ok = cmd.Params["file"].(string)
	if ok {
		// For online file
		if strings.HasPrefix(filename, "http://") || strings.HasPrefix(filename, "https://") {
			fileURL = filename
		} else {
			// read file and convert to base64
			var err error
			fileContent, err = os.ReadFile(filename)
			if err != nil {
				return nil, fmt.Errorf("failed to read file: %w", err)
			}
		}
	} else {
		fileURL, ok = cmd.Params["url"].(string)
		if !ok {
			return nil, fmt.Errorf("file or url not provided")
		}
	}

	payload := map[string]interface{}{}

	if modelID == "" {
		payload["provider_name"] = providerName
		payload["instance_name"] = instanceName
		payload["model_name"] = modelName
	} else {
		payload["model_id"] = modelID
	}

	if fileContent != nil {
		payload["content"] = fileContent
	} else {
		payload["url"] = fileURL
	}

	url := "/file/parse"

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to PARSE document: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to PARSE document: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("PARSE document failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

func (c *CLI) ListTasksUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName string

	// Check if composite_instance_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_instance_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 2 {
			return nil, fmt.Errorf("model name must be in format 'instance@provider'")
		}
		providerName = names[1]
		instanceName = names[0]
	} else {
		return nil, fmt.Errorf("no provider name or instance name")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/tasks", providerName, instanceName)

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list tasks: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list tasks failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ShowTaskUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var providerName, instanceName string

	// Check if composite_instance_name is provided in command
	if compositeModelName, ok := cmd.Params["composite_instance_name"].(string); ok && compositeModelName != "" {
		names := strings.Split(compositeModelName, "@")
		if len(names) != 2 {
			return nil, fmt.Errorf("model name must be in format 'instance@provider'")
		}
		providerName = names[1]
		instanceName = names[0]
	} else {
		return nil, fmt.Errorf("no provider name or instance name")
	}

	taskID, ok := cmd.Params["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("task id not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/tasks/%s", providerName, instanceName, taskID)

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get task: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}
	var result TaskResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get task failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) CheckProviderConnection(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
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

func (c *CLI) CheckProviderWithKey(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok || providerName == "" {
		return nil, fmt.Errorf("provider name not provided")
	}
	region, ok := cmd.Params["region"].(string)
	if !ok || region == "" {
		return nil, fmt.Errorf("region not provided")
	}
	apiKey, ok := cmd.Params["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("api_key not provided")
	}
	baseURL, _ := cmd.Params["base_url"].(string)

	var apiKeyValue interface{}
	if apiKey != "" {
		apiKeyValue = apiKey
	} else {
		apiKeyValue = nil
	}

	url := fmt.Sprintf("/providers/%s/connection", providerName)

	payload := map[string]interface{}{
		"region":  region,
		"api_key": apiKeyValue,
	}
	if baseURL != "" {
		payload["base_url"] = baseURL
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "api", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to check provider connection with key: %w", err)
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
func (c *CLI) UseModel(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var modelName, instanceName, providerName string
	var err error
	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if ok {
		modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
		if err != nil {
			return nil, err
		}
	}

	modelID, ok := cmd.Params["model_id"].(string)
	if !ok {
		modelID = ""
	}

	if modelID == "" && compositeModelName == "" {
		return nil, fmt.Errorf("model name or ID not provided and no current model set. Use 'use model' command first")
	}

	c.CurrentModel = &CurrentModel{
		Provider: providerName,
		Instance: instanceName,
		Model:    modelName,
		ModelID:  modelID,
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = fmt.Sprintf("Current model set to: %s/%s/%s", c.CurrentModel.Provider, c.CurrentModel.Instance, c.CurrentModel.Model)
	return &result, nil
}

func (c *CLI) AddCustomModel(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
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

	models, ok := cmd.Params["models"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("model name not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/models", providerName, instanceName)

	payload := map[string]interface{}{
		"provider_name": providerName,
		"instance_name": instanceName,
		"models":        models,
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
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

// InsertChunksFromFile inserts chunks from a JSON file
func (c *CLI) InsertChunksFromFile(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}
	filePath, ok := cmd.Params["file_path"].(string)
	if !ok {
		return nil, fmt.Errorf("file_path not provided")
	}

	payload := map[string]interface{}{
		"file_path": filePath,
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/tenant/insert_chunks_from_file", "web", nil, payload)
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
func (c *CLI) InsertMetadataFromFile(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	filePath, ok := cmd.Params["file_path"].(string)
	if !ok {
		return nil, fmt.Errorf("file_path not provided")
	}

	payload := map[string]interface{}{
		"file_path": filePath,
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/tenant/insert_metadata_from_file", "web", nil, payload)
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
func (c *CLI) UpdateChunk(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	chunkID, ok := cmd.Params["chunk_id"].(string)
	if !ok {
		return nil, fmt.Errorf("chunk_id not provided")
	}

	docID, ok := cmd.Params["doc_id"].(string)
	if !ok {
		return nil, fmt.Errorf("doc_id not provided")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	// Look up dataset_id from dataset_name
	datasetID, err := c.getDatasetID(datasetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset ID: %w", err)
	}

	jsonBody, ok := cmd.Params["json_body"].(string)
	if !ok {
		return nil, fmt.Errorf("json_body not provided")
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/chunk/update", "web", nil, payload)
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

// GetChunk retrieves a chunk by ID
func (c *CLI) GetChunk(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	chunkID, ok := cmd.Params["chunk_id"].(string)
	if !ok {
		return nil, fmt.Errorf("chunk_id not provided")
	}

	docID, ok := cmd.Params["doc_id"].(string)
	if !ok {
		return nil, fmt.Errorf("doc_id not provided")
	}

	datasetID, ok := cmd.Params["dataset_id"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_id not provided")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", fmt.Sprintf("/datasets/%s/documents/%s/chunks/%s", datasetID, docID, chunkID), "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get chunk: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ChunkResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get chunk failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// SetMeta sets metadata for a document
func (c *CLI) SetMeta(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/document/set_meta", "web", nil, payload)
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

// DeleteMeta deletes metadata for a document
// If keys is provided, deletes specific keys; otherwise deletes entire document metadata
func (c *CLI) DeleteMeta(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	docID, ok := cmd.Params["doc_id"].(string)
	if !ok {
		return nil, fmt.Errorf("doc_id not provided")
	}

	payload := map[string]interface{}{
		"doc_id": docID,
	}

	// If keys provided, include in payload for deleting specific keys
	if keysJSON, ok := cmd.Params["keys"].(string); ok {
		payload["keys"] = keysJSON
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/document/delete_meta", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to delete metadata: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to delete metadata: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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
		result.Message = fmt.Sprintf("Success to delete metadata for document: %s", docID)
	} else {
		result.Message = fmt.Sprintf("Failed to delete metadata: %v", resJSON)
	}
	result.Duration = 0
	return &result, nil
}

// RmTags removes tags from chunks in a dataset
func (c *CLI) RmTags(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", "/datasets/"+kbID+"/tags", "web", nil, payload)
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
func (c *CLI) RemoveChunks(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	docID, ok := cmd.Params["doc_id"].(string)
	if !ok {
		return nil, fmt.Errorf("doc_id not provided")
	}

	// Look up dataset ID by name
	datasetID, err := c.getDatasetID(datasetName)
	if err != nil {
		return nil, fmt.Errorf("dataset not found: %w", err)
	}

	payload := map[string]interface{}{}

	// Check if delete_all is set
	if deleteAll, ok := cmd.Params["delete_all"].(bool); ok && deleteAll {
		payload["delete_all"] = true
	} else if chunkIDs, ok := cmd.Params["chunk_ids"].([]string); ok {
		payload["chunk_ids"] = chunkIDs
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", "/datasets/"+datasetID+"/documents/"+docID+"/chunks", "web", nil, payload)
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

func (c *CLI) ParseDocumentsUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetID, ok := cmd.Params["dataset_id"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_id not provided")
	}

	documents, ok := cmd.Params["documents"].([]string)
	if !ok {
		return nil, fmt.Errorf("documents not provided")
	}

	url := fmt.Sprintf("/datasets/%s/documents/parse", datasetID)

	payload := map[string]interface{}{
		"documents": documents,
	}

	// Normal mode
	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list documents: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list documents failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

func (c *CLI) UserParseLocalFile(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	filename, ok := cmd.Params["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("filename not provided")
	}
	visionModel, ok := cmd.Params["vision_model"].(string)
	if !ok {
		visionModel = ""
	}
	chatModel, ok := cmd.Params["chat_model"].(string)
	if !ok {
		chatModel = ""
	}
	asrModel, ok := cmd.Params["asr_model"].(string)
	if !ok {
		asrModel = ""
	}
	ocrModel, ok := cmd.Params["ocr_model"].(string)
	if !ok {
		ocrModel = ""
	}
	embeddingModel, ok := cmd.Params["embedding_model"].(string)
	if !ok {
		embeddingModel = ""
	}
	docParseModel, ok := cmd.Params["doc_parse_model"].(string)
	if !ok {
		docParseModel = ""
	}

	fileType := utility.GetFileType(filename)
	config := map[string]string{
		"lib_type": "office_oxide",
	}
	fileParser, err := parser.GetParser(fileType, config)
	if err != nil {
		return nil, err
	}

	fileContent, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read dsl file: %w", err)
	}

	if err = fileParser.Parse(filename, fileContent); err != nil {
		return nil, formatRequestError("parse local file", err)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = fmt.Sprintf("Success to parse local file %q, vision: %v, chat: %v, asr: %v, ocr: %v, embedding: %v, doc_parse: %v", filename, visionModel, chatModel, asrModel, ocrModel, embeddingModel, docParseModel)
	fmt.Println(result.Message)
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

func (c *CLI) ListUserIngestionTasks(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	datasetID, ok := cmd.Params["dataset_id"].(*string)
	if !ok {
		datasetID = nil
	}

	payload := map[string]interface{}{
		"dataset_id": datasetID,
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/datasets/ingestion/tasks", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to list ingestion tasks: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list ingestion tasks:: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list ingestion tasks: failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserStartIngestionCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	documentID, ok := cmd.Params["document_id"].(string)
	if !ok {
		return nil, fmt.Errorf("document_id not provided")
	}

	datasetID, ok := cmd.Params["dataset_id"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_id not provided")
	}

	payload := map[string]interface{}{
		"documents":  []string{documentID},
		"dataset_id": datasetID,
	}

	url := fmt.Sprintf("/datasets/%s/documents/parse", datasetID)

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to ingest file: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ingest file: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ingest file failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserStopIngestionCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	tasks, ok := cmd.Params["tasks"].([]string)
	if !ok {
		return nil, fmt.Errorf("uri not provided")
	}
	payload := map[string]interface{}{
		"tasks": tasks,
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PUT", "/datasets/ingestion/tasks", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to ingest file: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ingest file: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ingest file failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserRemoveTaskCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIToken == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	tasks, ok := cmd.Params["tasks"].([]string)
	if !ok {
		return nil, fmt.Errorf("tasks not provided")
	}

	payload := map[string]interface{}{
		"tasks": tasks,
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", "/datasets/ingestion/tasks", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove tasks: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to remove tasks: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("remove tasks failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ChunkCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var result ExplainResponse
	start := time.Now()

	filename, ok := cmd.Params["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("filename not provided")
	}
	dslFilename, ok := cmd.Params["dsl"].(string)
	if !ok {
		return nil, fmt.Errorf("dsl not provided")
	}
	dsl, err := os.ReadFile(dslFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read dsl file: %w", err)
	}

	explain, ok := cmd.Params["explain"].(bool)
	if !ok {
		explain = false
	}

	engine := ingestion.NewChunkEngine()
	plan, err := engine.Compile(string(dsl))
	if err != nil {
		return nil, fmt.Errorf("compile failed: %w", err)
	}

	if explain {

		explanation, err := engine.Explain(plan)
		if err != nil {
			return nil, fmt.Errorf("explain error: %w", err)
		}

		result.Message = explanation
	} else {
		fileToChunking, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		chunkContext, err := engine.Execute(plan, string(fileToChunking))
		if err != nil {
			return nil, fmt.Errorf("chunking error: %w", err)
		}

		for _, resultChunk := range chunkContext.ResultChunks {
			fmt.Printf("Chunk index: %d\n", resultChunk.Index)
			fmt.Printf("Chunk size: %d\n", resultChunk.Size)
			fmt.Printf("Chunk content: \n%s\n", resultChunk.Content)
		}
	}

	result.Duration = time.Since(start).Seconds()
	result.Code = 0
	result.Message = fmt.Sprintf("Success to chunk %s", filename)
	return &result, nil
}

// OpenaiChat dispatches the parsed OPENAI_CHAT command to either a
// non-streaming oneshot call or a streaming SSE call, depending on the
// `stream` option.
func (c *CLI) OpenaiChat(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("OPENAI_CHAT is only allowed in USER mode")
	}
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIToken == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	body, err := buildOpenaiChatRequestBody(cmd)
	if err != nil {
		return nil, err
	}

	chatID, _ := cmd.Params["chat_id"].(string)
	url := fmt.Sprintf("/openai/%s/chat/completions", chatID)

	stream, _ := cmd.Params["stream"].(bool)
	if stream {
		return c.streamOpenaiChat(url, body)
	}
	return c.oneshotOpenaiChat(url, body)
}

// allowedExtraBodyKeys enumerates every top-level key the server
// accepts under `extra_body`. Anything else is rejected at CLI
// build time so the user gets a clear error before the request
// goes over the wire.
var allowedExtraBodyKeys = map[string]struct{}{
	"reference":          {},
	"reference_metadata": {},
	"metadata_condition": {},
}

// validateExtraBody checks the shape of an extra_body payload
// supplied by the user. It rejects:
//
//   - Unknown top-level keys (typos and unsupported fields).
//   - reference_metadata that's not an object, or whose
//     sub-fields have the wrong type.
//   - metadata_condition that's not an object, or whose
//     conditions are missing required fields.
//
// The error message names the offending path so the user can
// fix the JSON literal in their command without having to read
// the server source.
func validateExtraBody(eb map[string]interface{}) error {
	for k := range eb {
		if _, ok := allowedExtraBodyKeys[k]; !ok {
			return fmt.Errorf("OPENAI_CHAT extra_body: unknown field %q (valid: reference, reference_metadata, metadata_condition)", k)
		}
	}

	// reference_metadata: { include?: bool, fields?: string[] }
	if v, present := eb["reference_metadata"]; present {
		rm, ok := v.(map[string]interface{})
		if !ok {
			return fmt.Errorf("OPENAI_CHAT extra_body.reference_metadata must be an object, got %T", v)
		}
		if inc, ok := rm["include"]; ok {
			if _, ok := inc.(bool); !ok {
				return fmt.Errorf("OPENAI_CHAT extra_body.reference_metadata.include must be a boolean, got %T", inc)
			}
		}
		if fields, ok := rm["fields"]; ok {
			arr, ok := fields.([]interface{})
			if !ok {
				return fmt.Errorf("OPENAI_CHAT extra_body.reference_metadata.fields must be an array, got %T", fields)
			}
			for i, item := range arr {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("OPENAI_CHAT extra_body.reference_metadata.fields[%d] must be a string, got %T", i, item)
				}
			}
		}
	}

	// metadata_condition: { logic?: "and"|"or", conditions?: [{key, operator, value}, ...] }
	if v, present := eb["metadata_condition"]; present {
		mc, ok := v.(map[string]interface{})
		if !ok {
			return fmt.Errorf("OPENAI_CHAT extra_body.metadata_condition must be an object, got %T", v)
		}
		if logic, ok := mc["logic"]; ok {
			s, ok := logic.(string)
			if !ok {
				return fmt.Errorf("OPENAI_CHAT extra_body.metadata_condition.logic must be a string, got %T", logic)
			}
			if s != "and" && s != "or" {
				return fmt.Errorf("OPENAI_CHAT extra_body.metadata_condition.logic must be \"and\" or \"or\", got %q", s)
			}
		}
		if conds, ok := mc["conditions"]; ok {
			arr, ok := conds.([]interface{})
			if !ok {
				return fmt.Errorf("OPENAI_CHAT extra_body.metadata_condition.conditions must be an array, got %T", conds)
			}
			for i, item := range arr {
				cond, ok := item.(map[string]interface{})
				if !ok {
					return fmt.Errorf("OPENAI_CHAT extra_body.metadata_condition.conditions[%d] must be an object, got %T", i, item)
				}
				if _, ok := cond["key"]; !ok {
					return fmt.Errorf("OPENAI_CHAT extra_body.metadata_condition.conditions[%d] missing required field 'key'", i)
				}
			}
		}
	}

	return nil
}

// buildOpenaiChatRequestBody assembles the JSON payload that
// /api/v1/openai/<chat_id>/chat/completions expects
//
// RAGFlow-specific knobs (e.g. `reference`, `reference_metadata`,
// `metadata_condition`) flow in via the user-supplied `extra_body`
// JSON literal, which is validated against the `allowedExtraBodyKeys`
// allowlist above before the request goes out. `stop` and `user` are
// not first-class CLI options — the Python server does not inspect
// them, and the Go server has dropped them from its request struct;
// the parser rejects them as "unknown option" so there is exactly
// one place to set them.
//
// The `messages` array is built from three optional sources, in
// this order:
//  1. `system`         — single system message (if supplied)
//  2. `history`        — prior turns encoded as
//     "user:...,assistant:..." (if supplied)
//  3. positional <msg> — always the trailing user turn
func buildOpenaiChatRequestBody(cmd *Command) (map[string]interface{}, error) {
	msg, _ := cmd.Params["message"].(string)
	model, _ := cmd.Params["model"].(string)
	temp, _ := cmd.Params["temperature"].(float64)
	maxTokens, _ := cmd.Params["max_tokens"].(int)
	stream, _ := cmd.Params["stream"].(bool)

	messages := make([]map[string]interface{}, 0, 4)
	if v, ok := cmd.Params["system"].(string); ok && v != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": v})
	}
	if v, ok := cmd.Params["history_raw"].(string); ok && v != "" {
		delimiter, _ := cmd.Params["history_delimiter"].(string)
		turns, err := parseHistory(v, delimiter)
		if err != nil {
			return nil, fmt.Errorf("OPENAI_CHAT history: %w", err)
		}
		for _, t := range turns {
			messages = append(messages, map[string]interface{}{
				"role":    t["role"],
				"content": t["content"],
			})
		}
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": msg})

	body := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}
	// Only emit generation params when the user actually set them
	// (zero is the parser-default for "unset" and matches Python's
	// behavior of dropping the field).
	if temp != 0.0 {
		body["temperature"] = temp
	}
	if maxTokens != 0 {
		body["max_tokens"] = maxTokens
	}
	if v, ok := cmd.Params["top_p"].(float64); ok && v != 0.0 {
		body["top_p"] = v
	}
	if v, ok := cmd.Params["frequency_penalty"].(float64); ok && v != 0.0 {
		body["frequency_penalty"] = v
	}
	if v, ok := cmd.Params["presence_penalty"].(float64); ok && v != 0.0 {
		body["presence_penalty"] = v
	}

	var extraBody map[string]interface{}
	if v, ok := cmd.Params["extra_body"].(string); ok && v != "" {
		if err := json.Unmarshal([]byte(v), &extraBody); err != nil {
			return nil, fmt.Errorf("OPENAI_CHAT extra_body: invalid JSON: %w", err)
		}
	}
	// Validate the user's extra_body against the server's accepted
	// schema before the request goes over the wire.
	if err := validateExtraBody(extraBody); err != nil {
		return nil, err
	}
	if len(extraBody) > 0 {
		body["extra_body"] = extraBody
	}

	return body, nil
}

// oneshotOpenaiChat performs a non-streaming POST and returns an
// OpenAIChatResponse parsed from the JSON envelope. It calls the
// same HTTPClient.Request used by every other CLI command.
func (c *CLI) oneshotOpenaiChat(url string, body map[string]interface{}) (ResponseIf, error) {
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("POST", url, "web", nil, body)
	if err != nil {
		return nil, fmt.Errorf("openai_chat request: %w", err)
	}
	if resp.StatusCode != 200 {
		// Python wraps errors as `{"code":..., "message":...}`. Surface
		// the body verbatim so the user can read the upstream error.
		return &OpenAIChatResponse{
			Code:    resp.StatusCode,
			Message: string(resp.Body),
			raw:     resp.Body,
		}, nil
	}
	out := &OpenAIChatResponse{
		Duration: resp.Duration,
		raw:      resp.Body,
	}
	var wrapped struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    *openAIChatData `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &wrapped); err == nil && wrapped.Data != nil {
		out.Code = wrapped.Code
		out.Message = wrapped.Message
		out.Data = wrapped.Data
		if len(wrapped.Data.Choices) > 0 {
			out.Reasoning = wrapped.Data.Choices[0].Message.ReasoningContent
		}
		return out, nil
	}
	// Unwrapped (Go handler) shape.
	if err := json.Unmarshal(resp.Body, &out.Data); err != nil {
		return nil, fmt.Errorf("openai_chat: invalid response JSON: %w", err)
	}
	if out.Data != nil && len(out.Data.Choices) > 0 {
		out.Reasoning = out.Data.Choices[0].Message.ReasoningContent
	}
	return out, nil
}

// streamOpenaiChat performs a streaming POST and prints SSE chunks to
// stdout as they arrive
func (c *CLI) streamOpenaiChat(url string, body map[string]interface{}) (ResponseIf, error) {
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("POST", url, "web", nil, body)
	if err != nil {
		return nil, fmt.Errorf("openai_chat stream: %w", err)
	}
	if resp.StatusCode != 200 {
		return &OpenAIChatResponse{
			Code:     resp.StatusCode,
			Message:  string(resp.Body),
			Duration: resp.Duration,
			raw:      resp.Body,
		}, nil
	}
	full := string(resp.Body)
	var (
		fullContent string
		fullReason  string
		resolvedMod string
	)
	for _, line := range strings.Split(full, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *openAIChatUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Model != "" {
			resolvedMod = chunk.Model
		}
		if len(chunk.Choices) > 0 {
			if d := chunk.Choices[0].Delta.Content; d != "" {
				fullContent += d
			}
			if r := chunk.Choices[0].Delta.ReasoningContent; r != "" {
				fullReason += r
			}
		}
	}

	fullContent = strings.TrimLeft(fullContent, "\n\r")
	fullReason = strings.TrimLeft(fullReason, "\n\r")
	fullContent = stripThinkTags(fullContent)
	fullReason = stripThinkTags(fullReason)
	return &OpenAIChatResponse{
		Duration:  resp.Duration,
		Reasoning: fullReason,
		Data: &openAIChatData{
			Model:   resolvedMod,
			Choices: []openAIChatChoice{{Message: openAIChatMessage{Content: fullContent, ReasoningContent: fullReason}}},
		},
		streamed: true,
		raw:      resp.Body,
	}, nil
}

// stripThinkTags removes <think>…</think> wrappers from a streamed answer
func stripThinkTags(s string) string {
	var thinkTagRE = regexp.MustCompile(`(?s)<think>.*?</think>`)
	return thinkTagRE.ReplaceAllString(s, "")
}
