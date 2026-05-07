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
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/cache"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/server"
	"ragflow/internal/service"
	"ragflow/internal/utility"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Common errors
var (
	ErrUserNotFound = errors.New("user not found")
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

// SuccessResponse success response
type SuccessResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// ErrorResponse error response
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// success returns success response
func success(c *gin.Context, data interface{}, message string) {
	c.JSON(200, SuccessResponse{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// successNoData returns success response without data
func successNoData(c *gin.Context, message string) {
	c.JSON(200, SuccessResponse{
		Code:    0,
		Message: message,
		Data:    nil,
	})
}

// error returns error response
func errorResponse(c *gin.Context, message string, code int) {
	c.JSON(code, ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func responseWithCode(c *gin.Context, message string, httpCode int, errorCode common.ErrorCode) {
	if message == "" {
		c.JSON(httpCode, ErrorResponse{
			Code:    int(errorCode),
			Message: errorCode.Message(),
		})
	} else {
		c.JSON(httpCode, ErrorResponse{
			Code:    int(errorCode),
			Message: message,
		})
	}
}

// Health check
func (h *Handler) Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

// Ping ping endpoint
func (h *Handler) Ping(c *gin.Context) {
	successNoData(c, "pong")
}

// Login handle admin login
// @Summary Admin Login
// @Description Admin login verification using email, only superuser can login
// @Tags admin
// @Accept json
// @Produce json
// @Param request body service.EmailLoginRequest true "login info with email"
// @Success 200 {object} map[string]interface{}
// @Router /admin/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req service.EmailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	// Use userService.LoginByEmail with adminLogin=true
	// This allows default admin account to log in admin system
	user, code, err := h.userService.LoginByEmail(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
		})
		return
	}

	// Check if user is superuser (admin)
	if user.IsSuperuser == nil || !*user.IsSuperuser {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeForbidden,
			"message": "Only superuser can login admin system",
		})
		return
	}

	secretKey, err := server.GetSecretKey(cache.Get())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": fmt.Sprintf("Failed to get secret key: %s", err.Error()),
		})
		return
	}

	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": fmt.Sprintf("Failed to generate auth token: %s", err.Error()),
		})
		return
	}

	// Set Authorization header with access_token
	c.Header("Authorization", authToken)
	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "Welcome back!",
		"data":    user,
	})
}

// Logout handle logout
func (h *Handler) Logout(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		errorResponse(c, "Not authenticated", 401)
		return
	}

	if err := h.service.Logout(user); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "Logout successful")
}

// AuthCheck check admin auth
func (h *Handler) AuthCheck(c *gin.Context) {
	successNoData(c, "Admin is authorized")
}

// ListUsers handle list users
func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.service.ListUsers()
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, users, "Get all users")
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
		errorResponse(c, "Username and password are required", 400)
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	userInfo, err := h.service.CreateUser(req.Username, req.Password, req.Role)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userInfo, "User created successfully")
}

// GetUser handle get user
func (h *Handler) GetUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	userDetails, err := h.service.GetUserDetails(username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userDetails, "")
}

// DeleteUser handle delete user
func (h *Handler) DeleteUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	result, err := h.service.DeleteUser(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	detailsMsg := "Successfully deleted user. Details:\n"
	for _, detail := range result.DeletedDetails {
		detailsMsg += detail + "\n"
	}

	successNoData(c, detailsMsg)
}

// ChangePasswordHTTPRequest change password request
type ChangePasswordHTTPRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword handle change password
func (h *Handler) ChangePassword(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	var req ChangePasswordHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "New password is required", 400)
		return
	}

	if err := h.service.ChangePassword(username, req.NewPassword); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "Password updated successfully")
}

// UpdateActivateStatusHTTPRequest update activate status request
type UpdateActivateStatusHTTPRequest struct {
	ActivateStatus string `json:"activate_status" binding:"required"`
}

// UpdateUserActivateStatus handle update user activate status
func (h *Handler) UpdateUserActivateStatus(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	var req UpdateActivateStatusHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Activation status is required", 400)
		return
	}

	if req.ActivateStatus != "on" && req.ActivateStatus != "off" {
		errorResponse(c, "Activation status must be 'on' or 'off'", 400)
		return
	}

	isActive := req.ActivateStatus == "on"
	if err := h.service.UpdateUserActivateStatus(username, isActive); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "Activation status updated")
}

