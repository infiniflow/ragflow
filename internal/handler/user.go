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
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"strconv"
	"time"

	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
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

// oauthAuthCookie is the cookie the callback writes on success, so the SPA
// can pick up the signed access token after the redirect. The frontend
// reads it and either re-issues the value as an Authorization header on
// subsequent API calls or hands it off to its own token store. Not
// HttpOnly so the SPA's JS can read it.
const oauthAuthCookie = "ragflow_auth"

// setOAuthAuthCookie writes the signed access token so the SPA can pick it
// up after the redirect. Not HttpOnly so the SPA can copy it into its
// Authorization header on subsequent fetches. The server checks the token
// signature and expiry on each request.
func setOAuthAuthCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthAuthCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 7,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

// Register user registration
// @Summary User Registration
// @Description Create new user
// @Tags users
// @Param request body service.RegisterRequest true "registration info"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users [post]
func (h *UserHandler) Register(c *gin.Context) {
	ctx := c.Request.Context()
	startAt := time.Now()
	operationLog := &common.OperationLog{
		EventTime:  startAt,
		Operation:  "register",
		APIPath:    c.FullPath(),
		HTTPMethod: c.Request.Method,
		IPAddress:  c.ClientIP(),
	}
	defer func() {
		operationLog.DurationMS = time.Since(startAt).Milliseconds()
		common.Audit(operationLog)
	}()

	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	user, code, err := h.userService.Register(ctx, &req)
	if err != nil {
		var data interface{} = false
		if code == common.CodeExceptionError {
			data = nil
		}
		common.ResponseWithCodeData(c, code, data, err.Error())
		return
	}
	operationLog.UserID = user.ID
	operationLog.UserName = user.Email

	secretKey, err := server.GetSecretKey(ctx, kvrocks.Get())
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, err.Error())
		operationLog.Code = int(common.CodeBadRequest)
		operationLog.Error = err
		return
	}
	authToken, err := issueUserAccessToken(user, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
		operationLog.Code = int(common.CodeBadRequest)
		operationLog.Error = err
		return
	}

	c.Header("Authorization", authToken)
	setOAuthAuthCookie(c, authToken)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(ctx, user)
	common.SuccessWithData(c, profile, fmt.Sprintf("%s, welcome aboard!", req.Nickname))
}

// Login user login
// @Summary User Login
// @Description User login verification
// @Tags users
// @Param request body service.LoginRequest true "login info"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	startAt := time.Now()
	operationLog := &common.OperationLog{
		EventTime:  startAt,
		Operation:  "login",
		APIPath:    c.FullPath(),
		HTTPMethod: c.Request.Method,
		IPAddress:  c.ClientIP(),
	}
	defer func() {
		operationLog.DurationMS = time.Since(startAt).Milliseconds()
		common.Audit(operationLog)
	}()

	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		operationLog.Code = int(common.CodeBadRequest)
		operationLog.Error = err
		return
	}
	operationLog.UserName = req.Username

	user, code, err := h.userService.Login(ctx, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		operationLog.Code = int(code)
		operationLog.Error = err
		return
	}
	operationLog.UserID = user.ID

	// Sign the access token with the active tenant context.
	secretKey, err := server.GetSecretKey(ctx, kvrocks.Get())
	if err != nil {
		errMessage := fmt.Errorf("failed to get secret key: %s", err.Error())
		common.ResponseWithCodeData(c, common.CodeServerError, false, errMessage.Error())
		operationLog.Code = int(common.CodeServerError)
		operationLog.Error = errMessage
		return
	}
	authToken, err := issueUserAccessToken(user, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
		operationLog.Code = int(common.CodeServerError)
		operationLog.Error = err
		return
	}

	// Set Authorization header with signed token
	c.Header("Authorization", authToken)
	setOAuthAuthCookie(c, authToken)
	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(ctx, user)
	common.SuccessWithData(c, profile, "Welcome back!")
}

