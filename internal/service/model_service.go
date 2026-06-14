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

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/utility"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// parseModelName parses a composite model name in format "model@instance@provider" or "model@provider"
// Returns modelName, instanceName, providerName separately
func parseModelName(compositeName string) (modelName, instanceName, providerName string, err error) {
	parts := strings.Split(compositeName, "@")
	if len(parts) == 3 {
		// Format: model@instance@provider
		return parts[0], parts[1], parts[2], nil
	} else if len(parts) == 2 {
		// Format: model@provider -> instance defaults to "default"
		return parts[0], "default", parts[1], nil
	} else if len(parts) == 1 {
		return parts[0], "", "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	}

	return "", "", "", fmt.Errorf("invalid model name format: %s", compositeName)
}

func newModelDriverForBaseURL(driver modelModule.ModelDriver, providerName, region, baseURL string) (modelModule.ModelDriver, error) {
	if driver == nil {
		return nil, fmt.Errorf("provider %s driver not found", providerName)
	}

	if strings.TrimSpace(baseURL) == "" {
		return driver, nil
	}

	baseURLByRegion := map[string]string{
		region: baseURL,
	}
	if region == "" {
		baseURLByRegion["default"] = baseURL
	}

	newDriver := driver.NewInstance(baseURLByRegion)
	if newDriver == nil {
		return nil, fmt.Errorf("provider %s does not support custom base_url", providerName)
	}

	return newDriver, nil
}

func NewModelProviderService() *ModelProviderService {
	return &ModelProviderService{
		modelProviderDAO:     dao.NewTenantModelProviderDAO(),
		modelInstanceDAO:     dao.NewTenantModelInstanceDAO(),
		modelDAO:             dao.NewTenantModelDAO(),
		modelGroupDAO:        dao.NewTenantModelGroupDAO(),
		modelGroupMappingDAO: dao.NewTenantModelGroupMappingDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
	}
}

type ModelProviderService struct {
	modelProviderDAO     *dao.TenantModelProviderDAO
	modelInstanceDAO     *dao.TenantModelInstanceDAO
	modelDAO             *dao.TenantModelDAO
	modelGroupDAO        *dao.TenantModelGroupDAO
	modelGroupMappingDAO *dao.TenantModelGroupMappingDAO
	userTenantDAO        *dao.UserTenantDAO
}

// CheckConnectionRequest carries the credentials and optional instance selector
// for checking provider connectivity without creating a new model instance.
type CheckConnectionRequest struct {
	APIKey  string `json:"api_key"`
	Region  string `json:"region"`
	BaseURL string `json:"base_url"`
}

func (m *ModelProviderService) AddModelProvider(providerName, userID string) (common.ErrorCode, error) {

	_, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		return common.CodeNotFound, err
	}

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	providerID, err := utility.GenerateUUID1()
	if err != nil {
		return common.CodeServerError, errors.New("fail to get UUID")
	}

	tenantModelProvider := &entity.TenantModelProvider{
		ID:           providerID,
		ProviderName: providerName,
		TenantID:     tenantID,
	}
	err = m.modelProviderDAO.Create(tenantModelProvider)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("fail to create model provider: %s", err.Error())
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListProvidersOfTenant(userID string) ([]map[string]interface{}, common.ErrorCode, error) {

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	providerNames, err := m.modelProviderDAO.ListByID(tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	var result []map[string]interface{}
	for _, providerName := range providerNames {
		// Mirror Python's list_providers tenant branch: silently skip system-excluded
		// factory names (e.g. "Builtin", "Youdao", "FastEmbed", "BAAI",
		// "siliconflow_intl") and any stale entries whose factory is no longer in
		// the system pool. See api/apps/services/provider_api_service.py:108.
		if isExcludedTenantProvider(providerName) {
			continue
		}
		provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
		if err != nil {
			// Treat "provider not found in system pool" as a stale tenant entry
			// rather than a 500. Mirrors Python's factory_info_mapping.get(name)
			// truthy gate in api/apps/services/provider_api_service.py:108.
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			return nil, common.CodeServerError, err
		}
		result = append(result, provider)
	}

	return result, common.CodeSuccess, nil
}

// isExcludedTenantProvider returns true for system-pool names that the Python
// implementation (api/apps/services/provider_api_service.py:108) intentionally
// filters out when listing a tenant's providers.
func isExcludedTenantProvider(name string) bool {
	switch name {
	case "Youdao", "FastEmbed", "BAAI", "Builtin", "siliconflow_intl":
		return true
	}
	return false
}

func (m *ModelProviderService) DeleteModelProvider(providerName, userID string) (common.ErrorCode, error) {
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}
	tenantID := tenants[0].TenantID

	_, err = m.modelProviderDAO.DeleteByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListSupportedModels(providerName, instanceName, userID string) ([]map[string]interface{}, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, errors.New("fail to get tenant")
	}

	if len(tenants) == 0 {
		return nil, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	driver := providerInfo.ModelDriver

	// For local deployed models
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return nil, err
		}
	}

	modelList, err := driver.ListModels(apiConfig)
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for _, model := range modelList {
		result = append(result, map[string]interface{}{
			"name":          model.Name,
			"max_dimension": model.MaxDimension,
			"dimensions":    model.Dimensions,
			"max_tokens":    model.MaxTokens,
			"model_types":   model.ModelTypes,
			"thinking":      model.Thinking,
		})
		modelData := map[string]interface{}{
			"name":        model.Name,
			"dimension":   model.MaxDimension,
			"max_tokens":  model.MaxTokens,
			"model_types": model.ModelTypes,
			"thinking":    model.Thinking,
		}
		if len(model.Dimensions) > 0 {
			modelData["dimensions"] = model.Dimensions
		}
		result = append(result, modelData)
	}
	return result, nil
}

