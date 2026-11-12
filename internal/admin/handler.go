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

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/server"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler admin handler
type Handler struct {
	service     *Service
	userService *service.UserService
}

// NewHandler create admin handler
func NewHandler(svc *Service) *Handler {
	return &Handler{
		service:     svc,
		userService: service.NewUserService(),
	}
}

// ErrorResponse error response
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Healthz to get system health
func (h *Handler) Healthz(c *gin.Context) {
	result, allOK := service.GetComponentsHealthz(c.Request.Context())
	if allOK {
		common.RespondSuccess(c, result, "success")
	} else {
		common.Respond(c, http.StatusInternalServerError, common.CodeServerError, result, "")
	}
}

// Live endpoint
func (h *Handler) Live(c *gin.Context) {
	common.RespondSuccess(c, nil, "")
}

// Ping ping endpoint
func (h *Handler) Ping(c *gin.Context) {
	common.RespondSuccess(c, nil, "pong")
}

// Login handle admin login
// @Summary Admin Login
// @Description Admin login verification using email, only superuser can log in
// @Tags admin
// @Accept JSON
// @Produce JSON
// @Param request body service.EmailLoginRequest true "login info with email"
// @Success 200 {object} map[string]interface{}
// @Router /admin/login [post]
func (h *Handler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	var req service.EmailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
		return
	}

	// Use userService.LoginByEmail with adminLogin=true
	// This allows default admin account to log in admin system
	user, code, err := h.userService.LoginByEmail(ctx, &req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	// Check if user is superuser (admin)
	if user.IsSuperuser == nil || !*user.IsSuperuser {
		common.RespondError(c, common.CodeForbidden, "Only superuser can login admin system")
		return
	}

	secretKey, err := server.GetSecretKey(ctx, kvrocks.Get())
	if err != nil {
		common.RespondError(c, common.CodeServerError, fmt.Sprintf("Failed to get secret key: %s", err.Error()))
		return
	}

	authToken, err := common.DumpAccessToken(*user.AccessToken, "", secretKey)
	if err != nil {
		common.RespondError(c, common.CodeServerError, fmt.Sprintf("Failed to generate auth token: %s", err.Error()))
		return
	}

	// Set Authorization header with access_token
	c.Header("Authorization", authToken)

	common.RespondSuccess(c, user, "Welcome back!")
}

// Logout handle logout
func (h *Handler) Logout(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		common.RespondError(c, common.CodeUnauthorized, "Not authenticated")
		return
	}

	ctx := c.Request.Context()
	if err := h.service.Logout(ctx, user); err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Logout successful")
}

// AuthCheck check admin auth
func (h *Handler) AuthCheck(c *gin.Context) {
	common.RespondSuccess(c, nil, "Admin is authorized")
}

// ListUsers handle list users
func (h *Handler) ListUsers(c *gin.Context) {

	name := c.DefaultQuery("keyword", "")
	status := c.DefaultQuery("status", "")
	sort := c.DefaultQuery("sort", "")     // asc or desc
	orderBy := c.DefaultQuery("order", "") // order by field
	// The admin UI and existing admin clients request the user list without
	// pagination and slice it client-side, so omitting both page and page_size
	// returns every user (pageSizeInt=0). When pagination is requested, use
	// the standard pagination parser.
	query := c.Request.URL.Query()
	var (
		pageInt     int
		pageSizeInt int
	)
	if query.Has("page") || query.Has("page_size") {
		var err error
		pageInt, pageSizeInt, err = common.ParsePagination(query)
		if err != nil {
			common.RespondError(c, common.CodeBadRequest, err.Error())
			return
		}
	} else {
		pageInt = 1
		pageSizeInt = 0
	}

	plan := c.Query("plan") // plan name
	topInt, err := common.ParseRequestIntPositive(c, c.Query("top"), "top", 0)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	if !validateAdminUserListPagination(c, pageInt, pageSizeInt, topInt) {
		return
	}
	// quota is optional: nil when the query param is absent, &v when present
	// (including 0). This lets ListUsersEE distinguish "no quota filter" from
	// "filter by quota threshold 0".
	var quotaPtr *int
	if quotaStr := c.Query("quota"); quotaStr != "" {
		q, err := common.ParseRequestIntPositive(c, quotaStr, "quota", 0)
		if err != nil {
			common.RespondError(c, common.CodeBadRequest, err.Error())
			return
		}
		if q > 100 {
			common.RespondError(c, common.CodeBadRequest, "Quota must be less than or equal to 100")
			return
		}
		quotaPtr = &q
	}
	daysInt, err := common.ParseRequestIntPositive(c, c.Query("days"), "days", 0)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()
	var users []map[string]interface{}
	var total int64
	switch common.GetRAGFlowType() {
	case common.OpenSourceVersion:
		users, total, err = h.service.ListUsers(ctx, pageInt, pageSizeInt, name, status, sort, orderBy)
	case common.EnterpriseEdition:
		users, total, err = h.service.ListUsersEE(ctx, pageInt, pageSizeInt, name, status, sort, orderBy, plan, topInt, daysInt, quotaPtr)
	default:
		common.RespondError(c, common.CodeBadRequest, "Invalid RAGFlow type")
		return
	}
	if errors.Is(err, errInvalidUserListQuery) {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}
	common.RespondSuccessList(c, users, "List users", total)
}

