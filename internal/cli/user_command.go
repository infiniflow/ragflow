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
	"ragflow/internal/parser/chunk"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
	"strings"
	"time"
)

// Show server version to show RAGFlow server version
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *CLI) APIShowVersionCommand(cmd *Command) (ResponseIf, error) {
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

func (c *CLI) APISetLogLevelCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	logLevel, ok := cmd.Params["level"].(string)
	if !ok {
		return nil, fmt.Errorf("no log level")
	}

	payload := map[string]interface{}{
		"level": logLevel,
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("PUT", "/system/config/log", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to change log level: %w", err)
	}

	return HandleSimpleResponse(resp, "change log level")
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

	publicKey, err := c.GetPublicKeyPEM()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password, publicKey)
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

// APIListDatasetsCommand lists datasets for current user (user mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *CLI) APIListDatasetsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if httpClient.useAPIKey {
		authKind = "api"
	}

	if httpClient.LoginToken != nil {
		authKind = "web"
	}

	// Normal mode
	resp, err := httpClient.Request("GET", "/datasets", authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	return HandleCommonResponse(resp, "list datasets")
}

func (c *CLI) APIListDatasetDocumentsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !httpClient.useAPIKey {
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

func (c *CLI) APIListDatasetFilesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !httpClient.useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("no dataset name")
	}

	datasetID, err := c.getDatasetIDByName(datasetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset id: %w", err)
	}

	url := fmt.Sprintf("/datasets/%s/documents", datasetID)

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

