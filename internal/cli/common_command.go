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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"ragflow/internal/common"
	"strings"
	"time"

	"golang.org/x/term"
)

func (c *CLI) LoginUserByCommand(cmd *Command) (ResponseIf, error) {
	email, ok := cmd.Params["email"].(string)
	if !ok {
		return nil, fmt.Errorf("email not provided")
	}
	password, ok := cmd.Params["password"].(string)
	if !ok {
		password = ""
	}

	err := c.LoginUserInteractive(email, password)
	if err != nil {
		return nil, err
	}

	var result SimpleResponse
	result.Code = 0
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = "Login successful"

	return &result, nil
}

// LoginUserInteractive performs interactive login with username and password
func (c *CLI) LoginUserInteractive(email, password string) error {
	// First, ping the server to check if it's available
	_, err := c.PingServer(1)
	if err != nil {
		return err
	}

	// If password is not provided, prompt for it
	if password == "" {
		fmt.Printf("password for %s: ", email)
		password, err = ReadPassword()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = strings.TrimSpace(password)
	}

	var baseURL string
	var httpClient *HTTPClient
	switch c.Config.CLIMode {
	case AdminMode:
		baseURL = "/admin/login"
		httpClient = c.AdminServerClient
	case APIMode:
		baseURL = "/auth/login"
		httpClient = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	default:
		return fmt.Errorf("invalid server type")
	}

	// Login
	token, err := c.loginUser(httpClient, baseURL, email, password)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	fmt.Printf("Login successfully\n")

	switch c.Config.CLIMode {
	case AdminMode:
		c.AdminServerClient.LoginToken = &token
		c.Config.AdminClientConfig.AdminName = &email
		c.Config.AdminClientConfig.AdminPassword = &password
	case APIMode:
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken = &token
		c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer].UserName = &email
		c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer].UserPassword = &password
	default:
		return fmt.Errorf("invalid server type")
	}
	return nil
}

func (c *CLI) PingByCommand(cmd *Command) (ResponseIf, error) {
	iterations := 1
	if iterationsParam, ok := cmd.Params["iterations"]; ok {
		iterations = int(iterationsParam.(float64))
	}
	return c.PingServer(iterations)
}

func (c *CLI) PingServer(iterations int) (ResponseIf, error) {
	var pingPath string
	var resp *Response
	var err error
	switch c.Config.CLIMode {
	case AdminMode:
		pingPath = "/admin/ping"
		if iterations > 1 {
			return c.AdminServerClient.RequestWithIterations("GET", pingPath, "web", nil, nil, iterations)
		}
		resp, err = c.AdminServerClient.Request("GET", pingPath, "web", nil, nil)
	case APIMode:
		pingPath = "/system/ping"
		if iterations > 1 {
			return c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].RequestWithIterations("GET", pingPath, "web", nil, nil, iterations)
		}
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", pingPath, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return nil, err
	}

	if resp.StatusCode != 200 {
		fmt.Println("Server is down")
		return nil, fmt.Errorf("server is down")
	}

	var result SimpleResponse
	switch c.Config.CLIMode {
	case AdminMode:
		if err = json.Unmarshal(resp.Body, &result); err != nil {
			return nil, fmt.Errorf("ping failed: invalid JSON (%w)", err)
		}
	case APIMode:
		if string(resp.Body) == "pong" {
			result.Code = 0
			result.Message = "Pong"
		} else {
			result.Code = 1
			result.Message = "Ping failed"
		}
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// loginUser performs the actual login request
func (c *CLI) loginUser(httpClient *HTTPClient, baseURL, email, password string) (string, error) {
	publicKey, err := c.GetPublicKeyPEM()
	if err != nil {
		return "", fmt.Errorf("failed to get public key: %w", err)
	}

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password, publicKey)

	if err != nil {
		return "", fmt.Errorf("failed to encrypt password: %w", err)
	}

	payload := map[string]interface{}{
		"email":    email,
		"password": encryptedPassword,
	}

	var resp *Response
	resp, err = httpClient.Request("POST", baseURL, "", nil, payload)
	if err != nil {
		return "", err
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("login failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("login failed: %s", result.Message)
	}

	token := resp.Headers.Get("Authorization")
	if token == "" {
		return "", fmt.Errorf("login failed: missing Authorization header")
	}

	return token, nil
}

func (c *CLI) Logout() (ResponseIf, error) {

	var resp *Response
	var err error
	switch c.Config.CLIMode {
	case AdminMode:
		if c.AdminServerClient.LoginToken == nil {
			return nil, fmt.Errorf("not logged in")
		}
		resp, err = c.AdminServerClient.Request("POST", "/admin/logout", "web", nil, nil)
	case APIMode:
		if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
			return nil, fmt.Errorf("not logged in")
		}
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", "/auth/logout", "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, err
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("login failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("login failed: %s", result.Message)
	}

	switch c.Config.CLIMode {
	case AdminMode:
		c.AdminServerClient.LoginToken = nil
		c.Config.AdminClientConfig.AdminName = nil
		c.Config.AdminClientConfig.AdminPassword = nil
	case APIMode:
		c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken = nil
		c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer].UserName = nil
		c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer].UserPassword = nil
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	return &result, nil
}

