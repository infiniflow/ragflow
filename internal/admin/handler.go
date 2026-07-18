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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/redis"
	"ragflow/internal/handler"
	"ragflow/internal/server"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strconv"
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
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": result,
		})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": common.CodeServerError,
			"data": result,
		})
	}
}

// Live endpoint
func (h *Handler) Live(c *gin.Context) {
	common.SuccessNoData(c, "")
}

// Ping ping endpoint
func (h *Handler) Ping(c *gin.Context) {
	common.SuccessNoData(c, "pong")
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
	var req service.EmailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
		return
	}

	// Use userService.LoginByEmail with adminLogin=true
	// This allows default admin account to log in admin system
	user, code, err := h.userService.LoginByEmail(&req)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	// Check if user is superuser (admin)
	if user.IsSuperuser == nil || !*user.IsSuperuser {
		common.ErrorWithCode(c, common.CodeForbidden, "Only superuser can login admin system")
		return
	}

	secretKey, err := server.GetSecretKey(redis.Get())
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, fmt.Sprintf("Failed to get secret key: %s", err.Error()))
		return
	}

	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, fmt.Sprintf("Failed to generate auth token: %s", err.Error()))
		return
	}

	// Set Authorization header with access_token
	c.Header("Authorization", authToken)
	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	common.SuccessWithData(c, user, "Welcome back!")
}

// Logout handle logout
func (h *Handler) Logout(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		common.ErrorWithCode(c, common.CodeUnauthorized, "Not authenticated")
		return
	}

	if err := h.service.Logout(user); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoData(c, "Logout successful")
}

// AuthCheck check admin auth
func (h *Handler) AuthCheck(c *gin.Context) {
	common.SuccessNoData(c, "Admin is authorized")
}

// ListUsersRequest list users request
type ListUsersRequest struct {
	Enterprise *bool   `json:"enterprise"`
	UserStatus *string `json:"user_status"`
	OrderBy    *string `json:"order_by"`
	Top        *int    `json:"top"`
	Days       *int    `json:"days"`
	Quota      *int    `json:"quota"`
	Plan       *string `json:"plan"`
}

// ListUsers handle list users
func (h *Handler) ListUsers(c *gin.Context) {

	name := c.DefaultQuery("keyword", "")
	status := c.DefaultQuery("status", "")
	role := c.DefaultQuery("role", "")
	sort := c.DefaultQuery("sort", "")     // descending or ascending
	orderBy := c.DefaultQuery("order", "") // order by field
	pageInt, err := common.ParseRequestIntPositive(c, c.Query("page"), "page", 1)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	pageSizeInt, err := common.ParseRequestIntPositive(c, c.Query("page_size"), "page_size", 10)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	plan := c.Query("plan") // plan name
	topInt, err := common.ParseRequestIntPositive(c, c.Query("top"), "top", 0)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	quotaInt, err := common.ParseRequestIntPositive(c, c.Query("quota"), "quota", 0)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if quotaInt > 100 {
		common.ErrorWithCode(c, common.CodeBadRequest, "Quota must be less than or equal to 100")
		return
	}
	daysInt, err := common.ParseRequestIntPositive(c, c.Query("days"), "days", 0)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	var users []map[string]interface{}
	switch common.GetRAGFlowType() {
	case common.OpenSourceVersion:

		users, err = h.service.ListUsers(pageInt, pageSizeInt, name, status, sort, orderBy)
		if err != nil {
			common.ErrorWithCode(c, common.CodeServerError, err.Error())
			return
		}

		common.SuccessWithData(c, users, "List users")
		return
	case common.EnterpriseEdition:
		users, err = h.service.ListUsersEE(pageInt, pageSizeInt, name, status, role, sort, orderBy, plan, topInt, daysInt, quotaInt)
		if err != nil {
			common.ErrorWithCode(c, common.CodeServerError, err.Error())
			return
		}
	default:
		common.ErrorWithCode(c, common.CodeBadRequest, "Invalid RAGFlow type")
		return
	}
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
		common.ErrorWithCode(c, common.CodeBadRequest, "Username and password are required")
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	userInfo, err := h.service.CreateUser(req.Username, req.Password, req.Role)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userInfo, "User created successfully")
}

func getUserName(c *gin.Context) (string, error) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return "", err
	}
	if username == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Username is required")
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

	userDetails, err := h.service.GetUserDetails(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userDetails, "")
}

