package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// LLMHandler LLM handler
type LLMHandler struct {
	llmService  *service.LLMService
	userService *service.UserService
}

// NewLLMHandler create LLM handler
func NewLLMHandler(llmService *service.LLMService, userService *service.UserService) *LLMHandler {
	return &LLMHandler{
		llmService:  llmService,
		userService: userService,
	}
}

// GetMyLLMs get my LLMs
// @Summary Get My LLMs
// @Description Get LLM list for current tenant
// @Tags llm
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param include_details query string false "Include detailed fields" default(false)
// @Success 200 {object} map[string]interface{}
// @Router /v1/llm/my_llms [get]
func (h *LLMHandler) GetMyLLMs(c *gin.Context) {
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

	// Get tenant ID from user
	tenantID := user.ID
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "User has no tenant ID",
		})
		return
	}

	// Parse include_details query parameter
	includeDetailsStr := c.DefaultQuery("include_details", "false")
	includeDetails := includeDetailsStr == "true"

	// Get LLMs for tenant
	llms, err := h.llmService.GetMyLLMs(tenantID, includeDetails)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get LLMs",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": llms,
	})
}
