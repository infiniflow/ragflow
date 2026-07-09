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
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/service"
)

// chunkService is the consumer-side interface for ChunkHandler's service dependency.
type chunkService interface {
	RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
	Get(req *service.GetChunkRequest, userID string) (*service.GetChunkResponse, error)
	List(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error)
	SwitchChunks(userID, datasetID, documentID string, availableInt int, chunkIDs []string) error
	UpdateChunk(req *service.UpdateChunkRequest, userID string) error
	RemoveChunks(req *service.RemoveChunksRequest, userID string) (int64, error)
	Parse(userID, datasetID string, req *service.ParseFileRequest) (map[string]interface{}, common.ErrorCode, error)
	AddChunk(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error)
	StopParsing(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error)
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Bind JSON request
	var req service.RetrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
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
		common.SuccessWithData(c, &service.RetrievalTestResponse{
			Chunks:  []map[string]interface{}{},
			DocAggs: []map[string]interface{}{},
			Total:   0,
		}, "success")
		return
	}

	// Validate required fields
	if req.Datasets == nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_id is required")
		return
	}

	if len(req.Datasets) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_id array cannot be empty")
		return
	}
	if req.TopK != nil && *req.TopK <= 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "top_k must be greater than 0")
		return
	}

	// Call service with user ID for permission checks
	resp, err := h.chunkService.RetrievalTest(&req, user.ID)
	if err != nil {
		common.Warn("dataset search failed", zap.String("error", err.Error()))
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeServerError, nil, "dataset search failed")
		return
	}

	common.SuccessWithData(c, resp, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	chunkID := c.Param("chunk_id")
	if chunkID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "chunk_id is required")
		return
	}

	req := &service.GetChunkRequest{
		ChunkID: chunkID,
	}

	resp, err := h.chunkService.Get(req, user.ID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithData(c, resp.Chunk, "success")
}

// Parse reparse the datasets' files
func (h *ChunkHandler) Parse(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "user_id is required")
		return
	}
	datasetId := strings.TrimSpace(c.Param("dataset_id"))
	if datasetId == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "dataset_id is required")
		return
	}

	var req service.ParseFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	data, code, err := h.chunkService.Parse(userID, datasetId, &req)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, err.Error())
		return
	}

	common.ResponseWithCodeData(c, code, data, "success")
}

// ListChunks retrieves chunks for a document from path/query parameters.
func (h *ChunkHandler) ListChunks(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	if datasetID == "" || documentID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "dataset_id and document_id are required")
		return
	}

	page, err := parsePositiveQueryInt(c, "page", 1)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}
	size, err := parsePositiveQueryInt(c, "page_size", 30)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	req := service.ListChunksRequest{
		DatasetID: datasetID,
		DocID:     documentID,
		Page:      &page,
		Size:      &size,
		Keywords:  c.Query("keywords"),
	}
	available, ok, err := parseAvailableQuery(c.Query("available"))
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}
	if ok {
		req.AvailableInt = &available
	}

	resp, err := h.chunkService.List(&req, user.ID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeServerError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, resp, "success")
}

func parsePositiveQueryInt(c *gin.Context, name string, defaultValue int) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func parseAvailableQuery(raw string) (int, bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return 0, false, nil
	case "true", "1":
		return 1, true, nil
	case "false", "0":
		return 0, true, nil
	default:
		return 0, true, fmt.Errorf("available must be one of: true, false, 1, 0")
	}
}

func (h *ChunkHandler) StopParsing(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	var req service.StopParsingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	if len(req.DocumentIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "`document_ids` is required")
		return
	}

	resp, code, err := h.chunkService.StopParsing(user.ID, datasetID, req)
	if err != nil {
		var data interface{}
		if resp != nil {
			data = resp.Data
		}
		common.ResponseWithCodeData(c, code, data, err.Error())
		return
	}

	message := "success"
	var data interface{}
	if resp != nil {
		if resp.Message != "" {
			message = resp.Message
		}
		data = resp.Data
	}

	common.SuccessWithData(c, data, message)
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Bind JSON request
	var req service.ListChunksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
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
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithData(c, resp, "success")
}

// SwitchChunks enable or disable a chunk
func (h *ChunkHandler) SwitchChunks(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeAuthenticationError, nil, "user_id is required")
		return
	}

	// Get required ID
	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "dataset_id is required")
		return
	}

	documentID := strings.TrimSpace(c.Param("document_id"))
	if documentID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "document_id is required")
		return
	}

	var rawBody map[string]interface{}
	if err := json.NewDecoder(c.Request.Body).Decode(&rawBody); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	chunkIDs, ok := parseStringSlice(rawBody["chunk_ids"])
	if !ok || len(chunkIDs) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "`chunk_ids` is required.")
		return
	}

	if rawBody["available_int"] == nil && rawBody["available"] == nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "`available_int` or `available` is required.")
		return
	}

	availableInt, err := parseAvailableBody(rawBody)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	if err = h.chunkService.SwitchChunks(userID, datasetID, documentID, availableInt, chunkIDs); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeServerError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, true, "success")
}

