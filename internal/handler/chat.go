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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/config"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/service"
)

// allowedAudioExts maps allowed file extensions to their expected MIME types for validation.
var allowedAudioExts = map[string][]string{
	".wav":  {"audio/wav", "audio/x-wav"},
	".mp3":  {"audio/mpeg", "audio/mp3"},
	".m4a":  {"audio/mp4", "audio/x-m4a", "audio/m4a"},
	".aac":  {"audio/aac"},
	".flac": {"audio/flac"},
	".ogg":  {"audio/ogg"},
	".webm": {"audio/webm", "video/webm"},
	".opus": {"audio/ogg"}, // Opus in OGG container
	".wma":  {"audio/x-ms-wma"},
}

// ErrNotChatOwner is returned when a non-owner tries to modify a chat.
var ErrNotChatOwner = errors.New("only owner of chat authorized for this operation")

// maxAudioUploadBytes caps the size of an uploaded audio file for transcription (default: 50 MB).
// Initialized from centralized AudioConfig to avoid magic numbers.
var maxAudioUploadBytes int64 = func() int64 {
	return config.GetAudioConfig().MaxUploadBytes
}()

// ChatHandler handles chat-related HTTP requests including TTS and Transcription
type ChatHandler struct {
	chatService  *service.ChatService
	userService  *service.UserService
	audioService *service.AudioService
}

// NewChatHandler creates a new ChatHandler with required dependencies
func NewChatHandler(chatService *service.ChatService, userService *service.UserService, audioService *service.AudioService) *ChatHandler {
	return &ChatHandler{
		chatService:  chatService,
		userService:  userService,
		audioService: audioService,
	}
}

