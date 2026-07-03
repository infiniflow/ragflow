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

	return HandleSimpleResponse(resp, "ping admin")
}

// AdminShowVersionCommand show RAGFlow admin version
func (c *CLI) AdminShowVersionCommand(cmd *Command) (ResponseIf, error) {
	resp, err := c.AdminServerClient.Request("GET", "/admin/version", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show admin version: %w", err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("show admin version"))
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

	return HandleCommonDataResponse(resp, fmt.Sprintf("list resources"))
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

// AdminListProvidersCommand to list providers command (admin mode only)
func (c *CLI) AdminListProvidersCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := fmt.Sprintf("/admin/providers")

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	return HandleCommonResponse(resp, "list providers")
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

	return HandleCommonDataResponse(resp, fmt.Sprintf("create role"))
}

// AdminDropRoleCommand deletes the role (admin mode only)
func (c *CLI) AdminDropRoleCommand(cmd *Command) (ResponseIf, error) {
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

	return HandleCommonDataResponse(resp, fmt.Sprintf("drop role"))
}

// AdminAlterRole alters the role rights (admin mode only)
func (c *CLI) AdminAlterRole(cmd *Command) (ResponseIf, error) {
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

	return HandleCommonDataResponse(resp, fmt.Sprintf("alter role"))
}

// AdminGrantUserAdminCommand grants admin privileges to a user (admin mode only)
func (c *CLI) AdminGrantUserAdminCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/admin", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to grant admin: %w", err)
	}

	return HandleSimpleResponse(resp, "grant admin")
}

// AdminRevokeUserAdminCommand revokes admin privileges from a user (admin mode only)
func (c *CLI) AdminRevokeUserAdminCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/admin", encodedUserName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke admin: %w", err)
	}

	return HandleSimpleResponse(resp, "revoke admin")
}

// AdminGrantRolePermissionCommand grants permission to role (admin mode only)
func (c *CLI) AdminGrantRolePermissionCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	actions, ok := cmd.Params["actions"].([]string)
	if !ok {
		return nil, fmt.Errorf("actions not provided")
	}

	resource, ok := cmd.Params["resource"].(string)
	if !ok {
		return nil, fmt.Errorf("resource not provided")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	payload := map[string]interface{}{
		"actions":  actions,
		"resource": resource,
	}

	apiURL := fmt.Sprintf("/admin/roles/%s/permission", roleName)

	resp, err := c.AdminServerClient.Request("POST", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to grant permission to role: %w", err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("grant permission to role"))
}

// AdminRevokeRolePermissionCommand revokes permission from role (admin mode only)
func (c *CLI) AdminRevokeRolePermissionCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	actions, ok := cmd.Params["actions"].([]string)
	if !ok {
		return nil, fmt.Errorf("actions not provided")
	}

	resource, ok := cmd.Params["resource"].(string)
	if !ok {
		return nil, fmt.Errorf("resource not provided")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	payload := map[string]interface{}{
		"actions":  actions,
		"resource": resource,
	}

	apiURL := fmt.Sprintf("/admin/roles/%s/permission", roleName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke permission from role: %w", err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("revoke permission from role"))
}

// AdminShowRolePermissionCommand shows admin privileges from a user (admin mode only)
func (c *CLI) AdminShowRolePermissionCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	apiURL := fmt.Sprintf("/admin/roles/%s/permission", roleName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show role permission: %w", err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("show role permission"))
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

	publicKey, err := c.GetPublicKeyPEM()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password, publicKey)
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

	return HandleSimpleResponse(resp, "create user")
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

	encodedUserName := common.EncodeToBase64(userName)

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

// AdminActivateUser activates or deactivates a user (admin mode only)
func (c *CLI) AdminActivateUser(cmd *Command) (ResponseIf, error) {
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

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/activate", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to update user status: %w", err)
	}

	return HandleSimpleResponse(resp, "update user status")
}

