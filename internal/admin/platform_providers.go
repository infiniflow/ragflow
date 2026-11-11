// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package admin

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/handler"
	"ragflow/internal/service"
)

type platformProviderItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (h *Handler) ListPlatformProviders(c *gin.Context) {
	var providers []entity.TenantModelProvider
	if err := dao.DB.WithContext(c.Request.Context()).Where("tenant_id = ?", entity.PlatformTenantID).Order("provider_name").Find(&providers).Error; err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to load platform providers")
		return
	}
	items := make([]platformProviderItem, 0, len(providers))
	for _, provider := range providers {
		items = append(items, platformProviderItem{ID: provider.ID, Name: provider.ProviderName})
	}
	common.RespondSuccessList(c, items, "")
}

func (h *Handler) CreatePlatformProvider(c *gin.Context) {
	var req struct {
		ProviderName string `json:"provider_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	creatorID := strings.TrimSpace(c.GetString("user_id"))
	provider := dao.GetModelProviderManager().FindProvider(strings.TrimSpace(req.ProviderName))
	if provider == nil || strings.EqualFold(provider.Name, "MinerU.Net") || creatorID == "" {
		common.RespondError(c, common.CodeDataError, "Invalid provider")
		return
	}
	created, err := (dao.PlatformModelSettings{}).CreateProvider(c.Request.Context(), dao.DB, provider.Name, creatorID)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to create platform provider")
		return
	}
	common.RespondSuccess(c, platformProviderItem{ID: created.ID, Name: created.ProviderName}, "")
}

func (h *Handler) ListPlatformProviderCatalog(c *gin.Context) {
	providers, err := dao.GetModelProviderManager().ListProviders()
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to load provider catalog")
		return
	}
	filtered := make([]map[string]interface{}, 0, len(providers))
	for _, provider := range providers {
		if name, _ := provider["name"].(string); strings.EqualFold(name, "MinerU.Net") {
			continue
		}
		delete(provider, "url_suffix")
		delete(provider, "tags")
		filtered = append(filtered, provider)
	}
	common.RespondSuccessList(c, filtered, "")
}

func (h *Handler) ListPlatformInstances(c *gin.Context) {
	providerID := c.Param("provider_id")
	if _, err := service.PlatformProvider(c.Request.Context(), dao.DB, providerID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.RespondError(c, common.CodeNotFound, "Platform provider not found")
		} else {
			common.RespondError(c, common.CodeServerError, "Failed to load platform provider")
		}
		return
	}
	instances, err := dao.NewTenantModelInstanceDAO().ListSummariesByProviderIDs(c.Request.Context(), dao.DB, []string{providerID})
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to load platform instances")
		return
	}
	items := make([]gin.H, 0, len(instances))
	for _, instance := range instances {
		items = append(items, gin.H{"id": instance.ID, "provider_id": instance.ProviderID, "instance_name": instance.InstanceName, "status": instance.Status})
	}
	common.RespondSuccessList(c, items, "")
}

func (h *Handler) CreatePlatformInstance(c *gin.Context) {
	var req service.CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	if err := service.ValidateProviderInstanceName(req.InstanceName); err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}
	created, err := h.service.modelProviderService.CreatePlatformInstance(c.Request.Context(), dao.DB, c.Param("provider_id"), req)
	if errors.Is(err, service.ErrInvalidPlatformModelType) {
		common.RespondError(c, common.CodeArgumentError, "Invalid model type")
		return
	}
	if errors.Is(err, service.ErrPlatformModelNameRequired) {
		common.RespondError(c, common.CodeArgumentError, "Model name is required")
		return
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, service.ErrPlatformInstanceConflict) {
		common.RespondError(c, common.CodeConflict, "Platform instance already exists")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.RespondError(c, common.CodeNotFound, "Platform provider not found")
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to create platform instance")
		return
	}
	common.RespondSuccess(c, gin.H{"id": created.ID, "instance_name": created.InstanceName}, "")
}

func (h *Handler) ShowPlatformInstance(c *gin.Context) {
	result, err := service.PlatformInstanceDetails(c.Request.Context(), dao.DB, c.Param("provider_id"), c.Param("instance_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, service.ErrPlatformInstanceNotFound) {
		common.RespondError(c, common.CodeNotFound, "Platform instance not found")
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to load platform instance")
		return
	}
	common.RespondSuccess(c, result, "")
}

func (h *Handler) UpdatePlatformInstance(c *gin.Context) {
	var req service.UpdatePlatformInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ModelInfo == nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	if req.InstanceName != "" {
		if err := service.ValidateProviderInstanceName(req.InstanceName); err != nil {
			common.RespondError(c, common.CodeArgumentError, err.Error())
			return
		}
	}
	if req.ClearAPIKey {
		common.RespondError(c, common.CodeArgumentError, "clear_api_key is not supported")
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "inactive" {
		common.RespondError(c, common.CodeArgumentError, "Invalid status")
		return
	}
	err := h.service.modelProviderService.UpdatePlatformInstance(c.Request.Context(), dao.DB, c.Param("provider_id"), c.Param("instance_id"), req)
	if errors.Is(err, service.ErrInvalidPlatformModelType) {
		common.RespondError(c, common.CodeArgumentError, "Invalid model type")
		return
	}
	if errors.Is(err, service.ErrPlatformModelNameRequired) {
		common.RespondError(c, common.CodeArgumentError, "Model name is required")
		return
	}
	if errors.Is(err, service.ErrPlatformInstanceConflict) {
		common.RespondError(c, common.CodeConflict, "Platform instance already exists")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.RespondError(c, common.CodeNotFound, "Platform instance not found")
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to update platform instance")
		return
	}
	common.RespondSuccess(c, nil, "")
}

func (h *Handler) ListPlatformModels(c *gin.Context) {
	items, err := service.PlatformModels(c.Request.Context(), dao.DB, c.Param("provider_id"), c.Param("instance_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, service.ErrPlatformInstanceNotFound) {
		common.RespondError(c, common.CodeNotFound, "Platform instance not found")
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to load platform models")
		return
	}
	common.RespondSuccessList(c, items, "")
}

func (h *Handler) AddPlatformModel(c *gin.Context) {
	var req service.AddModelRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ModelName) == "" || len(req.ModelTypes) == 0 {
		common.RespondError(c, common.CodeArgumentError, "Model name and type are required")
		return
	}
	id, err := h.service.modelProviderService.AddPlatformModel(c.Request.Context(), dao.DB, c.Param("provider_id"), c.Param("instance_id"), req)
	if errors.Is(err, service.ErrInvalidPlatformModelType) {
		common.RespondError(c, common.CodeArgumentError, "Invalid model type")
		return
	}
	if errors.Is(err, service.ErrPlatformModelNameRequired) {
		common.RespondError(c, common.CodeArgumentError, "Model name is required")
		return
	}
	if errors.Is(err, service.ErrPlatformModelConflict) {
		common.RespondError(c, common.CodeConflict, "Platform model already exists")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.RespondError(c, common.CodeNotFound, "Platform instance not found")
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to add platform model")
		return
	}
	common.RespondSuccess(c, gin.H{"model_id": id}, "")
}

func (h *Handler) UpdatePlatformModel(c *gin.Context) {
	var req service.UpdatePlatformModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "inactive" {
		common.RespondError(c, common.CodeArgumentError, "Invalid status")
		return
	}
	if req.ModelTypes != nil && len(*req.ModelTypes) == 0 {
		common.RespondError(c, common.CodeArgumentError, "Invalid model type")
		return
	}
	err := service.UpdatePlatformModel(c.Request.Context(), dao.DB, c.Param("provider_id"), c.Param("instance_id"), c.Param("model_id"), req)
	if errors.Is(err, service.ErrInvalidPlatformModelType) {
		common.RespondError(c, common.CodeArgumentError, "Invalid model type")
		return
	}
	if errors.Is(err, service.ErrPlatformModelNameRequired) {
		common.RespondError(c, common.CodeArgumentError, "Model name is required")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, service.ErrPlatformInstanceNotFound) {
		common.RespondError(c, common.CodeNotFound, "Platform model not found")
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to update platform model")
		return
	}
	common.RespondSuccess(c, nil, "")
}

func (h *Handler) CheckPlatformConnection(c *gin.Context) {
	var req service.CheckConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	if err := service.CheckPlatformConnection(c.Request.Context(), dao.DB, c.Param("provider_id"), req); err != nil {
		common.RespondError(c, common.CodeDataError, "Provider connection failed")
		return
	}
	common.RespondSuccess(c, nil, "")
}

func platformDiscoveryContainsCredential(items any, apiKey string) bool {
	var values []string
	var raw any
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case string:
			if v != "" {
				values = append(values, v)
			}
		case map[string]any:
			for key, item := range v {
				lower := strings.ToLower(key)
				if service.IsProviderCredentialField(key) || strings.Contains(lower, "authorization") {
					collect(item)
				} else {
					switch item.(type) {
					case map[string]any, []any:
						collect(item)
					}
				}

			}
		case []any:
			for _, item := range v {
				collect(item)
			}
		}
	}
	if json.Unmarshal([]byte(apiKey), &raw) == nil {
		collect(raw)
	}
	if len(values) == 0 && apiKey != "" {
		values = append(values, apiKey)
	}
	data, err := json.Marshal(items)
	if err != nil {
		return true
	}
	var response any
	if json.Unmarshal(data, &response) != nil {
		return true
	}
	var contains func(any) bool
	contains = func(item any) bool {
		switch v := item.(type) {
		case string:
			for _, secret := range values {
				if containsCredential(v, secret) {
					return true
				}
			}
		case map[string]any:
			for _, child := range v {
				if contains(child) {
					return true
				}
			}
		case []any:
			for _, child := range v {
				if contains(child) {
					return true
				}
			}
		}
		return false
	}
	return contains(response)
}

// containsCredential rejects any occurrence, including embedded credentials.
func containsCredential(text, secret string) bool {
	return secret != "" && strings.Contains(text, secret)
}

func (h *Handler) DiscoverPlatformModels(c *gin.Context) {
	var req service.CheckConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	provider, config, err := service.ResolvePlatformProviderConfig(c.Request.Context(), dao.DB, c.Param("provider_id"), req)
	if err != nil {
		common.RespondError(c, common.CodeDataError, "Invalid platform provider configuration")
		return
	}
	items, err := handler.FetchProviderModels(c.Request.Context(), provider.ProviderName, config.APIKey, config.BaseURL, config.Region)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to discover provider models")
		return
	}
	if platformDiscoveryContainsCredential(items, config.APIKey) {
		common.RespondError(c, common.CodeServerError, "Failed to discover provider models")
		return
	}
	common.RespondSuccessList(c, items, "")
}