// DeleteUser handle delete user
func (h *Handler) DeleteUser(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	result, err := h.service.DeleteUser(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	detailsMsg := "Successfully deleted user. Details:\n"
	for _, detail := range result.DeletedDetails {
		detailsMsg += detail + "\n"
	}

	common.SuccessNoData(c, detailsMsg)
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
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "New password is required")
		return
	}

	if err := h.service.ChangePassword(username, req.NewPassword); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoData(c, "Password updated successfully")
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
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Activation status is required")
		return
	}

	if req.ActivateStatus != "on" && req.ActivateStatus != "off" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Activation status must be 'on' or 'off'")
		return
	}

	isActive := req.ActivateStatus == "on"
	if err := h.service.UpdateUserActivateStatus(username, isActive); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoData(c, "Activation status updated")
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
		common.ErrorWithCode(c, common.CodeConflict, "can't grant current user: "+username)
		return
	}

	if err := h.service.GrantAdmin(username); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoData(c, "Admin role granted")
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
		common.ErrorWithCode(c, common.CodeConflict, "can't revoke current user: "+username)
		return
	}

	if err = h.service.RevokeAdmin(username); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoData(c, "Admin role revoked")
}

// ListUserAPITokens handle get user API keys
func (h *Handler) ListUserAPITokens(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	apiKeys, err := h.service.ListUserAPITokens(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, apiKeys, "Get user API keys")
}

// GenerateUserAPIToken handle generate user API key
func (h *Handler) GenerateUserAPIToken(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	apiKey, err := h.service.GenerateUserAPIToken(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, apiKey, "API key generated successfully")
}

// DeleteUserAPIToken handle delete user API key
func (h *Handler) DeleteUserAPIToken(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	key := c.Param("token")
	if username == "" || key == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Username and key are required")
		return
	}

	if err = h.service.DeleteUserAPIToken(username, key); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessNoData(c, "API key deleted successfully")
}

// GetServices handle get all services
func (h *Handler) GetServices(c *gin.Context) {
	services, err := h.service.ListServices()
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeBadRequest, nil, err.Error())
		return
	}

	common.SuccessWithData(c, services, "Get all services")
}

// GetServicesByType handle get services by type
func (h *Handler) GetServicesByType(c *gin.Context) {
	serviceType := c.Param("service_type")
	if serviceType == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Service type is required")
		return
	}

	services, err := h.service.GetServicesByType(serviceType)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, services, "")
}

// GetService handle get service details
func (h *Handler) GetService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Service ID is required")
		return
	}

	// Get all services and find the one with matching ID
	allConfigs := server.GetAllConfigs()

	var targetService map[string]interface{}
	for _, config := range allConfigs {
		if id, ok := config["id"]; ok {
			if strconv.Itoa(id.(int)) == serviceID {
				targetService = config
				break
			}
		}
	}

	if targetService == nil {
		common.ErrorWithCode(c, common.CodeNotFound, "Service not found")
		return
	}

	serviceStatus, err := h.service.GetServiceDetails(targetService)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, serviceStatus, "")
}

// ShutdownService handle shutdown service
func (h *Handler) ShutdownService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Service ID is required")
		return
	}

	result, err := h.service.ShutdownService(serviceID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
}

// StartService handle start service
func (h *Handler) StartService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Service ID is required")
		return
	}

	result, err := h.service.StartService(serviceID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
}

// RestartService handle restart service
func (h *Handler) RestartService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Service ID is required")
		return
	}

	result, err := h.service.RestartService(serviceID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
}

