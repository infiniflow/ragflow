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
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	if roles == nil {
		roles = []map[string]interface{}{}
	}

	common.SuccessWithData(c, roles, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	role, err := h.service.CreateRole(req.RoleName, req.Description)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, role, "")
}

// ShowRole handle show role
func (h *Handler) ShowRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	role, err := h.service.ShowRole(roleName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, role, "")
}

// UpdateRoleHTTPRequest update role request
type UpdateRoleHTTPRequest struct {
	Description string `json:"description" binding:"required"`
}

// UpdateRole handle update role
func (h *Handler) UpdateRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	var req UpdateRoleHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role description is required")
		return
	}

	role, err := h.service.UpdateRole(roleName, req.Description)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, role, "")
}

// DropRole handle drop role
func (h *Handler) DropRole(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	role, err := h.service.DropRole(roleName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeNotFound, "Role not found")
		return
	}

	common.SuccessWithData(c, role, "")
}

// ShowRolePermission handle get role permission
func (h *Handler) ShowRolePermission(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	permissions, err := h.service.ShowRolePermission(roleName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, permissions, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	var req GrantRolePermissionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Permission is required")
		return
	}

	result, err := h.service.GrantRolePermission(roleName, req.Actions, req.Resource)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	var req RevokeRolePermissionHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Permission is required")
		return
	}

	result, err := h.service.RevokeRolePermission(roleName, req.Actions, req.Resource)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
}

// ListResources handle list role resources
func (h *Handler) ListResources(c *gin.Context) {
	resources, err := h.service.ListResources()
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "Role not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, resources, "")
}

// ListRolesWithPermission handle list roles with permission
func (h *Handler) ListRolesWithPermission(c *gin.Context) {
	roles, err := h.service.ListRolesWithPermission()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, roles, "")
}

func (h *Handler) ShowRoleDefaultModels(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	result, err := h.service.ShowRoleDefaultModels(roleName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}
	common.SuccessWithData(c, result, "Role default model set successfully")
}

type SetRoleDefaultModelRequest struct {
	ModelID   string `json:"model_id"`
	ModelType string `json:"model_type" binding:"required"`
}

func (h *Handler) SetRoleDefaultModel(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	var request SetRoleDefaultModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
	}

	result, err := h.service.SetRoleDefaultModel(roleName, request.ModelID, request.ModelType)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}
	common.SuccessWithData(c, result, "Role default model set successfully")
}

type ResetRoleDefaultModelRequest struct {
	ModelType string `json:"model_type" binding:"required"`
}

func (h *Handler) ResetRoleDefaultModel(c *gin.Context) {
	roleName := c.Param("role_name")
	if roleName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	var request ResetRoleDefaultModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	result, err := h.service.ResetRoleDefaultModel(roleName, request.ModelType)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}
	common.SuccessWithData(c, result, "Role default model set successfully")
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
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "List model providers successfully")
}

type AddProviderRequest struct {
	ProviderName string `json:"provider_name" binding:"required"`
}

func (h *Handler) AddModelProvider(c *gin.Context) {
	var req AddProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.AddModelProvider(req.ProviderName, userID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model provider added successfully")
}

func (h *Handler) ShowProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeNotFound, err.Error())
		return
	}
	common.SuccessWithData(c, provider, "success")
}

type DeleteProviderRequest struct {
	ProviderNames []string `json:"provider_names" binding:"required"`
}

func (h *Handler) DeleteModelProvider(c *gin.Context) {
	var req DeleteProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.DeleteModelProviders(userID, req.ProviderNames)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model provider deleted successfully")
}

func (h *Handler) ListModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	models, err := dao.GetModelProviderManager().ListModels(providerName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeNotFound, err.Error())
		return
	}

	common.SuccessWithData(c, models, "success")
}

func (h *Handler) ShowProviderModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	modelName := c.Param("model_name")
	if modelName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
		return
	}
	model, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeNotFound, err.Error())
		return
	}
	common.SuccessWithData(c, model, "success")
}

func (h *Handler) ListModelInstances(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.ListModelInstances(userID, providerName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instances listed successfully")
}

func (h *Handler) ShowProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}
	userID := c.GetString("user_id")

	result, err := h.service.ShowProviderInstance(userID, providerName, instanceName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instance shown successfully")
}

func (h *Handler) ShowProviderInstanceBalance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}
	userID := c.GetString("user_id")

	result, err := h.service.ShowProviderInstanceBalance(userID, providerName, instanceName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instance balance shown successfully")
}

