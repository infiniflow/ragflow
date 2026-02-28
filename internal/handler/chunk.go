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
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// ChunkHandler chunk handler
type ChunkHandler struct {
	chunkService *service.ChunkService
	userService  *service.UserService
}

// NewChunkHandler create chunk handler
func NewChunkHandler(chunkService *service.ChunkService, userService *service.UserService) *ChunkHandler {
	return &ChunkHandler{
		chunkService: chunkService,
		userService:  userService,
	}
}

// RetrievalTest performs retrieval test for chunks
// @Summary Retrieval Test
// @Description Test retrieval of chunks based on question and knowledge base
// @Tags chunks
// @Accept json
// @Produce json
// @Param request body service.RetrievalTestRequest true "retrieval test parameters"
// @Success 200 {object} map[string]interface{}
// @Router /v1/chunk/retrieval_test [post]
func (h *ChunkHandler) RetrievalTest(c *gin.Context) {
	// Extract access token from Authorization header
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

	// Bind JSON request
	var req service.RetrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Set default values for optional parameters
	if req.Page == nil {
		defaultPage := 1
		req.Page = &defaultPage
	}
	if req.Size == nil {
		defaultSize := 30
		req.Size = &defaultSize
	}
	if req.TopK == nil {
		defaultTopK := 1024
		req.TopK = &defaultTopK
	}
	if req.UseKG == nil {
		defaultUseKG := false
		req.UseKG = &defaultUseKG
	}

	// Validate required fields
	if req.Question == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "question is required",
		})
		return
	}
	if req.KbID == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "kb_id is required",
		})
		return
	}

	// Validate kb_id type: string or []string
	switch v := req.KbID.(type) {
	case string:
		if v == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "kb_id cannot be empty string",
			})
			return
		}
	case []interface{}:
		// Convert to []string
		var kbIDs []string
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				kbIDs = append(kbIDs, str)
			} else {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    400,
					"message": "kb_id array must contain non-empty strings",
				})
				return
			}
		}
		if len(kbIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "kb_id array cannot be empty",
			})
			return
		}
		// Convert back to interface{} for service
		req.KbID = kbIDs
	case []string:
		// Already correct type
		if len(v) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "kb_id array cannot be empty",
			})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "kb_id must be string or array of strings",
		})
		return
	}

	// Call service with user ID for permission checks
	resp, err := h.chunkService.RetrievalTest(&req, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    resp,
		"message": "success",
	})
}
