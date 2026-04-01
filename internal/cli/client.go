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
	"fmt"
	ce "ragflow/internal/cli/contextengine"
)

// PasswordPromptFunc is a function type for password input
type PasswordPromptFunc func(prompt string) (string, error)

// RAGFlowClient handles API interactions with the RAGFlow server
type RAGFlowClient struct {
	HTTPClient     *HTTPClient
	ServerType     string             // "admin" or "user"
	PasswordPrompt PasswordPromptFunc // Function for password input
	OutputFormat   OutputFormat       // Output format: table, plain, json
	ContextEngine  *ce.Engine         // Context Engine for virtual filesystem
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

	// Initialize Context Engine
	client.initContextEngine()

	return client
}

// initContextEngine initializes the Context Engine with all providers
func (c *RAGFlowClient) initContextEngine() {
	engine := ce.NewEngine()

	// Register providers
	engine.RegisterProvider(ce.NewDatasetProvider(&httpClientAdapter{c.HTTPClient}))

	c.ContextEngine = engine
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
	case "list_available_providers":
		return c.ListAvailableProviders(cmd)
	case "show_provider":
		return c.ShowProvider(cmd)
	case "list_provider_models":
		return c.ListModels(cmd)
	case "show_model":
		return c.ShowModel(cmd)
	// Provider commands
	case "add_provider":
		return c.AddProvider(cmd)
	case "list_providers":
		return c.ListProviders(cmd)
	case "delete_provider":
		return c.DeleteProvider(cmd)
	// ContextEngine commands
	case "ce_ls":
		return c.CEList(cmd)
	case "ce_search":
		return c.CESearch(cmd)
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