func (m *ModelProviderService) CreateProviderInstance(providerName, instanceName, apiKey, baseURL, region, userID string) (common.ErrorCode, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instanceID, err := utility.GenerateUUID1()
	if err != nil {
		return common.CodeServerError, errors.New("fail to get UUID")
	}

	extra := make(map[string]string)
	extra["region"] = region
	extra["base_url"] = baseURL
	// convert extra to string
	extraByte, err := json.Marshal(extra)
	if err != nil {
		return common.CodeServerError, errors.New("fail to marshal extra")
	}
	extraStr := string(extraByte)

	tenantModelProvider := &entity.TenantModelInstance{
		ID:           instanceID,
		InstanceName: instanceName,
		ProviderID:   provider.ID,
		APIKey:       apiKey,
		Status:       "active",
		Extra:        extraStr,
	}
	err = m.modelInstanceDAO.Create(tenantModelProvider)

	if err != nil {
		return common.CodeServerError, fmt.Errorf("fail to create model instance: %s", err.Error())
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListProviderInstances(providerName, userID string) ([]map[string]interface{}, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		// "provider not connected to this tenant" is a normal, expected state
		// (e.g. demo tenant has never added SiliconFlow). Mirrors the Python
		// contract in api/apps/services/provider_api_service.py:349-355 which
		// returns (False, "No provider found for provider '<name>'") on this
		// path. The REST layer maps that to get_error_data_result with
		// code=RetCode.DATA_ERROR (=102), so the Go port must do the same —
		// NOT a 500 server error.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("No provider found for provider '%s'", providerName)
		}
		return nil, common.CodeServerError, err
	}

	// Check if provider exists
	instances, err := m.modelInstanceDAO.GetAllInstancesByProviderID(provider.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Always emit a non-nil slice so the JSON encoder serializes [] rather
	// than null when the tenant has no instances on this provider. The
	// front-end calls .map() / .forEach() on the response array (see
	// web/src/pages/user-setting/setting-model/...) and would otherwise
	// crash on a freshly created tenant.
	result := make([]map[string]interface{}, 0, len(instances))
	for _, instance := range instances {
		// convert instance.Extra (JSON string) to map
		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		// Emit snake_case keys to match the IProviderInstance TypeScript
		// contract (web/src/interfaces/database/llm.ts) and the Python
		// `TenantModelInstanceService.query(joinedload(...))` response
		// shape. The Go port previously emitted camelCase
		// (`instanceName`/`providerID`/`apiKey`), which broke the
		// `instance.instance_name` reads in
		// web/src/pages/user-setting/setting-model/components/used-model.tsx
		// and made `useFetchInstanceModels(providerName,
		// instance.instance_name)` hit `/api/v1/providers/<p>/instances/undefined/models`.
		result = append(result, map[string]interface{}{
			"id":            instance.ID,
			"instance_name": instance.InstanceName,
			"provider_id":   instance.ProviderID,
			"api_key":       instance.APIKey,
			"status":        instance.Status,
			"extra":         instance.Extra,
		})
	}

	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowProviderInstance(providerName, instanceName, userID string) (map[string]interface{}, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// convert instance.Extra (JSON string) to map
	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Emit snake_case keys to match the IProviderInstance TypeScript
	// contract — see ListProviderInstances above. The previous shape
	// mixed conventions (`apikey` lowercase, `instanceName`/`providerID`
	// camelCase) and broke the front-end's `instance.api_key` /
	// `instance.instance_name` reads.
	result := map[string]interface{}{
		"id":            instance.ID,
		"instance_name": instance.InstanceName,
		"provider_id":   instance.ProviderID,
		"status":        instance.Status,
		"api_key":       instance.APIKey,
		"region":        extra["region"],
		"base_url":      extra["base_url"],
	}

	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowInstanceBalance(providerName, instanceName, userID string) (map[string]interface{}, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	baseURL := extra["base_url"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey
	apiConfig.BaseURL = &baseURL

	var result map[string]interface{}
	result, err = providerInfo.ModelDriver.Balance(apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) CheckConnection(providerName, apiKey, region, baseURL string, userID string) (common.ErrorCode, error) {
	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	apiKey = strings.TrimSpace(apiKey)
	region = strings.TrimSpace(region)
	baseURL = strings.TrimSpace(baseURL)
	if region == "" {
		region = "default"
	}

	driver := providerInfo.ModelDriver
	if strings.EqualFold(providerInfo.Class, "local") {
		if baseURL == "" {
			return common.CodeDataError, fmt.Errorf("base_url is required for local provider %s", providerName)
		}

		var err error
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return common.CodeServerError, err
		}
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: &apiKey,
		Region: &region,
	}

	err := driver.CheckConnection(apiConfig)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) CheckInstanceConnection(providerName, instanceName, userID string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return common.CodeServerError, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	driver := providerInfo.ModelDriver
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return common.CodeServerError, err
		}
	}

	err = driver.CheckConnection(apiConfig)
	if err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListTasks(providerName, instanceName, userID string) ([]modelModule.ListTaskStatus, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	driver := providerInfo.ModelDriver
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return nil, common.CodeServerError, err
		}
	}

	var listTaskResponse []modelModule.ListTaskStatus
	listTaskResponse, err = driver.ListTasks(apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return listTaskResponse, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowTask(providerName, instanceName, taskID, userID string) (*modelModule.TaskResponse, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	driver := providerInfo.ModelDriver
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return nil, common.CodeServerError, err
		}
	}

	var taskResponse *modelModule.TaskResponse
	taskResponse, err = driver.ShowTask(taskID, apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return taskResponse, common.CodeSuccess, nil
}

// ListTenantAddedModels returns the list of models the tenant has "added"
// across all of their provider instances. It is the Go port of Python's
// models_api_service.list_tenant_added_models
// (api/apps/services/models_api_service.py:300) and is the response
// contract for GET /api/v1/models — the endpoint that
// web/src/hooks/use-llm-request.tsx → useFetchAllAddedModels consumes.
//
// Per the Python algorithm, for each (provider × instance) we cross-reference the factory catalog (internal/entity/models/model.go
// ProviderManager.Providers) with the per-tenant overrides in
// tenant_model:
//
//	active_model_types   = tenant_model rows with status='active'
//	inactive_model_types = tenant_model rows with status='inactive'
//	factory_model_types  = provider.Models[i].ModelTypes
//	model_types = (factory ∪ active) \ inactive
//
// The Go port never WRITES to tenant_model, so in practice every model
// from the factory catalog is treated as added unless explicitly
// disabled (which today can only happen via SQL — the Go port has no
// enable/disable endpoint path that mutates tenant_model). This is
// intentional: the previous Go contract mistakenly routed /api/v1/models
// to ListTenantDefaultModels (which only enumerates the 6-7 default
// tenant fields and returned `[]` for any tenant without defaults),
// breaking the front-end's "View Models" list entirely.
func (m *ModelProviderService) ListTenantAddedModels(userID, modelTypeFilter string) ([]map[string]interface{}, common.ErrorCode, error) {
	// Resolve tenant. Match the convention used elsewhere in this file
	// (see ListProviderInstances, DropProviderInstances): take the first
	// tenant where the user has role=owner.
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(tenants) == 0 {
		// No tenant for the user → empty list, code=0. Python returns
		// get_result(data=[]) for the same path.
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}
	tenantID := tenants[0].TenantID

	if modelTypeFilter != "" {
		modelTypeFilter = strings.ToLower(strings.TrimSpace(modelTypeFilter))
	}

	providers, err := m.modelProviderDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(providers) == 0 {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}

	providerIDs := make([]string, 0, len(providers))
	providerInfoByID := make(map[string]*entity.TenantModelProvider, len(providers))
	for _, p := range providers {
		providerIDs = append(providerIDs, p.ID)
		providerInfoByID[p.ID] = p
	}

	instances, err := m.modelInstanceDAO.GetByProviderIDs(providerIDs)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(instances) == 0 {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}

	instanceIDs := make([]string, 0, len(instances))
	instanceInfoByID := make(map[string]*entity.TenantModelInstance, len(instances))
	for _, inst := range instances {
		instanceIDs = append(instanceIDs, inst.ID)
		instanceInfoByID[inst.ID] = inst
	}

	// Per-tenant enable/disable overrides. In the Go port this is
	// typically empty (no writers), but we still honor active/inactive
	// rows for correctness and parity.
	modelRecords, err := m.modelDAO.GetModelsByProviderIDsAndInstanceIDs(providerIDs, instanceIDs)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	activeByKey := make(map[string][]string)
	inactiveByKey := make(map[string][]string)
	for _, rec := range modelRecords {
		key := rec.ProviderID + "@" + rec.InstanceID + "@" + rec.ModelName
		if rec.Status == "inactive" {
			inactiveByKey[key] = append(inactiveByKey[key], rec.ModelType)
		} else {
			activeByKey[key] = append(activeByKey[key], rec.ModelType)
		}
	}

	// Group instances by provider_name for the outer loop.
	instancesByProviderName := make(map[string][]*entity.TenantModelInstance)
	for _, inst := range instances {
		p, ok := providerInfoByID[inst.ProviderID]
		if !ok {
			continue
		}
		instancesByProviderName[p.ProviderName] = append(instancesByProviderName[p.ProviderName], inst)
	}

	providerManager := dao.GetModelProviderManager()
	added := make([]map[string]interface{}, 0)

	// factory rank is not present in the Go entity.Provider struct, so we
	// follow Python's stable ordering intent (factory rank desc, then
	// provider_name, then instance_name) by simply iterating providers in
	// the tenant's own order. With one provider today this is a no-op.
	for _, p := range providers {
		factory := providerManager.FindProvider(p.ProviderName)
		if factory == nil {
			// Factory not in the static catalog. The tenant has linked
			// a provider we have no model list for. Skip — there is
			// nothing to expose.
			continue
		}
		factoryInstances := instancesByProviderName[p.ProviderName]
		if len(factoryInstances) == 0 {
			continue
		}
		for _, llm := range factory.Models {
			if modelTypeFilter != "" {
				match := false
				for _, t := range llm.ModelTypes {
					if strings.EqualFold(t, modelTypeFilter) {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}
			for _, inst := range factoryInstances {
				key := p.ID + "@" + inst.ID + "@" + llm.Name
				// Set-based merge: factory types ∪ active overrides \ inactive overrides.
				mergedSet := make(map[string]struct{}, len(llm.ModelTypes)+len(activeByKey[key]))
				for _, t := range llm.ModelTypes {
					mergedSet[t] = struct{}{}
				}
				for _, t := range activeByKey[key] {
					mergedSet[t] = struct{}{}
				}
				for _, t := range inactiveByKey[key] {
					delete(mergedSet, t)
				}
				if len(mergedSet) == 0 {
					continue
				}
				merged := make([]string, 0, len(mergedSet))
				for t := range mergedSet {
					merged = append(merged, t)
				}
				added = append(added, map[string]interface{}{
					"model_type":    merged,
					"name":          llm.Name,
					"provider_id":   inst.ProviderID,
					"provider_name": p.ProviderName,
					"instance_id":   inst.ID,
					"instance_name": inst.InstanceName,
				})
			}
		}
	}

	return added, common.CodeSuccess, nil
}

func (m *ModelProviderService) AlterProviderInstance(providerName, instanceName, newInstanceName, apiKey, userID string) (common.ErrorCode, error) {
	return common.CodeSuccess, nil
}
func (m *ModelProviderService) DropProviderInstances(providerName, userID string, instances []string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		// Tenant hasn't connected this provider. The DELETE request is a
		// no-op in that case — mirrors Python's drop_provider_instances in
		// api/apps/services/provider_api_service.py, which simply iterates
		// the (empty) instance list. The previous contract bubbled
		// "record not found" as a 500, which broke the front-end "remove
		// instance" flow when the UI's snapshot was slightly stale.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.CodeSuccess, nil
		}
		return common.CodeServerError, err
	}

	for _, instanceName := range instances {
		// Get model instance
		var tenantModelInstance *entity.TenantModelInstance
		tenantModelInstance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
		if err != nil {
			// The instance name isn't in the DB (e.g. UI holds a stale id
			// after the user already removed it on another tab). Match
			// Python and skip silently instead of returning 500.
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return common.CodeServerError, err
		}

		// Delete all models of this instance
		var count int64 = 0
		count, err = m.modelDAO.DeleteByProviderIDAndInstanceID(provider.ID, tenantModelInstance.ID)
		if err != nil {
			return common.CodeServerError, err
		}

		// Delete model instance
		count, err = m.modelInstanceDAO.DeleteByProviderIDAndInstanceName(provider.ID, instanceName)
		if err != nil {
			return common.CodeServerError, err
		}

		if count == 0 {
			return common.CodeNotFound, errors.New("provider instance not found")
		}
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) DropInstanceModels(providerName, instanceName, userID string, models []string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	var modelInstance *entity.TenantModelInstance
	modelInstance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	for _, modelName := range models {
		// Delete all models of this instance
		var count int64 = 0
		count, err = m.modelDAO.DeleteByProviderIDAndInstanceIDAndModelName(provider.ID, modelInstance.ID, modelName)
		if err != nil {
			return common.CodeServerError, err
		}

		if count == 0 {
			return common.CodeNotFound, fmt.Errorf("model: %s not found", modelName)
		}
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListInstanceModels(providerName, instanceName, userID string) ([]map[string]interface{}, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, err
	}

	// Get instance
	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, err
	}

	// Get all models for this instance
	disabledModels, err := m.modelDAO.GetModelsByInstanceID(instance.ID)
	if err != nil {
		return nil, err
	}

	allModels, err := dao.GetModelProviderManager().ListModels(providerName)

	// insert models name into a set
	modelNames := make(map[string]bool)
	for _, model := range disabledModels {
		if model.Status == "active" {
			modelData := map[string]interface{}{
				"name": model.ModelName,
			}
			allModels = append(allModels, modelData)
		} else {
			modelNames[model.ModelName] = true
		}

	}

	for _, model := range allModels {
		// convert model["name"] to string
		modelName := model["name"].(string)
		if modelNames[modelName] {
			model["status"] = "inactive"
		} else {
			model["status"] = "active"
		}
	}

	return allModels, nil
}

func (m *ModelProviderService) UpdateModelStatus(providerName, instanceName, modelName, userID, status string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	model, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		var modelID string
		modelID, err = utility.GenerateUUID1()
		if err != nil {
			return common.CodeServerError, errors.New("fail to get UUID")
		}

		var modelSchema *modelModule.Model
		modelSchema, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		// Get model info from provider
		model = &entity.TenantModel{
			ID:         modelID,
			ModelName:  modelName,
			ModelType:  modelSchema.ModelTypes[0],
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			Status:     status,
		}
		err = m.modelDAO.Create(model)
		if err != nil {
			return common.CodeServerError, errors.New("fail to create model")
		}
		return common.CodeSuccess, nil
	}

	count, err := m.modelDAO.DeleteByModelID(model.ID)
	if err != nil {
		return common.CodeServerError, err
	}
	if count == 0 {
		return common.CodeNotFound, errors.New("model not found")
	}

	return common.CodeSuccess, nil
}

type ModelInstanceAndProviderInfo struct {
	ProviderEntity *entity.TenantModelProvider
	ProviderInfo   *modelModule.Provider
	InstanceEntity *entity.TenantModelInstance
	ModelEntity    *entity.TenantModel
	ModelInfo      *modelModule.Model
	APIConfig      *modelModule.APIConfig
}

func (m *ModelProviderService) getModelInstanceAndProviderByName(providerName, instanceName, modelName *string, userID string, apiConfig *modelModule.APIConfig) (*ModelInstanceAndProviderInfo, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, err
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	providerEntity, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, *providerName)
	if err != nil {
		return nil, err
	}

	instanceEntity, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(providerEntity.ID, *instanceName)
	if err != nil {
		return nil, err
	}

	modelEntity, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(providerEntity.ID, instanceEntity.ID, *modelName)
	if err != nil {
		// Not found model
		modelEntity = nil
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(*providerName)
	if providerInfo == nil {
		return nil, errors.New("provider not found")
	}

	modelInfo, err := dao.GetModelProviderManager().GetModelByName(*providerName, *modelName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("provider %s model %s not found", *providerName, *modelName))
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instanceEntity.Extra), &extra)
	if err != nil {
		return nil, err
	}

	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}

	region := extra["region"]
	baseURL := extra["base_url"]

	apiConfig.ApiKey = &instanceEntity.APIKey
	apiConfig.BaseURL = &baseURL
	apiConfig.Region = &region

	var result = &ModelInstanceAndProviderInfo{
		ProviderEntity: providerEntity,
		ProviderInfo:   providerInfo,
		InstanceEntity: instanceEntity,
		ModelEntity:    modelEntity,
		ModelInfo:      modelInfo,
		APIConfig:      apiConfig,
	}

	return result, nil
}

