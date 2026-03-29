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
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// LoginUserInteractive performs interactive login with username and password
func (c *RAGFlowClient) LoginUserInteractive(username, password string) error {
	// First, ping the server to check if it's available
	// For admin mode, use /admin/ping with useAPIBase=true
	// For user mode, use /system/ping with useAPIBase=false
	var pingPath string
	var useAPIBase bool
	if c.ServerType == "admin" {
		pingPath = "/admin/ping"
		useAPIBase = true
	} else {
		pingPath = "/system/ping"
		useAPIBase = false
	}

	resp, err := c.HTTPClient.Request("GET", pingPath, useAPIBase, "web", nil, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	if resp.StatusCode != 200 {
		fmt.Println("Server is down")
		return fmt.Errorf("server is down")
	}

	// Check response - admin returns JSON with message "PONG", user returns plain "pong"
	resJSON, err := resp.JSON()
	if err == nil {
		// Admin mode returns {"code":0,"message":"PONG"}
		if msg, ok := resJSON["message"].(string); !ok || msg != "pong" {
			fmt.Println("Server is down")
			return fmt.Errorf("server is down")
		}
	} else {
		// User mode returns plain "pong"
		if string(resp.Body) != "pong" {
			fmt.Println("Server is down")
			return fmt.Errorf("server is down")
		}
	}

	// If password is not provided, prompt for it
	if password == "" {
		fmt.Printf("password for %s: ", username)
		var err error
		password, err = readPassword()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = strings.TrimSpace(password)
	}

	// Login
	token, err := c.loginUser(username, password)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	c.HTTPClient.LoginToken = token
	fmt.Printf("Login user %s successfully\n", username)
	return nil
}

// LoginUser performs user login
func (c *RAGFlowClient) LoginUser(cmd *Command) error {
	// First, ping the server to check if it's available
	// For admin mode, use /admin/ping with useAPIBase=true
	// For user mode, use /system/ping with useAPIBase=false
	var pingPath string
	var useAPIBase bool
	if c.ServerType == "admin" {
		pingPath = "/admin/ping"
		useAPIBase = true
	} else {
		pingPath = "/system/ping"
		useAPIBase = false
	}

	resp, err := c.HTTPClient.Request("GET", pingPath, useAPIBase, "web", nil, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	if resp.StatusCode != 200 {
		fmt.Println("Server is down")
		return fmt.Errorf("server is down")
	}

	// Check response - admin returns JSON with message "PONG", user returns plain "pong"
	resJSON, err := resp.JSON()
	if err == nil {
		// Admin mode returns {"code":0,"message":"PONG"}
		if msg, ok := resJSON["message"].(string); !ok || msg != "pong" {
			fmt.Println("Server is down")
			return fmt.Errorf("server is down")
		}
	} else {
		// User mode returns plain "pong"
		if string(resp.Body) != "pong" {
			fmt.Println("Server is down")
			return fmt.Errorf("server is down")
		}
	}

	email, ok := cmd.Params["email"].(string)
	if !ok {
		return fmt.Errorf("email not provided")
	}

	password, ok := cmd.Params["password"].(string)
	if !ok {
		// Get password from user input (hidden)
		fmt.Printf("password for %s: ", email)
		password, err = readPassword()
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

// loginUser performs the actual login request
func (c *RAGFlowClient) loginUser(email, password string) (string, error) {
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
	if c.ServerType == "admin" {
		path = "/admin/login"
	} else {
		path = "/user/login"
	}

	resp, err := c.HTTPClient.Request("POST", path, c.ServerType == "admin", "", nil, payload)
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

func (c *RAGFlowClient) Logout() (ResponseIf, error) {
	if c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("not logged in")
	}

	var path string
	if c.ServerType == "admin" {
		path = "/admin/logout"
	} else {
		path = "/user/logout"
	}

	resp, err := c.HTTPClient.Request("GET", path, c.ServerType == "admin", "web", nil, nil)
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

func (c *RAGFlowClient) ListPoolProviders(cmd *Command) (ResponseIf, error) {
	resp, err := c.HTTPClient.Request("GET", "/providers", true, "web", nil, nil)
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

func (c *RAGFlowClient) ShowPoolProvider(cmd *Command) (ResponseIf, error) {
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	endPoint := fmt.Sprint("/providers/%s", providerName)

	resp, err := c.HTTPClient.Request("GET", endPoint, true, "web", nil, nil)
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

func (c *RAGFlowClient) ListPoolModels(cmd *Command) (ResponseIf, error) {

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	endPoint := fmt.Sprint("/providers/%s/models", providerName)

	resp, err := c.HTTPClient.Request("GET", endPoint, true, "web", nil, nil)
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

func (c *RAGFlowClient) ShowPoolModel(cmd *Command) (ResponseIf, error) {
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	modelName, ok := cmd.Params["model_name"].(string)
	if !ok {
		return nil, fmt.Errorf("model_name not provided")
	}

	endPoint := fmt.Sprint("/providers/%s/models/%s", providerName, modelName)

	resp, err := c.HTTPClient.Request("GET", endPoint, true, "web", nil, nil)
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

// readPassword reads password from terminal without echoing
func readPassword() (string, error) {
	// Check if stdin is a terminal by trying to get terminal size
	if isTerminal() {
		// Use stty to disable echo
		cmd := exec.Command("stty", "-echo")
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			// Fallback: read normally
			return readPasswordFallback()
		}
		defer func() {
			// Re-enable echo
			cmd := exec.Command("stty", "echo")
			cmd.Stdin = os.Stdin
			cmd.Run()
		}()

		reader := bufio.NewReader(os.Stdin)
		password, err := reader.ReadString('\n')
		fmt.Println() // New line after password input
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(password), nil
	}

	// Fallback for non-terminal input (e.g., piped input)
	return readPasswordFallback()
}

// isTerminal checks if stdin is a terminal
func isTerminal() bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}

// readPasswordFallback reads password as plain text (fallback mode)
func readPasswordFallback() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(password), nil
}