func validateAdminUserListPagination(c *gin.Context, page, pageSize, top int) bool {
	// pageSize==0 is the "unpaged" sentinel: it only arrives from the omitted
	// page_size default, because an explicit page_size=0 is rejected here.
	query := c.Request.URL.Query()
	if page < 1 || (pageSize < 1 && query.Has("page_size")) {
		common.RespondError(c, common.CodeBadRequest, "page and page_size must be positive integers")
		return false
	}
	if query.Has("top") {
		if top < 1 {
			common.RespondError(c, common.CodeBadRequest, "top must be a positive integer")
			return false
		}
		if query.Has("page") || query.Has("page_size") {
			common.RespondError(c, common.CodeBadRequest, "top cannot be combined with page or page_size")
			return false
		}
		switch strings.ToLower(strings.TrimSpace(c.Query("order"))) {
		case "storage", "documents", "index":
		default:
			common.RespondError(c, common.CodeBadRequest, "top is only supported for storage, documents and index")
			return false
		}
	}
	return true
}

// CreateUserHTTPRequest create user request
type CreateUserHTTPRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

// CreateUser handle create user
func (h *Handler) CreateUser(c *gin.Context) {
	var req CreateUserHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Username and password are required: "+err.Error())
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}
	ctx := c.Request.Context()

	userInfo, err := h.service.CreateUser(ctx, req.Username, req.Password, req.Role)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, userInfo, "User created successfully")
}

func getUserName(c *gin.Context) (string, error) {
	username := c.Param("username")
	if username == "" {
		err := errors.New("username is required")
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return "", err
	}
	return username, nil
}

// GetUser handle get user
func (h *Handler) GetUser(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	userDetails, err := h.service.GetUserDetails(c.Request.Context(), username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.RespondError(c, common.CodeNotFound, "User not found")
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, []map[string]interface{}{userDetails}, "")
}

// DeleteUser handle delete user
func (h *Handler) DeleteUser(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	result, err := h.service.DeleteUser(c.Request.Context(), username)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	detailsMsg := "Successfully deleted user. Details:\n"
	for _, detail := range result.DeletedDetails {
		detailsMsg += detail + "\n"
	}

	common.RespondSuccess(c, nil, detailsMsg)
}

// ChangePasswordHTTPRequest change password request
type ChangePasswordHTTPRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword handle change password
func (h *Handler) ChangePassword(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	var req ChangePasswordHTTPRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "New password is required: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	if err = h.service.ChangePassword(ctx, username, req.NewPassword); err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Password updated successfully")
}

// UpdateActivateStatusHTTPRequest update activate status request
type UpdateActivateStatusHTTPRequest struct {
	ActivateStatus string `json:"activate_status" binding:"required"`
}