// APIListAgentsCommand lists agents
func (c *CLI) APIListAgentsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if httpClient.useAPIKey {
		authKind = "api"
	}

	if httpClient.LoginToken != nil {
		authKind = "web"
	}

	// Normal mode
	resp, err := httpClient.Request("GET", "/agents", authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list agents: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ListAgentsResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list agents failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// APIListChatsCommand lists chats
func (c *CLI) APIListChatsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if httpClient.useAPIKey {
		authKind = "api"
	}

	if httpClient.LoginToken != nil {
		authKind = "web"
	}

	// Normal mode
	resp, err := httpClient.Request("GET", "/chats", authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list chats: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list chats: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ListChatsResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list chats failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// APIListSearchesCommand lists searches
func (c *CLI) APIListSearchesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if httpClient.useAPIKey {
		authKind = "api"
	}

	if httpClient.LoginToken != nil {
		authKind = "web"
	}

	// Normal mode
	resp, err := httpClient.Request("GET", "/searches", authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list searches: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list searches: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ListSearchesResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list searches failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration

	return &result, nil
}

// APIListMemoriesCommand lists memories
func (c *CLI) APIListMemoriesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	authKind := "web"
	if httpClient.useAPIKey {
		authKind = "api"
	}

	if httpClient.LoginToken != nil {
		authKind = "web"
	}

	// Normal mode
	resp, err := httpClient.Request("GET", "/memories", authKind, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list memories: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ListMemoriesResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list memories failed: invalid JSON (%w)", err)
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
	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !httpClient.useAPIKey {
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

// DevGetMetadataCommand gets metadata for one or more datasets
func (c *CLI) DevGetMetadataCommand(cmd *Command) (ResponseIf, error) {
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

// APICreateAPIKeyCommand creates a new API key
func (c *CLI) APICreateAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	resp, err := httpClient.Request("POST", "/system/keys", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create key: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create key: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var createResult CommonDataResponse
	if err = json.Unmarshal(resp.Body, &createResult); err != nil {
		return nil, fmt.Errorf("create key failed: invalid JSON (%w)", err)
	}

	if createResult.Code != 0 {
		return nil, fmt.Errorf("%s", createResult.Message)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "API Key created successfully"
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) APICreateDatasetCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name parameter is required")
	}

	payload := map[string]interface{}{
		"name": datasetName,
	}

	resp, err := httpClient.Request("POST", "/datasets", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset: %w", err)
	}

	return HandleSimpleResponse(resp, "create dataset")
}

func (c *CLI) APICreateAgentCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	resp, err := httpClient.Request("POST", "/agents", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create agent: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var createResult CommonDataResponse
	if err = json.Unmarshal(resp.Body, &createResult); err != nil {
		return nil, fmt.Errorf("create agent failed: invalid JSON (%w)", err)
	}

	if createResult.Code != 0 {
		return nil, fmt.Errorf("%s", createResult.Message)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "Agent created successfully"
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) APICreateChatCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	resp, err := httpClient.Request("POST", "/chats", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create chat: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var createResult CommonDataResponse
	if err = json.Unmarshal(resp.Body, &createResult); err != nil {
		return nil, fmt.Errorf("create chat failed: invalid JSON (%w)", err)
	}

	if createResult.Code != 0 {
		return nil, fmt.Errorf("%s", createResult.Message)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "Chat created successfully"
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) APICreateSearchCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	searchName, ok := cmd.Params["search_name"].(string)
	if !ok {
		return nil, fmt.Errorf("search_name parameter is required")
	}

	payload := map[string]interface{}{
		"name": searchName,
	}

	resp, err := httpClient.Request("POST", "/searches", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create search: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create search: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var createResult CommonDataResponse
	if err = json.Unmarshal(resp.Body, &createResult); err != nil {
		return nil, fmt.Errorf("create search failed: invalid JSON (%w)", err)
	}

	if createResult.Code != 0 {
		return nil, fmt.Errorf("%s", createResult.Message)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "Search created successfully"
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) APICreateMemoryCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	// Determine auth kind based on whether API key is being used
	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	memoryName, ok := cmd.Params["memory_name"].(string)
	if !ok {
		return nil, fmt.Errorf("memory_name parameter is required")
	}

	payload := map[string]interface{}{
		"name": memoryName,
	}

	resp, err := httpClient.Request("POST", "/memories", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create memory: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var createResult CommonDataResponse
	if err = json.Unmarshal(resp.Body, &createResult); err != nil {
		return nil, fmt.Errorf("create memory failed: invalid JSON (%w)", err)
	}

	if createResult.Code != 0 {
		return nil, fmt.Errorf("%s", createResult.Message)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "Memory created successfully"
	result.Duration = resp.Duration
	return &result, nil
}

// APIListAPIKeysCommand lists all API keys for the current user
func (c *CLI) APIListAPIKeysCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("GET", "/system/keys", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	return HandleCommonResponse(resp, "list keys")
}

// APIDeleteAPIKeyCommand deletes an API key
func (c *CLI) APIDeleteAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	apiKey, ok := cmd.Params["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("key not provided")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("DELETE", fmt.Sprintf("/system/keys/%s", apiKey), "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to delete key: %w", err)
	}

	return HandleSimpleResponse(resp, "delete key")
}

// APISetAPIKeyCommand sets the API key after validating it
func (c *CLI) APISetAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	apiKey, ok := cmd.Params["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("key not provided")
	}

	// Save current token to restore if validation fails
	savedToken := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey
	savedUseAPIToken := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey

	// Set the new key temporarily for validation
	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey = &apiKey
	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey = true

	// Validate token by calling list tokens API
	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/system/tokens", "api", nil, nil)
	if err != nil {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey = savedUseAPIToken
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if resp.StatusCode != 200 {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		// Restore original token on error
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey = savedToken
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey = savedUseAPIToken
		return nil, fmt.Errorf("token validation failed: %s", result.Message)
	}

	// Token is valid, keep it set
	var successResult SimpleResponse
	successResult.Code = 0
	successResult.Message = "API key set successfully"
	successResult.Duration = resp.Duration
	return &successResult, nil
}

// APISetVariableCommand sets variable value
func (c *CLI) APISetVariableCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	varName, ok := cmd.Params["var_name"].(string)
	if !ok {
		return nil, fmt.Errorf("var_name not provided")
	}
	varValue, ok := cmd.Params["var_value"].(string)
	if !ok {
		return nil, fmt.Errorf("var_value not provided")
	}

	payload := map[string]interface{}{
		"var_name":  varName,
		"var_value": varValue,
	}
	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PUT", "/system/variables", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to set variable: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to set variable: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result MessageResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("set variable failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// APIShowVariableCommand displays variable value
func (c *CLI) APIShowVariableCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	varName, ok := cmd.Params["var_name"].(string)
	if !ok {
		return nil, fmt.Errorf("var_name not provided")
	}

	EncodedVarName := common.EncodeToBase64(varName)

	endPoint := fmt.Sprintf("/system/variables/%s", EncodedVarName)

	resp, err := httpClient.Request("GET", endPoint, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get variable: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get variable: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show variable failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	normalizeVariableRows(result.Data)
	result.Duration = resp.Duration
	return &result, nil
}

// APIShowAPIKeyCommand displays the current API key
func (c *CLI) APIShowAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil {
		return nil, fmt.Errorf("no API key is currently set")
	}

	var result CommonDataResponse
	result.Code = 0
	result.Message = ""
	result.Data = map[string]interface{}{
		"token": *c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey,
	}
	result.Duration = 0
	return &result, nil
}

// APIUnsetAPIKeyCommand removes the current API key
func (c *CLI) APIUnsetAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil {
		return nil, fmt.Errorf("no API key is currently set")
	}

	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey = nil
	c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey = false

	var result SimpleResponse
	result.Code = 0
	result.Message = "API key unset successfully"
	result.Duration = 0
	return &result, nil
}

// DevCreateChunkStoreCommand creates a chunk store in doc engine
func (c *CLI) DevCreateChunkStoreCommand(cmd *Command) (ResponseIf, error) {
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

// DevDropChunkStoreCommand drops a chunk store in doc engine
func (c *CLI) DevDropChunkStoreCommand(cmd *Command) (ResponseIf, error) {
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

// DevCreateMetadataStoreCommand creates the document metadata store for the tenant
func (c *CLI) DevCreateMetadataStoreCommand(cmd *Command) (ResponseIf, error) {
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

// DevDropMetadataStoreCommand drops the document metadata store for the tenant
func (c *CLI) DevDropMetadataStoreCommand(cmd *Command) (ResponseIf, error) {
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

// APIAddProviderCommand creates a new model provider
// ADD PROVIDER <name>
func (c *CLI) APIAddProviderCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	// Build payload
	payload := map[string]interface{}{
		"provider_name": providerName,
	}

	resp, err := httpClient.Request("PUT", "/providers", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	return HandleSimpleResponse(resp, "add provider")
}

// APIListProvidersCommand lists added providers
func (c *CLI) APIListProvidersCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/providers", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	return HandleCommonResponse(resp, "list providers")
}

// APIDeleteProviderCommand deletes a provider
// DELETE PROVIDER <name>
func (c *CLI) APIDeleteProviderCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	url := fmt.Sprintf("/providers/%s", providerName)

	resp, err := httpClient.Request("DELETE", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to delete provider: %w", err)
	}

	return HandleSimpleResponse(resp, "delete provider")
}

// APIDropDatasetCommand DROP DATASET 'dataset_name'
func (c *CLI) APIDropDatasetCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	datasetName, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name parameter is required")
	}

	datasetID, err := c.getDatasetIDByName(datasetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset ID: %w by dataset name: %s", err, datasetName)
	}

	payload := map[string]interface{}{
		"ids":        []string{datasetID},
		"delete_all": true,
	}

	resp, err := httpClient.Request("DELETE", "/datasets", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop dataset: %w", err)
	}

	return HandleSimpleResponse(resp, "drop dataset")
}

// APIDropAgentCommand DROP AGENT 'agent_name'
func (c *CLI) APIDropAgentCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	agentName, ok := cmd.Params["agent_name"].(string)
	if !ok {
		return nil, fmt.Errorf("agent_name parameter is required")
	}

	agentID, err := c.getAgentIDByName(agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent ID: %w by agent name: %s", err, agentName)
	}

	payload := map[string]interface{}{
		"ids":        []string{agentID},
		"delete_all": true,
	}

	resp, err := httpClient.Request("DELETE", "/agents", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop agent: %w", err)
	}

	return HandleSimpleResponse(resp, "drop agent")
}

// APIDropChatCommand DROP CHAT 'chat_name'
func (c *CLI) APIDropChatCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	chatName, ok := cmd.Params["chat_name"].(string)
	if !ok {
		return nil, fmt.Errorf("chat_name parameter is required")
	}

	chatID, err := c.getChatIDByName(chatName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat ID: %w by chat name: %s", err, chatName)
	}

	payload := map[string]interface{}{
		"ids":        []string{chatID},
		"delete_all": true,
	}

	resp, err := httpClient.Request("DELETE", "/chats", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to drop chat: %w", err)
	}

	return HandleSimpleResponse(resp, "drop chat")
}

// APIDropSearchCommand DROP SEARCH 'search_name'
func (c *CLI) APIDropSearchCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	searchName, ok := cmd.Params["search_name"].(string)
	if !ok {
		return nil, fmt.Errorf("search_name parameter is required")
	}

	searchID, err := c.getSearchIDByName(searchName)
	if err != nil {
		return nil, fmt.Errorf("failed to get search ID: %w by search name: %s", err, searchName)
	}

	endPoint := fmt.Sprintf("/searches/%s", searchID)

	resp, err := httpClient.Request("DELETE", endPoint, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop search: %w", err)
	}

	return HandleSimpleResponse(resp, "drop search")
}

// APIDropMemoryCommand DROP MEMORY 'memory_name'
func (c *CLI) APIDropMemoryCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

	if httpClient.LoginToken == nil && !c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].useAPIKey {
		return nil, fmt.Errorf("no authorization")
	}

	memoryName, ok := cmd.Params["memory_name"].(string)
	if !ok {
		return nil, fmt.Errorf("memory_name parameter is required")
	}

	memoryID, err := c.getMemoryIDByName(memoryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory ID: %w by memory name: %s", err, memoryName)
	}

	endPoint := fmt.Sprintf("/memories/%s", memoryID)

	resp, err := httpClient.Request("DELETE", endPoint, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop memory: %w", err)
	}

	return HandleSimpleResponse(resp, "drop memory")
}