func (h *Handler) CheckInstanceConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}
	userID := c.GetString("user_id")

	result, err := h.service.CheckInstanceConnection(userID, providerName, instanceName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instance connection checked successfully")
}

type CheckConnectionRequest struct {
	APIKey  string `json:"api_key"`
	Region  string `json:"region"`
	BaseURL string `json:"base_url"`
}

func (h *Handler) CheckProviderConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	var req CheckConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.CheckProviderConnection(userID, providerName, req.Region, req.APIKey, req.BaseURL)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instance connection checked successfully")
}

type AlterProviderInstanceRequest struct {
	InstanceName string `json:"instance_name"`
	APIKey       string `json:"api_key"`
}

func (h *Handler) AlterProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	var req AlterProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		common.ErrorWithCode(c, common.CodeUnauthorized, "Unauthorized")
		return
	}

	result, err := h.service.AlterProviderInstance(userID, providerName, instanceName, req.InstanceName, req.APIKey)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instance altered successfully")
}

type AddModelInstanceRequest struct {
	InstanceName string `json:"instance_name" binding:"required"`
}

func (h *Handler) AddModelInstance(c *gin.Context) {
	var req AddModelInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.AddModelInstance(userID, providerName, req.InstanceName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model instance added successfully")
}

type DropModelInstanceRequest struct {
	InstanceNames []string `json:"instance_names" binding:"required"`
}

func (h *Handler) DeleteModelInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	var req DropModelInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.DeleteModelInstances(userID, providerName, req.InstanceNames)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model provider added successfully")
}

func (h *Handler) ListInstanceModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.ListInstanceModels(userID, providerName, instanceName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Models listed successfully")
}

type EnableOrDisableModelRequest struct {
	ModelID string `json:"model_id"`
	Status  string `json:"status"`
}

func (h *Handler) EnableOrDisableModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	var req EnableOrDisableModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	modelID := strings.TrimSpace(req.ModelID)
	modelName := strings.TrimPrefix(c.Param("model_name"), "/")
	modelName = strings.TrimSpace(modelName)
	if modelName == "" && modelID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "model_name or model_id is required")
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.EnableOrDisableModel(userID, providerName, instanceName, modelName, modelID, req.Status)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Models listed successfully")
}

type AddModelsRequest struct {
	ModelNames []string `json:"model_names" binding:"required"`
}

func (h *Handler) AddModels(c *gin.Context) {
	var req AddModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.AddModels(userID, providerName, instanceName, req.ModelNames)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Models added successfully")
}

type DropModelsRequest struct {
	ModelNames []string `json:"model_names" binding:"required"`
}

func (h *Handler) DeleteModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	var req DropModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	userID := c.GetString("user_id")

	result, err := h.service.DeleteModels(userID, providerName, instanceName, req.ModelNames)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Model deleted successfully")
}

// GetSystemFingerprint handle get system fingerprint
func (h *Handler) GetSystemFingerprint(c *gin.Context) {
	fingerprint, err := h.service.GetSystemFingerprint()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, fingerprint, "")
}

type SetSystemLicenseRequest struct {
	License string `json:"license" binding:"required"`
}

// SetSystemLicense to set system license
func (h *Handler) SetSystemLicense(c *gin.Context) {
	var req SetSystemLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	err := h.service.SetSystemLicense(req.License)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}
	common.SuccessWithData(c, nil, "System license set successfully")
}

// ShowSystemLicense to get system license
func (h *Handler) ShowSystemLicense(c *gin.Context) {
	check, ok := c.GetQuery("check")
	if !ok {
		check = "false"
	}
	checkFlag, err := strconv.ParseBool(check)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	systemLicense, err := h.service.ShowSystemLicense(checkFlag)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, systemLicense, "")
}

type SetSystemLicenseConfigRequest struct {
	TimeRecordSaveInterval int64 `json:"value1" binding:"required"`
	TimeRecordTaskDuration int64 `json:"value2" binding:"required"`
}

func (h *Handler) UpdateSystemLicenseConfig(c *gin.Context) {
	var req SetSystemLicenseConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.UpdateSystemLicenseConfig(req.TimeRecordSaveInterval, req.TimeRecordTaskDuration)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}
	common.SuccessWithData(c, result, "System license config updated successfully")
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
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	userActivity, err := h.service.ShowUserActivity(req.Email, req.Days)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userActivity, "")
}

type ShowUserDatasetSummaryRequest struct {
	Dataset string `json:"dataset"`
}