func parseStringSlice(raw interface{}) ([]string, bool) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func parseAvailableBody(rawBody map[string]interface{}) (int, error) {
	if raw, ok := rawBody["available_int"]; ok {
		switch v := raw.(type) {
		case float64:
			return int(v), nil
		case int:
			return v, nil
		case bool:
			if v {
				return 1, nil
			}
			return 0, nil
		default:
			return 0, fmt.Errorf("available_int must be an integer")
		}
	}
	if raw, ok := rawBody["available"]; ok {
		switch v := raw.(type) {
		case bool:
			if v {
				return 1, nil
			}
			return 0, nil
		case float64:
			if v != 0 {
				return 1, nil
			}
			return 0, nil
		default:
			return 0, fmt.Errorf("available must be a boolean")
		}
	}
	return 0, fmt.Errorf("`available_int` or `available` is required.")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Validate allowed update fields and get IDs from body
	var rawBody map[string]interface{}
	if err := json.NewDecoder(c.Request.Body).Decode(&rawBody); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "invalid JSON body: "+err.Error())
		return
	}

	// Get required ID fields
	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "dataset_id is required")
		return
	}

	chunkID := strings.TrimSpace(c.Param("chunk_id"))
	if chunkID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "chunk_id is required")
		return
	}

	// Get document_id from request
	documentID := strings.TrimSpace(c.Param("document_id"))
	if documentID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "document_id is required")
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
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Update field '"+field+"' is not supported. Updatable fields: content, important_keywords, questions, available, positions, tag_kwd, tag_feas")
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
		var coded interface {
			Code() common.ErrorCode
		}
		if errors.As(err, &coded) {
			switch coded.Code() {
			case common.CodeArgumentError, common.CodeBadRequest, common.CodeDataError:
				common.ResponseWithHttpCodeData(c, http.StatusBadRequest, coded.Code(), nil, err.Error())
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    coded.Code(),
					"message": err.Error(),
				})
				return
			}
		}

		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithMessage(c, "chunk updated successfully")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Get document_id from URL path
	docID := c.Param("document_id")
	if docID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "document_id is required")
		return
	}

	var req service.RemoveChunksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
		return
	}

	req.DocID = docID

	if req.DocID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "doc_id is required")
		return
	}

	deletedCount, err := h.chunkService.RemoveChunks(&req, user.ID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithData(c, deletedCount, "success")
}

func addChunkStringField(rawBody map[string]json.RawMessage, field string) (string, error) {
	raw, ok := rawBody[field]
	if !ok {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("`%s` must be a string", field)
	}
	return value, nil
}

func addChunkStringPtrField(rawBody map[string]json.RawMessage, field string) (*string, error) {
	raw, ok := rawBody[field]
	if !ok {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("`%s` must be a string", field)
	}
	return &value, nil
}

func addChunkStringListField(rawBody map[string]json.RawMessage, field, listMessage, elementMessage string) ([]string, error) {
	raw, ok := rawBody[field]
	if !ok {
		return nil, nil
	}
	var values []interface{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, errors.New(listMessage)
	}
	result := make([]string, len(values))
	for i, value := range values {
		str, ok := value.(string)
		if !ok {
			return nil, errors.New(elementMessage)
		}
		result[i] = str
	}
	return result, nil
}

func addChunkResponseMessage(code common.ErrorCode, err error) string {
	if code == common.CodeServerError {
		common.Warn("add chunk failed", zap.String("error", err.Error()))
		return "Failed to add chunk"
	}
	return err.Error()
}

func (h *ChunkHandler) AddChunk(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := user.ID
	datasetID, documentID := strings.TrimSpace(c.Param("dataset_id")), strings.TrimSpace(c.Param("document_id"))

	var rawBody map[string]json.RawMessage
	if err := json.NewDecoder(c.Request.Body).Decode(&rawBody); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	content, err := addChunkStringField(rawBody, "content")
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	importantKeywords, err := addChunkStringListField(rawBody, "important_keywords", "`important_keywords` is required to be a list", "`important_keywords` must be a list of strings")
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	questions, err := addChunkStringListField(rawBody, "questions", "`questions` is required to be a list", "`questions` must be a list of strings")
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	tagKwd, err := addChunkStringListField(rawBody, "tag_kwd", "`tag_kwd` is required to be a list", "`tag_kwd` must be a list of strings")
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	imageBase64, err := addChunkStringPtrField(rawBody, "image_base64")
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	var tagFeas interface{}
	if raw, ok := rawBody["tag_feas"]; ok {
		if err = json.Unmarshal(raw, &tagFeas); err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
			return
		}
	}

	req := service.AddChunkRequest{
		DatasetID:         datasetID,
		DocumentID:        documentID,
		Content:           content,
		ImportantKeywords: importantKeywords,
		Questions:         questions,
		TagKwd:            tagKwd,
		TagFeas:           tagFeas,
		ImageBase64:       imageBase64,
	}

	resp, err := h.chunkService.AddChunk(&req, userID)
	if err != nil {
		var codedErr service.ErrorCoder
		if errors.As(err, &codedErr) {
			common.ResponseWithCodeData(c, codedErr.Code(), nil, addChunkResponseMessage(codedErr.Code(), err))
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, addChunkResponseMessage(common.CodeServerError, err))
		return
	}

	common.SuccessWithData(c, resp, "success")
}