func (m *ModelProviderService) getModelInstanceAndProviderByID(modelID *string, userID string, apiConfig *modelModule.APIConfig) (*ModelInstanceAndProviderInfo, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, err
	}

	tenantID := tenants[0].TenantID

	modelEntity, err := m.modelDAO.GetByID(*modelID)
	if err != nil {
		return nil, err
	}

	instanceEntity, err := m.modelInstanceDAO.GetByID(modelEntity.InstanceID)
	if err != nil {
		return nil, err
	}

	providerEntity, err := m.modelProviderDAO.GetByID(instanceEntity.ProviderID)
	if err != nil {
		return nil, err
	}

	if providerEntity.TenantID != tenantID {
		return nil, errors.New("provider not found")
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerEntity.ProviderName)
	if providerInfo == nil {
		return nil, errors.New("provider not found")
	}

	modelInfo, err := dao.GetModelProviderManager().GetModelByName(providerEntity.ProviderName, modelEntity.ModelName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("provider %s model %s not found", providerEntity.ProviderName, modelEntity.ModelName))
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instanceEntity.Extra), &extra)
	if err != nil {
		return nil, err
	}

	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}

	region := extra["region"]
	baseURL := extra["base_url"]

	apiConfig.ApiKey = &instanceEntity.APIKey
	apiConfig.BaseURL = &baseURL
	apiConfig.Region = &region

	var result = &ModelInstanceAndProviderInfo{
		ProviderEntity: providerEntity,
		ProviderInfo:   providerInfo,
		InstanceEntity: instanceEntity,
		ModelEntity:    modelEntity,
		ModelInfo:      modelInfo,
		APIConfig:      apiConfig,
	}

	return result, nil
}