// ListChats list chats
// @Summary List Chats
// @Description Get list of chats (dialogs) for the current user
// @Tags chat
// @Accept json
// @Produce json
// @Success 200 {object} service.ListChatsResponse
// @Router /api/v1/chats [get]
func (h *ChatHandler) ListChats(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
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

	// List chats - default to valid status "1" (same as Python StatusEnum.VALID.value)
	result, err := h.chatService.ListChats(userID, "1", keywords, page, pageSize, orderby, desc)
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

// Create creates a chat.
// @Summary Create Chat
// @Description Create a chat, aligned with Python POST /api/v1/chats.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body service.CreateChatRequest true "chat configuration"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chats [post]
func (h *ChatHandler) Create(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req map[string]interface{}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		jsonError(c, common.CodeArgumentError, err.Error())
		return
	}
	if req == nil {
		req = map[string]interface{}{}
	}

	result, code, err := h.chatService.Create(user.ID, req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
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
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
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
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
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

// RemoveDialogsRequest remove dialogs request
type RemoveDialogsRequest struct {
	DialogIDs []string `json:"dialog_ids" binding:"required"`
}

// RemoveChats remove/delete dialogs (soft delete by setting status to invalid)
// @Summary Remove Dialogs
// @Description Remove dialogs by setting their status to invalid. Only the owner of the dialog can perform this operation.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body RemoveDialogsRequest true "dialog IDs to remove"
// @Success 200 {object} map[string]interface{}
// @Router /v1/dialog/rm [post]
func (h *ChatHandler) RemoveChats(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse request body
	var req RemoveDialogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Call service to remove dialogs
	if err := h.chatService.RemoveChats(userID, req.DialogIDs); err != nil {
		// Check if it's an authorization error
		if err.Error() == "only owner of chat authorized for this operation" {
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

// DeleteChat soft deletes a chat by ID.
func (h *ChatHandler) DeleteChat(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	chatID := c.Param("chat_id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "chat_id is required",
		})
		return
	}

	if err := h.chatService.DeleteChat(userID, chatID); err != nil {
		if err.Error() == "no authorization" {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeAuthenticationError,
				"data":    false,
				"message": "No authorization.",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    true,
		"message": "success",
	})
}

// BulkDeleteChats soft deletes multiple chats owned by the current user.
func (h *ChatHandler) BulkDeleteChats(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeSuccess,
			"data":    map[string]interface{}{},
			"message": "success",
		})
		return
	}

	var req service.BulkDeleteChatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	if len(req.IDs) == 0 && !req.DeleteAll {
		if req.ChatID != "" {
			if err := h.chatService.DeleteChat(userID, req.ChatID); err != nil {
				if err.Error() == "no authorization" {
					c.JSON(http.StatusOK, gin.H{
						"code":    common.CodeAuthenticationError,
						"data":    false,
						"message": "No authorization.",
					})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"code":    common.CodeDataError,
					"data":    nil,
					"message": err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeSuccess,
				"data":    true,
				"message": "success",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeSuccess,
			"data":    map[string]interface{}{},
			"message": "success",
		})
		return
	}

	result, err := h.chatService.BulkDeleteChats(userID, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	message := "success"
	if errorsList, ok := result["errors"].([]string); ok && len(errorsList) > 0 {
		if successCount, ok := result["success_count"].(int); ok {
			message = "Partially deleted " + strconv.Itoa(successCount) + " chats with " + strconv.Itoa(len(errorsList)) + " errors"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": message,
	})
}

// GetChat get chat detail
// @Summary Get Chat Detail
// @Description Get detail of a chat by ID
// @Tags chat
// @Accept json
// @Produce json
// @Param chat_id path string true "chat ID"
// @Success 200 {object} service.GetChatResponse
// @Router /api/v1/chats/{chat_id} [get]
// Reference: api/apps/restful_apis/chat_api.py::get_chat
// Python implementation details:
// - Route: @manager.route("/chats/<chat_id>", methods=["GET"])
func (h *ChatHandler) GetChat(c *gin.Context) {
	// Get current user from context (same as Python current_user)
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get chat_id from path parameter (same as Python <chat_id>)
	chatID := c.Param("chat_id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "chat_id is required",
		})
		return
	}

	// Get chat detail with permission check
	chat, err := h.chatService.GetChat(userID, chatID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's an authorization error
		if errMsg == "no authorization" {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeAuthenticationError,
				"data":    false,
				"message": "No authorization.",
			})
			return
		}
		// Not found error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	// Build response (same as Python _build_chat_response)
	// The service already returns GetChatResponse with DatasetIDs and KBNames
	result := map[string]interface{}{
		"id":                       chat.ID,
		"tenant_id":                chat.TenantID,
		"name":                     chat.Name,
		"description":              chat.Description,
		"icon":                     chat.Icon,
		"language":                 chat.Language,
		"llm_id":                   chat.LLMID,
		"llm_setting":              chat.LLMSetting,
		"prompt_type":              chat.PromptType,
		"prompt_config":            chat.PromptConfig,
		"meta_data_filter":         chat.MetaDataFilter,
		"similarity_threshold":     chat.SimilarityThreshold,
		"vector_similarity_weight": chat.VectorSimilarityWeight,
		"top_n":                    chat.TopN,
		"top_k":                    chat.TopK,
		"do_refer":                 chat.DoRefer,
		"rerank_id":                chat.RerankID,
		"dataset_ids":              chat.DatasetIDs,
		"kb_names":                 chat.KBNames,
		"status":                   chat.Status,
		"create_time":              chat.CreateTime,
		"create_date":              chat.CreateDate,
		"update_time":              chat.UpdateTime,
		"update_date":              chat.UpdateDate,
		"tenant_llm_id":            chat.TenantLLMID,
		"tenant_rerank_id":         chat.TenantRerankID,
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

// UpdateChat updates a chat by ID using REST PUT semantics.
func (h *ChatHandler) UpdateChat(c *gin.Context) {
	h.updateChatByMethod(c, false)
}

// PatchChat updates a chat by ID using REST PATCH semantics.
func (h *ChatHandler) PatchChat(c *gin.Context) {
	h.updateChatByMethod(c, true)
}

func (h *ChatHandler) updateChatByMethod(c *gin.Context, patch bool) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	chatID := c.Param("chat_id")
	if chatID == "" {
		jsonError(c, common.CodeBadRequest, "chat_id is required")
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	var (
		result map[string]interface{}
		err    error
	)
	if patch {
		result, err = h.chatService.PatchChat(user.ID, chatID, req)
	} else {
		result, err = h.chatService.UpdateChat(user.ID, chatID, req)
	}
	if err != nil {
		if err.Error() == "no authorization" {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeAuthenticationError,
				"data":    false,
				"message": "No authorization.",
			})
			return
		}
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// SpeechRequest is the request body for the TTS endpoint
type SpeechRequest struct {
	Text  string `json:"text" binding:"required"`
	Voice string `json:"voice"`
}

// TTS synthesizes speech from text using the tenant's default TTS model
// Reference: Python api/apps/restful_apis/chat_api.py::tts
func (h *ChatHandler) TTS(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	tenantID := user.ID

	var req SpeechRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		jsonError(c, common.CodeDataError, "`text` is required")
		return
	}

	ttsModel, segments, err := h.audioService.PrepareSpeech(c.Request.Context(), tenantID, req.Text, req.Voice)
	if err != nil {
		logger.Error("TTS PrepareSpeech failed", err)
		jsonError(c, common.CodeServerError, "Failed to prepare speech")
		return
	}

	c.Header("Content-Type", "audio/mpeg")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(interface{ Flush() })

	err = h.audioService.StreamSpeech(c.Request.Context(), ttsModel, segments, func(chunk []byte) error {
		if _, err := c.Writer.Write(chunk); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	})
	if err != nil {
		// At this point audio headers have already been sent; we cannot switch to JSON.
		// Log the error and return - the client will see an incomplete/truncated response.
		logger.Error("TTS streaming error after headers sent", err)
	}
}