// GrantAdmin handle grant admin role
func (h *Handler) GrantAdmin(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	// Get current user email from context
	email, _ := c.Get("email")
	if email != nil && email.(string) == username {
		errorResponse(c, "can't grant current user: "+username, 409)
		return
	}

	if err := h.service.GrantAdmin(username); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "Admin role granted")
}

// RevokeAdmin handle revoke admin role
func (h *Handler) RevokeAdmin(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	// Get current user email from context
	email, _ := c.Get("email")
	if email != nil && email.(string) == username {
		errorResponse(c, "can't revoke current user: "+username, 409)
		return
	}

	if err := h.service.RevokeAdmin(username); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "Admin role revoked")
}

// GetUserDatasets handle get user datasets
func (h *Handler) GetUserDatasets(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	datasets, err := h.service.GetUserDatasets(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, datasets, "")
}

// GetUserAgents handle get user agents
func (h *Handler) GetUserAgents(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	agents, err := h.service.GetUserAgents(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, agents, "")
}

// ListUserAPITokens handle get user API keys
func (h *Handler) ListUserAPITokens(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	apiKeys, err := h.service.ListUserAPITokens(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, apiKeys, "Get user API keys")
}

// GenerateUserAPIToken handle generate user API key
func (h *Handler) GenerateUserAPIToken(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	apiKey, err := h.service.GenerateUserAPIToken(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, apiKey, "API key generated successfully")
}

// DeleteUserAPIToken handle delete user API key
func (h *Handler) DeleteUserAPIToken(c *gin.Context) {
	username := c.Param("username")
	key := c.Param("token")
	if username == "" || key == "" {
		errorResponse(c, "Username and key are required", 400)
		return
	}

	if err := h.service.DeleteUserAPIToken(username, key); err != nil {
		errorResponse(c, err.Error(), 404)
		return
	}

	successNoData(c, "API key deleted successfully")
}

// ListRoles handle list roles
func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.service.ListRoles()
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	if roles == nil {
		roles = []map[string]interface{}{}
	}

	success(c, gin.H{
		"roles": roles,
		"total": len(roles),
	}, "")
}

// CreateRoleHTTPRequest create role request
type CreateRoleHTTPRequest struct {
	RoleName    string `json:"role_name" binding:"required"`
	Description string `json:"description"`
}

// CreateRole handle create role
func (h *Handler) CreateRole(c *gin.Context) {
	var req CreateRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Role name is required", 400)
		return
	}

	role, err := h.service.CreateRole(req.RoleName, req.Description)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, role, "")
}

// GetRole handle get role
func (h *Handler) GetRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	role, err := h.service.GetRole(roleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, role, "")
}

// UpdateRoleHTTPRequest update role request
type UpdateRoleHTTPRequest struct {
	Description string `json:"description" binding:"required"`
}

// UpdateRole handle update role
func (h *Handler) UpdateRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var req UpdateRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Role description is required", 400)
		return
	}

	role, err := h.service.UpdateRole(roleName, req.Description)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, role, "")
}

// DeleteRole handle delete role
func (h *Handler) DeleteRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	if err := h.service.DeleteRole(roleName); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "")
}

// GetRolePermission handle get role permission
func (h *Handler) GetRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	permissions, err := h.service.GetRolePermission(roleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, permissions, "")
}

// GrantRolePermissionHTTPRequest grant role permission request
type GrantRolePermissionHTTPRequest struct {
	Actions  []string `json:"actions" binding:"required"`
	Resource string   `json:"resource" binding:"required"`
}

// GrantRolePermission handle grant role permission
func (h *Handler) GrantRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var req GrantRolePermissionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Permission is required", 400)
		return
	}

	result, err := h.service.GrantRolePermission(roleName, req.Actions, req.Resource)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

// RevokeRolePermissionHTTPRequest revoke role permission request
type RevokeRolePermissionHTTPRequest struct {
	Actions  []string `json:"actions" binding:"required"`
	Resource string   `json:"resource" binding:"required"`
}

// RevokeRolePermission handle revoke role permission
func (h *Handler) RevokeRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var req RevokeRolePermissionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Permission is required", 400)
		return
	}

	result, err := h.service.RevokeRolePermission(roleName, req.Actions, req.Resource)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

// UpdateUserRoleHTTPRequest update user role request
type UpdateUserRoleHTTPRequest struct {
	RoleName string `json:"role_name" binding:"required"`
}

