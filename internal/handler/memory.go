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
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
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
//   - message: "success" on success, error message on failure
//   - data: Memory object on success
//
// Business Logic:
//  1. Apply billing and application quotas
//  2. Parse and validate request parameters
//  3. Call service layer to create memory
//  4. Return creation result
func (h *MemoryHandler) CreateMemory(c *gin.Context) {
	user, errorCode, err := billingQuotaUser(c)
	if err != nil {
		common.RespondError(c, errorCode, err.Error())
		return
	}
	if !billingEnforceApps(c, user.ID) {
		return
	}

	// Parse JSON request body
	var req service.CreateMemoryRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	// Validate required field: name
	if strings.TrimSpace(req.Name) == "" {
		common.RespondError(c, common.CodeArgumentError, "name is required")
		return
	}

	// Validate required field: memory_type (must be non-empty array)
	if len(req.MemoryType) == 0 {
		common.RespondError(c, common.CodeArgumentError, "memory type is required and must be a list")
		return
	}

	// Validate required field: embd_id
	if req.EmbdID == "" {
		common.RespondError(c, common.CodeArgumentError, "embedding model ID is required")
		return
	}

	// Validate required field: llm_id
	if req.LLMID == "" {
		common.RespondError(c, common.CodeArgumentError, "language model ID is required")
		return
	}

	// Record request parsing completion time (for timing)
	ctx := c.Request.Context()

	// Call service layer to create memory
	result, err := h.memoryService.CreateMemory(ctx, &req)
	if err != nil {
		errMsg := err.Error()
		// Determine if it's an argument error and return appropriate error code
		if isArgumentError(errMsg) {
			common.RespondError(c, common.CodeArgumentError, errMsg)
			return
		}

		// Other errors return server error
		common.RespondError(c, common.CodeServerError, errMsg)
		return
	}

	// Return success response
	common.RespondSuccess(c, result, "success")
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
		common.RespondError(c, common.CodeArgumentError, "memory ID is required")
		return
	}

	// Parse JSON request body
	var req service.UpdateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()

	// Call service layer to update memory
	result, err := h.memoryService.UpdateMemory(ctx, memoryID, &req)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			common.RespondError(c, common.CodeNotFound, errMsg)
			return
		}

		// Check if it's an argument error
		if isArgumentError(errMsg) {
			common.RespondError(c, common.CodeArgumentError, errMsg)
			return
		}

		// Other errors return server error
		common.RespondError(c, common.CodeServerError, errMsg)
		return
	}

	// Return success response
	common.RespondSuccess(c, result, "success")
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
		common.RespondError(c, common.CodeArgumentError, "memory ID is required")
		return
	}

	ctx := c.Request.Context()

	// Call service layer to delete memory (with access control)
	err := h.memoryService.DeleteMemory(ctx, memoryID)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			common.RespondError(c, common.CodeNotFound, errMsg)
			return
		}

		// Other errors return server error
		common.RespondError(c, common.CodeServerError, errMsg)
		return
	}

	// Return success response
	common.RespondSuccess(c, nil, "success")
}

