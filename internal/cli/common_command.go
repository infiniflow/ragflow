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
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	result.SetOutputFormat(c.outputFormat)
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

	// Login
	token, err := c.loginUser(email, password)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	c.HTTPClient.LoginToken = token
	fmt.Printf("Login user %s successfully\n", email)
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
	switch c.Config.CLIMode {
	case AdminMode:
		pingPath = "/admin/ping"
	case UserMode:
		pingPath = "/system/ping"
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.HTTPClient.RequestWithIterations("GET", "/system/ping", "web", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", pingPath, "web", nil, nil)
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
			return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
		}
	case UserMode:
		if string(resp.Body) == "pong" {
			result.Code = 0
			result.Message = "Pong"
		} else {
			result.Code = 1
			result.Message = "Ping failed"
		}
	}

	result.Duration = resp.Duration
	return &result, nil
}

// loginUser performs the actual login request
func (c *CLI) loginUser(email, password string) (string, error) {
	// Encrypt password using scrypt (same as Python implementation)
	encryptedPassword, err := EncryptPassword(password)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt password: %w", err)
	}

	payload := map[string]interface{}{
		"email":    email,
		"password": encryptedPassword,
	}

	var path string
	switch c.Config.CLIMode {
	case AdminMode:
		path = "/admin/login"
	case UserMode:
		path = "/auth/login"
	default:
		return "", fmt.Errorf("invalid server type")
	}

	resp, err := c.HTTPClient.Request("POST", path, "", nil, payload)
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
	if c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("not logged in")
	}

	var path string
	switch c.Config.CLIMode {
	case AdminMode:
		path = "/admin/logout"
	case UserMode:
		path = "/auth/logout"
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	resp, err := c.HTTPClient.Request("POST", path, "web", nil, nil)
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

	return &result, nil
}

func (c *CLI) ListAvailableProviders(cmd *Command) (ResponseIf, error) {

	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = "/admin/providers?available=true"
	case UserMode:
		endPoint = "/providers?available=true"
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	resp, err := c.HTTPClient.Request("GET", endPoint, "web", nil, nil)
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

func (c *CLI) ShowProvider(cmd *Command) (ResponseIf, error) {
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s", providerName)
	case UserMode:
		endPoint = fmt.Sprintf("/providers/%s", providerName)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	resp, err := c.HTTPClient.Request("GET", endPoint, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show provider: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show provider: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to show provider: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ListModels(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/models", providerName)
	case UserMode:
		endPoint = fmt.Sprintf("/providers/%s/models", providerName)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	resp, err := c.HTTPClient.Request("GET", endPoint, "web", nil, nil)
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

func (c *CLI) ListSupportedModels(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/instances/%s/models?supported=true", providerName, instanceName)
	case UserMode:
		endPoint = fmt.Sprintf("/providers/%s/instances/%s/models?supported=true", providerName, instanceName)
	default:
		return nil, fmt.Errorf("invalid server type")
	}

	resp, err := c.HTTPClient.Request("GET", endPoint, "web", nil, nil)
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

func (c *CLI) ShowModel(cmd *Command) (ResponseIf, error) {
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model_name not provided")
	}

	var endPoint string
	switch c.Config.CLIMode {
	case AdminMode:
		endPoint = fmt.Sprintf("/admin/providers/%s/models/%s", providerName, modelName)
	case UserMode:
		endPoint = fmt.Sprintf("/providers/%s/models/%s", providerName, modelName)
	default:
		return nil, fmt.Errorf("invalid server type")
	}
	resp, err := c.HTTPClient.Request("GET", endPoint, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show model: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show model: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to show model: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) SetDefaultModel(cmd *Command) (ResponseIf, error) {

	modelType, ok := cmd.Params["model_type"].(string)
	if !ok {
		return nil, fmt.Errorf("model_type not provided")
	}

	compositeModelName, ok := cmd.Params["composite_model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model_name not provided")
	}

	var providerName, instanceName, modelName string
	names := strings.Split(compositeModelName, "/")
	if len(names) != 3 {
		return nil, fmt.Errorf("model name must be in format 'provider/instance/model'")
	}
	providerName = names[0]
	instanceName = names[1]
	modelName = names[2]

	payload := map[string]interface{}{
		"model_type":     modelType,
		"model_provider": providerName,
		"model_instance": instanceName,
		"model_name":     modelName,
	}

	resp, err := c.HTTPClient.Request("PATCH", "/models", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to set default model: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to set default model: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to set default model: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ResetDefaultModel(cmd *Command) (ResponseIf, error) {

	modelType, ok := cmd.Params["model_type"].(string)
	if !ok {
		return nil, fmt.Errorf("model_type not provided")
	}

	payload := map[string]interface{}{
		"model_type": modelType,
	}

	resp, err := c.HTTPClient.Request("PATCH", "/models", "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to reset default model: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to reset default model: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to reset default model: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ListDefaultModels(cmd *Command) (ResponseIf, error) {
	resp, err := c.HTTPClient.Request("GET", "/models", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list default models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list default models: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to list default models: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ShowCommonCurrent(cmd *Command) (ResponseIf, error) {
	var result CommonDataResponse
	result.Code = 0
	result.Data = make(map[string]interface{})
	result.Data["mode"] = c.Config.CLIMode
	if c.CurrentModel != nil {
		result.Data["model_provider"] = c.CurrentModel.Provider
		result.Data["model_instance"] = c.CurrentModel.Instance
		result.Data["model_model"] = c.CurrentModel.Model
	}
	return &result, nil
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
