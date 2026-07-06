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

// Package handler contains all HTTP request handlers
// This file implements Memory-related API endpoint handlers
// Each method corresponds to an API endpoint in the Python memory_api.py
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// MemoryHandler handles Memory-related HTTP requests
// Responsible for processing all Memory-related HTTP requests
// Each method corresponds to an API endpoint, implementing the same logic as Python memory_api.py
type MemoryHandler struct {
	memoryService *service.MemoryService // Reference to Memory business service layer
}

// NewMemoryHandler creates a new MemoryHandler instance
//
// Parameters:
//   - memoryService: Pointer to MemoryService business service layer
//
// Returns:
//   - *MemoryHandler: Initialized handler instance
func NewMemoryHandler(memoryService *service.MemoryService) *MemoryHandler {
	return &MemoryHandler{
		memoryService: memoryService,
	}
}

// CreateMemory handles POST request for creating Memory
// API Path: POST /api/v1/memories
//
// Function:
//   - Creates a new memory record
//   - Supports automatic system_prompt generation
//   - Supports name deduplication (if name exists, adds sequence number)
//
// Request Parameters (JSON Body):
//   - name (required): Memory name, max 128 characters
//   - memory_type (required): Memory type array, supports ["raw", "semantic", "episodic", "procedural"]
//   - embd_id (required): Embedding model ID
//   - llm_id (required): LLM model ID
//   - tenant_embd_id (optional): Tenant embedding model ID
//   - tenant_llm_id (optional): Tenant LLM model ID
//
// Response Format:
//   - code: Status code (0=success, other=error)
//   - message: true on success, error message on failure
//   - data: Memory object on success
//
// Business Logic (matching Python create_memory):
//  1. Validate user login status
//  2. Parse and validate request parameters
//  3. Call service layer to create memory
//  4. Return creation result
func (h *MemoryHandler) CreateMemory(c *gin.Context) {
	// Check if API timing is enabled
	// If RAGFLOW_API_TIMING environment variable is set, request processing time will be logged
	timingEnabled := os.Getenv("RAGFLOW_API_TIMING")
	var tStart time.Time
	if timingEnabled != "" {
		tStart = time.Now()
	}

	// Get current logged-in user information
	// GetUser is a context value set by the authentication middleware
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Parse JSON request body
	var req service.CreateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, nil, err.Error())
		return
	}

	// Validate required field: name
	if req.Name == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "name is required")
		return
	}

	// Validate required field: memory_type (must be non-empty array)
	if len(req.MemoryType) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory type is required and must be a list")
		return
	}

	// Validate required field: embd_id
	if req.EmbdID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "embedding model ID is required")
		return
	}

	// Validate required field: llm_id
	if req.LLMID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "language model ID is required")
		return
	}

	// Record request parsing completion time (for timing)
	tParsed := time.Now()

	// Call service layer to create memory
	result, err := h.memoryService.CreateMemory(userID, &req)
	if err != nil {
		// Log error if timing is enabled
		if timingEnabled != "" {
			totalMs := float64(time.Since(tStart).Microseconds()) / 1000.0
			parseMs := float64(tParsed.Sub(tStart).Microseconds()) / 1000.0
			_ = parseMs
			_ = totalMs
		}

		errMsg := err.Error()
		// Determine if it's an argument error and return appropriate error code
		if isArgumentError(errMsg) {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, errMsg)
			return
		}

		// Other errors return server error
		common.ResponseWithCodeData(c, common.CodeServerError, nil, errMsg)
		return
	}

	// Log success if timing is enabled
	if timingEnabled != "" {
		totalMs := float64(time.Since(tStart).Microseconds()) / 1000.0
		parseMs := float64(tParsed.Sub(tStart).Microseconds()) / 1000.0
		validateAndDbMs := totalMs - parseMs
		_ = parseMs
		_ = validateAndDbMs
		_ = totalMs
	}

	// Return success response
	common.SuccessWithData(c, result, "success")
}

