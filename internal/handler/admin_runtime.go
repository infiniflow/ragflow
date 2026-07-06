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
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// AdminRuntimeHandler exposes the per-tenant canvas-runtime override API
// used by the Phase 6 canary operators. It is intentionally small — the
// selector is the only collaborator it needs.
type AdminRuntimeHandler struct {
	selector *runtime.Selector
}

// NewAdminRuntimeHandler constructs an AdminRuntimeHandler backed by the
// supplied Selector. A nil selector is treated as a misconfiguration and
// the handler refuses every request with HTTP 500.
func NewAdminRuntimeHandler(selector *runtime.Selector) *AdminRuntimeHandler {
	return &AdminRuntimeHandler{selector: selector}
}

// setRuntimeRequest is the wire shape for POST
// /api/v1/admin/canvas-runtime/:tenant_id. The mode is required; empty or
// unknown values yield 400.
type setRuntimeRequest struct {
	Runtime string `json:"runtime"`
}

// setRuntimeResponse is what the operator sees in the 200 body.
type setRuntimeResponse struct {
	Code     common.ErrorCode `json:"code"`
	TenantID string           `json:"tenant_id"`
	Runtime  string           `json:"runtime"`
	Message  string           `json:"message"`
}

// ErrSelectorNotConfigured is returned when the handler was constructed
// without a backing Selector. It maps to HTTP 500 in the response path.
var ErrSelectorNotConfigured = errors.New("admin runtime: selector not configured")

// SetTenantRuntime implements POST /api/v1/admin/canvas-runtime/:tenant_id.
//
// Auth gap: this handler accepts any authenticated request. The dedicated
// admin-role middleware is a separate workstream; the Phase 6 PR documents
// the gap here so the staging canary operator flips tenants only via a
// trusted network. Production rollout MUST wire admin auth before opening
// this endpoint publicly.
func (h *AdminRuntimeHandler) SetTenantRuntime(c *gin.Context) {
	if h.selector == nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, ErrSelectorNotConfigured.Error())
		return
	}

	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "tenant_id is required")
		return
	}

	var req setRuntimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request body: "+err.Error())
		return
	}

	mode := runtime.RuntimeMode(req.Runtime)
	switch mode {
	case runtime.RuntimeGo, runtime.RuntimePython, runtime.RuntimeAuto:
		// allowed
	default:
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "runtime must be one of: go, python, auto")
		return
	}

	if err := h.selector.Set(c.Request.Context(), tenantID, mode); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	c.JSON(http.StatusOK, setRuntimeResponse{
		Code:     common.CodeSuccess,
		TenantID: tenantID,
		Runtime:  string(mode),
		Message:  "ok",
	})
}
