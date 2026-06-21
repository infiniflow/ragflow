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
	"ragflow/internal/common"
)

// PingServer pings the server to check if it's alive
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *CLI) PingAdmin(cmd *Command) (ResponseIf, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		return c.AdminServerClient.RequestWithIterations("GET", "/admin/ping", "web", nil, nil, iterations)
	}

	// Single mode
	resp, err := c.AdminServerClient.Request("GET", "/admin/ping", "web", nil, nil)
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

// AdminShowVersionCommand show RAGFlow admin version
func (c *CLI) AdminShowVersionCommand(cmd *Command) (ResponseIf, error) {
	resp, err := c.AdminServerClient.Request("GET", "/admin/version", "web", nil, nil)
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

// AdminListResourcesCommand to list resources command (admin mode only)
func (c *CLI) AdminListResourcesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/roles/resource", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list resources: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list resources failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// AdminListRolesCommand to list roles command (admin mode only)
func (c *CLI) AdminListRolesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.AdminServerClient.RequestWithIterations("GET", "/admin/roles", "admin", nil, nil, iterations)
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/roles", "admin", nil, nil)
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

// AdminCreateRoleCommand creates a new role (admin mode only)
func (c *CLI) AdminCreateRoleCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	payload := map[string]interface{}{
		"role_name": roleName,
	}
	description, ok := cmd.Params["description"].(string)
	if ok {
		payload["description"] = description
	}

	resp, err := c.AdminServerClient.Request("POST", "/admin/roles", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create role: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse

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
func (c *CLI) DropRole(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	resp, err := c.AdminServerClient.Request("DELETE", fmt.Sprintf("/admin/roles/%s", roleName), "admin", nil, nil)
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
func (c *CLI) AlterRole(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	resp, err := c.AdminServerClient.Request("PUT", fmt.Sprintf("/admin/roles/%s", roleName), "admin", nil, payload)
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
func (c *CLI) GrantAdmin(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/admin", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, nil)
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
func (c *CLI) RevokeAdmin(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/admin", encodedUserName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, nil)
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

// AdminCreateUserCommand creates a new user (admin mode only)
func (c *CLI) AdminCreateUserCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	resp, err := c.AdminServerClient.Request("POST", "/admin/users", "admin", nil, payload)
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

// AdminCreateUserAPIKeyCommand creates a new user API key (admin mode only)
func (c *CLI) AdminCreateUserAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)

	apiURL := fmt.Sprintf("/admin/users/%s/keys", encodedUserName)

	resp, err := c.AdminServerClient.Request("POST", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to create API key: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("create API key failed: invalid JSON (%w)", err)
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

// ActivateUser activates or deactivates a user (admin mode only)
func (c *CLI) ActivateUser(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/activate", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, payload)
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
func (c *CLI) AlterUserPassword(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/password", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, payload)
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

// AdminListServicesCommand lists all services (admin mode only)
func (c *CLI) AdminListServicesCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/services", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list services: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list services failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// AdminShowService show service (admin mode only)
func (c *CLI) AdminShowService(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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
		return c.AdminServerClient.RequestWithIterations("GET", endPoint, "admin", nil, nil, iterations)
	}

	resp, err := c.AdminServerClient.Request("GET", endPoint, "admin", nil, nil)
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

func normalizeVariableRows(rows []map[string]interface{}) {
	for _, row := range rows {
		if _, ok := row["setting_type"]; ok {
			delete(row, "source")
			continue
		}
		if _, ok := row["source"]; ok {
			row["setting_type"] = "config"
			delete(row, "source")
		}
	}
}

// AdminListVariables lists all system variables (admin mode only).
func (c *CLI) AdminListVariables(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/variables", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list variables: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list variables: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list variables failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	normalizeVariableRows(result.Data)
	result.Duration = resp.Duration
	return &result, nil
}

// AdminShowVariable shows system variables by exact name or name prefix (admin mode only).
func (c *CLI) AdminShowVariable(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	varName, ok := cmd.Params["var_name"].(string)
	if !ok {
		return nil, fmt.Errorf("var_name not provided")
	}

	payload := map[string]interface{}{"var_name": varName}

	resp, err := c.AdminServerClient.Request("GET", "/admin/variables", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to show variable: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show variable: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
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

func (c *CLI) AdminSetLicenseCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	license, ok := cmd.Params["license"].(string)
	if !ok {
		return nil, fmt.Errorf("license not provided")
	}

	payload := map[string]interface{}{
		"license": license,
	}
	resp, err := c.AdminServerClient.Request("POST", "/admin/system/license", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to set license: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to set license: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("set license failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminSetLicenseConfigCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	value1, ok := cmd.Params["number1"].(int)
	if !ok {
		return nil, fmt.Errorf("number1 not provided")
	}
	value2, ok := cmd.Params["number2"].(int)
	if !ok {
		return nil, fmt.Errorf("number2 not provided")
	}

	payload := map[string]interface{}{
		"value1": value1,
		"value2": value2,
	}
	resp, err := c.AdminServerClient.Request("PUT", "/admin/system/license/config", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to set license config: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to set license config: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("set license config failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// SetVariable updates a system variable (admin mode only).
func (c *CLI) SetVariable(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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
	resp, err := c.AdminServerClient.Request("PUT", "/admin/variables", "admin", nil, payload)
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

// AdminDropUserCommand deletes a user (admin mode only)
func (c *CLI) AdminDropUserCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s", encodedUserName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, nil)
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

// AdminDropUserAPIKeyCommand drops an API key for a user (admin mode only)
func (c *CLI) AdminDropUserAPIKeyCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	apiKey, ok := cmd.Params["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("api_key not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/keys/%s", encodedUserName, apiKey)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop API key: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop API key: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop API key failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ListUserDatasets lists datasets for a specific user (admin mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *CLI) ListUserDatasets(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/datasets", encodedUserName)

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.AdminServerClient.RequestWithIterations("GET", apiURL, "admin", nil, nil, iterations)
	}

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
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

// GrantPermission grants permission to a role (admin mode only)
func (c *CLI) GrantPermission(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/keys", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
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
func (c *CLI) RevokePermission(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	resp, err := c.AdminServerClient.Request("DELETE", fmt.Sprintf("/admin/roles/%s/permission", roleName), "admin", nil, payload)
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
func (c *CLI) AlterUserRole(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
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

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/role", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, payload)
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
func (c *CLI) ShowUserPermission(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/permission", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
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
func (c *CLI) GenerateAdminToken(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/keys", encodedUserName)

	resp, err := c.AdminServerClient.Request("POST", apiURL, "admin", nil, nil)
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
func (c *CLI) ListAdminTokens(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/keys", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
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

func (c *CLI) ListAdminTasks(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/tasks", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop token: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to drop token: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("drop token failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ListAdminIngestors(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}
	resp, err := c.AdminServerClient.Request("GET", "/admin/ingestors", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list ingestors: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list ingestors: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list ingestors failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) ListAdminIngestionTasks(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/ingestion/tasks", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list admin tasks: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list admin tasks: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list admin tasks failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminStopIngestionCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	tasks, ok := cmd.Params["tasks"].([]string)
	if !ok {
		return nil, fmt.Errorf("uri not provided")
	}
	payload := map[string]interface{}{
		"tasks": tasks,
	}

	resp, err := c.AdminServerClient.Request("PUT", "/admin/ingestion/tasks", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to ingest file: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ingest file: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ingest file failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminRemoveIngestionCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	tasks, ok := cmd.Params["tasks"].([]string)
	if !ok {
		return nil, fmt.Errorf("uri not provided")
	}
	payload := map[string]interface{}{
		"tasks": tasks,
	}

	resp, err := c.AdminServerClient.Request("DELETE", "/admin/ingestion/tasks", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to ingest file: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to ingest file: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ingest file failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShutdownIngestor(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	ingestorName, ok := cmd.Params["ingestor_name"].(string)
	if !ok {
		return nil, fmt.Errorf("ingestor_name not provided")
	}
	payload := map[string]interface{}{
		"ingestor_name": ingestorName,
	}

	resp, err := c.AdminServerClient.Request("DELETE", "/admin/ingestors", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to shutdown ingestor: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to shutdown ingestor: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("shutdown ingestor failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserListMessageQueueCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	pending, ok := cmd.Params["pending"].(bool)
	if !ok {
		return nil, fmt.Errorf("pending not provided")
	}
	payload := map[string]interface{}{
		"pending": pending,
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/queue/messages", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks in message queue: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list tasks in message queue: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list tasks in message queue failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserPublishMessageCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	message, ok := cmd.Params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message not provided")
	}
	payload := map[string]interface{}{
		"message": message,
	}

	resp, err := c.AdminServerClient.Request("POST", "/admin/queue/messages", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to publish message: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("publish message failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserPullMessageCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	messageCount, ok := cmd.Params["message_count"].(int)
	if !ok {
		return nil, fmt.Errorf("message_count not provided")
	}
	ackPolicy, ok := cmd.Params["ack_policy"].(string)
	if !ok {
		return nil, fmt.Errorf("ack_policy not provided")
	}

	payload := map[string]interface{}{
		"message_count": messageCount,
		"ack_policy":    ackPolicy,
	}

	resp, err := c.AdminServerClient.Request("PUT", "/admin/queue/messages", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to pull message: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to pull message: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("pull message failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) UserShowMessageQueueCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/queue", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show message queue: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show message queue: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show message queue failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminRemoveServiceCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}
	serviceNumber, ok := cmd.Params["service_number"].(int)
	if !ok {
		return nil, fmt.Errorf("service_number not provided")
	}

	payload := map[string]interface{}{
		"service_number": serviceNumber,
	}

	resp, err := c.AdminServerClient.Request("DELETE", "/admin/services", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove unavailable service: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to remove unavailable service: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result SimpleResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("remove unavailable service failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// AdminCheckLicenseCommand check license command (admin mode only)
func (c *CLI) AdminCheckLicenseCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := fmt.Sprintf("/admin/system/license?check=true")

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to check license: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to check license: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("check license failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// AdminShowFingerprintCommand show fingerprint command (admin mode only)
func (c *CLI) AdminShowFingerprintCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := fmt.Sprintf("/admin/system/fingerprint")

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show fingerprint: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show fingerprint: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show fingerprint failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// AdminShowLicenseCommand show license command (admin mode only)
func (c *CLI) AdminShowLicenseCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := fmt.Sprintf("/admin/system/license")

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show license: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to show license: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse

	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("show license failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = resp.Duration
	return &result, nil
}

// AdminShowUserCommand show user command (admin mode only)
func (c *CLI) AdminShowUserCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
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

// AdminShowRoleCommand show role command (admin mode only)
func (c *CLI) AdminShowRoleCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName := cmd.Params["role_name"].(string)

	endPoint := fmt.Sprintf("/admin/roles/%s/", roleName)

	resp, err := c.AdminServerClient.Request("GET", endPoint, "admin", nil, nil)
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

func (c *CLI) AdminShowUserActivityCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	days, ok := cmd.Params["days"].(int)
	if !ok {
		return nil, fmt.Errorf("days not provided")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	payload := map[string]interface{}{
		"days":  days,
		"email": email,
	}

	encodedUserName := common.EncodeEmail(email)
	apiURL := fmt.Sprintf("/admin/users/%s/activity", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get user activity: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user activity: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user activity failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUserSummaryCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/summary", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user summary failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUserDatasetCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	dataset, ok := cmd.Params["dataset_name"].(string)
	if !ok {
		return nil, fmt.Errorf("dataset_name not provided")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	payload := map[string]interface{}{
		"dataset": dataset,
	}

	encodedUserName := common.EncodeEmail(email)
	apiURL := fmt.Sprintf("/admin/users/%s/dataset", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get user dataset: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user dataset: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user dataset failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUserStorageCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(email)
	apiURL := fmt.Sprintf("/admin/users/%s/admin", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user storage failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUserQuotaCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(email)
	apiURL := fmt.Sprintf("/admin/users/%s/quota", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user storage failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUserIndexCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(email)
	apiURL := fmt.Sprintf("/admin/users/%s/index", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user storage failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUserPermissionCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(email)
	apiURL := fmt.Sprintf("/admin/users/%s/permission", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user permission: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user permission: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user permission failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUsersSummaryCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/users/summary"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get users summary: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get users summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get users summary failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowUsersActivityCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	days, ok := cmd.Params["days"].(int)
	if !ok {
		return nil, fmt.Errorf("days not provided")
	}
	window, ok := cmd.Params["window"].(int)
	if !ok {
		return nil, fmt.Errorf("window not provided")
	}

	payload := map[string]interface{}{
		"days":   days,
		"window": window,
	}

	apiURL := "/admin/users/activity"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get users activity: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get users activity: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get users activity failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

// ListUsers lists all users (admin mode only)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *CLI) AdminListUsersCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.AdminServerClient.RequestWithIterations("GET", "/admin/users", "admin", nil, nil, iterations)
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/users", "admin", nil, nil)
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

func (c *CLI) AdminListUsersConditionCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	var orderBy *string
	var userStatus *string
	var top *int
	var plan *string
	var quota *int
	var days *int

	orderByStr, ok := cmd.Params["order_by"].(string)
	if ok {
		orderBy = &orderByStr
	}
	userStatusStr, ok := cmd.Params["user_status"].(string)
	if ok {
		userStatus = &userStatusStr
	}
	topInt, ok := cmd.Params["top"].(int)
	if ok {
		top = &topInt
	}
	planStr, ok := cmd.Params["plan"].(string)
	if ok {
		plan = &planStr
	}
	quotaInt, ok := cmd.Params["quota"].(int)
	if ok {
		quota = &quotaInt
	}
	daysInt, ok := cmd.Params["days"].(int)
	if ok {
		days = &daysInt
	}

	payload := map[string]interface{}{
		"enterprise":  true,
		"order_by":    orderBy,
		"user_status": userStatus,
		"top":         top,
		"plan":        plan,
		"quota":       quota,
		"days":        days,
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/users", "admin", nil, payload)
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

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowDataSummaryCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/data/summary"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get users summary: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get users summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get users summary failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowDataOrphanCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/data/orphan"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get orphan data: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get orphan data: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get orphan data failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowDataStorageCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/data/storage"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get data storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get data storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get data storage failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowDataIndexCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/data/index"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get data index: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get data index: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get data index failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowQuotaSummaryCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/users/quota/summary"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get users quota summary: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get users quota summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get users quota summary failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminShowTasksSummaryCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/ingestion/tasks/summary"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get users quota summary: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get users quota summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get users quota summary failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminPurgeOrphanCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	preview, ok := cmd.Params["preview"].(bool)
	if !ok {
		return nil, fmt.Errorf("preview not provided")
	}

	payload := map[string]interface{}{
		"preview": preview,
	}

	apiURL := "/admin/data/orphan"

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to purge orphan data: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to purge orphan data: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("purge orphan data failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminPurgeUserCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	preview, ok := cmd.Params["preview"].(bool)
	if !ok {
		return nil, fmt.Errorf("preview not provided")
	}
	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	payload := map[string]interface{}{
		"preview": preview,
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/data", encodedUserName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to purge user %s: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to purge user %s: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("purge user %s failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminPurgeUsersCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	var preview bool
	var ok bool
	var planName string
	var planNamePtr *string
	var userStatus string
	var userStatusPtr *string
	var days int

	preview, ok = cmd.Params["preview"].(bool)
	if !ok {
		return nil, fmt.Errorf("preview not provided")
	}

	planName, ok = cmd.Params["plan_name"].(string)
	if ok {
		planNamePtr = &planName
	}

	userStatus, ok = cmd.Params["user_status"].(string)
	if ok {
		userStatusPtr = &userStatus
	}

	days, ok = cmd.Params["days"].(int)
	if !ok {
		return nil, fmt.Errorf("days not provided")
	}

	payload := map[string]interface{}{
		"preview":     preview,
		"days":        days,
		"plan":        planNamePtr,
		"user_status": userStatusPtr,
	}

	apiURL := "/admin/users/data"

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to purge users data: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to purge users data: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("purge users data failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminListUserIngestionTasksCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("plan_name not provided")
	}

	payload := map[string]interface{}{
		"email": userName,
	}

	status, ok := cmd.Params["status"].(string)
	if ok {
		payload["status"] = status
	}

	apiURL := "/admin/ingestion/tasks"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s ingestion tasks: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s ingestion tasks: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s ingestion tasks failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminListUserDatasetsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/datasets", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s datasets: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s datasets: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s datasets failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}
func (c *CLI) AdminListUserAgentsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/agents", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s agents: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s agents: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s agents failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}
func (c *CLI) AdminListUserChatsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/chats", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s chats: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s chats: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s chats failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}
func (c *CLI) AdminListUserSearchesCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/searches", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s searches: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s searches: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s searches failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}
func (c *CLI) AdminListUserModelsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/models", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s models: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s models: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s models failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}
func (c *CLI) AdminListUserFilesCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/files", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s files: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s files: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s files failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminListUserKeysCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeEmail(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/keys", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s keys: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list user %s keys: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list user %s keys failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminStopUserIngestionTasksCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	payload := map[string]interface{}{
		"email": userName,
	}

	status, ok := cmd.Params["status"].(string)
	if ok {
		payload["status"] = status
	}

	apiURL := "/admin/ingestion/tasks"

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to stop user %s ingestion tasks: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to stop user %s ingestion tasks: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("stop user %s ingestion tasks failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}

func (c *CLI) AdminRemoveUserIngestionTasksCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	payload := map[string]interface{}{
		"email": userName,
	}

	status, ok := cmd.Params["status"].(string)
	if ok {
		payload["status"] = status
	}

	apiURL := "/admin/ingestion/tasks"

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove user %s ingestion tasks: %w", userName, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to remove user %s ingestion tasks: HTTP %d, body: %s", userName, resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("remove user %s ingestion tasks failed: invalid JSON (%w)", userName, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = resp.Duration
	return &result, nil
}
