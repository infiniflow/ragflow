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

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// langfuseServiceIface defines the LangfuseService methods used by LangfuseHandler.
type langfuseServiceIface interface {
	SetAPIKey(tenantID string, req *service.SetAPIKeyRequest) (map[string]interface{}, error)
	GetAPIKey(tenantID string) (map[string]interface{}, error)
	DeleteAPIKey(tenantID string) (bool, error)
}

// LangfuseHandler manages Langfuse credential endpoints.
type LangfuseHandler struct {
	langfuseService langfuseServiceIface
}

// NewLangfuseHandler creates a LangfuseHandler.
func NewLangfuseHandler() *LangfuseHandler {
	return &LangfuseHandler{langfuseService: service.NewLangfuseService()}
}

// SetAPIKey handles POST/PUT /api/v1/langfuse/api-key.
// Validates the supplied keys against Langfuse, then upserts the record.
// Secret key is stored but not echoed back to the caller.
// @Summary Set Langfuse API key
// @Description Create or update the Langfuse credentials for the current tenant.
// @Tags langfuse
// @Accept json
// @Produce json
// @Param request body service.SetAPIKeyRequest true "Langfuse credentials"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/langfuse/api-key [post]
func (h *LangfuseHandler) SetAPIKey(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		jsonError(c, code, msg)
		return
	}

	var req service.SetAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": false, "message": err.Error()})
		return
	}

	data, err := h.langfuseService.SetAPIKey(user.ID, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": data, "message": "success"})
}

// GetAPIKey handles GET /api/v1/langfuse/api-key.
// Returns stored metadata and Langfuse project info; secret_key is never returned.
// @Summary Get Langfuse API key info
// @Description Retrieve the stored Langfuse credentials (without secret_key) and project info.
// @Tags langfuse
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/langfuse/api-key [get]
func (h *LangfuseHandler) GetAPIKey(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		jsonError(c, code, msg)
		return
	}

	data, err := h.langfuseService.GetAPIKey(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": false, "message": err.Error()})
		return
	}
	if data == nil {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": nil, "message": "Have not record any Langfuse keys."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": data, "message": "success"})
}

// DeleteAPIKey handles DELETE /api/v1/langfuse/api-key.
// Removes the stored Langfuse credentials for the current tenant.
// @Summary Delete Langfuse API key
// @Description Remove the stored Langfuse credentials for the current tenant.
// @Tags langfuse
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/langfuse/api-key [delete]
func (h *LangfuseHandler) DeleteAPIKey(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		jsonError(c, code, msg)
		return
	}

	deleted, err := h.langfuseService.DeleteAPIKey(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": false, "message": err.Error()})
		return
	}
	if !deleted {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": nil, "message": "Have not record any Langfuse keys."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": true, "message": "success"})
}