// AdminAlterUserPassword changes a user's password (admin mode only)
func (c *CLI) AdminAlterUserPassword(cmd *Command) (ResponseIf, error) {
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

	publicKey, err := c.GetPublicKeyPEM()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Encrypt password using RSA
	encryptedPassword, err := EncryptPassword(password, publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	payload := map[string]interface{}{
		"new_password": encryptedPassword,
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/password", encodedUserName)

	resp, err := c.AdminServerClient.Request("PUT", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to change user password: %w", err)
	}

	return HandleSimpleResponse(resp, "change user password")
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

	return HandleCommonResponse(resp, "list services")
}

// AdminStartServiceCommand starts a service (admin mode only)
func (c *CLI) AdminStartServiceCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	serviceIndex := cmd.Params["service_index"].(int)

	endPoint := fmt.Sprintf("/admin/services/%d", serviceIndex)

	resp, err := c.AdminServerClient.Request("POST", endPoint, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start service: %w", err)
	}

	return HandleCommonDataResponse(resp, "start service")
}

// AdminRestartServiceCommand restarts a service (admin mode only)
func (c *CLI) AdminRestartServiceCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	serviceIndex := cmd.Params["service_index"].(int)

	endPoint := fmt.Sprintf("/admin/services/%d", serviceIndex)

	resp, err := c.AdminServerClient.Request("PUT", endPoint, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to restart service: %w", err)
	}

	return HandleCommonDataResponse(resp, "restart service")
}

// AdminShutdownServiceCommand shuts down a service (admin mode only)
func (c *CLI) AdminShutdownServiceCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	serviceIndex := cmd.Params["service_index"].(int)

	endPoint := fmt.Sprintf("/admin/services/%d", serviceIndex)

	resp, err := c.AdminServerClient.Request("DELETE", endPoint, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to shutdown service: %w", err)
	}

	return HandleCommonDataResponse(resp, "shutdown service")
}

// AdminShowService show service (admin mode only)
func (c *CLI) AdminShowService(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	serviceIndex := cmd.Params["service_index"].(int)

	endPoint := fmt.Sprintf("/admin/services/%d", serviceIndex)

	resp, err := c.AdminServerClient.Request("GET", endPoint, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show service: %w", err)
	}

	return HandleCommonDataResponse(resp, "show service")
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

// AdminListVariablesCommand lists all system variables (admin mode only).
func (c *CLI) AdminListVariablesCommand(cmd *Command) (ResponseIf, error) {
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

// AdminListConfigsCommand lists all system configs (admin mode only).
func (c *CLI) AdminListConfigsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/configs", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list configs: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result CommonResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("list configs failed: invalid JSON (%w)", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	normalizeVariableRows(result.Data)
	result.Duration = resp.Duration
	return &result, nil
}

// AdminListEnvironmentsCommand lists all system environments (admin mode only).
func (c *CLI) AdminListEnvironmentsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/environments", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	return HandleCommonResponse(resp, "list environments")
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

	encodedVarName := common.EncodeToBase64(varName)

	endPoint := fmt.Sprintf("/admin/variables/%s", encodedVarName)

	resp, err := c.AdminServerClient.Request("GET", endPoint, "admin", nil, nil)
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

	return HandleCommonDataResponse(resp, "set license")
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

	return HandleCommonDataResponse(resp, "set license config")
}

// AdminSetVariableCommand updates a system variable (admin mode only).
func (c *CLI) AdminSetVariableCommand(cmd *Command) (ResponseIf, error) {
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

// AdminSetRoleDefaultModelsCommand set role default models (admin mode only).
func (c *CLI) AdminSetRoleDefaultModelsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	modelType, ok := cmd.Params["model_type"].(string)
	if !ok {
		return nil, fmt.Errorf("model_type not provided")
	}

	payload := map[string]interface{}{
		"model_type": modelType,
	}

	var modelName string
	modelID, ok := cmd.Params["model_id"].(string)
	if ok {
		payload["model_id"] = modelID
	} else {
		modelName, ok = cmd.Params["composite_model_name"].(string)
		if ok {
			payload["model_name"] = modelName
		} else {
			return nil, fmt.Errorf("model_id or model_name not provided")
		}
	}

	endPoint := fmt.Sprintf("/admin/roles/%s/default-models", roleName)

	resp, err := c.AdminServerClient.Request("PATCH", endPoint, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to set role default models: %w", err)
	}

	return HandleCommonDataResponse(resp, "set role default models")
}