// UpdateMemory handles PUT request for updating Memory
// API Path: PUT /api/v1/memories/:memory_id
//
// Function:
//   - Updates configuration information for the specified memory
//   - Supports partial updates: only update passed fields
//
// Request Parameters (JSON Body):
//   - name (optional): Memory name
//   - permissions (optional): Permission setting ["me", "team", "all"]
//   - llm_id (optional): LLM model ID
//   - embd_id (optional): Embedding model ID
//   - tenant_llm_id (optional): Tenant LLM model ID
//   - tenant_embd_id (optional): Tenant embedding model ID
//   - memory_type (optional): Memory type array
//   - memory_size (optional): Memory size, range (0, 5242880]
//   - forgetting_policy (optional): Forgetting policy, default "FIFO"
//   - temperature (optional): Temperature parameter, range [0, 1]
//   - avatar (optional): Avatar URL
//   - description (optional): Description
//   - system_prompt (optional): System prompt
//   - user_prompt (optional): User prompt
//
// Business Rules:
//   - name length <= 128 characters
//   - Cannot update tenant_embd_id, embd_id, memory_type when memory_size > 0
//   - When updating memory_type, system_prompt is automatically regenerated if it's the default
func (h *MemoryHandler) UpdateMemory(c *gin.Context) {
	// Get memory_id from URL path
	memoryID := c.Param("memory_id")
	if memoryID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory ID is required")
		return
	}

	// Get current logged-in user information
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Parse JSON request body
	var req service.UpdateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, nil, err.Error())
		return
	}

	// Call service layer to update memory
	result, err := h.memoryService.UpdateMemory(userID, memoryID, &req)
	if err != nil {
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, errMsg)
			return
		}

		// Check if it's an argument error
		if isArgumentError(errMsg) {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, errMsg)
			return
		}

		// Other errors return server error
		common.ResponseWithCodeData(c, common.CodeServerError, nil, errMsg)
		return
	}

	// Return success response
	common.SuccessWithData(c, result, "success")
}

// DeleteMemory handles DELETE request for deleting Memory
// API Path: DELETE /api/v1/memories/:memory_id
//
// Function:
//   - Deletes the specified memory record
//   - Also deletes associated message data
//
// Business Logic:
//  1. Check if memory exists
//  2. Delete memory record
//  3. Delete associated message index
func (h *MemoryHandler) DeleteMemory(c *gin.Context) {
	// Get memory_id from URL path
	memoryID := c.Param("memory_id")
	if memoryID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory ID is required")
		return
	}

	// Call service layer to delete memory
	err := h.memoryService.DeleteMemory(memoryID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, errMsg)
			return
		}

		// Other errors return server error
		common.ResponseWithCodeData(c, common.CodeServerError, nil, errMsg)
		return
	}

	// Return success response
	common.SuccessNoData(c, "success")
}

// ListMemories handles GET request for listing Memories
// API Path: GET /api/v1/memories
//
// Function:
//   - Lists memories accessible to the current user
//   - Supports multiple filter conditions
//   - Supports pagination and keyword search
//
// Query Parameters:
//   - memory_type (optional): Memory type filter, supports comma-separated multiple types
//   - tenant_id (optional): Tenant ID filter
//   - storage_type (optional): Storage type filter
//   - keywords (optional): Keyword search (fuzzy match on name)
//   - page (optional): Page number, default 1
//   - page_size (optional): Items per page, default 50
//
// Response Format:
//   - code: Status code
//   - message: true
//   - data.memory_list: Array of Memory objects
//   - data.total_count: Total record count
func (h *MemoryHandler) ListMemories(c *gin.Context) {
	// Get current logged-in user information
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Parse query parameters
	memoryTypesParam := c.Query("memory_type")
	tenantIDsParam := c.Query("tenant_id")
	storageType := c.Query("storage_type")
	keywords := c.Query("keywords")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "50")

	// Convert pagination parameters to integers
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	// Parse memory_type parameter (supports comma separation)
	var memoryTypes []string
	if memoryTypesParam != "" {
		if strings.Contains(memoryTypesParam, ",") {
			memoryTypes = strings.Split(memoryTypesParam, ",")
		} else {
			memoryTypes = []string{memoryTypesParam}
		}
	}

	// Parse tenant_id parameter
	// If not specified, service will get all tenants associated with the user
	var tenantIDs []string
	if tenantIDsParam != "" {
		if strings.Contains(tenantIDsParam, ",") {
			tenantIDs = strings.Split(tenantIDsParam, ",")
		} else {
			tenantIDs = []string{tenantIDsParam}
		}
	}

	// Call service layer to get memory list
	result, err := h.memoryService.ListMemories(user.ID, tenantIDs, memoryTypes, storageType, keywords, page, pageSize)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	// Return success response
	common.SuccessWithData(c, result, "success")
}

