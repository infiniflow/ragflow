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
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// LangfuseService is the behaviour the handler depends on (interface enables
// mocking in tests).
type LangfuseService interface {
	SetAPIKey(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error)
	GetAPIKey(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error)
	DeleteAPIKey(ctx context.Context, tenantID string) (bool, common.ErrorCode, string, error)
}

// userTenantFinder is the subset of *dao.UserTenantDAO that LangfuseHandler
// uses to translate an authenticated user id into a tenant id. It exists so
// unit tests can inject a stub without standing up a real DB. The concrete
// *dao.UserTenantDAO satisfies it via the methods below.
type userTenantFinder interface {
	GetByUserIDAndRole(ctx context.Context, db *gorm.DB, userID, role string) ([]entity.UserTenant, error)
}

// LangfuseHandler handles /langfuse/api-key HTTP requests.
//
// The entity / DAO / column naming for the stored row uses 1-char names
// (TenantID / tenant_id / GetByTenantID). The handlers used to pass
// user.ID into that field, which silently scoped every Langfuse row to the
// configuring user rather than to the tenant — meaning two users in the
// same tenant never saw each other's keys. userTenantFinder resolves
// user.ID -> tenants[0].TenantID so the entity/column naming matches the
// behaviour.
type LangfuseHandler struct {
	langfuseService LangfuseService
	userTenantDAO   userTenantFinder // nil falls back to dao.NewUserTenantDAO()
}

// NewLangfuseHandler creates a new Langfuse handler. userTenantDAO may be nil
// — production callers use NewLangfuse() and never pass a DAO; tests inject a
// stub to drive the resolution paths without a DB.
func NewLangfuseHandler(langfuseService LangfuseService, userTenantDAO userTenantFinder) *LangfuseHandler {
	if userTenantDAO == nil {
		userTenantDAO = dao.NewUserTenantDAO()
	}
	return &LangfuseHandler{
		langfuseService: langfuseService,
		userTenantDAO:   userTenantDAO,
	}
}

// NewLangfuse keeps a zero-arg constructor consistent with other handlers.
func NewLangfuse() *LangfuseHandler {
	return NewLangfuseHandler(service.NewLangfuseService(), nil)
}

// resolveLangfuseTenantID looks up the caller's tenant via user_tenant with
// the "owner" role and writes the HTTP error before returning ok=false so the
// caller can early-return on failure. The role matches api_token.go and
// providers.go; a user with no owner role cannot manage workspace credentials.
func (h *LangfuseHandler) resolveLangfuseTenantID(c *gin.Context, user *entity.User) (string, bool) {
	tenants, err := h.userTenantDAO.GetByUserIDAndRole(c.Request.Context(), dao.DB, user.ID, "owner")
	if err != nil || len(tenants) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Tenant not found")
		return "", false
	}
	return tenants[0].TenantID, true
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	tenantID, ok := h.resolveLangfuseTenantID(c, user)
	if !ok {
		return
	}

	var req SetLangfuseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()

	row, code, err := h.langfuseService.SetAPIKey(ctx, tenantID, req.SecretKey, req.PublicKey, req.Host)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	tenantID, ok := h.resolveLangfuseTenantID(c, user)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	data, code, message, err := h.langfuseService.GetAPIKey(ctx, tenantID)
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	tenantID, ok := h.resolveLangfuseTenantID(c, user)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	okDelete, code, message, err := h.langfuseService.DeleteAPIKey(ctx, tenantID)
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, message)
		return
	}
	// No record: mirror get_json_result(message=...) with data=nil.
	if message != "" {
		common.SuccessWithData(c, nil, message)
		return
	}
	common.SuccessWithData(c, okDelete, "success")
}