// AdminSetLogLevelCommand set log level (admin mode only).
func (c *CLI) AdminSetLogLevelCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	logLevel, ok := cmd.Params["level"].(string)
	if !ok {
		return nil, fmt.Errorf("no log level")
	}

	payload := map[string]interface{}{
		"level": logLevel,
	}

	resp, err := c.AdminServerClient.Request("PUT", "/admin/config/log", "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to change log level: %w", err)
	}

	return HandleSimpleResponse(resp, "change log level")
}

// AdminResetRoleDefaultModelsCommand reset role default models (admin mode only).
func (c *CLI) AdminResetRoleDefaultModelsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName, ok := cmd.Params["role_name"].(string)
	if !ok {
		return nil, fmt.Errorf("role_name not provided")
	}

	modelType, ok := cmd.Params["model_type"].(string)
	if !ok {
		return nil, fmt.Errorf("model_type not provided")
	}

	payload := map[string]interface{}{
		"model_type": modelType,
	}

	endPoint := fmt.Sprintf("/admin/roles/%s/default-models", roleName)

	resp, err := c.AdminServerClient.Request("DELETE", endPoint, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to reset role default models: %w", err)
	}

	return HandleCommonDataResponse(resp, "reset role default models")
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

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s", encodedUserName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop user: %w", err)
	}

	return HandleSimpleResponse(resp, "drop user")
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

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/keys/%s", encodedUserName, apiKey)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to drop API key: %w", err)
	}

	return HandleCommonDataResponse(resp, "drop API key")
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

	encodedUserName := common.EncodeToBase64(userName)
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

// ShowUserPermission shows user's permissions (admin mode only)
func (c *CLI) ShowUserPermission(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
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

func (c *CLI) ListAdminTasks(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/tasks", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	return HandleCommonResponse(resp, "list tasks")
}

func (c *CLI) ListAdminIngestors(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}
	resp, err := c.AdminServerClient.Request("GET", "/admin/ingestors", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list ingestors: %w", err)
	}

	return HandleCommonResponse(resp, "list ingestors")
}

func (c *CLI) ListAdminIngestionTasks(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/ingestion/tasks", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list admin tasks: %w", err)
	}

	return HandleCommonResponse(resp, "list admin tasks")
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
		return nil, fmt.Errorf("failed to stop ingestion: %w", err)
	}

	return HandleCommonResponse(resp, "stop ingestion")
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
		return nil, fmt.Errorf("failed to remove ingestion: %w", err)
	}

	return HandleCommonResponse(resp, "remove ingestion")
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

	return HandleCommonDataResponse(resp, "shutdown ingestor")
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
		return nil, fmt.Errorf("failed to list message queue: %w", err)
	}

	return HandleCommonResponse(resp, "list message queue")
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

	return HandleCommonResponse(resp, "pull message")
}

func (c *CLI) UserShowMessageQueueCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/queue", "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show message queue: %w", err)
	}

	return HandleCommonDataResponse(resp, "show message queue")
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

	return HandleCommonDataResponse(resp, "check license")
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

	return HandleCommonDataResponse(resp, "show fingerprint")
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

	return HandleCommonDataResponse(resp, "show license")
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

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show user: %w", err)
	}

	return HandleCommonDataResponse(resp, "show user")
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

	return HandleCommonDataResponse(resp, "show role")
}

