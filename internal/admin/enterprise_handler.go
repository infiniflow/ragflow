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
	"ragflow/internal/dao"
	"strconv"
	"strings"

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

	success(c, roles, "")
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

// ShowRole handle show role
func (h *Handler) ShowRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	role, err := h.service.ShowRole(roleName)
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

// DropRole handle drop role
func (h *Handler) DropRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	role, err := h.service.DropRole(roleName)
	if err != nil {
		errorResponse(c, "Role not found", 404)
		return
	}

	success(c, role, "")
}

// ShowRolePermission handle get role permission
func (h *Handler) ShowRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	permissions, err := h.service.ShowRolePermission(roleName)
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

// ListResources handle list role resources
func (h *Handler) ListResources(c *gin.Context) {
	resources, err := h.service.ListResources()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "Role not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, resources, "")
}

func (h *Handler) ShowRoleDefaultModels(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	result, err := h.service.ShowRoleDefaultModels(roleName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}
	success(c, result, "Role default model set successfully")
}

type SetRoleDefaultModelRequest struct {
	ModelID   string `json:"model_id"`
	ModelType string `json:"model_type" binding:"required"`
}

func (h *Handler) SetRoleDefaultModel(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var request SetRoleDefaultModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	result, err := h.service.SetRoleDefaultModel(roleName, request.ModelID, request.ModelType)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}
	success(c, result, "Role default model set successfully")
}

type ResetRoleDefaultModelRequest struct {
	ModelType string `json:"model_type" binding:"required"`
}

func (h *Handler) ResetRoleDefaultModel(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		errorResponse(c, "Role name is required", 400)
		return
	}

	var request ResetRoleDefaultModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	result, err := h.service.ResetRoleDefaultModel(roleName, request.ModelType)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}
	success(c, result, "Role default model set successfully")
}

func (h *Handler) ListModelProviders(c *gin.Context) {

	keywords := ""
	if queryKeywords := c.Query("available"); queryKeywords != "" {
		keywords = queryKeywords
	}

	// convert keywords to small case
	keywords = strings.ToLower(keywords)

	result, err := h.service.ListModelProviders()
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "List model providers successfully")
}

type AddProviderRequest struct {
	ProviderName string `json:"provider_name" binding:"required"`
}

func (h *Handler) AddModelProvider(c *gin.Context) {
	var req AddProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.AddModelProvider(req.ProviderName, userID)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model provider added successfully")
}

func (h *Handler) ShowProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    provider,
	})
}

type DeleteProviderRequest struct {
	ProviderNames []string `json:"provider_names" binding:"required"`
}

func (h *Handler) DeleteModelProvider(c *gin.Context) {
	var req DeleteProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.DeleteModelProviders(userID, req.ProviderNames)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model provider deleted successfully")
}

func (h *Handler) ListModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	models, err := dao.GetModelProviderManager().ListModels(providerName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    models,
	})
}

func (h *Handler) ShowProviderModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	modelName := c.Param("model_name")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}
	model, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    model,
	})
}

func (h *Handler) ListModelInstances(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.ListModelInstances(userID, providerName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instances listed successfully")
}

func (h *Handler) ShowProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}
	userID := c.GetString("user_id")

	result, err := h.service.ShowProviderInstance(userID, providerName, instanceName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instance shown successfully")
}

func (h *Handler) ShowProviderInstanceBalance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}
	userID := c.GetString("user_id")

	result, err := h.service.ShowProviderInstanceBalance(userID, providerName, instanceName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instance balance shown successfully")
}

func (h *Handler) CheckInstanceConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}
	userID := c.GetString("user_id")

	result, err := h.service.CheckInstanceConnection(userID, providerName, instanceName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instance connection checked successfully")
}

type CheckConnectionRequest struct {
	APIKey  string `json:"api_key"`
	Region  string `json:"region"`
	BaseURL string `json:"base_url"`
}

func (h *Handler) CheckProviderConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	var req CheckConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.CheckProviderConnection(userID, providerName, req.Region, req.APIKey, req.BaseURL)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instance connection checked successfully")
}

type AlterProviderInstanceRequest struct {
	InstanceName string `json:"instance_name"`
	APIKey       string `json:"api_key"`
}

func (h *Handler) AlterProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	var req AlterProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeUnauthorized,
			"message": "Unauthorized",
		})
		return
	}

	result, err := h.service.AlterProviderInstance(userID, providerName, instanceName, req.InstanceName, req.APIKey)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instance altered successfully")
}

type AddModelInstanceRequest struct {
	InstanceName string `json:"instance_name" binding:"required"`
}

func (h *Handler) AddModelInstance(c *gin.Context) {
	var req AddModelInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.AddModelInstance(userID, providerName, req.InstanceName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model instance added successfully")
}

type DropModelInstanceRequest struct {
	InstanceNames []string `json:"instance_names" binding:"required"`
}

func (h *Handler) DeleteModelInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	var req DropModelInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.DeleteModelInstances(userID, providerName, req.InstanceNames)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model provider added successfully")
}

func (h *Handler) ListInstanceModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.ListInstanceModels(userID, providerName, instanceName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Models listed successfully")
}

