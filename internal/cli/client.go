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
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	ce "ragflow/internal/cli/filesystem"
)

// PasswordPromptFunc is a function type for password input
type PasswordPromptFunc func(prompt string) (string, error)

// RAGFlowClient handles API interactions with the RAGFlow server
type RAGFlowClient struct {
	HTTPClient     *HTTPClient
	ServerType     string             // "admin" or "user"
	PasswordPrompt PasswordPromptFunc // Function for password input
	OutputFormat   OutputFormat       // Output format: table, plain, json
	FilesystemEngine *ce.Engine       // Filesystem engine for virtual filesystem
}

// NewRAGFlowClient creates a new RAGFlow client
func NewRAGFlowClient(serverType string) *RAGFlowClient {
	httpClient := NewHTTPClient()
	// Set port from configuration file based on server type
	if serverType == "admin" {
		httpClient.Port = 9381
	} else {
		httpClient.Port = 9380
	}

	client := &RAGFlowClient{
		HTTPClient: httpClient,
		ServerType: serverType,
	}

	// Initialize Filesystem Engine
	client.initFilesystemEngine()

	return client
}

// initFilesystemEngine initializes the Filesystem Engine with all providers
func (c *RAGFlowClient) initFilesystemEngine() {
	engine := ce.NewEngine()

	// Register providers
	engine.RegisterProvider(ce.NewDatasetProvider(&httpClientAdapter{c.HTTPClient}))

	c.FilesystemEngine = engine
}

// httpClientAdapter adapts HTTPClient to ce.HTTPClientInterface
type httpClientAdapter struct {
	client *HTTPClient
}

func (a *httpClientAdapter) Request(method, path string, useAPIBase bool, authKind string, headers map[string]string, jsonBody map[string]interface{}) (*ce.HTTPResponse, error) {
	// Auto-detect auth kind based on available tokens
	// If authKind is "auto" or empty, determine based on token availability
	if authKind == "auto" || authKind == "" {
		if a.client.useAPIToken && a.client.APIToken != "" {
			authKind = "api"
		} else if a.client.LoginToken != "" {
			authKind = "web"
		} else {
			authKind = "web" // default
		}
	}
	resp, err := a.client.Request(method, path, useAPIBase, authKind, headers, jsonBody)
	if err != nil {
		return nil, err
	}
	return &ce.HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
		Headers:    resp.Headers,
		Duration:   resp.Duration,
	}, nil
}

// ExecuteCommand executes a parsed command
// Returns benchmark result map for commands that support it (e.g., ping_server with iterations > 1)
func (c *RAGFlowClient) ExecuteCommand(cmd *Command) (ResponseIf, error) {
	switch c.ServerType {
	case "admin":
		// Admin mode: execute command with admin privileges
		return c.ExecuteAdminCommand(cmd)
	case "user":
		// User mode: execute command with user privileges
		return c.ExecuteUserCommand(cmd)
	default:
		return nil, fmt.Errorf("invalid server type: %s", c.ServerType)
	}
}