// ChatToModelWithMessages sends messages to the model with messages array
func (m *ModelProviderService) ChatToModelWithMessages(providerName, instanceName, modelName, modelID *string, userID string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig) (*modelModule.ChatResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ChatConfig{}
	}
	modelConfig.ModelClass = info.ModelInfo.Class

	var response *modelModule.ChatResponse
	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["chat"] && !info.ModelInfo.ModelTypeMap["vision"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "chat" && info.ModelEntity.ModelType != "vision" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	response, err = modelDriver.ChatWithMessages(*modelName, messages, info.APIConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

// ChatToModelStreamWithSender streams chat response directly via sender function ( the best performance, no channel)
func (m *ModelProviderService) ChatToModelStreamWithSender(providerName, instanceName, modelName, modelID *string, userID string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) (common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ChatConfig{}
	}
	modelConfig.ModelClass = info.ModelInfo.Class

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "chat" && info.ModelEntity.ModelType != "vision" {
				return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return common.CodeServerError, err
			}
		} else {
			return common.CodeServerError, errors.New("model is inactive")
		}
	}

	err = modelDriver.ChatStreamlyWithSender(*modelName, messages, apiConfig, modelConfig, sender)
	if err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

func validateEmbeddingDimension(model *modelModule.Model, requested int) error {
	if requested <= 0 || model == nil {
		return nil
	}

	if len(model.Dimensions) > 0 {
		for _, dim := range model.Dimensions {
			if dim == requested {
				return nil
			}
		}
		return fmt.Errorf(
			"dimension %d is not supported by model %s, supported dimensions: %v",
			requested,
			model.Name,
			model.Dimensions,
		)
	}
	if model.MaxDimension != nil && requested > *model.MaxDimension {
		return fmt.Errorf(
			"dimension %d is not supported by model %s, max dimension: %d",
			requested,
			model.Name,
			*model.MaxDimension,
		)
	}

	return nil
}