// UpdateUserRole handle update user role
func (h *Handler) UpdateUserRole(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	var req UpdateUserRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Role name is required", 400)
		return
	}

	result, err := h.service.UpdateUserRole(username, req.RoleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

// GetUserPermission handle get user permission
func (h *Handler) GetUserPermission(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	permissions, err := h.service.GetUserPermission(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, permissions, "")
}

// GetServices handle get all services
func (h *Handler) GetServices(c *gin.Context) {
	services, err := h.service.ListServices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    common.CodeServerError,
			"message": err.Error(),
		})
		return
	}

	success(c, services, "Get all services")
}

// GetServicesByType handle get services by type
func (h *Handler) GetServicesByType(c *gin.Context) {
	serviceType := c.Param("service_type")
	if serviceType == "" {
		errorResponse(c, "Service type is required", 400)
		return
	}

	services, err := h.service.GetServicesByType(serviceType)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, services, "")
}

// GetService handle get service details
func (h *Handler) GetService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		errorResponse(c, "Service ID is required", 400)
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
		errorResponse(c, "Service not found", 404)
		return
	}

	serviceStatus, err := h.service.GetServiceDetails(targetService)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, serviceStatus, "")
}

// ShutdownService handle shutdown service
func (h *Handler) ShutdownService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		errorResponse(c, "Service ID is required", 400)
		return
	}

	result, err := h.service.ShutdownService(serviceID)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

// RestartService handle restart service
func (h *Handler) RestartService(c *gin.Context) {
	serviceID := c.Param("service_id")
	if serviceID == "" {
		errorResponse(c, "Service ID is required", 400)
		return
	}

	result, err := h.service.RestartService(serviceID)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

func (h *Handler) ListProviders(c *gin.Context) {

	keywords := ""
	if queryKeywords := c.Query("available"); queryKeywords != "" {
		keywords = queryKeywords
	}

	// convert keywords to small case
	keywords = strings.ToLower(keywords)
	if keywords == "true" {
		// list pool providers
		providers, err := dao.GetModelProviderManager().ListProviders()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeNotFound,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    providers,
		})
	}
}

func (h *Handler) ShowProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    provider,
	})
}

func (h *Handler) ListModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	models, err := dao.GetModelProviderManager().ListModels(providerName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    models,
	})
}

func (h *Handler) ShowModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	modelName := c.Param("model_name")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}
	model, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    model,
	})
}