// ShowUserDatasetSummary handle show user dataset summary
func (h *Handler) ShowUserDatasetSummary(c *gin.Context) {
	var req ShowUserDatasetSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	username, err := getUserName(c)
	if err != nil {
		return
	}

	userDatasetSummary, err := h.service.ShowUserDatasetSummary(username, req.Dataset)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userDatasetSummary, "")
}

// ShowUserSummary handle show user summary
func (h *Handler) ShowUserSummary(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	userSummary, err := h.service.ShowUserSummary(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userSummary, "")
}

// ShowUserStorage handle show user storage
func (h *Handler) ShowUserStorage(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	userStorage, err := h.service.ShowUserStorage(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userStorage, "")
}

// ShowUserQuota handle show user quota
func (h *Handler) ShowUserQuota(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	userQuota, err := h.service.ShowUserQuota(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userQuota, "")
}

// ShowUserIndex handle show user index
func (h *Handler) ShowUserIndex(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	userIndex, err := h.service.ShowUserIndex(username)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, userIndex, "")
}

// UpdateUserRoleHTTPRequest update user role request
type UpdateUserRoleHTTPRequest struct {
	RoleName string `json:"role_name" binding:"required"`
}

// UpdateUserRole handle update user role
func (h *Handler) UpdateUserRole(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	var req UpdateUserRoleHTTPRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Role name is required")
		return
	}

	result, err := h.service.UpdateUserRole(username, req.RoleName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
}

// ShowUserPermission handle show user permission
func (h *Handler) ShowUserPermission(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	permissions, err := h.service.ShowUserPermission(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, permissions, "")
}

// ListUserDatasets handle show user datasets
func (h *Handler) ListUserDatasets(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	datasets, err := h.service.ListUserDatasets(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, datasets, "")
}

// ListUserAgents handle show user agents
func (h *Handler) ListUserAgents(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	agents, err := h.service.ListUserAgents(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, agents, "")
}

// ListUserChats handle show user chats
func (h *Handler) ListUserChats(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	chats, err := h.service.ListUserChats(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, chats, "")
}

// ListUserSearches handle show user searches
func (h *Handler) ListUserSearches(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	searches, err := h.service.ListUserSearches(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, searches, "")
}

// ListUserModels handle show user models
func (h *Handler) ListUserModels(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	models, err := h.service.ListUserModels(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, models, "")
}

// ListUserFiles handle show user files
func (h *Handler) ListUserFiles(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	files, err := h.service.ListUserFiles(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, files, "")
}

// ListUserProviders handle show user providers
func (h *Handler) ListUserProviders(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	providers, err := h.service.ListUserProviders(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, providers, "")
}

// ListUserProviderInstances handle show user provider instances
func (h *Handler) ListUserProviderInstances(c *gin.Context) {
	encodedUsername := c.Param("username")
	userName, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if userName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Username is required")
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Provider name is required")
		return
	}

	instances, err := h.service.ListUserProviderInstances(userName, providerName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, instances, "")
}

// ListUserProviderInstanceModels handle show user provider instance models
func (h *Handler) ListUserProviderInstanceModels(c *gin.Context) {
	encodedUsername := c.Param("username")
	userName, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if userName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Username is required")
		return
	}

	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Instance name is required")
		return
	}

	models, err := h.service.ListUserProviderInstanceModels(userName, providerName, instanceName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, models, "")
}

// ListUserDefaultModels handle show user default models
func (h *Handler) ListUserDefaultModels(c *gin.Context) {
	encodedUsername := c.Param("username")
	userName, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if userName == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Username is required")
		return
	}

	models, err := h.service.ListUserDefaultModels(userName)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, models, "")
}

// ShowUsersSummary handle show users summary
func (h *Handler) ShowUsersSummary(c *gin.Context) {
	usersSummary, err := h.service.ShowUsersSummary()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersSummary, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	usersActivity, err := h.service.ShowUsersActivity(req.Days, req.Window)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersActivity, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page index must be an integer")
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page size must be an integer")
			return
		}
	}

	usersReports, err := h.service.ListUsersReports(pageIndex, pageSize, req.Status, req.Plan, req.Days)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersReports, "")
}

// ListUsersStorage handle show users storage
func (h *Handler) ListUsersStorage(c *gin.Context) {

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page index must be an integer")
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page size must be an integer")
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Top must be an integer")
		}
	}

	usersStorage, err := h.service.ListUsersStorage(pageIndex, pageSize, top)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersStorage, "")
}

