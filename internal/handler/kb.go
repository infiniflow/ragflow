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
	"net/http"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/service"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// KnowledgebaseHandler handles knowledge base HTTP requests
type KnowledgebaseHandler struct {
	kbService       *service.KnowledgebaseService
	userService     *service.UserService
	documentService *service.DocumentService
}

// NewKnowledgebaseHandler creates a new knowledge base handler
func NewKnowledgebaseHandler(kbService *service.KnowledgebaseService, userService *service.UserService, documentService *service.DocumentService) *KnowledgebaseHandler {
	return &KnowledgebaseHandler{
		kbService:       kbService,
		userService:     userService,
		documentService: documentService,
	}
}

// jsonResponse sends a JSON response with code and message
func jsonResponse(c *gin.Context, code common.ErrorCode, data interface{}, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"data":    data,
		"message": message,
	})
}

// jsonError sends a JSON error response
func jsonError(c *gin.Context, code common.ErrorCode, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"data":    nil,
		"message": message,
	})
}

// HTTPError represents an HTTP error
type HTTPError struct {
	Code    common.ErrorCode
	Message string
}

// Error implements the error interface
func (e *HTTPError) Error() string {
	return e.Message
}

var (
	// ErrMissingAuth indicates missing authorization header
	ErrMissingAuth = &HTTPError{Code: common.CodeUnauthorized, Message: "Missing Authorization header"}
	// ErrInvalidToken indicates invalid access token
	ErrInvalidToken = &HTTPError{Code: common.CodeUnauthorized, Message: "Invalid access token"}
	ErrForbidden    = &HTTPError{Code: common.CodeForbidden, Message: "Forbidden user"}
)

