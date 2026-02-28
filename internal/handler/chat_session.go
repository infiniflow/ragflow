package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// ChatSessionHandler chat session (conversation) handler
type ChatSessionHandler struct {
	chatSessionService *service.ChatSessionService
	userService        *service.UserService
}

// NewChatSessionHandler create chat session handler
func NewChatSessionHandler(chatSessionService *service.ChatSessionService, userService *service.UserService) *ChatSessionHandler {
	return &ChatSessionHandler{
		chatSessionService: chatSessionService,
		userService:        userService,
	}
}

// SetChatSession create or update a chat session
// @Summary Set chat session
// @Description Create or update a chat session. If is_new is true, creates new chat session; otherwise updates existing one.
// @Tags chat_session
// @Accept json
// @Produce json
// @Param request body service.SetChatSessionRequest true "chat session configuration"
// @Success 200 {object} service.SetChatSessionResponse
// @Router /v1/conversation/set [post]
func (h *ChatSessionHandler) SetChatSession(c *gin.Context) {
	// Get access token from Authorization header
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
		return
	}

	// Get user by access token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Invalid access token",
		})
		return
	}
	userID := user.ID

	// Parse request body
	var req service.SetChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Call service to set chat session
	result, err := h.chatSessionService.SetChatSession(userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}

// RemoveChatSessionsRequest remove chat sessions request
type RemoveChatSessionsRequest struct {
	ConversationIDs []string `json:"conversation_ids" binding:"required"`
}

// RemoveChatSessions remove/delete chat sessions
// @Summary Remove Chat Sessions
// @Description Remove chat sessions by their IDs. Only the owner of the chat session can perform this operation.
// @Tags chat_session
// @Accept json
// @Produce json
// @Param request body RemoveChatSessionsRequest true "chat session IDs to remove"
// @Success 200 {object} map[string]interface{}
// @Router /v1/conversation/rm [post]
func (h *ChatSessionHandler) RemoveChatSessions(c *gin.Context) {
	// Get access token from Authorization header
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
		return
	}

	// Get user by access token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Invalid access token",
		})
		return
	}
	userID := user.ID

	// Parse request body
	var req RemoveChatSessionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Call service to remove chat sessions
	if err := h.chatSessionService.RemoveChatSessions(userID, req.ConversationIDs); err != nil {
		// Check if it's an authorization error
		if err.Error() == "Only owner of chat session authorized for this operation" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"data":    false,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    true,
		"message": "success",
	})
}

// ListChatSessions list chat sessions for a dialog
// @Summary List Chat Sessions
// @Description Get list of chat sessions for a specific dialog
// @Tags chat_session
// @Accept json
// @Produce json
// @Param dialog_id query string true "dialog ID"
// @Success 200 {object} service.ListChatSessionsResponse
// @Router /v1/conversation/list [get]
func (h *ChatSessionHandler) ListChatSessions(c *gin.Context) {
	// Get access token from Authorization header
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
		return
	}

	// Get user by access token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Invalid access token",
		})
		return
	}
	userID := user.ID

	// Get dialog_id from query parameter
	dialogID := c.Query("dialog_id")
	if dialogID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "dialog_id is required",
		})
		return
	}

	// Call service to list chat sessions
	result, err := h.chatSessionService.ListChatSessions(userID, dialogID)
	if err != nil {
		// Check if it's an authorization error
		if err.Error() == "Only owner of dialog authorized for this operation" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"data":    false,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result.Sessions,
		"message": "success",
	})
}
