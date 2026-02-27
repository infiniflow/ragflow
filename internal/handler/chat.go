package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// ChatHandler chat handler
type ChatHandler struct {
	chatService *service.ChatService
	userService *service.UserService
}

// NewChatHandler create chat handler
func NewChatHandler(chatService *service.ChatService, userService *service.UserService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		userService: userService,
	}
}

// ListChats list chats
// @Summary List Chats
// @Description Get list of chats (dialogs) for the current user
// @Tags chat
// @Accept json
// @Produce json
// @Success 200 {object} service.ListChatsResponse
// @Router /v1/dialog/list [get]
func (h *ChatHandler) ListChats(c *gin.Context) {
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

	// List chats - default to valid status "1" (same as Python StatusEnum.VALID.value)
	result, err := h.chatService.ListChats(userID, "1")
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

// ListChatsNext list chats with advanced filtering and pagination
// @Summary List Chats Next
// @Description Get list of chats with filtering, pagination and sorting (equivalent to list_dialogs_next)
// @Tags chat
// @Accept json
// @Produce json
// @Param keywords query string false "search keywords"
// @Param page query int false "page number"
// @Param page_size query int false "items per page"
// @Param orderby query string false "order by field (default: create_time)"
// @Param desc query bool false "descending order (default: true)"
// @Param request body service.ListChatsNextRequest true "filter options including owner_ids"
// @Success 200 {object} service.ListChatsNextResponse
// @Router /v1/dialog/next [post]
func (h *ChatHandler) ListChatsNext(c *gin.Context) {
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

	// Parse query parameters
	keywords := c.Query("keywords")

	page := 0
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	orderby := c.DefaultQuery("orderby", "create_time")

	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = descStr != "false"
	}

	// Parse request body for owner_ids
	var req service.ListChatsNextRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
	}

	// List chats with advanced filtering
	result, err := h.chatService.ListChatsNext(userID, keywords, page, pageSize, orderby, desc, req.OwnerIDs)
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

// SetDialog create or update a dialog
// @Summary Set Dialog
// @Description Create or update a dialog (chat). If dialog_id is provided, updates existing dialog; otherwise creates new one.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body service.SetDialogRequest true "dialog configuration"
// @Success 200 {object} service.SetDialogResponse
// @Router /v1/dialog/set [post]
func (h *ChatHandler) SetDialog(c *gin.Context) {
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
	var req service.SetDialogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Validate required field: prompt_config
	if req.PromptConfig == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "prompt_config is required",
		})
		return
	}

	// Call service to set dialog
	result, err := h.chatService.SetDialog(userID, &req)
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