// APIAddProviderInstanceCommand creates a new provider instance
// CREATE PROVIDER <name> INSTANCE <instance_name> KEY <api_key> URL <base_url> REGION <region>
func (c *CLI) APIAddProviderInstanceCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	resp, err := httpClient.Request("POST", url, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add provider instance: %w", err)
	}

	return HandleSimpleResponse(resp, "add provider instance")
}

// DELETE PROVIDER <name> INSTANCE <name>
func (c *CLI) APIDeleteProviderInstanceCommand(cmd *Command) (ResponseIf, error) {
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
		return nil, fmt.Errorf("failed to drop provider instance: %w", err)
	}

	return HandleSimpleResponse(resp, "drop provider instance")
}

// DELETE PROVIDER <name> INSTANCE <instance_name> MODELS <name1 name2 name3>
func (c *CLI) APIDeleteProviderInstanceModelCommand(cmd *Command) (ResponseIf, error) {
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
		return nil, fmt.Errorf("failed to delete model: %w", err)
	}

	return HandleSimpleResponse(resp, "delete model")
}

func isValidURL(str string) bool {
	u, err := netUrl.Parse(str)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func (c *CLI) APIChatToModelCommand(cmd *Command) (ResponseIf, error) {
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

	url := "/chat/to_model"

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

func (c *CLI) EmbedUserTextCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

func (c *CLI) APIRerankUserDocumentCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	return HandleCommonResponse(resp, "rerank document")
}

func (c *CLI) APITTSUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

func (c *CLI) APIASRUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

func (c *CLI) APIOCRUserCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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
	return HandleCommonDataResponse(resp, "OCR document")
}