// ListVariables handle list variables
func (h *Handler) ListVariables(c *gin.Context) {
	// Check if request has body content
	if c.Request.ContentLength == 0 || c.Request.ContentLength == -1 {
		// List all variables
		variables, err := h.service.ListAllVariables()
		if err != nil {
			common.ErrorWithCode(c, common.CodeServerError, err.Error())
			return
		}
		common.SuccessWithData(c, variables, "")
		return
	}

	// Get single variable by var_name from request body
	var req struct {
		VarName string `json:"var_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Invalid request body")
		return
	}

	if req.VarName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Var name is required")
		return
	}

	variable, err := h.service.GetVariable(req.VarName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, variable, "")
}

// ShowVariable handle show variable
func (h *Handler) ShowVariable(c *gin.Context) {
	encodedVarName := c.Param("var_name")
	varName, err := common.DecodeFromBase64(encodedVarName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if varName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Var name is required")
		return
	}

	variable, err := h.service.GetVariable(varName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, variable, "")
}

// SetVariableHTTPRequest set variable request
type SetVariableHTTPRequest struct {
	VarName  string `json:"var_name" binding:"required"`
	VarValue string `json:"var_value" binding:"required"`
}

// SetVariable handle set variable
// Python logic: update or create a system setting with the given name and value
func (h *Handler) SetVariable(c *gin.Context) {
	var req SetVariableHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Var name is required")
		return
	}

	if req.VarName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Var name is required")
		return
	}

	if req.VarValue == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Var value is required")
		return
	}

	if err := h.service.SetVariable(req.VarName, req.VarValue); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoData(c, "Set variable successfully")
}

// ListConfigs handle list configs
func (h *Handler) ListConfigs(c *gin.Context) {
	configs, err := h.service.ListAllConfigs()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, configs, "")
}

// ListEnvironments handle list environments
func (h *Handler) ListEnvironments(c *gin.Context) {
	environments, err := h.service.ListEnvironments()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, environments, "")
}

// GetVersion handle get version
func (h *Handler) GetVersion(c *gin.Context) {
	version, versionType := h.service.GetVersion()
	common.SuccessWithData(c, gin.H{"version": version, "version_type": versionType}, "")
}

// GetFingerprint handle get system fingerprint
func (h *Handler) GetFingerprint(c *gin.Context) {
	common.ResponseWithHttpCodeData(c, http.StatusNotImplemented, common.CodeBadRequest, nil, "method not implemented")
	return
}

type SetLicenseHTTPRequest struct {
	License string `json:"license" binding:"required"`
}

// SetLicense to set system license
func (h *Handler) SetLicense(c *gin.Context) {
	common.ResponseWithHttpCodeData(c, http.StatusNotImplemented, common.CodeBadRequest, nil, "method not implemented")
	return
}

type SetLicenseConfigHTTPRequest struct {
	TimeRecordSaveInterval int64 `json:"value1" binding:"required"`
	TimeRecordTaskDuration int64 `json:"value2" binding:"required"`
}

func (h *Handler) UpdateLicenseConfig(c *gin.Context) {
	common.ResponseWithHttpCodeData(c, http.StatusNotImplemented, common.CodeBadRequest, nil, "method not implemented")
	return
}

// ShowLicense to get system license
func (h *Handler) ShowLicense(c *gin.Context) {
	common.ResponseWithHttpCodeData(c, http.StatusNotImplemented, common.CodeBadRequest, nil, "method not implemented")
	return
}

// ListSandboxProviders handle list sandbox providers
func (h *Handler) ListSandboxProviders(c *gin.Context) {
	providers, err := h.service.ListSandboxProviders()
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, providers, "")
}

// GetSandboxProviderSchema handle get sandbox provider schema
func (h *Handler) GetSandboxProviderSchema(c *gin.Context) {
	providerID := c.Param("provider_id")
	if providerID == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Provider ID is required")
		return
	}

	schema, err := h.service.GetSandboxProviderSchema(providerID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, schema, "")
}

// GetSandboxConfig handle get sandbox config
func (h *Handler) GetSandboxConfig(c *gin.Context) {
	config, err := h.service.GetSandboxConfig()
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, config, "")
}

// SetSandboxConfigHTTPRequest set sandbox config request
type SetSandboxConfigHTTPRequest struct {
	ProviderType string                 `json:"provider_type" binding:"required"`
	Config       map[string]interface{} `json:"config"`
	SetActive    bool                   `json:"set_active"`
}

// SetSandboxConfig handle set sandbox config
func (h *Handler) SetSandboxConfig(c *gin.Context) {
	var req SetSandboxConfigHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Request body is required")
		return
	}

	if req.ProviderType == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Provider type is required")
		return
	}

	// Default to true for backward compatibility
	_ = c.Request.Body.Close()
	req.SetActive = true

	result, err := h.service.SetSandboxConfig(req.ProviderType, req.Config, req.SetActive)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Sandbox configuration updated successfully")
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
		common.ErrorWithCode(c, common.CodeBadRequest, "Request body is required")
		return
	}

	if req.ProviderType == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Provider type is required")
		return
	}

	result, err := h.service.TestSandboxConnection(req.ProviderType, req.Config)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid access token")
		return
	}

	common.SuccessWithData(c, result, "")
}

// AuthMiddleware JWT auth middleware
// Validates that the user is authenticated and is a superuser (admin)
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			common.ErrorWithCode(c, common.CodeUnauthorized, "Missing authorization header")
			c.Abort()
			return
		}

		// Get user by access token
		user, code, err := h.userService.GetUserByToken(token)
		if err != nil {
			common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, code, nil, "Invalid access token")
			c.Abort()
			return
		}

		if !*user.IsSuperuser {
			common.ResponseWithHttpCodeData(c, http.StatusForbidden, common.CodeForbidden, nil, "Permission denied")
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Next()
	}
}

// HandleNoRoute handle undefined routes
func (h *Handler) HandleNoRoute(c *gin.Context) {
	common.ResponseWithHttpCodeData(c, http.StatusNotFound, 404, nil, "The requested resource was not found")
}

// GetLogLevel returns the current log level
func (h *Handler) GetLogLevel(c *gin.Context) {
	level := common.GetLevel()
	common.SuccessWithData(c, gin.H{"level": level}, "SUCCESS")
}

// SetLogLevelRequest set log level request
type SetLogLevelRequest struct {
	Level string `json:"level" binding:"required"`
}

// SetLogLevel sets the log level at runtime
func (h *Handler) SetLogLevel(c *gin.Context) {
	var req SetLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Level is required")
		return
	}

	if err := common.SetLevel(req.Level); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, gin.H{"level": req.Level}, "SUCCESS")
}

func (h *Handler) ListMessagesFromQueue(c *gin.Context) {

	msgQueueEngine := engine.GetMessageQueueEngine()
	messages, err := msgQueueEngine.ListMessages("ingestion", false)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
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

	common.SuccessWithData(c, result, "List messages from queue successfully")
}

type PublishMessageToQueueRequest struct {
	Message string `json:"message" binding:"required"`
}

func (h *Handler) PublishMessageToQueue(c *gin.Context) {
	var req PublishMessageToQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Message is required")
		return
	}

	taskMessage := common.TaskMessage{
		TaskID:   req.Message,
		TaskType: common.TaskTypeIngestionTest,
	}

	// convert task
	taskMessageStr, err := json.Marshal(taskMessage)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	msgQueueEngine := engine.GetMessageQueueEngine()
	err = msgQueueEngine.PublishTask("tasks.RAGFLOW", taskMessageStr)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, nil, "Publish message successfully")
}

type PullMessageFromQueueRequest struct {
	MessageCount int    `json:"message_count" binding:"required"`
	AckPolicy    string `json:"ack_policy" binding:"required"`
}

func (h *Handler) PullMessageFromQueue(c *gin.Context) {
	var req PullMessageFromQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, fmt.Sprintf("Message count error: %s", err.Error()))
		return
	}

	msgQueueEngine := engine.GetMessageQueueEngine()
	err := msgQueueEngine.InitConsumer("tasks.RAGFLOW")
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	messages, err := msgQueueEngine.GetMessages(req.MessageCount)
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
			if err == nil {
				resultMessage["nack"] = "true"
			} else {
				resultMessage["nack"] = "false"
			}
			result = append(result, resultMessage)
		}
	}

	common.SuccessWithData(c, result, "Pull messages from queue successfully")
}

func (h *Handler) ShowMessageQueue(c *gin.Context) {

	msgQueueEngine := engine.GetMessageQueueEngine()
	result, err := msgQueueEngine.ShowMessageQueue()
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	common.SuccessWithData(c, result, "show message queue successfully")
}

type RemoveIngestionTaskRequest struct {
	Tasks  []string `json:"tasks"`
	Email  *string  `json:"email"`
	Status *string  `json:"status"`
}

func (h *Handler) RemoveIngestionTasks(c *gin.Context) {
	var req RemoveIngestionTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Task ID is required")
		return
	}

	if req.Email == nil && req.Status == nil {
		tasks, err := h.service.RemoveIngestionTasks(req.Tasks)
		if err != nil {
			common.ErrorWithCode(c, handler.IngestionTaskErrorCode(err), err.Error())
			return
		}

		common.SuccessWithData(c, tasks, "Remove tasks successfully")
	} else {
		tasks, err := h.service.RemoveIngestionTasksByCondition(req.Tasks, req.Email, req.Status)
		if err != nil {
			common.ErrorWithCode(c, handler.IngestionTaskErrorCode(err), err.Error())
			return
		}
		common.SuccessWithData(c, tasks, "Remove tasks successfully")
	}
}

type StopIngestionTaskRequest struct {
	Tasks  []string `json:"tasks"`
	Email  *string  `json:"email"`
	Status *string  `json:"status"`
}

func (h *Handler) StopIngestionTasks(c *gin.Context) {
	var req StopIngestionTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Task ID is required")
		return
	}

	if req.Email == nil && req.Status == nil {
		tasks, err := h.service.StopIngestionTasks(req.Tasks)
		if err != nil {
			common.ErrorWithCode(c, handler.IngestionTaskErrorCode(err), err.Error())
			return
		}
		var result []map[string]string
		for _, task := range tasks {
			result = append(result, map[string]string{
				"task_id": task.ID,
				"status":  task.Status,
			})
		}

		common.SuccessWithData(c, result, "Stop tasks successfully")
	} else {
		tasks, err := h.service.StopIngestionTasksByCondition(req.Tasks, req.Email, req.Status)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
			return
		}
		common.SuccessWithData(c, tasks, "Stop tasks successfully")
	}
}

type ListIngestionTasksRequest struct {
	Email  *string `json:"email"`
	Status *string `json:"status"`
}

// ListIngestionTasks handle list ingestion tasks
func (h *Handler) ListIngestionTasks(c *gin.Context) {
	var err error
	var tasks []map[string]interface{}
	var req ListIngestionTasksRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		tasks, err = h.service.ListIngestionTasks()
	} else {
		tasks, err = h.service.ListIngestionTasksByCondition(req.Email, req.Status)
	}

	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
	}
	common.SuccessWithData(c, tasks, "Get all tasks")
}

func (h *Handler) ListIngestors(c *gin.Context) {
	serverList := GlobalServerStore.ListInfos()
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
	common.SuccessWithData(c, ingestorResults, "Get all tasks")
}

type ShutdownIngestorRequest struct {
	IngestorID string `json:"ingestor_name" binding:"required"`
}

func (h *Handler) ShutdownIngestor(c *gin.Context) {
	var req ShutdownIngestorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Ingestor ID is required")
		return
	}

	taskID := utility.GenerateUUID()
	//ingestionManager.SubmitTask(&common.TaskAssignment{
	//	TaskId:     taskID,
	//	TaskType:   "SHUTDOWN",
	//	AssignedTo: req.IngestorID,
	//})

	common.SuccessWithData(c, gin.H{"task_id": taskID, "ingestor_id": req.IngestorID}, "Shutdown ingestor")
}

// Reports handle heartbeat reports from servers
func (h *Handler) Reports(c *gin.Context) {
	var req common.BaseMessage
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	// Set default timestamp if not provided
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	// Only process heartbeat messages for now
	if req.MessageType != common.MessageHeartbeat {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Unsupported report type: "+string(req.MessageType))
		return
	}

	// Handle the heartbeat
	errCode, message := h.service.HandleHeartbeat(&req)
	if errCode != common.CodeLicenseValid {
		common.ErrorWithCode(c, errCode, message)
		return
	}

	common.ErrorWithCode(c, errCode, message)
}

func (h *Handler) ListAllModels(c *gin.Context) {

	page := 0
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	// List models
	models, err := h.service.ListAllModels(page, pageSize)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, models, "")
}

func (h *Handler) ShowModel(c *gin.Context) {
	encodedModelName := c.Param("model_name")
	if encodedModelName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Encoded model name is empty")
		return
	}

	decodedModelName, err := common.DecodeFromBase64(encodedModelName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if decodedModelName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Decoded model name is empty")
		return
	}

	// Get model
	model, err := h.service.GetModelByModelName(decodedModelName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, model, "")

}

func (h *Handler) PingStore(c *gin.Context) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		common.ErrorWithCode(c, common.CodeServerError, "storage not initialized")
		return
	}

	if storageImpl.Health() {
		common.SuccessNoMessage(c, "SUCCESS")
	} else {
		common.ErrorWithCode(c, common.CodeServerError, "storage health check failed")
	}
}

func (h *Handler) PingCache(c *gin.Context) {
	redisClient := redis.Get()
	if redisClient.Health() {
		common.SuccessNoMessage(c, "SUCCESS")
	} else {
		common.ErrorWithCode(c, common.CodeServerError, "cache health check failed")
	}
}

func (h *Handler) PingEngine(c *gin.Context) {

	docEngine := engine.Get()
	ctx := context.Background()
	if err := docEngine.Ping(ctx); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessNoMessage(c, "SUCCESS")
}