// GetMemoryConfig handles GET request for getting Memory configuration
// API Path: GET /api/v1/memories/:memory_id/config
//
// Function:
//   - Gets complete configuration information for the specified memory
//   - Includes owner name (obtained via JOIN with user table)
//
// Response Format:
//   - code: Status code
//   - message: true
//   - data: Memory object, including owner_name field
func (h *MemoryHandler) GetMemoryConfig(c *gin.Context) {
	// Get memory_id from URL path
	memoryID := c.Param("memory_id")
	if memoryID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory ID is required")
		return
	}

	// Call service layer to get memory configuration
	result, err := h.memoryService.GetMemoryConfig(memoryID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, errMsg)
			return
		}

		// Other errors return server error
		common.ResponseWithCodeData(c, common.CodeServerError, nil, errMsg)
		return
	}

	// Return success response
	common.SuccessWithData(c, result, "success")
}

// GetMemoryMessages handles GET request for getting Memory messages
// API Path: GET /api/v1/memories/:memory_id
//
// Function:
//   - Gets message list associated with the specified memory
//   - Supports filtering by agent_id
//   - Supports keyword search and pagination
//
// Query Parameters:
//   - agent_id (optional): Agent ID filter, supports comma-separated multiple
//   - keywords (optional): Keyword search
//   - page (optional): Page number, default 1
//   - page_size (optional): Items per page, default 50
//
// Response Format:
//   - code: Status code
//   - message: true
//   - data.messages: Array of message objects
//   - data.storage_type: Storage type
func (h *MemoryHandler) GetMemoryMessages(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "user id is required")
		return
	}

	memoryID := strings.TrimSpace(c.Param("memory_id"))
	if memoryID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory_id is required")
		return
	}

	var agentIDs []string
	values := c.QueryArray("agent_id")
	for _, v := range values {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				agentIDs = append(agentIDs, p)
			}
		}
	}

	keywords := strings.TrimSpace(c.DefaultQuery("keywords", ""))
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "page must be a positive integer")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if err != nil || pageSize <= 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "page_size must be a positive integer")
		return
	}
	if pageSize > 100 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "page_size must be less than or equal to 100")
		return
	}

	data, err := h.memoryService.GetMemoryMessages(c.Request.Context(), userID, memoryID, agentIDs, keywords, page, pageSize)
	if err != nil {
		if isMemoryServiceNotFound(err) {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, err.Error())
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Internal server error")
		return
	}

	common.SuccessWithData(c, data, true)
}

type messageMemoryIDs []string

func (ids *messageMemoryIDs) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if strings.TrimSpace(single) != "" {
			*ids = []string{single}
		}
		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*ids = many
	return nil
}

type AddMessageRequest struct {
	MemoryIDs     messageMemoryIDs `json:"memory_id" binding:"required"`
	AgentID       string           `json:"agent_id" binding:"required"`
	SessionID     string           `json:"session_id" binding:"required"`
	UserInput     string           `json:"user_input" binding:"required"`
	AgentResponse string           `json:"agent_response" binding:"required"`
	UserID        string           `json:"user_id"`
}

// AddMessage handles POST request for adding messages
// API Path: POST /api/v1/messages
//
// Function:
//   - Adds messages to one or more memories
//   - Messages will be embedded and saved to vector database
//   - Creates asynchronous task for processing
//
// Request Parameters (JSON Body):
//   - memory_id (required): Memory ID or ID array
//   - agent_id (required): Agent ID
//   - session_id (required): Session ID
//   - user_input (required): User input
//   - agent_response (required): Agent response
//   - user_id (optional): User ID
func (h *MemoryHandler) AddMessage(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	currentUserID := strings.TrimSpace(user.ID)
	if currentUserID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "user_id is required")
		return
	}

	var reqBody AddMessageRequest
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "body arguments is required")
		return
	}
	if len(reqBody.MemoryIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory_id is required")
		return
	}

	effectiveUserID := currentUserID
	if v, ok := c.Get("auth_via_api_token"); ok {
		if authViaAPIToken, ok := v.(bool); authViaAPIToken && ok {
			effectiveUserID = strings.TrimSpace(reqBody.UserID)
			if effectiveUserID == "" {
				common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
					"user_id is required")
				return
			}
		}
	}

	msg := service.MemoryMessage{
		UserID:        effectiveUserID,
		AgentID:       reqBody.AgentID,
		SessionID:     reqBody.SessionID,
		UserInput:     reqBody.UserInput,
		AgentResponse: reqBody.AgentResponse,
	}

	ok, message, err := h.memoryService.AddMessage(c.Request.Context(), currentUserID, []string(reqBody.MemoryIDs), msg)
	if err != nil || !ok {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Some messages failed to add. Detail:"+message)
		return
	}

	common.SuccessNoData(c, message)
}

