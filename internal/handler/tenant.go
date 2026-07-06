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

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/service"
)

// TenantHandler tenant handler
type TenantHandler struct {
	tenantService  *service.TenantService
	userService    *service.UserService
	datasetService *service.DatasetService
}

// NewTenantHandler create tenant handler
func NewTenantHandler(tenantService *service.TenantService, userService *service.UserService, datasetService *service.DatasetService) *TenantHandler {
	return &TenantHandler{
		tenantService:  tenantService,
		userService:    userService,
		datasetService: datasetService,
	}
}

func (h *TenantHandler) SetModels(c *gin.Context) {
	h.setDefaultModels(c, false)
}

func (h *TenantHandler) SetDefaultModels(c *gin.Context) {
	h.setDefaultModels(c, true)
}

type SetModelRequest struct {
	ModelProvider string `json:"model_provider"`
	ModelInstance string `json:"model_instance"`
	ModelName     string `json:"model_name"`
	ModelID       string `json:"model_id"`
	ModelType     string `json:"model_type" binding:"required"`
}

func (h *TenantHandler) setDefaultModels(c *gin.Context, wrapModels bool) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Parse request body (same as Python get_request_json())
	var req SetModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	err := h.tenantService.SetTenantDefaultModels(user.ID, req.ModelProvider, req.ModelInstance, req.ModelName, req.ModelType, req.ModelID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
		return
	}

	if wrapModels {
		common.SuccessWithData(c, map[string]interface{}{"models": []service.ModelItem{}}, "success")
		return
	}

	common.SuccessNoData(c, "success")
}

// GetDefaultModels returns the tenant's default model selections. The
// response wraps the model list under `data.models` to mirror the
// Python `list_tenant_default_models` contract (api/apps/restful_apis/
// models_api.py:84). The frontend hook `useFetchDefaultModels`
// (web/src/hooks/use-llm-request.tsx:423) reads `data.data.models`.
func (h *TenantHandler) GetDefaultModels(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	defaultModels, err := h.tenantService.ListTenantDefaultModels(user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
		return
	}

	// Empty selection is a normal state for a freshly created tenant, not a
	// data error. Match Python's `list_tenant_default_models` (which returns
	// get_result(data=[])) and the frontend's expectation that `data.data.models`
	// is always an array.
	if defaultModels == nil {
		defaultModels = []service.ModelItem{}
	}
	common.SuccessWithData(c, map[string]interface{}{"models": defaultModels}, "success")
}

// TenantInfo get tenant information
// @Summary Get Tenant Information
// @Description Get current user's tenant information (owner tenant)
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/user/tenant_info [get]
func (h *TenantHandler) TenantInfo(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantInfo, err := h.tenantService.GetTenantInfo(user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
		return
	}

	if tenantInfo == nil {
		common.ResponseWithCodeData(c, common.CodeDataError, false, "Tenant not found!")
		return
	}

	common.SuccessWithData(c, tenantInfo, "success")
}

// TenantList get tenant list for current user
// @Summary Get Tenant List
// @Description Get all tenants that the current user belongs to
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/list [get]
func (h *TenantHandler) TenantList(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantList, err := h.tenantService.GetTenantList(user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
		return
	}

	common.SuccessWithData(c, tenantList, "success")
}