func (c *CLI) APIModelParseFileCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	return HandleCommonDataResponse(resp, "PARSE document")
}

func (c *CLI) APIListModelInstanceTasksCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("no provider name")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("no instance name")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/tasks", providerName, instanceName)

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", url, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list model parsing tasks: %w", err)
	}

	return HandleCommonResponse(resp, "list model parsing tasks")
}

// APIShowProviderInstanceTaskCommand shows the details of a task
func (c *CLI) APIShowProviderInstanceTaskCommand(cmd *Command) (ResponseIf, error) {

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("no provider name")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("no instance name")
	}

	taskID, ok := cmd.Params["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("task id not provided")
	}

	url := fmt.Sprintf("/providers/%s/instances/%s/tasks/%s", providerName, instanceName, taskID)

	resp, err := httpClient.Request("GET", url, "web", nil, nil)
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

// APIUseModelCommand sets the current model for chat
func (c *CLI) APIUseModelCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

func (c *CLI) APIAddCustomModelCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	return HandleSimpleResponse(resp, "add custom model")
}

// DevInsertChunksFromFileCommand inserts chunks from a JSON file
func (c *CLI) DevInsertChunksFromFileCommand(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/tenant/dev_insert_chunks_from_file", "web", nil, payload)
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

// DevInsertMetadataFromFileCommand inserts metadata from a JSON file
func (c *CLI) DevInsertMetadataFromFileCommand(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/tenant/dev_insert_metadata_from_file", "web", nil, payload)
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

// DevUpdateChunkCommand updates a chunk in a dataset
func (c *CLI) DevUpdateChunkCommand(cmd *Command) (ResponseIf, error) {
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

// DevGetChunkCommand retrieves a chunk by ID
func (c *CLI) DevGetChunkCommand(cmd *Command) (ResponseIf, error) {
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

// DevSetMetaCommand sets metadata for a document
func (c *CLI) DevSetMetaCommand(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/document/dev_set_meta", "web", nil, payload)
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

// DevDeleteMetaCommand deletes metadata for a document
// If keys is provided, deletes specific keys; otherwise deletes entire document metadata
func (c *CLI) DevDeleteMetaCommand(cmd *Command) (ResponseIf, error) {
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

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/document/dev_delete_meta", "web", nil, payload)
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

// DevRmTagsCommand removes tags from chunks in a dataset
func (c *CLI) DevRmTagsCommand(cmd *Command) (ResponseIf, error) {
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

// DevRemoveChunksCommand removes chunks from a document
func (c *CLI) DevRemoveChunksCommand(cmd *Command) (ResponseIf, error) {
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
			var count float64
			if count, ok = data["deleted_count"].(float64); ok {
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

func (c *CLI) APIParseDocumentsCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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
		return nil, fmt.Errorf("failed to parse documents: %w", err)
	}

	return HandleSimpleResponse(resp, "parse documents")
}

func (c *CLI) APIParseLocalFileCommand(cmd *Command) (ResponseIf, error) {
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

	parseResult := fileParser.ParseWithResult(filename, fileContent)
	if parseResult.Err != nil {
		return nil, formatRequestError("parse local file", parseResult.Err)
	}

	var result SimpleResponse
	result.Code = 0
	// codeql[go/clear-text-logging] False positive: filename is
	// reduced to filepath.Base(...) so the full path (which can
	// contain user-identifying directory components) never reaches
	// the log. The format is operator-facing status output, not a
	// server log.
	result.Message = fmt.Sprintf("Success to parse local file %q, vision: %v, chat: %v, asr: %v, ocr: %v, embedding: %v, doc_parse: %v", filepath.Base(filename), visionModel, chatModel, asrModel, ocrModel, embeddingModel, docParseModel)
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

func (c *CLI) APIListIngestionTasks(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	return HandleCommonResponse(resp, "list ingestion tasks")
}

// APIShowLogLevelCommand sets the log level for the system.
func (c *CLI) APIShowLogLevelCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/system/config/log", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get log level config: %w", err)
	}

	return HandleCommonDataResponse(resp, "get log level config")

}

// APIListEnvironmentsCommand lists all system environments (api mode only).
func (c *CLI) APIListEnvironmentsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/system/environments", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	return HandleCommonResponse(resp, "list environments")
}

// APIListVariablesCommand lists all system variables (api mode only).
func (c *CLI) APIListVariablesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
	}

	resp, err := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/system/variables", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list variables: %w", err)
	}

	return HandleCommonResponse(resp, "list variables")
}