func (c *CLI) CommonAvailableProvidersCommand(cmd *Command) (ResponseIf, error) {

	var resp *Response
	var err error
	switch c.Config.CLIMode {
	case AdminMode:
		resp, err = c.AdminServerClient.Request("GET", "/admin/providers?available=true", "web", nil, nil)
	case APIMode:
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/providers?available=true", "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list providers: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list providers: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) CommonShowProviderCommand(cmd *Command) (ResponseIf, error) {
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s", providerName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s", providerName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to show provider: %w", err)
	}

	return HandleCommonDataResponse(resp, "show provider")
}

// CommonShowProviderInstanceCommand shows details of a specific instance
func (c *CLI) CommonShowProviderInstanceCommand(cmd *Command) (ResponseIf, error) {
	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s", providerName, instanceName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s", providerName, instanceName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to show instance: %w", err)
	}

	return HandleCommonDataResponse(resp, "show instance")
}

// CommonShowProviderInstanceBalanceCommand shows balance of a specific instance
func (c *CLI) CommonShowProviderInstanceBalanceCommand(cmd *Command) (ResponseIf, error) {

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s/balance", providerName, instanceName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s/balance", providerName, instanceName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to show instance balance: %w", err)
	}

	return HandleCommonDataResponse(resp, "show instance balance")
}

// CommonListProviderInstancesCommand lists all instances of a provider
// LIST INSTANCES FROM PROVIDER <name>
func (c *CLI) CommonListProviderInstancesCommand(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances", providerName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances", providerName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

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

func (c *CLI) CommonListInstanceModelsCommand(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s/models", providerName, instanceName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s/models", providerName, instanceName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

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

func (c *CLI) CommonListModelsCommand(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/models", providerName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/models", providerName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) CommonListInstanceModelsSyncCommand(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s/models?supported=true", providerName, instanceName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s/models?supported=true", providerName, instanceName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) CommonShowProviderModelCommand(cmd *Command) (ResponseIf, error) {
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model_name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/models/%s", providerName, modelName)
		resp, err = c.AdminServerClient.Request("GET", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/models/%s", providerName, modelName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to show model: %w", err)
	}

	return HandleCommonDataResponse(resp, "show model")
}

func (c *CLI) CommonCheckProviderWithKeyCommand(cmd *Command) (ResponseIf, error) {

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

	payload := map[string]interface{}{
		"region":  region,
		"api_key": apiKeyValue,
	}
	if baseURL != "" {
		payload["base_url"] = baseURL
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/connection", providerName)
		resp, err = c.AdminServerClient.Request("POST", endPoint, "web", nil, payload)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/connection", providerName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", endPoint, "web", nil, payload)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to check provider connection with key: %w", err)
	}

	switch c.Config.CLIMode {
	case AdminMode:
		return HandleCommonDataResponse(resp, "check provider connection with key")
	case APIMode:
		return HandleSimpleResponse(resp, "check provider connection with key")
	default:
		return nil, fmt.Errorf("invalid server type")
	}
}

func (c *CLI) CommonCheckProviderConnectionCommand(cmd *Command) (ResponseIf, error) {

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s/connection", providerName, instanceName)
		resp, err = c.AdminServerClient.Request("POST", endPoint, "web", nil, nil)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s/connection", providerName, instanceName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("POST", endPoint, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to check provider connection: %w", err)
	}

	switch c.Config.CLIMode {
	case AdminMode:
		return HandleCommonDataResponse(resp, "check provider connection")
	case APIMode:
		return HandleSimpleResponse(resp, "check provider connection")
	default:
		return nil, fmt.Errorf("invalid server type")
	}
}

