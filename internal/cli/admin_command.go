package cli

import (
	"encoding/json"
	"fmt"
)

// PingServer pings the server to check if it's alive
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *RAGFlowClient) PingAdmin(cmd *Command) (ResponseIf, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.HTTPClient.RequestWithIterations("GET", "/admin/ping", false, "web", nil, nil, iterations)
	}

	// Single ping mode
	resp, err := c.HTTPClient.Request("GET", "/admin/ping", true, "web", nil, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Server is down")
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ping: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
	}

	return &result, nil
}

// ListUsers lists all users (admin mode only)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListUsers(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", "/admin/users", true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", "/admin/users", true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list users: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("list user failed: %s", result.Message)
	}

	for _, user := range result.Data {
		delete(user, "create_date")
	}

	//PrintTableSimple(result.Data)
	return &result, nil
}

// GrantAdmin grants admin privileges to a user (admin mode only)
func (c *RAGFlowClient) GrantAdmin(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("PUT", fmt.Sprintf("/admin/users/%s/admin", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to grant admin: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to grant admin: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("grant admin failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("grant admin failed: %s", result.Message)
	}
	return &result, nil
}

// RevokeAdmin revokes admin privileges from a user (admin mode only)
func (c *RAGFlowClient) RevokeAdmin(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/admin/users/%s/admin", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke admin: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to revoke admin: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("revoke admin failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("revoke admin failed: %s", result.Message)
	}
	return &result, nil
}

// CreateUser creates a new user (admin mode only)
func (c *RAGFlowClient) CreateUser(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	password, ok := cmd.Params["password"].(string)
	if !ok {
		return nil, fmt.Errorf("password not provided")
	}

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	payload := map[string]interface{}{
		"username": userName,
		"password": encryptedPassword,
		"role":     "user",
	}

	resp, err := c.HTTPClient.Request("POST", "/admin/users", true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create user: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("create user failed: %s", result.Message)
	}
	return &result, nil
}

// ActivateUser activates or deactivates a user (admin mode only)
func (c *RAGFlowClient) ActivateUser(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	activateStatus, ok := cmd.Params["activate_status"].(string)
	if !ok {
		return nil, fmt.Errorf("activate_status not provided")
	}

	// Validate activate_status
	if activateStatus != "on" && activateStatus != "off" {
		return nil, fmt.Errorf("activate_status must be 'on' or 'off'")
	}

	payload := map[string]interface{}{
		"activate_status": activateStatus,
	}

	resp, err := c.HTTPClient.Request("PUT", fmt.Sprintf("/admin/users/%s/activate", userName), true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to update user activate status: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to update user activate status: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("create user failed: %s", result.Message)
	}
	return &result, nil
}

// AlterUserPassword changes a user's password (admin mode only)
func (c *RAGFlowClient) AlterUserPassword(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	password, ok := cmd.Params["password"].(string)
	if !ok {
		return nil, fmt.Errorf("password not provided")
	}

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	payload := map[string]interface{}{
		"new_password": encryptedPassword,
	}

	resp, err := c.HTTPClient.Request("PUT", fmt.Sprintf("/admin/users/%s/password", userName), true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to change user password: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to change user password: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("create user failed: %s", result.Message)
	}
	return &result, nil
}

type listServicesResponse struct {
	Code    int                      `json:"code"`
	Data    []map[string]interface{} `json:"data"`
	Message string                   `json:"message"`
}

// ListServices lists all services (admin mode only)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListServices(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", "/admin/services", true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", "/admin/services", true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list services: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("list user failed: %s", result.Message)
	}

	for _, user := range result.Data {
		delete(user, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ListServices lists all services (admin mode only)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ShowService(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	serviceIndex := cmd.Params["number"].(int)

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	endPoint := fmt.Sprintf("/admin/services/%d", serviceIndex)

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", endPoint, true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", endPoint, true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show service: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show service: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result ShowResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("list user failed: %s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// DropUser deletes a user (admin mode only)
func (c *RAGFlowClient) DropUser(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/admin/users/%s", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to delete user: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to delete user: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("create user failed: %s", result.Message)
	}
	return &result, nil
}
