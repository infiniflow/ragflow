package handler

import (
	"fmt"
	"io"
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

// CompletionRequest completion request
type CompletionRequest struct {
	ConversationID string                   `json:"conversation_id" binding:"required"`
	Messages       []map[string]interface{} `json:"messages" binding:"required"`
	LLMID          string                   `json:"llm_id,omitempty"`
	Stream         bool                     `json:"stream,omitempty"`
	Temperature    float64                  `json:"temperature,omitempty"`
	TopP           float64                  `json:"top_p,omitempty"`
	FrequencyPenalty float64                `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64                `json:"presence_penalty,omitempty"`
	MaxTokens      int                      `json:"max_tokens,omitempty"`
}

// Completion chat completion
// @Summary Chat Completion
// @Description Send messages to the chat model and get a response. Supports streaming and non-streaming modes.
// @Tags chat_session
// @Accept json
// @Produce json
// @Param request body CompletionRequest true "completion request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/conversation/completion [post]
func (h *ChatSessionHandler) Completion(c *gin.Context) {
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
	var req CompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Build chat model config
	chatModelConfig := make(map[string]interface{})
	if req.Temperature != 0 {
		chatModelConfig["temperature"] = req.Temperature
	}
	if req.TopP != 0 {
		chatModelConfig["top_p"] = req.TopP
	}
	if req.FrequencyPenalty != 0 {
		chatModelConfig["frequency_penalty"] = req.FrequencyPenalty
	}
	if req.PresencePenalty != 0 {
		chatModelConfig["presence_penalty"] = req.PresencePenalty
	}
	if req.MaxTokens != 0 {
		chatModelConfig["max_tokens"] = req.MaxTokens
	}

	// Process messages - filter out system messages and initial assistant messages
	var processedMessages []map[string]interface{}
	for i, m := range req.Messages {
		role, _ := m["role"].(string)
		if role == "system" {
			continue
		}
		if role == "assistant" && len(processedMessages) == 0 {
			continue
		}
		processedMessages = append(processedMessages, m)
		_ = i
	}

	// Get last message ID if present
	var messageID string
	if len(processedMessages) > 0 {
		if id, ok := processedMessages[len(processedMessages)-1]["id"].(string); ok {
			messageID = id
		}
	}

	// Call service
	if req.Stream {
		// Streaming response
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		// Create a channel for streaming data
		streamChan := make(chan string)
		go func() {
			defer close(streamChan)
			err := h.chatSessionService.CompletionStream(userID, req.ConversationID, processedMessages, req.LLMID, chatModelConfig, messageID, streamChan)
			if err != nil {
				streamChan <- fmt.Sprintf("data: %s\n\n", err.Error())
			}
		}()

		// Stream data to client
		c.Stream(func(w io.Writer) bool {
			data, ok := <-streamChan
			if !ok {
				return false
			}
			c.Writer.Write([]byte(data))
			return true
		})
	} else {
		// Non-streaming response
		result, err := h.chatSessionService.Completion(userID, req.ConversationID, processedMessages, req.LLMID, chatModelConfig, messageID)
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
			"message": "",
		})
	}
}