// ForgetMessage handles DELETE request for forgetting messages.
// API Path: DELETE /api/v1/messages/{memory_id}:{message_id}
//
// Function:
//   - Soft-deletes the specified message (sets forget_at timestamp)
//   - Message is not immediately deleted from database, but marked as "forgotten"
//
// Parameter Format:
//   - memory_id: Memory ID
//   - message_id: Message ID (integer)
func (h *MemoryHandler) ForgetMessage(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	memoryID, messageID, err := parseMemoryMessagePath(c.Param("memory_message"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	if err = h.memoryService.ForgetMessage(c.Request.Context(), user.ID, memoryID, messageID); err != nil {
		errMsg := err.Error()
		if isMemoryServiceNotFound(err) {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, errMsg)
			return
		}

		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Internal server error:"+errMsg)
		return
	}

	common.SuccessNoData(c, true)
}

func isMemoryServiceNotFound(err error) bool {
	var notFoundErr *service.ResourceNotFoundError
	return errors.As(err, &notFoundErr) && notFoundErr.Resource == "Memory"
}

func parseMemoryMessagePath(memoryMessage string) (string, int64, error) {
	memoryMessage = strings.TrimSpace(memoryMessage)
	if memoryMessage == "" {
		return "", 0, errors.New("memory_id and message_id are required")
	}

	parts := strings.Split(memoryMessage, ":")
	if len(parts) != 2 {
		return "", 0, errors.New("message path must be formatted as memory_id:message_id")
	}

	memoryID := strings.TrimSpace(parts[0])
	messageIDText := strings.TrimSpace(parts[1])
	if memoryID == "" {
		return "", 0, errors.New("memory_id is required")
	}
	if messageIDText == "" {
		return "", 0, errors.New("message_id is required")
	}

	messageID, err := strconv.ParseInt(messageIDText, 10, 64)
	if err != nil || messageID < 0 {
		return "", 0, errors.New("message_id must be a non-negative integer")
	}

	return memoryID, messageID, nil
}

// UpdateMessage handles PUT request for updating message status
// API Path: PUT /api/v1/messages/:memory_id/:message_id
//
// Function:
//   - Updates status of the specified message
//   - status is a boolean, converted to integer for storage (true=1, false=0)
//
// Request Parameters (JSON Body):
//   - status (required): Message status, boolean
func (h *MemoryHandler) UpdateMessage(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "user id is required")
		return
	}

	memoryID, messageID, err := parseMemoryMessagePath(c.Param("memory_message"))
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeArgumentError), err.Error())
		return
	}

	var req map[string]interface{}
	if err = json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	status, ok := req["status"].(bool)
	if !ok {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Status must be a boolean.")
		return
	}

	ok, err = h.memoryService.UpdateMessageStatus(c.Request.Context(), userID, memoryID, messageID, status)
	if err != nil || !ok {
		if isMemoryServiceNotFound(err) {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, err.Error())
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Internal server error:"+err.Error())
		return
	}

	common.SuccessNoData(c, true)
}

// GetMessageContent handles GET request for getting message content
// API Path: GET /api/v1/messages/:memory_id/:message_id/content
//
// Function:
//   - Gets complete content of the specified message
//   - doc_id format: memory_id + "_" + message_id
//
// Parameter Format:
//   - memory_id: Memory ID
//   - message_id: Message ID (integer)
//

