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
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) ListAPIKeys(c *gin.Context) {
	// Get current user from context
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Unauthorized",
		})
		return
	}

	userModel, ok := user.(*entity.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Invalid user data",
		})
		return
	}

	// Get user's tenant with owner role
	userTenantDAO := dao.NewUserTenantDAO()
	tenants, err := userTenantDAO.GetByUserIDAndRole(userModel.ID, "owner")
	if err != nil || len(tenants) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Tenant not found",
		})
		return
	}

	tenantID := tenants[0].TenantID

	// Get keys for the tenant
	keys, err := h.systemService.ListAPIKeys(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to list keys",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    keys,
	})
}

func (h *SystemHandler) CreateKey(c *gin.Context) {
	// Get current user from context
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Unauthorized",
		})
		return
	}

	userModel, ok := user.(*entity.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Invalid user data",
		})
		return
	}

	// Get user's tenant with owner role
	userTenantDAO := dao.NewUserTenantDAO()
	tenants, err := userTenantDAO.GetByUserIDAndRole(userModel.ID, "owner")
	if err != nil || len(tenants) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Tenant not found",
		})
		return
	}

	tenantID := tenants[0].TenantID

	// Parse request
	var req service.CreateAPIKeyRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request",
		})
		return
	}

	// Create key
	key, err := h.systemService.CreateAPIKey(tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to create key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    key,
	})
}

func (h *SystemHandler) DeleteKey(c *gin.Context) {
	// Get current user from context
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Unauthorized",
		})
		return
	}

	userModel, ok := user.(*entity.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Invalid user data",
		})
		return
	}

	// Get user's tenant with owner role
	userTenantDAO := dao.NewUserTenantDAO()
	tenants, err := userTenantDAO.GetByUserIDAndRole(userModel.ID, "owner")
	if err != nil || len(tenants) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Tenant not found",
		})
		return
	}

	tenantID := tenants[0].TenantID

	// Get key from path parameter
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Key is required",
		})
		return
	}

	// Delete key
	if err = h.systemService.DeleteAPIKey(tenantID, key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to delete key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    true,
	})
}