func (c *RAGFlowClient) ExecuteAdminCommand(cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "login_user":
		return nil, c.LoginUser(cmd)
	case "logout":
		return c.Logout()
	case "ping":
		return c.PingAdmin(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(cmd)
	case "list_users":
		return c.ListUsers(cmd)
	case "list_services":
		return c.ListServices(cmd)
	case "grant_admin":
		return c.GrantAdmin(cmd)
	case "revoke_admin":
		return c.RevokeAdmin(cmd)
	case "create_user":
		return c.CreateUser(cmd)
	case "activate_user":
		return c.ActivateUser(cmd)
	case "alter_user":
		return c.AlterUserPassword(cmd)
	case "drop_user":
		return c.DropUser(cmd)
	case "show_service":
		return c.ShowService(cmd)
	case "show_version":
		return c.ShowAdminVersion(cmd)
	case "show_user":
		return c.ShowUser(cmd)
	case "list_datasets":
		return c.ListDatasets(cmd)
	case "list_agents":
		return c.ListAgents(cmd)
	case "generate_token":
		return c.GenerateAdminToken(cmd)
	case "list_tokens":
		return c.ListAdminTokens(cmd)
	case "drop_token":
		return c.DropAdminToken(cmd)
	case "list_available_providers":
		return c.ListAvailableProviders(cmd)
	case "show_provider":
		return c.ShowProvider(cmd)
	case "list_provider_models":
		return c.ListModels(cmd)
	case "show_model":
		return c.ShowModel(cmd)
	// TODO: Implement other commands
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}
func (c *RAGFlowClient) ExecuteUserCommand(cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "register_user":
		return c.RegisterUser(cmd)
	case "login_user":
		return nil, c.LoginUser(cmd)
	case "logout":
		return c.Logout()
	case "ping":
		return c.PingServer(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(cmd)
	case "search_on_datasets":
		return c.SearchOnDatasets(cmd)
	case "create_token":
		return c.CreateToken(cmd)
	case "list_tokens":
		return c.ListTokens(cmd)
	case "drop_token":
		return c.DropToken(cmd)
	case "set_token":
		return c.SetToken(cmd)
	case "show_token":
		return c.ShowToken(cmd)
	case "unset_token":
		return c.UnsetToken(cmd)
	case "show_version":
		return c.ShowServerVersion(cmd)
	case "create_index":
		return c.CreateIndex(cmd)
	case "drop_index":
		return c.DropIndex(cmd)
	case "create_doc_meta_index":
		return c.CreateDocMetaIndex(cmd)
	case "drop_doc_meta_index":
		return c.DropDocMetaIndex(cmd)
	case "list_pool_providers":
		return c.ListAvailableProviders(cmd)
	case "show_provider":
		return c.ShowProvider(cmd)
	case "list_provider_models":
		return c.ListModels(cmd)
	case "show_model":
		return c.ShowModel(cmd)
	// Provider commands
	case "create_provider":
		return c.CreateProvider(cmd)
	case "list_providers":
		return c.ListProviders(cmd)
	case "drop_provider":
		return c.DropProvider(cmd)
	// ContextEngine commands
	case "ce_ls":
		return c.CEList(cmd)
	case "ce_search":
		return c.CESearch(cmd)
	case "insert_dataset_from_file":
		return c.InsertDatasetFromFile(cmd)
	case "insert_metadata_from_file":
		return c.InsertMetadataFromFile(cmd)
	// TODO: Implement other commands
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}

// ShowCurrentUser shows the current logged-in user information
// TODO: Implement showing current user information when API is available
func (c *RAGFlowClient) ShowCurrentUser(cmd *Command) (map[string]interface{}, error) {
	// TODO: Call the appropriate API to get current user information
	// Currently there is no /admin/user/info or /user/info API available
	// The /admin/auth API only verifies authorization, does not return user info
	return nil, fmt.Errorf("command 'SHOW CURRENT USER' is not yet implemented")
}

type ResponseIf interface {
	Type() string
	PrintOut()
	TimeCost() float64
	SetOutputFormat(format OutputFormat)
}

type CommonResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Message      string                   `json:"message"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *CommonResponse) Type() string {
	return "common"
}

func (r *CommonResponse) TimeCost() float64 {
	return r.Duration
}

func (r *CommonResponse) SetOutputFormat(format OutputFormat) {
	r.outputFormat = format
}

func (r *CommonResponse) PrintOut() {
	if r.Code == 0 {
		PrintTableSimpleByFormat(r.Data, r.outputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type CommonDataResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *CommonDataResponse) Type() string {
	return "show"
}

func (r *CommonDataResponse) TimeCost() float64 {
	return r.Duration
}

func (r *CommonDataResponse) SetOutputFormat(format OutputFormat) {
	r.outputFormat = format
}

func (r *CommonDataResponse) PrintOut() {
	if r.Code == 0 {
		table := make([]map[string]interface{}, 0)
		table = append(table, r.Data)
		PrintTableSimpleByFormat(table, r.outputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type SimpleResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *SimpleResponse) Type() string {
	return "simple"
}

func (r *SimpleResponse) TimeCost() float64 {
	return r.Duration
}

func (r *SimpleResponse) SetOutputFormat(format OutputFormat) {
	r.outputFormat = format
}

func (r *SimpleResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println("SUCCESS")
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type RegisterResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *RegisterResponse) Type() string {
	return "register"
}

func (r *RegisterResponse) TimeCost() float64 {
	return r.Duration
}

func (r *RegisterResponse) SetOutputFormat(format OutputFormat) {
	r.outputFormat = format
}

func (r *RegisterResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println("Register successfully")
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type BenchmarkResponse struct {
	Code         int     `json:"code"`
	Duration     float64 `json:"duration"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	Concurrency  int
	outputFormat OutputFormat
}

func (r *BenchmarkResponse) Type() string {
	return "benchmark"
}

func (r *BenchmarkResponse) SetOutputFormat(format OutputFormat) {
	r.outputFormat = format
}

func (r *BenchmarkResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Printf("ERROR, Code: %d\n", r.Code)
		return
	}

	iterations := r.SuccessCount + r.FailureCount
	if r.Concurrency == 1 {
		if iterations == 1 {
			fmt.Printf("Latency: %fs\n", r.Duration)
		} else {
			fmt.Printf("Latency: %fs, QPS: %.1f, SUCCESS: %d, FAILURE: %d\n", r.Duration, float64(iterations)/r.Duration, r.SuccessCount, r.FailureCount)
		}
	} else {
		fmt.Printf("Concurrency: %d, Latency: %fs, QPS: %.1f, SUCCESS: %d, FAILURE: %d\n", r.Concurrency, r.Duration, float64(iterations)/r.Duration, r.SuccessCount, r.FailureCount)
	}
}