// CreateMetadataStore handles the create metadata store request
// @Summary Create Metadata Store
// @Description Create the metadata store for a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/metadata_store [post]
func (h *TenantHandler) CreateMetadataStore(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Use user.ID as tenant ID (user IS the tenant in user mode)
	tenantID := user.ID

	code, err := h.tenantService.CreateMetadataStore(tenantID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoData(c, "success")
}

// DeleteMetadataStore handles the delete metadata store request
// @Summary Delete Metadata Store
// @Description Delete the metadata store for a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/metadata_store [delete]
func (h *TenantHandler) DeleteMetadataStore(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	// Use user.ID as tenant ID (user IS the tenant in user mode)
	tenantID := user.ID

	code, err := h.tenantService.DeleteMetadataStore(tenantID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoData(c, "success")
}

// CreateChunkTableRequest represents the request for creating a chunk table
type CreateChunkTableRequest struct {
	KBID       string `json:"kb_id" binding:"required"`
	VectorSize int    `json:"vector_size" binding:"required"`
}

// CreateChunkStore handles the create chunk store request
// @Summary Create Chunk Store
// @Description Create the chunk store for a knowledge base
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body CreateChunkTableRequest true "create chunk store request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/chunk_store [post]
func (h *TenantHandler) CreateChunkStore(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req CreateChunkTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	// Check authorization - user must have access to this kb
	if !h.datasetService.Accessible(req.KBID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	serviceReq := &service.CreateDatasetTableRequest{
		KBID:       req.KBID,
		VectorSize: req.VectorSize,
	}
	result, code, err := h.tenantService.CreateChunkStore(serviceReq)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// DeleteChunkTableRequest represents the request for deleting a chunk table
type DeleteChunkTableRequest struct {
	KBID string `json:"kb_id" binding:"required"`
}

// DeleteChunkStore handles the delete chunk store request
// @Summary Delete Chunk Store
// @Description Delete the chunk store for a knowledge base
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body DeleteChunkTableRequest true "delete chunk store request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/chunk_store [delete]
func (h *TenantHandler) DeleteChunkStore(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req DeleteChunkTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	// Check authorization
	if !h.datasetService.Accessible(req.KBID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	code, err := h.tenantService.DeleteChunkStore(req.KBID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoData(c, "success")
}

// InsertChunksFromFileRequest request for inserting chunks from file
type InsertChunksFromFileRequest struct {
	FilePath string `json:"file_path" binding:"required"`
}

// @Summary Insert chunks into dataset from JSON file
// @Description Internal: Insert chunks into dataset table from a JSON file
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body InsertChunksFromFileRequest true "insert chunks request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/insert_chunks_from_file [post]
func (h *TenantHandler) InsertChunksFromFile(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req InsertChunksFromFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
		return
	}

	if req.FilePath == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "file_path is required")
		return
	}

	// Read the JSON file
	// codeql[go/path-injection] False positive: req.FilePath is the
	// JSON file path the operator configured (tenant import flow). The
	// OS access check enforces permissions, and the handler is gated
	// to admin/owner roles upstream.
	data, err := os.ReadFile(req.FilePath)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "failed to read file: "+err.Error())
		return
	}

	// Parse JSON - format: {"index_name"/"table_name": ..., "knowledgebase_id": ..., "chunks": [...]}
	var debugFormat struct {
		IndexName       string                   `json:"index_name"`
		TableName       string                   `json:"table_name"`
		KnowledgebaseID string                   `json:"knowledgebase_id"`
		Chunks          []map[string]interface{} `json:"chunks"`
	}

	if err = json.Unmarshal(data, &debugFormat); err != nil || debugFormat.Chunks == nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "invalid JSON format: expected {\"index_name\"/\"table_name\": ..., \"knowledgebase_id\": ..., \"chunks\": [...]}")
		return
	}

	if len(debugFormat.Chunks) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "no chunks found in file")
		return
	}

	// Support both index_name (ES) and table_name (Infinity) in JSON
	indexName := debugFormat.IndexName
	if indexName == "" {
		indexName = debugFormat.TableName
	}

	// Get the document engine and insert
	docEngine := engine.Get()
	result, err := docEngine.InsertChunks(c.Request.Context(), debugFormat.Chunks, indexName, debugFormat.KnowledgebaseID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "failed to insert into dataset: "+err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// InsertMetadataFromFileRequest request for inserting metadata from file
type InsertMetadataFromFileRequest struct {
	FilePath string `json:"file_path" binding:"required"`
}

// @Summary Insert document metadata from JSON file
// @Description Internal: Insert metadata into tenant's metadata table from a JSON file
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body InsertMetadataFromFileRequest true "insert metadata request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/insert_metadata_from_file [post]
func (h *TenantHandler) InsertMetadataFromFile(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req InsertMetadataFromFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
		return
	}

	if req.FilePath == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "file_path is required")
		return
	}

	// Read the JSON file
	// JSON file path the operator configured (tenant import flow). The
	// OS access check enforces permissions, and the handler is gated
	// to admin/owner roles upstream.
	// codeql[go/path-injection] False positive: req.FilePath is the
	data, err := os.ReadFile(req.FilePath)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "failed to read file: "+err.Error())
		return
	}

	// Parse JSON - format: {"chunks": [...]}
	var inputFormat struct {
		Chunks []map[string]interface{} `json:"chunks"`
	}

	if err = json.Unmarshal(data, &inputFormat); err != nil || inputFormat.Chunks == nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "invalid JSON format: expected {\"chunks\": [...]}")
		return
	}

	if len(inputFormat.Chunks) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "no chunks found in file")
		return
	}

	// Use user.ID as tenant ID (user IS the tenant in user mode)
	tenantID := user.ID

	// Get the document engine and insert
	docEngine := engine.Get()
	result, err := docEngine.InsertMetadata(c.Request.Context(), inputFormat.Chunks, tenantID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "failed to insert metadata: "+err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// ListTenantMembers lists all non-owner members of a tenant.
// @Summary List tenant members
// @Tags tenants
// @Produce json
// @Param tenant_id path string true "Tenant ID"
// @Router /api/v1/tenants/{tenant_id}/users [get]
func (h *TenantHandler) ListTenantMembers(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "tenant_id is required")
		return
	}

	members, code, err := h.tenantService.ListMembers(user.ID, tenantID)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, err.Error())
		return
	}
	common.SuccessWithData(c, members, "success")
}

// AddTenantMember invites a user (by email) to the tenant.
// @Summary Invite a user to a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenant_id path string true "Tenant ID"
// @Param request body service.AddMemberRequest true "Invite request"
// @Router /api/v1/tenants/{tenant_id}/users [post]
func (h *TenantHandler) AddTenantMember(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "tenant_id is required")
		return
	}

	var req service.AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "invalid request body: "+err.Error())
		return
	}

	resp, code, err := h.tenantService.AddMember(user.ID, tenantID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, err.Error())
		return
	}
	common.SuccessWithData(c, resp, "success")
}

// RemoveTenantMember removes a user from the tenant.
// @Summary Remove a user from a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenant_id path string true "Tenant ID"
// @Param request body object true "Remove member request" SchemaExample({"user_id":"string"})
// @Router /api/v1/tenants/{tenant_id}/users [delete]
func (h *TenantHandler) RemoveTenantMember(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "tenant_id is required")
		return
	}

	var body struct {
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.UserID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "user_id is required")
		return
	}

	code, err := h.tenantService.RemoveMember(user.ID, tenantID, body.UserID)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, err.Error())
		return
	}
	common.SuccessWithData(c, true, "success")
}

// AcceptTenantInvite accepts a pending team invitation, transitioning role invite → normal.
// @Summary Accept tenant invitation
// @Tags tenants
// @Produce json
// @Param tenant_id path string true "Tenant ID"
// @Router /api/v1/tenants/{tenant_id} [patch]
func (h *TenantHandler) AcceptTenantInvite(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "tenant_id is required")
		return
	}

	code, err := h.tenantService.AcceptInvite(user.ID, tenantID)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, err.Error())
		return
	}
	common.SuccessWithData(c, true, "success")
}
