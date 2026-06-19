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

package admin

import (
	"errors"
	"net/http"
	"ragflow/internal/common"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListRoles handle list roles
func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.service.ListRoles()
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	if roles == nil {
		roles = []map[string]interface{}{}
	}

	success(c, gin.H{
		"roles": roles,
		"total": len(roles),
	}, "")
}

// CreateRoleHTTPRequest create role request
type CreateRoleHTTPRequest struct {
	RoleName    string `json:"role_name" binding:"required"`
	Description string `json:"description"`
}

// CreateRole handle create role
func (h *Handler) CreateRole(c *gin.Context) {
	var req CreateRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Role name is required", 400)
		return
	}

	role, err := h.service.CreateRole(req.RoleName, req.Description)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, role, "")
}

// GetRole handle get role
func (h *Handler) GetRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	role, err := h.service.GetRole(roleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, role, "")
}

// UpdateRoleHTTPRequest update role request
type UpdateRoleHTTPRequest struct {
	Description string `json:"description" binding:"required"`
}

// UpdateRole handle update role
func (h *Handler) UpdateRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var req UpdateRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Role description is required", 400)
		return
	}

	role, err := h.service.UpdateRole(roleName, req.Description)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, role, "")
}

// DeleteRole handle delete role
func (h *Handler) DeleteRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	if err := h.service.DeleteRole(roleName); err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	successNoData(c, "")
}

// GetRolePermission handle get role permission
func (h *Handler) GetRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	permissions, err := h.service.GetRolePermission(roleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, permissions, "")
}

// GrantRolePermissionHTTPRequest grant role permission request
type GrantRolePermissionHTTPRequest struct {
	Actions  []string `json:"actions" binding:"required"`
	Resource string   `json:"resource" binding:"required"`
}

// GrantRolePermission handle grant role permission
func (h *Handler) GrantRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var req GrantRolePermissionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Permission is required", 400)
		return
	}

	result, err := h.service.GrantRolePermission(roleName, req.Actions, req.Resource)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

// RevokeRolePermissionHTTPRequest revoke role permission request
type RevokeRolePermissionHTTPRequest struct {
	Actions  []string `json:"actions" binding:"required"`
	Resource string   `json:"resource" binding:"required"`
}

// RevokeRolePermission handle revoke role permission
func (h *Handler) RevokeRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var req RevokeRolePermissionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Permission is required", 400)
		return
	}

	result, err := h.service.RevokeRolePermission(roleName, req.Actions, req.Resource)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

type ShowUserActivityRequest struct {
	Days  int    `json:"days"`
	Email string `json:"email"`
}

// ShowUserActivity handle show user activity
func (h *Handler) ShowUserActivity(c *gin.Context) {
	var req ShowUserActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}
	userActivity, err := h.service.ShowUserActivity(req.Email, req.Days)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userActivity, "")
}

type ShowUserDatasetSummaryRequest struct {
	Dataset string `json:"dataset"`
}

// ShowUserDatasetSummary handle show user dataset summary
func (h *Handler) ShowUserDatasetSummary(c *gin.Context) {
	var req ShowUserDatasetSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	userDatasetSummary, err := h.service.ShowUserDatasetSummary(username, req.Dataset)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userDatasetSummary, "")
}

// ShowUserSummary handle show user summary
func (h *Handler) ShowUserSummary(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	userSummary, err := h.service.ShowUserSummary(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userSummary, "")
}

// ShowUserStorage handle show user storage
func (h *Handler) ShowUserStorage(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	userStorage, err := h.service.ShowUserStorage(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userStorage, "")
}

// ShowUserQuota handle show user quota
func (h *Handler) ShowUserQuota(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	userQuota, err := h.service.ShowUserQuota(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userQuota, "")
}

// ShowUserIndex handle show user index
func (h *Handler) ShowUserIndex(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	userIndex, err := h.service.ShowUserIndex(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, userIndex, "")
}

// UpdateUserRoleHTTPRequest update user role request
type UpdateUserRoleHTTPRequest struct {
	RoleName string `json:"role_name" binding:"required"`
}

// UpdateUserRole handle update user role
func (h *Handler) UpdateUserRole(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	var req UpdateUserRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, "Role name is required", 400)
		return
	}

	result, err := h.service.UpdateUserRole(username, req.RoleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

// ShowUserPermission handle show user permission
func (h *Handler) ShowUserPermission(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	permissions, err := h.service.ShowUserPermission(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, permissions, "")
}

// ListUserDatasets handle show user datasets
func (h *Handler) ListUserDatasets(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	datasets, err := h.service.ListUserDatasets(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, datasets, "")
}

// ListUserAgents handle show user agents
func (h *Handler) ListUserAgents(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	agents, err := h.service.ListUserAgents(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, agents, "")
}

// ListUserChats handle show user chats
func (h *Handler) ListUserChats(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	chats, err := h.service.ListUserChats(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, chats, "")
}

// ListUserSearches handle show user searches
func (h *Handler) ListUserSearches(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	searches, err := h.service.ListUserSearches(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, searches, "")
}

// ListUserModels handle show user models
func (h *Handler) ListUserModels(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	models, err := h.service.ListUserModels(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, models, "")
}

// ListUserFiles handle show user files
func (h *Handler) ListUserFiles(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	files, err := h.service.ListUserFiles(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, files, "")
}

// ShowUsersSummary handle show users summary
func (h *Handler) ShowUsersSummary(c *gin.Context) {
	usersSummary, err := h.service.ShowUsersSummary()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersSummary, "")
}