// LoginByEmail user login by email
// @Summary User Login by Email
// @Description User login verification using email
// @Tags users
// @Param request body service.EmailLoginRequest true "login info with email"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/login [post]
func (h *UserHandler) LoginByEmail(c *gin.Context) {
	ctx := c.Request.Context()
	startAt := time.Now()
	operationLog := &common.OperationLog{
		EventTime:  startAt,
		Operation:  "login",
		APIPath:    c.FullPath(),
		HTTPMethod: c.Request.Method,
		IPAddress:  c.ClientIP(),
	}
	defer func() {
		operationLog.DurationMS = time.Since(startAt).Milliseconds()
		common.Audit(operationLog)
	}()

	var req service.EmailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		operationLog.Code = int(common.CodeBadRequest)
		operationLog.Error = err
		return
	}
	operationLog.UserName = req.Email

	if !local.IsAdminAvailable() {
		adminStatus := local.GetAdminStatus()
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, "No", adminStatus.Reason)
		operationLog.Code = int(common.CodeAuthenticationError)
		operationLog.Error = errors.New(adminStatus.Reason)
		return
	}

	user, code, err := h.userService.LoginByEmail(ctx, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		operationLog.Code = int(code)
		operationLog.Error = err
		return
	}
	operationLog.UserID = user.ID

	if user.IsSuperuser != nil && *user.IsSuperuser {
		common.ResponseWithCodeData(c, common.CodeForbidden, false, "super admin should not login to this page.")
		operationLog.Code = int(common.CodeForbidden)
		operationLog.Error = fmt.Errorf("super admin should not login to this page")
		return
	}

	secretKey, err := server.GetSecretKey(ctx, kvrocks.Get())
	if err != nil {
		errorMessage := fmt.Errorf("failed to get secret key: %s", err.Error())
		common.ResponseWithCodeData(c, common.CodeServerError, false, errorMessage.Error())
		operationLog.Code = int(common.CodeServerError)
		operationLog.Error = errorMessage
		return
	}
	authToken, err := issueUserAccessToken(user, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
		operationLog.Code = int(common.CodeServerError)
		operationLog.Error = err
		return
	}
	setOAuthAuthCookie(c, authToken)
	c.Header("Authorization", authToken)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(ctx, user)
	common.SuccessWithData(c, profile, "Welcome back!")
}

// GetUserByID get user by ID
// @Summary Get User Info
// @Description Get user details by ID
// @Tags users
// @Param id path int true "user ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users/{id} [get]
func (h *UserHandler) GetUserByID(c *gin.Context) {
	ctx := c.Request.Context()
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, "invalid user id")
		return
	}

	user, code, err := h.userService.GetUserByID(ctx, uint(id))
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, user, "success")
}

// Logout user logout
// @Summary User Logout
// @Description Logout user and invalidate access token
// @Tags users
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/logout [post]
func (h *UserHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()
	startAt := time.Now()
	operationLog := &common.OperationLog{
		EventTime:  startAt,
		Operation:  "logout",
		APIPath:    c.FullPath(),
		HTTPMethod: c.Request.Method,
		IPAddress:  c.ClientIP(),
	}
	defer func() {
		operationLog.DurationMS = time.Since(startAt).Milliseconds()
		common.Audit(operationLog)
	}()

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthAuthCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})

	// Same as AuthMiddleware@auth.go
	token := c.GetHeader("Authorization")
	if token == "" {
		common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, common.CodeUnauthorized, nil, "missing authorization header")
		c.Abort()
		operationLog.Code = int(common.CodeUnauthorized)
		operationLog.Error = fmt.Errorf("missing authorization header")
		return
	}

	// Get user by access token
	identity, code, err := h.userService.ResolveAccessToken(ctx, token)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, code, nil, "Invalid access token")
		c.Abort()
		operationLog.Code = int(code)
		operationLog.Error = err
		return
	}
	user := identity.User
	operationLog.UserID = user.ID
	operationLog.UserName = user.Email

	// Logout user
	code, err = h.userService.Logout(ctx, user)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		operationLog.Code = int(code)
		operationLog.Error = err
		return
	}

	common.SuccessWithData(c, true, "success")
}