func (c *CLI) APIStartIngestionCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	return HandleCommonResponse(resp, "ingest file")
}

func (c *CLI) APIStopIngestionCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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
		return nil, fmt.Errorf("failed to stop ingestion: %w", err)
	}

	return HandleCommonResponse(resp, "stop ingestion")
}

func (c *CLI) APIRemoveTaskCommand(cmd *Command) (ResponseIf, error) {
	if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

	return HandleCommonResponse(resp, "remove tasks")
}

func (c *CLI) DevChunkCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	var result ExplainResponse
	start := time.Now()

	filename, ok := cmd.Params["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("filename not provided")
	}
	optionsFilename, ok := cmd.Params["dsl"].(string)
	if !ok {
		return nil, fmt.Errorf("chunk options file not provided")
	}
	optionsRaw, err := os.ReadFile(optionsFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk options file: %w", err)
	}
	options, err := parseChunkOptions(optionsRaw)
	if err != nil {
		return nil, err
	}

	explain, ok := cmd.Params["explain"].(bool)
	if !ok {
		explain = false
	}

	if explain {
		explanation, err := explainChunkOptions(options)
		if err != nil {
			return nil, fmt.Errorf("explain error: %w", err)
		}
		result.Message = explanation
	} else {
		fileToChunking, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		chunkContext, err := chunk.Run(string(fileToChunking), options)
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

func parseChunkOptions(raw []byte) (chunk.ChunkOptions, error) {
	var options chunk.ChunkOptions
	if err := json.Unmarshal(raw, &options); err != nil {
		return options, fmt.Errorf("failed to parse chunk options file: %w", err)
	}
	return options, nil
}

func explainChunkOptions(options chunk.ChunkOptions) (string, error) {
	formatted, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// APIOpenaiChatCommand dispatches the parsed OPENAI_CHAT command to either a
// non-streaming oneshot call or a streaming SSE call, depending on the
// `stream` option.
func (c *CLI) APIOpenaiChatCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("OPENAI_CHAT is only allowed in USER mode")
	}
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API key not set. Please login first")
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

// ChatCompletions dispatches the parsed CHAT COMPLETIONS command to
// POST /api/v1/chat/completions.
func (c *CLI) ChatCompletions(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("CHAT COMPLETIONS is only allowed in USER mode")
	}
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient.APIKey == nil && httpClient.LoginToken == nil {
		return nil, fmt.Errorf("API token not set. Please login first")
	}

	body, err := buildChatCompletionsRequestBody(cmd)
	if err != nil {
		return nil, err
	}

	url := "/chat/completions"

	stream, _ := cmd.Params["stream"].(bool)
	if stream {
		return c.streamChatCompletions(url, body)
	}
	return c.oneshotChatCompletions(url, body)
}