// ListUsersDocuments handle show users documents
func (h *Handler) ListUsersDocuments(c *gin.Context) {

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page index must be an integer")
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page size must be an integer")
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Top must be an integer")
		}
	}

	usersDocuments, err := h.service.ListUsersDocuments(pageIndex, pageSize, top)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersDocuments, "")
}

// ListUsersIndex handle show users index
func (h *Handler) ListUsersIndex(c *gin.Context) {

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page index must be an integer")
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page size must be an integer")
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Top must be an integer")
		}
	}

	usersIndex, err := h.service.ListUsersIndex(pageIndex, pageSize, top)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersIndex, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	var err error
	pageIndex := 0
	pageIndexStr := c.Param("page")
	if pageIndexStr != "" {
		pageIndex, err = strconv.Atoi(pageIndexStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page index must be an integer")
			return
		}
	}
	pageSize := 10
	pageSizeStr := c.Param("page_size")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Page size must be an integer")
			return
		}
	}
	top := 10
	topStr := c.Param("top")
	if topStr != "" {
		top, err = strconv.Atoi(topStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Top must be an integer")
		}
	}

	usersQuota, err := h.service.ListUsersQuota(pageIndex, pageSize, top, request.QuotaThreshold, request.Plan, request.Days)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersQuota, "")
}

// ShowUsersPlanSummary handle show users plan summary
func (h *Handler) ShowUsersPlanSummary(c *gin.Context) {
	usersPlanSummary, err := h.service.ShowUsersPlanSummary()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersPlanSummary, "")
}

// ShowUsersPlan handle show users plan
func (h *Handler) ShowUsersPlan(c *gin.Context) {
	var quota int
	quotaStr := c.Query("quota")
	if quotaStr != "" {
		var err error
		quota, err = strconv.Atoi(quotaStr)
		if err != nil {
			common.ErrorWithCode(c, common.CodeBadRequest, "Quota must be an integer")
			return
		}
	}
	usersPlanQuota, err := h.service.ShowUsersPlanQuota(quota)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersPlanQuota, "")
}

// ShowUsersQuotaSummary handle show users quota summary
func (h *Handler) ShowUsersQuotaSummary(c *gin.Context) {
	usersQuotaSummary, err := h.service.ShowUsersQuotaSummary()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, usersQuotaSummary, "")
}

// ShowIngestionTasksSummary handle show ingestion tasks summary
func (h *Handler) ShowIngestionTasksSummary(c *gin.Context) {
	ingestionTasksSummary, err := h.service.ShowIngestionTasksSummary()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, ingestionTasksSummary, "")
}

// ShowDataSummary handle show data summary
func (h *Handler) ShowDataSummary(c *gin.Context) {
	dataSummary, err := h.service.ShowDataSummary()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, dataSummary, "")
}

// ShowDataOrphan handle show data orphan
func (h *Handler) ShowDataOrphan(c *gin.Context) {
	dataOrphan, err := h.service.ShowDataOrphan()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, dataOrphan, "")
}

// ShowDataStorage handle show data storage
func (h *Handler) ShowDataStorage(c *gin.Context) {
	dataStorage, err := h.service.ShowDataStorage()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, dataStorage, "")
}

// ShowDataIndex handle show data index
func (h *Handler) ShowDataIndex(c *gin.Context) {
	dataIndex, err := h.service.ShowDataIndex()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, dataIndex, "")
}

type PurgeOrphanDataRequest struct {
	Preview bool `json:"preview"`
}

// PurgeOrphanData handle purge orphan data
func (h *Handler) PurgeOrphanData(c *gin.Context) {
	var request PurgeOrphanDataRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.PurgeOrphanData(request.Preview)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Orphan data purged successfully")
}

type PurgeUserDataRequest struct {
	Preview bool `json:"preview"`
}

// PurgeUserData handle purge user data
func (h *Handler) PurgeUserData(c *gin.Context) {
	var request PurgeUserDataRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	username, err := getUserName(c)
	if err != nil {
		return
	}

	result, err := h.service.PurgeUserData(username, request.Preview)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
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
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.PurgeUsersData(request.Preview, request.Days, request.Plan, request.UserStatus)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			common.ErrorWithCode(c, common.CodeNotFound, "User not found")
			return
		}
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "")
}

// GenerateUserAPIKey handle create tenant API key
func (h *Handler) GenerateUserAPIKey(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}

	apiKey, err := h.service.GenerateUserAPIKey(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, apiKey, "API key generated successfully")
}

