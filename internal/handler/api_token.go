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
	"errors"
	"io"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) ListAPIKeys(c *gin.Context) {
	ctx := c.Request.Context()
	_, tenantID, code, err := currentAPIKeyOwner(c)
	if err != nil {
		respondAPIKeyContextError(c, code, err)
		return
	}

	// Get keys for the tenant
	keys, err := h.systemService.ListAPIKeys(ctx, tenantID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, "Failed to list keys")
		return
	}

	common.SuccessWithData(c, keys, "success")
}

func (h *SystemHandler) CreateKey(c *gin.Context) {
	ctx := c.Request.Context()
	user, tenantID, code, err := currentAPIKeyOwner(c)
	if err != nil {
		respondAPIKeyContextError(c, code, err)
		return
	}

	// Parse request. An empty body is valid (all fields are optional);
	// ShouldBind reports io.EOF for it.
	var req service.CreateAPIKeyRequest
	if err = c.ShouldBind(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Invalid request")
		return
	}

	// Create key
	key, err := h.systemService.CreateAPIKey(ctx, user.ID, tenantID, &req)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, "Failed to create key")
		return
	}

	common.SuccessWithData(c, key, "success")
}

func (h *SystemHandler) DeleteKey(c *gin.Context) {
	ctx := c.Request.Context()
	_, tenantID, code, err := currentAPIKeyOwner(c)
	if err != nil {
		respondAPIKeyContextError(c, code, err)
		return
	}

	// Get key from path parameter
	key := c.Param("key")
	if key == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Key is required")
		return
	}

	// Delete key
	if err = h.systemService.DeleteAPIKey(ctx, tenantID, key); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, "Failed to delete key")
		return
	}

	common.SuccessWithData(c, true, "success")
}

func currentAPIKeyOwner(c *gin.Context) (*entity.User, string, common.ErrorCode, error) {
	userValue, exists := c.Get("user")
	if !exists {
		return nil, "", common.CodeUnauthorized, errors.New("Unauthorized")
	}
	user, ok := userValue.(*entity.User)
	if !ok {
		return nil, "", common.CodeServerError, errors.New("Invalid user data")
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		// Community Edition keeps the single-tenant user ID as its scope.
		tenantID = user.ID
	}
	membership, err := dao.NewUserTenantDAO().FilterByUserIDAndTenantID(c.Request.Context(), dao.DB, user.ID, tenantID)
	if err != nil || membership.Role != "owner" {
		return nil, "", common.CodeForbidden, errors.New("Only the tenant owner can manage API keys")
	}
	return user, tenantID, common.CodeSuccess, nil
}

func respondAPIKeyContextError(c *gin.Context, code common.ErrorCode, err error) {
	status := http.StatusForbidden
	if code == common.CodeUnauthorized {
		status = http.StatusUnauthorized
	} else if code == common.CodeServerError {
		status = http.StatusInternalServerError
	}
	common.ResponseWithHttpCodeData(c, status, code, nil, err.Error())
}