// buildChatCompletionsRequestBody assembles the JSON payload for
// POST /api/v1/chat/completions.
//
// When system or history is provided, a `messages` array is built;
// otherwise just `question` is sent and the server normalizes it.
func buildChatCompletionsRequestBody(cmd *Command) (map[string]interface{}, error) {
	chatID, _ := cmd.Params["chat_id"].(string)
	question, _ := cmd.Params["question"].(string)
	stream, _ := cmd.Params["stream"].(bool)

	body := map[string]interface{}{
		"chat_id": chatID,
		"stream":  stream,
	}

	// Optional session_id
	if v, ok := cmd.Params["session"].(string); ok && v != "" {
		body["session_id"] = v
	}

	// Optional llm_id
	if v, ok := cmd.Params["llm"].(string); ok && v != "" {
		body["llm_id"] = v
	}

	// Build messages from system + history when provided; otherwise send question.
	system, hasSystem := cmd.Params["system"].(string)
	historyRaw, hasHistory := cmd.Params["history_raw"].(string)

	if hasSystem || hasHistory {
		messages := make([]map[string]interface{}, 0, 4)
		if hasSystem && system != "" {
			messages = append(messages, map[string]interface{}{"role": "system", "content": system})
		}
		if hasHistory && historyRaw != "" {
			delimiter, _ := cmd.Params["history_delimiter"].(string)
			turns, err := parseHistory(historyRaw, delimiter)
			if err != nil {
				return nil, fmt.Errorf("CHAT COMPLETIONS history: %w", err)
			}
			for _, t := range turns {
				messages = append(messages, map[string]interface{}{
					"role":    t["role"],
					"content": t["content"],
				})
			}
		}
		messages = append(messages, map[string]interface{}{"role": "user", "content": question})
		body["messages"] = messages
	} else {
		body["question"] = question
	}

	// Optional flags — only emit when explicitly set
	if isSet(cmd, "pass_all_history") && cmd.Params["pass_all_history"].(bool) {
		body["pass_all_history_messages"] = true
	}
	if isSet(cmd, "legacy") && cmd.Params["legacy"].(bool) {
		body["legacy"] = true
	}

	// Generation params — only emit when explicitly set
	if isSet(cmd, "temperature") {
		body["temperature"] = cmd.Params["temperature"]
	}
	if isSet(cmd, "max_tokens") {
		body["max_tokens"] = cmd.Params["max_tokens"]
	}
	if isSet(cmd, "top_p") {
		body["top_p"] = cmd.Params["top_p"]
	}
	if isSet(cmd, "frequency_penalty") {
		body["frequency_penalty"] = cmd.Params["frequency_penalty"]
	}
	if isSet(cmd, "presence_penalty") {
		body["presence_penalty"] = cmd.Params["presence_penalty"]
	}

	return body, nil
}