// UpdateUserActivateStatus handle update user activate status
func (h *Handler) UpdateUserActivateStatus(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	var req UpdateActivateStatusHTTPRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Activation status is required: "+err.Error())
		return
	}

	if req.ActivateStatus != "on" && req.ActivateStatus != "off" {
		common.RespondError(c, common.CodeBadRequest, "Activation status must be 'on' or 'off'")
		return
	}

	isActive := req.ActivateStatus == "on"
	ctx := c.Request.Context()
	if err = h.service.UpdateUserActivateStatus(ctx, username, isActive); err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Activation status updated")
}

// GrantAdmin handle grant admin role
func (h *Handler) GrantAdmin(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	// Get current user email from context
	email, _ := c.Get("email")
	if email != nil && email.(string) == username {
		common.RespondError(c, common.CodeConflict, "can't grant current user: "+username)
		return
	}

	ctx := c.Request.Context()
	if err = h.service.GrantAdmin(ctx, username); err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Admin role granted")
}

// RevokeAdmin handle revoke admin role
func (h *Handler) RevokeAdmin(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	// Get current user email from context
	email, _ := c.Get("email")
	if email != nil && email.(string) == username {
		common.RespondError(c, common.CodeConflict, "can't revoke current user: "+username)
		return
	}

	ctx := c.Request.Context()
	if err = h.service.RevokeAdmin(ctx, username); err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Admin role revoked")
}

// ListUserAPITokens handle get user API keys
func (h *Handler) ListUserAPITokens(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	ctx := c.Request.Context()
	apiKeys, err := h.service.ListUserAPITokens(ctx, username)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, apiKeys, "Get user API keys")
}

// GenerateUserAPIToken handle generate user API key
func (h *Handler) GenerateUserAPIToken(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	ctx := c.Request.Context()
	apiKey, err := h.service.GenerateUserAPIToken(ctx, username)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, apiKey, "API key generated successfully")
}

// DeleteUserAPIToken handle delete user API key
func (h *Handler) DeleteUserAPIToken(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}
	key := c.Param("token")
	if key == "" {
		common.RespondError(c, common.CodeBadRequest, "Username and key are required")
		return
	}

	ctx := c.Request.Context()
	if err = h.service.DeleteUserAPIToken(ctx, username, key); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "API key deleted successfully")
}

// GetServices handle get all services
func (h *Handler) GetServices(c *gin.Context) {
	ctx := c.Request.Context()
	services, err := h.service.ListServices(ctx)
	if err != nil {
		common.Respond(c, http.StatusInternalServerError, common.CodeBadRequest, nil, err.Error())
		return
	}

	common.RespondSuccessList(c, services, "Get all services")
}

// GetServicesByType handle get services by type
func (h *Handler) GetServicesByType(c *gin.Context) {
	serviceType := c.Param("service_type")
	if serviceType == "" {
		common.RespondError(c, common.CodeBadRequest, "Service type is required")
		return
	}

	services, err := h.service.GetServicesByType(serviceType)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, services, "")
}

// GetService handle get service details
func (h *Handler) GetService(c *gin.Context) {
	serviceName := c.Param("service_name")
	if serviceName == "" {
		common.RespondError(c, common.CodeBadRequest, "Service name is required")
		return
	}

	ctx := c.Request.Context()
	services, err := h.service.ListServices(ctx)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	for _, serviceInstance := range services {
		if serviceInstance.Name == serviceName {
			common.RespondSuccess(c, serviceInstance, "")
			return
		}
	}

	common.RespondError(c, common.CodeNotFound, "Service not found")
}

// ShutdownService handle shutdown service
func (h *Handler) ShutdownService(c *gin.Context) {
	serviceName := c.Param("service_name")
	if serviceName == "" {
		common.RespondError(c, common.CodeBadRequest, "Service name is required")
		return
	}

	result, err := h.service.ShutdownService(serviceName)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, result, "")
}

// StartService handle start service
func (h *Handler) StartService(c *gin.Context) {
	serviceName := c.Param("service_name")
	if serviceName == "" {
		common.RespondError(c, common.CodeBadRequest, "Service name is required")
		return
	}

	result, err := h.service.StartService(serviceName)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, result, "")
}