type EnableOrDisableModelRequest struct {
	ModelID string `json:"model_id"`
	Status  string `json:"status"`
}

func (h *Handler) EnableOrDisableModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	var req EnableOrDisableModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	modelID := strings.TrimSpace(req.ModelID)
	modelName := strings.TrimPrefix(c.Param("model_name"), "/")
	modelName = strings.TrimSpace(modelName)
	if modelName == "" && modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"message": "model_name or model_id is required",
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.EnableOrDisableModel(userID, providerName, instanceName, modelName, modelID, req.Status)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Models listed successfully")
}

type AddModelsRequest struct {
	ModelNames []string `json:"model_names" binding:"required"`
}

func (h *Handler) AddModels(c *gin.Context) {
	var req AddModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.AddModels(userID, providerName, instanceName, req.ModelNames)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Models added successfully")
}

type DropModelsRequest struct {
	ModelNames []string `json:"model_names" binding:"required"`
}

func (h *Handler) DeleteModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	var req DropModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.DeleteModels(userID, providerName, instanceName, req.ModelNames)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "Model deleted successfully")
}

// GetSystemFingerprint handle get system fingerprint
func (h *Handler) GetSystemFingerprint(c *gin.Context) {
	fingerprint, err := h.service.GetSystemFingerprint()
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, fingerprint, "")
}

type SetSystemLicenseRequest struct {
	License string `json:"license" binding:"required"`
}

// SetSystemLicense to set system license
func (h *Handler) SetSystemLicense(c *gin.Context) {
	var req SetSystemLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	err := h.service.SetSystemLicense(req.License)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}
	success(c, nil, "System license set successfully")
}

// ShowSystemLicense to get system license
func (h *Handler) ShowSystemLicense(c *gin.Context) {
	check, ok := c.GetQuery("check")
	if !ok {
		check = "false"
	}
	checkFlag, err := strconv.ParseBool(check)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	systemLicense, err := h.service.ShowSystemLicense(checkFlag)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, systemLicense, "")
}

type SetSystemLicenseConfigRequest struct {
	TimeRecordSaveInterval int64 `json:"value1" binding:"required"`
	TimeRecordTaskDuration int64 `json:"value2" binding:"required"`
}

func (h *Handler) UpdateSystemLicenseConfig(c *gin.Context) {
	var req SetSystemLicenseConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}
	result, err := h.service.UpdateSystemLicenseConfig(req.TimeRecordSaveInterval, req.TimeRecordTaskDuration)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}
	success(c, result, "System license config updated successfully")
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

	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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

// ListUserProviders handle show user providers
func (h *Handler) ListUserProviders(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	providers, err := h.service.ListUserProviders(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, providers, "")
}

// ListUserProviderInstances handle show user provider instances
func (h *Handler) ListUserProviderInstances(c *gin.Context) {
	encodedUsername := c.Param("username")
	userName, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	if userName == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		errorResponse(c, "Provider name is required", 400)
		return
	}

	instances, err := h.service.ListUserProviderInstances(userName, providerName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, instances, "")
}

// ListUserProviderInstanceModels handle show user provider instance models
func (h *Handler) ListUserProviderInstanceModels(c *gin.Context) {
	encodedUsername := c.Param("username")
	userName, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	if userName == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		errorResponse(c, "Provider name is required", 400)
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		errorResponse(c, "Instance name is required", 400)
		return
	}

	models, err := h.service.ListUserProviderInstanceModels(userName, providerName, instanceName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, models, "")
}

// ListUserDefaultModels handle show user default models
func (h *Handler) ListUserDefaultModels(c *gin.Context) {
	encodedUsername := c.Param("username")
	userName, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	if userName == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	models, err := h.service.ListUserDefaultModels(userName)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, models, "")
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

// ShowUsersPlanSummary handle show users plan summary
func (h *Handler) ShowUsersPlanSummary(c *gin.Context) {
	usersPlanSummary, err := h.service.ShowUsersPlanSummary()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			errorResponse(c, "User not found", 404)
			return
		}
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, usersPlanSummary, "")
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
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
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

// GenerateUserAPIKey handle create tenant API key
func (h *Handler) GenerateUserAPIKey(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}

	apiKey, err := h.service.GenerateUserAPIKey(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, apiKey, "API key generated successfully")
}

// DeleteUserAPIKey handle delete user API key
func (h *Handler) DeleteUserAPIKey(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	key := c.Param("key")
	if username == "" || key == "" {
		errorResponse(c, "Username and key are required", 400)
		return
	}

	result, err := h.service.DeleteUserAPIKey(username, key)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "API key deleted successfully")
}

// ListUserAPIKeys handle list user API keys
func (h *Handler) ListUserAPIKeys(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		errorResponse(c, err.Error(), 400)
		return
	}
	if username == "" {
		errorResponse(c, "Username is required", 400)
		return
	}

	result, err := h.service.ListUserAPIKeys(username)
	if err != nil {
		errorResponse(c, err.Error(), 500)
		return
	}

	success(c, result, "API keys listed successfully")
}
