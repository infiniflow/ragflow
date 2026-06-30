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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strings"

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

// ListChatSessions list chat sessions for a dialog
// @Summary List Chat Sessions
// @Description Get list of chat sessions for a specific dialog
// @Tags chat_session
// @Accept json
// @Produce json
// @Param chat_id query string true "chat ID"
// @Success 200 {object} service.ListChatSessionsResponse
// @Router /api/v1/chats/<chat_id>/sessions [get]
func (h *ChatSessionHandler) ListChatSessions(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get chat_id from query parameter
	chatID := c.Param("chat_id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "chat_id is required",
		})
		return
	}

	// Call service to list chat sessions
	result, err := h.chatSessionService.ListChatSessions(userID, chatID)
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

type ChatCompletionsRequest struct {
	ChatID                 string                   `json:"chat_id,omitempty"`
	SessionID              string                   `json:"session_id,omitempty"`
	ConversationID         string                   `json:"conversation_id,omitempty"`
	Messages               []map[string]interface{} `json:"messages,omitempty"`
	Question               string                   `json:"question,omitempty"`
	Files                  []interface{}            `json:"files,omitempty"`
	LLMID                  string                   `json:"llm_id,omitempty"`
	PassAllHistoryMessages *bool                    `json:"pass_all_history_messages,omitempty"`
	PassAllHistory         *bool                    `json:"pass_all_history,omitempty"`
	Legacy                 bool                     `json:"legacy,omitempty"`
	Stream                 *bool                    `json:"stream"`
	Temperature            *float64                 `json:"temperature,omitempty"`
	TopP                   *float64                 `json:"top_p,omitempty"`
	FrequencyPenalty       *float64                 `json:"frequency_penalty,omitempty"`
	PresencePenalty        *float64                 `json:"presence_penalty,omitempty"`
	MaxTokens              *int                     `json:"max_tokens,omitempty"`
}

// ChatCompletions chat completion
// @Summary Chat Completion
// @Description Send messages to the chat model and get a response. Supports streaming and non-streaming modes.
// @Tags chat_session
// @Accept json
// @Produce json
// @Param request body ChatCompletionsRequest true "chat completion request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chat/completions [post]
func (h *ChatSessionHandler) ChatCompletions(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	var rawBody map[string]interface{}
	if err := c.ShouldBindJSON(&rawBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	var req ChatCompletionsRequest
	b, err := json.Marshal(rawBody)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := json.Unmarshal(b, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// Normalize session_id / conversation_id
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = req.ConversationID
	}

	// Build generation config
	genConfig := make(map[string]interface{})
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		genConfig["top_p"] = *req.TopP
	}
	if req.FrequencyPenalty != nil {
		genConfig["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		genConfig["presence_penalty"] = *req.PresencePenalty
	}
	if req.MaxTokens != nil {
		genConfig["max_tokens"] = *req.MaxTokens
	}

	// Resolve pass_all_history from either alias
	passAllHistory := false
	if req.PassAllHistory != nil {
		passAllHistory = *req.PassAllHistory
	}
	if req.PassAllHistoryMessages != nil {
		passAllHistory = *req.PassAllHistoryMessages
	}

	// Remove known keys from rawBody; what remains is passthrough kwargs
	knownKeys := []string{
		"chat_id", "session_id", "conversation_id",
		"messages", "question", "files",
		"llm_id",
		"pass_all_history_messages", "pass_all_history",
		"legacy", "stream",
		"temperature", "top_p", "frequency_penalty", "presence_penalty", "max_tokens",
	}
	for _, key := range knownKeys {
		delete(rawBody, key)
	}
	kwargs := rawBody

	// Determine stream mode
	streamMode := true
	if req.Stream != nil {
		streamMode = *req.Stream
	}

	if streamMode {
		disableWriteDeadlineForSSE(c)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		streamChan := make(chan string)
		reqCtx := c.Request.Context()
		go func() {
			defer close(streamChan)
			_, _ = h.chatSessionService.ChatCompletions(
				reqCtx, userID,
				req.ChatID, sessionID,
				req.Messages, req.Question, req.Files,
				req.LLMID, genConfig, kwargs,
				passAllHistory, req.Legacy,
				true, streamChan,
			)
		}()

		c.Stream(func(w io.Writer) bool {
			data, ok := <-streamChan
			if !ok {
				return false
			}
			c.Writer.Write([]byte(data))
			return true
		})
	} else {
		result, err := h.chatSessionService.ChatCompletions(
			c.Request.Context(), userID,
			req.ChatID, sessionID,
			req.Messages, req.Question, req.Files,
			req.LLMID, genConfig, kwargs,
			passAllHistory, req.Legacy,
			false, nil,
		)
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

func (h *ChatSessionHandler) GetSession(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := user.ID
	chatID, sessionID := c.Param("chat_id"), c.Param("session_id")

	result, code, err := h.chatSessionService.GetSession(userID, chatID, sessionID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}
	jsonResponse(c, common.CodeSuccess, result, "success")
}

// CreateSession create a session in a dialog
func (h *ChatSessionHandler) CreateSession(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeBadRequest, "user_id is required")
		return
	}

	chatID := strings.TrimSpace(c.Param("chat_id"))
	if chatID == "" {
		jsonError(c, common.CodeBadRequest, "chat_id is required")
		return
	}

	req := map[string]interface{}{}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			req = map[string]interface{}{}
		} else {
			jsonError(c, common.CodeArgumentError, err.Error())
			return
		}
	}
	if req == nil {
		req = map[string]interface{}{}
	}

	result, code, err := h.chatSessionService.CreateSession(userID, chatID, req)
	if err != nil {
		if code == common.CodeAuthenticationError {
			jsonResponse(c, code, false, err.Error())
			return
		}
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteSessions delete a session in a dialog
func (h *ChatSessionHandler) DeleteSessions(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	chatID := strings.TrimSpace(c.Param("chat_id"))
	if chatID == "" {
		jsonError(c, common.CodeBadRequest, "chat_id is required")
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeBadRequest, "user_id is required")
		return
	}

	req := map[string]interface{}{}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			req = map[string]interface{}{}
		} else {
			jsonError(c, common.CodeArgumentError, err.Error())
			return
		}
	}
	if req == nil {
		req = map[string]interface{}{}
	}

	result, message, code, err := h.chatSessionService.DeleteSessions(userID, chatID, req)
	if err != nil {
		if code == common.CodeAuthenticationError {
			jsonResponse(c, code, false, err.Error())
			return
		}
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, message)
}

func (h *ChatSessionHandler) UpdateSession(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := user.ID
	chatID, sessionID := c.Param("chat_id"), c.Param("session_id")

	req := map[string]any{}
	if err := c.ShouldBindJSON(&req); err != nil {
		if errors.Is(err, io.EOF) {
			jsonError(c, common.CodeArgumentError, "Request body cannot be empty")
			return
		}
		jsonError(c, common.CodeArgumentError, "Invalid request: "+err.Error())
		return
	}
	if len(req) == 0 {
		jsonError(c, common.CodeArgumentError, "Request body cannot be empty")
		return
	}

	result, code, err := h.chatSessionService.UpdateSession(userID, chatID, sessionID, req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}
	jsonResponse(c, common.CodeSuccess, result, "success")
}