// Info get user profile information
// @Summary Get User Profile
// @Description Get current user's profile information
// @Tags users
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/info [get]
func (h *UserHandler) Info(c *gin.Context) {
	ctx := c.Request.Context()
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	// Get user profile
	profile := h.userService.GetUserProfile(ctx, user)

	common.SuccessWithData(c, profile, "success")
}

// Setting update user settings
// @Summary Update User Settings
// @Description Update current user's settings
// @Tags users
// @Security ApiKeyAuth
// @Param request body service.UpdateSettingsRequest true "user settings"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users/me [patch]
func (h *UserHandler) Setting(c *gin.Context) {
	ctx := c.Request.Context()
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	// Parse request
	var req service.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	// Update user settings
	code, err := h.userService.UpdateUserSettings(ctx, user, &req)
	if err != nil {
		if code == common.CodeExceptionError {
			common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
			return
		}
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, true, "success")
}

// ChangePassword change user password
// @Summary Change User Password
// @Description Change current user's password
// @Tags users
// @Security ApiKeyAuth
// @Param request body service.ChangePasswordRequest true "password change info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/setting/password [post]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	ctx := c.Request.Context()
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	// Parse request
	var req service.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	// Change password
	code, err := h.userService.ChangePassword(ctx, user, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, true, "password changed successfully")
}

// SetTenantInfo update tenant information
// @Summary Set Tenant Info
// @Description Update tenant model configuration
// @Tags users
// @Security ApiKeyAuth
// @Param request body service.SetTenantInfoRequest true "tenant info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/set_tenant_info [post]
func (h *UserHandler) SetTenantInfo(c *gin.Context) {
	ctx := c.Request.Context()
	startAt := time.Now()
	operationLog := &common.OperationLog{
		EventTime:  startAt,
		Operation:  "set_tenant_info",
		APIPath:    c.FullPath(),
		HTTPMethod: c.Request.Method,
		IPAddress:  c.ClientIP(),
	}
	defer func() {
		operationLog.DurationMS = time.Since(startAt).Milliseconds()
		common.Audit(operationLog)
	}()

	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		operationLog.Code = int(errorCode)
		operationLog.Error = errors.New(errorMessage)
		return
	}
	operationLog.UserID = user.ID
	operationLog.UserName = user.Email

	requiredKeys := []string{"tenant_id", "asr_id", "embd_id", "img2txt_id", "llm_id"}
	missingArgumentMessage := "required argument are missing: tenant_id,asr_id,embd_id,img2txt_id,llm_id; "

	var payload map[string]interface{}
	if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, missingArgumentMessage)
		operationLog.Code = int(common.CodeArgumentError)
		operationLog.Error = err
		return
	}

	missing := make([]string, 0, len(requiredKeys))
	for _, key := range requiredKeys {
		if _, ok := payload[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("required argument are missing: %s; ", joinStrings(missing)))
		operationLog.Code = int(common.CodeArgumentError)
		operationLog.Error = errors.New(fmt.Sprintf("required argument are missing: %s; ", joinStrings(missing)))
		return
	}

	req := service.SetTenantInfoRequest{Raw: payload}
	if value, ok := payload["tenant_id"].(string); ok {
		req.TenantID = &value
	}
	if value, ok := payload["asr_id"].(string); ok {
		req.ASRID = &value
	}
	if value, ok := payload["embd_id"].(string); ok {
		req.EmbdID = &value
	}
	if value, ok := payload["img2txt_id"].(string); ok {
		req.Img2TxtID = &value
	}
	if value, ok := payload["llm_id"].(string); ok {
		req.LLMID = &value
	}
	if value, ok := payload["rerank_id"].(string); ok {
		req.RerankID = &value
	}
	if value, ok := payload["tts_id"].(string); ok {
		req.TTSID = &value
	}

	code, err := h.userService.SetTenantInfo(ctx, user.ID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, err.Error())
		operationLog.Code = int(code)
		operationLog.Error = err
		return
	}

	common.SuccessWithData(c, true, "success")
}

func joinStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for i := 1; i < len(values); i++ {
		result += "," + values[i]
	}
	return result
}
