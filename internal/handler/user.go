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
	"ragflow/internal/engine/redis"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/utility"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

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
// @Router /api/v1/users [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	user, code, err := h.userService.Register(&req)
	if err != nil {
		var data interface{} = false
		if code == common.CodeExceptionError {
			data = nil
		}
		common.ResponseWithCodeData(c, code, data, err.Error())
		return
	}

	secretKey, err := server.GetSecretKey(redis.Get())
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, err.Error())
		return
	}
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
		return
	}

	c.Header("Authorization", authToken)
	setOAuthAuthCookie(c, authToken)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(user)
	common.SuccessWithData(c, profile, fmt.Sprintf("%s, welcome aboard!", req.Nickname))
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
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	user, code, err := h.userService.Login(&req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	// Sign the access_token using itsdangerous (compatible with Python)
	secretKey, err := server.GetSecretKey(redis.Get())
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, fmt.Sprintf("Failed to get secret key: %s", err.Error()))
		return
	}
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
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

	profile := h.userService.GetUserProfile(user)
	common.SuccessWithData(c, profile, "Welcome back!")
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
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	if !local.IsAdminAvailable() {
		license := local.GetAdminStatus()
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, "No", license.Reason)
		return
	}

	user, code, err := h.userService.LoginByEmail(&req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	secretKey, err := server.GetSecretKey(redis.Get())
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, fmt.Sprintf("Failed to get secret key: %s", err.Error()))
		return
	}
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
		return
	}
	setOAuthAuthCookie(c, authToken)
	c.Header("Authorization", authToken)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	profile := h.userService.GetUserProfile(user)
	common.SuccessWithData(c, profile, "Welcome back!")
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
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, "invalid user id")
		return
	}

	user, code, err := h.userService.GetUserByID(uint(id))
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, user, "success")
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
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, gin.H{
		"items":     users,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "success")
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
		common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, 401, nil, "Missing Authorization header")
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

	// Logout user
	code, err = h.userService.Logout(user)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, true, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Get user profile
	profile := h.userService.GetUserProfile(user)

	common.SuccessWithData(c, profile, "success")
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
// @Router /api/v1/users/me [patch]
func (h *UserHandler) Setting(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Parse request
	var req service.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	// Update user settings
	code, err := h.userService.UpdateUserSettings(user, &req)
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
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.ChangePasswordRequest true "password change info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/setting/password [post]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Parse request
	var req service.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	// Change password
	code, err := h.userService.ChangePassword(user, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, true, "password changed successfully")
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
		common.ResponseWithCodeData(c, code, []interface{}{}, "Load channels failure, error: "+err.Error())
		return
	}

	common.SuccessWithData(c, channels, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	requiredKeys := []string{"tenant_id", "asr_id", "embd_id", "img2txt_id", "llm_id"}
	missingArgumentMessage := "required argument are missing: tenant_id,asr_id,embd_id,img2txt_id,llm_id; "

	var payload map[string]interface{}
	if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, missingArgumentMessage)
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

	code, err := h.userService.SetTenantInfo(user.ID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, err.Error())
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

// ---- Forgot-password flow (fixes #15282) -----------------------------
//
// Mirrors api/apps/restful_apis/user_api.py /auth/password/... endpoints.
//
// Contract divergence from Python: the Python endpoint returns a
// rendered image (Content-Type: image/JPEG) from the python-captcha
// library and stores the captcha under captcha:<email>. This Go port
// returns a server-issued captcha_id plus a PNG captcha image (as a
// data URL the FE drops straight into <img src>), and stores
// captcha:<captcha_id>. The plaintext text only ever appears as
// raster pixels — the OTP step reuses the captcha_id to look the
// expected text up server-side.
//
// The PNG is rendered using stdlib `image/png` + a hand-rolled 5x7
// bitmap font in internal/utility/captcha_png.go, because no Go
// captcha library is vendored in go.mod (no network during build).
// PR #15290 review (Hz-186) explicitly asked for a raster after the
// earlier SVG implementation: the SVG embedded the answer in <text>
// nodes, so a scripted client could base64-decode the response and
// grep the captcha directly. PNG closes that attack — the response
// bytes never reference the original text.

type forgotCaptchaRequest struct {
	Email string `form:"email" json:"email"`
}

