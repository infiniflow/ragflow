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
