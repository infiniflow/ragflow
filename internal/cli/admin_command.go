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
	"encoding/json"
	"fmt"
	"net/url"
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

	// Single mode
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
	result.Duration = resp.Duration
	return &result, nil
}

// Show admin version to show RAGFlow admin version
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *RAGFlowClient) ShowAdminVersion(cmd *Command) (ResponseIf, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.HTTPClient.RequestWithIterations("GET", "/admin/version", false, "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.HTTPClient.Request("GET", "/admin/version", true, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show admin version: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show admin version: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show admin version failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// ListRoles to list roles (admin mode only)
func (c *RAGFlowClient) ListRoles(cmd *Command) (ResponseIf, error) {
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
		return c.HTTPClient.RequestWithIterations("GET", "/admin/roles", true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", "/admin/roles", true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list roles: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list roles failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	for _, user := range result.Data {
		delete(user, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ShowRole to show role (admin mode only)
func (c *RAGFlowClient) ShowRole(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	roleName := cmd.Params["role_name"].(string)

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	endPoint := fmt.Sprintf("/admin/roles/%s/", roleName)

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", endPoint, true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", endPoint, true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show role: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show role: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show role failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// CreateRole creates a new role (admin mode only)
func (c *RAGFlowClient) CreateRole(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	description, ok := cmd.Params["description"].(string)
	payload := map[string]interface{}{
		"role_name": roleName,
	}
	if ok {
		payload["description"] = description
	}

	resp, err := c.HTTPClient.Request("POST", "/admin/roles", true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create role: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create role failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// DropRole deletes the role (admin mode only)
func (c *RAGFlowClient) DropRole(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/admin/roles/%s", roleName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop role: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop role: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop role failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// AlterRole alters the role rights (admin mode only)
func (c *RAGFlowClient) AlterRole(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	description, ok := cmd.Params["description"].(string)
	payload := map[string]interface{}{
		"role_name": roleName,
	}
	if ok {
		payload["description"] = description
	}

	resp, err := c.HTTPClient.Request("PUT", fmt.Sprintf("/admin/roles/%s", roleName), true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to alter role: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to alter role: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("alter role failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
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
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
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
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
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
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
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
		return nil, fmt.Errorf("failed to update user status: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to update user status: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("update user status failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
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
		return nil, fmt.Errorf("change user password failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

type listServicesResponse struct {
	Code    int                      `json:"code"`
	Data    []map[string]interface{} `json:"data"`
	Message string                   `json:"message"`
}

// ListServices lists all services (admin mode only)
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
		return nil, fmt.Errorf("%s", result.Message)
	}

	for _, user := range result.Data {
		delete(user, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// Show service show service (admin mode only)
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

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show service failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
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
		return nil, fmt.Errorf("%s", result.Message)
	}

	for _, user := range result.Data {
		delete(user, "create_date")
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
		return nil, fmt.Errorf("failed to drop user: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop user: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// Show user show user (admin mode only)
func (c *RAGFlowClient) ShowUser(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show user: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show user: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show user failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// ListDatasets lists datasets for a specific user (admin mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListDatasets(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", fmt.Sprintf("/admin/users/%s/datasets", userName), true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/datasets", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	data, ok := resJSON["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	// Convert to slice of maps and remove avatar
	tableData := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if itemMap, ok := item.(map[string]interface{}); ok {
			delete(itemMap, "avatar")
			tableData = append(tableData, itemMap)
		}
	}

	PrintTableSimple(tableData)
	return nil, nil
}

// ListAgents lists agents for a specific user (admin mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListAgents(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", fmt.Sprintf("/admin/users/%s/agents", userName), true, "admin", nil, nil, iterations)
	}

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/agents", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list agents: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	data, ok := resJSON["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	// Convert to slice of maps and remove avatar
	tableData := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if itemMap, ok := item.(map[string]interface{}); ok {
			delete(itemMap, "avatar")
			tableData = append(tableData, itemMap)
		}
	}

	PrintTableSimple(tableData)
	return nil, nil
}

// GrantPermission grants permission to a role (admin mode only)
func (c *RAGFlowClient) GrantPermission(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/keys", userName), true, "admin", nil, nil)
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

	// Remove extra field from data
	for _, item := range result.Data {
		delete(item, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// RevokePermission revokes permission from a role (admin mode only)
func (c *RAGFlowClient) RevokePermission(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	resource, ok := cmd.Params["resource"].(string)
	if !ok {
		return nil, fmt.Errorf("resource not provided")
	}

	actionsRaw, ok := cmd.Params["actions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("actions not provided")
	}

	actions := make([]string, 0, len(actionsRaw))
	for _, action := range actionsRaw {
		if actionStr, ok := action.(string); ok {
			actions = append(actions, actionStr)
		}
	}

	payload := map[string]interface{}{
		"resource": resource,
		"actions":  actions,
	}

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/admin/roles/%s/permission", roleName), true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke permission: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to revoke permission: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("revoke permission failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	// Remove extra field from data
	for _, item := range result.Data {
		delete(item, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// AlterUserRole alters user's role (admin mode only)
func (c *RAGFlowClient) AlterUserRole(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	payload := map[string]interface{}{
		"role_name": roleName,
	}

	resp, err := c.HTTPClient.Request("PUT", fmt.Sprintf("/admin/users/%s/role", userName), true, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to alter user role: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to alter user role: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("alter user role failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	// Remove extra field from data
	for _, item := range result.Data {
		delete(item, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ShowUserPermission shows user's permissions (admin mode only)
func (c *RAGFlowClient) ShowUserPermission(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/permission", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show user permission: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show user permission: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show user permission failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	// Remove extra field from data
	for _, item := range result.Data {
		delete(item, "extra")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// GenerateAdminToken generates an API token for a user (admin mode only)
func (c *RAGFlowClient) GenerateAdminToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("POST", fmt.Sprintf("/admin/users/%s/keys", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to generate token: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("generate token failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	delete(result.Data, "update_date")
	delete(result.Data, "update_time")
	delete(result.Data, "create_time")

	result.Duration = resp.Duration
	return &result, nil
}

// ListAdminTokens lists all API tokens for a user (admin mode only)
func (c *RAGFlowClient) ListAdminTokens(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/keys", userName), true, "admin", nil, nil)
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

	// Remove extra field from data
	for _, item := range result.Data {
		delete(item, "dialog_id")
		delete(item, "source")
		delete(item, "update_date")
		delete(item, "update_time")
		delete(item, "create_time")
	}

	result.Duration = resp.Duration
	return &result, nil
}

// DropToken drops an API token for a user (admin mode only)
func (c *RAGFlowClient) DropAdminToken(cmd *Command) (ResponseIf, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	token, ok := cmd.Params["token"].(string)
	if !ok {
		return nil, fmt.Errorf("token not provided")
	}

	// URL encode the token to handle special characters
	encodedToken := url.QueryEscape(token)

	resp, err := c.HTTPClient.Request("DELETE", fmt.Sprintf("/admin/users/%s/keys/%s", userName, encodedToken), true, "admin", nil, nil)
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