// EmbedText sends texts to the embedding model
func (m *ModelProviderService) EmbedText(providerName, instanceName, modelName, modelID *string, userID string, texts []string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.EmbeddingConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["embedding"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an embedding model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "embedding" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an embedding model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	if err = validateEmbeddingDimension(info.ModelInfo, modelConfig.Dimension); err != nil {
		return nil, common.CodeBadRequest, err
	}

	var response []modelModule.EmbeddingData
	response, err = modelDriver.Embed(modelName, texts, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil || len(response) == 0 {
		return nil, common.CodeServerError, errors.New("empty embed response")
	}

	return response, common.CodeSuccess, nil
}

// RerankDocument sends texts to the embedding model
func (m *ModelProviderService) RerankDocument(providerName, instanceName, modelName, modelID *string, userID, query string, documents []string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.RerankConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["rerank"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a rerank model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "rerank" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a rerank model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.RerankResponse
	response, err = modelDriver.Rerank(modelName, query, documents, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	return response, common.CodeSuccess, nil
}

// TranscribeAudio transcribe audio file to text
func (m *ModelProviderService) TranscribeAudio(providerName, instanceName, modelName, modelID *string, userID string, audioFile *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ASRConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["asr"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "asr" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.ASRResponse
	response, err = modelDriver.TranscribeAudio(modelName, audioFile, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

// TranscribeAudioStream transcribe audio file to text stream directly via sender function ( the best performance, no channel)
func (m *ModelProviderService) TranscribeAudioStream(providerName, instanceName, modelName, modelID *string, userID string, audioFile *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ASRConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["asr"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "asr" {
				return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return common.CodeServerError, err
			}
		} else {
			return common.CodeServerError, errors.New("model is inactive")
		}
	}

	err = modelDriver.TranscribeAudioWithSender(modelName, audioFile, apiConfig, modelConfig, sender)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

// AudioSpeech convert audio to speech
func (m *ModelProviderService) AudioSpeech(providerName, instanceName, modelName, modelID *string, userID string, audioContent *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.TTSConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["tts"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "tts" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.TTSResponse
	response, err = modelDriver.AudioSpeech(modelName, audioContent, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

func (m *ModelProviderService) AudioSpeechStream(providerName, instanceName, modelName, modelID *string, userID string, audioContent *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.TTSConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["tts"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "tts" {
				return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return common.CodeServerError, err
			}
		} else {
			return common.CodeServerError, errors.New("model is inactive")
		}
	}

	err = modelDriver.AudioSpeechWithSender(modelName, audioContent, apiConfig, modelConfig, sender)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) OCRFile(providerName, instanceName, modelName, modelID *string, userID string, content []byte, url *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.OCRConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["ocr"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an OCR model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "ocr" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an OCR model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.OCRFileResponse
	response, err = modelDriver.OCRFile(modelName, content, url, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

func (m *ModelProviderService) ParseFile(providerName, instanceName, modelName, modelID *string, userID string, content []byte, url *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ParseFileConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["doc_parse"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a ParseFile model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if info.ModelEntity.ModelType != "doc_parse" {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a ParseFile model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.ParseFileResponse
	response, err = modelDriver.ParseFile(modelName, content, url, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

// GetEmbeddingModel returns an EmbeddingModel wrapper for the given tenant
func (m *ModelProviderService) GetEmbeddingModel(tenantID, compositeModelName string) (*modelModule.EmbeddingModel, error) {
	driver, modelName, apiConfig, maxTokens, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens), nil
}

// GetChatModel  returns a ChatModel wrapper for the given tenant
func (m *ModelProviderService) GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error) {
	driver, modelName, apiConfig, _, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig), nil
}

// GetRerankModel returns a RerankModel wrapper for the given tenant
func (m *ModelProviderService) GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error) {
	driver, modelName, apiConfig, _, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModel(driver, &modelName, apiConfig), nil
}

type AddModelRequest struct {
	ProviderName string         `json:"provider_name"`
	InstanceName string         `json:"instance_name"`
	Models       []ModelRequest `json:"models"`
}

func (m *ModelProviderService) GetTenantDefaultModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	if modelType == entity.ModelTypeOCR {
		return nil, "", nil, 0, fmt.Errorf("OCR model name is required")
	}

	tenantSvc := NewTenantService()
	modelName, err := tenantSvc.GetDefaultModelName(tenantID, modelType)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("failed to get default model name for type %s: %w", modelType, err)
	}
	if modelName == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}

	return m.GetModelConfigFromProviderInstance(tenantID, modelType, modelName)
}

// GetModelTypeByName returns the list of model types the given model is enrolled as.
func (m *ModelProviderService) GetModelTypeByName(tenantID, modelName string) ([]entity.ModelType, error) {
	common.Debug("GetModelTypeByName",
		zap.String("tenantID", tenantID),
		zap.String("modelName", modelName))

	pureModelName, instanceName, providerName, err := parseModelName(modelName)
	if err != nil {
		return nil, err
	}

	// Direct provider lookup
	provider, provErr := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if provErr != nil {
		return nil, fmt.Errorf("provider %q lookup failed: %w", providerName, provErr)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %q not found for model %q", providerName, modelName)
	}

	// Direct instance lookup
	instance, instErr := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if instErr != nil {
		return nil, fmt.Errorf("instance %q lookup failed: %w", instanceName, instErr)
	}
	if instance == nil {
		return nil, fmt.Errorf("instance %q not found for model %q", instanceName, modelName)
	}

	// Direct model lookup
	modelObjs, modelErr := m.modelDAO.GetModelsByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, pureModelName)
	if modelErr == nil && len(modelObjs) > 0 {
		types := make([]entity.ModelType, 0, len(modelObjs))
		for _, obj := range modelObjs {
			types = append(types, entity.ModelType(obj.ModelType))
		}
		return types, nil
	}

	// Fallback: factory LLM catalog
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]

	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
	// Use the case-insensitive FindProvider / findModel helpers. The Go's
	// conf/models/*.json files use mixed case ("SiliconFlow") while the
	// Python's conf/llm_factories.json uses all-caps ("SILICONFLOW"); a strict
	// `==` here would fail for any case mismatch. The rest of the Go codebase
	// uses these helpers too.
	targetProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
	if targetProvider == nil {
		return nil, fmt.Errorf("model provider config not found: %s", providerName)
	}
	for i := range targetProvider.Models {
		if strings.EqualFold(targetProvider.Models[i].Name, pureModelName) {
			if len(targetProvider.Models[i].ModelTypes) == 0 {
				return nil, fmt.Errorf("model %q has no model_types in factory catalog", pureModelName)
			}
			return []entity.ModelType{entity.ModelType(targetProvider.Models[i].ModelTypes[0])}, nil
		}
	}
	return nil, fmt.Errorf("model %q not found for model %q", pureModelName, modelName)
}

type AddCustomModelRequest struct {
	ProviderName string   `json:"provider_name"`
	InstanceName string   `json:"instance_name"`
	ModelName    string   `json:"model_name"`
	ModelTypes   []string `json:"model_types"`
	MaxTokens    int      `json:"max_tokens"`
	Thinking     *bool    `json:"thinking"`
}

type ModelRequest struct {
	ModelName  string   `json:"model_name"`
	ModelTypes []string `json:"model_types"`
	MaxTokens  int      `json:"max_tokens"`
	Thinking   *bool    `json:"thinking"`
}

func (m *ModelProviderService) AddModel(request *AddModelRequest, userID string) (common.ErrorCode, error) {
	if request == nil {
		return common.CodeBadRequest, errors.New("request is required")
	}
	if len(request.Models) == 0 {
		return common.CodeBadRequest, errors.New("models is required")
	}

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, request.ProviderName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, request.InstanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	seen := make(map[string]struct{})
	models := make([]*entity.TenantModel, 0, len(request.Models))

	for _, model := range request.Models {
		modelName := strings.TrimSpace(model.ModelName)
		if len(model.ModelTypes) == 0 {
			return common.CodeBadRequest, errors.New("model types is required")
		}
		modelType := strings.TrimSpace(model.ModelTypes[0])

		if modelName == "" {
			return common.CodeBadRequest, errors.New("model name is required")
		}
		if modelType == "" {
			return common.CodeBadRequest, errors.New("model type is required")
		}

		duplicateKey := strings.ToLower(modelName)
		if _, ok := seen[duplicateKey]; ok {
			return common.CodeConflict, fmt.Errorf("duplicate model in request: %s", modelName)
		}
		seen[duplicateKey] = struct{}{}

		_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
		if err == nil {
			return common.CodeConflict, fmt.Errorf("model already exists: %s", modelName)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return common.CodeServerError, err
		}

		var modelID string
		modelID, err = utility.GenerateUUID1()
		if err != nil {
			return common.CodeServerError, errors.New("fail to get UUID")
		}

		extra := map[string]interface{}{
			"max_tokens":  model.MaxTokens,
			"model_types": []string{modelType},
		}
		if model.Thinking != nil {
			extra["thinking"] = *model.Thinking
		}

		var extraByte []byte
		extraByte, err = json.Marshal(extra)
		if err != nil {
			return common.CodeServerError, errors.New("fail to marshal extra")
		}

		models = append(models, &entity.TenantModel{
			ID:         modelID,
			ModelName:  modelName,
			ModelType:  modelType,
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			Status:     "active",
			Extra:      string(extraByte),
		})
	}

	if err = m.modelDAO.CreateBatch(models); err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

// GetModelConfigFromProviderInstance get model config from provider instance
// modelName@instance@provider or
// "model@provider" — the provider is required and is looked up directly via
// tenant_model_provider. For 2-part names the instance defaults to "default".
// If the model is enrolled in tenant_model, that row is used (and INACTIVE rows
// raise). Otherwise, the factory's LLM catalog is consulted, with
// region=intl + siliconflow redirected to the siliconflow_intl factory.
func (m *ModelProviderService) GetModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	common.Debug("GetModelConfigFromProviderInstance",
		zap.String("tenantID", tenantID),
		zap.String("modelName", modelName),
		zap.String("modelType", string(modelType)))

	// TEI builtin embedding short-circuit
	if modelType == entity.ModelTypeEmbedding && strings.Contains(os.Getenv("COMPOSE_PROFILES"), "tei-") {
		parts := strings.Split(modelName, "@")
		teiPure := parts[0]
		teiProvider := ""
		switch len(parts) {
		case 2:
			teiProvider = parts[1]
		case 3:
			teiProvider = parts[2]
		}
		if teiPure == os.Getenv("TEI_MODEL") && (teiProvider == "Builtin" || teiProvider == "") {
			builtinDriver := modelModule.GetBuiltinEmbeddingModel(teiPure)
			if builtinDriver == nil {
				return nil, "", nil, 0, fmt.Errorf("builtin (TEI) embedding model %q not found", teiPure)
			}
			teiBaseURL := os.Getenv("TEI_BASE_URL")
			apiConfig := &modelModule.APIConfig{ApiKey: nil, Region: nil, BaseURL: &teiBaseURL}
			return builtinDriver, teiPure, apiConfig, 0, nil
		}
	}

	// Generic Builtin provider short-circuit. The tenant_model_provider /
	// tenant_model_instance tables don't have a row for "Builtin" because it's
	// a local service, not a tenant-enrolled provider — so the direct lookups
	// below would raise. Mirrors the private getModelConfig's Builtin branch.
	//
	// Gated on ModelTypeEmbedding because the Builtin driver here is the TEI
	// embedding endpoint; the underlying BuiltinModel's Chat/Rerank/AudioSpeech
	// /OCR methods all return hard "not supported" errors. A chat/rerank/etc.
	// request that names a Builtin provider must fall through to the standard
	// branch, which surfaces an accurate "provider not found" instead of
	// handing back an embedding-only driver.
	parts := strings.Split(modelName, "@")
	if modelType == entity.ModelTypeEmbedding && len(parts) >= 2 && parts[len(parts)-1] == "Builtin" {
		pureModelName := parts[0]
		builtinDriver := modelModule.GetBuiltinEmbeddingModel(pureModelName)
		if builtinDriver == nil {
			return nil, "", nil, 0, fmt.Errorf("builtin embedding model %q not found", pureModelName)
		}
		apiKey := ""
		region := ""
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}
		maxTokens := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName("Builtin", pureModelName); mi != nil {
			if mi.MaxTokens == nil {
				maxTokens = 0
			} else {
				maxTokens = *mi.MaxTokens
			}
		}
		return builtinDriver, pureModelName, apiConfig, maxTokens, nil
	}

	pureModelName, instanceName, providerName, err := parseModelName(modelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	// Direct provider lookup
	provider, provErr := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if provErr != nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q lookup failed: %w", providerName, provErr)
	}
	if provider == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q not found for model %q", providerName, modelName)
	}

	// Direct instance lookup
	instance, instErr := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if instErr != nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q lookup failed: %w", instanceName, instErr)
	}
	if instance == nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q not found for model %q", instanceName, modelName)
	}

	// Decode api_key and extra fields from the instance row
	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	// Direct model lookup
	modelObj, modelErr := m.modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
		provider.ID, instance.ID, string(modelType), pureModelName,
	)
	switch {
	case modelErr == nil:
		// Happy path: tenant enrolled this model.
		// INACTIVE check
		if modelObj.Status == "inactive" {
			return nil, "", nil, 0, fmt.Errorf("model %q is disabled", modelName)
		}

		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", providerName)
		}
		driver, driverErr := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, baseURL)
		if driverErr != nil {
			return nil, "", nil, 0, driverErr
		}
		maxTokens := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName(providerName, pureModelName); mi != nil {
			if mi.MaxTokens == nil {
				maxTokens = 0
			} else {
				maxTokens = *mi.MaxTokens
			}
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
		return driver, modelObj.ModelName, apiConfig, maxTokens, nil
	case errors.Is(modelErr, gorm.ErrRecordNotFound):
		// Tenant hasn't enrolled this model. Fall through to the factory catalog.
		common.Debug("GetModelConfigFromProviderInstance: tenant has no row for model, falling back to factory catalog",
			zap.String("tenantID", tenantID),
			zap.String("providerName", providerName),
			zap.String("instanceName", instanceName),
			zap.String("modelType", string(modelType)),
			zap.String("modelName", pureModelName))
	default:
		// Surface unexpected DAO errors (e.g. transient DB failure) instead of
		// silently resolving from the factory catalog — that would mask disabled
		// or tenant-specific configurations.
		return nil, "", nil, 0, fmt.Errorf("model %q lookup failed: %w", modelName, modelErr)
	}

	// Fallback: factory LLM catalog
	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
	// Use the case-insensitive FindProvider / findModel helpers. The Go's
	// conf/models/*.json files use mixed case ("SiliconFlow") while the
	// Python's conf/llm_factories.json uses all-caps ("SILICONFLOW"); a strict
	// `==` here would fail for any case mismatch even though the Python passes
	// (its FACTORY_LLM_INFOS and the parsed provider_name happen to agree on
	// casing). The rest of the Go codebase uses these helpers too.
	targetProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
	if targetProvider == nil {
		return nil, "", nil, 0, fmt.Errorf("model provider config not found: %s", providerName)
	}
	var llmInfo *modelModule.Model
	for i := range targetProvider.Models {
		if strings.EqualFold(targetProvider.Models[i].Name, pureModelName) {
			llmInfo = targetProvider.Models[i]
			break
		}
	}
	if llmInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("model config not found: %s", modelName)
	}
	driver, driverErr := newModelDriverForBaseURL(targetProvider.ModelDriver, providerName, region, baseURL)
	if driverErr != nil {
		return nil, "", nil, 0, driverErr
	}
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	maxTokens := 0
	if llmInfo.MaxTokens != nil {
		maxTokens = *llmInfo.MaxTokens
	}
	return driver, llmInfo.Name, apiConfig, maxTokens, nil
}