// Transcription transcribes an uploaded audio file to text using enhanced security validation
// Reference: Python api/apps/restful_apis/chat_api.py::transcription
func (h *ChatHandler) Transcription(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	tenantID := user.ID

	streamMode := strings.ToLower(c.PostForm("stream")) == "true"

	fileHeader, err := c.FormFile("file")
	if err != nil {
		jsonError(c, common.CodeDataError, "Missing 'file' in multipart form-data")
		return
	}

	// Security validation: size check
	if fileHeader.Size > maxAudioUploadBytes {
		logger.Warn("Audio upload exceeds size limit",
			zap.Int64("size", fileHeader.Size),
			zap.Int64("max", maxAudioUploadBytes),
			zap.String("user", user.ID))
		jsonError(c, common.CodeBadRequest,
			fmt.Sprintf("Audio file too large: %d bytes (max %d)", fileHeader.Size, maxAudioUploadBytes))
		return
	}

	// Security validation: extension check
	suffix := strings.ToLower(filepath.Ext(fileHeader.Filename))
	allowedMIMEs, extAllowed := allowedAudioExts[suffix]
	if !extAllowed {
		allowed := make([]string, 0, len(allowedAudioExts))
		for ext := range allowedAudioExts {
			allowed = append(allowed, ext)
		}
		logger.Warn("Unsupported audio format upload attempted",
			zap.String("extension", suffix),
			zap.String("user", user.ID))
		jsonError(c, common.CodeDataError,
			fmt.Sprintf("Unsupported audio format: %s. Allowed: %v", suffix, strings.Join(allowed, ", ")))
		return
	}

	// Save to temp file first for content inspection
	tmpFile, err := os.CreateTemp("", "*"+suffix)
	if err != nil {
		logger.Error("Failed to create temp file for audio upload", err)
		jsonError(c, common.CodeServerError, "Failed to process upload")
		return
	}
	tempAudioPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tempAudioPath)

	if err := c.SaveUploadedFile(fileHeader, tempAudioPath); err != nil {
		logger.Error("Failed to save uploaded audio file", err)
		jsonError(c, common.CodeServerError, "Failed to save upload")
		return
	}

	// Security validation: MIME type detection from actual content
	fileInfo, statErr := os.Stat(tempAudioPath)
	if statErr != nil {
		logger.Error("Failed to stat temp audio file", statErr)
		jsonError(c, common.CodeServerError, "Failed to validate upload")
		return
	}

	// Check for zero-byte files
	if fileInfo.Size() == 0 {
		logger.Warn("Empty audio file uploaded", zap.String("user", user.ID))
		jsonError(c, common.CodeDataError, "Uploaded file is empty")
		return
	}

	// Detect actual content type (basic magic byte detection)
	detectedMIME, detectErr := detectAudioMIMEType(tempAudioPath)
	if detectErr != nil || detectedMIME == "" {
		logger.Warn("Could not determine audio content type",
			zap.Error(detectErr),
			zap.String("path", tempAudioPath))
	} else if !isMIMEAllowed(detectedMIME, allowedMIMEs) {
		logger.Error("MIME type mismatch - possible file spoofing attack",
			fmt.Errorf("expected: %v, detected: %s", allowedMIMEs, detectedMIME))
		jsonError(c, common.CodeBadRequest, "Invalid file content: declared format does not match actual content")
		return
	}

	if streamMode {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		flusher, _ := c.Writer.(interface{ Flush() })
		sendErr := h.audioService.StreamTranscription(c.Request.Context(), tenantID, tempAudioPath, func(evt entity.TranscriptionEvent) error {
			payload, mErr := json.Marshal(evt)
			if mErr != nil {
				return mErr
			}
			if _, wErr := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); wErr != nil {
				return wErr
			}
			if flusher != nil {
				flusher.Flush()
			}
			return nil
		})
		if sendErr != nil && flusher != nil {
			flusher.Flush()
		}
		return
	}

	text, err := h.audioService.Transcription(c.Request.Context(), tenantID, tempAudioPath)
	if err != nil {
		logger.Error("Transcription failed", err)
		jsonError(c, common.CodeServerError, "Failed to transcribe audio")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": gin.H{"text": text},
	})
}