// AdminShowRoleDefaultModelsCommand show role default models command (admin mode only)
func (c *CLI) AdminShowRoleDefaultModelsCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	roleName := cmd.Params["role_name"].(string)

	endPoint := fmt.Sprintf("/admin/roles/%s/default-models", roleName)

	resp, err := c.AdminServerClient.Request("GET", endPoint, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to show role default models: %w", err)
	}

	return HandleCommonResponse(resp, "show role default models")
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

	encodedUserName := common.EncodeToBase64(email)
	apiURL := fmt.Sprintf("/admin/users/%s/activity", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get user activity: %w", err)
	}

	return HandleCommonDataResponse(resp, "get user activity")
}

func (c *CLI) AdminShowUserSummaryCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/summary", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result OrderedCommonDataResponse
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

	encodedUserName := common.EncodeToBase64(email)
	apiURL := fmt.Sprintf("/admin/users/%s/dataset", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get user dataset: %w", err)
	}

	return HandleCommonDataResponse(resp, "get user dataset")
}

func (c *CLI) AdminShowUserStorageCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	email, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(email)
	apiURL := fmt.Sprintf("/admin/users/%s/storage", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result UserStorageResponse
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

	encodedUserName := common.EncodeToBase64(email)
	apiURL := fmt.Sprintf("/admin/users/%s/quota", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result UserQuotaResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user quota failed: invalid JSON (%w)", err)
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

	encodedUserName := common.EncodeToBase64(email)
	apiURL := fmt.Sprintf("/admin/users/%s/index", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user storage: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user storage: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result UserIndexResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get user index failed: invalid JSON (%w)", err)
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

	encodedUserName := common.EncodeToBase64(email)
	apiURL := fmt.Sprintf("/admin/users/%s/permission", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user permission: %w", err)
	}

	return HandleCommonDataResponse(resp, "get user permission")
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

	var result OrderedCommonDataResponse
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

	return HandleCommonDataResponse(resp, "get users activity")
}

func (c *CLI) AdminShowUsersPlanCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	apiURL := "/admin/users/plan/summary"

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get users activity: %w", err)
	}

	return HandleCommonDataResponse(resp, "get users plan")
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

	var result OrderedCommonResponse
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
		return nil, fmt.Errorf("failed to get data summary: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get data summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result OrderedCommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get data summary failed: invalid JSON (%w)", err)
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

	var result OrderedCommonDataResponse
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

	var result OrderedCommonDataResponse
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

	var result OrderedCommonDataResponse
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

	var result QuotaSummaryResponse
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
		return nil, fmt.Errorf("failed to get tasks summary: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get tasks summary: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	var result OrderedCommonDataResponse
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("get tasks summary failed: invalid JSON (%w)", err)
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

	return HandleCommonDataResponse(resp, "purge orphan data")
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

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/data", encodedUserName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to purge user %s: %w", userName, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("purge user %s", userName))
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

	return HandleCommonDataResponse(resp, "purge users data")
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

	var result OrderedCommonResponse
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

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/datasets", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s datasets: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s datasets", userName))
}
func (c *CLI) AdminListUserAgentsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/agents", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s agents: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s agents", userName))
}
func (c *CLI) AdminListUserChatsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/chats", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s chats: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s chats", userName))
}

func (c *CLI) AdminListUserSearchesCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/searches", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s searches: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s searches", userName))
}

func (c *CLI) AdminListUserModelsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/models", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s models: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s models", userName))
}

func (c *CLI) AdminListUserFilesCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/files", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s files: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s files", userName))
}

func (c *CLI) AdminListUserKeysCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
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

func (c *CLI) AdminListUserProvidersCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/providers", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s providers: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s providers", userName))
}

func (c *CLI) AdminListUserProviderInstancesCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/providers/%s/instances", encodedUserName, providerName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s providers: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s provider instances", userName))
}

func (c *CLI) AdminListUserProviderInstanceModelsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}
	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}
	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/providers/%s/instances/%s/models", encodedUserName, providerName, instanceName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s provider instance models: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s provider instance models", userName))
}

func (c *CLI) AdminListUserDefaultModelsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	encodedUserName := common.EncodeToBase64(userName)
	apiURL := fmt.Sprintf("/admin/users/%s/default-models", encodedUserName)

	resp, err := c.AdminServerClient.Request("GET", apiURL, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list user %s default models: %w", userName, err)
	}

	return HandleCommonResponse(resp, fmt.Sprintf("list user %s default models", userName))
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

	return HandleCommonResponse(resp, fmt.Sprintf("stop user %s ingestion tasks", userName))
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

	return HandleCommonResponse(resp, fmt.Sprintf("remove user %s ingestion tasks", userName))
}

// AdminAddProviderCommand add provider
func (c *CLI) AdminAddProviderCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	payload := map[string]interface{}{
		"provider_name": providerName,
	}

	apiURL := fmt.Sprintf("/admin/providers")

	resp, err := c.AdminServerClient.Request("POST", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add provider %s: %w", providerName, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("add provider %s", providerName))
}

// AdminAddModelInstanceCommand add model instance
func (c *CLI) AdminAddModelInstanceCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	payload := map[string]interface{}{
		"instance_name": instanceName,
	}

	apiURL := fmt.Sprintf("/admin/providers/%s/instances", providerName)

	resp, err := c.AdminServerClient.Request("POST", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add model instance %s: %w", instanceName, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("add model instance %s", instanceName))
}

// AdminAddModelsCommand add models
func (c *CLI) AdminAddModelsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	modelNames, ok := cmd.Params["model_names"].([]string)
	if !ok {
		return nil, fmt.Errorf("model_names not provided")
	}

	payload := map[string]interface{}{
		"model_names": modelNames,
	}

	apiURL := fmt.Sprintf("/admin/providers/%s/instances/%s/models", providerName, instanceName)

	resp, err := c.AdminServerClient.Request("POST", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to add models %s: %w", modelNames, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("add models %s", modelNames))
}

// AdminDeleteProvidersCommand delete providers
func (c *CLI) AdminDeleteProvidersCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	providerNames, ok := cmd.Params["provider_names"].([]string)
	if !ok {
		return nil, fmt.Errorf("provider_names not provided")
	}

	payload := map[string]interface{}{
		"provider_names": providerNames,
	}

	apiURL := fmt.Sprintf("/admin/providers/")

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove providers %s: %w", providerNames, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("remove providers %s", providerNames))
}

// AdminDeleteInstancesCommand delete instances
func (c *CLI) AdminDeleteInstancesCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	instanceNames, ok := cmd.Params["instance_names"].([]string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	payload := map[string]interface{}{
		"instance_names": instanceNames,
	}

	apiURL := fmt.Sprintf("/admin/providers/%s/instances", providerName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove instance %s: %w", instanceNames, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("remove instance %s", instanceNames))
}

// AdminDeleteModelsCommand delete models
func (c *CLI) AdminDeleteModelsCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	providerName, ok := cmd.Params["provider_name"].(string)
	if !ok {
		return nil, fmt.Errorf("provider_name not provided")
	}

	instanceName, ok := cmd.Params["instance_name"].(string)
	if !ok {
		return nil, fmt.Errorf("instance_name not provided")
	}

	modelNames, ok := cmd.Params["model_names"].([]string)
	if !ok {
		return nil, fmt.Errorf("model_names not provided")
	}

	payload := map[string]interface{}{
		"model_names": modelNames,
	}

	apiURL := fmt.Sprintf("/admin/providers/%s/instances/%s/models", providerName, instanceName)

	resp, err := c.AdminServerClient.Request("DELETE", apiURL, "admin", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to remove model %s: %w", modelNames, err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("remove model %s", modelNames))
}

func (c *CLI) AdminShowLogLevelCommand(cmd *Command) (ResponseIf, error) {

	if c.Config.CLIMode != AdminMode {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode or already login")
	}

	resp, err := c.AdminServerClient.Request("GET", "/admin/config/log", "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get log level config: %w", err)
	}

	return HandleCommonDataResponse(resp, fmt.Sprintf("get log level config"))
}