func (h *MemoryHandler) GetMessageContent(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "user id is required")
		return
	}

	memoryID, messageID, err := parseMemoryMessagePath(c.Param("memory_message"))
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeArgumentError), err.Error())
		return
	}

	data, err := h.memoryService.GetMessageContent(c.Request.Context(), userID, memoryID, messageID)
	if err != nil {
		if _, ok := err.(*service.ResourceNotFoundError); ok {
			common.ResponseWithCodeData(c, common.CodeNotFound, nil, err.Error())
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, data, true)
}

// SearchMessage handles GET request for searching messages
// API Path: GET /api/v1/messages/search
//
// Function:
//   - Searches messages across multiple memories
//   - Supports vector similarity search and keyword search
//   - Fuses results from both search methods
//
// Query Parameters:
//   - memory_id (optional): Memory ID list, supports comma separation
//   - query (optional): Search query text
//   - similarity_threshold (optional): Similarity threshold, default 0.2
//   - keywords_similarity_weight (optional): Keyword weight, default 0.7
//   - top_n (optional): Number of results to return, default 5
//   - agent_id (optional): Agent ID filter
//   - session_id (optional): Session ID filter
//   - user_id (optional): User ID filter
func (h *MemoryHandler) SearchMessage(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "user id is required")
		return
	}

	var memoryIDs []string
	values := c.QueryArray("memory_id")
	for _, v := range values {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				memoryIDs = append(memoryIDs, p)
			}
		}
	}

	query := c.Query("query")

	similarityThreshold, _ := strconv.ParseFloat(c.DefaultQuery("similarity_threshold", "0.2"), 64)
	keywordsSimilarityWeight, _ := strconv.ParseFloat(c.DefaultQuery("keywords_similarity_weight", "0.7"), 64)
	topN, _ := strconv.Atoi(c.DefaultQuery("top_n", "5"))

	agentID := c.DefaultQuery("agent_id", "")
	sessionID := c.DefaultQuery("session_id", "")

	filterDict := map[string]interface{}{
		"memory_id":  memoryIDs,
		"agent_id":   agentID,
		"session_id": sessionID,
		"user_id":    c.DefaultQuery("user_id", ""),
	}

	params := map[string]interface{}{
		"query":                      query,
		"similarity_threshold":       similarityThreshold,
		"keywords_similarity_weight": keywordsSimilarityWeight,
		"top_n":                      topN,
	}

	res, code, err := h.memoryService.SearchMessage(c.Request.Context(), userID, filterDict, params)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, res, true)
}

// GetMessages handles GET request for getting message list
// API Path: GET /api/v1/messages
//
// Function:
//   - Gets recent messages from specified memories
//   - Supports filtering by agent_id and session_id
//
// Query Parameters:
//   - memory_id (required): Memory ID list, supports comma separation
//   - agent_id (optional): Agent ID filter
//   - session_id (optional): Session ID filter
//   - limit (optional): Number of results to return, default 10
func (h *MemoryHandler) GetMessages(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "user id is required")
		return
	}

	var memoryIDs []string
	values := c.QueryArray("memory_id")
	for _, v := range values {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				memoryIDs = append(memoryIDs, p)
			}
		}
	}

	agentID := c.DefaultQuery("agent_id", "")
	sessionID := c.DefaultQuery("session_id", "")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if len(memoryIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "memory_ids is required.")
		return
	}

	data, code, err := h.memoryService.GetMessages(c.Request.Context(), memoryIDs, userID, agentID, sessionID, limit)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, data, true)
}

// isArgumentError determines if an error message is an argument error
//
// Function:
//   - Checks if the error message contains any argument validation-related prefixes
//   - Used to distinguish argument errors from server errors
//
// Parameters:
//   - msg: Error message string
//
// Returns:
//   - bool: true if it's an argument error, false otherwise
func isArgumentError(msg string) bool {
	// Define list of argument error prefixes
	// Matches Python ArgumentException error messages
	argumentErrorPrefixes := []string{
		"memory name cannot be empty",  // Memory name cannot be empty
		"memory name exceeds limit",    // Memory name exceeds limit
		"memory type must be a list",   // memory_type must be a list
		"memory type is not supported", // Unsupported memory_type
	}
	// Check if error message starts with any prefix
	for _, prefix := range argumentErrorPrefixes {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