// GetVariables handle get variables
// Python logic: if request body is empty, list all variables; otherwise get single variable by var_name from body
func (h *Handler) GetVariables(c *gin.Context) {
	// Check if request has body content
	if c.Request.ContentLength == 0 || c.Request.ContentLength == -1 {
		// List all variables
		variables, err := h.service.GetAllVariables()
		if err != nil {
			errorResponse(c, err.Error(), 500)
			return
		}
		success(c, variables, "")
		return
	}

	// Get single variable by var_name from request body
	var req struct {
		VarName string `json:"var_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Invalid request body", 400)
		return
	}

	if req.VarName == "" {
		errorResponse(c, "Var name is required", 400)
		return
	}

	variable, err := h.service.GetVariable(req.VarName)
	if err != nil {
		// Check if it's an AdminException
		if adminErr, ok := err.(*AdminException); ok {
			errorResponse(c, adminErr.Message, 400)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, variable, "")
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
		errorResponse(c, "Var name is required", 400)
		return
	}

	if req.VarName == "" {
		errorResponse(c, "Var name is required", 400)
		return
	}

	if req.VarValue == "" {
		errorResponse(c, "Var value is required", 400)
		return
	}

	if err := h.service.SetVariable(req.VarName, req.VarValue); err != nil {
		// Check if it's an AdminException
		if adminErr, ok := err.(*AdminException); ok {
			errorResponse(c, adminErr.Message, 400)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "Set variable successfully")
}

// GetConfigs handle get configs
// Python logic: return all service configurations
func (h *Handler) GetConfigs(c *gin.Context) {
	configs, err := h.service.GetAllConfigs()
	if err != nil {
		// Check if it's an AdminException
		if adminErr, ok := err.(*AdminException); ok {
			errorResponse(c, adminErr.Message, 400)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, configs, "")
}

// GetEnvironments handle get environments
// Python logic: return important environment variables
func (h *Handler) GetEnvironments(c *gin.Context) {
	environments, err := h.service.GetAllEnvironments()
	if err != nil {
		// Check if it's an AdminException
		if adminErr, ok := err.(*AdminException); ok {
			errorResponse(c, adminErr.Message, 400)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, environments, "")
}

// GetVersion handle get version
func (h *Handler) GetVersion(c *gin.Context) {
	version := h.service.GetVersion()
	success(c, gin.H{"version": version}, "")
}

// GetFingerprint handle get system fingerprint
func (h *Handler) GetFingerprint(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    common.CodeServerError,
		"message": "method not implemented",
	})
	return
}

type SetLicenseHTTPRequest struct {
	License string `json:"license" binding:"required"`
}

// SetLicense to set system license
func (h *Handler) SetLicense(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    common.CodeServerError,
		"message": "method not implemented",
	})
	return
}

type SetLicenseConfigHTTPRequest struct {
	TimeRecordSaveInterval int64 `json:"value1" binding:"required"`
	TimeRecordTaskDuration int64 `json:"value2" binding:"required"`
}

func (h *Handler) UpdateLicenseConfig(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    common.CodeServerError,
		"message": "method not implemented",
	})
	return
}

// ShowLicense to get system license
func (h *Handler) ShowLicense(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    common.CodeServerError,
		"message": "method not implemented",
	})
	return
}

// ListSandboxProviders handle list sandbox providers
func (h *Handler) ListSandboxProviders(c *gin.Context) {
	providers, err := h.service.ListSandboxProviders()
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}

	success(c, providers, "")
}

// GetSandboxProviderSchema handle get sandbox provider schema
func (h *Handler) GetSandboxProviderSchema(c *gin.Context) {
	providerID := c.Param("provider_id")
	if providerID == "" {
		errorResponse(c, "Provider ID is required", 400)
		return
	}

	schema, err := h.service.GetSandboxProviderSchema(providerID)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}

	success(c, schema, "")
}

// GetSandboxConfig handle get sandbox config
func (h *Handler) GetSandboxConfig(c *gin.Context) {
	config, err := h.service.GetSandboxConfig()
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}

	success(c, config, "")
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
		errorResponse(c, "Request body is required", 400)
		return
	}

	if req.ProviderType == "" {
		errorResponse(c, "provider_type is required", 400)
		return
	}

	// Default to true for backward compatibility
	_ = c.Request.Body.Close()
	req.SetActive = true

	result, err := h.service.SetSandboxConfig(req.ProviderType, req.Config, req.SetActive)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}

	success(c, result, "Sandbox configuration updated successfully")
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
		errorResponse(c, "Request body is required", 400)
		return
	}

	if req.ProviderType == "" {
		errorResponse(c, "provider_type is required", 400)
		return
	}

	result, err := h.service.TestSandboxConnection(req.ProviderType, req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"message": "Invalid access token",
		})
		return
	}

	success(c, result, "")
}

// AuthMiddleware JWT auth middleware
// Validates that the user is authenticated and is a superuser (admin)
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			errorResponse(c, "missing authorization header", 401)
			c.Abort()
			return
		}

		// Get user by access token
		user, code, err := h.userService.GetUserByToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    code,
				"message": "Invalid access token",
			})
			c.Abort()
			return
		}

		if !*user.IsSuperuser {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    common.CodeForbidden,
				"message": "Permission denied",
			})
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
	c.JSON(http.StatusNotFound, ErrorResponse{
		Code:    404,
		Message: "The requested resource was not found",
	})
}

// GetLogLevel returns the current log level
func (h *Handler) GetLogLevel(c *gin.Context) {
	level := common.GetLevel()
	success(c, gin.H{"level": level}, "")
}

// SetLogLevelRequest set log level request
type SetLogLevelRequest struct {
	Level string `json:"level" binding:"required"`
}

// SetLogLevel sets the log level at runtime
func (h *Handler) SetLogLevel(c *gin.Context) {
	var req SetLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "level is required", 400)
		return
	}

	if err := common.SetLevel(req.Level); err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}

	success(c, gin.H{"level": req.Level}, "Log level updated successfully")
}

// Reports handle heartbeat reports from servers
func (h *Handler) Reports(c *gin.Context) {
	var req common.BaseMessage
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Set default timestamp if not provided
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	// Only process heartbeat messages for now
	if req.MessageType != common.MessageHeartbeat {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"message": "Unsupported report type: " + string(req.MessageType),
		})
		return
	}

	// Handle the heartbeat
	errCode, message := h.service.HandleHeartbeat(&req)
	if errCode != common.CodeLicenseValid {
		responseWithCode(c, message, 500, errCode)
		return
	}

	responseWithCode(c, message, http.StatusOK, errCode)
}
