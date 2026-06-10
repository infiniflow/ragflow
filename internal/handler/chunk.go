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
	"net/http"
	"strings"
	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/service"
)

// chunkService is the consumer-side interface for ChunkHandler's service dependency.
type chunkService interface {
	RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
	Get(req *service.GetChunkRequest, userID string) (*service.GetChunkResponse, error)
	List(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error)
	UpdateChunk(req *service.UpdateChunkRequest, userID string) error
	RemoveChunks(req *service.RemoveChunksRequest, userID string) (int64, error)
}

// ChunkHandler chunk handler
type ChunkHandler struct {
	chunkService chunkService
	userService  *service.UserService
}

// NewChunkHandler create chunk handler
func NewChunkHandler(chunkService chunkService, userService *service.UserService) *ChunkHandler {
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
// @Router /api/v1/datasets/search [post]
func (h *ChunkHandler) RetrievalTest(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Bind JSON request
	var req service.RetrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
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

	// Strip and validate question.  Matching Python chunk_api.py which returns
	// an empty result for blank questions rather than an error.
	if strings.TrimSpace(req.Question) == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    int(common.CodeSuccess),
			"data": &service.RetrievalTestResponse{
				Chunks:  []map[string]interface{}{},
				DocAggs: []map[string]interface{}{},
				Total:   0,
			},
			"message": "success",
		})
		return
	}

	// Validate required fields
	if req.Datasets == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "kb_id is required",
		})
		return
	}

	if len(req.Datasets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "kb_id array cannot be empty",
		})
		return
	}
	if req.TopK != nil && *req.TopK <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "top_k must be greater than 0",
		})
		return
	}

	// Call service with user ID for permission checks
	resp, err := h.chunkService.RetrievalTest(&req, user.ID)
	if err != nil {
		common.Warn("dataset search failed", zap.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    common.CodeServerError,
			"data":    nil,
			"message": "dataset search failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    int(common.CodeSuccess),
		"data":    resp,
		"message": "success",
	})
}

// Get retrieves a chunk by ID.
// @Summary Get Chunk
// @Description Retrieve a single chunk by its ID.
// @Tags chunks
// @Accept json
// @Produce json
// @Param dataset_id path string true "Dataset ID"
// @Param document_id path string true "Document ID"
// @Param chunk_id path string true "Chunk ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id} [get]
func (h *ChunkHandler) Get(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	chunkID := c.Param("chunk_id")
	if chunkID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "chunk_id is required",
		})
		return
	}

	req := &service.GetChunkRequest{
		ChunkID: chunkID,
	}

	resp, err := h.chunkService.Get(req, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    resp.Chunk,
		"message": "success",
	})
}

// List retrieves chunks for a document.
// @Summary List Chunks
// @Description Retrieve paginated chunks for a document with optional filtering.
// @Tags chunks
// @Accept json
// @Produce json
// @Param request body service.ListChunksRequest true "List chunks parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chunk/list [post]
func (h *ChunkHandler) List(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Bind JSON request
	var req service.ListChunksRequest
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

	resp, err := h.chunkService.List(&req, user.ID)
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

// UpdateChunk updates a chunk
// @Summary Update Chunk
// @Description Update chunk fields
// @Tags chunks
// @Accept json
// @Produce json
// @Param request body service.UpdateChunkRequest true "update chunk"
// @Success 200 {object} map[string]interface{}
// @Router /v1/chunk/update [post]
func (h *ChunkHandler) UpdateChunk(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Validate allowed update fields and get IDs from body
	var rawBody map[string]interface{}
	if err := json.NewDecoder(c.Request.Body).Decode(&rawBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid JSON body: " + err.Error(),
		})
		return
	}

	// Get required ID fields
	datasetID, ok := rawBody["dataset_id"].(string)
	if !ok || datasetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "dataset_id is required",
		})
		return
	}
	chunkID, ok := rawBody["chunk_id"].(string)
	if !ok || chunkID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "chunk_id is required",
		})
		return
	}

	// Get document_id from request
	documentID, ok := rawBody["document_id"].(string)
	if !ok || documentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "doc_id is required",
		})
		return
	}

	// Allowed fields for update (exclude ID fields)
	allowedFields := map[string]bool{
		"content":            true,
		"important_keywords": true,
		"questions":          true,
		"available":          true,
		"positions":          true,
		"tag_kwd":            true,
		"tag_feas":           true,
	}
	for field := range rawBody {
		if field != "dataset_id" && field != "document_id" && field != "chunk_id" && !allowedFields[field] {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "Update field '" + field + "' is not supported. Updatable fields: content, important_keywords, questions, available, positions, tag_kwd, tag_feas",
			})
			return
		}
	}

	// Build UpdateChunkRequest from rawBody
	var req service.UpdateChunkRequest
	if content, ok := rawBody["content"].(string); ok {
		req.Content = &content
	}
	if importantKwd, ok := rawBody["important_keywords"].([]interface{}); ok {
		req.ImportantKwd = make([]string, len(importantKwd))
		for i, v := range importantKwd {
			if s, ok := v.(string); ok {
				req.ImportantKwd[i] = s
			}
		}
	}
	if questions, ok := rawBody["questions"].([]interface{}); ok {
		req.Questions = make([]string, len(questions))
		for i, v := range questions {
			if s, ok := v.(string); ok {
				req.Questions[i] = s
			}
		}
	}
	if available, ok := rawBody["available"].(bool); ok {
		req.Available = &available
	}
	if positions, ok := rawBody["positions"].([]interface{}); ok {
		req.Positions = positions
	}
	if tagKwd, ok := rawBody["tag_kwd"].([]interface{}); ok {
		req.TagKwd = make([]string, len(tagKwd))
		for i, v := range tagKwd {
			if s, ok := v.(string); ok {
				req.TagKwd[i] = s
			}
		}
	}
	req.TagFeas = rawBody["tag_feas"]

	// Set path parameters
	req.DatasetID = datasetID
	req.DocumentID = documentID
	req.ChunkID = chunkID

	err := h.chunkService.UpdateChunk(&req, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "chunk updated successfully",
	})
}

// RemoveChunks handles chunk removal requests
// @Summary Remove Chunks
// @Description Remove chunks from a document
// @Tags chunks
// @Accept json
// @Produce json
// @Param request body service.RemoveChunksRequest true "remove chunks request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/documents/{document_id}/chunks [delete]
func (h *ChunkHandler) RemoveChunks(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Get document_id from URL path
	docID := c.Param("document_id")
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "document_id is required",
		})
		return
	}

	var req service.RemoveChunksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	req.DocID = docID

	if req.DocID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "doc_id is required",
		})
		return
	}

	deletedCount, err := h.chunkService.RemoveChunks(&req, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    deletedCount,
		"message": "success",
	})
}