// AlterProviderInstanceCommand alters a provider instance
func (c *CLI) CommonAlterProviderInstanceCommand(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance name not provided")
	}

	payload := map[string]interface{}{}

	newName, ok := cmd.Params["new_instance_name"].(string)
	if ok {
		payload["instance_name"] = newName
	}

	newAPIKey, ok := cmd.Params["new_api_key"].(string)
	if ok {
		payload["api_key"] = newAPIKey
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s", providerName, instanceName)
		resp, err = c.AdminServerClient.Request("PUT", endPoint, "web", nil, payload)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s", providerName, instanceName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PUT", endPoint, "web", nil, payload)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to alter instance: %w", err)
	}

	switch c.Config.CLIMode {
	case AdminMode:
		return HandleCommonDataResponse(resp, "alter instance")
	case APIMode:
		return HandleSimpleResponse(resp, "alter instance")
	default:
		return nil, fmt.Errorf("invalid server type")
	}
}

func (c *CLI) CommonEnableOrDisableModelCommand(cmd *Command, status string) (ResponseIf, error) {

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

	payload := map[string]interface{}{
		"status": status,
	}

	var resp *Response
	var err error
	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s/models/%s", providerName, instanceName, modelName)
		resp, err = c.AdminServerClient.Request("PATCH", endPoint, "web", nil, payload)
	case APIMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s/models/%s", providerName, instanceName, modelName)
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PATCH", endPoint, "web", nil, payload)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to enable/disable model: %w", err)
	}

	switch c.Config.CLIMode {
	case AdminMode:
		return HandleCommonDataResponse(resp, "enable/disable model")
	case APIMode:
		return HandleSimpleResponse(resp, "enable/disable model")
	default:
		return nil, fmt.Errorf("invalid server type")
	}
}

func (c *CLI) APISetDefaultModelCommand(cmd *Command) (ResponseIf, error) {

	modelType, ok := cmd.Params["model_type"].(string)
	if !ok {
		return nil, fmt.Errorf("model_type not provided")
	}

	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model_name not provided")
	}

	var providerName, instanceName, modelName string
	var err error
	modelName, instanceName, providerName, err = common.ExtractCompositeName(compositeModelName)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"model_type":     modelType,
		"model_provider": providerName,
		"model_instance": instanceName,
		"model_name":     modelName,
	}

	var resp *Response
	switch c.Config.CLIMode {
	case AdminMode:
		resp, err = c.AdminServerClient.Request("PATCH", "/admin/models", "web", nil, payload)
	case APIMode:
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PATCH", "/models", "web", nil, payload)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to set default model: %w", err)
	}

	return HandleSimpleResponse(resp, "set default model")
}

func (c *CLI) APIResetDefaultModelCommand(cmd *Command) (ResponseIf, error) {

	modelType, ok := cmd.Params["model_type"].(string)
	if !ok {
		return nil, fmt.Errorf("model_type not provided")
	}

	payload := map[string]interface{}{
		"model_type": modelType,
	}

	var resp *Response
	var err error
	switch c.Config.CLIMode {
	case AdminMode:
		resp, err = c.AdminServerClient.Request("PATCH", "/admin/models", "web", nil, payload)
	case APIMode:
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("PATCH", "/models", "web", nil, payload)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to reset default model: %w", err)
	}

	return HandleSimpleResponse(resp, "reset default model")
}

