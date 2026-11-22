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
	"context"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// LangfuseService is the behaviour the handler depends on (interface enables
// mocking in tests).
type LangfuseService interface {
	SetAPIKey(ctx context.Context, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error)
	GetAPIKey(ctx context.Context) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error)
	DeleteAPIKey(ctx context.Context) (bool, common.ErrorCode, string, error)
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
	common.StartOperationLog(c, "set_langfuse_api_key")
	defer common.FinishOperationLog(c)

	var req SetLangfuseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeDataError, "Invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()

	row, code, err := h.langfuseService.SetAPIKey(ctx, req.SecretKey, req.PublicKey, req.Host)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	// Echo back the stored keys, matching the Python langfuse_keys payload.
	common.RespondSuccess(c, gin.H{
		"tenant_id":  row.TenantID,
		"secret_key": row.SecretKey,
		"public_key": row.PublicKey,
		"host":       row.Host,
	}, "success")
}

// GetAPIKey handles GET /langfuse/api-key.
func (h *LangfuseHandler) GetAPIKey(c *gin.Context) {
	ctx := c.Request.Context()

	data, code, message, err := h.langfuseService.GetAPIKey(ctx)
	if err != nil {
		common.RespondError(c, code, message)
		return
	}
	common.RespondSuccess(c, data, message)
}

// DeleteAPIKey handles DELETE /langfuse/api-key.
func (h *LangfuseHandler) DeleteAPIKey(c *gin.Context) {
	common.StartOperationLog(c, "delete_langfuse_api_key")
	defer common.FinishOperationLog(c)

	ctx := c.Request.Context()

	_, code, message, err := h.langfuseService.DeleteAPIKey(ctx)
	if err != nil {
		common.RespondError(c, code, message)
		return
	}
	common.RespondSuccess(c, nil, message)
}
