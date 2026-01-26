package handler

import (
	"net/http"
	"strconv"
	"strings"

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
// @Router /api/v1/users/register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	user, err := h.userService.Register(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "registration successful",
		"data": gin.H{
			"id":       user.ID,
			"nickname": user.Nickname,
			"email":    user.Email,
		},
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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	user, err := h.userService.Login(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": err.Error(),
		})
		return
	}

	// Set Authorization header with access_token
	if user.AccessToken != nil {
		c.Header("Authorization", *user.AccessToken)
	}
	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "Welcome back!",
		"data":    user,
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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	user, err := h.userService.LoginByEmail(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": err.Error(),
		})
		return
	}

	// Set Authorization header with access_token
	if user.AccessToken != nil {
		c.Header("Authorization", *user.AccessToken)
	}
	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Access-Control-Expose-Headers", "Authorization")

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "Welcome back!",
		"data":    user,
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid user id",
		})
		return
	}

	user, err := h.userService.GetUserByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "user not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": user,
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

	users, total, err := h.userService.ListUsers(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get users",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"items":     users,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// Helper function to extract token from request
func extractToken(c *gin.Context) string {
	// Try to get token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		// Expected format: "Bearer token"
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}
	// Try to get token from query parameter
	token := c.Query("token")
	if token != "" {
		return token
	}
	// Try to get token from form data
	token = c.PostForm("token")
	if token != "" {
		return token
	}
	return ""
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
	// Extract token from request
	token := extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Access token required",
		})
		return
	}

	// Get user by token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid access token",
		})
		return
	}

	// Logout user
	if err := h.userService.Logout(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to logout",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "logout successful",
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
	// Extract token from request
	token := extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Access token required",
		})
		return
	}

	// Get user by token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid access token",
		})
		return
	}

	// Get user profile
	profile := h.userService.GetUserProfile(user)

	c.JSON(http.StatusOK, gin.H{
		"data": profile,
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
	token := extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Access token required",
		})
		return
	}

	// Get user by token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid access token",
		})
		return
	}

	// Parse request
	var req service.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Update user settings
	if err := h.userService.UpdateUserSettings(user, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "settings updated successfully",
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
	// Extract token from request
	token := extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Access token required",
		})
		return
	}

	// Get user by token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid access token",
		})
		return
	}

	// Parse request
	var req service.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Change password
	if err := h.userService.ChangePassword(user, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "password changed successfully",
	})
}