// oneshotChatCompletions performs a non-streaming POST and returns a
// ChatCompletionsResponse parsed from the RAGFlow-internal JSON envelope.
func (c *CLI) oneshotChatCompletions(url string, body map[string]interface{}) (ResponseIf, error) {
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	resp, err := httpClient.Request("POST", url, "web", nil, body)
	if err != nil {
		return nil, fmt.Errorf("chat completions request: %w", err)
	}
	if resp.StatusCode != 200 {
		return &ChatCompletionsResponse{
			Code:    resp.StatusCode,
			Message: string(resp.Body),
			raw:     resp.Body,
		}, nil
	}
	out := &ChatCompletionsResponse{
		Duration: resp.Duration,
		raw:      resp.Body,
	}
	// RAGFlow returns {code, data: {answer, reference, ...}, message}.
	var envelope struct {
		Code    int                 `json:"code"`
		Message string              `json:"message"`
		Data    *chatCompletionData `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("chat completions: invalid response JSON: %w", err)
	}
	out.Code = envelope.Code
	out.Message = envelope.Message
	out.Data = envelope.Data
	return out, nil
}

// streamChatCompletions performs a streaming POST and collects SSE chunks.
func (c *CLI) streamChatCompletions(url string, body map[string]interface{}) (ResponseIf, error) {
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	reader, err := httpClient.RequestStream("POST", url, "web", nil, body)
	if err != nil {
		return nil, fmt.Errorf("chat completions stream: %w", err)
	}
	defer reader.Close()

	start := time.Now()
	scanner := bufio.NewScanner(reader)
	var fullContent string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimPrefix(line, "data:")
		payload = strings.TrimSpace(payload)
		if payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Code    int                `json:"code"`
			Message string             `json:"message"`
			Data    chatCompletionData `json:"data"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Data.Answer != "" {
			fullContent += chunk.Data.Answer
		}
	}

	fullContent = strings.TrimLeft(fullContent, "\n\r")
	return &ChatCompletionsResponse{
		Duration: time.Since(start).Seconds(),
		Data: &chatCompletionData{
			Answer: fullContent,
		},
		streamed: true,
	}, nil
}
