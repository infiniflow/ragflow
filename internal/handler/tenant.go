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
	tenantService *service.TenantService
	userService   *service.UserService
}

// NewTenantHandler create tenant handler
func NewTenantHandler(tenantService *service.TenantService, userService *service.UserService) *TenantHandler {
	return &TenantHandler{
		tenantService: tenantService,
		userService:   userService,
	}
}

func (h *TenantHandler) GetModels(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	defaultModels, err := h.tenantService.ListTenantDefaultModels(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	if defaultModels == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": "No default models",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    defaultModels,
	})
}

type SetModelRequest struct {
	ModelProvider string `json:"model_provider" binding:"required"`
	ModelInstance string `json:"model_instance" binding:"required"`
	ModelName     string `json:"model_name" binding:"required"`
	ModelType     string `json:"model_type" binding:"required"`
}

func (h *TenantHandler) SetModels(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Parse request body (same as Python get_request_json())
	var req SetModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	err := h.tenantService.SetTenantDefaultModels(user.ID, req.ModelProvider, req.ModelInstance, req.ModelName, req.ModelType)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    nil,
	})
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	tenantInfo, err := h.tenantService.GetTenantInfo(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	if tenantInfo == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": "Tenant not found!",
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    tenantInfo,
	})
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	tenantList, err := h.tenantService.GetTenantList(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    tenantList,
	})
}

// CreateMetadataInDocEngine handles the create doc meta table request
// @Summary Create Doc Meta Table
// @Description Create the document metadata table for a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/doc_engine_metadata_table [post]
func (h *TenantHandler) CreateMetadataInDocEngine(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Use user.ID as tenant ID (user IS the tenant in user mode)
	tenantID := user.ID

	code, err := h.tenantService.CreateMetadataInDocEngine(tenantID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    nil,
	})
}

// DeleteMetadataInDocEngine handles the delete doc meta table request
// @Summary Delete Metadata In Doc Engine
// @Description Delete the document metadata table for a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/tenant/doc_engine_metadata_table [delete]
func (h *TenantHandler) DeleteMetadataInDocEngine(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Use user.ID as tenant ID (user IS the tenant in user mode)
	tenantID := user.ID

	code, err := h.tenantService.DeleteMetadataInDocEngine(tenantID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    nil,
	})
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req InsertMetadataFromFileRequest
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

	// Parse JSON - format: {"chunks": [...]}
	var inputFormat struct {
		Chunks []map[string]interface{} `json:"chunks"`
	}

	if err := json.Unmarshal(data, &inputFormat); err != nil || inputFormat.Chunks == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid JSON format: expected {\"chunks\": [...]}",
		})
		return
	}

	if len(inputFormat.Chunks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "no chunks found in file",
		})
		return
	}

	// Use user.ID as tenant ID (user IS the tenant in user mode)
	tenantID := user.ID

	// Get the document engine and insert
	docEngine := engine.Get()
	result, err := docEngine.InsertMetadata(c.Request.Context(), inputFormat.Chunks, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to insert metadata: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}