// detectAudioMIMEType performs basic magic byte detection for common audio formats.
// Returns the detected MIME type or empty string on failure.
func detectAudioMIMEType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read first 12 bytes for magic number detection
	header := make([]byte, 12)
	n, err := file.Read(header)
	if err != nil && n == 0 {
		return "", err
	}

	// RIFF/WAV format
	if n >= 4 && string(header[0:4]) == "RIFF" {
		return "audio/wav", nil
	}

	// MP3 ID3 tag or sync word
	if n >= 2 && (header[0] == 0xFF && (header[1]&0xE0) == 0xE0) {
		return "audio/mpeg", nil
	}
	if n >= 3 && string(header[0:3]) == "ID3" {
		return "audio/mpeg", nil
	}

	// OGG/Opus format
	if n >= 4 && string(header[0:4]) == "OggS" {
		return "audio/ogg", nil
	}

	// FLAC format
	if n >= 4 && string(header[0:4]) == "fLaC" {
		return "audio/flac", nil
	}

	// MP4/M4A/AAC format (ftyp box)
	if n >= 8 && string(header[4:8]) == "ftyp" {
		return "audio/mp4", nil
	}

	// WebM format (EBML header)
	if n >= 4 && binary.BigEndian.Uint32(header[0:4]) == 0x1A45DFA3 {
		return "video/webm", nil
	}

	return "", fmt.Errorf("unknown audio format")
}

// isMIMEAllowed checks if the detected MIME type is in the allowed list for this extension.
func isMIMEAllowed(mime string, allowed []string) bool {
	for _, a := range allowed {
		if mime == a {
			return true
		}
	}
	return false
}
