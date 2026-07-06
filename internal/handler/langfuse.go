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
	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// LangfuseService is the behaviour the handler depends on (interface enables
// mocking in tests).
type LangfuseService interface {
	SetAPIKey(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error)
	GetAPIKey(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error)
	DeleteAPIKey(tenantID string) (bool, common.ErrorCode, string, error)
}

// LangfuseHandler handles /langfuse/api-key HTTP requests.
type LangfuseHandler struct {
	langfuseService LangfuseService
}

// NewLangfuseHandler creates a new Langfuse handler.
func NewLangfuseHandler(langfuseService LangfuseService) *LangfuseHandler {
	return &LangfuseHandler{langfuseService: langfuseService}
}

// NewLangfuse keeps a zero-arg constructor consistent with other handlers.
func NewLangfuse() *LangfuseHandler {
	return NewLangfuseHandler(service.NewLangfuseService())
}

// SetLangfuseRequest is the POST/PUT body. Empty-value validation happens in
// the service layer to reproduce the Python "Missing required fields" message.
type SetLangfuseRequest struct {
	SecretKey string `json:"secret_key"`
	PublicKey string `json:"public_key"`
	Host      string `json:"host"`
}

// SetAPIKey handles POST/PUT /langfuse/api-key.
func (h *LangfuseHandler) SetAPIKey(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req SetLangfuseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Invalid request: "+err.Error())
		return
	}

	row, code, err := h.langfuseService.SetAPIKey(user.ID, req.SecretKey, req.PublicKey, req.Host)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	// Echo back the stored keys, matching the Python langfuse_keys payload.
	common.SuccessWithData(c, gin.H{
		"tenant_id":  row.TenantID,
		"secret_key": row.SecretKey,
		"public_key": row.PublicKey,
		"host":       row.Host,
	}, "success")
}

// GetAPIKey handles GET /langfuse/api-key.
func (h *LangfuseHandler) GetAPIKey(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	data, code, message, err := h.langfuseService.GetAPIKey(user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, message)
		return
	}
	common.ResponseWithCodeData(c, code, data, message)
}

// DeleteAPIKey handles DELETE /langfuse/api-key.
func (h *LangfuseHandler) DeleteAPIKey(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	ok, code, message, err := h.langfuseService.DeleteAPIKey(user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, message)
		return
	}
	// No record: mirror get_json_result(message=...) with data=nil.
	if message != "" {
		common.SuccessWithData(c, nil, message)
		return
	}
	common.SuccessWithData(c, ok, "success")
}