type ShowUsersActivityRequest struct {
	Days   *int `json:"days"`
	Window *int `json:"window"`
}

// ShowUsersActivity handle show users activity
func (h *Handler) ShowUsersActivity(c *gin.Context) {
	var req ShowUsersActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}
	usersActivity, err := h.service.ShowUsersActivity(req.Days, req.Window)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersActivity, "")
}

type ListUsersReportsRequest struct {
	Status *string `json:"status"`
	Days   *int    `json:"days"`
	Plan   *string `json:"plan"`
}

// ListUsersReports handle show users reports
func (h *Handler) ListUsersReports(c *gin.Context) {
	var req ListUsersReportsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			errorResponse(c, "Page index must be an integer", 400)
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			errorResponse(c, "Page size must be an integer", 400)
			return
		}
	}

	usersReports, err := h.service.ListUsersReports(pageIndex, pageSize, req.Status, req.Plan, req.Days)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersReports, "")
}

// ListUsersStorage handle show users storage
func (h *Handler) ListUsersStorage(c *gin.Context) {

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			errorResponse(c, "Page index must be an integer", 400)
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			errorResponse(c, "Page size must be an integer", 400)
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			errorResponse(c, "Top must be an integer", 400)
		}
	}

	usersStorage, err := h.service.ListUsersStorage(pageIndex, pageSize, top)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersStorage, "")
}

// ListUsersDocuments handle show users documents
func (h *Handler) ListUsersDocuments(c *gin.Context) {

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			errorResponse(c, "Page index must be an integer", 400)
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			errorResponse(c, "Page size must be an integer", 400)
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			errorResponse(c, "Top must be an integer", 400)
		}
	}

	usersDocuments, err := h.service.ListUsersDocuments(pageIndex, pageSize, top)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersDocuments, "")
}

// ListUsersIndex handle show users index
func (h *Handler) ListUsersIndex(c *gin.Context) {

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			errorResponse(c, "Page index must be an integer", 400)
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			errorResponse(c, "Page size must be an integer", 400)
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			errorResponse(c, "Top must be an integer", 400)
		}
	}

	usersIndex, err := h.service.ListUsersIndex(pageIndex, pageSize, top)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersIndex, "")
}

type ListUsersQuotaRequest struct {
	QuotaThreshold *int    `json:"quota_threshold"`
	Plan           *string `json:"plan"`
	Days           *int    `json:"days"`
}

// ListUsersQuota handle show users quota
func (h *Handler) ListUsersQuota(c *gin.Context) {
	var request ListUsersQuotaRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			errorResponse(c, "Page index must be an integer", 400)
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			errorResponse(c, "Page size must be an integer", 400)
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			errorResponse(c, "Top must be an integer", 400)
		}
	}

	usersQuota, err := h.service.ListUsersQuota(pageIndex, pageSize, top, request.QuotaThreshold, request.Plan, request.Days)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersQuota, "")
}

// ShowUsersQuotaSummary handle show users quota summary
func (h *Handler) ShowUsersQuotaSummary(c *gin.Context) {
	usersQuotaSummary, err := h.service.ShowUsersQuotaSummary()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersQuotaSummary, "")
}

// ShowIngestionTasksSummary handle show ingestion tasks summary
func (h *Handler) ShowIngestionTasksSummary(c *gin.Context) {
	ingestionTasksSummary, err := h.service.ShowIngestionTasksSummary()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, ingestionTasksSummary, "")
}

// ShowDataSummary handle show data summary
func (h *Handler) ShowDataSummary(c *gin.Context) {
	dataSummary, err := h.service.ShowDataSummary()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, dataSummary, "")
}

// ShowDataOrphan handle show data orphan
func (h *Handler) ShowDataOrphan(c *gin.Context) {
	dataOrphan, err := h.service.ShowDataOrphan()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, dataOrphan, "")
}

// ShowDataStorage handle show data storage
func (h *Handler) ShowDataStorage(c *gin.Context) {
	dataStorage, err := h.service.ShowDataStorage()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, dataStorage, "")
}

// ShowDataIndex handle show data index
func (h *Handler) ShowDataIndex(c *gin.Context) {
	dataIndex, err := h.service.ShowDataIndex()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, dataIndex, "")
}

type PurgeOrphanDataRequest struct {
	Preview bool `json:"preview"`
}

// PurgeOrphanData handle purge orphan data
func (h *Handler) PurgeOrphanData(c *gin.Context) {
	var request PurgeOrphanDataRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}
	result, err := h.service.PurgeOrphanData(request.Preview)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Orphan data purged successfully")
}

type PurgeUserDataRequest struct {
	Preview bool `json:"preview"`
}

// PurgeUserData handle purge user data
func (h *Handler) PurgeUserData(c *gin.Context) {
	var request PurgeUserDataRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}
	username := c.Param("username")
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	result, err := h.service.PurgeUserData(username, request.Preview)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}

type PurgeUsersDataRequest struct {
	Preview    bool    `json:"preview"`
	Days       int     `json:"days"`
	Plan       *string `json:"plan"`
	UserStatus *string `json:"user_status"`
}

// PurgeUsersData handle purge users data
func (h *Handler) PurgeUsersData(c *gin.Context) {
	var request PurgeUsersDataRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	result, err := h.service.PurgeUsersData(request.Preview, request.Days, request.Plan, request.UserStatus)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "")
}
