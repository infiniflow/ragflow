package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// TenantHandler tenant handler
type TenantHandler struct {
	tenantService *service.TenantService
	userService   *service.UserService
}

// NewTenantHandler create tenant handler
func NewTenantHandler(tenantService *service.TenantService, userService *service.UserService) *TenantHandler {
	return &TenantHandler{
		tenantService: tenantService,
		userService:   userService,
	}
}

// TenantInfo get tenant information
// @Summary Get Tenant Information
// @Description Get current user's tenant information (owner tenant)
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/tenant_info [get]
func (h *TenantHandler) TenantInfo(c *gin.Context) {
	// Extract token from request
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
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

	// Get tenant info
	tenantInfo, err := h.tenantService.GetTenantInfo(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get tenant information",
		})
		return
	}

	if tenantInfo == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Tenant not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": tenantInfo,
	})
}

// TenantList get tenant list for current user
// @Summary Get Tenant List
// @Description Get all tenants that the current user belongs to
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/list [get]
func (h *TenantHandler) TenantList(c *gin.Context) {
	// Extract token from request
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
		return
	}

	// Get user by token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Invalid access token",
		})
		return
	}

	// Get tenant list
	tenantList, err := h.tenantService.GetTenantList(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get tenant list",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": tenantList,
	})
}
