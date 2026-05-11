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

package handler

import (
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/utility"
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// UserHandler user handler
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler create user handler
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// Register user registration
// @Summary User Registration
// @Description Create new user
// @Tags users
// @Accept json
// @Produce json
// @Param request body service.RegisterRequest true "registration info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	user, code, err := h.userService.Register(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	variables := server.GetVariables()
	secretKey := variables.SecretKey
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to generate auth token",
			"data":    false,
		})
		return
	}

	c.Header("Authorization", authToken)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(user)
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": fmt.Sprintf("%s, welcome aboard!", req.Nickname),
		"data":    profile,
	})
}

// Login user login
// @Summary User Login
// @Description User login verification
// @Tags users
// @Accept json
// @Produce json
// @Param request body service.LoginRequest true "login info"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	user, code, err := h.userService.Login(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	// Sign the access_token using itsdangerous (compatible with Python)
	variables := server.GetVariables()
	secretKey := variables.SecretKey
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to generate auth token",
			"data":    false,
		})
		return
	}

	// Set Authorization header with signed token
	c.Header("Authorization", authToken)
	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(user)
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "Welcome back!",
		"data":    profile,
	})
}

// LoginByEmail user login by email
// @Summary User Login by Email
// @Description User login verification using email
// @Tags users
// @Accept json
// @Produce json
// @Param request body service.EmailLoginRequest true "login info with email"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/login [post]
func (h *UserHandler) LoginByEmail(c *gin.Context) {
	var req service.EmailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	if !local.IsAdminAvailable() {
		license := local.GetAdminStatus()
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeAuthenticationError,
			"message": license.Reason,
			"data":    "No",
		})
		return
	}

	user, code, err := h.userService.LoginByEmail(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	variables := server.GetVariables()
	secretKey := variables.SecretKey
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to generate auth token",
			"data":    false,
		})
		return
	}

	c.Header("Authorization", authToken)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(user)
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "Welcome back!",
		"data":    profile,
	})
}

// GetUserByID get user by ID
// @Summary Get User Info
// @Description Get user details by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "user ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users/{id} [get]
func (h *UserHandler) GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": "invalid user id",
			"data":    false,
		})
		return
	}

	user, code, err := h.userService.GetUserByID(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    user,
	})
}

// ListUsers user list
// @Summary User List
// @Description Get paginated user list
// @Tags users
// @Accept json
// @Produce json
// @Param page query int false "page number" default(1)
// @Param page_size query int false "items per page" default(10)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	users, total, code, err := h.userService.ListUsers(page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data": gin.H{
			"items":     users,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// Logout user logout
// @Summary User Logout
// @Description Logout user and invalidate access token
// @Tags users
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/logout [post]
func (h *UserHandler) Logout(c *gin.Context) {
	// Same as AuthMiddleware@auth.go
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
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

	// Logout user
	code, err = h.userService.Logout(user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    true,
		"message": "success",
	})
}

// Info get user profile information
// @Summary Get User Profile
// @Description Get current user's profile information
// @Tags users
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/info [get]
func (h *UserHandler) Info(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Get user profile
	profile := h.userService.GetUserProfile(user)

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    profile,
	})
}

// Setting update user settings
// @Summary Update User Settings
// @Description Update current user's settings
// @Tags users
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.UpdateSettingsRequest true "user settings"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/setting [post]
func (h *UserHandler) Setting(c *gin.Context) {
	// Extract token from request
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeUnauthorized,
			"message": "Missing Authorization header",
			"data":    false,
		})
		return
	}

	// Get user by token
	user, code, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	// Parse request
	var req service.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	// Update user settings
	code, err = h.userService.UpdateUserSettings(user, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "settings updated successfully",
		"data":    true,
	})
}

// ChangePassword change user password
// @Summary Change User Password
// @Description Change current user's password
// @Tags users
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.ChangePasswordRequest true "password change info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/setting/password [post]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Parse request
	var req service.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	// Change password
	code, err := h.userService.ChangePassword(user, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "password changed successfully",
		"data":    true,
	})
}

// GetLoginChannels get all supported authentication channels
// @Summary Get Login Channels
// @Description Get all supported OAuth authentication channels
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/login/channels [get]
func (h *UserHandler) GetLoginChannels(c *gin.Context) {
	channels, code, err := h.userService.GetLoginChannels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": "Load channels failure, error: " + err.Error(),
			"data":    []interface{}{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    channels,
	})
}

// SetTenantInfo update tenant information
// @Summary Set Tenant Info
// @Description Update tenant model configuration
// @Tags users
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.SetTenantInfoRequest true "tenant info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/set_tenant_info [post]
func (h *UserHandler) SetTenantInfo(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.SetTenantInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	err := h.userService.SetTenantInfo(user.ID, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    true,
	})
}