// RestartService handle restart service
func (h *Handler) RestartService(c *gin.Context) {
	serviceName := c.Param("service_name")
	if serviceName == "" {
		common.RespondError(c, common.CodeBadRequest, "Service name is required")
		return
	}

	result, err := h.service.RestartService(serviceName)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, result, "")
}

// ListVariables handle list variables
func (h *Handler) ListVariables(c *gin.Context) {
	ctx := c.Request.Context()
	// Check if request has body content
	if c.Request.ContentLength == 0 || c.Request.ContentLength == -1 {
		// List all variables
		variables, err := h.service.ListAllVariables(ctx)
		if err != nil {
			common.RespondError(c, common.CodeServerError, err.Error())
			return
		}
		common.RespondSuccessList(c, variables, "")
		return
	}

	// Get single variable by var_name from request body
	var req struct {
		VarName string `json:"var_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.VarName == "" {
		common.RespondError(c, common.CodeBadRequest, "Var name is required")
		return
	}

	variable, err := h.service.GetVariable(ctx, req.VarName)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, variable, "")
}

// ShowVariable handle show variable
func (h *Handler) ShowVariable(c *gin.Context) {
	encodedVarName := c.Param("var_name")
	varName, err := common.DecodeFromBase64(encodedVarName)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	if varName == "" {
		common.RespondError(c, common.CodeBadRequest, "Var name is required")
		return
	}

	ctx := c.Request.Context()
	variable, err := h.service.GetVariable(ctx, varName)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, variable, "")
}

// SetVariableHTTPRequest set variable request
type SetVariableHTTPRequest struct {
	VarName  string          `json:"var_name"`
	VarValue json.RawMessage `json:"var_value"`
}

// Missing values are reported separately from unparsable ones so the client can
// tell "you forgot the field" apart from "the field is malformed".
var (
	errMissingVariableValue = errors.New("Var value is required")
	errInvalidVariableValue = errors.New("Var value is invalid")
)

// flattenedVariableValue renders a JSON value into the textual form stored in
// system_settings.value. Every data type shares that one textual column, so
// scalars arriving as their native JSON types are rendered back to their
// literal instead of being rejected for not being a JSON string. Objects and
// arrays keep their JSON encoding verbatim, which is the stored form of
// json-typed settings.
func (r SetVariableHTTPRequest) flattenedVariableValue() (string, error) {
	raw := bytes.TrimSpace(r.VarValue)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", errMissingVariableValue
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}

	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		return strconv.FormatBool(boolean), nil
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String(), nil
	}

	if json.Valid(raw) {
		return string(raw), nil
	}
	return "", errInvalidVariableValue
}

// SetVariable handle set variable
func (h *Handler) SetVariable(c *gin.Context) {
	var req SetVariableHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.VarName == "" {
		common.RespondError(c, common.CodeBadRequest, "Var name is required")
		return
	}

	varValue, err := req.flattenedVariableValue()
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()
	if err := h.service.SetVariable(ctx, req.VarName, varValue); err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Set variable successfully")
}

// ListConfigs returns configurations, including plaintext credentials, for
// superuser inspection. AuthMiddleware enforces superuser access.
func (h *Handler) ListConfigs(c *gin.Context) {
	configs, err := h.service.ListAllConfigs()
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, configs, "")
}

// ListEnvironments handle list environments
func (h *Handler) ListEnvironments(c *gin.Context) {
	environments, err := h.service.ListEnvironments()
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, environments, "")
}

// GetVersion handle get version
func (h *Handler) GetVersion(c *gin.Context) {
	version, versionType := h.service.GetVersion()
	common.RespondSuccess(c, gin.H{"version": version, "version_type": versionType}, "")
}

// ListSandboxProviders handle list sandbox providers
func (h *Handler) ListSandboxProviders(c *gin.Context) {
	providers, err := h.service.ListSandboxProviders()
	if err != nil {
		writeSandboxError(c, err)
		return
	}

	common.RespondSuccessList(c, providers, "")
}

// GetSandboxProviderSchema handle get sandbox provider schema
func (h *Handler) GetSandboxProviderSchema(c *gin.Context) {
	providerID := c.Param("provider_id")
	if providerID == "" {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Provider ID is required")
		return
	}

	schema, err := h.service.GetSandboxProviderSchema(providerID)
	if err != nil {
		writeSandboxError(c, err)
		return
	}

	common.RespondSuccess(c, schema, "")
}

// GetSandboxConfig handle get sandbox config
func (h *Handler) GetSandboxConfig(c *gin.Context) {
	config, err := h.service.GetSandboxConfig(c.Request.Context(), c.Query("provider_type"))
	if err != nil {
		writeSandboxError(c, err)
		return
	}

	common.RespondSuccess(c, config, "")
}

// SetSandboxConfigHTTPRequest set sandbox config request
type SetSandboxConfigHTTPRequest struct {
	ProviderType string                 `json:"provider_type" binding:"required"`
	Config       map[string]interface{} `json:"config"`
	SetActive    *bool                  `json:"set_active"`
}

// SetSandboxConfig handle set sandbox config
func (h *Handler) SetSandboxConfig(c *gin.Context) {
	var req SetSandboxConfigHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid JSON request: provider_type must be a nonempty string, config an object, and set_active a boolean: "+err.Error())
		return
	}

	if req.ProviderType == "" {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Provider type is required")
		return
	}

	setActive := req.SetActive == nil || *req.SetActive
	result, err := h.service.SetSandboxConfig(c.Request.Context(), req.ProviderType, req.Config, setActive)
	if err != nil {
		writeSandboxError(c, err)
		return
	}

	common.RespondSuccess(c, result, "Sandbox configuration updated successfully")
}

// TestSandboxConnectionHTTPRequest test sandbox connection request
type TestSandboxConnectionHTTPRequest struct {
	ProviderType string                 `json:"provider_type" binding:"required"`
	Config       map[string]interface{} `json:"config"`
}

// TestSandboxConnection handle test sandbox connection
func (h *Handler) TestSandboxConnection(c *gin.Context) {
	var req TestSandboxConnectionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid JSON request: provider_type must be a nonempty string and config an object: "+err.Error())
		return
	}

	if req.ProviderType == "" {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Provider type is required")
		return
	}

	result, err := h.service.TestSandboxConnection(c.Request.Context(), req.ProviderType, req.Config)
	if err != nil {
		writeSandboxError(c, err)
		return
	}
	if success, ok := result["success"].(bool); ok && !success {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, result, "Sandbox connection test failed")
		return
	}

	common.RespondSuccess(c, result, "")
}

func writeSandboxError(c *gin.Context, err error) {
	var coded *common.CodedError
	if errors.As(err, &coded) {
		common.Respond(c, http.StatusBadRequest, coded.Code, nil, coded.Message)
		return
	}
	common.Respond(c, http.StatusInternalServerError, common.CodeServerError, nil, err.Error())
}

// AuthMiddleware authenticates system administrators for admin routes.
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		token := c.GetHeader("Authorization")
		if token == "" {
			common.RespondError(c, common.CodeUnauthorized, "Missing authorization header")
			c.Abort()
			return
		}

		// Get user by access token
		identity, code, err := h.userService.ResolveAccessToken(ctx, token)
		if err != nil {
			common.Respond(c, http.StatusUnauthorized, code, nil, "Invalid access token")
			c.Abort()
			return
		}

		user := identity.User
		if user.IsSuperuser == nil || !*user.IsSuperuser {
			common.Respond(c, http.StatusForbidden, common.CodeForbidden, nil, "Permission denied")
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("tenant_id", identity.TenantID)
		c.Set("email", user.Email)
		c.Next()
	}
}

// HandleNoRoute handle undefined routes
func (h *Handler) HandleNoRoute(c *gin.Context) {
	common.Respond(c, http.StatusNotFound, 404, nil, "The requested resource was not found")
}

// GetLogLevel returns the current log level
func (h *Handler) GetLogLevel(c *gin.Context) {
	level := common.GetLogLevel()
	common.RespondSuccess(c, gin.H{"level": level}, "SUCCESS")
}

// SetLogLevelRequest set log level request
type SetLogLevelRequest struct {
	Level string `json:"level" binding:"required"`
}

// SetLogLevel sets the log level at runtime
func (h *Handler) SetLogLevel(c *gin.Context) {
	var req SetLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Level is required: "+err.Error())
		return
	}

	if err := common.SetLogLevel(req.Level); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	common.RespondSuccess(c, gin.H{"level": req.Level}, "SUCCESS")
}

func (h *Handler) ListMessagesFromQueue(c *gin.Context) {

	msgQueueEngine := engine.GetMessageQueueEngine()
	messages, err := msgQueueEngine.ListMessages("ingestion", false)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	var result []map[string]string
	for _, message := range messages {
		var taskMessage common.TaskMessage
		err = json.Unmarshal([]byte(message["message"]), &taskMessage)
		if err != nil {
			return
		}
		result = append(result, map[string]string{
			"subject": message["subject"],
			"id":      taskMessage.TaskID,
			"type":    taskMessage.TaskType,
		})
	}

	common.RespondSuccessList(c, result, "List messages from queue successfully")
}

type PublishMessageToQueueRequest struct {
	Message string `json:"message" binding:"required"`
}

func (h *Handler) PublishMessageToQueue(c *gin.Context) {
	var req PublishMessageToQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Message is required: "+err.Error())
		return
	}

	taskMessage := common.TaskMessage{
		TaskID:   req.Message,
		TaskType: common.TaskTypeIngestionTest,
	}

	// convert task
	taskMessageStr, err := json.Marshal(taskMessage)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	msgQueueEngine := engine.GetMessageQueueEngine()
	err = msgQueueEngine.PublishTask(common.DefaultTierSubject(), taskMessageStr)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "Publish message successfully")
}

type PullMessageFromQueueRequest struct {
	MessageCount int    `json:"message_count" binding:"required,gt=0"`
	AckPolicy    string `json:"ack_policy" binding:"required"`
}

func (h *Handler) PullMessageFromQueue(c *gin.Context) {
	var req PullMessageFromQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, fmt.Sprintf("Message count error: %s", err.Error()))
		return
	}
	if req.MessageCount > common.MaxManualPullMessages {
		common.RespondError(c, common.CodeBadRequest, fmt.Sprintf("message count must be between 1 and %d", common.MaxManualPullMessages))
		return
	}

	msgQueueEngine := engine.GetMessageQueueEngine()
	if err := msgQueueEngine.InitConsumer(common.DefaultTierSubject()); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	pullCtx, cancel := context.WithTimeout(c.Request.Context(), common.HandlerPullTimeout)
	defer cancel()
	messages, err := msgQueueEngine.PullMessages(pullCtx, req.MessageCount)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	var result []map[string]string
	if req.AckPolicy == "ACK" {
		for _, message := range messages {
			taskMessage := message.GetMessage()
			resultMessage := map[string]string{
				"id":   taskMessage.TaskID,
				"type": taskMessage.TaskType,
			}
			err = message.Ack()
			if err == nil {
				resultMessage["ack"] = "true"
			} else {
				resultMessage["ack"] = "false"
			}
			result = append(result, resultMessage)
		}
	} else {
		for _, message := range messages {
			taskMessage := message.GetMessage()
			resultMessage := map[string]string{
				"id":   taskMessage.TaskID,
				"type": taskMessage.TaskType,
			}
			err = message.Nack()
			if err == nil {
				resultMessage["nack"] = "true"
			} else {
				resultMessage["nack"] = "false"
			}
			result = append(result, resultMessage)
		}
	}

	common.RespondSuccessList(c, result, "Pull messages from queue successfully")
}

func (h *Handler) ShowMessageQueue(c *gin.Context) {

	msgQueueEngine := engine.GetMessageQueueEngine()
	result, err := msgQueueEngine.ShowMessageQueue()
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	common.RespondSuccess(c, result, "show message queue successfully")
}

func (h *Handler) ListIngestors(c *gin.Context) {
	serverList := GlobalServerStore.ListInfos()
	serverList = AppendServiceList(serverList)
	var ingestorResults []map[string]string
	now := time.Now()
	for _, ingestorServer := range serverList {
		if ingestorServer.ServerType == common.ServerTypeIngestion {
			ingestorResult := map[string]string{}
			ingestorResult["name"] = ingestorServer.ServerName
			ingestorResult["host"] = ingestorServer.Host
			ingestorResult["status"] = ingestorServer.Version
			if now.Sub(ingestorServer.Timestamp) < 30*time.Second {
				ingestorResult["status"] = "alive"
			} else {
				ingestorResult["status"] = "timeout"
			}
			ingestorResults = append(ingestorResults, ingestorResult)
		}
	}
	common.RespondSuccessList(c, ingestorResults, "Get all tasks")
}

type ShutdownIngestorRequest struct {
	IngestorID string `json:"ingestor_name" binding:"required"`
}

func (h *Handler) ShutdownIngestor(c *gin.Context) {
	var req ShutdownIngestorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Ingestor ID is required: "+err.Error())
		return
	}

	taskID := common.GenerateUUID()
	//ingestionManager.SubmitTask(&common.TaskAssignment{
	//	TaskId:     taskID,
	//	TaskType:   "SHUTDOWN",
	//	AssignedTo: req.IngestorID,
	//})

	common.RespondSuccess(c, gin.H{"task_id": taskID, "ingestor_id": req.IngestorID}, "Shutdown ingestor")
}

// Reports handle heartbeat reports from servers
func (h *Handler) Reports(c *gin.Context) {
	var req common.BaseMessage
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	// Set default timestamp if not provided
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	// Only process heartbeat messages for now
	if req.MessageType != common.MessageHeartbeat {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Unsupported report type: "+string(req.MessageType))
		return
	}

	// Handle the heartbeat
	errCode, data, message := h.service.HandleHeartbeat(&req)
	common.Respond(c, http.StatusOK, errCode, data, message)
}

func (h *Handler) ListAllModels(c *gin.Context) {

	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	// List models
	models, err := h.service.modelProviderService.ListAllModels(page, pageSize)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccessList(c, models, "")
}

func (h *Handler) ShowModel(c *gin.Context) {
	encodedModelName := c.Param("model_name")
	if encodedModelName == "" {
		common.Respond(c, http.StatusBadRequest, 400, nil, "Encoded model name is empty")
		return
	}

	decodedModelName, err := common.DecodeFromBase64(encodedModelName)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	if decodedModelName == "" {
		common.RespondError(c, common.CodeBadRequest, "Decoded model name is empty")
		return
	}

	// Get model
	model, err := h.service.GetModelByModelName(decodedModelName)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, model, "")

}

func (h *Handler) PingStore(c *gin.Context) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		common.RespondError(c, common.CodeServerError, "storage not initialized")
		return
	}

	ctx := c.Request.Context()
	if storageImpl.Health(ctx) {
		common.RespondSuccess(c, nil, "success")
	} else {
		common.RespondError(c, common.CodeServerError, "storage health check failed")
	}
}

func (h *Handler) PingCache(c *gin.Context) {
	ctx := c.Request.Context()
	redisClient := kvrocks.Get()
	if redisClient.Health(ctx) {
		common.RespondSuccess(c, nil, "success")
	} else {
		common.RespondError(c, common.CodeServerError, "cache health check failed")
	}
}

func (h *Handler) PingEngine(c *gin.Context) {
	docEngine := engine.Get()
	ctx := c.Request.Context()
	if err := docEngine.Ping(ctx); err != nil {
		var coded interface {
			Code() common.ErrorCode
		}
		if errors.As(err, &coded) {
			common.Respond(c, http.StatusServiceUnavailable, coded.Code(), nil, err.Error())
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "success")
}

func (h *Handler) GetHardwareInfo(c *gin.Context) {
	hardwareInfo, err := utility.GetHardwareInfo()
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}
	common.RespondSuccess(c, hardwareInfo, "SUCCESS")
}
