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
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse JSON request body
	var req service.CreateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	// Validate required field: name
	if req.Name == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "name is required",
			"data":    nil,
		})
		return
	}

	// Validate required field: memory_type (must be non-empty array)
	if len(req.MemoryType) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "memory_type is required and must be a list",
			"data":    nil,
		})
		return
	}

	// Validate required field: embd_id
	if req.EmbdID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "embd_id is required",
			"data":    nil,
		})
		return
	}

	// Validate required field: llm_id
	if req.LLMID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "llm_id is required",
			"data":    nil,
		})
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
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeArgumentError,
				"message": errMsg,
				"data":    nil,
			})
			return
		}

		// Other errors return server error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": errMsg,
			"data":    nil,
		})
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
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": true,
		"data":    result,
	})
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
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "memory_id is required",
			"data":    nil,
		})
		return
	}

	// Get current logged-in user information
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse JSON request body
	var req service.UpdateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	// Call service layer to update memory
	result, err := h.memoryService.UpdateMemory(userID, memoryID, &req)
	if err != nil {
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeNotFound,
				"message": errMsg,
				"data":    nil,
			})
			return
		}

		// Check if it's an argument error
		if isArgumentError(errMsg) {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeArgumentError,
				"message": errMsg,
				"data":    nil,
			})
			return
		}

		// Other errors return server error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": errMsg,
			"data":    nil,
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": true,
		"data":    result,
	})
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
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "memory_id is required",
			"data":    nil,
		})
		return
	}

	// Call service layer to delete memory
	err := h.memoryService.DeleteMemory(memoryID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeNotFound,
				"message": errMsg,
				"data":    nil,
			})
			return
		}

		// Other errors return server error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": errMsg,
			"data":    nil,
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": true,
		"data":    nil,
	})
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
		jsonError(c, errorCode, errorMessage)
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
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": true,
		"data":    result,
	})
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
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "memory_id is required",
			"data":    nil,
		})
		return
	}

	// Call service layer to get memory configuration
	result, err := h.memoryService.GetMemoryConfig(memoryID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's a "not found" error
		if strings.Contains(errMsg, "not found") {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeNotFound,
				"message": errMsg,
				"data":    nil,
			})
			return
		}

		// Other errors return server error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": errMsg,
			"data":    nil,
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": true,
		"data":    result,
	})
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
//
// TODO: Implementation pending - depends on CanvasService and TaskService
func (h *MemoryHandler) GetMemoryMessages(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "GetMemoryMessages not implemented - pending CanvasService and TaskService dependencies",
		"data":    nil,
	})
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
//
// TODO: Implementation pending - depends on embedding engine
func (h *MemoryHandler) AddMessage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "AddMessage not implemented - pending embedding engine dependency",
		"data":    nil,
	})
}

// ForgetMessage handles DELETE request for forgetting messages
// API Path: DELETE /api/v1/messages/:memory_id/:message_id
//
// Function:
//   - Soft-deletes the specified message (sets forget_at timestamp)
//   - Message is not immediately deleted from database, but marked as "forgotten"
//
// Parameter Format:
//   - memory_id: Memory ID
//   - message_id: Message ID (integer)
//
// TODO: Implementation pending - depends on embedding engine
func (h *MemoryHandler) ForgetMessage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "ForgetMessage not implemented - pending embedding engine dependency",
		"data":    nil,
	})
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
//
// TODO: Implementation pending - depends on embedding engine
func (h *MemoryHandler) UpdateMessage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "UpdateMessage not implemented - pending embedding engine dependency",
		"data":    nil,
	})
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
//
// TODO: Implementation pending - depends on embedding engine
func (h *MemoryHandler) SearchMessage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "SearchMessage not implemented - pending embedding engine dependency",
		"data":    nil,
	})
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
//
// TODO: Implementation pending - depends on embedding engine
func (h *MemoryHandler) GetMessages(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "GetMessages not implemented - pending embedding engine dependency",
		"data":    nil,
	})
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
// TODO: Implementation pending - depends on embedding engine
func (h *MemoryHandler) GetMessageContent(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeServerError,
		"message": "GetMessageContent not implemented - pending embedding engine dependency",
		"data":    nil,
	})
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