// CreateKB handles the create knowledge base request
// @Summary Create Knowledge Base
// @Description Create a new knowledge base (dataset)
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.CreateKBRequest true "knowledge base info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/create [post]
func (h *KnowledgebaseHandler) CreateKB(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateKBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.kbService.CreateKB(&req, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// UpdateKB handles the update knowledge base request
// @Summary Update Knowledge Base
// @Description Update an existing knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.UpdateKBRequest true "knowledge base update info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/update [post]
func (h *KnowledgebaseHandler) UpdateKB(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.UpdateKBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.kbService.UpdateKB(&req, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "authorization") {
			jsonError(c, common.CodeAuthenticationError, err.Error())
			return
		}
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// UpdateMetadataSetting handles the update metadata setting request
// @Summary Update Metadata Setting
// @Description Update metadata settings for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.UpdateMetadataSettingRequest true "metadata setting info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/update_metadata_setting [post]
func (h *KnowledgebaseHandler) UpdateMetadataSetting(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.UpdateMetadataSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.kbService.UpdateMetadataSetting(&req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// GetDetail handles the get knowledge base detail request
// @Summary Get Knowledge Base Detail
// @Description Get detailed information about a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id query string true "Knowledge Base ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/detail [get]
func (h *KnowledgebaseHandler) GetDetail(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Query("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	result, code, err := h.kbService.GetDetail(kbID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "authorized") {
			jsonError(c, common.CodeOperatingError, err.Error())
			return
		}
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// ListKbs handles the list knowledge bases request
// @Summary List Knowledge Bases
// @Description List knowledge bases with pagination and filtering
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.ListKbsRequest true "list options"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/list [post]
func (h *KnowledgebaseHandler) ListKbs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.ListKbsRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			jsonError(c, common.CodeDataError, err.Error())
			return
		}
	}

	// Get parameters from request or query string
	keywords := ""
	if req.Keywords != nil {
		keywords = *req.Keywords
	} else if queryKeywords := c.Query("keywords"); queryKeywords != "" {
		keywords = queryKeywords
	}

	page := 0
	if req.Page != nil {
		page = *req.Page
	} else if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if req.PageSize != nil {
		pageSize = *req.PageSize
	} else if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	parserID := ""
	if req.ParserID != nil {
		parserID = *req.ParserID
	} else if queryParserID := c.Query("parser_id"); queryParserID != "" {
		parserID = queryParserID
	}

	orderby := "update_time"
	if req.Orderby != nil {
		orderby = *req.Orderby
	} else if queryOrderby := c.Query("orderby"); queryOrderby != "" {
		orderby = queryOrderby
	}

	desc := true
	if req.Desc != nil {
		desc = *req.Desc
	} else if descStr := c.Query("desc"); descStr != "" {
		desc = strings.ToLower(descStr) == "true"
	}

	var ownerIDs []string
	if req.OwnerIDs != nil {
		ownerIDs = *req.OwnerIDs
	}

	result, code, err := h.kbService.ListKbs(keywords, page, pageSize, parserID, orderby, desc, ownerIDs, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteKB handles the delete knowledge base request
// @Summary Delete Knowledge Base
// @Description Soft delete a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body object{kb_id string} true "knowledge base id"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/rm [post]
func (h *KnowledgebaseHandler) DeleteKB(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req struct {
		KBID string `json:"kb_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	code, err := h.kbService.DeleteKB(req.KBID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "authorization") {
			jsonError(c, common.CodeAuthenticationError, err.Error())
			return
		}
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
}

// ListTags handles the list tags request for a knowledge base
// @Summary List Tags
// @Description List tags for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id path string true "Knowledge Base ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/{kb_id}/tags [get]
func (h *KnowledgebaseHandler) ListTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Param("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	if !h.kbService.Accessible(kbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	jsonResponse(c, common.CodeSuccess, []string{}, "success")
}

// ListTagsFromKbs handles the list tags from multiple knowledge bases request
// @Summary List Tags from Knowledge Bases
// @Description List tags from multiple knowledge bases
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_ids query string true "Comma-separated Knowledge Base IDs"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/tags [get]
func (h *KnowledgebaseHandler) ListTagsFromKbs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbIDsStr := c.Query("kb_ids")
	if kbIDsStr == "" {
		jsonError(c, common.CodeDataError, "kb_ids is required")
		return
	}

	kbIDs := strings.Split(kbIDsStr, ",")
	for _, kbID := range kbIDs {
		if !h.kbService.Accessible(kbID, user.ID) {
			jsonError(c, common.CodeAuthenticationError, "No authorization.")
			return
		}
	}

	jsonResponse(c, common.CodeSuccess, []string{}, "success")
}

// RemoveTags handles the remove tags request
// @Summary Remove Tags
// @Description Remove tags from a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id path string true "Knowledge Base ID"
// @Param request body object{tags []string} true "tags to remove"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/{kb_id}/rm_tags [post]
func (h *KnowledgebaseHandler) RemoveTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Param("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	if !h.kbService.Accessible(kbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	// Get KB to find tenant_id and build index name
	kb, err := h.kbService.GetByID(kbID)
	if err != nil {
		jsonError(c, common.CodeDataError, "knowledge base not found")
		return
	}

	// Build index name prefix: ragflow_<tenant_id>
	indexName := "ragflow_" + kb.TenantID

	// For each tag, call UpdateChunk to remove it from documents
	for _, tag := range req.Tags {
		condition := map[string]interface{}{
			"tag_kwd": tag,
			"kb_id":   kbID,
		}
		newValue := map[string]interface{}{
			"remove": map[string]interface{}{
				"tag_kwd": tag,
			},
		}
		err := h.kbService.RemoveTag(condition, newValue, indexName, kbID)
		if err != nil {
			jsonError(c, common.CodeServerError, "Failed to remove tag: "+err.Error())
			return
		}
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
}

// RenameTag handles the rename tag request
// @Summary Rename Tag
// @Description Rename a tag in a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id path string true "Knowledge Base ID"
// @Param request body object{from_tag string, to_tag string} true "tag rename info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/{kb_id}/rename_tag [post]
func (h *KnowledgebaseHandler) RenameTag(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Param("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	if !h.kbService.Accessible(kbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	var req struct {
		FromTag string `json:"from_tag" binding:"required"`
		ToTag   string `json:"to_tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
}

// KnowledgeGraph handles the get knowledge graph request
// @Summary Get Knowledge Graph
// @Description Get knowledge graph for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id path string true "Knowledge Base ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/{kb_id}/knowledge_graph [get]
func (h *KnowledgebaseHandler) KnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Param("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	if !h.kbService.Accessible(kbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	result := map[string]interface{}{
		"graph":    map[string]interface{}{},
		"mind_map": map[string]interface{}{},
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteKnowledgeGraph handles the delete knowledge graph request
// @Summary Delete Knowledge Graph
// @Description Delete knowledge graph for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id path string true "Knowledge Base ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/{kb_id}/knowledge_graph [delete]
func (h *KnowledgebaseHandler) DeleteKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Param("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	if !h.kbService.Accessible(kbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
}

// GetMeta handles the get metadata request
// @Summary Get Metadata
// @Description Get metadata for knowledge bases
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_ids query string true "Comma-separated Knowledge Base IDs"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/get_meta [get]
func (h *KnowledgebaseHandler) GetMeta(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbIDsStr := c.Query("kb_ids")
	if kbIDsStr == "" {
		jsonError(c, common.CodeDataError, "kb_ids is required")
		return
	}

	kbIDs := strings.Split(kbIDsStr, ",")
	for _, kbID := range kbIDs {
		if !h.kbService.Accessible(kbID, user.ID) {
			jsonError(c, common.CodeAuthenticationError, "No authorization.")
			return
		}
	}

	meta, err := h.documentService.GetMetadataByKBs(kbIDs)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, meta, "success")
}

// GetBasicInfo handles the get basic info request
// @Summary Get Basic Info
// @Description Get basic information for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param kb_id query string true "Knowledge Base ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/basic_info [get]
func (h *KnowledgebaseHandler) GetBasicInfo(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Query("kb_id")
	if kbID == "" {
		jsonError(c, common.CodeDataError, "kb_id is required")
		return
	}

	if !h.kbService.Accessible(kbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	jsonResponse(c, common.CodeSuccess, map[string]interface{}{}, "success")
}

// CreateDatasetInDocEngine handles the create dataset request for a knowledge base
// @Summary Create Dataset in Doc Engine
// @Description Create the Infinity table for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.CreateDatasetTableRequest true "create dataset request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/doc_engine_table [post]
func (h *KnowledgebaseHandler) CreateDatasetInDocEngine(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateDatasetTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	// Check authorization
	if !h.kbService.Accessible(req.KBID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	result, code, err := h.kbService.CreateDatasetInDocEngine(&req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteDatasetInDocEngineRequest represents the request for deleting a dataset table
type DeleteDatasetInDocEngineRequest struct {
	KBID string `json:"kb_id" binding:"required"`
}

// DeleteDatasetInDocEngine handles the delete dataset request for a knowledge base
// @Summary Delete Dataset in Doc Engine
// @Description Delete the Infinity table for a knowledge base
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body DeleteDatasetInDocEngineRequest true "delete dataset request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/doc_engine_table [delete]
func (h *KnowledgebaseHandler) DeleteDatasetInDocEngine(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req DeleteDatasetInDocEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	// Check authorization
	if !h.kbService.Accessible(req.KBID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	code, err := h.kbService.DeleteDatasetInDocEngine(req.KBID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, nil, "success")
}

// InsertDatasetFromFileRequest request for inserting chunks into dataset from file
type InsertDatasetFromFileRequest struct {
	FilePath string `json:"file_path" binding:"required"`
}

// @Summary Insert chunks into dataset from file
// @Description Internal: Insert into dataset table from a JSON file (table name extracted from file)
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body InsertDatasetFromFileRequest true "insert dataset request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/kb/insert_from_file [post]
func (h *KnowledgebaseHandler) InsertDatasetFromFile(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req InsertDatasetFromFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	if req.FilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "file_path is required",
		})
		return
	}

	// Read the JSON file
	data, err := os.ReadFile(req.FilePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "failed to read file: " + err.Error(),
		})
		return
	}

	// Parse JSON - format: {"table_name": ..., "knowledgebase_id": ..., "chunks": [...]}
	var debugFormat struct {
		TableNamePrefix string                   `json:"table_name"`
		KnowledgebaseID string                   `json:"knowledgebase_id"`
		Chunks          []map[string]interface{} `json:"chunks"`
	}

	if err := json.Unmarshal(data, &debugFormat); err != nil || debugFormat.Chunks == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid JSON format: expected {\"table_name\": ..., \"knowledgebase_id\": ..., \"chunks\": [...]}",
		})
		return
	}

	if len(debugFormat.Chunks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "no chunks found in file",
		})
		return
	}

	// Get the document engine and insert
	docEngine := engine.Get()
	result, err := docEngine.InsertDataset(c.Request.Context(), debugFormat.Chunks, debugFormat.TableNamePrefix, debugFormat.KnowledgebaseID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to insert into dataset: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}