// getModelConfig returns the model driver, model name, API config, and max tokens for a model
func (m *ModelProviderService) getModelConfig(tenantID, compositeModelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelName, instanceName, providerName, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	// Check if provider exists (skip for Builtin provider)
	var providerID string
	if providerName != "Builtin" {
		provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
		if err != nil {
			return nil, "", nil, 0, err
		}
		if provider == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %s not found", providerName)
		}
		providerID = provider.ID
	} else {
		common.Debug("getModelConfig skipping provider lookup for Builtin")
	}

	// Get instance (skip for Builtin provider since it doesn't use tenant_model_instance)
	var instance *entity.TenantModelInstance
	if providerName != "Builtin" {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(providerID, instanceName)
		if err != nil {
			return nil, "", nil, 0, err
		}
		if instance == nil {
			return nil, "", nil, 0, fmt.Errorf("instance %s not found for provider %s", instanceName, providerName)
		}
		common.Debug("getModelConfig instance found", zap.String("instanceName", instanceName))
	}

	var extra map[string]string
	var region string
	if instance != nil {
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, "", nil, 0, err
		}
		region = extra["region"]
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		if providerName != "Builtin" {
			return nil, "", nil, 0, fmt.Errorf("model provider config not found: %s", providerName)
		}
	}

	// Get model info to extract max_tokens
	modelInfo, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	maxTokens := 0
	if err == nil && modelInfo != nil {
		if modelInfo.MaxTokens == nil {
			maxTokens = 0
		} else {
			maxTokens = *modelInfo.MaxTokens
		}
	}

	// For Builtin provider, use empty APIKey and skip tenant_model lookup
	var apiKey string
	if providerName == "Builtin" {
		apiKey = ""
		// For Builtin, we need to get the ModelDriver from somewhere
		// Since Builtin models are handled locally, we return a special driver
		builtinDriver := modelModule.GetBuiltinEmbeddingModel(modelName)
		if builtinDriver == nil {
			return nil, "", nil, 0, fmt.Errorf("builtin embedding model %s not found", modelName)
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}
		return builtinDriver, modelName, apiConfig, maxTokens, nil
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(providerID, instance.ID, modelName)
	if err != nil {
		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, "", nil, 0, fmt.Errorf("provider %s model %s not found", providerName, modelName)
		}
	}
	apiKey = instance.APIKey

	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}
	return providerInfo.ModelDriver, modelName, apiConfig, maxTokens, nil
}

// ListAllModels list all models
func (m *ModelProviderService) ListAllModels(pageIndex, pageSize int) ([]map[string]interface{}, error) {
	models, err := dao.GetModelProviderManager().ListAllModels()
	if err != nil {
		return nil, err
	}
	if pageSize > 0 && pageIndex >= 0 && pageIndex*pageSize < len(models) {
		return models[pageIndex*pageSize : (pageIndex+1)*pageSize], nil
	}
	return models, nil
}

func (m *ModelProviderService) ShowModel(modelName string) (*modelModule.Model, error) {
	return dao.GetModelProviderManager().GetModelByNameOrAlias(modelName), nil
}
