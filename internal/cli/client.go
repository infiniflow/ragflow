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
	"io"

	ce "ragflow/internal/cli/filesystem"
)

// PasswordPromptFunc is a function type for password input
type PasswordPromptFunc func(prompt string) (string, error)

// CurrentModel holds the current model configuration
type CurrentModel struct {
	Provider string
	Instance string
	Model    string
}

// RAGFlowClient handles API interactions with the RAGFlow server
type RAGFlowClient struct {
	HTTPClient     *HTTPClient
	ServerType     string             // "admin" or "user"
	PasswordPrompt PasswordPromptFunc // Function for password input
	OutputFormat   OutputFormat       // Output format: table, plain, json
	ContextEngine  *ce.Engine         // Context Engine for virtual filesystem
	CurrentModel   *CurrentModel      // Current model configuration
}

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
	engine.RegisterProvider(ce.NewFileProvider(&httpClientAdapter{c.HTTPClient}))
	engine.RegisterProvider(ce.NewSkillProvider(&httpClientAdapter{c.HTTPClient}))

	c.ContextEngine = engine
}

// httpClientAdapter adapts HTTPClient to ce.HTTPClientInterface
type httpClientAdapter struct {
	client *HTTPClient
}

func (a *httpClientAdapter) Request(method, path string, authKind string, headers map[string]string, jsonBody map[string]interface{}) (*ce.HTTPResponse, error) {
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
	resp, err := a.client.Request(method, path, authKind, headers, jsonBody)
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

func (a *httpClientAdapter) UploadMultipart(path string, contentType string, body io.Reader) error {
	return a.client.UploadMultipart(path, contentType, body)
}

// ShowCurrentUser shows the current logged-in user information
// TODO: Implement showing current user information when API is available
func (c *RAGFlowClient) ShowCurrentUser(cmd *Command) (map[string]interface{}, error) {
	// TODO: Call the appropriate API to get current user information
	// Currently there is no /admin/user/info or /user/info API available
	// The /admin/auth API only verifies authorization, does not return user info
	return nil, fmt.Errorf("command 'SHOW CURRENT USER' is not yet implemented")
}
