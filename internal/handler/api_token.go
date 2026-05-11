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

// ListTokens list all API tokens for the current user's tenant
// @Summary List API Tokens
// @Description List all API tokens for the current user's tenant
// @Tags system
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/system/tokens [get]
func (h *SystemHandler) ListTokens(c *gin.Context) {
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

	// Get tokens for the tenant
	tokens, err := h.systemService.ListAPITokens(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to list tokens",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    tokens,
	})
}

// CreateToken creates a new API token for the current user's tenant
// @Summary Create API Token
// @Description Generate a new API token for the current user's tenant
// @Tags system
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param name query string false "Name of the token"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/system/tokens [post]
func (h *SystemHandler) CreateToken(c *gin.Context) {
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
	var req service.CreateAPITokenRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request",
		})
		return
	}

	// Create token
	token, err := h.systemService.CreateAPIToken(tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to create token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    token,
	})
}

// DeleteToken deletes an API token
// @Summary Delete API Token
// @Description Remove an API token for the current user's tenant
// @Tags system
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param token path string true "The API token to remove"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/system/tokens/{token} [delete]
func (h *SystemHandler) DeleteToken(c *gin.Context) {
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

	// Get token from path parameter
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Token is required",
		})
		return
	}

	// Delete token
	if err := h.systemService.DeleteAPIToken(tenantID, token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to delete token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    true,
	})
}