// DeleteUserAPIKey handle delete user API key
func (h *Handler) DeleteUserAPIKey(c *gin.Context) {
	encodedUsername := c.Param("username")
	username, err := common.DecodeFromBase64(encodedUsername)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	key := c.Param("key")
	if username == "" || key == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Username and key are required")
		return
	}

	result, err := h.service.DeleteUserAPIKey(username, key)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "API key deleted successfully")
}

// ListUserAPIKeys handle list user API keys
func (h *Handler) ListUserAPIKeys(c *gin.Context) {
	username, err := getUserName(c)
	if err != nil {
		return
	}

	result, err := h.service.ListUserAPIKeys(username)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "API keys listed successfully")
}

// DownloadSensitiveWords handle download sensitive words
func (h *Handler) DownloadSensitiveWords(c *gin.Context) {
	result, err := h.service.DownloadSensitiveWords()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Sensitive words downloaded successfully")
}

// UploadSensitiveWords handle upload sensitive words
func (h *Handler) UploadSensitiveWords(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil || form == nil || len(form.File["file"]) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "No file part!")
		return
	}
	files := form.File["file"]
	for _, fh := range files {
		if fh == nil || fh.Filename == "" {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "No file or filename is empty")
			return
		}
	}
	if len(files) != 1 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Only one file is allowed")
		return
	}
	file := files[0]
	result, err := h.service.UploadSensitiveWords(file)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Sensitive words uploaded successfully")
}

type BindVerificationEmailRequest struct {
	Email    string `json:"email"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	UseTLS   bool   `json:"use_tls"`
	UseSSL   bool   `json:"use_ssl"`
}

// BindVerificationEmail handle bind verification email
func (h *Handler) BindVerificationEmail(c *gin.Context) {
	var request BindVerificationEmailRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if request.Email == "" || request.Host == "" || request.Username == "" || request.Password == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Email, host, username, and password are required")
		return
	}

	result, err := h.service.BindVerificationEmail(request.Email, request.Host, request.Port, request.Username, request.Password, request.UseTLS, request.UseSSL)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Verification email bound successfully")
}

// ShowVerificationEmail handle show verification email
func (h *Handler) ShowVerificationEmail(c *gin.Context) {
	result, err := h.service.ShowVerificationEmail()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Verification email shown successfully")
}

// ShowWhiteList handle show white list
func (h *Handler) ShowWhiteList(c *gin.Context) {
	result, err := h.service.ShowWhiteList()
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "White list shown successfully")
}

type AddWhiteListRequest struct {
	Email string `json:"email"`
}

// AddWhiteList handle add white list
func (h *Handler) AddWhiteList(c *gin.Context) {
	var request AddWhiteListRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if request.Email == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Email is required")
		return
	}

	result, err := h.service.AddWhiteList(request.Email)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "White list added successfully")
}

// BatchAddWhiteList handle batch add white list
func (h *Handler) BatchAddWhiteList(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil || form == nil || len(form.File["file"]) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "No file part!")
		return
	}
	files := form.File["file"]
	for _, fh := range files {
		if fh == nil || fh.Filename == "" {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "No file or filename is empty")
			return
		}
	}
	if len(files) != 1 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Only one file is allowed")
		return
	}
	file := files[0]
	result, err := h.service.BatchAddWhiteList(file)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Batch add white list successfully")
}

// UpdateWhiteList handle update white list
func (h *Handler) UpdateWhiteList(c *gin.Context) {
	id := c.Param("id")

	var request AddWhiteListRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if request.Email == "" {
		common.ErrorWithCode(c, common.CodeBadRequest, "Email is required")
		return
	}

	result, err := h.service.UpdateWhiteList(id, request.Email)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "White list updated successfully")
}

// DeleteWhiteList handle delete white list
func (h *Handler) DeleteWhiteList(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, "Invalid id")
		return
	}

	result, err := h.service.DeleteWhiteList(id)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "White list updated successfully")
}

type BatchDeleteWhiteListRequest struct {
	Ids []int `json:"ids"`
}

// BatchDeleteWhiteList handle batch delete white list
func (h *Handler) BatchDeleteWhiteList(c *gin.Context) {
	var request BatchDeleteWhiteListRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ErrorWithCode(c, common.CodeBadRequest, err.Error())
		return
	}
	if len(request.Ids) == 0 {
		common.ErrorWithCode(c, common.CodeBadRequest, "Ids are required")
		return
	}

	result, err := h.service.BatchDeleteWhiteList(request.Ids)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}

	common.SuccessWithData(c, result, "Batch delete white list successfully")
}