// ForgotCaptcha POST /api/v1/auth/password/forgot/captcha
// @Summary Issue forgot-password captcha
// @Description Generates a captcha for the email and stores it in Redis
// for 60 seconds keyed by a server-issued captcha_id. Returns the id
// and a PNG image (data URL) the FE renders inside <img src>. The
// plaintext code never appears in the response — only as raster
// pixels — so a scripted client can't regex it out (fixes the
// SVG-text leak from the previous iteration, per PR #15290 review).
// @Tags auth
// @Accept json
// @Produce json
// @Param email query string false "user email (also accepted in JSON body)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/auth/password/forgot/captcha [post]
func (h *UserHandler) ForgotCaptcha(c *gin.Context) {
	var req forgotCaptchaRequest
	// Python reads from request.args (query string), accept both for parity.
	if v := c.Query("email"); v != "" {
		req.Email = v
	} else {
		_ = c.ShouldBindJSON(&req)
	}

	captchaID, captchaImage, errCode, err := h.userService.ForgotIssueCaptcha(req.Email)
	if err != nil {
		common.ResponseWithCodeData(c, errCode, false, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{
		"captcha_id":    captchaID,
		"captcha_image": captchaImage,
	}, "captcha issued")
}

type forgotSendOTPRequest struct {
	Email     string `json:"email"`
	CaptchaID string `json:"captcha_id"`
	Captcha   string `json:"captcha"`
}

// ForgotSendOTP POST /api/v1/auth/password/forgot/otp
// @Summary Send forgot-password OTP
// @Description Validates the captcha (looked up by captcha_id), then
// mints a one-time code, stores a salted hash in Redis (5 min TTL,
// attempt cap, resend cooldown), and emails the OTP to the user.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body forgotSendOTPRequest true "email + captcha_id + captcha"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/auth/password/forgot/otp [post]
func (h *UserHandler) ForgotSendOTP(c *gin.Context) {
	var req forgotSendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, false, err.Error())
		return
	}
	errCode, err := h.userService.ForgotSendOTP(req.Email, req.CaptchaID, req.Captcha)
	if err != nil {
		common.ResponseWithCodeData(c, errCode, false, err.Error())
		return
	}
	common.SuccessWithData(c, true, "verification passed, email sent")
}

type forgotVerifyOTPRequest struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

// ForgotVerifyOTP POST /api/v1/auth/password/forgot/otp/verify
// @Summary Verify forgot-password OTP
// @Description Consumes the OTP if it matches, sets a short-lived
// verified flag the reset endpoint will gate on. Wrong-OTP attempts
// are counted and a 30-minute lockout kicks in at the limit.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body forgotVerifyOTPRequest true "email + otp"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/auth/password/forgot/otp/verify [post]
func (h *UserHandler) ForgotVerifyOTP(c *gin.Context) {
	var req forgotVerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, false, err.Error())
		return
	}
	errCode, err := h.userService.ForgotVerifyOTP(req.Email, req.OTP)
	if err != nil {
		common.ResponseWithCodeData(c, errCode, false, err.Error())
		return
	}
	common.SuccessWithData(c, true, "otp verified")
}

// ForgotResetPassword POST /api/v1/auth/password/reset
// @Summary Reset password after OTP verification
// @Description Requires a successful prior verify call (verified flag
// set in Redis). Updates the password hash and rotates the access
// token so the response can auto-login the user.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body service.ForgotResetPasswordRequest true "email + new password"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/auth/password/reset [post]
func (h *UserHandler) ForgotResetPassword(c *gin.Context) {
	var req service.ForgotResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, false, err.Error())
		return
	}

	user, code, err := h.userService.ForgotResetPassword(&req)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	secretKey, err := server.GetSecretKey(redis.Get())
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, fmt.Sprintf("Failed to get secret key: %s", err.Error()))
		return
	}
	authToken, err := utility.DumpAccessToken(*user.AccessToken, secretKey)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, false, "Failed to generate auth token")
		return
	}
	c.Header("Authorization", authToken)
	c.Header("Access-Control-Expose-Headers", "Authorization")

	// GetUserProfile includes the password hash and the live access_token,
	// which must never appear in the reset response body (the token is
	// already in the Authorization header). Mirror the Python contract
	// `user.to_safe_dict(for_self=True)` by stripping those fields before
	// writing. PR #15290 review.
	profile := h.userService.GetUserProfile(user)
	delete(profile, "password")
	delete(profile, "access_token")
	common.SuccessWithData(c, profile, "Password reset successful. Logged in.")
}