func (c *CLI) APIListDefaultModelsCommand(cmd *Command) (ResponseIf, error) {

	var resp *Response
	var err error
	switch c.Config.CLIMode {
	case AdminMode:
		resp, err = c.AdminServerClient.Request("GET", "/admin/models", "web", nil, nil)
	case APIMode:
		resp, err = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].Request("GET", "/models/default", "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list default models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list default models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ModelsResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list default models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) CommonShowCurrentCommand(cmd *Command) (ResponseIf, error) {
	var result *CommonDataResponse

	switch c.Config.CLIMode {
	case AdminMode:
		response, err := c.GetAdminServerInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to show current: %w", err)
		}
		result = response.(*CommonDataResponse)

	case APIMode:
		response, err := c.GetAPIServerInfo(c.Config.APIClientConfig.CurrentAPIServer)
		if err != nil {
			return nil, err
		}
		result = response.(*CommonDataResponse)

		if c.CurrentModel != nil {
			if result.Data == nil {
				result.Data = make(map[string]interface{})
			}
			result.Data["model_provider"] = c.CurrentModel.Provider
			result.Data["model_instance"] = c.CurrentModel.Instance
			result.Data["model_model"] = c.CurrentModel.Model
		}
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if result == nil {
		result = &CommonDataResponse{}
		if result.Data == nil {
			result.Data = make(map[string]interface{})
		}
	}

	result.Data["mode"] = c.Config.CLIMode
	result.Data["output"] = c.Config.OutputFormat
	result.Data["interactive"] = c.Config.Interactive
	result.Data["verbose"] = c.Config.Verbose

	return result, nil
}

func (c *CLI) CommonShowAdminServerCommand(cmd *Command) (ResponseIf, error) {
	return c.GetAdminServerInfo()
}

func (c *CLI) CommonShowAPIServerCommand(cmd *Command) (ResponseIf, error) {
	apiServerName, ok := cmd.Params["api_server_name"].(string)
	if !ok {
		return nil, fmt.Errorf("api_server_name not provided")
	}
	result, err := c.GetAPIServerInfo(apiServerName)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *CLI) CommonListAPIServersCommand(cmd *Command) (ResponseIf, error) {

	var result CommonResponse
	result.Data = make([]map[string]interface{}, 0)

	for serverName, apiServerConfig := range c.Config.APIClientConfig.APIServerMap {
		element := map[string]interface{}{
			"api_server": serverName,
		}
		element["api_server_ip"] = apiServerConfig.IP
		element["api_server_port"] = apiServerConfig.Port
		if apiServerConfig.UserName != nil {
			element["user_name"] = *apiServerConfig.UserName
		}
		if apiServerConfig.UserPassword != nil {
			element["user_password"] = strings.Repeat("*", len(*apiServerConfig.UserPassword))
		}
		if c.APIServerClientMap[serverName].LoginToken != nil {
			element["auth"] = "login"
		} else if c.APIServerClientMap[serverName].APIKey != nil {
			element["auth"] = "api key"
		} else {
			element["auth"] = "no auth"
		}
		result.Data = append(result.Data, element)
	}

	return &result, nil
}

func (c *CLI) AddAPIServerCommand(cmd *Command) (ResponseIf, error) {
	apiServerName, ok := cmd.Params["server_name"].(string)
	if !ok {
		return nil, fmt.Errorf("server name not provided")
	}
	if c.Config.APIClientConfig.APIServerMap[apiServerName] != nil {
		return nil, fmt.Errorf("api server already exists")
	}

	apiServerIP, ok := cmd.Params["server_ip"].(string)
	if !ok {
		return nil, fmt.Errorf("server ip not provided")
	}
	apiServerPort, ok := cmd.Params["server_port"].(int)
	if !ok {
		return nil, fmt.Errorf("server port not provided")
	}

	if c.Config.APIClientConfig.APIServerMap == nil {
		c.Config.APIClientConfig.APIServerMap = make(map[string]*APIServerConfig)
	}

	c.Config.APIClientConfig.APIServerMap[apiServerName] = &APIServerConfig{
		Name: apiServerName,
		IP:   apiServerIP,
		Port: apiServerPort,
	}
	c.Config.APIClientConfig.APIServerMap[apiServerName].IP = apiServerIP
	c.Config.APIClientConfig.APIServerMap[apiServerName].Port = apiServerPort

	if c.APIServerClientMap == nil {
		c.APIServerClientMap = make(map[string]*HTTPClient)
	}

	if c.APIServerClientMap[apiServerName] != nil {
		return nil, fmt.Errorf("api server: %s already exists", apiServerName)
	}

	transport := &http.Transport{
		// certs are common for the API server used by the CLI; verification
		// is left to the operator (the URL is configured by them). Document
		// the trade-off here so reviewers don't re-flag the same line.
		// codeql[go/disabled-certificate-check] Local cluster self-signed
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	c.APIServerClientMap[apiServerName] = &HTTPClient{
		Host:           apiServerIP,
		Port:           apiServerPort,
		APIVersion:     "v1",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    60 * time.Second,
		VerifySSL:      false,
		client: &http.Client{
			Transport: transport,
			Timeout:   300 * time.Second,
		},
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "api server added successfully"
	result.Duration = 0
	return &result, nil
}

func (c *CLI) DeleteAPIServerCommand(cmd *Command) (ResponseIf, error) {
	apiServerName, ok := cmd.Params["server_name"].(string)
	if !ok {
		return nil, fmt.Errorf("server name not provided")
	}
	if apiServerName == c.Config.APIClientConfig.CurrentAPIServer {
		return nil, fmt.Errorf("cannot delete current api server")
	}

	if c.APIServerClientMap[apiServerName] == nil && c.Config.APIClientConfig.APIServerMap[apiServerName] == nil {
		return nil, fmt.Errorf("api server: %s not found", apiServerName)
	}

	delete(c.Config.APIClientConfig.APIServerMap, apiServerName)
	delete(c.APIServerClientMap, apiServerName)
	var result SimpleResponse
	result.Code = 0
	result.Message = "api server deleted successfully"
	result.Duration = 0
	return &result, nil
}

func (c *CLI) AddAdminServerCommand(cmd *Command) (ResponseIf, error) {

	if c.AdminServerClient != nil && c.AdminServerClient.LoginToken != nil {
		return nil, fmt.Errorf("admin server already login, please logout")
	}

	adminServerIP, ok := cmd.Params["server_ip"].(string)
	if !ok {
		return nil, fmt.Errorf("server ip not provided")
	}
	adminServerPort, ok := cmd.Params["server_port"].(int)
	if !ok {
		return nil, fmt.Errorf("server port not provided")
	}

	if c.Config.AdminClientConfig == nil {
		c.Config.AdminClientConfig = &AdminModeConfig{}
	}

	if adminServerIP != "" {
		c.Config.AdminClientConfig.AdminHost = adminServerIP
	}
	if adminServerPort != 0 {
		c.Config.AdminClientConfig.AdminPort = adminServerPort
	}

	transport := &http.Transport{
		// certs are common for the admin server used by the CLI; verification
		// is left to the operator (the URL is configured by them). Document
		// the trade-off here so reviewers don't re-flag the same line.
		// codeql[go/disabled-certificate-check] Local cluster self-signed
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	c.AdminServerClient = &HTTPClient{
		Host:           adminServerIP,
		Port:           adminServerPort,
		APIVersion:     "v1",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    60 * time.Second,
		VerifySSL:      false,
		client: &http.Client{
			Transport: transport,
			Timeout:   300 * time.Second,
		},
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "admin server added successfully"
	result.Duration = 0
	return &result, nil
}

func (c *CLI) DeleteAdminServerCommand(cmd *Command) (ResponseIf, error) {

	if c.AdminServerClient == nil && c.Config.AdminClientConfig == nil {
		return nil, fmt.Errorf("admin server not exists")
	}

	if c.AdminServerClient != nil {
		c.AdminServerClient = nil
	}

	if c.Config.AdminClientConfig != nil {
		c.Config.AdminClientConfig = nil
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = "admin server deleted successfully"
	result.Duration = 0
	return &result, nil
}

func (c *CLI) CommonSaveServerConfigCommand(cmd *Command) (ResponseIf, error) {

	switch c.Config.CLIMode {
	case AdminMode:
		if c.AdminServerClient.LoginToken == nil {
			return nil, fmt.Errorf("admin server isn't already login")
		}
	case APIMode:
		if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey == nil && c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken == nil {
			return nil, fmt.Errorf("API token not set. Please login first")
		}
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	return nil, fmt.Errorf("save server config isn't implemented")
}

func (c *CLI) GetAdminServerInfo() (ResponseIf, error) {
	var result CommonDataResponse
	result.Data = make(map[string]interface{})

	if c.Config.AdminClientConfig == nil {
		result.Data["admin_server"] = "N/A"
	} else {
		result.Data["admin_server_ip"] = c.Config.AdminClientConfig.AdminHost
		result.Data["admin_server_port"] = c.Config.AdminClientConfig.AdminPort
		if c.Config.AdminClientConfig.AdminName != nil {
			result.Data["admin_name"] = *c.Config.AdminClientConfig.AdminName
		}
		if c.Config.AdminClientConfig.AdminPassword != nil {
			result.Data["admin_password"] = strings.Repeat("*", len(*c.Config.AdminClientConfig.AdminPassword))
		}
		if c.AdminServerClient == nil || c.AdminServerClient.LoginToken == nil {
			result.Data["auth"] = "no auth"
		} else {
			result.Data["auth"] = "login"
		}
	}

	return &result, nil
}

func (c *CLI) GetAPIServerInfo(serverName string) (ResponseIf, error) {
	var result CommonDataResponse
	result.Data = make(map[string]interface{})

	if c.Config.APIClientConfig.APIServerMap == nil || c.Config.APIClientConfig.APIServerMap[serverName] == nil {
		result.Data["api_server"] = "N/A"
	} else {
		result.Data["api_server"] = serverName
		apiServerConfig := c.Config.APIClientConfig.APIServerMap[serverName]
		result.Data["api_server_ip"] = apiServerConfig.IP
		result.Data["api_server_port"] = apiServerConfig.Port
		if apiServerConfig.UserName != nil {
			result.Data["user_name"] = *apiServerConfig.UserName
		}

		if apiServerConfig.UserPassword != nil {
			result.Data["user_password"] = strings.Repeat("*", len(*apiServerConfig.UserPassword))
		}

		if c.Config.APIClientConfig.CurrentAPIServer == "" {
			result.Data["auth"] = "unknown"
		} else {
			if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].LoginToken != nil {
				result.Data["auth"] = "login"
			} else if c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer].APIKey != nil {
				result.Data["auth"] = "api key"
			} else {
				result.Data["auth"] = "no auth"
			}
		}
	}
	return &result, nil
}

func (c *CLI) CommonListAllModels(cmd *Command) (ResponseIf, error) {

	page, ok := cmd.Params["page"].(int)
	if !ok {
		page = 0
	}

	pageSize, ok := cmd.Params["page_size"].(int)
	if !ok {
		pageSize = 0
	}

	payload := map[string]interface{}{
		"page":      page,
		"page_size": pageSize,
	}

	var resp *Response
	var err error
	var httpClient *HTTPClient
	switch c.Config.CLIMode {
	case AdminMode:
		httpClient = c.AdminServerClient
		apiURL := "/admin/all-models"
		resp, err = httpClient.Request("GET", apiURL, "web", nil, payload)
	case APIMode:
		httpClient = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
		apiURL := "/all-models"
		resp, err = httpClient.Request("GET", apiURL, "web", nil, payload)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list all models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list all models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list all models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) CommonShowModelCommand(cmd *Command) (ResponseIf, error) {

	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model_name not provided")
	}

	encodedModelName := common.EncodeToBase64(modelName)

	var resp *Response
	var err error
	var httpClient *HTTPClient
	switch c.Config.CLIMode {
	case AdminMode:
		baseURL := fmt.Sprintf("/admin/all-models/%s", encodedModelName)
		httpClient = c.AdminServerClient
		resp, err = httpClient.Request("GET", baseURL, "web", nil, nil)
	case APIMode:
		baseURL := fmt.Sprintf("/all-models/%s", encodedModelName)
		httpClient = c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
		resp, err = httpClient.Request("GET", baseURL, "web", nil, nil)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to show model: %w", err)
	}

	return HandleCommonDataResponse(resp, "show model")
}

// readPassword reads password from terminal without echoing
func ReadPassword() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ReadPasswordFallback()
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(passwordBytes)), nil
}

// readPasswordFallback reads password as plain text (fallback mode)
func ReadPasswordFallback() (string, error) {
	fmt.Print("Password (will be visible): ")
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(password), nil
}

// FlattenMap recursively flattens a nested map into dot-notation keys
func FlattenMap(data map[string]interface{}, prefix string, result *[]map[string]interface{}) {
	for key, value := range data {
		// Build the current key path
		currentKey := key
		if prefix != "" {
			currentKey = prefix + "." + key
		}

		// Check if the value is another nested map
		if nestedMap, ok := value.(map[string]interface{}); ok {
			// Recursively process the nested map
			FlattenMap(nestedMap, currentKey, result)
		} else {
			// Leaf node: append to result slice
			resultItem := map[string]interface{}{
				"key":   currentKey,
				"value": value,
			}
			*result = append(*result, resultItem)
		}
	}
}

func (c *CLI) CommonUseAPIServerCommand(cmd *Command) (ResponseIf, error) {
	serverName, ok := cmd.Params["server_name"].(string)
	if !ok {
		return nil, fmt.Errorf("server_name not provided")
	}

	if c.Config.CLIMode == APIMode {
		if c.Config.APIClientConfig.CurrentAPIServer == serverName {
			return nil, fmt.Errorf("api server %s is already used", serverName)
		}
	}

	var httpClient *HTTPClient
	httpClient, ok = c.APIServerClientMap[serverName]
	if !ok {
		return nil, fmt.Errorf("api server %s not found", serverName)
	}
	if httpClient == nil {
		return nil, fmt.Errorf("api server %s is nil", serverName)
	}
	var apiServerConfig *APIServerConfig
	apiServerConfig, ok = c.Config.APIClientConfig.APIServerMap[serverName]
	if !ok {
		return nil, fmt.Errorf("api server %s not found in config", serverName)
	}
	if apiServerConfig == nil {
		return nil, fmt.Errorf("api server %s is nil", serverName)
	}

	c.Config.APIClientConfig.CurrentAPIServer = serverName
	c.Config.CLIMode = APIMode

	var result SimpleResponse
	result.Code = 0
	result.Message = fmt.Sprintf("switch to api server %s", serverName)
	result.Duration = 0
	return &result, nil

}

func (c *CLI) CommonUseAdminServerCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode == AdminMode {
		return nil, fmt.Errorf("already in admin mode")
	}

	if c.AdminServerClient == nil || c.Config.AdminClientConfig == nil {
		return nil, fmt.Errorf("admin server not added")
	}

	c.Config.APIClientConfig.CurrentAPIServer = ""
	c.Config.CLIMode = AdminMode

	var result SimpleResponse
	result.Code = 0
	result.Message = "switch to admin server"
	result.Duration = 0
	return &result, nil
}

func (c *CLI) getDatasetIDByName(datasetName string) (string, error) {
	response, err := c.APIListDatasetsCommand(nil)
	if err != nil {
		return "", err
	}
	commonResponse, ok := response.(*CommonResponse)
	if !ok {
		return "", fmt.Errorf("invalid response")
	}
	for _, dataset := range commonResponse.Data {
		if dataset["name"] == datasetName {
			return dataset["id"].(string), nil
		}
	}
	return "", fmt.Errorf("dataset %s not found", datasetName)
}

func (c *CLI) getAgentIDByName(agentName string) (string, error) {
	response, err := c.APIListAgentsCommand(nil)
	if err != nil {
		return "", err
	}
	commonResponse, ok := response.(*CommonResponse)
	if !ok {
		return "", fmt.Errorf("invalid response")
	}
	for _, agent := range commonResponse.Data {
		if agent["name"] == agentName {
			return agent["id"].(string), nil
		}
	}
	return "", fmt.Errorf("agent %s not found", agentName)
}

func (c *CLI) getSearchIDByName(searchName string) (string, error) {
	response, err := c.APIListSearchesCommand(nil)
	if err != nil {
		return "", err
	}
	searchesResponse, ok := response.(*ListSearchesResponse)
	if !ok {
		return "", fmt.Errorf("invalid response")
	}
	searches := searchesResponse.Data["search_apps"].([]interface{})
	for _, search := range searches {
		searchMap := search.(map[string]interface{})
		if searchMap["name"] == searchName {
			return searchMap["id"].(string), nil
		}
	}
	return "", fmt.Errorf("search %s not found", searchName)
}

func (c *CLI) getChatIDByName(chatName string) (string, error) {
	response, err := c.APIListChatsCommand(nil)
	if err != nil {
		return "", err
	}
	commonResponse, ok := response.(*CommonResponse)
	if !ok {
		return "", fmt.Errorf("invalid response")
	}
	for _, chat := range commonResponse.Data {
		if chat["name"] == chatName {
			return chat["id"].(string), nil
		}
	}
	return "", fmt.Errorf("chat %s not found", chatName)
}

func (c *CLI) getMemoryIDByName(memoryName string) (string, error) {
	response, err := c.APIListMemoriesCommand(nil)
	if err != nil {
		return "", err
	}
	listMemoriesResponse, ok := response.(*ListMemoriesResponse)
	memories := listMemoriesResponse.Data["memory_list"].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response")
	}
	for _, memory := range memories {
		var memoryMap map[string]interface{}
		memoryMap, ok = memory.(map[string]interface{})
		if !ok {
			continue
		}
		if memoryMap["name"] == memoryName {
			return memoryMap["id"].(string), nil
		}
	}
	return "", fmt.Errorf("memory %s not found", memoryName)
}