// splitQueryValues returns every value sent under the repeated query key,
// splitting each occurrence on commas, trimming whitespace and dropping empty
// entries. It returns nil when the key carries no values.
func splitQueryValues(c *gin.Context, key string) []string {
	var values []string
	for _, item := range c.QueryArray(key) {
		for _, value := range strings.Split(item, ",") {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				values = append(values, trimmed)
			}
		}
	}
	return values
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
//   - memory_type (optional): Memory type filter, supports repeated keys and comma-separated multiple types
//   - tenant_id (optional): Tenant ID filter, supports repeated keys and comma-separated multiple IDs
//   - storage_type (optional): Storage type filter
//   - keywords (optional): Keyword search (fuzzy match on name)
//   - page (optional): Page number, default 1
//   - page_size (optional): Items per page, default 30
//
// Response Format:
//   - code: Status code
//   - message: "success"
//   - data.memory_list: Array of Memory objects
//   - data.total_count: Total record count
func (h *MemoryHandler) ListMemories(c *gin.Context) {
	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	if c.Query("type") == "filter" {
		result, err := h.memoryService.ListMemoryFilters(c.Request.Context())
		if err != nil {
			common.RespondError(c, common.CodeServerError, err.Error())
			return
		}
		common.RespondSuccess(c, result, "success")
		return
	}

	// Parse query parameters
	storageType := c.Query("storage_type")
	keywords := c.Query("keywords")

	// Parse memory_type parameter (repeated query keys and comma separation)
	memoryTypes := splitQueryValues(c, "memory_type")

	// Parse tenant_id parameter (repeated query keys and comma separation)
	// If not specified, service will get all tenants associated with the user
	tenantIDs := splitQueryValues(c, "tenant_id")

	ctx := c.Request.Context()

	// Call service layer to get memory list
	result, err := h.memoryService.ListMemories(ctx, tenantIDs, memoryTypes, storageType, keywords, page, pageSize, c.Query("canvas_id"))
	if err != nil {
		code := common.CodeServerError
		if errors.Is(err, permission.ErrUnauthenticated) || errors.Is(err, permission.ErrPermissionDenied) ||
			errors.Is(err, permission.ErrMembershipNotFound) || errors.Is(err, permission.ErrResourceNotFound) {
			code, err = permissionresponse.Normalize(err)
		}
		common.RespondError(c, code, err.Error())
		return
	}

	// Return success response
	common.RespondSuccess(c, result, "success")
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
//   - message: "success"
//   - data: Memory object, including owner_name field
func (h *MemoryHandler) GetMemoryConfig(c *gin.Context) {
	// Get memory_id from URL path
	memoryID := c.Param("memory_id")
	if memoryID == "" {
		common.RespondError(c, common.CodeArgumentError, "memory ID is required")
		return
	}

	ctx := c.Request.Context()

	// Call service layer to get memory configuration (with access control)
	result, err := h.memoryService.GetMemoryConfig(ctx, memoryID)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			common.RespondError(c, common.CodeNotFound, errMsg)
			return
		}

		// Other errors return server error
		common.RespondError(c, common.CodeServerError, errMsg)
		return
	}

	// Return success response
	common.RespondSuccess(c, result, "success")
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
//   - page_size (optional): Items per page, default 30
//
// Response Format:
//   - code: Status code
//   - message: "success"
//   - data.messages: Array of message objects
//   - data.storage_type: Storage type
func (h *MemoryHandler) GetMemoryMessages(c *gin.Context) {

	memoryID := strings.TrimSpace(c.Param("memory_id"))
	if memoryID == "" {
		common.RespondError(c, common.CodeArgumentError, "memory_id is required")
		return
	}

	agentIDs := splitQueryValues(c, "agent_id")

	keywords := strings.TrimSpace(c.DefaultQuery("keywords", ""))
	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	data, err := h.memoryService.GetMemoryMessages(c.Request.Context(), memoryID, agentIDs, keywords, page, pageSize)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		if isMemoryServiceNotFound(err) {
			common.RespondError(c, common.CodeNotFound, err.Error())
			return
		}
		common.RespondError(c, common.CodeServerError, "Internal server error")
		return
	}

	common.RespondSuccess(c, data, "success")
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
	currentUserID := strings.TrimSpace(permission.UserIDFromContext(c.Request.Context()))

	var reqBody AddMessageRequest
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		common.RespondError(c, common.CodeArgumentError, "body arguments is required")
		return
	}
	if len(reqBody.MemoryIDs) == 0 {
		common.RespondError(c, common.CodeArgumentError, "memory_id is required")
		return
	}

	effectiveUserID := currentUserID
	if v, ok := c.Get("auth_via_api_token"); ok {
		if authViaAPIToken, ok := v.(bool); authViaAPIToken && ok {
			effectiveUserID = strings.TrimSpace(reqBody.UserID)
			if effectiveUserID == "" {
				common.RespondError(c, common.CodeArgumentError, "user_id is required")
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

	ok, message, err := h.memoryService.AddMessage(c.Request.Context(), []string(reqBody.MemoryIDs), msg)
	if err != nil || !ok {
		common.RespondError(c, common.CodeServerError, "Some messages failed to add. Detail:"+message)
		return
	}

	common.RespondSuccess(c, nil, message)
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

	memoryID, messageID, err := parseMemoryMessagePath(c.Param("memory_message"))
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	if err = h.memoryService.ForgetMessage(c.Request.Context(), memoryID, messageID); err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		errMsg := err.Error()
		if isMemoryServiceNotFound(err) {
			common.RespondError(c, common.CodeNotFound, errMsg)
			return
		}

		common.RespondError(c, common.CodeServerError, "Internal server error:"+errMsg)
		return
	}

	common.RespondSuccess(c, nil, "success")
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

	memoryID, messageID, err := parseMemoryMessagePath(c.Param("memory_message"))
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	var req map[string]interface{}
	if err = json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	status, ok := req["status"].(bool)
	if !ok {
		common.RespondError(c, common.CodeArgumentError, "Status must be a boolean.")
		return
	}

	ok, err = h.memoryService.UpdateMessageStatus(c.Request.Context(), memoryID, messageID, status)
	if err != nil || !ok {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		if isMemoryServiceNotFound(err) {
			common.RespondError(c, common.CodeNotFound, err.Error())
			return
		}
		common.RespondError(c, common.CodeServerError, "Internal server error:"+err.Error())
		return
	}

	common.RespondSuccess(c, nil, "success")
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

	memoryID, messageID, err := parseMemoryMessagePath(c.Param("memory_message"))
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	data, err := h.memoryService.GetMessageContent(c.Request.Context(), memoryID, messageID)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		if _, ok := err.(*service.ResourceNotFoundError); ok {
			common.RespondError(c, common.CodeNotFound, err.Error())
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}

	common.RespondSuccess(c, data, "success")
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

	memoryIDs := splitQueryValues(c, "memory_id")

	query := c.Query("query")

	similarityThreshold, err := strconv.ParseFloat(c.DefaultQuery("similarity_threshold", "0.2"), 64)
	if err != nil || math.IsNaN(similarityThreshold) || math.IsInf(similarityThreshold, 0) || similarityThreshold < 0 || similarityThreshold > 1 {
		common.RespondError(c, common.CodeArgumentError, "similarity_threshold must be a finite number between 0 and 1")
		return
	}
	keywordsSimilarityWeight, err := strconv.ParseFloat(c.DefaultQuery("keywords_similarity_weight", "0.7"), 64)
	if err != nil || math.IsNaN(keywordsSimilarityWeight) || math.IsInf(keywordsSimilarityWeight, 0) || keywordsSimilarityWeight < 0 || keywordsSimilarityWeight > 1 {
		common.RespondError(c, common.CodeArgumentError, "keywords_similarity_weight must be a finite number between 0 and 1")
		return
	}
	topN, _ := strconv.Atoi(c.DefaultQuery("top_n", "5"))
	if topN <= 0 || topN > 100 {
		common.RespondError(c, common.CodeArgumentError, "top_n must be between 1 and 100")
		return
	}

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

	res, code, err := h.memoryService.SearchMessage(c.Request.Context(), filterDict, params)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccessList(c, res, "success")
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

	memoryIDs := splitQueryValues(c, "memory_id")

	agentID := c.DefaultQuery("agent_id", "")
	sessionID := c.DefaultQuery("session_id", "")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		common.RespondError(c, common.CodeArgumentError, "limit must be between 1 and 100")
		return
	}
	if len(memoryIDs) == 0 {
		common.RespondError(c, common.CodeArgumentError, "memory_ids is required.")
		return
	}

	data, code, err := h.memoryService.GetMessages(c.Request.Context(), memoryIDs, agentID, sessionID, limit)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccessList(c, data, "success")
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
		"memory name cannot be empty",  // Python: "Memory name cannot be empty or whitespace."
		"name cannot be empty",         // ValidateName trimmed-empty case
		"memory name exceeds limit",    // Memory name exceeds limit
		"name length is",               // ValidateName exceeds limit
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
