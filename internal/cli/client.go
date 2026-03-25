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

// PasswordPromptFunc is a function type for password input
type PasswordPromptFunc func(prompt string) (string, error)

// RAGFlowClient handles API interactions with the RAGFlow server
type RAGFlowClient struct {
	HTTPClient     *HTTPClient
	ServerType     string             // "admin" or "user"
	PasswordPrompt PasswordPromptFunc // Function for password input
	OutputFormat   OutputFormat       // Output format: table, plain, json
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

	return &RAGFlowClient{
		HTTPClient: httpClient,
		ServerType: serverType,
	}
}

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
	case "create_token":
		return c.CreateAdminToken(cmd)
	case "list_tokens":
		return c.ListAdminTokens(cmd)
	case "drop_token":
		return c.DropAdminToken(cmd)
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