func (r *BenchmarkResponse) TimeCost() float64 {
	return r.Duration
}

type KeyValueResponse struct {
	Code         int    `json:"code"`
	Key          string `json:"key"`
	Value        string `json:"data"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *KeyValueResponse) Type() string {
	return "data"
}

func (r *KeyValueResponse) TimeCost() float64 {
	return r.Duration
}

func (r *KeyValueResponse) SetOutputFormat(format OutputFormat) {
	r.outputFormat = format
}

func (r *KeyValueResponse) PrintOut() {
	if r.Code == 0 {
		table := make([]map[string]interface{}, 0)
		// insert r.key and r.value into table
		table = append(table, map[string]interface{}{
			"key":   r.Key,
			"value": r.Value,
		})
		PrintTableSimpleByFormat(table, r.outputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d\n", r.Code)
	}
}

// ==================== Filesystem Commands ====================

// CEListResponse represents the response for ls command
type CEListResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Message      string                   `json:"message"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *CEListResponse) Type() string { return "ce_ls" }
func (r *CEListResponse) TimeCost() float64 { return r.Duration }
func (r *CEListResponse) SetOutputFormat(format OutputFormat) { r.outputFormat = format }
func (r *CEListResponse) PrintOut() {
	if r.Code == 0 {
		PrintTableSimpleByFormat(r.Data, r.outputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

// CEList handles the ls command - lists nodes using Filesystem
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

	// Execute list command through Filesystem
	ctx := context.Background()
	result, err := c.FilesystemEngine.List(ctx, path, opts)
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

// getStringValue safely converts interface{} to string
func getStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// formatTimeValue converts a timestamp (milliseconds or string) to readable format
func formatTimeValue(v interface{}) string {
	if v == nil {
		return ""
	}

	var ts int64
	switch val := v.(type) {
	case float64:
		ts = int64(val)
	case int64:
		ts = val
	case int:
		ts = int64(val)
	case string:
		// Try to parse as number
		if _, err := fmt.Sscanf(val, "%d", &ts); err != nil {
			// If it's already a formatted date string, return as is
			return val
		}
	default:
		return fmt.Sprintf("%v", v)
	}

	// Convert milliseconds to seconds if timestamp is in milliseconds (13 digits)
	if ts > 1e12 {
		ts = ts / 1000
	}

	t := time.Unix(ts, 0)
	return t.Format("2006-01-02 15:04:05")
}

// CESearchResponse represents the response for search command
type CESearchResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Total        int                      `json:"total"`
	Message      string                   `json:"message"`
	Duration     float64
	outputFormat OutputFormat
}

func (r *CESearchResponse) Type() string { return "ce_search" }
func (r *CESearchResponse) TimeCost() float64 { return r.Duration }
func (r *CESearchResponse) SetOutputFormat(format OutputFormat) { r.outputFormat = format }
func (r *CESearchResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Printf("Found %d results:\n", r.Total)
		PrintTableSimpleByFormat(r.Data, r.outputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

// CESearch handles the search command using Filesystem
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

	// Execute search command through Filesystem
	ctx := context.Background()
	result, err := c.FilesystemEngine.Search(ctx, path, opts)
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
